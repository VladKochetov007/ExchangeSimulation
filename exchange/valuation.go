package exchange

import (
	"fmt"
	"slices"

	etypes "exchange_sim/types"
)

// MarkedAccount returns a synchronous, strict mark-to-market account report.
// It is intended for research/risk telemetry rather than order admission. The
// caller supplies every conversion into one reporting asset because the venue
// does not invent an FX graph or silently ignore a non-zero balance.
func (e *DefaultExchange) MarkedAccount(clientID uint64, spec etypes.AccountValuationSpec) (etypes.MarkedAccountSnapshot, error) {
	if spec.ReportAsset == "" || spec.ReportPrecision <= 0 {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked account requires report asset and precision")
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	client := e.Clients[clientID]
	if client == nil {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked account unknown client %d", clientID)
	}

	timestamp := e.Clock.NowUnixNano()
	balance := client.GetBalanceSnapshot(timestamp)
	spotEquity, err := valueWallet(balance.SpotBalances, spec)
	if err != nil {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked spot wallet: %w", err)
	}
	perpEquity, err := valueWallet(balance.PerpBalances, spec)
	if err != nil {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked perp wallet: %w", err)
	}
	isolateEquity, err := valueIsolated(client, spec)
	if err != nil {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked isolated collateral: %w", err)
	}

	positions := e.Positions.GetAllPositions(clientID)
	slices.SortFunc(positions, func(a, b Position) int {
		if a.Symbol != b.Symbol {
			return stringsCompare(a.Symbol, b.Symbol)
		}
		return int(a.PositionSide) - int(b.PositionSide)
	})
	positionSnapshots := make([]etypes.PositionSnapshot, 0, len(positions))
	var derivativeUPnL, optionMarketValue, maintenance int64
	for _, pos := range positions {
		book := e.Books[pos.Symbol]
		if book == nil {
			return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked position %s has no live book", pos.Symbol)
		}
		mark, err := riskMark(book.Instrument, book)
		if err != nil {
			return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked position %s: %w", pos.Symbol, err)
		}
		var unrealized int64
		var ok bool
		if _, linear := book.Instrument.(Margined); linear {
			unrealized, ok = e.tryPositionUPnL(&pos, mark, book.Instrument.BasePrecision())
		} else {
			unrealized, ok = tryPositionUPnL(&pos, mark, book.Instrument.BasePrecision())
		}
		if !ok {
			return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked position %s has unrepresentable PnL", pos.Symbol)
		}
		if _, cashPremium := book.Instrument.(PositionMarginer); cashPremium {
			marketValue, ok := etypes.TryMulDiv(pos.Size, mark, book.Instrument.BasePrecision())
			if !ok {
				return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked option %s has unrepresentable market value", pos.Symbol)
			}
			convertedValue, err := valueInReport(marketValue, book.Instrument.QuoteAsset(), spec)
			if err != nil {
				return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked option %s value: %w", pos.Symbol, err)
			}
			optionMarketValue, ok = etypes.TryAdd(optionMarketValue, convertedValue)
			if !ok {
				return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked option value overflows reporting asset")
			}
		} else {
			convertedUPnL, err := valueInReport(unrealized, book.Instrument.QuoteAsset(), spec)
			if err != nil {
				return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked position %s PnL: %w", pos.Symbol, err)
			}
			derivativeUPnL, ok = etypes.TryAdd(derivativeUPnL, convertedUPnL)
			if !ok {
				return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked derivative PnL overflows reporting asset")
			}
		}
		positionMaintenance, ok := positionMaintenanceAtMark(book.Instrument, pos.Size, mark)
		if !ok {
			return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked position %s has unrepresentable maintenance", pos.Symbol)
		}
		convertedMaintenance, err := valueInReport(positionMaintenance, book.Instrument.QuoteAsset(), spec)
		if err != nil {
			return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked position %s maintenance: %w", pos.Symbol, err)
		}
		maintenance, ok = etypes.TryAdd(maintenance, convertedMaintenance)
		if !ok {
			return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked maintenance overflows reporting asset")
		}
		positionSnapshots = append(positionSnapshots, etypes.PositionSnapshot{
			Symbol:         pos.Symbol,
			PositionSide:   pos.PositionSide,
			Size:           pos.Size,
			EntryPrice:     pos.EntryPrice,
			MarkPrice:      &mark,
			UnrealizedPnL:  unrealized,
			MarginType:     client.MarginMode,
			IsolatedMargin: pos.Margin,
		})
	}

	equity, ok := etypes.TryAdd(spotEquity, perpEquity)
	if !ok {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked wallet equity overflows reporting asset")
	}
	equity, ok = etypes.TryAdd(equity, derivativeUPnL)
	if !ok {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked equity overflows reporting asset")
	}
	equity, ok = etypes.TryAdd(equity, isolateEquity)
	if !ok {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked isolated equity overflows reporting asset")
	}
	equity, ok = etypes.TryAdd(equity, optionMarketValue)
	if !ok {
		return etypes.MarkedAccountSnapshot{}, fmt.Errorf("exchange: marked option value overflows reporting asset")
	}
	return etypes.MarkedAccountSnapshot{
		AccountSnapshot:      etypes.AccountSnapshot{BalanceSnapshot: *balance, Positions: positionSnapshots},
		ReportAsset:          spec.ReportAsset,
		ReportPrecision:      spec.ReportPrecision,
		SpotEquity:           spotEquity,
		PerpCashEquity:       perpEquity,
		IsolatedEquity:       isolateEquity,
		DerivativeUnrealized: derivativeUPnL,
		OptionMarketValue:    optionMarketValue,
		Maintenance:          maintenance,
		Equity:               equity,
	}, nil
}

func valueIsolated(client *Client, spec etypes.AccountValuationSpec) (int64, error) {
	var total int64
	symbols := make([]string, 0, len(client.IsolatedPositions))
	for symbol := range client.IsolatedPositions {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	for _, symbol := range symbols {
		isolated := client.IsolatedPositions[symbol]
		if isolated == nil {
			continue
		}
		collateralAssets := make([]string, 0, len(isolated.Collateral))
		for asset := range isolated.Collateral {
			collateralAssets = append(collateralAssets, asset)
		}
		slices.Sort(collateralAssets)
		for _, asset := range collateralAssets {
			amount := isolated.Collateral[asset]
			value, err := valueInReport(amount, asset, spec)
			if err != nil {
				return 0, err
			}
			var ok bool
			total, ok = etypes.TryAdd(total, value)
			if !ok {
				return 0, fmt.Errorf("collateral value overflows reporting asset")
			}
		}
		borrowedAssets := make([]string, 0, len(isolated.Borrowed))
		for asset := range isolated.Borrowed {
			borrowedAssets = append(borrowedAssets, asset)
		}
		slices.Sort(borrowedAssets)
		for _, asset := range borrowedAssets {
			amount := isolated.Borrowed[asset]
			value, err := valueInReport(amount, asset, spec)
			if err != nil {
				return 0, err
			}
			var ok bool
			total, ok = etypes.TrySub(total, value)
			if !ok {
				return 0, fmt.Errorf("isolated debt value overflows reporting asset")
			}
		}
	}
	return total, nil
}

func valueWallet(rows []etypes.AssetBalance, spec etypes.AccountValuationSpec) (int64, error) {
	var total int64
	for _, row := range rows {
		value, err := valueInReport(row.NetAsset, row.Asset, spec)
		if err != nil {
			return 0, err
		}
		var ok bool
		total, ok = etypes.TryAdd(total, value)
		if !ok {
			return 0, fmt.Errorf("wallet value overflows reporting asset")
		}
	}
	return total, nil
}

func valueInReport(amount int64, asset string, spec etypes.AccountValuationSpec) (int64, error) {
	if amount == 0 {
		return 0, nil
	}
	mark, ok := spec.AssetMarks[asset]
	if !ok || mark.Price <= 0 || mark.Precision <= 0 {
		return 0, fmt.Errorf("%s/%s conversion requires a positive mark: %w", asset, spec.ReportAsset, etypes.ErrPriceDomain)
	}
	value, ok := etypes.TryMulDiv(amount, mark.Price, mark.Precision)
	if !ok {
		return 0, fmt.Errorf("%s conversion overflows", asset)
	}
	return value, nil
}

// riskMark is deliberately narrower than a display midpoint. A position
// marginer (currently options) has its own risk mark; perps and dated futures
// use their stored funding mark, falling back to a live reference only before
// the first mark update. Unsupported derivative types fail closed.
func riskMark(inst Instrument, book *OrderBook) (int64, error) {
	// An initialized out-of-the-money option can have a zero premium. Its
	// atomic underlying mark distinguishes that valid zero from an option that
	// has not yet received its first derivative mark update.
	if option, ok := inst.(*EuropeanOption); ok {
		mark, err := option.PositionMark()
		if err != nil {
			return 0, fmt.Errorf("option risk mark: %w", err)
		}
		underlying, err := option.UnderlyingMark()
		if err != nil {
			return 0, fmt.Errorf("option underlying risk mark: %w", err)
		}
		if underlying <= 0 || mark < 0 {
			return 0, fmt.Errorf("option risk mark: %w", etypes.ErrPriceDomain)
		}
		return mark, nil
	}
	if pm, ok := inst.(PositionMarginer); ok {
		mark, err := pm.PositionMark()
		if err != nil {
			return 0, fmt.Errorf("position risk mark: %w", err)
		}
		return mark, nil
	}
	if perp := marginCore(inst); perp != nil {
		fundingRate := perp.GetFundingRate()
		mark := fundingRate.MarkPrice
		if fundingRate.MarkAvailable {
			return mark, nil
		}
		mark, err := liveBookReferencePrice(book)
		if err != nil {
			return 0, fmt.Errorf("perp risk mark: %w", err)
		}
		return mark, nil
	}
	return 0, fmt.Errorf("risk mark for %s: %w", inst.Symbol(), ErrNoBookPrice)
}

func positionMaintenanceAtMark(inst Instrument, size, mark int64) (int64, bool) {
	if pm, ok := inst.(PositionMarginer); ok {
		maintenance := pm.MaintenanceForPosition(size, inst.BasePrecision())
		return maintenance, maintenance >= 0
	}
	perp := marginCore(inst)
	if perp == nil {
		return 0, false
	}
	notional, ok := etypes.TryAbsMulDiv(size, mark, inst.BasePrecision())
	if !ok {
		return 0, false
	}
	return etypes.TryMulBps(notional, perp.MaintenanceMarginRate)
}

func tryPositionUPnL(pos *Position, mark, precision int64) (int64, bool) {
	return etypes.TryPriceChangeMulDiv(pos.Size, mark, pos.EntryPrice, precision)
}

func (e *DefaultExchange) tryPositionUPnL(pos *Position, mark, precision int64) (int64, bool) {
	if accounting, ok := e.Positions.(etypes.ExactLinearPositionStore); ok {
		if pnl, valid := accounting.PositionUnrealizedPnL(*pos, mark, precision); valid {
			return pnl, true
		}
		if e.requireExactLinearAccounting {
			return 0, false
		}
	}
	return tryPositionUPnL(pos, mark, precision)
}

func (e *DefaultExchange) positionUPnL(pos *Position, mark, precision int64) int64 {
	pnl, ok := e.tryPositionUPnL(pos, mark, precision)
	if !ok {
		panic("exchange: position PnL overflows int64")
	}
	return pnl
}

func (e *DefaultExchange) positionUPnLForInstrument(pos *Position, mark int64, instrument Instrument) int64 {
	if _, linear := instrument.(Margined); linear {
		return e.positionUPnL(pos, mark, instrument.BasePrecision())
	}
	return positionUPnL(pos, mark, instrument.BasePrecision())
}

func positiveMagnitude(value int64) (int64, bool) {
	if value >= 0 {
		return value, true
	}
	return etypes.TrySub(0, value)
}

func stringsCompare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
