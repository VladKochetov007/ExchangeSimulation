package derivsim

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// OptionTakerConfig drives random option flow. PBuy is the gamma-sign
// control for the hedging-feedback experiment: PBuy near 1 loads the dealer
// with short options (dealer short gamma, amplifying hedger), PBuy near 0
// makes the dealer net long gamma (damping hedger).
type OptionTakerConfig struct {
	Underlying string
	PBuy       float64
	LotQty     int64
	Interval   time.Duration
	Seed       int64
	// IncludeFutures adds dated futures to the taking universe, giving the
	// futures books flow of their own (needed for untethered basis dynamics).
	IncludeFutures bool
	// ContractTypes restricts the universe to these instrument types, which is
	// how a participant can be dedicated to one kind of contract. Empty keeps
	// options, plus futures when IncludeFutures is set.
	//
	// It matters because a taker drawing uniformly from a mixed universe gives
	// each book flow in proportion to how many contracts of its kind are
	// listed: with thirty option strikes beside two dated futures, the futures
	// see a twentieth of the flow and trade about once an hour.
	ContractTypes []string
}

// OptionTaker lifts a random live option quote each interval.
type OptionTaker struct {
	*actor.BaseActor
	cfg        OptionTakerConfig
	set        *contractSet
	rng        *rand.Rand
	subscribed bool
}

func NewOptionTaker(id uint64, gw actor.Gateway, cfg OptionTakerConfig) *OptionTaker {
	t := &OptionTaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		rng:       rand.New(rand.NewSource(cfg.Seed)),
	}
	t.SetHandler(t)
	t.AddTicker(cfg.Interval, t.onTick)
	return t
}

func (t *OptionTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	t.set.handle(evt)
}

// tradesType reports whether a contract kind is in this taker's universe.
func (t *OptionTaker) tradesType(contractType string) bool {
	if len(t.cfg.ContractTypes) > 0 {
		for _, allowed := range t.cfg.ContractTypes {
			if allowed == contractType {
				return true
			}
		}
		return false
	}
	return contractType == "OPTION" || (t.cfg.IncludeFutures && contractType == "FUTURE")
}

func (t *OptionTaker) onTick(_ time.Time) {
	if !t.subscribed {
		t.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		t.subscribed = true
		return
	}
	universe := make([]string, 0, len(t.set.contracts))
	for _, c := range t.set.orderedContracts() {
		if t.tradesType(c.Type) {
			universe = append(universe, c.Symbol)
		}
	}
	if len(universe) == 0 {
		return
	}
	sym := universe[t.rng.Intn(len(universe))]
	side := exchange.Sell
	if t.rng.Float64() < t.cfg.PBuy {
		side = exchange.Buy
	}
	reqID := t.SubmitOrder(sym, side, exchange.Market, 0, t.cfg.LotQty)
	t.set.trackRequest(reqID, sym)
}
