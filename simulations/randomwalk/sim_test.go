package randomwalk

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

func digestRandomwalkLogs(t *testing.T, dir string) string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(%q): %v", dir, err)
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatalf("Rel(%q): %v", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		fmt.Fprintf(hash, "%s\x00", rel)
		hash.Write(data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func digestRandomwalkState(sim *Sim) string {
	ex := sim.Exchange()
	ex.RLock()
	defer ex.RUnlock()

	hash := sha256.New()
	fmt.Fprintf(hash, "next=%d\n", ex.NextOrderID)
	symbols := make([]string, 0, len(ex.Books))
	for symbol := range ex.Books {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	for _, symbol := range symbols {
		book := ex.Books[symbol]
		fmt.Fprintf(hash, "book %s bids=%v asks=%v\n", symbol, book.Bids.GetSnapshot(), book.Asks.GetSnapshot())
	}

	clientIDs := make([]uint64, 0, len(ex.Clients))
	for clientID := range ex.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	sort.Slice(clientIDs, func(i, j int) bool { return clientIDs[i] < clientIDs[j] })
	writeBalances := func(label string, balances map[string]int64) {
		assets := make([]string, 0, len(balances))
		for asset := range balances {
			assets = append(assets, asset)
		}
		sort.Strings(assets)
		for _, asset := range assets {
			fmt.Fprintf(hash, "%s %s=%d\n", label, asset, balances[asset])
		}
	}
	for _, clientID := range clientIDs {
		client := ex.Clients[clientID]
		fmt.Fprintf(hash, "client %d\n", clientID)
		writeBalances("spot", client.Balances)
		writeBalances("spot_reserved", client.Reserved)
		writeBalances("perp", client.PerpBalances)
		writeBalances("perp_reserved", client.PerpReserved)
		writeBalances("borrowed", client.Borrowed)
		writeBalances("borrowed_spot", client.BorrowedSpot)
		positions := ex.Positions.GetAllPositions(clientID)
		sort.Slice(positions, func(i, j int) bool {
			if positions[i].Symbol != positions[j].Symbol {
				return positions[i].Symbol < positions[j].Symbol
			}
			return positions[i].PositionSide < positions[j].PositionSide
		})
		for _, position := range positions {
			fmt.Fprintf(hash, "position %+v\n", position)
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

type randomwalkStubGateway struct {
	requests []etypes.Request
	respCh   chan etypes.Response
	mdCh     chan *etypes.MarketDataMsg
}

func newRandomwalkStubGateway() *randomwalkStubGateway {
	return &randomwalkStubGateway{
		respCh: make(chan etypes.Response, 16),
		mdCh:   make(chan *etypes.MarketDataMsg, 16),
	}
}

func (g *randomwalkStubGateway) ID() uint64                                 { return 1 }
func (g *randomwalkStubGateway) Send(req etypes.Request)                    { g.requests = append(g.requests, req) }
func (g *randomwalkStubGateway) Responses() <-chan etypes.Response          { return g.respCh }
func (g *randomwalkStubGateway) MarketDataCh() <-chan *etypes.MarketDataMsg { return g.mdCh }
func (g *randomwalkStubGateway) IsRunning() bool                            { return true }

func (g *randomwalkStubGateway) placedOrders() []*etypes.OrderRequest {
	var orders []*etypes.OrderRequest
	for _, request := range g.requests {
		if request.Type == etypes.ReqPlaceOrder {
			orders = append(orders, request.OrderReq)
		}
	}
	return orders
}

func orderForSide(t *testing.T, orders []*etypes.OrderRequest, side exchange.Side) *etypes.OrderRequest {
	t.Helper()
	for _, order := range orders {
		if order.Side == side {
			return order
		}
	}
	t.Fatalf("missing %s order", side)
	return nil
}

func TestMarketMakerDepthIsNotionalNormalized(t *testing.T) {
	for _, asset := range assets {
		qty := quantityForUSDNotional(marketMakerLevelNotional, asset.price)
		notional, ok := etypes.TryMulDiv(qty, asset.price, btcPrecision)
		if !ok {
			t.Fatalf("%s: level notional overflow", asset.name)
		}
		tolerance := asset.price / btcPrecision
		if tolerance < 1 {
			tolerance = 1
		}
		if diff := notional - marketMakerLevelNotional; diff < -tolerance || diff > tolerance {
			t.Fatalf("%s: level notional = %d, want approximately %d", asset.name, notional, marketMakerLevelNotional)
		}
	}
}

func TestRandomWalkMaintainsTwoSidedQuotes(t *testing.T) {
	sim, err := NewSimWithConfig(10*time.Second, SimConfig{LogDir: t.TempDir(), SnapshotOnly: true})
	if err != nil {
		t.Fatalf("NewSimWithConfig: %v", err)
	}
	defer sim.Close()

	ctx := context.Background()
	sim.Exchange().StartAutomation(ctx)
	defer sim.Exchange().StopAutomation()
	var emptyAt []time.Duration
	sim.Runner.SetProgressCallback(1_000, func(done, _ int) {
		for _, mm := range sim.MMs {
			for _, symbol := range mm.Symbols() {
				bidQty, askQty := sim.Exchange().GetBestLiquidity(symbol)
				if bidQty == 0 || askQty == 0 {
					emptyAt = append(emptyAt, time.Duration(done)*time.Millisecond)
					return
				}
			}
		}
	})
	if err := sim.Runner.Run(ctx); err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if len(emptyAt) > 0 {
		t.Fatalf("one or more quiescent snapshots had an empty book: %v", emptyAt)
	}

	for _, mm := range sim.MMs {
		for _, symbol := range mm.Symbols() {
			bidQty, askQty := sim.Exchange().GetBestLiquidity(symbol)
			if bidQty == 0 || askQty == 0 {
				var pending, requests int
				for ref, orderIDs := range mm.pending {
					if ref.symbol == symbol {
						pending += len(orderIDs)
					}
				}
				for _, ref := range mm.reqToQuote {
					if ref.symbol == symbol {
						requests++
					}
				}
				t.Errorf("%s has empty final book: bid=%d ask=%d pending=%d requests=%d", symbol, bidQty, askQty, pending, requests)
			}
		}
	}
}

// The phase runtime is a stronger contract than ordinary quiescence: the full
// simulated ecology must produce identical logs and terminal state regardless
// of how many OS threads Go is allowed to schedule. This catches a goroutine
// that accidentally re-enters the direct-mount model path.
func TestRandomWalkDeterministicPhaseDigestAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) (stateDigest, logDigest string) {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)

		logDir := t.TempDir()
		sim, err := NewSimWithConfig(10*time.Second, SimConfig{LogDir: logDir, SnapshotOnly: true})
		if err != nil {
			t.Fatalf("NewSimWithConfig: %v", err)
		}
		ctx := context.Background()
		sim.Exchange().StartAutomation(ctx)
		defer sim.Exchange().StopAutomation()
		if err := sim.Runner.Run(ctx); err != nil {
			t.Fatalf("Runner.Run with GOMAXPROCS=%d: %v", procs, err)
		}
		stateDigest = digestRandomwalkState(sim)
		sim.Close()
		return stateDigest, digestRandomwalkLogs(t, logDir)
	}

	stateOne, logsOne := run(1)
	stateMany, logsMany := run(14)
	if stateOne != stateMany {
		t.Fatalf("terminal state digest differs: GOMAXPROCS=1 %s, GOMAXPROCS=14 %s", stateOne, stateMany)
	}
	if logsOne != logsMany {
		t.Fatalf("log digest differs: GOMAXPROCS=1 %s, GOMAXPROCS=14 %s", logsOne, logsMany)
	}
}

func TestTriArbRunsOncePerCoherentSnapshotTimestamp(t *testing.T) {
	gw := exchange.NewClientGateway(12)
	arb := NewTriArbActor(12, gw, TriArbConfig{
		CrossSymbol:    "DEF-ABC",
		BaseUSDSymbol:  "DEF-USD",
		QuoteUSDSymbol: "ABC-USD",
		TargetNotional: 1_000,
		MinProfitBps:   1,
		BasePrecision:  1,
		CheckInterval:  100 * time.Millisecond,
	})

	// DEF is materially cheaper through ABC, so each coherent snapshot would
	// otherwise trigger an execution.
	updateBooks := func(timestamp int64, symbols ...string) {
		t.Helper()
		for _, symbol := range symbols {
			bid, ask := int64(0), int64(0)
			switch symbol {
			case "DEF-ABC":
				bid, ask = 9, 10
			case "DEF-USD":
				bid, ask = 250, 251
			case "ABC-USD":
				bid, ask = 20, 21
			default:
				t.Fatalf("unknown symbol %q", symbol)
			}
			arb.onSnapshot(actor.BookSnapshotEvent{
				Symbol:    symbol,
				Timestamp: timestamp,
				Snapshot: &exchange.BookSnapshot{
					Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: 1_000}},
					Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: 1_000}},
				},
			})
		}
	}

	nextRequest := func() exchange.Request {
		t.Helper()
		select {
		case req := <-gw.RequestCh:
			return req
		default:
			t.Fatal("expected triangular-arbitrage order request")
			return exchange.Request{}
		}
	}
	completeCycle := func(first exchange.Request) {
		t.Helper()
		requests := [3]exchange.Request{first}
		for leg := range requests {
			orderID := uint64(100 + leg)
			arb.onAccepted(actor.OrderAcceptedEvent{
				RequestID: requests[leg].OrderReq.RequestID,
				OrderID:   orderID,
			})
			arb.onFilled(actor.OrderFillEvent{OrderID: orderID, IsFull: true})
			if leg < len(requests)-1 {
				requests[leg+1] = nextRequest()
			}
		}
		if arb.executing {
			t.Fatal("triangular-arbitrage cycle did not return to idle")
		}
	}

	allSymbols := []string{"DEF-ABC", "DEF-USD", "ABC-USD"}
	updateBooks(1_000, allSymbols...)
	arb.onTick(time.Unix(0, 0))
	completeCycle(nextRequest())

	// The actor is idle again, but unchanged cached books are stale: no repeat
	// execution may be launched by later 100 ms checks.
	for i := 0; i < 5; i++ {
		arb.onTick(time.Unix(0, int64(i+1)*int64(100*time.Millisecond)))
	}
	select {
	case req := <-gw.RequestCh:
		t.Fatalf("stale snapshot submitted duplicate cycle: %+v", req)
	default:
	}

	// Two refreshed legs are insufficient; a new execution can start only when
	// all three observations carry the same new timestamp.
	updateBooks(2_000, "DEF-ABC", "DEF-USD")
	arb.onTick(time.Unix(0, 600*int64(time.Millisecond)))
	select {
	case req := <-gw.RequestCh:
		t.Fatalf("incoherent snapshots submitted cycle: %+v", req)
	default:
	}
	updateBooks(2_000, "ABC-USD")
	arb.onTick(time.Unix(0, 700*int64(time.Millisecond)))
	completeCycle(nextRequest())
}

