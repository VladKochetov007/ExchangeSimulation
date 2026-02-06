package lifecycle

import (
	"context"
	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
	"testing"
	"time"
)

type MockActor struct {
	id      uint64
	running bool
}

func (ma *MockActor) Start(ctx context.Context) error {
	ma.running = true
	return nil
}

func (ma *MockActor) Stop() error {
	ma.running = false
	return nil
}

func (ma *MockActor) ID() uint64 {
	return ma.id
}

func (ma *MockActor) Gateway() *exchange.ClientGateway {
	return nil
}

func (ma *MockActor) OnEvent(event *actor.Event) {}

func TestAlwaysSatisfied(t *testing.T) {
	cond := &AlwaysSatisfied{}
	if !cond.IsSatisfied() {
		t.Error("AlwaysSatisfied should always return true")
	}
}

func TestDataAvailableCondition(t *testing.T) {
	buffer := actors.NewCircularBuffer(5)
	cond := NewDataAvailableCondition(buffer)

	if cond.IsSatisfied() {
		t.Error("Condition should not be satisfied when buffer is empty")
	}

	for i := 0; i < 5; i++ {
		buffer.Add(int64(i * 100))
	}

	if !cond.IsSatisfied() {
		t.Error("Condition should be satisfied when buffer is full")
	}
}

func TestCompositeConditionAND(t *testing.T) {
	buffer1 := actors.NewCircularBuffer(3)
	buffer2 := actors.NewCircularBuffer(3)

	cond1 := NewDataAvailableCondition(buffer1)
	cond2 := NewDataAvailableCondition(buffer2)

	composite := NewCompositeCondition(true, cond1, cond2)

	if composite.IsSatisfied() {
		t.Error("AND condition should not be satisfied when any condition is false")
	}

	for i := 0; i < 3; i++ {
		buffer1.Add(int64(i))
	}

	if composite.IsSatisfied() {
		t.Error("AND condition should not be satisfied when buffer2 is not full")
	}

	for i := 0; i < 3; i++ {
		buffer2.Add(int64(i))
	}

	if !composite.IsSatisfied() {
		t.Error("AND condition should be satisfied when all conditions are true")
	}
}

func TestCompositeConditionOR(t *testing.T) {
	buffer1 := actors.NewCircularBuffer(3)
	buffer2 := actors.NewCircularBuffer(3)

	cond1 := NewDataAvailableCondition(buffer1)
	cond2 := NewDataAvailableCondition(buffer2)

	composite := NewCompositeCondition(false, cond1, cond2)

	if composite.IsSatisfied() {
		t.Error("OR condition should not be satisfied when all conditions are false")
	}

	for i := 0; i < 3; i++ {
		buffer1.Add(int64(i))
	}

	if !composite.IsSatisfied() {
		t.Error("OR condition should be satisfied when any condition is true")
	}
}

func TestLifecycleManager(t *testing.T) {
	lm := NewLifecycleManager()

	actor1 := &MockActor{id: 1}
	actor2 := &MockActor{id: 2}

	cond1 := &AlwaysSatisfied{}
	buffer := actors.NewCircularBuffer(3)
	cond2 := NewDataAvailableCondition(buffer)

	lm.RegisterActor(actor1, cond1)
	lm.RegisterActor(actor2, cond2)

	ctx := context.Background()
	lm.CheckAndStart(ctx)

	if !actor1.running {
		t.Error("Actor1 should be running (always satisfied)")
	}

	if actor2.running {
		t.Error("Actor2 should not be running yet (data not available)")
	}

	for i := 0; i < 3; i++ {
		buffer.Add(int64(i))
	}

	lm.CheckAndStart(ctx)

	if !actor2.running {
		t.Error("Actor2 should be running now (data available)")
	}

	if !lm.AllStarted() {
		t.Error("All actors should be started")
	}
}

func TestMarketMonitor(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	mm := NewMarketMonitor(ex, "BTC/USD", 50*time.Millisecond)
	mockActor := &MockActor{id: 1}
	mm.AddRestartActor(mockActor)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go mm.Monitor(ctx)

	<-ctx.Done()
}
