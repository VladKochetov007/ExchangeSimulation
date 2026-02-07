package simulation

import (
	"time"

	"exchange_sim/exchange"
)

// ExchangeConfig configures a single exchange
type ExchangeConfig struct {
	Name      string   // Exchange name (e.g., "binance", "okx")
	Symbols   []string // Symbols available on this exchange
	LatencyMs int      // Simulated latency in milliseconds
}

// MultiSimConfig configures a multi-exchange simulation
type MultiSimConfig struct {
	Exchanges          []ExchangeConfig // Exchange configurations
	GlobalAssets       []string         // Base assets: BTC, ETH, SOL, etc.
	QuoteAsset         string           // Typically "USD"
	OverlapRatio       float64          // 0.0-1.0, how many pairs overlap between exchanges
	SpotToFuturesRatio float64          // 0.5 = 50% spot, 50% futures

	LPsPerSymbol    int // FirstLPs per symbol
	MMsPerSymbol    int // MarketMakers per symbol
	TakersPerSymbol int // Random takers per symbol

	LPSpreadBps   int64         // LP spread in basis points
	MMSpreadBps   int64         // MM spread in basis points
	TakerInterval time.Duration // Taker order interval

	InitialBalances map[string]int64 // Asset -> balance
	Duration        time.Duration    // Simulation duration
	LogDir          string           // Log output directory

	SimSpeedup   float64 // Speedup factor (e.g., 50.0 for 50x speed)
	LPSkewFactor float64 // How much inventory affects price (e.g. 0.0001 per unit)
}

// DefaultMultiSimConfig returns a reasonable default configuration
func DefaultMultiSimConfig() MultiSimConfig {
	return MultiSimConfig{
		Exchanges: []ExchangeConfig{
			{Name: "binance", LatencyMs: 1},
			{Name: "okx", LatencyMs: 5},
			{Name: "bybit", LatencyMs: 3},
		},
		GlobalAssets:       []string{"BTC", "ETH", "SOL", "XRP", "DOGE"},
		QuoteAsset:         "USD",
		OverlapRatio:       0.6,
		SpotToFuturesRatio: 0.5,
		LPsPerSymbol:       2,
		MMsPerSymbol:       3,
		TakersPerSymbol:    1,
		LPSpreadBps:        20,
		MMSpreadBps:        10,
		TakerInterval:      500 * time.Millisecond,
		InitialBalances: map[string]int64{
			"BTC":  100 * exchange.BTC_PRECISION,
			"ETH":  1000 * exchange.ETH_PRECISION,
			"SOL":  10000 * exchange.SATOSHI,
			"XRP":  100000 * exchange.SATOSHI,
			"DOGE": 1000000 * exchange.SATOSHI,
			"USD":  100000000 * exchange.USD_PRECISION,
		},
		Duration:     0,
		LogDir:       "logs",
		SimSpeedup:   50.0,   // 50x speedup
		LPSkewFactor: 0.0005, // 5 bps per unit inventory skew (will need tuning)
	}


}

// GenerateSymbolsWithOverlap generates symbol sets for multiple exchanges with controlled overlap
func GenerateSymbolsWithOverlap(assets []string, numExchanges int, overlapRatio float64) [][]string {
	if numExchanges == 0 || len(assets) == 0 {
		return nil
	}

	result := make([][]string, numExchanges)

	// Calculate how many assets should be common (overlap)
	overlapCount := int(float64(len(assets)) * overlapRatio)
	if overlapCount < 1 && len(assets) > 0 {
		overlapCount = 1 // At least one overlap
	}

	// Common assets (first overlapCount assets are on all exchanges)
	commonAssets := assets[:overlapCount]

	// Remaining assets distributed round-robin
	remainingAssets := assets[overlapCount:]

	for i := 0; i < numExchanges; i++ {
		result[i] = make([]string, 0, len(assets))
		// Add common assets
		result[i] = append(result[i], commonAssets...)

		// Add some unique assets for each exchange (round-robin)
		for j, asset := range remainingAssets {
			if j%numExchanges == i {
				result[i] = append(result[i], asset)
			}
		}
	}

	return result
}

// GenerateInstruments creates spot and perp instruments for given assets
func GenerateInstruments(assets []string, quoteAsset string, spotRatio float64) map[string]exchange.Instrument {
	instruments := make(map[string]exchange.Instrument)

	spotCount := int(float64(len(assets)) * spotRatio)
	if spotCount < 1 && len(assets) > 0 {
		spotCount = 1
	}

	for i, asset := range assets {
		symbol := asset + quoteAsset

		var basePrecision, quotePrecision, tickSize, minSize int64

		switch asset {
		case "BTC":
			basePrecision = exchange.BTC_PRECISION
			quotePrecision = exchange.USD_PRECISION
			tickSize = exchange.DOLLAR_TICK
			minSize = exchange.SATOSHI / 1000
		case "ETH":
			basePrecision = exchange.ETH_PRECISION
			quotePrecision = exchange.USD_PRECISION
			tickSize = exchange.ETH_PRECISION / 100
			minSize = exchange.ETH_PRECISION / 1000
		default:
			// Generic asset
			basePrecision = exchange.SATOSHI
			quotePrecision = exchange.USD_PRECISION
			tickSize = exchange.SATOSHI
			minSize = exchange.SATOSHI / 100
		}

		if i < spotCount {
			instruments[symbol] = exchange.NewSpotInstrument(
				symbol, asset, quoteAsset,
				basePrecision, quotePrecision,
				tickSize, minSize,
			)
		} else {
			instruments[symbol] = exchange.NewPerpFutures(
				symbol, asset, quoteAsset,
				basePrecision, quotePrecision,
				tickSize, minSize,
			)
		}
	}

	return instruments
}

// GetBootstrapPrices returns reasonable bootstrap prices for common assets
func GetBootstrapPrices() map[string]int64 {
	return map[string]int64{
		"BTCUSD":  100000 * exchange.BTC_PRECISION, // $100,000
		"ETHUSD":  3500 * exchange.ETH_PRECISION,   // $3,500
		"SOLUSD":  150 * exchange.SATOSHI,          // $150
		"XRPUSD":  2 * exchange.SATOSHI,            // $2
		"DOGEUSD": exchange.SATOSHI / 10,           // $0.10
	}
}
