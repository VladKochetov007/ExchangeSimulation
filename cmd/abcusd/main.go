package main

import (
	"context"
	"log"

	"exchange_sim/exchange"
	"exchange_sim/simulations/abcusd"
)

func main() {
	sim, err := abcusd.NewSim()
	if err != nil {
		log.Fatal(err)
	}
	defer sim.Close()

	ctx := context.Background()
	sim.Exchange().StartAutomation(ctx)
	defer sim.Exchange().StopAutomation()

	log.Println("ABC simulation: 25 sim-hours starting...")
	if err := sim.Runner.Run(ctx); err != nil {
		log.Fatal(err)
	}

	const usd = float64(exchange.USD_PRECISION)
	log.Printf("ABC-USD final mid:  $%.2f", float64(sim.MM.Mid("ABC-USD"))/usd)
	log.Printf("ABC-PERP final mid: $%.2f", float64(sim.MM.Mid("ABC-PERP"))/usd)
	log.Println("Logs written to logs/")
}
