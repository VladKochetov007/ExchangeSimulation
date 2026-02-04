package exchange

type Side uint8

const (
	Buy Side = iota
	Sell
)

type OrderType uint8

const (
	Market OrderType = iota
	LimitOrder
)

type TimeInForce uint8

const (
	GTC TimeInForce = iota
	IOC
	FOK
)

type Visibility uint8

const (
	Normal Visibility = iota
	Iceberg
	Hidden
)

type OrderStatus uint8

const (
	Open OrderStatus = iota
	PartialFill
	Filled
	Cancelled
	Rejected
)

type RejectReason uint8

const (
	RejectInsufficientBalance RejectReason = iota
	RejectInvalidPrice
	RejectInvalidQty
	RejectUnknownClient
	RejectUnknownInstrument
	RejectSelfTrade
	RejectDuplicateOrderID
	RejectOrderNotFound
	RejectOrderNotOwned
	RejectOrderAlreadyFilled
	RejectFOKNotFilled
)

type Order struct {
	ID          uint64
	ClientID    uint64
	Side        Side
	Type        OrderType
	TimeInForce TimeInForce
	Price       int64
	Qty         int64
	FilledQty   int64
	Visibility  Visibility
	IcebergQty  int64
	Status      OrderStatus
	Timestamp   int64

	Prev   *Order
	Next   *Order
	Parent *Limit
}

type Limit struct {
	Price    int64
	TotalQty int64
	OrderCnt int32

	Head *Order
	Tail *Order

	Prev *Limit
	Next *Limit
}

type RequestType uint8

const (
	ReqPlaceOrder RequestType = iota
	ReqCancelOrder
	ReqQueryBalance
	ReqQueryOrders
	ReqSubscribe
	ReqUnsubscribe
)

type QueryType uint8

const (
	QueryBalance QueryType = iota
	QueryOrders
	QueryOrder
)

type Request struct {
	Type      RequestType
	OrderReq  *OrderRequest
	CancelReq *CancelRequest
	QueryReq  *QueryRequest
}

type Response struct {
	RequestID uint64
	Success   bool
	Data      any
	Error     RejectReason
}

type FillNotification struct {
	OrderID   uint64
	ClientID  uint64
	TradeID   uint64
	Qty       int64
	Price     int64
	Side      Side
	IsFull    bool
	FeeAmount int64
	FeeAsset  string
}

type OrderRequest struct {
	RequestID   uint64
	Side        Side
	Type        OrderType
	Price       int64
	Qty         int64
	Symbol      string
	TimeInForce TimeInForce
	Visibility  Visibility
	IcebergQty  int64
}

type CancelRequest struct {
	RequestID uint64
	OrderID   uint64
}

type QueryRequest struct {
	RequestID uint64
	QueryType QueryType
	Symbol    string
}

type BalanceSnapshot struct {
	Timestamp int64
	Balances  []AssetBalance
}

type AssetBalance struct {
	Asset     string
	Total     int64
	Available int64
	Reserved  int64
}

type MDType uint8

const (
	MDSnapshot MDType = iota
	MDDelta
	MDTrade
	MDFunding
	MDOpenInterest
)

type MarketDataMsg struct {
	Type      MDType
	Symbol    string
	SeqNum    uint64
	Timestamp int64
	Data      any
}

type BookSnapshot struct {
	Bids []PriceLevel
	Asks []PriceLevel
}

type BookDelta struct {
	Side       Side
	Price      int64
	VisibleQty int64
	HiddenQty  int64
}

type Trade struct {
	TradeID      uint64
	Price        int64
	Qty          int64
	Side         Side
	TakerOrderID uint64
	MakerOrderID uint64
}

type PriceLevel struct {
	Price      int64
	VisibleQty int64
	HiddenQty  int64
}

type Subscription struct {
	ClientID uint64
	Symbol   string
	Types    []MDType
}

type Execution struct {
	TakerOrderID  uint64
	MakerOrderID  uint64
	TakerClientID uint64
	MakerClientID uint64
	Price         int64
	Qty           int64
	Timestamp     int64
}

type Fee struct {
	Asset  string
	Amount int64
}

type FundingRate struct {
	Symbol      string
	Rate        int64
	NextFunding int64
	Interval    int64
	MarkPrice   int64
	IndexPrice  int64
}

type OpenInterest struct {
	Symbol         string
	TotalContracts int64
	Timestamp      int64
}

type Position struct {
	ClientID   uint64
	Symbol     string
	Size       int64
	EntryPrice int64
	Margin     int64
}

type PriorityType uint8

const (
	PriorityPrice PriorityType = iota
	PriorityTime
	PrioritySize
	PriorityVisibility
	PriorityProRata
)

type Priority struct {
	Primary   PriorityType
	Secondary PriorityType
	Tertiary  PriorityType
}
