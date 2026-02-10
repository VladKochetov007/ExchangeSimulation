package exchange

type Side uint8

const (
	Buy Side = iota
	Sell
)

func (s Side) String() string {
	if s == Buy {
		return "BUY"
	}
	return "SELL"
}

func (s Side) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

type OrderType uint8

const (
	Market OrderType = iota
	LimitOrder
)

func (ot OrderType) String() string {
	if ot == Market {
		return "MARKET"
	}
	return "LIMIT"
}

func (ot OrderType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ot.String() + `"`), nil
}

type TimeInForce uint8

const (
	GTC TimeInForce = iota
	IOC
	FOK
)

func (tif TimeInForce) String() string {
	switch tif {
	case GTC:
		return "GTC"
	case IOC:
		return "IOC"
	case FOK:
		return "FOK"
	default:
		return "UNKNOWN"
	}
}

func (tif TimeInForce) MarshalJSON() ([]byte, error) {
	return []byte(`"` + tif.String() + `"`), nil
}

type Visibility uint8

const (
	Normal Visibility = iota
	Iceberg
	Hidden
)

func (v Visibility) String() string {
	switch v {
	case Normal:
		return "NORMAL"
	case Iceberg:
		return "ICEBERG"
	case Hidden:
		return "HIDDEN"
	default:
		return "UNKNOWN"
	}
}

func (v Visibility) MarshalJSON() ([]byte, error) {
	return []byte(`"` + v.String() + `"`), nil
}

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

func (rr RejectReason) String() string {
	switch rr {
	case RejectInsufficientBalance:
		return "INSUFFICIENT_BALANCE"
	case RejectInvalidPrice:
		return "INVALID_PRICE"
	case RejectInvalidQty:
		return "INVALID_QTY"
	case RejectUnknownClient:
		return "UNKNOWN_CLIENT"
	case RejectUnknownInstrument:
		return "UNKNOWN_INSTRUMENT"
	case RejectSelfTrade:
		return "SELF_TRADE"
	case RejectDuplicateOrderID:
		return "DUPLICATE_ORDER_ID"
	case RejectOrderNotFound:
		return "ORDER_NOT_FOUND"
	case RejectOrderNotOwned:
		return "ORDER_NOT_OWNED"
	case RejectOrderAlreadyFilled:
		return "ORDER_ALREADY_FILLED"
	case RejectFOKNotFilled:
		return "FOK_NOT_FILLED"
	default:
		return "UNKNOWN"
	}
}

func (rr RejectReason) MarshalJSON() ([]byte, error) {
	return []byte(`"` + rr.String() + `"`), nil
}

type Order struct {
	ID          uint64      `json:"order_id"`
	ClientID    uint64      `json:"client_id"`
	Side        Side        `json:"side"`
	Type        OrderType   `json:"type"`
	TimeInForce TimeInForce `json:"time_in_force"`
	Price       int64       `json:"price"`
	Qty         int64       `json:"qty"`
	FilledQty   int64       `json:"filled_qty"`
	Visibility  Visibility  `json:"visibility"`
	IcebergQty  int64       `json:"iceberg_qty"`
	Status      OrderStatus `json:"status"`
	Timestamp   int64       `json:"timestamp"`

	Prev   *Order `json:"-"`
	Next   *Order `json:"-"`
	Parent *Limit `json:"-"`
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
	RequestID uint64       `json:"request_id"`
	Success   bool         `json:"success"`
	Data      any          `json:"data,omitempty"`
	Error     RejectReason `json:"error"`
}

type FillNotification struct {
	OrderID   uint64 `json:"order_id"`
	ClientID  uint64 `json:"client_id"`
	TradeID   uint64 `json:"trade_id"`
	Qty       int64  `json:"qty"`
	Price     int64  `json:"price"`
	Side      Side   `json:"side"`
	IsFull    bool   `json:"is_full"`
	FeeAmount int64  `json:"fee_amount"`
	FeeAsset  string `json:"fee_asset"`
}

type OrderRequest struct {
	RequestID   uint64      `json:"request_id"`
	Side        Side        `json:"side"`
	Type        OrderType   `json:"type"`
	Price       int64       `json:"price"`
	Qty         int64       `json:"qty"`
	Symbol      string      `json:"symbol"`
	TimeInForce TimeInForce `json:"time_in_force"`
	Visibility  Visibility  `json:"visibility"`
	IcebergQty  int64       `json:"iceberg_qty"`
}

type CancelRequest struct {
	RequestID uint64 `json:"request_id"`
	OrderID   uint64 `json:"order_id"`
}

type QueryRequest struct {
	RequestID uint64    `json:"request_id"`
	QueryType QueryType `json:"query_type"`
	Symbol    string    `json:"symbol"`
}

type BalanceSnapshot struct {
	Timestamp int64            `json:"timestamp"`
	Balances  []AssetBalance   `json:"balances"`
}

