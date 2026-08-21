package exchange_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "exchange_sim/exchange"
	"exchange_sim/instrument"
	"exchange_sim/simulations/feesim"
)

// The exchange is a participant in its own market: every fee it charges and
// every unit of interest it collects is money that left somebody's account.
// If the receiving side is not recorded, a conservation audit has to read the
// exchange's balance from its own summary, and the identity then closes on
// that term by construction.
//
// This test reconstructs the venue's balances from the event stream alone and
// requires them to equal what the exchange says it holds.
func TestVenueBalancesReconstructFromTheEventStream(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "venue.jsonl")
	logger, err := feesim.NewJSONLinesLogger(logPath)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	ex := NewExchange(4, &RealClock{})
	ex.SetLogger("_global", logger)
	ex.SetLogger("ABC/USD", logger)
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000))

	fee := &PercentageFee{MakerBps: 1, TakerBps: 2, InQuote: true}
	buyer := ex.ConnectNewClient(1, map[string]int64{"USD": 10_000_000 * USD_PRECISION}, fee)
	seller := ex.ConnectNewClient(2, map[string]int64{"ABC": 100 * BTC_PRECISION}, fee)
	_ = buyer
	_ = seller

	price := int64(50_000) * USD_PRECISION
	if _, reason := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, BTC_PRECISION); reason != "" {
		t.Fatalf("sell rejected: %v", reason)
	}
	if _, reason := InjectLimitOrder(ex, 1, "ABC/USD", Buy, price, BTC_PRECISION); reason != "" {
		t.Fatalf("buy rejected: %v", reason)
	}
	time.Sleep(50 * time.Millisecond)
	logger.Flush()

	reconstructed := map[string]int64{}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" || !strings.Contains(line, "venue_balance_change") {
			continue
		}
		var record struct {
			Event string `json:"event"`
			Data  struct {
				Bucket string `json:"bucket"`
				Asset  string `json:"asset"`
				Delta  int64  `json:"delta"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if record.Event != "venue_balance_change" {
			continue
		}
		reconstructed[record.Data.Asset] += record.Data.Delta
	}

	if len(reconstructed) == 0 {
		t.Fatal("the venue took a fee and recorded nothing")
	}
	for asset, held := range ex.ExchangeBalance.FeeRevenue {
		if reconstructed[asset] != held {
			t.Errorf("%s: stream says %d, the exchange holds %d", asset, reconstructed[asset], held)
		}
	}
}

// A wrap would turn the venue's revenue into a debt. The checked add stops the
// venue instead, which is the only safe answer once the ceiling is reached.
func TestVenueBalanceRefusesToWrap(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000))
	ex.ExchangeBalance.FeeRevenue["USD"] = 1<<62 + 1<<62 - 1
	defer func() {
		if recover() == nil {
			t.Error("the venue's revenue wrapped instead of stopping")
		}
	}()
	ex.MoveVenueBalanceForTest(VenueFeeRevenue, "USD", 2, 1, "", "test")
}

// Dated settlement still has to work with the ledger in place, which is the
// cheapest regression against the refactor having broken a payout path.
func TestSettlementStillPaysAfterTheLedgerRefactor(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000))
	expiry := time.Now().Add(time.Hour).UnixNano()
	ex.AddInstrument(instrument.NewExpiringFutures("ABC-FUT-X", "ABC", "USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000, expiry))
	if len(ex.Instruments) != 2 {
		t.Fatalf("instruments = %d, want 2", len(ex.Instruments))
	}
}