func TestMarketMakerRequotesMissingSideWithoutReplacingLiveQuote(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewMarketMaker(1, gw, MMConfig{
		Symbols: []string{"ABC-USD"}, BootstrapPrice: 100,
		Levels: 1, LevelSpacing: 1, LevelSize: 1, TickSize: 1,
		RefreshInterval: time.Hour,
	})
	const symbol = "ABC-USD"
	mm.quote(symbol)
	orders := gw.placedOrders()
	if len(orders) != 2 {
		t.Fatalf("initial orders = %d, want 2", len(orders))
	}
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectSelfTrade})
	mm.ensureQuoted(symbol)

	orders = gw.placedOrders()
	if len(orders) != 3 {
		t.Fatalf("one missing ask must produce one replacement, got %d total orders", len(orders))
	}
	if got := orders[2].Side; got != exchange.Sell {
		t.Fatalf("replacement side = %s, want sell", got)
	}
	mm.ensureQuoted(symbol)
	if got := len(gw.placedOrders()); got != 3 {
		t.Fatalf("side-aware in-flight tracking stacked orders: got %d, want 3", got)
	}
}

func TestMarketMakerKeepsInsufficientSideWithdrawnUntilOppositeFill(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewMarketMaker(1, gw, MMConfig{
		Symbols: []string{"ABC-USD"}, BootstrapPrice: 100,
		Levels: 1, LevelSpacing: 1, LevelSize: 1, TickSize: 1,
		RefreshInterval: time.Hour,
	})
	const symbol = "ABC-USD"
	mm.quote(symbol)
	orders := gw.placedOrders()
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectInsufficientBalance})
	mm.ensureQuoted(symbol)
	mm.ensureQuoted(symbol)
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("insufficient sell inventory must withdraw ask without retry spam, got %d orders", got)
	}

	mm.onFilled(actor.OrderFillEvent{OrderID: 11, Price: 100, IsFull: true})
	orders = gw.placedOrders()
	if len(orders) != 4 {
		t.Fatalf("buy fill must re-enable the withdrawn ask, got %d total orders", len(orders))
	}
	if got := orders[3].Side; got != exchange.Sell {
		t.Fatalf("re-enabled side = %s, want sell", got)
	}
}

