package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
	"exchange_sim/instrument"
	etypes "exchange_sim/types"
)

// A participant that subscribes to the reference-data feed after a contract
// was listed must still learn that the contract exists. Without the replay it
// spends the whole run unaware of the chain it arrived to find, which is
// invisible over a few minutes and fatal over a lifecycle run.
func TestInstrumentFeedReplaysWhatIsAlreadyListed(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000))

	expiry := time.Now().Add(24 * time.Hour).UnixNano()
	ex.AddInstrument(instrument.NewExpiringFutures("ABC-FUT-1", "ABC", "USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000, expiry))

	gateway := ex.ConnectNewClient(1, map[string]int64{"USD": 1_000_000 * USD_PRECISION}, &FixedFee{})
	client, ok := gateway.(*ClientGateway)
	if !ok {
		t.Fatalf("gateway = %T, want a client gateway", gateway)
	}
	response := ex.Subscribe(1, &QueryRequest{RequestID: 1, Symbol: InstrumentFeedSymbol}, client)
	if !response.Success {
		t.Fatalf("subscribe failed: %+v", response)
	}

	announced := map[string]string{}
	draining := true
	for draining {
		select {
		case msg := <-client.MarketDataChan():
			if msg.Type != MDInstrument {
				continue
			}
			announcement, ok := msg.Data.(*etypes.InstrumentAnnouncement)
			if !ok {
				t.Fatalf("reference data carried %T", msg.Data)
			}
			announced[announcement.Symbol] = announcement.Action
		default:
			draining = false
		}
	}
	if announced["ABC-FUT-1"] != "listed" {
		t.Errorf("the listed future was not replayed to a late subscriber: %v", announced)
	}
	// A spot book has no lifecycle to announce, so it must not appear.
	if _, replayed := announced["ABC/USD"]; replayed {
		t.Errorf("a spot book was announced on the reference-data feed: %v", announced)
	}
}
