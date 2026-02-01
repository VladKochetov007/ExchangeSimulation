package exchange

func linkOrder(limit *Limit, order *Order) {
	order.Parent = limit
	if limit.Head == nil {
		limit.Head = order
		limit.Tail = order
		order.Prev = nil
		order.Next = nil
	} else {
		limit.Tail.Next = order
		order.Prev = limit.Tail
		order.Next = nil
		limit.Tail = order
	}
	limit.TotalQty += order.Qty - order.FilledQty
	limit.OrderCnt++
}

func unlinkOrder(order *Order) {
	limit := order.Parent
	if order.Prev != nil {
		order.Prev.Next = order.Next
	} else {
		limit.Head = order.Next
	}
	if order.Next != nil {
		order.Next.Prev = order.Prev
	} else {
		limit.Tail = order.Prev
	}
	limit.TotalQty -= order.Qty - order.FilledQty
	limit.OrderCnt--
	order.Prev = nil
	order.Next = nil
	order.Parent = nil
}

func resetOrder(order *Order) {
	order.ID = 0
	order.ClientID = 0
	order.Side = Buy
	order.Type = Market
	order.TimeInForce = GTC
	order.Price = 0
	order.Qty = 0
	order.FilledQty = 0
	order.Visibility = Normal
	order.IcebergQty = 0
	order.Status = Open
	order.Timestamp = 0
	order.Prev = nil
	order.Next = nil
	order.Parent = nil
}

func resetLimit(limit *Limit) {
	limit.Price = 0
	limit.TotalQty = 0
	limit.OrderCnt = 0
	limit.Head = nil
	limit.Tail = nil
	limit.Prev = nil
	limit.Next = nil
}
