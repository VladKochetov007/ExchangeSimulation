package repeatedspot

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type makerRecordingGateway struct {
	requests []exchange.Request
	response chan exchange.Response
	data     chan *exchange.MarketDataMsg
}

func (gateway *makerRecordingGateway) ID() uint64 { return 7 }
func (gateway *makerRecordingGateway) Send(request exchange.Request) {
	gateway.requests = append(gateway.requests, request)
}
func (gateway *makerRecordingGateway) Responses() <-chan exchange.Response { return gateway.response }
func (gateway *makerRecordingGateway) MarketDataCh() <-chan *exchange.MarketDataMsg {
	return gateway.data
}
func (gateway *makerRecordingGateway) IsRunning() bool { return true }

func makerFixture(t *testing.T) (*RecurringMaker, *makerRecordingGateway) {
	t.Helper()
	gateway := &makerRecordingGateway{response: make(chan exchange.Response), data: make(chan *exchange.MarketDataMsg)}
	maker, err := NewRecurringMaker(7, gateway, RecurringMakerConfig{
		Symbol: "ABC/USD", QuoteQty: 6, MinQuoteQty: 1, WorkingLimit: 10,
		TickSize: 1, QuoteInterval: time.Second, RequoteBps: 0,
	}, func(input MakerQuoteInput) (MakerQuote, bool) {
		return MakerQuote{BidPrice: input.MidPrice - 2, AskPrice: input.MidPrice + 2}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	return maker, gateway
}

func observeMakerBook(maker *RecurringMaker, bid, ask int64) {
	snapshot := &exchange.BookSnapshot{}
	if bid > 0 {
		snapshot.Bids = []exchange.PriceLevel{{Price: bid, VisibleQty: 1}}
	}
	if ask > 0 {
		snapshot.Asks = []exchange.PriceLevel{{Price: ask, VisibleQty: 1}}
	}
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: "ABC/USD", Snapshot: snapshot}})
}

func acceptMakerQuote(maker *RecurringMaker, requestID, orderID uint64) {
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted,
		Data: actor.OrderAcceptedEvent{RequestID: requestID, OrderID: orderID}})
}

func cancelMakerQuote(maker *RecurringMaker, orderID uint64, remaining int64) {
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelled,
		Data: actor.OrderCancelledEvent{OrderID: orderID, RemainingQty: remaining}})
}

func TestRecurringMakerCancelAndWaitWithoutFreeCapacity(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 1 || gateway.requests[0].Type != exchange.ReqSubscribe {
		t.Fatalf("first tick should only subscribe: %+v", gateway.requests)
	}
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 3 {
		t.Fatalf("first quote pair absent: %d requests", len(gateway.requests))
	}
	for _, request := range gateway.requests[1:] {
		if request.OrderReq == nil || !request.OrderReq.PostOnly || request.OrderReq.Qty != 6 {
			t.Fatalf("unmatched quote instructions: %+v", request)
		}
	}
	if lower, upper, err := maker.inventory.envelope(); err != nil || lower != -6 || upper != 6 {
		t.Fatalf("pending exposure = (%d,%d,%v)", lower, upper, err)
	}
	observeMakerBook(maker, 100, 102)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 3 {
		t.Fatal("pending orders were replaced before acknowledgement")
	}
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 5 || gateway.requests[3].Type != exchange.ReqCancelOrder || gateway.requests[4].Type != exchange.ReqCancelOrder {
		t.Fatalf("replacement must begin with both cancels: %+v", gateway.requests)
	}
	maker.onTick(time.Time{})
	if len(gateway.requests) != 5 {
		t.Fatal("cancel-in-flight created replacement capacity")
	}
	cancelMakerQuote(maker, 101, 6)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 5 {
		t.Fatal("one cancellation released the whole quote pair")
	}
	cancelMakerQuote(maker, 102, 6)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 7 || gateway.requests[5].OrderReq.Price != 99 || gateway.requests[6].OrderReq.Price != 103 {
		t.Fatalf("reconciled replacement pair absent: %+v", gateway.requests)
	}
	if maker.Fault() != nil {
		t.Fatal(maker.Fault())
	}
}

func TestRecurringMakerPartialFillShrinksSameSideQuoteAtCap(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{OrderID: 101, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 6, IsFull: true, TradeID: 501}})
	maker.onTick(time.Time{})
	if len(gateway.requests) != 4 || gateway.requests[3].Type != exchange.ReqCancelOrder {
		t.Fatalf("surviving ask must be cancelled before resizing bid: %+v", gateway.requests)
	}
	cancelMakerQuote(maker, 102, 6)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 6 || gateway.requests[4].OrderReq.Qty != 4 || gateway.requests[5].OrderReq.Qty != 6 {
		t.Fatalf("cap-aware pair quantities = %+v", gateway.requests)
	}
	if lower, upper, err := maker.inventory.envelope(); err != nil || lower != 0 || upper != 10 {
		t.Fatalf("cap-boundary exposure = (%d,%d,%v)", lower, upper, err)
	}
}

func TestRecurringMakerProcessesFirstTradeIDZero(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderPartialFill,
		Data: actor.OrderFillEvent{OrderID: 101, Symbol: "ABC/USD", Side: exchange.Buy,
			Qty: 1, IsFull: false, TradeID: 0}})
	if err := maker.Fault(); err != nil {
		t.Fatalf("maker rejected legitimate first exchange trade: %v", err)
	}
	if maker.inventory.filled != 1 {
		t.Fatalf("maker did not reconcile first fill: %d", maker.inventory.filled)
	}
}

