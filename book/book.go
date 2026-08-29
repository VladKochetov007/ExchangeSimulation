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
	initialDisplay := order.DisplayRemaining
	if order.Visibility == etypes.Iceberg && initialDisplay == 0 {
		if order.IcebergQty <= 0 {
			return false
		}
		initialDisplay = min(order.IcebergQty, remaining)
	}

	order.DisplayRemaining = initialDisplay
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
	order.PostOnly = false
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
// icebergs the live display tranche (DisplayRemaining) is authoritative.
// LinkOrder initializes that transient field exactly once on insertion.
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
	// byClient indexes resting orders by owner. Admission answers three
	// questions per order placement that each concern one client's own resting
	// orders — exposure, hedge-reduce, and self-crossing quotes — and each
	// previously scanned the whole side to find them, which is O(book) work to
	// examine a handful of orders. Profiling put two of those scans at 3.48%
	// and 1.66% of simulator CPU.
	//
	// It is nil on preview clones. A clone is built order by order and thrown
	// away without being queried, so maintaining an index for it would add
	// allocation to the largest allocation site in the simulator to serve
	// nobody. OrdersForClient returns nil when the index is absent, which is
	// how callers know to fall back to the full scan.
	byClient map[uint64]map[uint64]*etypes.Order
}

// NewBook creates an empty one-sided book.
func NewBook(side etypes.Side) *Book {
	return NewBookWithCapacity(side, 1024, 256)
}

// NewBookWithCapacity creates an empty one-sided book with capacity hints for
// its order and price-level indexes. The hints affect allocation only: queue
// ordering, matching, and all public book state are identical to NewBook.
//
// Preview books use the live book's current sizes instead of the deliberately
// generous capacities used for long-lived venue books. A preview is detached
// and short-lived, so reserving 1,024 order and 256 level slots for every
// preflight clone otherwise dominates allocation and GC work.
func NewBookWithCapacity(side etypes.Side, orderCapacity, limitCapacity int) *Book {
	return newBookWithOwnerIndex(side, orderCapacity, limitCapacity, true)
}

// NewDetachedBook creates a book for short-lived preview matching. It behaves
// identically to NewBookWithCapacity for every queue, matching and public state
// question, and differs only in not maintaining the owner index that admission
// checks use on live venue books.
func NewDetachedBook(side etypes.Side, orderCapacity, limitCapacity int) *Book {
	return newBookWithOwnerIndex(side, orderCapacity, limitCapacity, false)
}

func newBookWithOwnerIndex(side etypes.Side, orderCapacity, limitCapacity int, trackOwners bool) *Book {
	book := &Book{
		Side:   side,
		Orders: make(map[uint64]*etypes.Order, orderCapacity),
		Limits: make(map[int64]*etypes.Limit, limitCapacity),
	}
	if trackOwners {
		book.byClient = make(map[uint64]map[uint64]*etypes.Order)
	}
	return book
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
	if b.byClient != nil {
		owned := b.byClient[order.ClientID]
		if owned == nil {
			owned = make(map[uint64]*etypes.Order)
			b.byClient[order.ClientID] = owned
		}
		owned[order.ID] = order
	}
	return true
}

// OrdersForClient returns the client's resting orders on this side, or nil when
// this book does not maintain the owner index and the caller must scan Orders.
//
// The returned map is owned by the book: read it, do not retain or mutate it.
func (b *Book) OrdersForClient(clientID uint64) map[uint64]*etypes.Order {
	if b == nil || b.byClient == nil {
		return nil
	}
	return b.byClient[clientID]
}

// TracksOwners reports whether OrdersForClient is authoritative for this book.
func (b *Book) TracksOwners() bool { return b != nil && b.byClient != nil }

// forgetOwner removes an order from the owner index.
func (b *Book) forgetOwner(order *etypes.Order) {
	if b.byClient == nil {
		return
	}
	owned := b.byClient[order.ClientID]
	if owned == nil {
		return
	}
	delete(owned, order.ID)
	if len(owned) == 0 {
		delete(b.byClient, order.ClientID)
	}
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
		b.forgetOwner(order)
		return order
	}
	UnlinkOrder(order)
	delete(b.Orders, orderID)
	b.forgetOwner(order)
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
