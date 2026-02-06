package lifecycle

import (
	"context"
	"exchange_sim/actor"
	"sync"
)

type LifecycleManager struct {
	actors     []actor.Actor
	conditions map[actor.Actor]StartCondition
	started    map[actor.Actor]bool
	mu         sync.Mutex
}

func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{
		actors:     make([]actor.Actor, 0),
		conditions: make(map[actor.Actor]StartCondition),
		started:    make(map[actor.Actor]bool),
	}
}

func (lm *LifecycleManager) RegisterActor(a actor.Actor, condition StartCondition) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.actors = append(lm.actors, a)
	lm.conditions[a] = condition
	lm.started[a] = false
}

func (lm *LifecycleManager) CheckAndStart(ctx context.Context) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, a := range lm.actors {
		if !lm.started[a] && lm.conditions[a].IsSatisfied() {
			a.Start(ctx)
			lm.started[a] = true
		}
	}
}

func (lm *LifecycleManager) AllStarted() bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, a := range lm.actors {
		if !lm.started[a] {
			return false
		}
	}
	return true
}

func (lm *LifecycleManager) StopAll() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, a := range lm.actors {
		if lm.started[a] {
			a.Stop()
		}
	}
}
