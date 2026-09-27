package exchange

import (
	"math"
	"reflect"
	"slices"
	"testing"
)

type fundingCallbackOnlyStore struct{ PositionStore }

type fundingDuplicatingStore struct{ PositionStore }

func (store fundingDuplicatingStore) SnapshotFundingPositions(symbol string) ([]Position, error) {
	position := Position{ClientID: 1, Symbol: symbol, PositionSide: PositionBoth, Size: 1}
	return []Position{position, position}, nil
}

func TestCaptureFundingAccountSnapshotUsesRegisteredVenueState(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	clock.now = 15
	fill := ex.PlaceOrder(1, &OrderRequest{Symbol: "ABC-PERP", Side: Buy,
		Type: LimitOrder, Price: 105, Qty: 1, TimeInForce: GTC})
	if !fill.Success {
		t.Fatalf("perp fixture trade rejected: %v", fill.Error)
	}
	clock.now = 20
	requestedRoster := []uint64{4, 2, 1, 3}
	request := FundingAccountSnapshotRequest{
		VenueID: "N", PerpSymbol: "ABC-PERP", TimestampNano: 20,
		ExpectedClientIDs: requestedRoster,
	}
	snapshot, err := ex.CaptureFundingAccountSnapshot(request)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.VenueID != "N" || snapshot.PerpSymbol != "ABC-PERP" || snapshot.QuoteAsset != "USD" ||
		snapshot.BasePrecision != 1 || snapshot.TimestampNano != 20 || len(snapshot.Accounts) != 4 ||
		!slices.Equal(requestedRoster, []uint64{4, 2, 1, 3}) {
		t.Fatalf("source identity, roster or request mutated: %+v", snapshot)
	}
	for index, account := range snapshot.Accounts {
		if account.ClientID != uint64(index+1) || account.PerpCash != 100_000 {
			t.Fatalf("account source disagrees with venue cash: %+v", account)
		}
	}
	if snapshot.Accounts[0].NetPosition != 1 || snapshot.Accounts[3].NetPosition != -1 ||
		snapshot.Accounts[1].NetPosition != 0 || snapshot.Accounts[2].NetPosition != 0 ||
		len(snapshot.Accounts[0].Legs) != 1 || snapshot.Accounts[0].Legs[0] != (FundingPositionLeg{PositionBoth, 1}) ||
		len(snapshot.Accounts[3].Legs) != 1 || snapshot.Accounts[3].Legs[0] != (FundingPositionLeg{PositionBoth, -1}) {
		t.Fatalf("traded and flat positions were not sourced completely: %+v", snapshot.Accounts)
	}
	again, err := ex.CaptureFundingAccountSnapshot(request)
	if err != nil || !reflect.DeepEqual(snapshot, again) {
		t.Fatalf("same state produced different ordered snapshot: %+v, %v", again, err)
	}
}

func TestCaptureFundingAccountSnapshotNetsHedgeLegsAndKeepsFlatClients(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	positions := ex.Positions.(*PositionManager)
	positions.UpdatePosition(1, "ABC-PERP", 2, 100, Buy, PositionLong)
	positions.UpdatePosition(1, "ABC-PERP", 1, 100, Sell, PositionShort)
	positions.UpdatePosition(2, "ABC-PERP", 1, 100, Sell, PositionShort)
	clock.now = 20
	snapshot, err := ex.CaptureFundingAccountSnapshot(fundingAccountSourceRequest())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Accounts[0].NetPosition != 1 || snapshot.Accounts[1].NetPosition != -1 ||
		len(snapshot.Accounts[0].Legs) != 2 ||
		snapshot.Accounts[0].Legs[0] != (FundingPositionLeg{PositionLong, 2}) ||
		snapshot.Accounts[0].Legs[1] != (FundingPositionLeg{PositionShort, -1}) ||
		len(snapshot.Accounts[2].Legs) != 0 || len(snapshot.Accounts[3].Legs) != 0 {
		t.Fatalf("hedge netting or flat account retention failed: %+v", snapshot.Accounts)
	}
}

func TestCaptureFundingAccountSnapshotKeepsCompleteFlatRoster(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	ex.Positions.UpdatePosition(1, "ABC-PERP", 1, 100, Buy, PositionBoth)
	ex.Positions.UpdatePosition(1, "ABC-PERP", 1, 100, Sell, PositionBoth)
	clock.now = 20
	snapshot, err := ex.CaptureFundingAccountSnapshot(fundingAccountSourceRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Accounts) != 4 {
		t.Fatalf("flat roster omitted accounts: %+v", snapshot.Accounts)
	}
	for _, account := range snapshot.Accounts {
		if account.NetPosition != 0 || len(account.Legs) != 0 || account.PerpCash != 100_000 {
			t.Fatalf("flat account misreported: %+v", account)
		}
	}
}

