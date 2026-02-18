package actors

import (
	"testing"

	"exchange_sim/exchange"
)

// --- FixedHalfSpread ---

func TestFixedHalfSpread_ReturnsConstant(t *testing.T) {
	m := &FixedHalfSpread{Bps: 50}
	for _, inv := range []int64{-1000, 0, 1000} {
		if got := m.HalfSpread(nil, inv); got != 50 {
			t.Errorf("inventory %d: want 50, got %d", inv, got)
		}
	}
}

// --- OFISpreadModel ---

func TestOFI_InitialSpreadIsBase(t *testing.T) {
	m := &OFISpreadModel{BaseBps: 10, MaxExtraBps: 50, WindowVolume: 1000}
	if got := m.HalfSpread(nil, 0); got != 10 {
		t.Errorf("fresh model: want BaseBps=10, got %d", got)
	}
}

func TestOFI_DecayFactorZeroDefaultsTo900(t *testing.T) {
	m := &OFISpreadModel{BaseBps: 0, MaxExtraBps: 1000, WindowVolume: 0, DecayFactor: 0}
	m.OnTrade(exchange.Buy, 1000)
	// decay=900: signedVolume=900, totalVolume=900
	// denom=max(0,900)=900, extra=min(1000*900/900,1000)=1000 (capped at MaxExtraBps)
	if got := m.HalfSpread(nil, 0); got != 1000 {
		t.Errorf("DecayFactor=0 should default to 900, want saturated extra=1000, got %d", got)
	}
}

func TestOFI_DecayFactorExplicit(t *testing.T) {
	// DecayFactor=500 → retain 50% per trade. After one buy of 1000:
	// signedVolume=500, totalVolume=500, WindowVolume=0 → denom=500 → saturates at MaxExtraBps.
	m := &OFISpreadModel{BaseBps: 0, MaxExtraBps: 1000, WindowVolume: 0, DecayFactor: 500}
	m.OnTrade(exchange.Buy, 1000)
	if got := m.HalfSpread(nil, 0); got != 1000 {
		t.Errorf("DecayFactor=500 should saturate: want 1000, got %d", got)
	}
}

func TestOFI_BuyTradeWidensSpread(t *testing.T) {
	// DecayFactor=1000 (no decay). After buy 500:
	// signedVolume=500, totalVolume=500
	// denom=max(1000,500)=1000, extra=min(50*500/1000,50)=25
	m := &OFISpreadModel{BaseBps: 10, MaxExtraBps: 50, WindowVolume: 1000, DecayFactor: 1000}
	m.OnTrade(exchange.Buy, 500)
	if got := m.HalfSpread(nil, 0); got != 35 {
		t.Errorf("want 35 (10+25), got %d", got)
	}
}

func TestOFI_SellTradeWidensSpreadSymmetrically(t *testing.T) {
	m := &OFISpreadModel{BaseBps: 10, MaxExtraBps: 50, WindowVolume: 1000, DecayFactor: 1000}
	m.OnTrade(exchange.Sell, 500) // |imbalance| same as buy 500
	if got := m.HalfSpread(nil, 0); got != 35 {
		t.Errorf("sell imbalance should widen identically: want 35, got %d", got)
	}
}

func TestOFI_MaxExtraBpsCapped(t *testing.T) {
	// WindowVolume=100, buy 1000 >> denom → extra saturates at MaxExtraBps.
	m := &OFISpreadModel{BaseBps: 10, MaxExtraBps: 50, WindowVolume: 100, DecayFactor: 1000}
	m.OnTrade(exchange.Buy, 1000)
	if got := m.HalfSpread(nil, 0); got != 60 {
		t.Errorf("saturated: want 60 (10+50), got %d", got)
	}
}

func TestOFI_BalancedFlowCollapseToBase(t *testing.T) {
	m := &OFISpreadModel{BaseBps: 10, MaxExtraBps: 50, WindowVolume: 1000, DecayFactor: 1000}
	m.OnTrade(exchange.Buy, 500)
	m.OnTrade(exchange.Sell, 500) // equal sell cancels signed volume
	if got := m.HalfSpread(nil, 0); got != 10 {
		t.Errorf("balanced flow: want base 10, got %d", got)
	}
}

func TestOFI_TotalVolumeFloorAtOne(t *testing.T) {
	// DecayFactor=1 (0.1% retention) → totalVolume floor must prevent div-by-zero.
	m := &OFISpreadModel{BaseBps: 5, MaxExtraBps: 20, WindowVolume: 0, DecayFactor: 1}
	for range 1000 {
		m.OnTrade(exchange.Buy, 100)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HalfSpread panicked: %v", r)
		}
	}()
	m.HalfSpread(nil, 0)
}
