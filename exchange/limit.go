package exchange

func newLimit(price int64) *Limit {
	return &Limit{
		Price:    price,
		TotalQty: 0,
		OrderCnt: 0,
		Head:     nil,
		Tail:     nil,
		Prev:     nil,
		Next:     nil,
	}
}

func isEmpty(limit *Limit) bool {
	return limit.OrderCnt == 0
}

func visibleQty(limit *Limit) int64 {
	qty := int64(0)
	for o := limit.Head; o != nil; o = o.Next {
		if o.Visibility == Normal {
			qty += o.Qty - o.FilledQty
		} else if o.Visibility == Iceberg {
			qty += o.IcebergQty
		}
	}
	return qty
}