func TestCaptureFundingAccountSnapshotRejectsIncompleteOrMalformedSources(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*DefaultExchange, *fundingSourceClock, *FundingAccountSnapshotRequest)
	}{
		{"wrong-venue", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest) {
			request.VenueID = "S"
		}},
		{"wrong-clock", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest) {
			request.TimestampNano = 19
		}},
		{"spot-instead-of-perp", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest) {
			request.PerpSymbol = "ABC-USD"
		}},
		{"omitted-client", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest) {
			request.ExpectedClientIDs = []uint64{1, 2, 3}
		}},
		{"duplicate-roster-id", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest) {
			request.ExpectedClientIDs = []uint64{1, 2, 2, 4}
		}},
		{"new-client-after-roster-freeze", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.ConnectNewClient(5, map[string]int64{"USD": 100}, &FixedFee{})
		}},
		{"misbound-book-symbol", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Books["ABC-PERP"].Symbol = "OTHER-PERP"
		}},
		{"misbound-book-instrument", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Books["ABC-PERP"].Instrument = NewPerpFutures("ABC-PERP", "CDF", "USD", 1, 1, 1, 1)
		}},
		{"wrong-client-identity", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Clients[1].ID = 99
		}},
		{"negative-perp-cash", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Clients[1].PerpBalances["USD"] = -1
		}},
		{"orphan-position", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Positions.UpdatePosition(99, "ABC-PERP", 1, 100, Buy, PositionBoth)
		}},
		{"orphan-closed-position", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Positions.UpdatePosition(99, "ABC-PERP", 1, 100, Buy, PositionBoth)
			ex.Positions.UpdatePosition(99, "ABC-PERP", 1, 100, Sell, PositionBoth)
		}},
		{"unmatched-position", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Positions.UpdatePosition(1, "ABC-PERP", 1, 100, Buy, PositionBoth)
		}},
		{"unknown-stored-side", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			positions := ex.Positions.(*PositionManager)
			positions.Lock()
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "ABC-PERP", PositionSide: PositionSide(255), Size: 1})
			positions.Unlock()
		}},
		{"zero-size-unknown-stored-side", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			positions := ex.Positions.(*PositionManager)
			positions.Lock()
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "ABC-PERP", PositionSide: PositionSide(255)})
			positions.Unlock()
		}},
		{"wrong-stored-client", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			positions := ex.Positions.(*PositionManager)
			positions.Lock()
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 2, Symbol: "ABC-PERP", PositionSide: PositionBoth, Size: 1})
			positions.Unlock()
		}},
		{"wrong-stored-symbol", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			positions := ex.Positions.(*PositionManager)
			positions.Lock()
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "OTHER-PERP", PositionSide: PositionBoth, Size: 1})
			positions.Unlock()
		}},
		{"long-with-negative-size", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			positions := ex.Positions.(*PositionManager)
			positions.Lock()
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "ABC-PERP", PositionSide: PositionLong, Size: -1})
			positions.Unlock()
		}},
		{"client-net-overflow", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			positions := ex.Positions.(*PositionManager)
			positions.Lock()
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "ABC-PERP", PositionSide: PositionBoth, Size: math.MaxInt64})
			positions.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "ABC-PERP", PositionSide: PositionLong, Size: math.MaxInt64})
			positions.Unlock()
		}},
		{"callback-only-position-store", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Positions = fundingCallbackOnlyStore{ex.Positions}
		}},
		{"duplicate-custom-snapshot", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest) {
			ex.Positions = fundingDuplicatingStore{ex.Positions}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ex, clock := fundingBookPairFixture(t, true)
			clock.now = 20
			request := fundingAccountSourceRequest()
			test.mutate(ex, clock, &request)
			snapshot, err := ex.CaptureFundingAccountSnapshot(request)
			if err == nil || !reflect.DeepEqual(snapshot, FundingAccountSnapshot{}) {
				t.Fatalf("malformed source returned a usable account snapshot: %+v, %v", snapshot, err)
			}
		})
	}
}

func fundingAccountSourceRequest() FundingAccountSnapshotRequest {
	return FundingAccountSnapshotRequest{
		VenueID: "N", PerpSymbol: "ABC-PERP", TimestampNano: 20,
		ExpectedClientIDs: []uint64{1, 2, 3, 4},
	}
}
