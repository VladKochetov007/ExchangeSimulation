package book

import (
	"sync"

	etypes "exchange_sim/exchange/types"
)

// Limit pool — only used within this package.
var limitPool = sync.Pool{
	New: func() any { return &etypes.Limit{} },
}

func getLimit(price int64) *etypes.Limit {
	l := limitPool.Get().(*etypes.Limit)
	l.Price = price
	return l
}

// GetLimit retrieves a Limit from the pool with the given price.
func GetLimit(price int64) *etypes.Limit { return getLimit(price) }

func putLimit(l *etypes.Limit) {
	resetLimit(l)
	limitPool.Put(l)
}

func resetLimit(l *etypes.Limit) {
	l.Price = 0
	l.TotalQty = 0
	l.OrderCnt = 0
	l.Head = nil
	l.Tail = nil
	l.Prev = nil
	l.Next = nil
}

// LinkOrder appends order to the limit's queue.
func LinkOrder(limit *etypes.Limit, order *etypes.Order) {
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

// UnlinkOrder removes order from its limit's queue without deleting it from the book.
func UnlinkOrder(order *etypes.Order) {
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

// ResetOrder zeroes all fields of order for pool reuse.
func ResetOrder(order *etypes.Order) {
	order.ID = 0
	order.ClientID = 0
	order.Side = etypes.Buy
	order.Type = etypes.Market
	order.TimeInForce = etypes.GTC
	order.Price = 0
	order.Qty = 0
	order.FilledQty = 0
	order.Visibility = etypes.Normal
	order.IcebergQty = 0
	order.Status = etypes.Open
	order.Timestamp = 0
	order.Prev = nil
	order.Next = nil
	order.Parent = nil
}

// IsEmpty reports whether the limit has no resting orders.
func IsEmpty(limit *etypes.Limit) bool {
	return limit.OrderCnt == 0
}

// VisibleQty returns the total visible quantity at a limit level.
func VisibleQty(limit *etypes.Limit) int64 {
	var qty int64
	for o := limit.Head; o != nil; o = o.Next {
		remaining := o.Qty - o.FilledQty
		if o.Visibility == etypes.Normal {
			qty += remaining
		} else if o.Visibility == etypes.Iceberg {
			if remaining < o.IcebergQty {
				qty += remaining
			} else {
				qty += o.IcebergQty
			}
		}
	}
	return qty
}

// Book is a one-sided order book (all bids or all asks).
type Book struct {
	Side       etypes.Side
	Best       *etypes.Limit
	ActiveHead *etypes.Limit
	ActiveTail *etypes.Limit
	Orders     map[uint64]*etypes.Order
	Limits     map[int64]*etypes.Limit
}

// NewBook creates an empty one-sided book.
func NewBook(side etypes.Side) *Book {
	return &Book{
		Side:   side,
		Orders: make(map[uint64]*etypes.Order, 1024),
		Limits: make(map[int64]*etypes.Limit, 256),
	}
}

func (b *Book) AddOrder(order *etypes.Order) {
	limit := b.Limits[order.Price]
	if limit == nil {
		limit = getLimit(order.Price)
		b.Limits[order.Price] = limit
		b.insertLimit(limit)
		b.updateBest(limit)
	}
	LinkOrder(limit, order)
	b.Orders[order.ID] = order
}

func (b *Book) CancelOrder(orderID uint64) *etypes.Order {
	order := b.Orders[orderID]
	if order == nil {
		return nil
	}
	limit := order.Parent
	UnlinkOrder(order)
	delete(b.Orders, orderID)
	if IsEmpty(limit) {
		b.RemoveLimit(limit)
	}
	return order
}

func (b *Book) insertLimit(limit *etypes.Limit) {
	if b.ActiveHead == nil {
		b.ActiveHead = limit
		b.ActiveTail = limit
		return
	}

	if b.Side == etypes.Buy {
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

func (b *Book) RemoveLimit(limit *etypes.Limit) {
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

func (b *Book) updateBest(limit *etypes.Limit) {
	if b.Best == nil {
		b.Best = limit
		return
	}
	if b.Side == etypes.Buy {
		if limit.Price > b.Best.Price {
			b.Best = limit
		}
	} else {
		if limit.Price < b.Best.Price {
			b.Best = limit
		}
	}
}

// GetSnapshot returns up to 20 price levels for market data publishing.
func (b *Book) GetSnapshot() []etypes.PriceLevel {
	levels := make([]etypes.PriceLevel, 0, 20)
	for l := b.ActiveHead; l != nil && len(levels) < 20; l = l.Next {
		visible := VisibleQty(l)
		hidden := l.TotalQty - visible
		if visible > 0 || hidden > 0 {
			levels = append(levels, etypes.PriceLevel{
				Price:      l.Price,
				VisibleQty: visible,
				HiddenQty:  hidden,
			})
		}
	}
	return levels
}

