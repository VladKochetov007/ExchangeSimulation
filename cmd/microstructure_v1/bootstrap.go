package main

import (
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
)

func CreateBootstrapActors(ex *exchange.Exchange, marketConfig *MarketConfig, startActorID uint64) ([]actor.Actor, uint64) {
	bootstrapActors := []actor.Actor{}
	nextActorID := startActorID

	largeBalances := map[string]int64{
		"ABC": 100 * ASSET_PRECISION,
		"BCD": 200 * ASSET_PRECISION,
		"CDE": 500 * ASSET_PRECISION,
		"DEF": 1000 * ASSET_PRECISION,
		"EFG": 5000 * ASSET_PRECISION,
		"USD": 100_000_000 * USD_PRECISION,
	}

	fees := &exchange.FixedFee{}

	for symbol, bootstrapPrice := range marketConfig.BootstrapPrices {
		gateway := ex.ConnectClient(nextActorID, largeBalances, fees)

		instrument := marketConfig.Instruments[symbol]
		baseAsset := instrument.BaseAsset()
		quoteAsset := instrument.QuoteAsset()
		baseBalance := largeBalances[baseAsset]
		quoteBalance := largeBalances[quoteAsset]

		if instrument.IsPerp() {
			ex.AddPerpBalance(nextActorID, baseAsset, largeBalances[baseAsset])
			ex.AddPerpBalance(nextActorID, quoteAsset, largeBalances[quoteAsset])
		}

		firstLP := actors.NewFirstLP(nextActorID, gateway, actors.FirstLPConfig{
			Symbol:            symbol,
			HalfSpreadBps:     50,
			LiquidityMultiple: 50,
			MonitorInterval:   100 * time.Millisecond,
			MinExitSize:       ASSET_PRECISION / 100,
			BootstrapPrice:    bootstrapPrice,
			ExitStrategy:      actors.DefaultExitStrategy,
		})

		firstLP.SetInitialState(instrument)
		firstLP.UpdateBalances(baseBalance, quoteBalance)

		bootstrapActors = append(bootstrapActors, firstLP)
		nextActorID++
	}

	return bootstrapActors, nextActorID
}

func ShutdownBootstrapActors(ex *exchange.Exchange, bootstrapActors []actor.Actor) {
	for _, a := range bootstrapActors {
		lp, ok := a.(*actors.FirstLiquidityProvidingActor)
		if !ok {
			continue
		}

		gw := a.Gateway()

		gw.Mu.Lock()
		gw.Running = false
		gw.Mu.Unlock()

		ex.CancelAllClientOrders(gw.ClientID)
		ex.MDPublisher.Unsubscribe(gw.ClientID, lp.Symbol)
	}
}
