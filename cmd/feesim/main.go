package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulations/feesim"
)

const simDuration = time.Minute * 30

func printProgress(done, total int, simTotal time.Duration, started time.Time) {
	pct := float64(done) / float64(total)
	elapsed := time.Since(started)

	var eta string
	if pct > 0 {
		remaining := time.Duration(float64(elapsed) / pct * (1 - pct))
		eta = remaining.Round(time.Second).String()
	} else {
		eta = "?"
	}

	simElapsed := time.Duration(float64(simTotal) * pct)

	const barWidth = 30
	filled := int(pct * barWidth)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	fmt.Printf("\r[%s] %5.1f%%  sim %s / %s  ETA %s     ",
		bar, pct*100,
		fmtDuration(simElapsed), fmtDuration(simTotal),
		eta,
	)
}

func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}

func main() {
	zeroFee := flag.Bool("zerofee", false, "run with zero fees")
	flag.Parse()

	cfg := feesim.DefaultSimConfig()
	if *zeroFee {
		cfg.SpotTakerBps = 0
		cfg.PerpTakerBps = 0
		cfg.LogDir = "logs/feesim-zerofee"
	}

	sim, err := feesim.NewSim(simDuration, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer sim.Close()

	started := time.Now()
	sim.Runner.SetProgressCallback(100_000, func(done, total int) {
		printProgress(done, total, simDuration, started)
	})

	ctx, cancel := context.WithCancel(context.Background())
	sim.Exchange().StartAutomation(ctx)

	log.Println("starting feesim (10 min sim, 5 markets, fee-aware arbs)...")
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})
	if err := sim.Runner.Run(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println()

	const usd = float64(exchange.USD_PRECISION)
	for _, mm := range sim.MMs {
		log.Printf("%-12s final mid: $%.2f", mm.Symbol(), float64(mm.Mid())/usd)
	}
	for _, arb := range sim.BasisArbs {
		log.Printf("BasisArb %s/%s  pos=%d",
			arb.Symbol(), arb.PerpSymbol(), arb.Position())
	}
	log.Printf("Logs written to %s/", cfg.LogDir)
}
