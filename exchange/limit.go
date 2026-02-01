package exchange

func isEmpty(limit *Limit) bool {
	return limit.OrderCnt == 0
}

func visibleQty(limit *Limit) int64 {
	var qty int64
	for o := limit.Head; o != nil; o = o.Next {
		if o.Visibility == Normal {
			qty += o.Qty - o.FilledQty
		} else if o.Visibility == Iceberg {
			qty += o.IcebergQty
		}
	}
	return qty
}