func TestMarketMakerKeepsPartiallyFundedSideQuotedAfterReprice(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewMarketMaker(1, gw, MMConfig{
		Symbols: []string{"ABC-USD"}, BootstrapPrice: 100,
		Levels: 5, LevelSpacing: 1, LevelSize: 1, TickSize: 1,
		RefreshInterval: time.Hour,
	})
	const symbol = "ABC-USD"
	mm.quote(symbol)
	orders := gw.placedOrders()
	if len(orders) != 10 {
		t.Fatalf("initial orders = %d, want 10", len(orders))
	}

	// Four asks reserve successfully, then the fifth is rejected because the
	// account has enough base inventory for only four quote levels.
	for i, order := range orders {
		if order.Side == exchange.Buy || i < 9 {
			mm.onAccepted(actor.OrderAcceptedEvent{OrderID: uint64(100 + i), RequestID: order.RequestID})
		}
	}
	lastAsk := orders[len(orders)-1]
	if lastAsk.Side != exchange.Sell {
		t.Fatalf("last order side = %s, want sell", lastAsk.Side)
	}
	mm.onRejected(actor.OrderRejectedEvent{RequestID: lastAsk.RequestID, Reason: exchange.RejectInsufficientBalance})

	ref := quoteRef{symbol: symbol, side: exchange.Sell}
	if mm.withdrawn[ref] {
		t.Fatal("partial sell admission incorrectly withdrew the funded sell levels")
	}
	mm.cancelAllForSym(symbol)
	mm.quote(symbol)

	orders = gw.placedOrders()
	if got := len(orders); got != 20 {
		t.Fatalf("reprice orders = %d, want 20 with both sides retained", got)
	}
	for _, order := range orders[10:] {
		if order.Side != exchange.Buy && order.Side != exchange.Sell {
			t.Fatalf("unexpected reprice side %s", order.Side)
		}
	}
	if got := orders[19].Side; got != exchange.Sell {
		t.Fatalf("reprice omitted sell side; last order side = %s", got)
	}
}

