package multivenue

import (
	"context"
	"math"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// LatentLiquidityConfig describes a population of unexpressed trading
// intentions.
//
// The square-root impact law is attributed to liquidity that is mostly latent:
// participants hold reservation prices they have not posted, those prices
// diffuse, and an intention only becomes an order when its reservation price
// crosses the market. The resulting latent density vanishes linearly at the
// transaction price, which is what makes impact concave in size — a large
// order eats into a region where there is progressively less to trade against.
//
// Impact measured without such a population is mechanical instead: a parent
// order depletes a displayed quote of fixed size and the price moves in
// proportion, which is why this simulator produced a superlinear exponent.
type LatentLiquidityConfig struct {
	Symbol        string        `json:"symbol"`
	BasePrecision int64         `json:"base_precision"`
	TickSize      int64         `json:"tick_size"`
	Interval      time.Duration `json:"interval"`

	// DepositsPerTick is how many new intentions appear each interval, and
	// CancelProbability how likely an existing one is abandoned. Their ratio
	// sets how much latent volume accumulates.
	DepositsPerTick   int     `json:"deposits_per_tick"`
	CancelProbability float64 `json:"cancel_probability"`
	// DiffusionBps is the standard deviation of one step of a reservation
	// price, in basis points of the current price.
	DiffusionBps float64 `json:"diffusion_bps"`
	// SpreadBps is the half-width of the band in which new reservation prices
	// are placed around the current price.
	SpreadBps float64 `json:"spread_bps"`

	// ConversionsPerTick bounds how many crossed intentions become orders in
	// one interval. Converting a single one throttles the population to a
	// trickle: the supply the square-root derivation relies on is the whole
	// crossed region reacting, not one participant per tick.
	ConversionsPerTick int `json:"conversions_per_tick"`
	// RevealBps is the distance from the midpoint within which an intention
	// posts a resting order at its reservation price.
	//
	// This is the difference between latent demand and latent liquidity. An
	// intention that waits until its reservation price has crossed the market
	// arrives as demand and consumes the book. An intention revealed while it
	// is still near but not through the price becomes the supply a large order
	// trades into, which is the density the square-root derivation integrates
	// over. Zero keeps the crossing behaviour.
	RevealBps float64 `json:"reveal_bps"`
	// RevealsPerTick bounds how many orders are posted or pulled per interval.
	RevealsPerTick int `json:"reveals_per_tick"`
	// PostAsLimit makes a converted intention rest at its reservation price
	// instead of crossing the spread. This is the distinction between latent
	// demand and latent liquidity: a crossing intention consumes the book,
	// whereas a resting one materialises as the supply a large order trades
	// into, which is what the square-root derivation describes.
	PostAsLimit   bool  `json:"post_as_limit"`
	IntentionQty  int64 `json:"intention_qty"`
	MaxIntentions int   `json:"max_intentions"`
	Seed          int64 `json:"seed"`
}

type latentIntention struct {
	buy         bool
	reservation float64
	// orderID is non-zero once the intention is resting in the book.
	orderID   uint64
	requestID uint64
}

// LatentLiquidity converts diffusing reservation prices into marketable orders.
type LatentLiquidity struct {
	*actor.BaseActor
	cfg        LatentLiquidityConfig
	rng        *rand.Rand
	intentions []latentIntention

	bid, ask       int64
	bidQty, askQty int64
	pending        bool
	subscribed     bool
	converted      int
}

func NewLatentLiquidity(id uint64, gw actor.Gateway, cfg LatentLiquidityConfig) *LatentLiquidity {
	l := &LatentLiquidity{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
	}
	l.SetHandler(l)
	l.AddTicker(cfg.Interval, l.onTick)
	return l
}

// Intentions is the current latent population size, and Converted the number
// of intentions that have become orders.
func (l *LatentLiquidity) Intentions() int { return len(l.intentions) }
func (l *LatentLiquidity) Converted() int  { return l.converted }

func (l *LatentLiquidity) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != l.cfg.Symbol || e.Snapshot == nil {
			return
		}
		l.bid, l.bidQty, l.ask, l.askQty = 0, 0, 0, 0
		if len(e.Snapshot.Bids) > 0 {
			l.bid, l.bidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
		}
		if len(e.Snapshot.Asks) > 0 {
			l.ask, l.askQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
		}
	case actor.EventOrderAccepted:
		e := evt.Data.(actor.OrderAcceptedEvent)
		for index := range l.intentions {
			if l.intentions[index].requestID == e.RequestID {
				l.intentions[index].orderID = e.OrderID
				return
			}
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.IsFull {
			l.pending = false
			// A filled intention has done what it wanted and leaves the
			// population; the deposit process replaces it.
			l.dropByOrderID(e.OrderID)
		}
	case actor.EventOrderCancelled:
		l.pending = false
		e := evt.Data.(actor.OrderCancelledEvent)
		for index := range l.intentions {
			if l.intentions[index].orderID == e.OrderID {
				l.intentions[index].orderID, l.intentions[index].requestID = 0, 0
				return
			}
		}
	case actor.EventOrderRejected:
		l.pending = false
		e := evt.Data.(actor.OrderRejectedEvent)
		for index := range l.intentions {
			if l.intentions[index].requestID == e.RequestID {
				l.intentions[index].requestID = 0
				return
			}
		}
	}
}

