// latencylab runs the controlled public-signal conversion race. It is a causal
// latency mechanism test, not a claim about long-horizon strategy profitability.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"exchange_sim/simulations/latencylab"
)

func main() {
	alphaLatency := flag.Duration("alpha-latency", time.Millisecond, "alpha request/response/market-data latency")
	betaLatency := flag.Duration("beta-latency", 5*time.Millisecond, "beta request/response/market-data latency")
	reverseActors := flag.Bool("reverse-actors", false, "register beta before alpha without changing physical client IDs")
	flag.Parse()

	sim, err := latencylab.NewSim(latencylab.Config{
		AlphaLatency:  *alphaLatency,
		BetaLatency:   *betaLatency,
		ReverseActors: *reverseActors,
	})
	if err != nil {
		log.Fatal(err)
	}
	result, err := sim.Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Fatal(err)
	}
}
