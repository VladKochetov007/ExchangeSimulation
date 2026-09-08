package exchange

import "testing"

type malformedExpiryInstrument struct {
	Instrument
	expiry      int64
	settlement  int64
	panicCash   bool
	negativeFee bool
}

func (m *malformedExpiryInstrument) ExpiryCashFlow(size, entryPrice, settlementPrice, basePrecision int64) int64 {
	if m.panicCash {
		panic("test expiry arithmetic failure")
	}
	return 0
}

func (m *malformedExpiryInstrument) DeliveryFee(size, settlementPrice, basePrecision int64) int64 {
	if m.negativeFee {
		return -1
	}
	return 0
}

func (m *malformedExpiryInstrument) ExpiryNano() int64 { return m.expiry }

func (m *malformedExpiryInstrument) ObserveSettlement(int64, int64) {}

func (m *malformedExpiryInstrument) SettlementPrice() (int64, error) { return m.settlement, nil }

func expiryCohortOption(symbol string, expiry int64) *EuropeanOption {
	option := NewEuropeanOption(symbol, "ABC", "USD", "ABC/USD", 1, 1, 1, 1, 0, expiry, true)
	option.DeliveryFeeBps = 0
	return option
}

func TestSameExpiryOptionCohortSettlesAgainstAggregateWalletState(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	first := expiryCohortOption("ABC-C-FIRST", clock.now)
	second := expiryCohortOption("ABC-C-SECOND", clock.now)
	first.ObserveSettlement(100, clock.now)
	second.ObserveSettlement(100, clock.now)
	ex.AddInstrument(first)
	ex.AddInstrument(second)
	for clientID, balance := range map[uint64]int{1: 50, 2: 1_000, 3: 1_000} {
		ex.ConnectNewClient(clientID, nil, &FixedFee{})
		ex.AddPerpBalance(clientID, "USD", int64(balance))
	}

	// Client 1 is short the first option and long the second. Each individual
	// settlement would temporarily drive its wallet negative, but the same-
	// expiry portfolio is solvent when both cash flows are committed together.
	ex.Positions.UpdatePosition(1, first.Symbol(), 1, 0, Sell, PositionBoth)
	ex.Positions.UpdatePosition(2, first.Symbol(), 1, 0, Buy, PositionBoth)
	ex.Positions.UpdatePosition(1, second.Symbol(), 1, 0, Buy, PositionBoth)
	ex.Positions.UpdatePosition(3, second.Symbol(), 1, 0, Sell, PositionBoth)

	ex.CheckExpiries()

	for _, symbol := range []string{first.Symbol(), second.Symbol()} {
		if ex.Instruments[symbol] != nil || ex.Books[symbol] != nil {
			t.Fatalf("same-expiry option %s was not settled atomically", symbol)
		}
	}
	if got, want := ex.Clients[1].PerpBalance("USD"), int64(50); got != want {
		t.Fatalf("aggregate wallet balance = %d, want %d", got, want)
	}
	for _, clientID := range []uint64{2, 3} {
		if position := ex.Positions.GetPosition(clientID, first.Symbol()); position != nil && position.Size != 0 {
			t.Fatalf("client %d retained first option position: %#v", clientID, position)
		}
	}
	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("aggregate expiry settlement broke conservation: %+v", violations)
	}
}

func TestSameExpiryUnavailableMemberDefersWholeCohort(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()

	available := expiryCohortOption("ABC-C-AVAILABLE", clock.now)
	unavailable := expiryCohortOption("ABC-C-UNAVAILABLE", clock.now)
	available.ObserveSettlement(100, clock.now)
	ex.AddInstrument(available)
	ex.AddInstrument(unavailable)

	ex.CheckExpiries()

	for _, option := range []*EuropeanOption{available, unavailable} {
		if ex.Instruments[option.Symbol()] == nil || ex.Books[option.Symbol()] == nil {
			t.Fatalf("cohort member %s was delisted despite unresolved sibling", option.Symbol())
		}
		pending, ok := ex.settlementPending[option.Symbol()]
		if !ok || pending.State != expiryStateSettlementPending || pending.Attempts != 1 {
			t.Fatalf("cohort member %s pending state = %#v", option.Symbol(), pending)
		}
	}
}

func TestMalformedExpiryArithmeticDefersWithoutMutation(t *testing.T) {
	cases := []struct {
		name        string
		panicCash   bool
		negativeFee bool
	}{
		{name: "cash-flow panic", panicCash: true},
		{name: "negative fee", negativeFee: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			clock := &expiryManualClock{now: 100}
			ex := NewExchange(2, clock)
			defer ex.Shutdown()

			future := &malformedExpiryInstrument{
				Instrument:  NewSpotInstrument("ABC-MALFORMED", "ABC", "USD", 1, 1, 1, 1),
				expiry:      clock.now,
				settlement:  100,
				panicCash:   test.panicCash,
				negativeFee: test.negativeFee,
			}
			ex.AddInstrument(future)
			ex.ConnectNewClient(1, nil, &FixedFee{})
			ex.AddPerpBalance(1, "USD", 1_000)
			ex.Positions.UpdatePosition(1, future.Symbol(), 1, 100, Buy, PositionBoth)

			ex.CheckExpiries()

			if _, pending := ex.settlementPending[future.Symbol()]; !pending {
				t.Fatal("malformed expiry was not deferred")
			}
			if ex.Books[future.Symbol()] == nil || ex.Instruments[future.Symbol()] == nil {
				t.Fatal("malformed expiry was delisted despite failed preflight")
			}
			if got := ex.Clients[1].PerpBalance("USD"); got != 1_000 {
				t.Fatalf("failed expiry changed client balance to %d", got)
			}
			if position := ex.Positions.GetPosition(1, future.Symbol()); position == nil || position.Size != 1 {
				t.Fatalf("failed expiry changed position to %#v", position)
			}
		})
	}
}
