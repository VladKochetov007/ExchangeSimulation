package repeatedspot

import (
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type LocalReferenceFixedMakerConfig struct {
	BoundedFixedMakerConfig
	MaxAge time.Duration `json:"local_reference_max_age_ns"`
}

func NewLocalReferenceFixedMakerPolicy(config LocalReferenceFixedMakerConfig) (PolicyDefinition, error) {
	if config.MaxAge < 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: negative local reference lifetime")
	}
	if _, err := NewBoundedFixedMakerPolicy(config.BoundedFixedMakerConfig); err != nil {
		return PolicyDefinition{}, err
	}
	return DefinePolicy("bounded_fixed_maker_v3", config,
		func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, effective LocalReferenceFixedMakerConfig) (actor.Actor, error) {
			return NewRecurringMakerWithLocalReference(id, gateway, effective.Maker,
				fixedMakerQuoteRule(effective.SpreadBps, effective.Maker.TickSize), effective.MaxAge)
		})
}

type LocalReferenceStoikovMakerConfig struct {
	BoundedStoikovMakerConfig
	MaxAge time.Duration `json:"local_reference_max_age_ns"`
}

func NewLocalReferenceStoikovMakerPolicy(config LocalReferenceStoikovMakerConfig) (PolicyDefinition, error) {
	if config.MaxAge < 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: negative local reference lifetime")
	}
	if _, err := NewBoundedStoikovMakerPolicy(config.BoundedStoikovMakerConfig); err != nil {
		return PolicyDefinition{}, err
	}
	return DefinePolicy("bounded_stoikov_maker_v3", config,
		func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, effective LocalReferenceStoikovMakerConfig) (actor.Actor, error) {
			return NewRecurringMakerWithLocalReference(id, gateway, effective.Maker,
				stoikovMakerQuoteRule(effective.BoundedStoikovMakerConfig), effective.MaxAge)
		})
}
