package repeatedspot

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

type MeasurementWindow struct {
	StartAt int64 `json:"start_at_ns"`
	EndAt   int64 `json:"end_at_ns"`
}

type InventoryRiskSummary struct {
	WindowNanos              int64  `json:"window_ns"`
	SignedBaseUnitNanos      string `json:"signed_base_unit_ns"`
	AbsoluteBaseUnitNanos    string `json:"absolute_base_unit_ns"`
	SquaredBaseUnitNanos     string `json:"squared_base_unit_ns"`
	MaxAbsoluteBaseUnits     string `json:"max_absolute_base_units"`
	TimeAtFilledLimitNanos   int64  `json:"time_at_filled_limit_ns"`
	TerminalNetFillBaseUnits int64  `json:"terminal_net_fill_base_units"`
}

type AccountInventoryRisk struct {
	Exchange InventoryRiskSummary  `json:"exchange"`
	Local    *InventoryRiskSummary `json:"maker_local,omitempty"`
}

type inventoryRiskSeries struct {
	window         MeasurementWindow
	lastAt         int64
	current        int64
	workingLimit   int64
	signed         big.Int
	absolute       big.Int
	squared        big.Int
	maximum        big.Int
	atWorkingLimit int64
}

func newInventoryRiskSeries(startAt int64, window MeasurementWindow, workingLimit int64) *inventoryRiskSeries {
	return &inventoryRiskSeries{window: window, lastAt: startAt, workingLimit: workingLimit}
}

func (series *inventoryRiskSeries) advance(at int64) error {
	if at < series.lastAt {
		return errors.New("repeated spot: inventory risk time regressed")
	}
	from := max(series.lastAt, series.window.StartAt)
	until := min(at, series.window.EndAt)
	series.lastAt = at
	if until <= from {
		return nil
	}
	duration := until - from
	level := big.NewInt(series.current)
	absLevel := new(big.Int).Abs(level)
	series.signed.Add(&series.signed, new(big.Int).Mul(level, big.NewInt(duration)))
	series.absolute.Add(&series.absolute, new(big.Int).Mul(absLevel, big.NewInt(duration)))
	series.squared.Add(&series.squared, new(big.Int).Mul(new(big.Int).Mul(level, level), big.NewInt(duration)))
	if absLevel.Cmp(&series.maximum) > 0 {
		series.maximum.Set(absLevel)
	}
	if series.workingLimit > 0 && absLevel.Cmp(big.NewInt(series.workingLimit)) >= 0 {
		series.atWorkingLimit += duration
	}
	return nil
}

func (series *inventoryRiskSeries) add(at, delta int64) error {
	if err := series.advance(at); err != nil {
		return err
	}
	updated, ok := checkedAdd(series.current, delta)
	if !ok {
		return errors.New("repeated spot: inventory risk fill overflows")
	}
	series.current = updated
	return nil
}

func (series *inventoryRiskSeries) finish(at int64) (InventoryRiskSummary, error) {
	if err := series.advance(at); err != nil {
		return InventoryRiskSummary{}, err
	}
	return InventoryRiskSummary{WindowNanos: series.window.EndAt - series.window.StartAt,
		SignedBaseUnitNanos: series.signed.String(), AbsoluteBaseUnitNanos: series.absolute.String(),
		SquaredBaseUnitNanos: series.squared.String(), MaxAbsoluteBaseUnits: series.maximum.String(),
		TimeAtFilledLimitNanos: series.atWorkingLimit, TerminalNetFillBaseUnits: series.current}, nil
}

func (state *replayState) exchangeInventoryFill(event Event) error {
	var fill recordedFill
	if err := json.Unmarshal(event.Payload, &fill); err != nil {
		return err
	}
	account := state.accounts[event.ClientID]
	if account == nil || fill.Qty <= 0 || !validSide(fill.Side) {
		return errors.New("repeated spot: exchange fill has invalid inventory owner or side")
	}
	delta := fill.Qty
	if fill.Side == "SELL" {
		delta = -delta
	}
	return account.exchangeRisk.add(event.Timestamp, delta)
}

func (state *replayState) finishInventoryRisk(terminalAt int64) error {
	baseAsset := state.contract.Instrument.BaseAsset
	for _, account := range state.accounts {
		exchangeRisk, err := account.exchangeRisk.finish(terminalAt)
		if err != nil {
			return err
		}
		if exchangeRisk.TerminalNetFillBaseUnits != account.current[baseAsset]-account.initial[baseAsset] {
			return fmt.Errorf("repeated spot: client %d exchange fill inventory disagrees with base ledger", account.clientID)
		}
		account.inventoryRisk.Exchange = exchangeRisk
		if account.localRisk == nil {
			continue
		}
		localRisk, err := account.localRisk.finish(terminalAt)
		if err != nil {
			return err
		}
		if localRisk.TerminalNetFillBaseUnits != account.localResponses.NetProcessedFillBase {
			return fmt.Errorf("repeated spot: maker %d processed inventory risk disagrees with local responses", account.clientID)
		}
		account.inventoryRisk.Local = &localRisk
	}
	return nil
}
