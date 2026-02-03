package exchange

func isEmpty(limit *Limit) bool {
	return limit.OrderCnt == 0
}

func visibleQty(limit *Limit) int64 {
	var qty int64
	for o := limit.Head; o != nil; o = o.Next {
		remaining := o.Qty - o.FilledQty
		if o.Visibility == Normal {
			qty += remaining
		} else if o.Visibility == Iceberg {
			if remaining < o.IcebergQty {
				qty += remaining
			} else {
				qty += o.IcebergQty
			}
		}
	}
	return qty
}