func (l *LatentLiquidity) onTick(time.Time) {
	if !l.subscribed {
		l.Subscribe(l.cfg.Symbol, exchange.MDSnapshot)
		l.subscribed = true
		return
	}
	if l.bid <= 0 || l.ask <= 0 {
		return
	}
	mid := float64(l.bid+l.ask) / 2
	l.evolve(mid)
	l.deposit(mid)
	if l.cfg.RevealBps > 0 {
		l.reveal(mid)
		return
	}
	l.convert()
}

// dropByOrderID removes an intention whose resting order has been filled.
func (l *LatentLiquidity) dropByOrderID(orderID uint64) {
	for index := range l.intentions {
		if l.intentions[index].orderID == orderID {
			l.intentions = append(l.intentions[:index], l.intentions[index+1:]...)
			l.converted++
			return
		}
	}
}

// reveal posts resting orders for intentions near the price and pulls them
// when their reservation price drifts away, so the visible book tracks the
// latent density around the midpoint.
func (l *LatentLiquidity) reveal(mid float64) {
	band := mid * l.cfg.RevealBps / 10_000
	budget := l.cfg.RevealsPerTick
	if budget <= 0 {
		budget = 10
	}
	for index := range l.intentions {
		if budget <= 0 {
			return
		}
		intention := &l.intentions[index]
		near := math.Abs(intention.reservation-mid) <= band
		switch {
		case near && intention.orderID == 0 && intention.requestID == 0:
			if l.post(intention) {
				budget--
			}
		case !near && intention.orderID != 0:
			l.CancelOrder(intention.orderID)
			intention.orderID, intention.requestID = 0, 0
			budget--
		}
	}
}

// post rests one intention at its reservation price, on the side that does not
// cross the market.
func (l *LatentLiquidity) post(intention *latentIntention) bool {
	price := int64(intention.reservation)
	tick := l.cfg.TickSize
	side := exchange.Buy
	if !intention.buy {
		side = exchange.Sell
	}
	if tick > 0 {
		if side == exchange.Buy {
			price = price / tick * tick
		} else {
			price = (price + tick - 1) / tick * tick
		}
	}
	// Never cross: a revealed intention is liquidity, not demand.
	if price <= 0 || (side == exchange.Buy && price >= l.ask) || (side == exchange.Sell && price <= l.bid) {
		return false
	}
	intention.requestID = l.SubmitOrder(l.cfg.Symbol, side, exchange.LimitOrder, price, l.cfg.IntentionQty)
	return true
}

// evolve diffuses every reservation price and abandons some intentions. The
// diffusion is what refills the region near the price after it is consumed.
func (l *LatentLiquidity) evolve(mid float64) {
	step := mid * l.cfg.DiffusionBps / 10_000
	kept := l.intentions[:0]
	for _, intention := range l.intentions {
		if l.rng.Float64() < l.cfg.CancelProbability {
			continue
		}
		intention.reservation += l.rng.NormFloat64() * step
		if !finite(intention.reservation) || intention.reservation <= 0 {
			continue
		}
		kept = append(kept, intention)
	}
	l.intentions = kept
}

// deposit places new intentions on both sides, away from the current price.
func (l *LatentLiquidity) deposit(mid float64) {
	width := mid * l.cfg.SpreadBps / 10_000
	for i := 0; i < l.cfg.DepositsPerTick && len(l.intentions) < l.cfg.MaxIntentions; i++ {
		buy := l.rng.Intn(2) == 0
		offset := l.rng.Float64() * width
		reservation := mid - offset
		if !buy {
			reservation = mid + offset
		}
		if reservation > 0 {
			l.intentions = append(l.intentions, latentIntention{buy: buy, reservation: reservation})
		}
	}
}

// convert turns the intentions whose reservation price has crossed the market
// into marketable orders, up to the configured number per interval.
func (l *LatentLiquidity) convert() {
	budget := l.cfg.ConversionsPerTick
	if budget <= 0 {
		budget = 1
	}
	kept := l.intentions[:0]
	for _, intention := range l.intentions {
		crossedBuy := intention.buy && intention.reservation >= float64(l.ask)
		crossedSell := !intention.buy && intention.reservation <= float64(l.bid)
		if budget <= 0 || (!crossedBuy && !crossedSell) {
			kept = append(kept, intention)
			continue
		}
		side, price, available := exchange.Buy, l.ask, l.askQty
		if crossedSell {
			side, price, available = exchange.Sell, l.bid, l.bidQty
		}
		quantity := l.cfg.IntentionQty
		if l.cfg.PostAsLimit {
			// Rest at the reservation price rather than taking: the intention
			// becomes visible supply instead of consuming it.
			price = int64(intention.reservation)
			available = 0
		}
		if available > 0 && quantity > available {
			quantity = available
		}
		if quantity <= 0 || price <= 0 {
			// The intention is abandoned rather than retried: it wanted to
			// trade and there was nothing to trade against.
			continue
		}
		if tick := l.cfg.TickSize; tick > 0 {
			if side == exchange.Buy {
				price = (price + tick - 1) / tick * tick
			} else {
				price = price / tick * tick
			}
		}
		timeInForce := exchange.IOC
		if l.cfg.PostAsLimit {
			timeInForce = exchange.GTC
		}
		l.SubmitOrderWithTimeInForce(l.cfg.Symbol, side, exchange.LimitOrder, price, quantity, timeInForce)
		l.converted++
		budget--
	}
	l.intentions = kept
}
