package analysis

import (
	"fmt"
	"sort"
	"strings"
)

type CrossVenueExchangeFill struct {
	Event     Event
	OrderID   uint64
	TradeID   uint64
	Symbol    string
	Side      string
	Qty       int64
	Price     int64
	FeeAmount int64
	FeeAsset  string
	Role      string
	Receipt   *CrossVenueResponseReceiptRecord
}

type crossVenueTrade struct {
	Event        Event
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         string `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type crossVenueFillWire struct {
	OrderID   uint64 `json:"order_id"`
	TradeID   uint64 `json:"trade_id"`
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Qty       int64  `json:"qty"`
	Price     int64  `json:"price"`
	FeeAmount int64  `json:"fee_amount"`
	FeeAsset  string `json:"fee_asset"`
	Role      string `json:"role"`
}

type crossVenueFillKey struct {
	venueID  string
	clientID uint64
	orderID  uint64
	tradeID  uint64
}

type crossVenueTradeKey struct {
	venueID string
	tradeID uint64
}

// CollectCrossVenueExchangeFills independently joins settled router fills to
// the venue's Trade records and optional actor-inbox notifications. A missing
// notification is retained, not invented: it may be horizon-censored and must
// be classified by the registered protocol using the response schedule.
func (r *Run) CollectCrossVenueExchangeFills(venues [2]string, clients map[string]uint64, symbol string, receipts []CrossVenueResponseReceiptRecord) ([]CrossVenueExchangeFill, error) {
	if r == nil || symbol == "" || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || len(clients) != 2 || clients[venues[0]] == 0 || clients[venues[1]] == 0 {
		return nil, fmt.Errorf("cross-venue fills: invalid selection")
	}
	spotLogName := strings.ReplaceAll(symbol, "/", "-")
	files := make([]string, 0, 2)
	for _, venue := range venues {
		selected := r.BookFiles(venue, spotLogName)
		if len(selected) != 1 {
			return nil, fmt.Errorf("cross-venue fills: venue %s has %d selected spot logs", venue, len(selected))
		}
		files = append(files, selected[0])
	}
	trades := make(map[crossVenueTradeKey]crossVenueTrade)
	fills := make(map[crossVenueFillKey]CrossVenueExchangeFill)
	var scanErr error
	if err := r.Scan(ScanOptions{Events: []string{"Trade", "OrderFill"}, Files: files, FilesSelected: true, Workers: 1}, func(event Event) {
		if scanErr != nil {
			return
		}
		if event.GlobalSequence == 0 || event.VenueID != venues[0] && event.VenueID != venues[1] || event.Symbol != "" && event.Symbol != symbol {
			scanErr = fmt.Errorf("event lacks selected global venue/symbol identity")
			return
		}
		if event.Name == "Trade" {
			var trade crossVenueTrade
			if err := decodeRequiredJSON(event.Raw(), &trade, "trade_id", "price", "qty", "side", "taker_order_id", "maker_order_id"); err != nil {
				scanErr = fmt.Errorf("malformed Trade evidence: %v", err)
				return
			}
			if trade.Qty <= 0 || trade.TakerOrderID == 0 || trade.MakerOrderID == 0 || trade.Side != "BUY" && trade.Side != "SELL" {
				scanErr = fmt.Errorf("Trade has invalid quantity, order or side identity")
				return
			}
			trade.Event = event
			key := crossVenueTradeKey{event.VenueID, trade.TradeID}
			if _, exists := trades[key]; exists {
				scanErr = fmt.Errorf("duplicate Trade identity %s/%d", event.VenueID, trade.TradeID)
				return
			}
			trades[key] = trade
			return
		}
		if event.ClientID != clients[event.VenueID] {
			return
		}
		var fill crossVenueFillWire
		if err := decodeRequiredJSON(event.Raw(), &fill, "order_id", "trade_id", "symbol", "side", "qty", "price", "fee_amount", "fee_asset", "role"); err != nil {
			scanErr = fmt.Errorf("malformed router OrderFill evidence: %v", err)
			return
		}
		if fill.OrderID == 0 || fill.Qty <= 0 || fill.Symbol != symbol || fill.Role != "taker" || fill.Side != "BUY" && fill.Side != "SELL" {
			scanErr = fmt.Errorf("router OrderFill has invalid order, quantity, symbol, side or role")
			return
		}
		key := crossVenueFillKey{event.VenueID, event.ClientID, fill.OrderID, fill.TradeID}
		if _, exists := fills[key]; exists {
			scanErr = fmt.Errorf("duplicate router OrderFill identity")
			return
		}
		fills[key] = CrossVenueExchangeFill{Event: event, OrderID: fill.OrderID, TradeID: fill.TradeID, Symbol: fill.Symbol, Side: fill.Side, Qty: fill.Qty, Price: fill.Price, FeeAmount: fill.FeeAmount, FeeAsset: fill.FeeAsset, Role: fill.Role}
	}); err != nil {
		return nil, err
	}
	if scanErr != nil {
		return nil, fmt.Errorf("cross-venue fills: %w", scanErr)
	}
	for key, fill := range fills {
		trade, exists := trades[crossVenueTradeKey{key.venueID, key.tradeID}]
		if !exists || trade.TakerOrderID != key.orderID || trade.Side != fill.Side || trade.Price != fill.Price || trade.Qty != fill.Qty || trade.Event.SimTS != fill.Event.SimTS || trade.Event.GlobalSequence >= fill.Event.GlobalSequence {
			return nil, fmt.Errorf("cross-venue fills: OrderFill %s/%d/%d lacks prior matching taker Trade", key.venueID, key.orderID, key.tradeID)
		}
	}
	seen := make(map[crossVenueFillKey]struct{}, len(receipts))
	for index := range receipts {
		receipt := &receipts[index]
		if err := validateCrossVenueResponseReceipt(*receipt, venues, receipt.Payload.RouterID); err != nil {
			return nil, fmt.Errorf("cross-venue fills: invalid actor response receipt: %w", err)
		}
		if receipt.Payload.Kind != "FILL" {
			continue
		}
		key := crossVenueFillKey{receipt.Payload.VenueID, receipt.Payload.ClientID, receipt.Payload.OrderID, receipt.Payload.TradeID}
		fill, exists := fills[key]
		if _, duplicate := seen[key]; duplicate || !exists {
			return nil, fmt.Errorf("cross-venue fills: unanchored or duplicate actor fill notification")
		}
		seen[key] = struct{}{}
		if fill.Qty != receipt.Payload.Qty || fill.Price != receipt.Payload.Price || fill.Side != receipt.Payload.Side || fill.FeeAmount != receipt.Payload.FeeAmount || fill.FeeAsset != receipt.Payload.FeeAsset || fill.Event.SimTS != receipt.Payload.ExchangeAt || fill.Event.GlobalSequence >= receipt.Event.GlobalSequence {
			return nil, fmt.Errorf("cross-venue fills: actor notification contradicts settled exchange fill")
		}
		fill.Receipt = receipt
		fills[key] = fill
	}
	result := make([]CrossVenueExchangeFill, 0, len(fills))
	for _, fill := range fills {
		result = append(result, fill)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Event.GlobalSequence < result[right].Event.GlobalSequence
	})
	return result, nil
}