func TestRecurringMakerInvalidCancelRemainderHaltsPolicy(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	cancelMakerQuote(maker, 101, 5)
	if maker.Fault() == nil {
		t.Fatal("contradictory terminal quantity did not fault maker")
	}
	requestCount := len(gateway.requests)
	maker.onTick(time.Time{})
	if len(gateway.requests) != requestCount {
		t.Fatal("faulted maker submitted a new order")
	}
}

func TestRecurringMakerWithdrawsWithoutTwoSidedDeliveredBook(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	observeMakerBook(maker, 99, 0)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 5 || gateway.requests[3].Type != exchange.ReqCancelOrder || gateway.requests[4].Type != exchange.ReqCancelOrder {
		t.Fatal("maker retained quotes after losing its declared reference")
	}
	cancelMakerQuote(maker, 101, 6)
	cancelMakerQuote(maker, 102, 6)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 5 {
		t.Fatal("maker requoted without a two-sided delivered book")
	}
}

func TestRecurringMakerFillBeforeCancelRejectionIsOrdinaryRace(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	observeMakerBook(maker, 100, 102)
	maker.onTick(time.Time{})
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{OrderID: 101, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 6, IsFull: true, TradeID: 501}})
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelRejected,
		Data: actor.OrderCancelRejectedEvent{OrderID: 101, Reason: exchange.RejectOrderAlreadyFilled}})
	if maker.Fault() != nil {
		t.Fatalf("earlier full fill followed by cancel rejection is valid: %v", maker.Fault())
	}
	if _, pending := maker.cancelRequested[101]; pending {
		t.Fatal("terminal order retained a cancel-pending state")
	}
	if _, stillWorking := maker.inventory.orders[101]; stillWorking {
		t.Fatal("full fill retained working exposure")
	}
}

func TestRecurringMakerPartialFillDuringCancelReconcilesBeforeReplacement(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	observeMakerBook(maker, 100, 102)
	maker.onTick(time.Time{})
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderPartialFill,
		Data: actor.OrderFillEvent{OrderID: 101, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 2, TradeID: 501}})
	if lower, upper, err := maker.inventory.envelope(); err != nil || lower != -4 || upper != 6 {
		t.Fatalf("cancel-in-flight partial exposure = (%d,%d,%v)", lower, upper, err)
	}
	cancelMakerQuote(maker, 101, 4)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 5 {
		t.Fatal("one reconciled cancel released pair early")
	}
	cancelMakerQuote(maker, 102, 6)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 7 {
		t.Fatalf("no replacement after both reconciliations: %d", len(gateway.requests))
	}
	if lower, upper, err := maker.inventory.envelope(); err != nil || lower != -4 || upper != 8 {
		t.Fatalf("replacement exposure = (%d,%d,%v)", lower, upper, err)
	}
}

func TestRecurringMakerOneSidedRiskCapDoesNotForceTwoSidedQuote(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.inventory.filled = maker.config.WorkingLimit
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 2 || gateway.requests[1].OrderReq.Side != exchange.Sell {
		t.Fatalf("maker at long cap should only offer inventory: %+v", gateway.requests)
	}
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 102)
	maker.onTick(time.Time{})
	if len(gateway.requests) != 2 {
		t.Fatal("one-sided economically feasible quote was churned merely because the bid was absent")
	}
}

func TestRecurringMakerRejectedUnknownTerminalRemainsUnreconciled(t *testing.T) {
	maker, gateway := makerFixture(t)
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	acceptMakerQuote(maker, gateway.requests[1].OrderReq.RequestID, 101)
	acceptMakerQuote(maker, gateway.requests[2].OrderReq.RequestID, 102)
	observeMakerBook(maker, 100, 102)
	maker.onTick(time.Time{})
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelRejected,
		Data: actor.OrderCancelRejectedEvent{OrderID: 101, Reason: exchange.RejectOrderNotFound}})
	if maker.Fault() == nil {
		t.Fatal("unreconciled terminal event must invalidate completion")
	}
	requestCount := len(gateway.requests)
	maker.onTick(time.Time{})
	if len(gateway.requests) != requestCount {
		t.Fatal("unreconciled cancel rejection triggered new orders")
	}
	cancelMakerQuote(maker, 101, 6)
	if maker.Fault() != nil {
		t.Fatalf("subsequent terminal notice should reconcile order: %v", maker.Fault())
	}
}

type fixtureFaultActor struct{ *actor.BaseActor }

func (participant *fixtureFaultActor) HandleEvent(context.Context, *actor.Event) {}
func (participant *fixtureFaultActor) Fault() error                              { return errors.New("fixture risk fault") }

func TestWorldRejectsActorRiskFaultAfterCompleteClock(t *testing.T) {
	config := fixtureConfig(t)
	policy, err := DefinePolicy("fault_probe", struct{}{}, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, _ struct{}) (actor.Actor, error) {
		probe := &fixtureFaultActor{BaseActor: actor.NewBaseActor(id, gateway)}
		probe.SetHandler(probe)
		return probe, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	config.Participants[1].Policy = policy
	world, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "fixture risk fault") {
		t.Fatalf("complete clock masked actor risk fault: %v", err)
	}
}

func TestQuoteRefreshThresholdUsesExactLargePriceArithmetic(t *testing.T) {
	for _, test := range []struct {
		difference int64
		reference  int64
		threshold  int64
		want       bool
	}{
		{1, 10_000, 1, true},
		{1, 10_001, 1, false},
		{math.MaxInt64 - 1, math.MaxInt64, 10_000, false},
		{math.MaxInt64, math.MaxInt64, 10_000, true},
	} {
		if got := priceMovedAtLeastBps(test.difference, test.reference, test.threshold); got != test.want {
			t.Fatalf("priceMovedAtLeastBps(%d, %d, %d) = %t, want %t",
				test.difference, test.reference, test.threshold, got, test.want)
		}
	}
}