type AssetBalance struct {
	Asset     string `json:"asset"`
	Total     int64  `json:"total"`
	Available int64  `json:"available"`
	Reserved  int64  `json:"reserved"`
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

type Execution struct {
	TakerOrderID  uint64 `json:"taker_order_id"`
	MakerOrderID  uint64 `json:"maker_order_id"`
	TakerClientID uint64 `json:"taker_client_id"`
	MakerClientID uint64 `json:"maker_client_id"`
	Price         int64  `json:"price"`
	Qty           int64  `json:"qty"`
	Timestamp     int64  `json:"timestamp"`
}

type Fee struct {
	Asset  string `json:"asset"`
	Amount int64  `json:"amount"`
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

type Position struct {
	ClientID   uint64 `json:"client_id"`
	Symbol     string `json:"symbol"`
	Size       int64  `json:"size"`
	EntryPrice int64  `json:"entry_price"`
	Margin     int64  `json:"margin"`
}

type MarginCallEvent struct {
	Timestamp        int64  `json:"timestamp"`
	ClientID         uint64 `json:"client_id"`
	Symbol           string `json:"symbol"`
	MarginRatioBps   int64  `json:"margin_ratio_bps"`
	LiquidationPrice int64  `json:"liquidation_price"`
}

type LiquidationEvent struct {
	Timestamp     int64  `json:"timestamp"`
	ClientID      uint64 `json:"client_id"`
	Symbol        string `json:"symbol"`
	PositionSize  int64  `json:"position_size"`
	FillPrice     int64  `json:"fill_price"`
	RemainingDebt int64  `json:"remaining_debt"`
}

type InsuranceFundEvent struct {
	Timestamp int64  `json:"timestamp"`
	Symbol    string `json:"symbol"`
	Delta     int64  `json:"delta"`
	Balance   int64  `json:"balance"`
}

type MarginInterestEvent struct {
	Timestamp int64  `json:"timestamp"`
	ClientID  uint64 `json:"client_id"`
	Asset     string `json:"asset"`
	Amount    int64  `json:"amount"`
}

type TransferEvent struct {
	Timestamp  int64  `json:"timestamp"`
	ClientID   uint64 `json:"client_id"`
	FromWallet string `json:"from_wallet"`
	ToWallet   string `json:"to_wallet"`
	Asset      string `json:"asset"`
	Amount     int64  `json:"amount"`
}

type BalanceChangeEvent struct {
	Timestamp int64          `json:"timestamp"`
	ClientID  uint64         `json:"client_id"`
	Symbol    string         `json:"symbol"`
	Reason    string         `json:"reason"`
	Changes   []BalanceDelta `json:"changes"`
}

type BalanceDelta struct {
	Asset      string `json:"asset"`
	Wallet     string `json:"wallet"`
	OldBalance int64  `json:"old_balance"`
	NewBalance int64  `json:"new_balance"`
	Delta      int64  `json:"delta"`
}

type BalanceSnapshotComplete struct {
	Timestamp    int64            `json:"timestamp"`
	ClientID     uint64           `json:"client_id"`
	SpotBalances []AssetBalance   `json:"spot_balances"`
	PerpBalances []AssetBalance   `json:"perp_balances"`
	Borrowed     map[string]int64 `json:"borrowed"`
}

type BorrowEvent struct {
	Timestamp      int64  `json:"timestamp"`
	ClientID       uint64 `json:"client_id"`
	Asset          string `json:"asset"`
	Amount         int64  `json:"amount"`
	Reason         string `json:"reason"`
	MarginMode     string `json:"margin_mode"`
	InterestRate   int64  `json:"interest_rate_bps"`
	CollateralUsed int64  `json:"collateral_used"`
}

type RepayEvent struct {
	Timestamp     int64  `json:"timestamp"`
	ClientID      uint64 `json:"client_id"`
	Asset         string `json:"asset"`
	Principal     int64  `json:"principal"`
	Interest      int64  `json:"interest"`
	RemainingDebt int64  `json:"remaining_debt"`
}

type MarginMode int

const (
	CrossMargin    MarginMode = 0
	IsolatedMargin MarginMode = 1
)

func (m MarginMode) String() string {
	switch m {
	case CrossMargin:
		return "cross"
	case IsolatedMargin:
		return "isolated"
	default:
		return "unknown"
	}
}

type BorrowingConfig struct {
	Enabled           bool
	AutoBorrowSpot    bool
	AutoBorrowPerp    bool
	DefaultMarginMode MarginMode

	BorrowRates       map[string]int64
	CollateralFactors map[string]float64
	MaxBorrowPerAsset map[string]int64

	PriceOracle CollateralPriceOracle
}

type CollateralPriceOracle interface {
	GetPrice(asset string) int64
}

type IsolatedPosition struct {
	Symbol     string
	Collateral map[string]int64
	Borrowed   map[string]int64
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
