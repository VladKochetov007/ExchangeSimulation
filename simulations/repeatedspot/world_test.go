package repeatedspot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulations/feesim"
	"exchange_sim/simulations/multivenue"
)

type fixtureAggressor struct {
	*actor.BaseActor
	symbol string
	side   exchange.Side
	price  int64
	qty    int64
	sent   bool
}

type fixtureCanceller struct {
	*actor.BaseActor
	orderID           uint64
	cancelSent        bool
	cancelledObserved bool
}

func (canceller *fixtureCanceller) Start(ctx context.Context) error {
	if err := canceller.BaseActor.Start(ctx); err != nil {
		return err
	}
	canceller.SubmitPostOnlyOrder("ABC/USD", exchange.Buy, 98, 1)
	return nil
}

func (canceller *fixtureCanceller) HandleEvent(_ context.Context, event *actor.Event) {
	if event.Type == actor.EventOrderAccepted {
		canceller.orderID = event.Data.(actor.OrderAcceptedEvent).OrderID
	}
	if event.Type == actor.EventOrderCancelled {
		canceller.cancelledObserved = true
	}
}

func (canceller *fixtureCanceller) cancel(time.Time) {
	if canceller.orderID != 0 && !canceller.cancelSent {
		canceller.CancelOrder(canceller.orderID)
		canceller.cancelSent = true
	}
}

type fixtureEvents struct{ events []*actor.Event }

func (recorder *fixtureEvents) HandleEvent(_ context.Context, event *actor.Event) {
	recorder.events = append(recorder.events, event)
}

type forwardingEvents struct {
	inner   actor.EventHandler
	fills   int
	cancels int
}

func (recorder *forwardingEvents) HandleEvent(ctx context.Context, event *actor.Event) {
	if event.Type == actor.EventOrderFilled || event.Type == actor.EventOrderPartialFill {
		recorder.fills++
	}
	if event.Type == actor.EventOrderCancelled {
		recorder.cancels++
	}
	recorder.inner.HandleEvent(ctx, event)
}

func (aggressor *fixtureAggressor) place(time.Time) {
	if aggressor.sent {
		return
	}
	aggressor.sent = true
	aggressor.SubmitOrder(aggressor.symbol, aggressor.side, exchange.LimitOrder, aggressor.price, aggressor.qty)
}

func (aggressor *fixtureAggressor) HandleEvent(context.Context, *actor.Event) {}

func fixtureConfig(t *testing.T) Config {
	return fixtureConfigWithSide(t, exchange.Buy)
}

