package actors

import (
	"log"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type InternalFundingArbConfig struct {
	ActorID         uint64
	SpotSymbol      string
	PerpSymbol      string
	SpotInstrument  exchange.Instrument
	PerpInstrument  exchange.Instrument
	
	// Funding Rate Trigger (bps)
	MinFundingRate  int64
	ExitFundingRate int64
	
	// Basis Trigger (bps) - (PerpPrice - SpotPrice) / SpotPrice
	BasisThresholdBps     int64 
	ExitBasisThresholdBps int64
	
	MaxPositionSize int64
}

type ArbMode int

const (
	ModeNone ArbMode = iota
	ModeContango    // Long Spot, Short Perp (Positive basis or funding)
	ModeBackwardation // Short Spot, Long Perp (Negative basis or funding)
)

type InternalFundingArb struct {
	id        uint64
	config    InternalFundingArbConfig
	spotOMS   *actor.NettingOMS
	perpOMS   *actor.NettingOMS
	baseAsset string

	spotMid         int64
	perpMid         int64
	fundingRate     int64
	lastNextFunding int64
	
	currentMode ArbMode
	pendingExit bool // exit orders submitted, waiting for fills

	pendingRequests map[uint64]string // requestID → symbol
	orderSymbols    map[uint64]string // orderID → symbol
}

func NewInternalFundingArb(config InternalFundingArbConfig) *InternalFundingArb {
	baseAsset := ""
	if config.SpotInstrument != nil {
		baseAsset = config.SpotInstrument.BaseAsset()
	}

	return &InternalFundingArb{
		id:              config.ActorID,
		config:          config,
		spotOMS:         actor.NewNettingOMS(),
		perpOMS:         actor.NewNettingOMS(),
		baseAsset:       baseAsset,
		pendingRequests: make(map[uint64]string),
		orderSymbols:    make(map[uint64]string),
		currentMode:     ModeNone,
	}
}

func (ifa *InternalFundingArb) GetID() uint64 {
	return ifa.id
}

func (ifa *InternalFundingArb) GetSymbols() []string {
	return []string{ifa.config.SpotSymbol, ifa.config.PerpSymbol}
}

func (ifa *InternalFundingArb) OnEvent(event *actor.Event, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	switch event.Type {
	case actor.EventBookSnapshot:
		ifa.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		ifa.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		ifa.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)
	case actor.EventFundingUpdate:
		ifa.onFundingUpdate(event.Data.(actor.FundingUpdateEvent), ctx)
	case actor.EventOrderRejected:
		ifa.onOrderRejected(event.Data.(actor.OrderRejectedEvent))
	}

	ifa.evaluateStrategy(ctx, submit)
}

func (ifa *InternalFundingArb) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	mid := snap.Snapshot.Bids[0].Price + (snap.Snapshot.Asks[0].Price-snap.Snapshot.Bids[0].Price)/2

	if snap.Symbol == ifa.config.SpotSymbol {
		ifa.spotMid = mid
	} else if snap.Symbol == ifa.config.PerpSymbol {
		ifa.perpMid = mid
	}
}

func (ifa *InternalFundingArb) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	if symbol, ok := ifa.pendingRequests[accepted.RequestID]; ok {
		ifa.orderSymbols[accepted.OrderID] = symbol
		delete(ifa.pendingRequests, accepted.RequestID)
	}
}

