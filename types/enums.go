package types

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

// RejectReason describes why a request was refused.
type RejectReason = string

const (
	RejectInsufficientBalance RejectReason = "INSUFFICIENT_BALANCE"
	RejectInvalidPrice        RejectReason = "INVALID_PRICE"
	RejectInvalidQty          RejectReason = "INVALID_QTY"
	RejectUnknownClient       RejectReason = "UNKNOWN_CLIENT"
	RejectUnknownInstrument   RejectReason = "UNKNOWN_INSTRUMENT"
	RejectSelfTrade           RejectReason = "SELF_TRADE"
	RejectDuplicateOrderID    RejectReason = "DUPLICATE_ORDER_ID"
	RejectOrderNotFound       RejectReason = "ORDER_NOT_FOUND"
	RejectOrderNotOwned       RejectReason = "ORDER_NOT_OWNED"
	RejectOrderAlreadyFilled  RejectReason = "ORDER_ALREADY_FILLED"
	RejectFOKNotFilled        RejectReason = "FOK_NOT_FILLED"
	RejectUnknownRequest      RejectReason = "UNKNOWN_REQUEST"
)

// RequestType identifies a request sent through a Gateway.
type RequestType = string

const (
	ReqPlaceOrder   RequestType = "place_order"
	ReqCancelOrder  RequestType = "cancel_order"
	ReqQueryBalance RequestType = "query_balance"
	ReqQueryOrders  RequestType = "query_orders"
	ReqQueryAccount RequestType = "query_account"
	ReqSubscribe    RequestType = "subscribe"
	ReqUnsubscribe  RequestType = "unsubscribe"
)

type PositionSide uint8

const (
	PositionBoth  PositionSide = iota // one-way mode (netting)
	PositionLong                       // hedge mode long
	PositionShort                      // hedge mode short
)

func (ps PositionSide) String() string {
	switch ps {
	case PositionLong:
		return "LONG"
	case PositionShort:
		return "SHORT"
	default:
		return "BOTH"
	}
}

type QueryType uint8

const (
	QueryBalance QueryType = iota
	QueryOrders
	QueryOrder
)

type MDType uint8

const (
	MDSnapshot MDType = iota
	MDDelta
	MDTrade
	MDFunding
	MDOpenInterest
)

type MarginMode uint8

const (
	CrossMargin    MarginMode = iota
	IsolatedMargin
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
