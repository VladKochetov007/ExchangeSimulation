package types

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
