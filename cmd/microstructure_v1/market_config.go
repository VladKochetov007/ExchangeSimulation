package main

import (
	"exchange_sim/exchange"
)

const (
	ASSET_PRECISION = 100_000_000
	USD_PRECISION   = 100_000
	CENT_TICK       = 1
)

type MarketConfig struct {
	Instruments     map[string]exchange.Instrument
	BootstrapPrices map[string]int64
	Assets          []string
}

func CreateMarketConfig() *MarketConfig {
	assets := []string{"ABC", "BCD", "CDE", "DEF", "EFG"}

	usdPrices := map[string]int64{
		"ABC": 50_000 * USD_PRECISION,
		"BCD": 25_000 * USD_PRECISION,
		"CDE": 10_000 * USD_PRECISION,
		"DEF": 5_000 * USD_PRECISION,
		"EFG": 1_000 * USD_PRECISION,
	}

	instruments := make(map[string]exchange.Instrument)

	for _, asset := range assets {
		spotSymbol := asset + "/USD"
		instruments[spotSymbol] = exchange.NewSpotInstrument(
			spotSymbol,
			asset,
			"USD",
			ASSET_PRECISION,
			USD_PRECISION,
			CENT_TICK,
			ASSET_PRECISION/1000,
		)

		perpSymbol := asset + "-PERP"
		instruments[perpSymbol] = exchange.NewPerpFutures(
			perpSymbol,
			asset,
			"USD",
			ASSET_PRECISION,
			USD_PRECISION,
			CENT_TICK,
			ASSET_PRECISION/1000,
		)
	}

	for _, quoteAsset := range assets[1:] {
		crossSymbol := quoteAsset + "/ABC"
		instruments[crossSymbol] = exchange.NewSpotInstrument(
			crossSymbol,
			quoteAsset,
			"ABC",
			ASSET_PRECISION,
			ASSET_PRECISION,
			1,
			ASSET_PRECISION/1000,
		)
	}

	allPrices := make(map[string]int64)
	for asset, usdPrice := range usdPrices {
		allPrices[asset+"/USD"] = usdPrice
		allPrices[asset+"-PERP"] = usdPrice
	}

	for _, asset := range assets[1:] {
		crossSymbol := asset + "/ABC"
		allPrices[crossSymbol] = (usdPrices[asset] * ASSET_PRECISION) / usdPrices["ABC"]
	}

	return &MarketConfig{
		Instruments:     instruments,
		BootstrapPrices: allPrices,
		Assets:          assets,
	}
}

func GetSymbolsByType() map[string][]string {
	assets := []string{"ABC", "BCD", "CDE", "DEF", "EFG"}

	usdSpot := make([]string, len(assets))
	for i, asset := range assets {
		usdSpot[i] = asset + "/USD"
	}

	abcSpot := make([]string, len(assets)-1)
	for i, asset := range assets[1:] {
		abcSpot[i] = asset + "/ABC"
	}

	perps := make([]string, len(assets))
	for i, asset := range assets {
		perps[i] = asset + "-PERP"
	}

	return map[string][]string{
		"usd_spot": usdSpot,
		"abc_spot": abcSpot,
		"perps":    perps,
	}
}
