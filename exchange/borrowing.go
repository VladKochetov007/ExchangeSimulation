package exchange

import (
	"errors"
	"fmt"
)

// BorrowContext carries already-resolved, mutable client state for a single borrow/repay call.
// The exchange holds its lock before constructing this; BorrowingManager must not acquire any lock.
type BorrowContext struct {
	Client    *Client
	ClientID  uint64
	Timestamp int64
	// CreditSpot routes the borrowed funds to the spot wallet. Spot auto-borrow
	// must credit the wallet the reservation retry reads, or the loan is booked
	// while the order still rejects.
	CreditSpot bool
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
	// A non-positive borrow would move the balance and debt the wrong way: a
	// negative amount conjures negative debt (the exchange owing the client) and
	// silently drains the wallet, slipping past every downstream limit check.
	if amount <= 0 {
		return errors.New("borrow amount must be positive")
	}

	if ctx.Client.MarginMode == CrossMargin {
		if err := bm.validateCrossMarginCollateral(ctx.Client, asset, amount); err != nil {
			return err
		}
	} else {
		return errors.New("isolated margin borrow requires position context")
	}
	collateral, err := bm.CalculateCollateralUsed(asset, amount)
	if err != nil {
		return fmt.Errorf("borrow collateral %s: %w", asset, err)
	}

	if limit := bm.Config.MaxBorrowPerAsset[asset]; limit > 0 {
		if ctx.Client.Borrowed[asset]+amount > limit {
			return errors.New("exceeds max borrow limit")
		}
	}

	oldBorrowed := ctx.Client.Borrowed[asset]
	ctx.Client.Borrowed[asset] += amount

	var walletDelta BalanceDelta
	if ctx.CreditSpot {
		oldSpot := ctx.Client.Balances[asset]
		ctx.Client.Balances[asset] += amount
		// Record the attribution: equity, interest, and snapshots charge this
		// liability to the wallet that actually received the cash.
		ctx.Client.BorrowedSpot[asset] += amount
		walletDelta = spotDelta(asset, oldSpot, ctx.Client.Balances[asset])
	} else {
		oldPerp := ctx.Client.PerpBalances[asset]
		ctx.Client.PerpBalances[asset] += amount
		walletDelta = perpDelta(asset, oldPerp, ctx.Client.PerpBalances[asset])
	}

	rate := bm.getRate(asset)
	if ctx.LogBalance != nil {
		ctx.LogBalance("borrow", []BalanceDelta{
			walletDelta,
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
	if ctx.Client == nil {
		return errors.New("unknown client")
	}
	// A negative repayment subtracts a negative, inflating BOTH the wallet and
	// the outstanding debt — free credit paired with more liability.
	if amount <= 0 {
		return errors.New("repay amount must be positive")
	}
	borrowed := ctx.Client.Borrowed[asset]
	if borrowed == 0 {
		return errors.New("no outstanding debt")
	}
	if amount > borrowed {
		amount = borrowed
	}

	// A borrow credits either the perp wallet (margin borrow) or the spot wallet
	// (auto-borrow for a spot order). Repay may split the debit across both —
	// perp first, preserving the historical margin-repay path — so debt is never
	// stuck while the account holds enough cash spread over two wallets. The
	// check is atomic: nothing moves unless the combined available covers it.
	perpDraw := min(amount, max(0, ctx.Client.PerpAvailable(asset)))
	spotDraw := amount - perpDraw
	if spotDraw > ctx.Client.GetAvailable(asset) {
		return errors.New("insufficient balance to repay")
	}

	oldPerp := ctx.Client.PerpBalances[asset]
	oldSpot := ctx.Client.Balances[asset]
	ctx.Client.PerpBalances[asset] -= perpDraw
	ctx.Client.Balances[asset] -= spotDraw

	oldBorrowed := ctx.Client.Borrowed[asset]
	ctx.Client.Borrowed[asset] -= amount
	// Cash returned from the spot wallet retires spot-attributed debt first;
	// clamp so the attribution never exceeds the remaining total.
	ctx.Client.BorrowedSpot[asset] = min(
		max(0, ctx.Client.BorrowedSpot[asset]-spotDraw),
		ctx.Client.Borrowed[asset],
	)

	if ctx.LogBalance != nil {
		changes := make([]BalanceDelta, 0, 3)
		if perpDraw > 0 {
			changes = append(changes, perpDelta(asset, oldPerp, ctx.Client.PerpBalances[asset]))
		}
		if spotDraw > 0 {
			changes = append(changes, spotDelta(asset, oldSpot, ctx.Client.Balances[asset]))
		}
		changes = append(changes, borrowedDelta(asset, oldBorrowed, ctx.Client.Borrowed[asset]))
		ctx.LogBalance("repay", changes)
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

	priceFor := func(asset string) (int64, error) {
		price, err := bm.Config.PriceSource.Price(asset)
		if err != nil {
			return 0, fmt.Errorf("collateral price for %s: %w", asset, err)
		}
		if price <= 0 {
			return 0, fmt.Errorf("collateral price for %s: %w", asset, ErrNoBookPrice)
		}
		return price, nil
	}

	// Gross asset value: negative balances subtract — skipping them would let
	// a client deep underwater in one asset pledge the others at full value.
	totalAssetValue := int64(0)
	for _, asset := range sortedAssetNames(client.PerpBalances) {
		balance := client.PerpBalances[asset]
		if balance == 0 {
			continue
		}
		price, err := priceFor(asset)
		if err != nil {
			return err
		}
		totalAssetValue += MulDiv(balance, price, bm.assetPrecision(asset))
	}
	for _, asset := range sortedAssetNames(client.Balances) {
		balance := client.Balances[asset]
		if balance == 0 {
			continue
		}
		price, err := priceFor(asset)
		if err != nil {
			return err
		}
		totalAssetValue += MulDiv(balance, price, bm.assetPrecision(asset))
	}

	existingBorrowValue := int64(0)
	for _, asset := range sortedAssetNames(client.Borrowed) {
		borrowed := client.Borrowed[asset]
		if borrowed <= 0 {
			continue
		}
		price, err := priceFor(asset)
		if err != nil {
			return err
		}
		existingBorrowValue += MulDiv(borrowed, price, bm.assetPrecision(asset))
	}

	borrowPrice, err := priceFor(borrowAsset)
	if err != nil {
		return fmt.Errorf("borrow asset %s: %w", borrowAsset, err)
	}
	newBorrowValue := MulDiv(borrowAmount, borrowPrice, bm.assetPrecision(borrowAsset))

	// Limit against NET equity (assets minus debt): borrowed-in cash sits in
	// the balances, so limiting against gross assets would let each borrow
	// enlarge the base for the next one — factor/(1−factor) × equity instead
	// of factor × equity.
	equity := totalAssetValue - existingBorrowValue
	if equity <= 0 {
		return errors.New("insufficient collateral")
	}
	maxBorrowValue := int64(float64(equity) * bm.getCollateralFactor(borrowAsset))

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

// assetPrecision returns units per whole asset for collateral valuation,
// defaulting to BTC precision for unconfigured assets.
func (bm *BorrowingManager) assetPrecision(asset string) int64 {
	if p := bm.Config.AssetPrecisions[asset]; p > 0 {
		return p
	}
	return btcPrecision
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

func (bm *BorrowingManager) CalculateCollateralUsed(asset string, amount int64) (int64, error) {
	if bm.Config.PriceSource == nil {
		return 0, errors.New("price oracle not configured")
	}
	price, err := bm.Config.PriceSource.Price(asset)
	if err != nil {
		return 0, fmt.Errorf("collateral price for %s: %w", asset, err)
	}
	if price <= 0 {
		return 0, fmt.Errorf("collateral price for %s: %w", asset, ErrNoBookPrice)
	}
	factor := bm.getCollateralFactor(asset)
	if factor == 0 {
		return 0, errors.New("collateral factor unavailable")
	}
	return int64(float64(MulDiv(amount, price, bm.assetPrecision(asset))) / factor), nil
}
