package position

import (
	"testing"
)

func TestFixedSizing(t *testing.T) {
	fs := &FixedSizing{PositionSize: 1000}

	size := fs.CalculateSize(500, 1000, 100000)
	if size != 1000 {
		t.Errorf("FixedSizing with positive signal should return %d, got %d", 1000, size)
	}

	size = fs.CalculateSize(-500, 1000, 100000)
	if size != -1000 {
		t.Errorf("FixedSizing with negative signal should return %d, got %d", -1000, size)
	}

	size = fs.CalculateSize(0, 1000, 100000)
	if size != 0 {
		t.Errorf("FixedSizing with zero signal should return 0, got %d", size)
	}
}

func TestProportionalSizing(t *testing.T) {
	ps := &ProportionalSizing{}

	size := ps.CalculateSize(500, 1000, 100000)
	expected := int64(50000)
	if size != expected {
		t.Errorf("ProportionalSizing should return %d, got %d", expected, size)
	}

	size = ps.CalculateSize(-300, 1000, 100000)
	expected = int64(-30000)
	if size != expected {
		t.Errorf("ProportionalSizing should return %d, got %d", expected, size)
	}

	size = ps.CalculateSize(100, 0, 100000)
	if size != 0 {
		t.Error("ProportionalSizing with zero totalSignal should return 0")
	}
}

func TestKellyCriterion(t *testing.T) {
	kc := &KellyCriterion{EdgeScale: 10000}

	size := kc.CalculateSize(500, 1000, 100000)
	if size < 0 {
		t.Error("KellyCriterion with positive signal should return positive size")
	}

	size = kc.CalculateSize(-500, 1000, 100000)
	if size > 0 {
		t.Error("KellyCriterion with negative signal should return negative size")
	}

	size = kc.CalculateSize(100, 0, 100000)
	if size != 0 {
		t.Error("KellyCriterion with zero totalSignal should return 0")
	}
}

func TestPositionManagerTargetPosition(t *testing.T) {
	ps := &ProportionalSizing{}
	pm := &PositionManager{
		policy:           ps,
		allocatedCapital: 100000,
	}

	midPrice := int64(50000)
	target := pm.TargetPosition(500, 1000, midPrice)
	expectedValue := int64(50000)
	expectedQty := expectedValue / midPrice

	if target != expectedQty {
		t.Errorf("TargetPosition should return %d, got %d", expectedQty, target)
	}

	target = pm.TargetPosition(500, 1000, 0)
	if target != 0 {
		t.Error("TargetPosition with zero midPrice should return 0")
	}
}
