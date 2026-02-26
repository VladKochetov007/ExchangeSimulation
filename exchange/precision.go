package exchange

const (
	// Asset precisions (units per whole unit)
	BTC_PRECISION  = 100_000_000
	ETH_PRECISION  = 1_000_000
	USD_PRECISION  = 100_000
	USDT_PRECISION = 100_000

	// Common price tick sizes for USD-denominated pairs
	CENT_TICK    = USD_PRECISION / 100 // 0.01 USD tick
	DOLLAR_TICK  = USD_PRECISION       // 1 USD tick
	HUNDRED_TICK = 100 * USD_PRECISION // 100 USD tick
)

// BTCAmount converts a float BTC amount to integer satoshi units.
func BTCAmount(btc float64) int64 { return int64(btc * float64(BTC_PRECISION)) }

// ETHAmount converts a float ETH amount to integer micro-ether units.
func ETHAmount(eth float64) int64 { return int64(eth * float64(ETH_PRECISION)) }

// USDAmount converts a float USD amount to integer precision units.
func USDAmount(usd float64) int64 { return int64(usd * float64(USD_PRECISION)) }

// USDTAmount converts a float USDT amount to integer precision units.
func USDTAmount(usdt float64) int64 { return int64(usdt * float64(USDT_PRECISION)) }

// PriceUSD converts a float price to USD precision units, rounded down to tickSize.
func PriceUSD(price float64, tickSize int64) int64 {
	raw := int64(price * float64(USD_PRECISION))
	return (raw / tickSize) * tickSize
}
