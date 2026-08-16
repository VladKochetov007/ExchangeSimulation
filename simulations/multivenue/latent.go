package multivenue

import (
	"context"
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
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		if evt.Data.(actor.OrderFillEvent).IsFull {
			l.pending = false
		}
	case actor.EventOrderCancelled, actor.EventOrderRejected:
		l.pending = false
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
	l.convert()
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