func (ifa *InternalFundingArb) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	precision := int64(100_000_000)
	if ifa.config.SpotInstrument != nil {
		precision = ifa.config.SpotInstrument.BasePrecision()
	}

	symbol, ok := ifa.orderSymbols[fill.OrderID]
	if !ok {
		// Fallback detection logic if orderIDs lost
		if fill.Side == exchange.Buy {
			if ifa.currentMode == ModeContango {
				symbol = ifa.config.SpotSymbol
			} else {
				symbol = ifa.config.PerpSymbol
			}
		} else {
			if ifa.currentMode == ModeContango {
				symbol = ifa.config.PerpSymbol
			} else {
				symbol = ifa.config.SpotSymbol
			}
		}
	}
	if fill.IsFull {
		delete(ifa.orderSymbols, fill.OrderID)
	}

	if symbol == ifa.config.SpotSymbol {
		ifa.spotOMS.OnFill(ifa.config.SpotSymbol, fill, precision)
		ctx.OnFill(ifa.id, ifa.config.SpotSymbol, fill, precision, ifa.baseAsset)
	} else {
		ifa.perpOMS.OnFill(ifa.config.PerpSymbol, fill, precision)
		ctx.OnFill(ifa.id, ifa.config.PerpSymbol, fill, precision, ifa.baseAsset)
	}
}

func (ifa *InternalFundingArb) onFundingUpdate(update actor.FundingUpdateEvent, ctx *actor.SharedContext) {
	if update.Symbol != ifa.config.PerpSymbol {
		return
	}
	// Funding settlement logic
	if ifa.lastNextFunding != 0 && update.FundingRate.NextFunding > ifa.lastNextFunding && ifa.currentMode != ModeNone {
		precision := int64(100_000_000)
		if ifa.config.PerpInstrument != nil {
			precision = ifa.config.PerpInstrument.BasePrecision()
		}
		perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)
		if perpPos != 0 {
			absPos := perpPos
			if absPos < 0 {
				absPos = -absPos
			}
			entryPrice := ifa.perpOMS.GetPosition(ifa.config.PerpSymbol).AvgPrice
			if entryPrice == 0 {
				entryPrice = ifa.perpMid
			}
			fundingAmount := absPos * entryPrice / precision * ifa.fundingRate / 10000
			
			// Pay or Receive funding
			if (perpPos < 0 && ifa.fundingRate > 0) || (perpPos > 0 && ifa.fundingRate < 0) {
				// Receive funding
				ctx.UpdateBalances("", 0, fundingAmount)
			} else if (perpPos < 0 && ifa.fundingRate < 0) || (perpPos > 0 && ifa.fundingRate > 0) {
				// Pay funding
				ctx.UpdateBalances("", 0, -fundingAmount)
			}
		}
	}
	ifa.fundingRate = update.FundingRate.Rate
	ifa.lastNextFunding = update.FundingRate.NextFunding
}

func (ifa *InternalFundingArb) onOrderRejected(rejected actor.OrderRejectedEvent) {
	if _, ok := ifa.pendingRequests[rejected.RequestID]; !ok {
		return
	}
	delete(ifa.pendingRequests, rejected.RequestID)
	if ifa.pendingExit {
		ifa.pendingExit = false // allow exit retry
	} else {
		ifa.currentMode = ModeNone // entry rejected: allow re-entry
	}
}

func (ifa *InternalFundingArb) evaluateStrategy(ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	if ifa.spotMid == 0 || ifa.perpMid == 0 {
		return
	}

	// Basis calculation in bps: (Perp - Spot) / Spot * 10000
	basisBps := (ifa.perpMid - ifa.spotMid) * 10000 / ifa.spotMid

	if ifa.currentMode == ModeNone {
		// Entry logic
		if basisBps >= ifa.config.BasisThresholdBps || ifa.fundingRate >= ifa.config.MinFundingRate {
			ifa.enterPosition(ctx, submit, ModeContango)
		} else if basisBps <= -ifa.config.BasisThresholdBps || ifa.fundingRate <= -ifa.config.MinFundingRate {
			ifa.enterPosition(ctx, submit, ModeBackwardation)
		}
	} else {
		if ifa.pendingExit {
			// Check if exit fills have arrived (OMS shows flat position).
			spotFlat := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol) == 0
			perpFlat := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol) == 0
			if spotFlat && perpFlat {
				ifa.currentMode = ModeNone
				ifa.pendingExit = false
			}
		} else {
			// Exit when either signal has reversed — OR so that an arb entered
			// on basis alone exits when basis compresses, even if funding hasn't
			// dropped (and vice-versa).
			shouldExit := false
			if ifa.currentMode == ModeContango {
				if basisBps < ifa.config.ExitBasisThresholdBps || ifa.fundingRate < ifa.config.ExitFundingRate {
					shouldExit = true
				}
			} else if ifa.currentMode == ModeBackwardation {
				if basisBps > -ifa.config.ExitBasisThresholdBps || ifa.fundingRate > -ifa.config.ExitFundingRate {
					shouldExit = true
				}
			}
			
			if shouldExit {
				ifa.exitPosition(ctx, submit)
			}
		}
	}
}

