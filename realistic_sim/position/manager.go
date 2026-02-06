package position

import (
	"exchange_sim/actor"
)

type SizingPolicy interface {
	CalculateSize(signal int64, totalSignal int64, allocatedCapital int64) int64
}

type FixedSizing struct {
	PositionSize int64
}

func (fs *FixedSizing) CalculateSize(signal int64, totalSignal int64, allocatedCapital int64) int64 {
	if signal == 0 {
		return 0
	}
	if signal > 0 {
		return fs.PositionSize
	}
	return -fs.PositionSize
}

type ProportionalSizing struct{}

func (ps *ProportionalSizing) CalculateSize(signal int64, totalSignal int64, allocatedCapital int64) int64 {
	if totalSignal == 0 {
		return 0
	}
	return (allocatedCapital * signal) / totalSignal
}

type KellyCriterion struct {
	EdgeScale int64
}

func (kc *KellyCriterion) CalculateSize(signal int64, totalSignal int64, allocatedCapital int64) int64 {
	if totalSignal == 0 {
		return 0
	}
	edge := (signal * kc.EdgeScale) / totalSignal
	variance := int64(10000)
	fraction := edge / variance
	return (allocatedCapital * fraction) / 10000
}

type PositionManager struct {
	oms              *actor.NettingOMS
	policy           SizingPolicy
	allocatedCapital int64
}

func NewPositionManager(oms *actor.NettingOMS, policy SizingPolicy, allocatedCapital int64) *PositionManager {
	return &PositionManager{
		oms:              oms,
		policy:           policy,
		allocatedCapital: allocatedCapital,
	}
}

func (pm *PositionManager) TargetPosition(signal int64, totalSignal int64, midPrice int64) int64 {
	if midPrice == 0 {
		return 0
	}
	targetValue := pm.policy.CalculateSize(signal, totalSignal, pm.allocatedCapital)
	return targetValue / midPrice
}

func (pm *PositionManager) GetPosition(symbol string) int64 {
	pos := pm.oms.GetPosition(symbol)
	if pos == nil || pos.Qty == 0 {
		return 0
	}
	if pos.Side == 1 {
		return -int64(pos.Qty)
	}
	return int64(pos.Qty)
}
