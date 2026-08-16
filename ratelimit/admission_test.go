package ratelimit

import "testing"

func TestRiskReducingRequestsTakeThePriorityLane(t *testing.T) {
	for kind, wantPriority := range map[RequestKind]bool{
		KindCancelOrder:     true,
		KindCancelAll:       true,
		KindPlaceOrder:      false,
		KindQueryBalance:    false,
		KindSubscribe:       false,
		KindPlaceReduceOnly: true,
	} {
		if got := kind.RiskReducing(); got != wantPriority {
			t.Fatalf("%v risk-reducing = %v, want %v", kind, got, wantPriority)
		}
	}
}

// The point of splitting the queue: when the engine saturates, the lane that
// fills is the one carrying new risk, and the venue can refuse it honestly
// while still accepting the requests that take risk off.
func TestSecondaryLaneSaturatesWhilePriorityStillAccepts(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: Depth(2), SecondaryDepth: Depth(2)})

	for i := 0; i < 2; i++ {
		if decision, _ := queue.Offer(KindPlaceOrder); !decision.Allowed {
			t.Fatalf("secondary lane rejected below its depth at %d: %+v", i, decision)
		}
	}
	decision, _ := queue.Offer(KindPlaceOrder)
	if decision.Allowed {
		t.Fatal("secondary lane accepted beyond its depth")
	}
	if decision.Limit != "queue_secondary" {
		t.Fatalf("overload rejection did not name the saturated lane: %+v", decision)
	}
	if !decision.Overloaded {
		t.Fatalf("overload rejection was not marked overloaded: %+v", decision)
	}
	// The whole purpose: cancels still get through.
	if decision, _ := queue.Offer(KindCancelOrder); !decision.Allowed {
		t.Fatalf("priority lane rejected a cancel while only the secondary lane was full: %+v", decision)
	}
}

func TestPriorityLaneCanAlsoSaturate(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: Depth(1), SecondaryDepth: Depth(1)})
	queue.Offer(KindCancelOrder)

	decision, _ := queue.Offer(KindCancelOrder)
	if decision.Allowed {
		t.Fatal("priority lane accepted beyond its depth")
	}
	if decision.Limit != "queue_priority" {
		t.Fatalf("rejection named the wrong lane: %+v", decision)
	}
}

func TestCompletingWorkFreesTheLaneItCameFrom(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: Depth(1), SecondaryDepth: Depth(1)})
	_, slot := queue.Offer(KindPlaceOrder)
	if decision, _ := queue.Offer(KindPlaceOrder); decision.Allowed {
		t.Fatal("secondary lane accepted beyond its depth")
	}

	queue.Complete(slot)
	if decision, _ := queue.Offer(KindPlaceOrder); !decision.Allowed {
		t.Fatalf("completing work did not free the secondary lane: %+v", decision)
	}
	// Redeeming a spent slot must not create capacity.
	queue.Complete(slot)
	queue.Complete(slot)
	queue.Offer(KindPlaceOrder)
	if decision, _ := queue.Offer(KindPlaceOrder); decision.Allowed {
		t.Fatal("over-completion manufactured queue capacity")
	}
}

func TestZeroDepthMeansUnlimited(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{})
	for i := 0; i < 1000; i++ {
		if decision, _ := queue.Offer(KindPlaceOrder); !decision.Allowed {
			t.Fatalf("unconfigured queue rejected at %d: %+v", i, decision)
		}
	}
}

// A venue's scheme is several budgets at once, each in its own currency. All
// must admit, and the first that refuses is the one reported. A query must not
// consume the order-count budget, which is why that meter charges it nothing.
func TestGateChargesEveryLimiterAndReportsTheFirstRefusal(t *testing.T) {
	// Two budgets in different currencies, as venues publish them: a placement
	// costs 10 of request weight and 1 of order count.
	weight := NewFixedWindow("weight", 100, minute)
	orders := NewFixedWindow("orders", 2, second)
	gate := NewGate([]Meter{
		{Limiter: weight, Cost: StaticCost{Table: map[RequestKind]int64{KindPlaceOrder: 10}}},
		{Limiter: orders, Cost: StaticCost{Table: map[RequestKind]int64{KindPlaceOrder: 1}}},
	}, nil)

	for i := 0; i < 2; i++ {
		if decision, _ := gate.Admit("acct", KindPlaceOrder, 0); !decision.Allowed {
			t.Fatalf("request %d rejected unexpectedly: %+v", i, decision)
		}
	}
	decision, _ := gate.Admit("acct", KindPlaceOrder, 0)
	if decision.Allowed {
		t.Fatal("order-count budget did not bind")
	}
	if decision.Limit != "orders" {
		t.Fatalf("rejection named %q, want the order-count budget", decision.Limit)
	}
	// The binding limit must not have consumed the others' budgets on the way
	// through, or a rejected request would still cost weight.
	if used := weight.Used("acct", 0); used != 20 {
		t.Fatalf("weight used = %d, want 20: a rejected request was charged", used)
	}
}

func TestGateWithoutACostForAKindChargesTheDefault(t *testing.T) {
	weight := NewFixedWindow("weight", 10, minute)
	gate := NewGate([]Meter{{Limiter: weight, Cost: StaticCost{Default: 4}}}, nil)

	gate.Admit("acct", KindQueryBalance, 0)
	if used := weight.Used("acct", 0); used != 4 {
		t.Fatalf("weight used = %d, want the default cost 4", used)
	}
}
