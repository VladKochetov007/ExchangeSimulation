package exchange_test

import (
	"errors"
	"testing"

	. "exchange_sim/exchange"
)

// H-023. validateCrossMarginCollateral values an account by summing its cash
// balances at oracle prices and subtracting its debt. It never looks at
// positions, so an unrealized loss — which has not touched the cash balance —
// is invisible to the borrow limit.
//
// The liquidation engine does look: buildAccountMarginProfile adds every
// position's unrealized PnL to equity. Two parts of the same system therefore
// disagree about what an account is worth, and the disagreement points one way,
// because the borrow gate is the more generous of the two.
type fixedPriceSource map[string]int64

func (p fixedPriceSource) Price(asset string) (int64, error) {
	if price, ok := p[asset]; ok {
		return price, nil
	}
	return 0, errors.New("no price for " + asset)
}

func TestAuditBorrowLimitIgnoresUnrealizedLosses(t *testing.T) {
	const symbol = "ABC-PERP"

	// borrowable reports the largest amount the gate will admit, found by
	// bisection, for an account holding `cash` and a position of `size` entered
	// at 100 while the mark sits at `mark`.
	borrowable := func(t *testing.T, cash int64, size int64, mark int64) int64 {
		t.Helper()
		build := func() *DefaultExchange {
			ex := NewExchange(4, &RealClock{})
			perp := NewPerpFutures(symbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
			ex.AddInstrument(perp)
			if err := perp.UpdateFundingRate(mark, mark); err != nil {
				t.Fatalf("mark: %v", err)
			}
			if err := ex.EnableBorrowing(BorrowingConfig{
				Enabled:           true,
				DefaultMarginMode: CrossMargin,
				CollateralFactors: map[string]float64{"USD": 0.5},
				MaxBorrowPerAsset: map[string]int64{"USD": 1_000_000 * USD_PRECISION},
				AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "ABC": BTC_PRECISION},
				PriceSource:       fixedPriceSource{"USD": USD_PRECISION, "ABC": mark},
			}); err != nil {
				t.Fatalf("enable borrowing: %v", err)
			}
			ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
			ex.AddPerpBalance(1, "USD", cash)
			if size != 0 {
				pm := ex.Positions.(*PositionManager)
				pm.Lock()
				pm.InjectPosition(1, symbol, &Position{
					ClientID: 1, Symbol: symbol, PositionSide: PositionBoth,
					Size: size, EntryPrice: USDAmount(100),
				})
				pm.Unlock()
			}
			return ex
		}
		low, high := int64(0), cash
		for low+1 < high {
			mid := low + (high-low)/2
			if build().BorrowMargin(1, "USD", mid, "audit") == nil {
				low = mid
			} else {
				high = mid
			}
		}
		return low
	}

	const cash = 1_000 * USD_PRECISION

	// Control: no position at all. Half of 1000 USD of equity at a 0.5 factor.
	flat := borrowable(t, cash, 0, USDAmount(100))
	t.Logf("no position: borrowable %d (%.2f USD)", flat, float64(flat)/USD_PRECISION)

	// The same cash, but the account is long 5 ABC entered at 100 with the mark
	// at 60: an unrealized loss of 5*(60-100) = -200 USD. Its economic equity is
	// 800, so a limit that saw the loss would admit 400, not 500.
	losing := borrowable(t, cash, BTCAmount(5), USDAmount(60))
	t.Logf("long 5 at 100 with the mark at 60 (unrealized -200 USD): borrowable %d (%.2f USD)",
		losing, float64(losing)/USD_PRECISION)

	if flat <= 0 {
		t.Fatalf("the control account could not borrow at all, so this fixture proves nothing")
	}
	// Pinned, not prescribed. How much leverage an account may take is
	// scientific economics; what this records is that the two engines disagree
	// and which way the disagreement points.
	if losing != flat {
		t.Errorf("the borrow limit now moves with the position (%d then %d): the disagreement this test documents "+
			"has changed and the finding needs re-reading", flat, losing)
	}
	t.Logf("both borrow %.2f USD; an engine that counted the 200 USD loss would admit 400.00",
		float64(flat)/USD_PRECISION)
}

// The same omission from the other side. A reservation is an earmark inside
// PerpBalances, and the gate sums PerpBalances, so capital already committed to
// a resting order is counted again as free collateral.
func TestAuditBorrowLimitIgnoresCommittedCollateral(t *testing.T) {
	const symbol = "ABC-PERP"
	const cash = 1_000 * USD_PRECISION

	build := func(t *testing.T) *DefaultExchange {
		t.Helper()
		ex := NewExchange(4, &RealClock{})
		perp := NewPerpFutures(symbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
		ex.AddInstrument(perp)
		if err := perp.UpdateFundingRate(USDAmount(100), USDAmount(100)); err != nil {
			t.Fatalf("mark: %v", err)
		}
		if err := ex.EnableBorrowing(BorrowingConfig{
			Enabled:           true,
			DefaultMarginMode: CrossMargin,
			CollateralFactors: map[string]float64{"USD": 0.5},
			MaxBorrowPerAsset: map[string]int64{"USD": 1_000_000 * USD_PRECISION},
			AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "ABC": BTC_PRECISION},
			PriceSource:       fixedPriceSource{"USD": USD_PRECISION, "ABC": USDAmount(100)},
		}); err != nil {
			t.Fatalf("enable borrowing: %v", err)
		}
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(1, "USD", cash)
		ex.AddPerpBalance(2, "USD", cash)
		return ex
	}

	// Control: nothing committed.
	free := build(t)
	if err := free.BorrowMargin(1, "USD", 500*USD_PRECISION, "audit"); err != nil {
		t.Fatalf("the control account could not borrow half its equity: %v", err)
	}

	// The same account, but with a resting bid that has earmarked most of the
	// cash. PerpAvailable is what the account can actually spend.
	committed := build(t)
	resp := committed.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: symbol, Side: Buy, Type: LimitOrder,
		Price: USDAmount(100), Qty: BTCAmount(90), TimeInForce: GTC,
	})
	if !resp.Success {
		t.Fatalf("resting order rejected: %#v", resp)
	}
	reserved := committed.Clients[1].PerpReserved["USD"]
	available := committed.Clients[1].PerpAvailable("USD")
	t.Logf("cash %d, reserved %d, available %d", cash, reserved, available)
	if reserved <= cash/2 {
		t.Fatalf("the resting order committed only %d of %d; this fixture needs most of the cash earmarked",
			reserved, cash)
	}

	if err := committed.BorrowMargin(1, "USD", 500*USD_PRECISION, "audit"); err != nil {
		t.Errorf("the gate now refuses a committed account (%v): the behaviour this test documents has changed", err)
	} else {
		t.Logf("an account with only %.2f USD actually available borrowed 500.00 USD: the same capital backs "+
			"the resting order and the loan at once", float64(available)/USD_PRECISION)
	}
}
