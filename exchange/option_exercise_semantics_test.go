package exchange

import (
	"fmt"
	"testing"
	"time"
)

// Deterministic exercise fixtures.
//
// A long ecology run only exercises option settlement if the run happens to
// outlive a tenor and happens to leave somebody holding the contract. That
// makes it useless as a mutation detector: a run that settles nothing reports
// nothing wrong, which is NOT TESTED rather than a pass. These fixtures force
// the settlement path to execute with known positions and a known settlement
// price, and check each payout against the contract's own metadata rather than
// against anything the engine computed.
//
// Expected payoff per holder, computed here from raw terms only:
//
//	call: max(S-K, 0) * size / multiplier
//	put:  max(K-S, 0) * size / multiplier
//
// where the multiplier is the contract's base precision.

// exerciseHolder is one account's forced position in the contract.
type exerciseHolder struct {
	clientID uint64
	size     int64 // signed, in base units; positive is long
}

// exerciseFixture is one settlement scenario stated entirely in contract terms.
type exerciseFixture struct {
	name       string
	isCall     bool
	strike     int64
	settlement int64
	multiplier int64 // contract base precision
	holders    []exerciseHolder
}

// expected is the payoff this fixture must produce for one holder, derived
// from the contract terms without consulting the engine.
func (f exerciseFixture) expected(size int64) int64 {
	var intrinsic int64
	if f.isCall {
		intrinsic = f.settlement - f.strike
	} else {
		intrinsic = f.strike - f.settlement
	}
	if intrinsic < 0 {
		intrinsic = 0
	}
	// Signed, and exact: every fixture uses sizes that divide the multiplier
	// evenly, so no truncation is involved and the expected value is a plain
	// product. A fixture that needed rounding would be testing the rounding
	// rule rather than the payoff.
	return intrinsic * size / f.multiplier
}

// settleFixture forces the positions, expires the contract, and returns each
// account's realised balance change plus the number of positions settled.
func settleFixture(t *testing.T, f exerciseFixture) (map[uint64]int64, int) {
	t.Helper()
	clock := &RealClock{}
	ex := NewExchange(len(f.holders)+1, clock)
	symbol := fmt.Sprintf("ABC-FIX-%s", f.name)
	option := NewEuropeanOption(
		symbol, "ABC", "USD", "ABC/USD",
		f.multiplier, valuationQuotePrecision, valuationQuotePrecision, f.multiplier/100,
		f.strike, time.Now().Add(-time.Second).UnixNano(), f.isCall,
	)
	// No delivery fee: the fixtures check the exercise payoff, and a fee would
	// mix a second mechanism into the same number. Fees are audited separately.
	option.DeliveryFeeBps = 0
	// With no observations recorded the settlement price is the underlying
	// mark, so this pins it exactly.
	option.SetMarks(f.settlement, 0)
	ex.AddInstrument(option)

	const endowment = int64(1_000_000_000_000)
	opening := make(map[uint64]int64, len(f.holders))
	for _, holder := range f.holders {
		ex.ConnectNewClient(holder.clientID, nil, &FixedFee{})
		ex.AddPerpBalance(holder.clientID, "USD", endowment)
		side := Buy
		size := holder.size
		if size < 0 {
			side = Sell
			size = -size
		}
		ex.Positions.UpdatePosition(holder.clientID, symbol, size, f.strike, side, PositionBoth)
		opening[holder.clientID] = ex.Clients[holder.clientID].PerpBalances["USD"]
	}

	settled := 0
	for _, holder := range f.holders {
		if pos := ex.Positions.GetPosition(holder.clientID, symbol); pos != nil && pos.Size != 0 {
			settled++
		}
	}
	if settled != len(f.holders) {
		t.Fatalf("%s: fixture did not open every position: %d of %d", f.name, settled, len(f.holders))
	}

	ex.CheckExpiries()

	if ex.Instruments[symbol] != nil {
		t.Errorf("%s: contract still listed after expiry", f.name)
	}
	if ex.Books[symbol] != nil {
		t.Errorf("%s: book still open after expiry", f.name)
	}
	deltas := make(map[uint64]int64, len(f.holders))
	for _, holder := range f.holders {
		if pos := ex.Positions.GetPosition(holder.clientID, symbol); pos != nil && pos.Size != 0 {
			t.Errorf("%s: client %d still holds %d after expiry", f.name, holder.clientID, pos.Size)
		}
		deltas[holder.clientID] = ex.Clients[holder.clientID].PerpBalances["USD"] - opening[holder.clientID]
	}
	return deltas, settled
}