func (ifa *InternalFundingArb) enterPosition(ctx *actor.SharedContext, submit actor.OrderSubmitter, mode ArbMode) {
	spotPos := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol)
	perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)

	if spotPos != 0 || perpPos != 0 {
		return
	}

	ifa.currentMode = mode
	log.Printf("[InternalFundingArb %d] Entering %v: SpotMid=%d, PerpMid=%d, Funding=%d bps", 
		ifa.id, mode, ifa.spotMid, ifa.perpMid, ifa.fundingRate)

	precision := int64(100_000_000)
	if ifa.config.SpotInstrument != nil {
		precision = ifa.config.SpotInstrument.BasePrecision()
	}

	if mode == ModeContango {
		// Long Spot, Short Perp
		if !ctx.CanReserveQuote(ifa.config.MaxPositionSize * ifa.spotMid / precision) {
			ifa.currentMode = ModeNone
			return
		}
		spotReqID := submit(ifa.config.SpotSymbol, exchange.Buy, exchange.Market, 0, ifa.config.MaxPositionSize)
		perpReqID := submit(ifa.config.PerpSymbol, exchange.Sell, exchange.Market, 0, ifa.config.MaxPositionSize)
		ifa.pendingRequests[spotReqID] = ifa.config.SpotSymbol
		ifa.pendingRequests[perpReqID] = ifa.config.PerpSymbol
	} else {
		// Short Spot, Long Perp — requires holding the base asset to sell spot.
		if ctx.GetBaseBalance(ifa.baseAsset) < ifa.config.MaxPositionSize {
			ifa.currentMode = ModeNone
			return
		}
		spotReqID := submit(ifa.config.SpotSymbol, exchange.Sell, exchange.Market, 0, ifa.config.MaxPositionSize)
		perpReqID := submit(ifa.config.PerpSymbol, exchange.Buy, exchange.Market, 0, ifa.config.MaxPositionSize)
		ifa.pendingRequests[spotReqID] = ifa.config.SpotSymbol
		ifa.pendingRequests[perpReqID] = ifa.config.PerpSymbol
	}
}

func (ifa *InternalFundingArb) exitPosition(_ *actor.SharedContext, submit actor.OrderSubmitter) {
	spotPos := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol)
	perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)

	log.Printf("[InternalFundingArb %d] Exiting Mode %v: SpotPos=%d, PerpPos=%d", 
		ifa.id, ifa.currentMode, spotPos, perpPos)

	if spotPos != 0 {
		side := exchange.Sell
		qty := spotPos
		if spotPos < 0 {
			side = exchange.Buy
			qty = -spotPos
		}
		reqID := submit(ifa.config.SpotSymbol, side, exchange.Market, 0, qty)
		ifa.pendingRequests[reqID] = ifa.config.SpotSymbol
	}

	if perpPos != 0 {
		side := exchange.Buy
		qty := -perpPos
		if perpPos > 0 {
			side = exchange.Sell
			qty = perpPos
		}
		reqID := submit(ifa.config.PerpSymbol, side, exchange.Market, 0, qty)
		ifa.pendingRequests[reqID] = ifa.config.PerpSymbol
	}

	ifa.pendingExit = true
}

func (m ArbMode) String() string {
	switch m {
	case ModeContango:
		return "Contango (L Spot/S Perp)"
	case ModeBackwardation:
		return "Backwardation (S Spot/L Perp)"
	default:
		return "None"
	}
}