func fixtureConfigWithSide(t *testing.T, side exchange.Side) Config {
	t.Helper()
	seed, err := NewSeedOncePolicy(SeedOnceConfig{Symbol: "ABC/USD", BidPrice: 99, AskPrice: 101, BidQty: 2, AskQty: 2})
	if err != nil {
		t.Fatal(err)
	}
	price, baseBalance := int64(101), int64(0)
	if side == exchange.Sell {
		price, baseBalance = 99, 10
	}
	configuredSide := "buy"
	if side == exchange.Sell {
		configuredSide = "sell"
	}
	aggressor, err := DefinePolicy("fixture_aggressor", struct {
		Symbol string `json:"symbol"`
		Side   string `json:"side"`
		Price  int64  `json:"price"`
		Qty    int64  `json:"qty"`
	}{"ABC/USD", configuredSide, price, 2}, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, params struct {
		Symbol string `json:"symbol"`
		Side   string `json:"side"`
		Price  int64  `json:"price"`
		Qty    int64  `json:"qty"`
	}) (actor.Actor, error) {
		orderSide := exchange.Buy
		if params.Side == "sell" {
			orderSide = exchange.Sell
		}
		result := &fixtureAggressor{BaseActor: actor.NewBaseActor(id, gateway), symbol: params.Symbol,
			side: orderSide, price: params.Price, qty: params.Qty}
		result.SetHandler(result)
		result.AddTicker(time.Second, result.place)
		return result, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fees, err := NewPercentageFee(PercentageFeeConfig{InQuote: true})
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		VenueID: "fixture", Instrument: InstrumentConfig{Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Step: time.Second, Iterations: 5, SnapshotInterval: time.Second, ForbidBorrowing: true,
		Participants: []Participant{
			{ActorID: 1, ClientID: 1, Role: "seed", Balances: map[string]int64{"ABC": 10, "USD": 1000}, Policy: seed, Fees: fees},
			{ActorID: 2, ClientID: 2, Role: "aggressor", Balances: map[string]int64{"ABC": baseBalance, "USD": 1000}, Policy: aggressor, Fees: fees},
		},
	}
}

func TestSeedOnceFillDoesNotReplenish(t *testing.T) {
	world, err := Build(fixtureConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	seedEvents, buyerEvents := &fixtureEvents{}, &fixtureEvents{}
	world.actors[0].(*SeedOnce).SetHandler(seedEvents)
	world.actors[1].(*fixtureAggressor).SetHandler(buyerEvents)
	if err := world.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(world.exchange.Books) != 1 || len(world.exchange.Instruments) != 1 {
		t.Fatalf("expected one spot instrument, got %d books and %d instruments", len(world.exchange.Books), len(world.exchange.Instruments))
	}
	bidQty, askQty := world.exchange.GetBestLiquidity("ABC/USD")
	if bidQty != 2 || askQty != 0 {
		t.Fatalf("seed should retain only the untouched bid; bid=%d ask=%d", bidQty, askQty)
	}
	if got := world.exchange.Clients[1].GetBalance("ABC"); got != 8 {
		t.Fatalf("seed base balance = %d, want 8 after selling two units", got)
	}
	if got := world.exchange.Clients[1].GetBalance("USD"); got != 1202 {
		t.Fatalf("seed quote balance = %d, want 1202", got)
	}
	if got := world.exchange.Clients[2].GetBalance("ABC"); got != 2 {
		t.Fatalf("buyer base balance = %d, want 2", got)
	}
	if len(seedEvents.events) < 3 || len(buyerEvents.events) < 2 {
		t.Fatalf("missing accepted/fill responses: seed=%d buyer=%d", len(seedEvents.events), len(buyerEvents.events))
	}
	if err := world.Run(context.Background()); err == nil {
		t.Fatal("world allowed a second run")
	}
}

func TestSeedOnceBidFillDoesNotReplenish(t *testing.T) {
	world, err := Build(fixtureConfigWithSide(t, exchange.Sell))
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	bidQty, askQty := world.exchange.GetBestLiquidity("ABC/USD")
	if bidQty != 0 || askQty != 2 {
		t.Fatalf("seed should retain only the untouched ask; bid=%d ask=%d", bidQty, askQty)
	}
	seedAccount := world.exchange.Clients[1]
	if got := seedAccount.GetBalance("ABC"); got != 12 {
		t.Fatalf("seed base balance = %d, want 12 after buying two units", got)
	}
	if got := seedAccount.GetBalance("USD"); got != 802 {
		t.Fatalf("seed quote balance = %d, want 802", got)
	}
	if got := world.exchange.Clients[2].GetBalance("ABC"); got != 8 {
		t.Fatalf("seller base balance = %d, want 8", got)
	}
}

func TestOrdinaryCancelReleasesFiniteReservation(t *testing.T) {
	config := fixtureConfig(t)
	policy, err := DefinePolicy("fixture_canceller", struct{}{}, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, _ struct{}) (actor.Actor, error) {
		canceller := &fixtureCanceller{BaseActor: actor.NewBaseActor(id, gateway)}
		canceller.SetHandler(canceller)
		canceller.AddTicker(time.Second, canceller.cancel)
		return canceller, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	config.Participants[1].Policy = policy
	world, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	canceller := world.actors[1].(*fixtureCanceller)
	if !canceller.cancelledObserved {
		t.Fatal("ordinary cancellation was not observed")
	}
	if reserved := world.exchange.Clients[2].GetReserved("USD"); reserved != 0 {
		t.Fatalf("cancelled order left %d USD reserved", reserved)
	}
	bidQty, askQty := world.exchange.GetBestLiquidity("ABC/USD")
	if bidQty != 2 || askQty != 2 {
		t.Fatalf("seed depth changed after other actor's cancel: bid=%d ask=%d", bidQty, askQty)
	}
}

func TestCancelledRunCannotBeReportedComplete(t *testing.T) {
	world, err := Build(fixtureConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := world.Run(ctx); err == nil || !strings.Contains(err.Error(), "incomplete world") {
		t.Fatalf("cancelled run reported completion: %v", err)
	}
}

func TestContractCapturesEffectiveParameters(t *testing.T) {
	config := fixtureConfig(t)
	world, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	before := world.ContractJSON()
	config.Participants[0].Balances["USD"] = 0
	config.Participants[0].Policy.parameters[0] = 'x'
	world.ContractJSON()[0] = 'x'
	if got := world.ContractJSON(); string(got) != string(before) {
		t.Fatal("contract changed after caller mutation")
	}
	digest := sha256.Sum256(before)
	if world.ContractSHA256() != hex.EncodeToString(digest[:]) {
		t.Fatal("contract digest does not bind exported bytes")
	}
	var contract worldContract
	if err := json.Unmarshal(before, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Participants[0].Balances["USD"] != 1000 || contract.Participants[0].Policy.Name != "seed_once_v1" ||
		!contract.ForbidBorrowing || contract.MatchingRule != "price_time_fifo" || !contract.DeterministicPhases ||
		contract.PhaseMaxRounds != phaseMaxRounds || contract.AutomationEnabled {
		t.Fatalf("wrong effective contract: %+v", contract)
	}
	actors := world.Actors()
	actors[0] = nil
	if world.Actors()[0] == nil || world.Venue() == nil {
		t.Fatal("world accessors failed to preserve owned handles")
	}
}

func TestBuildRejectsInvalidResourcesAndIdentities(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{"duplicate client", func(c *Config) { c.Participants[1].ClientID = 1 }, "duplicate client"},
		{"duplicate actor", func(c *Config) { c.Participants[1].ActorID = 1 }, "duplicate actor"},
		{"negative balance", func(c *Config) { c.Participants[0].Balances["ABC"] = -1 }, "balance"},
		{"undeclared asset", func(c *Config) { c.Participants[0].Balances["EUR"] = 2 }, "balance"},
		{"negative delay", func(c *Config) { c.Participants[0].Latency.RequestLatency = -1 }, "participant"},
		{"zero step", func(c *Config) { c.Step = 0 }, "world"},
		{"negative start", func(c *Config) { c.StartUnixNano = -1 }, "world"},
		{"missing policy", func(c *Config) { c.Participants[0].Policy = PolicyDefinition{} }, "participant"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := fixtureConfig(t)
			test.change(&config)
			_, err := Build(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestPolicyDefinitionRejectsUnserializableSide(t *testing.T) {
	_, err := DefinePolicy("invalid_side", struct {
		Side exchange.Side `json:"side"`
	}{Side: exchange.Buy}, func(uint64, actor.Gateway, exchange.TickerFactory, struct {
		Side exchange.Side `json:"side"`
	}) (actor.Actor, error) {
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "do not round-trip") {
		t.Fatalf("expected a parameter round-trip error, got %v", err)
	}
}

func TestDraftE0CompositionAndSimulatedRecurrence(t *testing.T) {
	pure, err := NewFixedMakerPolicy(FixedMakerConfig{
		Symbol: e0Symbol, SpreadBps: 2, RequoteBps: 1,
		QuoteQty: e0BasePrecision / 10, MaxInventory: 10 * e0BasePrecision,
		QuoteInterval: 5 * time.Second, TickSize: e0TickSize, PostOnlyCancelBeforeReplace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stoikov, err := NewStoikovSpotPolicy(StoikovSpotConfig{
		Symbol: e0Symbol, VenueID: "E0-ABC-SPOT", BootstrapPrice: 50_000 * e0QuotePrecision,
		BasePrecision: e0BasePrecision, QuotePrecision: e0QuotePrecision, TickSize: e0TickSize,
		QuoteQty: e0BasePrecision / 10, QuoteInterval: 5 * time.Second,
		InventoryLimit: 10 * e0BasePrecision, InventoryHorizon: 600 * time.Second,
		RelativeRiskAversion: 50, RelativeFillDecay: 20_000, InitialLogVariancePerSec: 1e-8,
		VolatilityHalfLife: 120 * time.Second, VolatilitySampleInterval: 30 * time.Second,
		MaxLogVarianceMultiple: 4, MinHalfSpreadTicks: 1, RequoteBps: 1, CancelBeforeReplace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := DraftE0Config([4]PolicyDefinition{pure, pure, stoikov, stoikov}, [4]int64{1, 2, 3, 4}, [2]int64{5, 6})
	if err != nil {
		t.Fatal(err)
	}
	config.Iterations = 40 // synthetic fixture, not the 55-minute research horizon
	world, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(world.actors) != 11 || len(world.exchange.Books) != 1 || len(world.exchange.Instruments) != 1 {
		t.Fatalf("unexpected draft roster/books: %d actors, %d books", len(world.actors), len(world.exchange.Books))
	}
	for _, participant := range config.Participants[1:5] {
		if participant.Balances["ABC"] != 50*e0BasePrecision || participant.Balances["USD"] != 2_500_000*e0QuotePrecision {
			t.Fatalf("unequal maker endowment in slot %d", participant.ActorID)
		}
	}
	var takerOrders int
	var makerEvents []*forwardingEvents
	for _, participant := range world.actors {
		if taker, ok := participant.(*feesim.RandomTaker); ok {
			taker.SetOrderDecisionObserver(func(exchange.Request) { takerOrders++ })
		}
		switch maker := participant.(type) {
		case *multivenue.FixedDistanceMaker:
			recorder := &forwardingEvents{inner: maker}
			maker.SetHandler(recorder)
			makerEvents = append(makerEvents, recorder)
		case *multivenue.StoikovMarketMaker:
			recorder := &forwardingEvents{inner: maker}
			maker.SetHandler(recorder)
			makerEvents = append(makerEvents, recorder)
		}
	}
	if err := world.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if takerOrders < 2 {
		t.Fatalf("recurring takers emitted only %d orders over 40 simulated seconds", takerOrders)
	}
	var fills, cancels int
	for _, events := range makerEvents {
		fills += events.fills
		cancels += events.cancels
	}
	t.Logf("synthetic recurring fixture: taker-orders=%d maker-fills=%d maker-cancels=%d", takerOrders, fills, cancels)
	if fills == 0 {
		t.Fatal("no recurring maker received a fill")
	}
	if world.exchange.Clients[1].GetBalance("ABC") < 0 || world.exchange.Clients[1].GetBalance("USD") < 0 {
		t.Fatal("seed account overdrew")
	}
	if _, ok := world.actors[2].(*multivenue.FixedDistanceMaker); !ok {
		t.Fatal("expected the pure-maker adapter in slot 2")
	}
	if _, ok := world.actors[3].(*multivenue.StoikovMarketMaker); !ok {
		t.Fatal("expected the Stoikov adapter in slot 3")
	}
}
