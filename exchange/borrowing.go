package exchange

import "errors"

// BorrowContext carries already-resolved, mutable client state for a single borrow/repay call.
// The exchange holds its lock before constructing this; BorrowingManager must not acquire any lock.
type BorrowContext struct {
	Client    *Client
	ClientID  uint64
	Timestamp int64
	LogBalance func(reason string, changes []BalanceDelta)
	LogEvent   func(event string, data any)
}

type BorrowingManager struct {
	Config BorrowingConfig
}

func NewBorrowingManager(config BorrowingConfig) *BorrowingManager {
	return &BorrowingManager{Config: config}
}

func (bm *BorrowingManager) BorrowMargin(ctx BorrowContext, asset string, amount int64, reason string) error {
	if !bm.Config.Enabled {
		return errors.New("borrowing disabled")
	}
	if ctx.Client == nil {
		return errors.New("unknown client")
	}

	if ctx.Client.MarginMode == CrossMargin {
		if err := bm.validateCrossMarginCollateral(ctx.Client, asset, amount); err != nil {
			return err
		}
	} else {
		return errors.New("isolated margin borrow requires position context")
	}

	if limit := bm.Config.MaxBorrowPerAsset[asset]; limit > 0 {
		if ctx.Client.Borrowed[asset]+amount > limit {
			return errors.New("exceeds max borrow limit")
		}
	}

	oldBorrowed := ctx.Client.Borrowed[asset]
	ctx.Client.Borrowed[asset] += amount

	oldPerp := ctx.Client.PerpBalances[asset]
	ctx.Client.PerpBalances[asset] += amount

	rate := bm.getRate(asset)
	collateral := bm.CalculateCollateralUsed(asset, amount)

	if ctx.LogBalance != nil {
		ctx.LogBalance("borrow", []BalanceDelta{
			perpDelta(asset, oldPerp, ctx.Client.PerpBalances[asset]),
			borrowedDelta(asset, oldBorrowed, ctx.Client.Borrowed[asset]),
		})
	}
	if ctx.LogEvent != nil {
		ctx.LogEvent("borrow", BorrowEvent{
			Timestamp:      ctx.Timestamp,
			ClientID:       ctx.ClientID,
			Asset:          asset,
			Amount:         amount,
			Reason:         reason,
			MarginMode:     ctx.Client.MarginMode.String(),
			InterestRate:   rate,
			CollateralUsed: collateral,
		})
	}

	return nil
}

func (bm *BorrowingManager) RepayMargin(ctx BorrowContext, asset string, amount int64) error {
	borrowed := ctx.Client.Borrowed[asset]
	if borrowed == 0 {
		return errors.New("no outstanding debt")
	}
	if amount > borrowed {
		amount = borrowed
	}
	if ctx.Client.PerpAvailable(asset) < amount {
		return errors.New("insufficient balance to repay")
	}

	oldPerp := ctx.Client.PerpBalances[asset]
	ctx.Client.PerpBalances[asset] -= amount

	oldBorrowed := ctx.Client.Borrowed[asset]
	ctx.Client.Borrowed[asset] -= amount

	if ctx.LogBalance != nil {
		ctx.LogBalance("repay", []BalanceDelta{
			perpDelta(asset, oldPerp, ctx.Client.PerpBalances[asset]),
			borrowedDelta(asset, oldBorrowed, ctx.Client.Borrowed[asset]),
		})
	}
	if ctx.LogEvent != nil {
		ctx.LogEvent("repay", RepayEvent{
			Timestamp:     ctx.Timestamp,
			ClientID:      ctx.ClientID,
			Asset:         asset,
			Principal:     amount,
			Interest:      0,
			RemainingDebt: ctx.Client.Borrowed[asset],
		})
	}

	return nil
}

func (bm *BorrowingManager) validateCrossMarginCollateral(client *Client, borrowAsset string, borrowAmount int64) error {
	if bm.Config.PriceSource == nil {
		return errors.New("price oracle not configured")
	}

	totalCollateralValue := int64(0)
	for asset, balance := range client.PerpBalances {
		if balance <= 0 {
			continue
		}
		if price := bm.Config.PriceSource.Price(asset); price > 0 {
			totalCollateralValue += (balance / btcPrecision) * price
		}
	}

	existingBorrowValue := int64(0)
	for asset, borrowed := range client.Borrowed {
		if borrowed <= 0 {
			continue
		}
		if price := bm.Config.PriceSource.Price(asset); price > 0 {
			existingBorrowValue += (borrowed / btcPrecision) * price
		}
	}

	borrowPrice := bm.Config.PriceSource.Price(borrowAsset)
	if borrowPrice == 0 {
		return errors.New("price unavailable")
	}
	newBorrowValue := (borrowAmount / btcPrecision) * borrowPrice
	maxBorrowValue := int64(float64(totalCollateralValue) * bm.getCollateralFactor(borrowAsset))

	if existingBorrowValue+newBorrowValue > maxBorrowValue {
		return errors.New("insufficient collateral")
	}
	return nil
}

func (bm *BorrowingManager) getRate(asset string) int64 {
	if rate, ok := bm.Config.BorrowRates[asset]; ok {
		return rate
	}
	if rate, ok := bm.Config.BorrowRates["default"]; ok {
		return rate
	}
	return 500
}

func (bm *BorrowingManager) getCollateralFactor(asset string) float64 {
	if factor, ok := bm.Config.CollateralFactors[asset]; ok {
		return factor
	}
	if factor, ok := bm.Config.CollateralFactors["default"]; ok {
		return factor
	}
	return 0.75
}

func (bm *BorrowingManager) CalculateCollateralUsed(asset string, amount int64) int64 {
	if bm.Config.PriceSource == nil {
		return 0
	}
	price := bm.Config.PriceSource.Price(asset)
	if price == 0 {
		return 0
	}
	factor := bm.getCollateralFactor(asset)
	if factor == 0 {
		return 0
	}
	return int64(float64((amount/btcPrecision)*price) / factor)
}
