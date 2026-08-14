package book

import (
	"math"
	"sync"

	etypes "exchange_sim/types"
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

// LinkOrder appends order to the limit's queue. It returns false without
// changing either object if the remaining quantity cannot be represented in
// the level aggregate. Callers must handle a false result as an order reject;
// saturating TotalQty would make later cancels and fills corrupt the book.
func LinkOrder(limit *etypes.Limit, order *etypes.Order) bool {
	if limit == nil || order == nil || order.Parent != nil || order.Prev != nil || order.Next != nil {
		return false
	}
	if (limit.Head == nil) != (limit.Tail == nil) || limit.TotalQty < 0 || limit.OrderCnt < 0 || limit.OrderCnt == math.MaxInt32 {
		return false
	}
	remaining, ok := etypes.TrySub(order.Qty, order.FilledQty)
	if !ok || remaining <= 0 {
		return false
	}
	if _, ok := etypes.TryAdd(limit.TotalQty, remaining); !ok {
		return false
	}

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
	limit.TotalQty += remaining
	limit.OrderCnt++
	return true
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
	order.PositionSide = etypes.PositionBoth
	order.Type = etypes.Market
	order.TimeInForce = etypes.GTC
	order.Price = 0
	order.Qty = 0
	order.FilledQty = 0
	order.Visibility = etypes.Normal
	order.IcebergQty = 0
	order.DisplayRemaining = 0
	order.Status = etypes.Open
	order.Timestamp = 0
	order.Reserved = 0
	order.FeeReserved = nil
	order.Prev = nil
	order.Next = nil
	order.Parent = nil
}

// IsEmpty reports whether the limit has no resting orders.
func IsEmpty(limit *etypes.Limit) bool {
	return limit.OrderCnt == 0
}

// VisibleQty returns the total visible quantity at a limit level. For
// icebergs the live display tranche (DisplayRemaining) is authoritative;
// orders injected without one fall back to min(remaining, IcebergQty).
func VisibleQty(limit *etypes.Limit) int64 {
	var qty int64
	for o := limit.Head; o != nil; o = o.Next {
		remaining, ok := etypes.TrySub(o.Qty, o.FilledQty)
		if !ok || remaining <= 0 {
			continue
		}
		visible := int64(0)
		if o.Visibility == etypes.Normal {
			visible = remaining
		} else if o.Visibility == etypes.Iceberg {
			display := o.DisplayRemaining
			if display == 0 {
				display = o.IcebergQty
			}
			if display > 0 {
				visible = min(remaining, display)
			}
		}
		if visible == 0 {
			continue
		}
		var added bool
		qty, added = etypes.TryAdd(qty, visible)
		if !added {
			// The exact public depth is no longer representable. Saturation is
			// conservative and, unlike integer wraparound, cannot advertise
			// negative liquidity to clients.
			return math.MaxInt64
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

func (b *Book) AddOrder(order *etypes.Order) bool {
	if b == nil || order == nil {
		return false
	}
	if _, exists := b.Orders[order.ID]; exists {
		return false
	}
	limit := b.Limits[order.Price]
	if limit == nil {
		limit = getLimit(order.Price)
		if !LinkOrder(limit, order) {
			putLimit(limit)
			return false
		}
		b.Limits[order.Price] = limit
		b.insertLimit(limit)
		b.updateBest(limit)
	} else if !LinkOrder(limit, order) {
		return false
	}
	b.Orders[order.ID] = order
	return true
}

func (b *Book) CancelOrder(orderID uint64) *etypes.Order {
	order := b.Orders[orderID]
	if order == nil {
		return nil
	}
	limit := order.Parent
	if limit == nil {
		// Fully filled and already unlinked by the matcher; only the ID index remains.
		delete(b.Orders, orderID)
		return order
	}
	UnlinkOrder(order)
	delete(b.Orders, orderID)
	if IsEmpty(limit) {
		b.RemoveLimit(limit)
	}
	return order
}

// RemoveFilledOrder deletes a fully filled order from the ID index after
// settlement. The matcher unlinks filled orders from their price level but
// leaves them in Orders so settlement can read the reservation ledger.
func (b *Book) RemoveFilledOrder(orderID uint64) *etypes.Order {
	return b.CancelOrder(orderID)
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

// GetSnapshot returns up to 20 price levels including hidden depth.
// This is the god view for loggers and internal tooling — public market data
// must use GetPublicSnapshot so dark liquidity stays dark.
func (b *Book) GetSnapshot() []etypes.PriceLevel {
	levels := make([]etypes.PriceLevel, 0, 20)
	for l := b.ActiveHead; l != nil && len(levels) < 20; l = l.Next {
		visible := VisibleQty(l)
		hidden, ok := etypes.TrySub(l.TotalQty, visible)
		if !ok || hidden < 0 {
			hidden = 0
		}
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

// GetPublicSnapshot returns up to 20 displayed price levels. Hidden orders and
// the reserve portion of icebergs are excluded, matching what a real venue
// broadcasts to subscribers.
func (b *Book) GetPublicSnapshot() []etypes.PriceLevel {
	levels := make([]etypes.PriceLevel, 0, 20)
	for l := b.ActiveHead; l != nil && len(levels) < 20; l = l.Next {
		if visible := VisibleQty(l); visible > 0 {
			levels = append(levels, etypes.PriceLevel{
				Price:      l.Price,
				VisibleQty: visible,
			})
		}
	}
	return levels
}
