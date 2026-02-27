package types

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
	OrderID       uint64       `json:"order_id"`
	ClientID      uint64       `json:"client_id"`
	TradeID       uint64       `json:"trade_id"`
	Symbol        string       `json:"symbol"`
	Qty           int64        `json:"qty"`
	Price         int64        `json:"price"`
	Side          Side         `json:"side"`
	PositionSide  PositionSide `json:"position_side"`
	IsFull        bool         `json:"is_full"`
	FeeAmount     int64        `json:"fee_amount"`
	FeeAsset      string       `json:"fee_asset"`
	RealizedPnL   int64        `json:"realized_pnl"`
	NewSize       int64        `json:"new_size"`
	NewEntryPrice int64        `json:"new_entry_price"`
}

type OrderRequest struct {
	RequestID    uint64       `json:"request_id"`
	Side         Side         `json:"side"`
	Type         OrderType    `json:"type"`
	Price        int64        `json:"price"`
	Qty          int64        `json:"qty"`
	Symbol       string       `json:"symbol"`
	TimeInForce  TimeInForce  `json:"time_in_force"`
	Visibility   Visibility   `json:"visibility"`
	IcebergQty   int64        `json:"iceberg_qty"`
	PositionSide PositionSide `json:"position_side"`
}

type CancelRequest struct {
	RequestID uint64 `json:"request_id"`
	OrderID   uint64 `json:"order_id"`
}

type QueryRequest struct {
	RequestID uint64    `json:"request_id"`
	QueryType QueryType `json:"query_type"`
	Symbol    string    `json:"symbol"`
	Types     []MDType  `json:"types,omitempty"`
}
