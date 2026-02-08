package main

import "time"

// InstrumentSpec defines a single tradeable instrument.
type InstrumentSpec struct {
	Symbol         string
	Base           string
	Quote          string
	BasePrecision  int64
	QuotePrecision int64
	TickSize       int64
	MinOrderSize   int64
	BootstrapPrice int64 // initial mid price (in quote precision units)
	IsPerp         bool
}

// IndustrialConfig is the top-level simulation configuration.
type IndustrialConfig struct {
	// Instruments to trade
	Instruments []InstrumentSpec

	// Actor counts and balances
	FirstLPPerSymbol int
	FirstLPBalance   map[string]int64 // asset -> amount in precision units

	PureMMPerSymbol int
	PureMMSpreads   []int64         // one spread per MM, len must equal PureMMPerSymbol
	PureMMBalance   map[string]int64

	StoikovPerSymbol int
	StoikovBalance   map[string]int64

	FundingArbPairs []FundingArbPairSpec // one entry per spot/perp pair
	FundingArbBalance map[string]int64

	RandomTakerPerSymbol int
	TakerBalance         map[string]int64

	// Simulation timing
	Duration         time.Duration // sim time to run
	Speedup          float64       // real-to-sim time ratio (1000 = 1000x)
	SnapshotInterval time.Duration // how often to publish L3 snapshots

	// Logging
	LogDir string
}

// FundingArbPairSpec ties a spot symbol to its perp counterpart.
type FundingArbPairSpec struct {
	SpotSymbol string
	PerpSymbol string
}

func defaultConfig() IndustrialConfig {
	const (
		btcPrecision = 100_000_000
		usdPrecision = 100_000
		centTick     = btcPrecision / 100
	)
	return IndustrialConfig{
		Instruments: []InstrumentSpec{
			{
				Symbol: "BTCUSD", Base: "BTC", Quote: "USD",
				BasePrecision: btcPrecision, QuotePrecision: usdPrecision,
				TickSize: centTick, MinOrderSize: btcPrecision / 1000,
				BootstrapPrice: 100_000 * usdPrecision,
			},
			{
				Symbol: "BTC-PERP", Base: "BTC", Quote: "USD",
				BasePrecision: btcPrecision, QuotePrecision: usdPrecision,
				TickSize: centTick, MinOrderSize: btcPrecision / 1000,
				BootstrapPrice: 100_000 * usdPrecision,
				IsPerp:         true,
			},
		},

		FirstLPPerSymbol: 1,
		FirstLPBalance: map[string]int64{
			"BTC": 100 * btcPrecision,
			"USD": 10_000_000 * usdPrecision,
		},

		PureMMPerSymbol: 3,
		PureMMSpreads:   []int64{5, 10, 20},
		PureMMBalance: map[string]int64{
			"BTC": 50 * btcPrecision,
			"USD": 5_000_000 * usdPrecision,
		},

		StoikovPerSymbol: 2,
		StoikovBalance: map[string]int64{
			"BTC": 30 * btcPrecision,
			"USD": 3_000_000 * usdPrecision,
		},

		FundingArbPairs: []FundingArbPairSpec{
			{SpotSymbol: "BTCUSD", PerpSymbol: "BTC-PERP"},
		},
		FundingArbBalance: map[string]int64{
			"BTC": 20 * btcPrecision,
			"USD": 2_000_000 * usdPrecision,
		},

		RandomTakerPerSymbol: 2,
		TakerBalance: map[string]int64{
			"BTC": 10 * btcPrecision,
			"USD": 1_000_000 * usdPrecision,
		},

		Duration:         30 * 24 * time.Hour, // 30 days sim time
		Speedup:          1000,
		SnapshotInterval: 100 * time.Millisecond,

		LogDir: "logs/industrial",
	}
}
