package repeatedspot

import (
	"bytes"
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/analysis"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	worldspot "exchange_sim/simulations/repeatedspot"
)

type testAggressor struct {
	*actor.BaseActor
	sent    bool
	enabled bool
	market  bool
}

func (aggressor *testAggressor) HandleEvent(context.Context, *actor.Event) {}

func (aggressor *testAggressor) onTick(time.Time) {
	if aggressor.sent || !aggressor.enabled {
		return
	}
	aggressor.sent = true
	if aggressor.market {
		aggressor.SubmitOrder("ABC/USD", exchange.Buy, exchange.Market, 0, 1)
	} else {
		aggressor.SubmitOrder("ABC/USD", exchange.Buy, exchange.LimitOrder, 101, 1)
	}
}

func fixtureWorld(t *testing.T) *worldspot.World {
	return fixtureWorldWithAggression(t, true)
}

func fixtureWorldWithAggression(t *testing.T, enabled bool) *worldspot.World {
	return fixtureWorldWithMaker(t, enabled, false)
}

func fixtureWorldWithMaker(t *testing.T, enabled, stoikov bool) *worldspot.World {
	return fixtureWorldWithBook(t, enabled, stoikov, 2, 2*time.Second, false)
}

func fixtureWorldWithBook(t *testing.T, enabled, stoikov bool, seedAskQty int64, quoteInterval time.Duration, marketOrder bool) *worldspot.World {
	t.Helper()
	seed, err := worldspot.NewSeedOncePolicy(worldspot.SeedOnceConfig{Symbol: "ABC/USD", BidPrice: 99, AskPrice: 101, BidQty: 2, AskQty: seedAskQty})
	if err != nil {
		t.Fatal(err)
	}
	makerConfig := worldspot.RecurringMakerConfig{Symbol: "ABC/USD", QuoteQty: 1, MinQuoteQty: 1,
		WorkingLimit: 5, TickSize: 1, QuoteInterval: quoteInterval,
		InitialLogVariancePerSecond: 1e-8, VolatilityHalfLife: 4 * time.Second,
		VolatilitySampleInterval: time.Second, MaxLogVarianceMultiple: 4}
	var maker worldspot.PolicyDefinition
	if stoikov {
		maker, err = worldspot.NewBoundedStoikovMakerPolicy(worldspot.BoundedStoikovMakerConfig{
			Maker: makerConfig, QuotePrecision: 1, RelativeRiskAversion: 50,
			RelativeFillDecay: 20_000, InventoryHorizon: 10 * time.Second,
			MinHalfSpreadTicks: 1,
		})
	} else {
		maker, err = worldspot.NewBoundedFixedMakerPolicy(worldspot.BoundedFixedMakerConfig{
			Maker: makerConfig, SpreadBps: 100,
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	params := struct {
		Enabled bool `json:"enabled"`
		Market  bool `json:"market"`
	}{enabled, marketOrder}
	aggressor, err := worldspot.DefinePolicy("fixture_aggressor", params, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, decoded struct {
		Enabled bool `json:"enabled"`
		Market  bool `json:"market"`
	}) (actor.Actor, error) {
		result := &testAggressor{BaseActor: actor.NewBaseActor(id, gateway), enabled: decoded.Enabled, market: decoded.Market}
		result.SetHandler(result)
		result.AddTicker(3*time.Second, result.onTick)
		return result, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fees, err := worldspot.NewPercentageFee(worldspot.PercentageFeeConfig{TakerBps: 100, InQuote: true})
	if err != nil {
		t.Fatal(err)
	}
	deployment := worldspot.Deployment{RequestLatency: time.Nanosecond, ResponseLatency: time.Nanosecond, MarketDataLatency: time.Nanosecond}
	world, err := worldspot.Build(worldspot.Config{
		VenueID: "fixture", Instrument: worldspot.InstrumentConfig{Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Step: time.Second, Iterations: 12, SnapshotInterval: time.Second, ForbidBorrowing: true,
		RecordSnapshotProjectionEvidence: true,
		Participants: []worldspot.Participant{
			{ActorID: 1, ClientID: 1, Role: "seed", Balances: map[string]int64{"ABC": 10, "USD": 1000}, Fees: fees, Latency: deployment, Policy: seed},
			{ActorID: 2, ClientID: 2, Role: "maker", Balances: map[string]int64{"ABC": 10, "USD": 1000}, Fees: fees, Latency: deployment, Policy: maker},
			{ActorID: 3, ClientID: 3, Role: "aggressor", Balances: map[string]int64{"ABC": 0, "USD": 1000}, Fees: fees, Latency: deployment, Policy: aggressor},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return world
}

func TestCaptureProductionPathAndInformationSidecars(t *testing.T) {
	world := fixtureWorld(t)
	directory := t.TempDir()
	receipts, err := simulation.NewMarketDataReceiptRecorder(directory)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	capture, err := NewCapture(world, &raw, receipts)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := capture.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	if err := WalkEvidence(bytes.NewReader(raw.Bytes()), identity, func(event Event) error {
		counts[event.Source+"/"+event.Name]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"control/world_begin", "control/world_run_start", "control/world_end", "exchange/balance_snapshot", "exchange/balance_change", "exchange/venue_balance_change", "exchange/fee_revenue", "exchange/Trade", "exchange/OrderFill", "actor/order_send", "actor/maker_decision"} {
		if counts[name] == 0 {
			t.Fatalf("production capture missing %s: %+v", name, counts)
		}
	}
	audit, err := analysis.AuditMarketDataReceipts(directory)
	if err != nil || !audit.Valid || audit.Decisions == 0 || audit.Receipts == 0 {
		t.Fatalf("invalid information-boundary audit: %+v, %v", audit, err)
	}
	replay, err := Replay(world.ContractJSON(), bytes.NewReader(raw.Bytes()), identity, directory)
	if err != nil {
		t.Fatal(err)
	}
	if replay.TradeCount != 1 || len(replay.Accounts) != 3 || replay.TerminalMarkStatus == "" || replay.VenueFeeRevenue["USD"] != 1 {
		t.Fatalf("incomplete independent economic replay: %+v", replay)
	}
	if replay.Accounts[0].Outbound.PlaceSent != 2 || replay.Accounts[0].Outbound.PlaceAccepted != 2 ||
		replay.Accounts[2].Outbound.PlaceSent != 1 || replay.Accounts[2].Outbound.PlaceAccepted != 1 {
		t.Fatalf("outbound-to-exchange placement funnel did not reconstruct: %+v", replay.Accounts)
	}
	market := replay.Market
	if market.HorizonNanos != 12*int64(time.Second) || market.PublicSnapshotMessages == 0 ||
		market.TradeVolumeBaseUnits != "1" || market.TradeNotionalQuoteUnits != "101" ||
		market.TwoSidedNanos+market.BidOnlyNanos+market.AskOnlyNanos+market.EmptyNanos != market.HorizonNanos {
		t.Fatalf("incomplete or nonpartitioned time-weighted market evidence: %+v", market)
	}
}

func TestValidNoTradeWorldIsNotEvidenceFailure(t *testing.T) {
	world := fixtureWorldWithAggression(t, false)
	directory := t.TempDir()
	receipts, err := simulation.NewMarketDataReceiptRecorder(directory)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	capture, err := NewCapture(world, &raw, receipts)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := capture.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := Replay(world.ContractJSON(), bytes.NewReader(raw.Bytes()), identity, directory)
	if err != nil {
		t.Fatal(err)
	}
	if replay.TradeCount != 0 {
		t.Fatalf("no-trade control unexpectedly traded: %+v", replay)
	}
	if replay.TerminalMidQuote != nil {
		for _, account := range replay.Accounts {
			if account.BenchmarkGain == nil || *account.BenchmarkGain != 0 {
				t.Fatalf("unchanged no-trade account has nonzero or unavailable benchmark gain: %+v", account)
			}
		}
	}
}

func TestIndependentReplayJoinsMarketOrderToActualFill(t *testing.T) {
	world := fixtureWorldWithBook(t, true, false, 2, 2*time.Second, true)
	directory := t.TempDir()
	receipts, err := simulation.NewMarketDataReceiptRecorder(directory)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	capture, err := NewCapture(world, &raw, receipts)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := capture.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := Replay(world.ContractJSON(), bytes.NewReader(raw.Bytes()), identity, directory)
	if err != nil {
		t.Fatal(err)
	}
	if replay.TradeCount != 1 || replay.Accounts[2].Outbound.PlaceSent != 1 ||
		replay.Accounts[2].Outbound.PlaceAccepted != 1 || replay.Accounts[2].FillCount != 1 {
		t.Fatalf("market-order request/fill path did not reconcile: %+v", replay)
	}
}
