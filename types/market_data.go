package types

type MarketDataMsg struct {
	Type      MDType
	Symbol    string
	SeqNum    uint64
	Timestamp int64
	Data      any
}

type BookSnapshot struct {
	Bids []PriceLevel `json:"bids"`
	Asks []PriceLevel `json:"asks"`
}

type BookDelta struct {
	Side       Side  `json:"side"`
	Price      int64 `json:"price"`
	VisibleQty int64 `json:"visible_qty"`
	HiddenQty  int64 `json:"hidden_qty"`
}

type Trade struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         Side   `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type PriceLevel struct {
	Price      int64 `json:"price"`
	VisibleQty int64 `json:"visible_qty"`
	HiddenQty  int64 `json:"hidden_qty"`
}

type Subscription struct {
	ClientID uint64   `json:"client_id"`
	Symbol   string   `json:"symbol"`
	Types    []MDType `json:"types"`
}

type FundingRate struct {
	Symbol      string `json:"symbol"`
	Rate        int64  `json:"rate"`
	NextFunding int64  `json:"next_funding"`
	Interval    int64  `json:"interval"`
	MarkPrice   int64  `json:"mark_price"`
	IndexPrice  int64  `json:"index_price"`
}

type OpenInterest struct {
	Symbol         string `json:"symbol"`
	TotalContracts int64  `json:"total_contracts"`
	Timestamp      int64  `json:"timestamp"`
}
