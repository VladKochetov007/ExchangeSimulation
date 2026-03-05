package main

import (
	"context"
	"log"

	"exchange_sim/exchange"
	"exchange_sim/simulations/randomwalk"
)

func main() {
	sim, err := randomwalk.NewSim()
	if err != nil {
		log.Fatal(err)
	}
	defer sim.Close()

	ctx := context.Background()
	sim.Exchange().StartAutomation(ctx)
	defer sim.Exchange().StopAutomation()

	log.Println("3-asset random walk: 900 sim-seconds starting...")
	if err := sim.Runner.Run(ctx); err != nil {
		log.Fatal(err)
	}

	const usd = float64(exchange.USD_PRECISION)
	for _, mm := range sim.MMs {
		for _, sym := range []string{mm.Symbols()[0], mm.Symbols()[1]} {
			log.Printf("%-12s final mid: $%.2f", sym, float64(mm.Mid(sym))/usd)
		}
	}
	log.Println("Logs written to logs/randomwalk/")
}