func TestCrossPairMMRequotesMissingSideWithoutReplacingLiveQuote(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewCrossPairMM(1, gw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC"},
		BaseUSDSymbols: []string{"DEF-USD"},
		QuoteUSDSymbol: "ABC-USD", QuotePrecision: 1,
		TickSizes:  map[string]int64{"DEF-ABC": 1},
		LevelSizes: map[string]int64{"DEF-ABC": 1},
		Levels:     1, LevelSpacing: 1, RefreshInterval: time.Hour,
	})
	const symbol = "DEF-ABC"
	mm.usdMids["ABC-USD"] = 100
	mm.usdMids["DEF-USD"] = 200
	mm.recomputeMids()
	mm.quote(symbol)
	orders := gw.placedOrders()
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectSelfTrade})
	mm.onTick(time.Now())

	orders = gw.placedOrders()
	if len(orders) != 3 {
		t.Fatalf("one missing cross ask must produce one replacement, got %d total orders", len(orders))
	}
	if got := orders[2].Side; got != exchange.Sell {
		t.Fatalf("replacement side = %s, want sell", got)
	}
	mm.onTick(time.Now())
	if got := len(gw.placedOrders()); got != 3 {
		t.Fatalf("cross maker stacked orders after side replacement: got %d, want 3", got)
	}
}

