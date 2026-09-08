package exchange

import "testing"

type liquidationEventCollector struct {
	events []*LiquidationEvent
}

func (c *liquidationEventCollector) OnMarginCall(*MarginCallEvent) {}

func (c *liquidationEventCollector) OnLiquidation(event *LiquidationEvent) {
	c.events = append(c.events, event)
}

func (c *liquidationEventCollector) OnInsuranceFund(*InsuranceFundEvent) {}

func liquidationLogRecords(log *recordingLogger) []map[string]any {
	result := make([]map[string]any, 0)
	for _, record := range log.records {
		if record.event != "liquidation" {
			continue
		}
		payload, ok := record.data.(map[string]any)
		if ok {
			result = append(result, payload)
		}
	}
	return result
}

func TestLiquidationEvidenceReportsActualPartialExecution(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	perp := NewPerpFutures("PARTIAL-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.ConnectNewClient(2, nil, &FixedFee{})
	ex.AddPerpBalance(2, "USD", 1_000_000)
	log := &recordingLogger{}
	ex.SetLogger(perp.Symbol(), log)

	if delta := ex.Positions.UpdatePosition(1, perp.Symbol(), 10, 100, Buy, PositionBoth); delta.NewSize != 10 {
		t.Fatalf("position delta = %#v", delta)
	}
	if err := perp.UpdateFundingRate(50, 50); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: perp.Symbol(), Side: Buy, Type: LimitOrder,
		Price: 50, Qty: 4, TimeInForce: GTC, PositionSide: PositionBoth,
	}); !response.Success {
		t.Fatalf("covering liquidity rejected: %s", response.Error)
	}

	ex.CheckLiquidations(perp.Symbol(), perp, 50)
	position := ex.Positions.GetPosition(1, perp.Symbol())
	if position == nil || position.Size != 6 {
		t.Fatalf("residual position = %#v, want 6", position)
	}
	records := liquidationLogRecords(log)
	if len(records) != 1 {
		t.Fatalf("liquidation records = %#v, want one record", records)
	}
	record := records[0]
	for field, want := range map[string]any{
		"position_side":   "BOTH",
		"position_size":   int64(10),
		"attempted_qty":   int64(10),
		"filled_qty":      int64(4),
		"remaining_qty":   int64(6),
		"filled_notional": int64(200),
		"vwap_price":      int64(50),
		"fill_price":      int64(50),
		"remaining_debt":  int64(0),
	} {
		if got := record[field]; got != want {
			t.Fatalf("liquidation %s = %#v, want %#v; record=%#v", field, got, want, record)
		}
	}
	liquidationID, ok := record["liquidation_id"].(uint64)
	if !ok || liquidationID == 0 {
		t.Fatalf("liquidation_id = %#v, want positive uint64", record["liquidation_id"])
	}
}

func TestAccountLiquidationEvidenceAttributesDeficitOnce(t *testing.T) {
	ex, a, _ := seedCrossMarginLiquidationCase(t)
	defer ex.Shutdown()
	collector := &liquidationEventCollector{}
	ex.LiquidationHandler = collector
	// The two positions have offsetting mark PnL. A small negative wallet makes
	// the terminal account deficit deterministic after both positions close.
	ex.AddPerpBalance(1, "USD", -10)

	ex.checkLiquidationsAtEpoch(a.Symbol(), a, 50, ex.markEpoch)
	if len(collector.events) != 2 {
		t.Fatalf("liquidation events = %#v, want two position receipts", collector.events)
	}
	first := collector.events[0]
	second := collector.events[1]
	if first.LiquidationID == 0 || first.LiquidationID != second.LiquidationID {
		t.Fatalf("liquidation IDs = %d and %d, want one account batch", first.LiquidationID, second.LiquidationID)
	}
	if first.AttemptedQty != 10 || first.FilledQty != 10 || first.RemainingQty != 0 || first.VWAPPrice != 50 {
		t.Fatalf("first execution summary = %#v", first)
	}
	if second.AttemptedQty != 10 || second.FilledQty != 10 || second.RemainingQty != 0 || second.VWAPPrice != 150 {
		t.Fatalf("second execution summary = %#v", second)
	}
	if first.RemainingDebt != 10 || second.RemainingDebt != 0 {
		t.Fatalf("deficit attribution = %d and %d, want 10 and 0", first.RemainingDebt, second.RemainingDebt)
	}
	if got := ex.ExchangeBalance.InsuranceFund["USD"]; got != -10 {
		t.Fatalf("insurance deficit = %d, want -10", got)
	}
}
