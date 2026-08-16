package exchange

import (
	"testing"

	"exchange_sim/ratelimit"
)

// admitAll is the null policy: a venue without a configured budget must behave
// exactly as it did before request gating existed.
type recordingPolicy struct {
	seen     []ratelimit.RequestKind
	refuse   map[ratelimit.RequestKind]Response
	released int
}

func (p *recordingPolicy) Admit(clientID uint64, kind ratelimit.RequestKind, now int64) (RequestPermit, Response, bool) {
	p.seen = append(p.seen, kind)
	if resp, refused := p.refuse[kind]; refused {
		return RequestPermit{}, resp, false
	}
	return RequestPermit{Held: true}, Response{}, true
}

func (p *recordingPolicy) Release(RequestPermit) { p.released++ }

func TestRequestKindsAreClassifiedForTheGate(t *testing.T) {
	for requestType, want := range map[RequestType]ratelimit.RequestKind{
		ReqPlaceOrder:       ratelimit.KindPlaceOrder,
		ReqCancelOrder:      ratelimit.KindCancelOrder,
		ReqQueryBalance:     ratelimit.KindQueryBalance,
		ReqQueryAccount:     ratelimit.KindQueryAccount,
		ReqQueryInstruments: ratelimit.KindQueryOrder,
		ReqSubscribe:        ratelimit.KindSubscribe,
		ReqUnsubscribe:      ratelimit.KindUnsubscribe,
	} {
		if got := classifyRequest(Request{Type: requestType}); got != want {
			t.Fatalf("%s classified as %v, want %v", requestType, got, want)
		}
	}
}

// A reduce-only order is risk-reducing even though it is a placement, and the
// venue must classify it that way or a saturated queue will refuse the one
// request that would make the client safer.
func TestReduceOnlyPlacementIsClassifiedAsRiskReducing(t *testing.T) {
	req := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{ReduceOnly: true}}
	kind := classifyRequest(req)
	if kind != ratelimit.KindPlaceReduceOnly {
		t.Fatalf("reduce-only placement classified as %v", kind)
	}
	if !kind.RiskReducing() {
		t.Fatal("reduce-only placement is not risk-reducing")
	}
}

func TestRefusedRequestsAreRejectedWithoutTouchingTheBook(t *testing.T) {
	policy := &recordingPolicy{refuse: map[ratelimit.RequestKind]Response{
		ratelimit.KindPlaceOrder: {Success: false, Error: RejectRateLimited},
	}}
	exchange := newGatedExchange(t, policy)
	const clientID = uint64(1)
	exchange.ConnectNewClient(clientID, map[string]int64{"USD": 1_000_000 * USD_PRECISION}, nil)

	resp := exchange.PlaceOrderGated(clientID, &OrderRequest{
		Symbol: "ABC/USD", Side: Buy, Type: LimitOrder, Price: 100 * USD_PRECISION, Qty: BTC_PRECISION,
	})
	if resp.Success {
		t.Fatal("a refused request was executed")
	}
	if resp.Error != RejectRateLimited {
		t.Fatalf("reason = %q, want %q", resp.Error, RejectRateLimited)
	}
	book := exchange.Books["ABC/USD"]
	if book != nil && book.Bids.Best != nil {
		t.Fatal("a refused order reached the book")
	}
}

func TestPermitsAreReleasedAfterTheRequestCompletes(t *testing.T) {
	policy := &recordingPolicy{}
	exchange := newGatedExchange(t, policy)
	const clientID = uint64(1)
	exchange.ConnectNewClient(clientID, map[string]int64{"USD": 1_000_000 * USD_PRECISION}, nil)

	exchange.PlaceOrderGated(clientID, &OrderRequest{
		Symbol: "ABC/USD", Side: Buy, Type: LimitOrder, Price: 100 * USD_PRECISION, Qty: BTC_PRECISION,
	})
	if policy.released != 1 {
		t.Fatalf("permits released = %d, want 1: a completed request kept its queue slot", policy.released)
	}
}

func newGatedExchange(t *testing.T, policy RequestPolicy) *DefaultExchange {
	t.Helper()
	exchange := NewExchange(100, &fixedClock{})
	exchange.RequestPolicy = policy
	exchange.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000))
	return exchange
}

type fixedClock struct{}

func (fixedClock) NowUnixNano() int64 { return 0 }
func (fixedClock) NowUnix() int64     { return 0 }