func TestCrossPairMMKeepsInsufficientSideWithdrawnUntilOppositeFill(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewCrossPairMM(1, gw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC"},
		BaseUSDSymbols: []string{"DEF-USD"},
		QuoteUSDSymbol: "ABC-USD", QuotePrecision: 1,
		TickSizes:  map[string]int64{"DEF-ABC": 1},
		LevelSizes: map[string]int64{"DEF-ABC": 1},
		Levels:     1, LevelSpacing: 1, RefreshInterval: time.Hour,
	})
	const symbol = "DEF-ABC"
	mm.usdMids["ABC-USD"] = 100
	mm.usdMids["DEF-USD"] = 200
	mm.recomputeMids()
	mm.quote(symbol)
	orders := gw.placedOrders()
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectInsufficientBalance})
	mm.onTick(time.Now())
	mm.onTick(time.Now())
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("insufficient cross inventory must withdraw ask without retry spam, got %d orders", got)
	}

	mm.onFilled(actor.OrderFillEvent{OrderID: 11, IsFull: true})
	orders = gw.placedOrders()
	if len(orders) != 4 {
		t.Fatalf("cross buy fill must re-enable withdrawn ask, got %d total orders", len(orders))
	}
	if got := orders[3].Side; got != exchange.Sell {
		t.Fatalf("re-enabled side = %s, want sell", got)
	}
}

func TestCrossPairMMSellRestoresSharedQuoteInventoryAcrossSymbols(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewCrossPairMM(1, gw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC", "GHI-ABC"},
		BaseUSDSymbols: []string{"DEF-USD", "GHI-USD"},
		QuoteUSDSymbol: "ABC-USD", QuotePrecision: 1,
		TickSizes:  map[string]int64{"DEF-ABC": 1, "GHI-ABC": 1},
		LevelSizes: map[string]int64{"DEF-ABC": 1, "GHI-ABC": 1},
		Levels:     1, LevelSpacing: 1, RefreshInterval: time.Hour,
	})
	mm.usdMids["ABC-USD"] = 100
	mm.usdMids["DEF-USD"] = 200
	mm.usdMids["GHI-USD"] = 300
	mm.recomputeMids()
	mm.quote("DEF-ABC")
	mm.quote("GHI-ABC")

	orders := gw.placedOrders()
	var defAsk, ghiBid *etypes.OrderRequest
	for _, order := range orders {
		switch {
		case order.Symbol == "DEF-ABC" && order.Side == exchange.Sell:
			defAsk = order
		case order.Symbol == "GHI-ABC" && order.Side == exchange.Buy:
			ghiBid = order
		}
	}
	if defAsk == nil || ghiBid == nil {
		t.Fatalf("missing initial orders: DEF ask=%v GHI bid=%v", defAsk, ghiBid)
	}

	// The GHI bid was rejected because the shared ABC inventory was reserved
	// elsewhere. A DEF sell later restores ABC and must re-open that GHI bid.
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ghiBid.RequestID, Reason: exchange.RejectInsufficientBalance})
	ghiBidRef := quoteRef{symbol: "GHI-ABC", side: exchange.Buy}
	if !mm.withdrawn[ghiBidRef] {
		t.Fatal("GHI bid must be withdrawn after insufficient shared quote inventory")
	}
	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: defAsk.RequestID})
	mm.onFilled(actor.OrderFillEvent{OrderID: 11, IsFull: true})

	if mm.withdrawn[ghiBidRef] {
		t.Fatal("DEF sell did not restore shared ABC capacity for GHI bid")
	}
	orders = gw.placedOrders()
	if got, want := len(orders), 7; got != want {
		t.Fatalf("orders after DEF sell = %d, want %d (DEF re-quote plus GHI bid)", got, want)
	}
	if got := orders[len(orders)-1]; got.Symbol != "GHI-ABC" || got.Side != exchange.Buy {
		t.Fatalf("restored quote = %+v, want GHI-ABC buy", got)
	}
}

