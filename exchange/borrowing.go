package exchange

import "errors"

type BorrowingManager struct {
	exchange *Exchange
	Config   BorrowingConfig
}

func NewBorrowingManager(exchange *Exchange, config BorrowingConfig) *BorrowingManager {
	return &BorrowingManager{
		exchange: exchange,
		Config:   config,
	}
}

func (bm *BorrowingManager) BorrowMargin(
	clientID uint64,
	asset string,
	amount int64,
	reason string,
) error {
	if !bm.Config.Enabled {
		return errors.New("borrowing disabled")
	}

	bm.exchange.mu.Lock()
	defer bm.exchange.mu.Unlock()

	client := bm.exchange.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	if client.MarginMode == CrossMargin {
		if err := bm.validateCrossMarginCollateral(client, asset, amount); err != nil {
			return err
		}
	} else {
		return errors.New("isolated margin borrow requires position context")
	}

	if limit := bm.Config.MaxBorrowPerAsset[asset]; limit > 0 {
		if client.Borrowed[asset]+amount > limit {
			return errors.New("exceeds max borrow limit")
		}
	}

	oldBorrowed := client.Borrowed[asset]
	client.Borrowed[asset] += amount

	oldPerp := client.PerpBalances[asset]
	client.PerpBalances[asset] += amount

	timestamp := bm.exchange.Clock.NowUnixNano()
	rate := bm.getRate(asset)
	collateral := bm.CalculateCollateralUsed(asset, amount)

	logBalanceChange(bm.exchange, timestamp, clientID, "", "borrow", []BalanceDelta{
		perpDelta(asset, oldPerp, client.PerpBalances[asset]),
		borrowedDelta(asset, oldBorrowed, client.Borrowed[asset]),
	})

	if log := bm.exchange.getLogger("_global"); log != nil {
		log.LogEvent(timestamp, clientID, "borrow", BorrowEvent{
			Timestamp:      timestamp,
			ClientID:       clientID,
			Asset:          asset,
			Amount:         amount,
			Reason:         reason,
			MarginMode:     client.MarginMode.String(),
			InterestRate:   rate,
			CollateralUsed: collateral,
		})
	}

	return nil
}

func (bm *BorrowingManager) RepayMargin(clientID uint64, asset string, amount int64) error {
	bm.exchange.mu.Lock()
	defer bm.exchange.mu.Unlock()

	client := bm.exchange.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	borrowed := client.Borrowed[asset]
	if borrowed == 0 {
		return errors.New("no outstanding debt")
	}

	if amount > borrowed {
		amount = borrowed
	}

	if client.PerpAvailable(asset) < amount {
		return errors.New("insufficient balance to repay")
	}

	oldPerp := client.PerpBalances[asset]
	client.PerpBalances[asset] -= amount

	oldBorrowed := client.Borrowed[asset]
	client.Borrowed[asset] -= amount

	timestamp := bm.exchange.Clock.NowUnixNano()

	logBalanceChange(bm.exchange, timestamp, clientID, "", "repay", []BalanceDelta{
		perpDelta(asset, oldPerp, client.PerpBalances[asset]),
		borrowedDelta(asset, oldBorrowed, client.Borrowed[asset]),
	})

	if log := bm.exchange.getLogger("_global"); log != nil {
		log.LogEvent(timestamp, clientID, "repay", RepayEvent{
			Timestamp:     timestamp,
			ClientID:      clientID,
			Asset:         asset,
			Principal:     amount,
			Interest:      0,
			RemainingDebt: client.Borrowed[asset],
		})
	}

	return nil
}

func (bm *BorrowingManager) AutoBorrowForSpotTrade(
	clientID uint64,
	asset string,
	required int64,
) (bool, error) {
	if !bm.Config.Enabled || !bm.Config.AutoBorrowSpot {
		return false, nil
	}

	bm.exchange.mu.RLock()
	client := bm.exchange.Clients[clientID]
	available := client.GetAvailable(asset)
	bm.exchange.mu.RUnlock()

	if available >= required {
		return false, nil
	}

	shortfall := required - available
	if err := bm.BorrowMargin(clientID, asset, shortfall, "auto_spot"); err != nil {
		return false, err
	}

	return true, nil
}

func (bm *BorrowingManager) AutoBorrowForPerpTrade(
	clientID uint64,
	asset string,
	required int64,
) (bool, error) {
	if !bm.Config.Enabled || !bm.Config.AutoBorrowPerp {
		return false, nil
	}

	bm.exchange.mu.RLock()
	client := bm.exchange.Clients[clientID]
	available := client.PerpAvailable(asset)
	bm.exchange.mu.RUnlock()

	if available >= required {
		return false, nil
	}

	shortfall := required - available
	if err := bm.BorrowMargin(clientID, asset, shortfall, "auto_perp"); err != nil {
		return false, err
	}

	return true, nil
}

func (bm *BorrowingManager) validateCrossMarginCollateral(
	client *Client,
	borrowAsset string,
	borrowAmount int64,
) error {
	if bm.Config.PriceSource == nil {
		return errors.New("price oracle not configured")
	}

	totalCollateralValue := int64(0)
	for asset, balance := range client.PerpBalances {
		if balance <= 0 {
			continue
		}
		price := bm.Config.PriceSource.Price(asset)
		if price > 0 {
			// Avoid overflow
			totalCollateralValue += (balance / btcPrecision) * price
		}
	}

	existingBorrowValue := int64(0)
	for asset, borrowed := range client.Borrowed {
		if borrowed <= 0 {
			continue
		}
		price := bm.Config.PriceSource.Price(asset)
		if price > 0 {
			// Avoid overflow
			existingBorrowValue += (borrowed / btcPrecision) * price
		}
	}

	borrowPrice := bm.Config.PriceSource.Price(borrowAsset)
	if borrowPrice == 0 {
		return errors.New("price unavailable")
	}
	// Avoid overflow
	newBorrowValue := (borrowAmount / btcPrecision) * borrowPrice

	factor := bm.getCollateralFactor(borrowAsset)

	maxBorrowValue := int64(float64(totalCollateralValue) * factor)
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
	// Avoid overflow
	borrowValue := (amount / btcPrecision) * price
	return int64(float64(borrowValue) / factor)
}
