package actor

import (
	"exchange_sim/exchange"
	"sync"
)

type OMSType uint8

const (
	OMSNetting OMSType = iota
	OMSHedging
)

type Position struct {
	InstrumentID string
	PositionID   string
	Side         exchange.Side
	Qty          int64
	AvgPrice     int64
	RealizedPnL  int64
}

type OMS interface {
	OnFill(instrumentID string, fill OrderFillEvent, precision int64)
	GetNetPosition(instrumentID string) int64
	GetPosition(instrumentID string) *Position
	GetPositions(instrumentID string) []*Position
	Reset(instrumentID string)
}

type NettingOMS struct {
	positions map[string]*Position
	mu        sync.RWMutex
}

func NewNettingOMS() *NettingOMS {
	return &NettingOMS{
		positions: make(map[string]*Position),
	}
}

func (o *NettingOMS) OnFill(instrumentID string, fill OrderFillEvent, precision int64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	pos := o.positions[instrumentID]
	if pos == nil {
		pos = &Position{
			InstrumentID: instrumentID,
			PositionID:   "net",
			Qty:          0,
			AvgPrice:     0,
			RealizedPnL:  0,
		}
		o.positions[instrumentID] = pos
	}

	deltaQty := fill.Qty
	if fill.Side == exchange.Sell {
		deltaQty = -deltaQty
	}

	if pos.Qty == 0 {
		pos.Qty = deltaQty
		pos.AvgPrice = fill.Price
		pos.Side = fill.Side
		return
	}

	sameDirection := (pos.Qty > 0 && deltaQty > 0) || (pos.Qty < 0 && deltaQty < 0)

	if sameDirection {
		oldQty := pos.Qty
		newQty := pos.Qty + deltaQty
		if newQty != 0 {
			w1 := (oldQty * 1000) / newQty
			w2 := (deltaQty * 1000) / newQty
			pos.AvgPrice = (pos.AvgPrice*w1 + fill.Price*w2) / 1000
		}
		pos.Qty = newQty
	} else {
		absQty := deltaQty
		if absQty < 0 {
			absQty = -absQty
		}
		absPos := pos.Qty
		if absPos < 0 {
			absPos = -absPos
		}

		if absQty <= absPos {
			pnl := (fill.Price - pos.AvgPrice) / precision * absQty
			if pos.Qty < 0 {
				pnl = -pnl
			}
			pos.RealizedPnL += pnl

			pos.Qty += deltaQty
			if pos.Qty == 0 {
				pos.AvgPrice = 0
				pos.Side = exchange.Buy
			}
		} else {
			pnl := (fill.Price - pos.AvgPrice) / precision * absPos
			if pos.Qty < 0 {
				pnl = -pnl
			}
			pos.RealizedPnL += pnl

			pos.Qty += deltaQty
			pos.AvgPrice = fill.Price
			if pos.Qty > 0 {
				pos.Side = exchange.Buy
			} else {
				pos.Side = exchange.Sell
			}
		}
	}
}

func (o *NettingOMS) GetNetPosition(instrumentID string) int64 {
	o.mu.RLock()
	pos := o.positions[instrumentID]
	o.mu.RUnlock()
	if pos == nil {
		return 0
	}
	return pos.Qty
}

func (o *NettingOMS) GetPosition(instrumentID string) *Position {
	o.mu.RLock()
	pos := o.positions[instrumentID]
	o.mu.RUnlock()
	if pos == nil {
		return &Position{InstrumentID: instrumentID, PositionID: "net"}
	}
	return &Position{
		InstrumentID: pos.InstrumentID,
		PositionID:   pos.PositionID,
		Side:         pos.Side,
		Qty:          pos.Qty,
		AvgPrice:     pos.AvgPrice,
		RealizedPnL:  pos.RealizedPnL,
	}
}

func (o *NettingOMS) GetPositions(instrumentID string) []*Position {
	pos := o.GetPosition(instrumentID)
	if pos.Qty == 0 {
		return nil
	}
	return []*Position{pos}
}

func (o *NettingOMS) Reset(instrumentID string) {
	o.mu.Lock()
	delete(o.positions, instrumentID)
	o.mu.Unlock()
}

type HedgingOMS struct {
	positions map[string]map[string]*Position
	nextPosID map[string]uint64
	mu        sync.RWMutex
}

func NewHedgingOMS() *HedgingOMS {
	return &HedgingOMS{
		positions: make(map[string]map[string]*Position),
		nextPosID: make(map[string]uint64),
	}
}

func (o *HedgingOMS) OnFill(instrumentID string, fill OrderFillEvent, precision int64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.positions[instrumentID] == nil {
		o.positions[instrumentID] = make(map[string]*Position)
	}
	if o.nextPosID[instrumentID] == 0 {
		o.nextPosID[instrumentID] = 1
	}

	posID := ""
	for id, pos := range o.positions[instrumentID] {
		if pos.Side == fill.Side {
			posID = id
			break
		}
	}

	if posID == "" {
		posID = instrumentID + "-" + string(rune('0'+o.nextPosID[instrumentID]))
		o.nextPosID[instrumentID]++
	}

	pos := o.positions[instrumentID][posID]
	if pos == nil {
		pos = &Position{
			InstrumentID: instrumentID,
			PositionID:   posID,
			Side:         fill.Side,
			Qty:          0,
			AvgPrice:     0,
			RealizedPnL:  0,
		}
		o.positions[instrumentID][posID] = pos
	}

	oldQty := pos.Qty
	newQty := pos.Qty + fill.Qty
	if newQty > 0 {
		w1 := (oldQty * 1000) / newQty
		w2 := (fill.Qty * 1000) / newQty
		pos.AvgPrice = (pos.AvgPrice*w1 + fill.Price*w2) / 1000
	}
	pos.Qty = newQty
}

func (o *HedgingOMS) GetNetPosition(instrumentID string) int64 {
	o.mu.RLock()
	defer o.mu.RUnlock()

	net := int64(0)
	if o.positions[instrumentID] == nil {
		return 0
	}

	for _, pos := range o.positions[instrumentID] {
		if pos.Side == exchange.Buy {
			net += pos.Qty
		} else {
			net -= pos.Qty
		}
	}
	return net
}

func (o *HedgingOMS) GetPosition(instrumentID string) *Position {
	positions := o.GetPositions(instrumentID)
	if len(positions) == 0 {
		return &Position{InstrumentID: instrumentID}
	}
	return positions[0]
}

func (o *HedgingOMS) GetPositions(instrumentID string) []*Position {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.positions[instrumentID] == nil {
		return nil
	}

	result := make([]*Position, 0, len(o.positions[instrumentID]))
	for _, pos := range o.positions[instrumentID] {
		result = append(result, &Position{
			InstrumentID: pos.InstrumentID,
			PositionID:   pos.PositionID,
			Side:         pos.Side,
			Qty:          pos.Qty,
			AvgPrice:     pos.AvgPrice,
			RealizedPnL:  pos.RealizedPnL,
		})
	}
	return result
}

func (o *HedgingOMS) Reset(instrumentID string) {
	o.mu.Lock()
	delete(o.positions, instrumentID)
	delete(o.nextPosID, instrumentID)
	o.mu.Unlock()
}
