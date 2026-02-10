package exchange

import "errors"

type BorrowingManager struct {
	exchange *Exchange
	config   BorrowingConfig
}

func NewBorrowingManager(exchange *Exchange, config BorrowingConfig) *BorrowingManager {
	return &BorrowingManager{
		exchange: exchange,
		config:   config,
	}
}

func (bm *BorrowingManager) BorrowMargin(
	clientID uint64,
	asset string,
	amount int64,
	reason string,
) error {
	if !bm.config.Enabled {
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

	if limit := bm.config.MaxBorrowPerAsset[asset]; limit > 0 {
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
	collateral := bm.calculateCollateralUsed(asset, amount)

	bm.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, "", "borrow", []BalanceDelta{
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

	bm.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, "", "repay", []BalanceDelta{
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
	if !bm.config.Enabled || !bm.config.AutoBorrowSpot {
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
	if !bm.config.Enabled || !bm.config.AutoBorrowPerp {
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
	if bm.config.PriceOracle == nil {
		return errors.New("price oracle not configured")
	}

	totalCollateralValue := int64(0)
	for asset, balance := range client.PerpBalances {
		if balance <= 0 {
			continue
		}
		price := bm.config.PriceOracle.GetPrice(asset)
		if price > 0 {
			totalCollateralValue += balance * price / SATOSHI
		}
	}

	existingBorrowValue := int64(0)
	for asset, borrowed := range client.Borrowed {
		if borrowed <= 0 {
			continue
		}
		price := bm.config.PriceOracle.GetPrice(asset)
		if price > 0 {
			existingBorrowValue += borrowed * price / SATOSHI
		}
	}

	borrowPrice := bm.config.PriceOracle.GetPrice(borrowAsset)
	if borrowPrice == 0 {
		return errors.New("price unavailable")
	}
	newBorrowValue := borrowAmount * borrowPrice / SATOSHI

	factor := bm.getCollateralFactor(borrowAsset)

	maxBorrowValue := int64(float64(totalCollateralValue) * factor)
	if existingBorrowValue+newBorrowValue > maxBorrowValue {
		return errors.New("insufficient collateral")
	}

	return nil
}

func (bm *BorrowingManager) getRate(asset string) int64 {
	if rate, ok := bm.config.BorrowRates[asset]; ok {
		return rate
	}
	if rate, ok := bm.config.BorrowRates["default"]; ok {
		return rate
	}
	return 500
}

func (bm *BorrowingManager) getCollateralFactor(asset string) float64 {
	if factor, ok := bm.config.CollateralFactors[asset]; ok {
		return factor
	}
	if factor, ok := bm.config.CollateralFactors["default"]; ok {
		return factor
	}
	return 0.75
}

func (bm *BorrowingManager) calculateCollateralUsed(asset string, amount int64) int64 {
	if bm.config.PriceOracle == nil {
		return 0
	}
	price := bm.config.PriceOracle.GetPrice(asset)
	if price == 0 {
		return 0
	}
	factor := bm.getCollateralFactor(asset)
	if factor == 0 {
		return 0
	}
	borrowValue := amount * price / SATOSHI
	return int64(float64(borrowValue) / factor)
}