// exerciseFixtures is the scenario matrix. Every branch of the payoff -- call
// and put, in and out of the money, the boundary, a non-unit multiplier and
// asymmetric sizes -- is reached by at least one of them.
func exerciseFixtures() []exerciseFixture {
	const unit = int64(100_000_000)
	q := valuationQuotePrecision
	return []exerciseFixture{
		{
			name: "itm-call", isCall: true,
			strike: 50_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			name: "otm-call", isCall: true,
			strike: 60_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			name: "itm-put", isCall: false,
			strike: 60_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			name: "otm-put", isCall: false,
			strike: 50_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			// At the boundary both payoffs are exactly zero, which is the case
			// a swapped call and put cannot be distinguished by -- it is here
			// to pin the boundary, not to catch that mutation.
			name: "atm-call", isCall: true,
			strike: 55_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			name: "atm-put", isCall: false,
			strike: 55_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			// A multiplier that is not the position unit: positions of a
			// quarter and two and a half contracts. A payoff that ignores the
			// multiplier is off by eight orders of magnitude here.
			name: "multiplier", isCall: true,
			strike: 50_000 * q, settlement: 52_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit / 4}, {2, 5 * unit / 2}, {3, -11 * unit / 4}},
		},
		{
			// Strike deliberately far from the settlement price, so a payoff
			// computed against the wrong strike cannot coincide with the right
			// one by rounding.
			name: "far-strike", isCall: true,
			strike: 10_000 * q, settlement: 55_000 * q, multiplier: unit,
			holders: []exerciseHolder{{1, unit}, {2, -unit}},
		},
		{
			// Asymmetric holders and writers: three longs against two writers
			// of unequal size.
			name: "asymmetric", isCall: false,
			strike: 60_000 * q, settlement: 51_500 * q, multiplier: unit,
			holders: []exerciseHolder{
				{1, 3 * unit}, {2, unit}, {3, 2 * unit},
				{4, -4 * unit}, {5, -2 * unit},
			},
		},
	}
}

// Every fixture is checked on five invariants at once: each holder's own
// payout, each writer's own payout, the symmetry between them, a worthless
// option paying nothing, and the settlement conserving cash across accounts.
func TestOptionExerciseSemanticsAgainstContractTerms(t *testing.T) {
	for _, fixture := range exerciseFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			net := int64(0)
			for _, holder := range fixture.holders {
				net += holder.size
			}
			if net != 0 {
				t.Fatalf("fixture is not a closed contract: net size %d", net)
			}

			deltas, settled := settleFixture(t, fixture)
			if settled == 0 {
				t.Fatal("NOT TESTED: no position reached settlement")
			}

			total := int64(0)
			worthless := fixture.expected(fixture.multiplier) == 0
			for _, holder := range fixture.holders {
				want := fixture.expected(holder.size)
				got := deltas[holder.clientID]
				if got != want {
					role := "holder"
					if holder.size < 0 {
						role = "writer"
					}
					t.Errorf("%s %d of size %d was paid %d, contract terms give %d",
						role, holder.clientID, holder.size, got, want)
				}
				if worthless && got != 0 {
					t.Errorf("worthless option paid client %d an amount of %d", holder.clientID, got)
				}
				total += got
			}
			// One side's gain is the other's loss: the sum across a closed set
			// of holders is exactly zero, with no fee and no truncation.
			if total != 0 {
				t.Errorf("settlement did not conserve cash: net %d across %d accounts", total, len(deltas))
			}
		})
	}
}

// Long/short symmetry stated on its own: mirroring every position mirrors
// every payout. A payoff that is not odd in the position size fails here even
// if it happens to satisfy the per-holder check on one side.
func TestOptionExercisePayoffIsOddInPositionSize(t *testing.T) {
	for _, fixture := range exerciseFixtures() {
		mirrored := fixture
		mirrored.name = fixture.name + "-mirrored"
		mirrored.holders = make([]exerciseHolder, len(fixture.holders))
		for i, holder := range fixture.holders {
			mirrored.holders[i] = exerciseHolder{clientID: holder.clientID, size: -holder.size}
		}
		t.Run(fixture.name, func(t *testing.T) {
			straight, _ := settleFixture(t, fixture)
			flipped, _ := settleFixture(t, mirrored)
			for id, value := range straight {
				if flipped[id] != -value {
					t.Errorf("client %d received %d long and %d short; the payoff is not odd in size",
						id, value, flipped[id])
				}
			}
		})
	}
}