func TestCrossPairMMKeepsPartiallyFundedSideQuotedAfterReprice(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewCrossPairMM(1, gw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC"},
		BaseUSDSymbols: []string{"DEF-USD"},
		QuoteUSDSymbol: "ABC-USD", QuotePrecision: 1,
		TickSizes:  map[string]int64{"DEF-ABC": 1},
		LevelSizes: map[string]int64{"DEF-ABC": 1},
		Levels:     5, LevelSpacing: 1, RefreshInterval: time.Hour,
	})
	const symbol = "DEF-ABC"
	mm.usdMids["ABC-USD"] = 100
	mm.usdMids["DEF-USD"] = 1_000
	mm.recomputeMids()
	mm.quote(symbol)
	orders := gw.placedOrders()
	if len(orders) != 10 {
		t.Fatalf("initial orders = %d, want 10", len(orders))
	}

	for i, order := range orders {
		if order.Side == exchange.Buy || i < 9 {
			mm.onAccepted(actor.OrderAcceptedEvent{OrderID: uint64(100 + i), RequestID: order.RequestID})
		}
	}
	lastAsk := orders[len(orders)-1]
	if lastAsk.Side != exchange.Sell {
		t.Fatalf("last order side = %s, want sell", lastAsk.Side)
	}
	mm.onRejected(actor.OrderRejectedEvent{RequestID: lastAsk.RequestID, Reason: exchange.RejectInsufficientBalance})

	ref := quoteRef{symbol: symbol, side: exchange.Sell}
	if mm.withdrawn[ref] {
		t.Fatal("partial cross sell admission incorrectly withdrew funded sell levels")
	}
	mm.cancelAllForSym(symbol)
	mm.quote(symbol)

	orders = gw.placedOrders()
	if got := len(orders); got != 20 {
		t.Fatalf("reprice orders = %d, want 20 with both sides retained", got)
	}
	if got := orders[19].Side; got != exchange.Sell {
		t.Fatalf("reprice omitted cross sell side; last order side = %s", got)
	}
}

func basisSnapshot(symbol string, timestamp, bid, ask int64) actor.BookSnapshotEvent {
	return actor.BookSnapshotEvent{
		Symbol:    symbol,
		Timestamp: timestamp,
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: exchange.BTC_PRECISION}},
			Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: exchange.BTC_PRECISION}},
		},
	}
}

func updateBasisBooks(a *BasisArbActor, timestamp, spotBid, spotAsk, perpBid, perpAsk int64) {
	a.onSnapshot(basisSnapshot(a.cfg.SpotSymbol, timestamp, spotBid, spotAsk))
	a.onSnapshot(basisSnapshot(a.cfg.PerpSymbol, timestamp, perpBid, perpAsk))
}

