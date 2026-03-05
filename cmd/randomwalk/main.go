package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulations/randomwalk"
)

const simDuration = time.Minute * 10

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
	sim, err := randomwalk.NewSim(simDuration)
	if err != nil {
		log.Fatal(err)
	}
	defer sim.Close()

	started := time.Now()
	sim.Runner.SetProgressCallback(100_000, func(done, total int) {
		printProgress(done, total, simDuration, started)
	})

	ctx := context.Background()
	sim.Exchange().StartAutomation(ctx)
	defer sim.Exchange().StopAutomation()

	log.Println("starting...")
	if err := sim.Runner.Run(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println() // newline after progress bar

	const usd = float64(exchange.USD_PRECISION)
	for _, mm := range sim.MMs {
		for _, sym := range []string{mm.Symbols()[0], mm.Symbols()[1]} {
			log.Printf("%-12s final mid: $%.2f", sym, float64(mm.Mid(sym))/usd)
		}
	}
	log.Println("Logs written to logs/randomwalk/")
}
