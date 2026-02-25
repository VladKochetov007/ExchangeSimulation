package exchange

type Book struct {
	Side       Side
	Best       *Limit
	ActiveHead *Limit
	ActiveTail *Limit
	Orders     map[uint64]*Order
	Limits     map[int64]*Limit
}

func newBook(side Side) *Book {
	return &Book{
		Side:   side,
		Orders: make(map[uint64]*Order, 1024),
		Limits: make(map[int64]*Limit, 256),
	}
}

func (b *Book) addOrder(order *Order) {
	limit := b.Limits[order.Price]
	if limit == nil {
		limit = getLimit(order.Price)
		b.Limits[order.Price] = limit
		b.insertLimit(limit)
		b.updateBest(limit)
	}
	linkOrder(limit, order)
	b.Orders[order.ID] = order
}

func (b *Book) cancelOrder(orderID uint64) *Order {
	order := b.Orders[orderID]
	if order == nil {
		return nil
	}
	limit := order.Parent
	unlinkOrder(order)
	delete(b.Orders, orderID)
	if isEmpty(limit) {
		b.removeLimit(limit)
	}
	return order
}

func (b *Book) insertLimit(limit *Limit) {
	if b.ActiveHead == nil {
		b.ActiveHead = limit
		b.ActiveTail = limit
		return
	}

	if b.Side == Buy {
		for l := b.ActiveHead; l != nil; l = l.Next {
			if limit.Price > l.Price {
				limit.Next = l
				limit.Prev = l.Prev
				if l.Prev != nil {
					l.Prev.Next = limit
				} else {
					b.ActiveHead = limit
				}
				l.Prev = limit
				return
			}
		}
	} else {
		for l := b.ActiveHead; l != nil; l = l.Next {
			if limit.Price < l.Price {
				limit.Next = l
				limit.Prev = l.Prev
				if l.Prev != nil {
					l.Prev.Next = limit
				} else {
					b.ActiveHead = limit
				}
				l.Prev = limit
				return
			}
		}
	}

	b.ActiveTail.Next = limit
	limit.Prev = b.ActiveTail
	b.ActiveTail = limit
}

func (b *Book) removeLimit(limit *Limit) {
	if limit.Prev != nil {
		limit.Prev.Next = limit.Next
	} else {
		b.ActiveHead = limit.Next
	}
	if limit.Next != nil {
		limit.Next.Prev = limit.Prev
	} else {
		b.ActiveTail = limit.Prev
	}
	delete(b.Limits, limit.Price)
	if b.Best == limit {
		b.Best = b.ActiveHead
	}
	putLimit(limit)
}

func (b *Book) updateBest(limit *Limit) {
	if b.Best == nil {
		b.Best = limit
		return
	}
	if b.Side == Buy {
		if limit.Price > b.Best.Price {
			b.Best = limit
		}
	} else {
		if limit.Price < b.Best.Price {
			b.Best = limit
		}
	}
}

func (ob *OrderBook) findOrder(orderID uint64) *Order {
	if o := ob.Bids.Orders[orderID]; o != nil {
		return o
	}
	return ob.Asks.Orders[orderID]
}

func (b *Book) getSnapshot() []PriceLevel {
	levels := make([]PriceLevel, 0, 20)
	for l := b.ActiveHead; l != nil && len(levels) < 20; l = l.Next {
		visible := visibleQty(l)
		hidden := l.TotalQty - visible
		if visible > 0 || hidden > 0 {
			levels = append(levels, PriceLevel{
				Price:      l.Price,
				VisibleQty: visible,
				HiddenQty:  hidden,
			})
		}
	}
	return levels
}