func TestBasisArbUsesFreshExecutableFeeAwareBooks(t *testing.T) {
	gw := newRandomwalkStubGateway()
	arb := NewBasisArbActor(5, gw, BasisArbConfig{
		SpotSymbol:      "ABC-USD",
		PerpSymbol:      "ABC-PERP",
		SpotTakerFeeBps: 5,
		PerpTakerFeeBps: 5,
		LotSize:         exchange.BTC_PRECISION,
		MaxPosition:     2,
	})

	// A trade print alone is neither a current quote nor a two-sided execution
	// opportunity. The old actor would have used these prices directly.
	arb.HandleEvent(context.Background(), &actor.Event{Type: actor.EventTrade, Data: actor.TradeEvent{
		Symbol: "ABC-PERP", Trade: &exchange.Trade{Price: 200 * exchange.USD_PRECISION},
	}})
	arb.checkBasis()
	if got := len(gw.placedOrders()); got != 0 {
		t.Fatalf("trade print without books submitted %d orders", got)
	}

	// The mid-price basis looks positive, but crossing the actual spread pays
	// more in two 5 bps taker fees than the $0.08 gross difference earns.
	updateBasisBooks(arb, 1,
		100*exchange.USD_PRECISION, 100*exchange.USD_PRECISION,
		100*exchange.USD_PRECISION+8*exchange.CENT_TICK, 100*exchange.USD_PRECISION+10*exchange.CENT_TICK)
	arb.checkBasis()
	if got := len(gw.placedOrders()); got != 0 {
		t.Fatalf("fee-negative executable basis submitted %d orders", got)
	}

	// A fresh coherent snapshot with enough executable edge can open one pair.
	updateBasisBooks(arb, 2,
		100*exchange.USD_PRECISION, 100*exchange.USD_PRECISION,
		100*exchange.USD_PRECISION+20*exchange.CENT_TICK, 100*exchange.USD_PRECISION+25*exchange.CENT_TICK)
	arb.checkBasis()
	orders := gw.placedOrders()
	if len(orders) != 2 {
		t.Fatalf("executable fee-positive basis submitted %d orders, want 2", len(orders))
	}
	if orders[0].Symbol != "ABC-PERP" || orders[0].Side != exchange.Sell {
		t.Fatalf("perp opening leg = %+v, want sell ABC-PERP", orders[0])
	}
	if orders[1].Symbol != "ABC-USD" || orders[1].Side != exchange.Buy {
		t.Fatalf("spot opening leg = %+v, want buy ABC-USD", orders[1])
	}

	arb.onAccepted(actor.OrderAcceptedEvent{RequestID: orders[0].RequestID, OrderID: 101})
	arb.onAccepted(actor.OrderAcceptedEvent{RequestID: orders[1].RequestID, OrderID: 102})
	arb.onFilled(actor.OrderFillEvent{OrderID: 101, Symbol: "ABC-PERP", Side: exchange.Sell, Qty: exchange.BTC_PRECISION, IsFull: true})
	if arb.position != 0 || arb.pair == nil {
		t.Fatalf("one completed leg committed position=%d pair=%v", arb.position, arb.pair)
	}
	arb.onFilled(actor.OrderFillEvent{OrderID: 102, Symbol: "ABC-USD", Side: exchange.Buy, Qty: exchange.BTC_PRECISION, IsFull: true})
	if arb.position != 1 || arb.pair != nil {
		t.Fatalf("matched pair state position=%d pair=%v, want 1/nil", arb.position, arb.pair)
	}

	// Settling quickly must not replay the same stale book snapshot.
	arb.checkBasis()
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("stale snapshot submitted duplicate pair: got %d orders, want 2", got)
	}
}

