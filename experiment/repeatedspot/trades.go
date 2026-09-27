package repeatedspot

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"exchange_sim/exchange"
	worldspot "exchange_sim/simulations/repeatedspot"
)

type acceptedOrder struct {
	OrderID   uint64 `json:"order_id"`
	RequestID uint64 `json:"request_id"`
	ClientID  uint64 `json:"client_id"`
	Side      string `json:"side"`
	Qty       int64  `json:"qty"`
	Price     int64  `json:"price"`
}

type recordedTrade struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         string `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type recordedFill struct {
	OrderID      uint64 `json:"order_id"`
	Symbol       string `json:"symbol"`
	Qty          int64  `json:"qty"`
	Price        int64  `json:"price"`
	Side         string `json:"side"`
	TradeID      uint64 `json:"trade_id"`
	Role         string `json:"role"`
	FeeAmount    int64  `json:"fee_amount"`
	FeeAsset     string `json:"fee_asset"`
	FilledQty    int64  `json:"filled_qty"`
	RemainingQty int64  `json:"remaining_qty"`
}

type settlementRecord struct {
	clientID  uint64
	timestamp int64
	changes   map[string]int64
}

type tradeRecord struct {
	trade       recordedTrade
	timestamp   int64
	settlements []settlementRecord
	fills       []struct {
		clientID uint64
		fill     recordedFill
	}
	feeEvidence int64
	venueFees   int64
}

type tradeAudit struct {
	instrument        worldspot.InstrumentConfig
	orders            map[uint64]acceptedOrder
	trades            map[uint64]*tradeRecord
	pendingSettlement []settlementRecord
	pendingVenueFees  map[uint64]int64
	pendingFeeEvents  map[uint64]int64
	lastTradeID       uint64
	hasLastTrade      bool
}

func newTradeAudit(instrument worldspot.InstrumentConfig) *tradeAudit {
	return &tradeAudit{instrument: instrument, orders: make(map[uint64]acceptedOrder),
		trades: make(map[uint64]*tradeRecord), pendingVenueFees: make(map[uint64]int64),
		pendingFeeEvents: make(map[uint64]int64)}
}

func (audit *tradeAudit) settlement(event Event, change exchange.BalanceChangeEvent) error {
	rows := make(map[string]int64, len(change.Changes))
	for _, delta := range change.Changes {
		rows[delta.Asset] = delta.Delta
	}
	audit.pendingSettlement = append(audit.pendingSettlement, settlementRecord{clientID: event.ClientID,
		timestamp: event.Timestamp, changes: rows})
	if len(audit.pendingSettlement) > 2 {
		return errors.New("repeated spot: more than two unanchored spot settlements")
	}
	return nil
}

func (audit *tradeAudit) venueFee(change exchange.VenueBalanceEvent) error {
	if audit.trades[change.TradeID] != nil {
		return errors.New("repeated spot: venue fee posted after its trade")
	}
	if change.Reason != "taker_fee" && change.Reason != "maker_fee" || change.Delta < 0 {
		return errors.New("repeated spot: unexpected E0 venue fee reason or rebate")
	}
	amount, ok := checkedAdd(audit.pendingVenueFees[change.TradeID], change.Delta)
	if !ok {
		return errors.New("repeated spot: venue fee total overflow")
	}
	audit.pendingVenueFees[change.TradeID] = amount
	return nil
}

func (audit *tradeAudit) visit(event Event) error {
	switch event.Name {
	case "OrderAccepted":
		var order acceptedOrder
		if err := json.Unmarshal(event.Payload, &order); err != nil {
			return err
		}
		if order.OrderID == 0 || order.RequestID == 0 || order.ClientID != event.ClientID ||
			order.Qty <= 0 || order.Price < 0 || !validSide(order.Side) || audit.orders[order.OrderID].OrderID != 0 {
			return errors.New("repeated spot: duplicate or invalid accepted order")
		}
		audit.orders[order.OrderID] = order
	case "OrderRejected":
		var rejected struct {
			RequestID uint64 `json:"request_id"`
			Symbol    string `json:"symbol"`
			Qty       int64  `json:"qty"`
		}
		if err := json.Unmarshal(event.Payload, &rejected); err != nil || rejected.RequestID == 0 || rejected.Symbol != audit.instrument.Symbol || rejected.Qty <= 0 {
			return errors.New("repeated spot: malformed order rejection")
		}
	case "OrderCancelled", "OrderCancelRejected":
		var cancellation struct {
			OrderID uint64 `json:"order_id"`
		}
		if err := json.Unmarshal(event.Payload, &cancellation); err != nil || cancellation.OrderID == 0 || audit.orders[cancellation.OrderID].OrderID == 0 {
			return errors.New("repeated spot: cancellation references unknown order")
		}
	case "BookSnapshot":
		var snapshot struct {
			Bids []exchange.PriceLevel `json:"bids"`
			Asks []exchange.PriceLevel `json:"asks"`
		}
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			return err
		}
	case "BookDelta":
		var delta struct {
			Price      int64 `json:"price"`
			VisibleQty int64 `json:"visible_qty"`
			HiddenQty  int64 `json:"hidden_qty"`
		}
		if err := json.Unmarshal(event.Payload, &delta); err != nil || delta.Price <= 0 || delta.VisibleQty < 0 || delta.HiddenQty < 0 {
			return errors.New("repeated spot: malformed book delta")
		}
	case "Trade":
		var trade recordedTrade
		if err := json.Unmarshal(event.Payload, &trade); err != nil {
			return err
		}
		if trade.Price <= 0 || trade.Qty <= 0 || !validSide(trade.Side) ||
			trade.MakerOrderID == trade.TakerOrderID || audit.trades[trade.TradeID] != nil || len(audit.pendingSettlement) != 2 {
			return errors.New("repeated spot: trade lacks exactly two prior settlements or has invalid identity")
		}
		if audit.hasLastTrade && len(audit.trades[audit.lastTradeID].fills) != 2 {
			return errors.New("repeated spot: previous trade lacks both fills before next trade")
		}
		if audit.orders[trade.TakerOrderID].OrderID == 0 || audit.orders[trade.MakerOrderID].OrderID == 0 {
			return errors.New("repeated spot: trade references unaccepted order")
		}
		for _, settlement := range audit.pendingSettlement {
			if settlement.timestamp != event.Timestamp {
				return errors.New("repeated spot: trade/settlement timestamp mismatch")
			}
		}
		audit.trades[trade.TradeID] = &tradeRecord{trade: trade, timestamp: event.Timestamp,
			settlements: audit.pendingSettlement}
		audit.pendingSettlement = nil
		audit.lastTradeID, audit.hasLastTrade = trade.TradeID, true
	case "OrderFill":
		var fill recordedFill
		if err := json.Unmarshal(event.Payload, &fill); err != nil {
			return err
		}
		record := audit.trades[fill.TradeID]
		if record == nil || fill.OrderID == 0 || fill.Symbol != audit.instrument.Symbol || fill.Price != record.trade.Price ||
			fill.Qty != record.trade.Qty || !validSide(fill.Side) || len(record.fills) >= 2 ||
			fill.FilledQty <= 0 || fill.RemainingQty < 0 || fill.FeeAmount < 0 {
			return errors.New("repeated spot: invalid, duplicate or unanchored order fill")
		}
		order := audit.orders[fill.OrderID]
		if order.OrderID == 0 || order.ClientID != event.ClientID || order.Side != fill.Side ||
			fill.FilledQty > order.Qty || fill.RemainingQty != order.Qty-fill.FilledQty ||
			fill.FeeAmount > 0 && fill.FeeAsset != audit.instrument.QuoteAsset {
			return errors.New("repeated spot: fill disagrees with accepted order or fee asset")
		}
		record.fills = append(record.fills, struct {
			clientID uint64
			fill     recordedFill
		}{event.ClientID, fill})
	case "fee_revenue":
		var fee exchange.FeeRevenueEvent
		if err := decodePayload(event.Payload, &fee); err != nil {
			return err
		}
		if fee.Timestamp != event.Timestamp || fee.Symbol != audit.instrument.Symbol ||
			fee.Asset != audit.instrument.QuoteAsset || fee.TakerFee < 0 || fee.MakerFee < 0 {
			return errors.New("repeated spot: malformed fee-revenue event")
		}
		if audit.trades[fee.TradeID] != nil {
			return errors.New("repeated spot: fee-revenue event posted after its trade")
		}
		amount, ok := checkedAdd(fee.TakerFee, fee.MakerFee)
		if !ok {
			return errors.New("repeated spot: fee-revenue arithmetic overflow")
		}
		audit.pendingFeeEvents[fee.TradeID], ok = checkedAdd(audit.pendingFeeEvents[fee.TradeID], amount)
		if !ok {
			return errors.New("repeated spot: cumulative fee-revenue overflow")
		}
	default:
		return fmt.Errorf("repeated spot: unsupported trade-related event %q", event.Name)
	}
	return nil
}

func (audit *tradeAudit) finish(state *replayState) error {
	if len(audit.pendingSettlement) != 0 {
		return errors.New("repeated spot: unanchored spot settlement at completion")
	}
	for tradeID, amount := range audit.pendingVenueFees {
		if audit.trades[tradeID] == nil {
			return fmt.Errorf("repeated spot: unanchored venue fee for trade %d", tradeID)
		}
		if audit.trades[tradeID].venueFees != 0 {
			return errors.New("repeated spot: venue fee was assigned twice")
		}
		audit.trades[tradeID].venueFees = amount
	}
	for tradeID, amount := range audit.pendingFeeEvents {
		if audit.trades[tradeID] == nil {
			return fmt.Errorf("repeated spot: unanchored fee-revenue event for trade %d", tradeID)
		}
		audit.trades[tradeID].feeEvidence = amount
	}
	for _, record := range audit.trades {
		if err := audit.checkTrade(record, state); err != nil {
			return err
		}
	}
	return nil
}

func (audit *tradeAudit) checkTrade(record *tradeRecord, state *replayState) error {
	if len(record.fills) != 2 || len(record.settlements) != 2 || record.settlements[0].clientID == record.settlements[1].clientID {
		return errors.New("repeated spot: trade has incomplete or same-account legs")
	}
	notional := new(big.Int).Mul(big.NewInt(record.trade.Qty), big.NewInt(record.trade.Price))
	notional.Quo(notional, big.NewInt(audit.instrument.BasePrecision))
	if !notional.IsInt64() || notional.Sign() <= 0 {
		return errors.New("repeated spot: invalid exact trade notional")
	}
	fees := int64(0)
	seenRoles := make(map[string]bool)
	seenClients := make(map[uint64]bool)
	for _, leg := range record.fills {
		fill := leg.fill
		if seenRoles[fill.Role] || seenClients[leg.clientID] || fill.Role != "taker" && fill.Role != "maker" {
			return errors.New("repeated spot: duplicated fill role or client")
		}
		seenRoles[fill.Role], seenClients[leg.clientID] = true, true
		if fill.Role == "taker" && (fill.OrderID != record.trade.TakerOrderID || fill.Side != record.trade.Side) ||
			fill.Role == "maker" && (fill.OrderID != record.trade.MakerOrderID || fill.Side == record.trade.Side) {
			return errors.New("repeated spot: fill role/side/order identity mismatch")
		}
		var expectedBase, expectedQuote int64
		var ok bool
		if fill.Side == "BUY" {
			expectedBase = record.trade.Qty
			expectedQuote, ok = checkedAdd(-notional.Int64(), -fill.FeeAmount)
		} else {
			expectedBase = -record.trade.Qty
			expectedQuote, ok = checkedAdd(notional.Int64(), -fill.FeeAmount)
		}
		if !ok {
			return errors.New("repeated spot: spot settlement arithmetic overflows")
		}
		matched := false
		for _, settlement := range record.settlements {
			if settlement.clientID != leg.clientID {
				continue
			}
			if settlement.changes[audit.instrument.BaseAsset] != expectedBase ||
				settlement.changes[audit.instrument.QuoteAsset] != expectedQuote {
				return errors.New("repeated spot: spot settlement disagrees with independent fill arithmetic")
			}
			matched = true
		}
		if !matched {
			return errors.New("repeated spot: fill has no client settlement")
		}
		feesNext, ok := checkedAdd(fees, fill.FeeAmount)
		if !ok {
			return errors.New("repeated spot: fill fee total overflow")
		}
		fees = feesNext
		account := state.accounts[leg.clientID]
		account.feePaidQuote, ok = checkedAdd(account.feePaidQuote, fill.FeeAmount)
		if !ok {
			return errors.New("repeated spot: account fee total overflow")
		}
		account.fillCount++
	}
	if !seenRoles["taker"] || !seenRoles["maker"] || fees != record.venueFees || fees != record.feeEvidence {
		return errors.New("repeated spot: fill fees do not reconcile with venue and fee-revenue evidence")
	}
	return nil
}

func validSide(side string) bool { return side == "BUY" || side == "SELL" }
