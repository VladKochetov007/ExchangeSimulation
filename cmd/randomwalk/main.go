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

	log.Println("ABC-PERP random walk: 900 sim-seconds starting...")
	if err := sim.Runner.Run(context.Background()); err != nil {
		log.Fatal(err)
	}

	const usd = float64(exchange.USD_PRECISION)
	log.Printf("ABC-PERP final mid: $%.2f", float64(sim.MM.Mid())/usd)
	log.Println("Logs written to logs/randomwalk/")
}