func TestBasisArbNeutralizesPartialRejectedPairBeforeReentry(t *testing.T) {
	gw := newRandomwalkStubGateway()
	arb := NewBasisArbActor(5, gw, BasisArbConfig{
		SpotSymbol:  "ABC-USD",
		PerpSymbol:  "ABC-PERP",
		LotSize:     exchange.BTC_PRECISION,
		MaxPosition: 2,
	})
	updateBasisBooks(arb, 1,
		100*exchange.USD_PRECISION, 100*exchange.USD_PRECISION,
		102*exchange.USD_PRECISION, 103*exchange.USD_PRECISION)
	arb.checkBasis()
	orders := gw.placedOrders()
	if len(orders) != 2 {
		t.Fatalf("opening orders = %d, want 2", len(orders))
	}

	perp, spot := orders[0], orders[1]
	arb.onAccepted(actor.OrderAcceptedEvent{RequestID: perp.RequestID, OrderID: 201})
	arb.onAccepted(actor.OrderAcceptedEvent{RequestID: spot.RequestID, OrderID: 202})
	halfLot := int64(exchange.BTC_PRECISION / 2)
	arb.onFilled(actor.OrderFillEvent{OrderID: 201, Symbol: "ABC-PERP", Side: exchange.Sell, Qty: halfLot})
	arb.onRejected(actor.OrderRejectedEvent{RequestID: spot.RequestID, Reason: exchange.RejectInsufficientBalance})
	arb.onCancelled(actor.OrderCancelledEvent{OrderID: 201, RemainingQty: halfLot})

	orders = gw.placedOrders()
	if len(orders) != 3 {
		t.Fatalf("partial/rejected pair submitted %d orders, want one compensating order", len(orders))
	}
	hedge := orders[2]
	if hedge.Symbol != "ABC-PERP" || hedge.Side != exchange.Buy || hedge.Qty != halfLot {
		t.Fatalf("compensating order = %+v, want buy %d ABC-PERP", hedge, halfLot)
	}
	if arb.position != 0 || arb.pair == nil || !arb.pair.recovering {
		t.Fatalf("partial pair was counted or forgotten: position=%d pair=%+v", arb.position, arb.pair)
	}

	// The active compensating order blocks re-entry, even on fresh market data.
	updateBasisBooks(arb, 2,
		100*exchange.USD_PRECISION, 100*exchange.USD_PRECISION,
		102*exchange.USD_PRECISION, 103*exchange.USD_PRECISION)
	arb.checkBasis()
	if got := len(gw.placedOrders()); got != 3 {
		t.Fatalf("unresolved pair permitted re-entry: got %d orders, want 3", got)
	}

	// A rejected compensating order preserves the residual and retries only
	// after another coherent book update, rather than losing the exposure or
	// sending an immediate retry storm.
	arb.onRejected(actor.OrderRejectedEvent{RequestID: hedge.RequestID, Reason: exchange.RejectInsufficientBalance})
	arb.checkBasis()
	if got := len(gw.placedOrders()); got != 3 {
		t.Fatalf("unchanged snapshot retried rejected hedge: got %d orders, want 3", got)
	}
	updateBasisBooks(arb, 3,
		100*exchange.USD_PRECISION, 100*exchange.USD_PRECISION,
		102*exchange.USD_PRECISION, 103*exchange.USD_PRECISION)
	arb.checkBasis()
	orders = gw.placedOrders()
	if len(orders) != 4 {
		t.Fatalf("fresh snapshot did not retry hedge: got %d orders, want 4", len(orders))
	}
	hedge = orders[3]
	arb.onAccepted(actor.OrderAcceptedEvent{RequestID: hedge.RequestID, OrderID: 203})
	arb.onFilled(actor.OrderFillEvent{OrderID: 203, Symbol: "ABC-PERP", Side: exchange.Buy, Qty: halfLot, IsFull: true})
	if arb.position != 0 || arb.pair != nil {
		t.Fatalf("completed compensation state position=%d pair=%v, want 0/nil", arb.position, arb.pair)
	}
}
