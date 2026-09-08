package derivsim

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestContractSetOrderedContracts(t *testing.T) {
	set := newContractSet("ABC/USD")
	for _, symbol := range []string{"ABC-2", "ABC-1", "ABC-3"} {
		set.contracts[symbol] = &Contract{Symbol: symbol}
	}
	got := set.orderedContracts()
	for i, want := range []string{"ABC-1", "ABC-2", "ABC-3"} {
		if got[i].Symbol != want {
			t.Fatalf("contract %d = %q, want %q", i, got[i].Symbol, want)
		}
	}
}

func TestContractSetRoutesForcedFillByAuthoritativeSymbol(t *testing.T) {
	set := newContractSet("ABC/USD")
	var got string
	set.onFill = func(symbol string, _ actor.OrderFillEvent) { got = symbol }
	set.handle(&actor.Event{Type: actor.EventInstrument, Data: actor.InstrumentEvent{Announcement: &exchange.InstrumentAnnouncement{
		Action: "listed", Symbol: "ABC-C-100", InstrumentType: "OPTION", Underlying: "ABC/USD", ExpiryNano: time.Now().Add(time.Hour).UnixNano(),
	}}})
	set.handle(&actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 999, Symbol: "ABC-C-100", Qty: 1, Side: exchange.Buy, IsFull: true, Forced: true,
	}})
	if got != "ABC-C-100" {
		t.Fatalf("forced fill symbol = %q, want authoritative contract symbol", got)
	}
	if len(set.earlyFill) != 0 {
		t.Fatalf("forced fill populated early-fill buffer: %#v", set.earlyFill)
	}
}
