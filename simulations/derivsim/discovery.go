package derivsim

import (
	"cmp"
	"slices"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// Contract is a live derivative discovered from the instrument feed.
type Contract struct {
	Symbol     string
	Type       string // "FUTURE" | "OPTION"
	Underlying string
	Strike     int64
	IsCall     bool
	ExpiryNano int64
	// ListedNano is the timestamp on the live reference-data announcement.
	// It is not reconstructed from ExpiryNano: rolling option chains can list
	// several short-tenor generations during one simulation.
	ListedNano int64
}

// contractSet tracks live contracts announced on the reference-data feed and
// resolves orderID -> symbol so fills can be attributed per contract even
// though fill events carry no symbol. Fills that arrive before their accept
// (marketable orders) are buffered and replayed once the accept lands.
type contractSet struct {
	contracts map[string]*Contract
	reqToSym  map[uint64]string
	orderSym  map[uint64]string
	earlyFill map[uint64][]actor.OrderFillEvent
	underly   string

	onList   func(c *Contract)
	onSettle func(c *Contract, settlementPrice int64)
	onFill   func(symbol string, e actor.OrderFillEvent)
	onAccept func(symbol string, reqID, orderID uint64)
	onReject func(reqID uint64)
}

func newContractSet(underlying string) *contractSet {
	return &contractSet{
		contracts: make(map[string]*Contract),
		reqToSym:  make(map[uint64]string),
		orderSym:  make(map[uint64]string),
		earlyFill: make(map[uint64][]actor.OrderFillEvent),
		underly:   underlying,
	}
}

func (cs *contractSet) trackRequest(reqID uint64, symbol string) {
	cs.reqToSym[reqID] = symbol
}

func (cs *contractSet) orderedContracts() []*Contract {
	contracts := make([]*Contract, 0, len(cs.contracts))
	for _, contract := range cs.contracts {
		contracts = append(contracts, contract)
	}
	slices.SortFunc(contracts, func(a, b *Contract) int {
		return cmp.Compare(a.Symbol, b.Symbol)
	})
	return contracts
}

// handle processes discovery/lifecycle/fill events; returns true if consumed.
func (cs *contractSet) handle(evt *actor.Event) bool {
	switch evt.Type {
	case actor.EventInstrument:
		e := evt.Data.(actor.InstrumentEvent)
		ann := e.Announcement
		if ann.Underlying != cs.underly {
			return true
		}
		switch ann.Action {
		case "listed":
			c := &Contract{
				Symbol: ann.Symbol, Type: ann.InstrumentType, Underlying: ann.Underlying,
				Strike: ann.Strike, IsCall: ann.IsCall, ExpiryNano: ann.ExpiryNano, ListedNano: e.Timestamp,
			}
			cs.contracts[ann.Symbol] = c
			if cs.onList != nil {
				cs.onList(c)
			}
		case "settled":
			if c, ok := cs.contracts[ann.Symbol]; ok {
				delete(cs.contracts, ann.Symbol)
				if cs.onSettle != nil {
					cs.onSettle(c, ann.SettlementPrice)
				}
			}
		}
		return true

	case actor.EventOrderAccepted:
		e := evt.Data.(actor.OrderAcceptedEvent)
		sym, ok := cs.reqToSym[e.RequestID]
		if !ok {
			return false
		}
		delete(cs.reqToSym, e.RequestID)
		cs.orderSym[e.OrderID] = sym
		if cs.onAccept != nil {
			cs.onAccept(sym, e.RequestID, e.OrderID)
		}
		for _, fill := range cs.earlyFill[e.OrderID] {
			cs.dispatchFill(sym, fill)
		}
		delete(cs.earlyFill, e.OrderID)
		return true

	case actor.EventOrderRejected:
		e := evt.Data.(actor.OrderRejectedEvent)
		if _, ok := cs.reqToSym[e.RequestID]; !ok {
			return false
		}
		delete(cs.reqToSym, e.RequestID)
		if cs.onReject != nil {
			cs.onReject(e.RequestID)
		}
		return true

	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		sym, ok := cs.orderSym[e.OrderID]
		if !ok {
			// Marketable order: fill delivered before its accept.
			cs.earlyFill[e.OrderID] = append(cs.earlyFill[e.OrderID], e)
			return true
		}
		cs.dispatchFill(sym, e)
		return true

	case actor.EventOrderCancelled:
		e := evt.Data.(actor.OrderCancelledEvent)
		delete(cs.orderSym, e.OrderID)
		return true
	}
	return false
}

func (cs *contractSet) dispatchFill(sym string, e actor.OrderFillEvent) {
	if e.IsFull {
		defer delete(cs.orderSym, e.OrderID)
	}
	if cs.onFill != nil {
		cs.onFill(sym, e)
	}
}

// signedQty converts a fill to a signed base-quantity delta.
func signedQty(e actor.OrderFillEvent) int64 {
	if e.Side == exchange.Buy {
		return e.Qty
	}
	return -e.Qty
}
