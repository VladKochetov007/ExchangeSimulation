package exchange

import "exchange_sim/ratelimit"

// RequestPermit is what a policy hands back when it admits a request. It is
// returned to the policy once the request has been processed, so a venue that
// models a bounded execution queue can free the slot it took.
type RequestPermit struct {
	Held bool
	Slot ratelimit.Slot
}

// RequestPolicy decides whether the venue will accept a request now. It is the
// seam between the exchange and whatever budget scheme a venue publishes:
// the exchange knows what was asked and when, and the policy knows what that
// costs and whether there is room.
//
// A nil policy means an unmetered venue, which is what every existing scenario
// expects, so gating is opt-in.
type RequestPolicy interface {
	// Admit reports whether the request may proceed. When it may not, the
	// Response is sent to the client unchanged, so the policy chooses the
	// rejection reason and any retry advice it carries.
	Admit(clientID uint64, kind ratelimit.RequestKind, now int64) (RequestPermit, Response, bool)
	// Release returns a permit once the request has been processed.
	Release(RequestPermit)
}

// classifyRequest maps a request onto the kind a budget scheme prices and a
// queue prioritises. Reduce-only placements are their own kind because they add
// no risk: a saturated venue should still accept them.
func classifyRequest(req Request) ratelimit.RequestKind {
	switch req.Type {
	case ReqPlaceOrder:
		if req.OrderReq != nil && req.OrderReq.ReduceOnly {
			return ratelimit.KindPlaceReduceOnly
		}
		return ratelimit.KindPlaceOrder
	case ReqCancelOrder:
		return ratelimit.KindCancelOrder
	case ReqQueryBalance:
		return ratelimit.KindQueryBalance
	case ReqQueryAccount:
		return ratelimit.KindQueryAccount
	case ReqQueryInstruments:
		return ratelimit.KindQueryOrder
	case ReqSubscribe:
		return ratelimit.KindSubscribe
	case ReqUnsubscribe:
		return ratelimit.KindUnsubscribe
	default:
		return ratelimit.KindUnknown
	}
}

// admitRequest asks the configured policy whether a request may proceed. It
// returns the permit to release afterwards, the rejection to send when refused,
// and whether to continue.
func (e *DefaultExchange) admitRequest(clientID uint64, req Request) (RequestPermit, Response, bool) {
	if e.RequestPolicy == nil {
		return RequestPermit{}, Response{}, true
	}
	return e.RequestPolicy.Admit(clientID, classifyRequest(req), e.Clock.NowUnixNano())
}

func (e *DefaultExchange) releasePermit(permit RequestPermit) {
	if e.RequestPolicy != nil && permit.Held {
		e.RequestPolicy.Release(permit)
	}
}

// PlaceOrderGated is PlaceOrder with the venue's request policy applied. It
// exists so a caller can exercise gating without going through a gateway.
func (e *DefaultExchange) PlaceOrderGated(clientID uint64, req *OrderRequest) Response {
	permit, rejection, admitted := e.admitRequest(clientID, Request{Type: ReqPlaceOrder, OrderReq: req})
	if !admitted {
		return rejection
	}
	defer e.releasePermit(permit)
	return e.PlaceOrder(clientID, req)
}
