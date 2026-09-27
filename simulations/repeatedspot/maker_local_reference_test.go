package repeatedspot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestLocalReferencePolicyParametersRoundTrip(t *testing.T) {
	makerConfig := RecurringMakerConfig{Symbol: "ABC/USD", QuoteQty: 6, MinQuoteQty: 1,
		WorkingLimit: 10, TickSize: 1, QuoteInterval: time.Second}
	policy, err := NewLocalReferenceFixedMakerPolicy(LocalReferenceFixedMakerConfig{
		BoundedFixedMakerConfig: BoundedFixedMakerConfig{Maker: makerConfig, SpreadBps: 2},
		MaxAge:                  15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	var parameters struct {
		Maker             RecurringMakerConfig `json:"maker"`
		SpreadBps         int64                `json:"spread_bps"`
		LocalReferenceAge int64                `json:"local_reference_max_age_ns"`
	}
	if err := json.Unmarshal(policy.parameters, &parameters); err != nil {
		t.Fatal(err)
	}
	if policy.name != "bounded_fixed_maker_v3" || parameters.Maker != makerConfig ||
		parameters.SpreadBps != 2 || parameters.LocalReferenceAge != int64(15*time.Second) {
		t.Fatalf("effective local-reference policy is not reconstructible: %s", policy.parameters)
	}
}

func localReferenceFixture(t *testing.T, maxAge time.Duration) (*RecurringMaker, *makerRecordingGateway, *[]MakerDecision) {
	t.Helper()
	gateway := &makerRecordingGateway{response: make(chan exchange.Response), data: make(chan *exchange.MarketDataMsg)}
	maker, err := NewRecurringMakerWithLocalReference(7, gateway, RecurringMakerConfig{
		Symbol: "ABC/USD", QuoteQty: 6, MinQuoteQty: 1, WorkingLimit: 10,
		TickSize: 1, QuoteInterval: time.Second,
	}, func(input MakerQuoteInput) (MakerQuote, bool) {
		return MakerQuote{BidPrice: input.MidPrice - 2, AskPrice: input.MidPrice + 2}, true
	}, maxAge)
	if err != nil {
		t.Fatal(err)
	}
	decisions := new([]MakerDecision)
	maker.SetDecisionObserver(func(decision MakerDecision) { *decisions = append(*decisions, decision) })
	maker.onTick(time.Unix(0, 0))
	return maker, gateway, decisions
}

func deliverMakerBookAt(maker *RecurringMaker, at time.Duration, sequence uint64, bid, ask int64) {
	snapshot := &exchange.BookSnapshot{}
	if bid > 0 {
		snapshot.Bids = []exchange.PriceLevel{{Price: bid, VisibleQty: 1}}
	}
	if ask > 0 {
		snapshot.Asks = []exchange.PriceLevel{{Price: ask, VisibleQty: 1}}
	}
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: "ABC/USD", Timestamp: int64(at), SeqNum: sequence, Snapshot: snapshot}})
}

func TestLocalReferenceQuotesFromDeliveredTwoSidedCacheOnlyBeforeExpiry(t *testing.T) {
	maker, gateway, decisions := localReferenceFixture(t, 15*time.Second)
	deliverMakerBookAt(maker, time.Second, 11, 99, 101)
	deliverMakerBookAt(maker, 2*time.Second, 12, 99, 0)
	maker.onTick(time.Unix(0, int64(5*time.Second)))
	if err := maker.Fault(); err != nil {
		t.Fatal(err)
	}
	if len(gateway.requests) != 3 || gateway.requests[1].OrderReq.Price != 98 ||
		gateway.requests[2].OrderReq.Price != 102 {
		t.Fatalf("no ordinary quote from the delivered local reference: %+v", gateway.requests)
	}
	last := (*decisions)[len(*decisions)-1]
	if last.Action != "evaluate_placements" || last.ReferenceMode != "cached_two_sided" ||
		last.ReferenceMid != 100 || last.ReferenceSourceAt != int64(time.Second) ||
		last.ReferenceSequence != 11 || last.BestBid != 99 || last.BestAsk != 0 {
		t.Fatalf("cached decision omitted its local source identity: %+v", last)
	}

	expired, expiredGateway, expiredDecisions := localReferenceFixture(t, 15*time.Second)
	deliverMakerBookAt(expired, time.Second, 11, 99, 101)
	deliverMakerBookAt(expired, 2*time.Second, 12, 99, 0)
	expired.onTick(time.Unix(0, int64(16*time.Second)))
	if len(expiredGateway.requests) != 1 || (*expiredDecisions)[1].Action != "no_usable_quote" ||
		(*expiredDecisions)[1].ReferenceMode != "unavailable" {
		t.Fatalf("reference at the exact expiry boundary remained active: %+v", *expiredDecisions)
	}
}

func TestLocalReferenceNeverSeenCrossedAndDisabledDoNotRecover(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		maxAge time.Duration
		books  [][4]int64
	}{
		{name: "never_seen", maxAge: 15 * time.Second, books: [][4]int64{{1, 11, 99, 0}}},
		{name: "crossed_latest", maxAge: 15 * time.Second, books: [][4]int64{{1, 11, 99, 101}, {2, 12, 101, 100}}},
		{name: "disabled", maxAge: 0, books: [][4]int64{{1, 11, 99, 101}, {2, 12, 99, 0}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			maker, gateway, decisions := localReferenceFixture(t, scenario.maxAge)
			for _, book := range scenario.books {
				deliverMakerBookAt(maker, time.Duration(book[0])*time.Second, uint64(book[1]), book[2], book[3])
			}
			maker.onTick(time.Unix(0, int64(5*time.Second)))
			if len(gateway.requests) != 1 || (*decisions)[1].Action != "no_usable_quote" ||
				(*decisions)[1].ReferenceMode != "unavailable" {
				t.Fatalf("invalid cache generated a quote: %+v, requests=%+v", *decisions, gateway.requests)
			}
		})
	}
}

func TestLocalReferenceRiskLimitCanLeaveOneSideAbsent(t *testing.T) {
	maker, gateway, decisions := localReferenceFixture(t, 15*time.Second)
	maker.inventory.filled = maker.config.WorkingLimit
	deliverMakerBookAt(maker, time.Second, 11, 99, 101)
	deliverMakerBookAt(maker, 2*time.Second, 12, 99, 0)
	maker.onTick(time.Unix(0, int64(5*time.Second)))
	if len(gateway.requests) != 2 || gateway.requests[1].OrderReq.Side != exchange.Sell ||
		(*decisions)[1].ReferenceMode != "cached_two_sided" {
		t.Fatalf("cached reference forced a prohibited buy side: %+v", gateway.requests)
	}
}
