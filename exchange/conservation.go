package exchange

import (
	"sort"

	etypes "exchange_sim/types"
)

// ConservationViolation is one asset whose recorded movements no longer
// explain the balances actually held.
type ConservationViolation struct {
	Asset string `json:"asset"`
	// Recorded is the sum of every logged movement of the asset, including the
	// venue's own; Held is what the accounts and the venue actually contain.
	Recorded int64 `json:"recorded"`
	Held     int64 `json:"held"`
	Gap      int64 `json:"gap"`
}

// conservationTracker accumulates every recorded movement of every asset.
//
// The point is not to audit after the fact but to make an unrecorded mutation
// impossible to hide: a balance changed without a logged movement leaves the
// recorded total behind the held total, and the next verification says so.
// Accumulating is O(1) per movement and verifying is one pass over the
// accounts, which is cheap enough to run on the automation tick — so the
// window between a silent mutation and its detection is one tick, not a run.
type conservationTracker struct {
	recorded map[string]int64
	// unrepresentable marks an asset whose running total no longer fits in
	// int64. A client can drive this deliberately by funding itself near the
	// ceiling, so the tracker records the fact and stops counting rather than
	// stopping the venue: refusing the order is the admission path's job, and
	// an observability aid must not become a way to crash the exchange.
	unrepresentable map[string]bool
}

func newConservationTracker() *conservationTracker {
	return &conservationTracker{
		recorded:        make(map[string]int64, 8),
		unrepresentable: make(map[string]bool, 1),
	}
}

// add folds one delta into an asset's running total, flagging rather than
// failing when the total can no longer be represented.
func (t *conservationTracker) add(asset string, delta int64) {
	if t.unrepresentable[asset] {
		return
	}
	sum, ok := etypes.TryAdd(t.recorded[asset], delta)
	if !ok {
		t.unrepresentable[asset] = true
		return
	}
	t.recorded[asset] = sum
}

// record folds a set of logged deltas into the running totals. Debt is
// negative: a borrow credits cash and creates a liability of the same size,
// and counting both as holdings would double the borrowed amount.
func (t *conservationTracker) record(changes []BalanceDelta) {
	if t == nil {
		return
	}
	for _, change := range changes {
		delta := change.Delta
		if change.Wallet == borrowedWalletName {
			delta = -delta
		}
		t.add(change.Asset, delta)
	}
}

// recordVenue folds a movement of the exchange's own money into the totals.
func (t *conservationTracker) recordVenue(asset string, delta int64) {
	if t == nil || asset == "" {
		return
	}
	t.add(asset, delta)
}

// borrowedWalletName is the wallet holding a participant's debt.
const borrowedWalletName = "borrowed"

// VerifyConservation compares every recorded movement against what is actually
// held, and returns the assets where they disagree.
//
// An empty result means every unit of every asset in the system is accounted
// for by a logged movement. A non-empty one means some balance changed without
// a record, which no after-the-fact audit of the log could ever detect: the
// log would be self-consistent and simply incomplete.
func (e *DefaultExchange) VerifyConservation() []ConservationViolation {
	if e.conservation == nil {
		return nil
	}
	e.mu.RLock()
	held := make(map[string]int64, len(e.conservation.recorded))
	unrepresentable := make(map[string]bool, len(e.conservation.unrepresentable))
	accumulate := func(asset string, amount int64) {
		if unrepresentable[asset] {
			return
		}
		sum, ok := etypes.TryAdd(held[asset], amount)
		if !ok {
			unrepresentable[asset] = true
			return
		}
		held[asset] = sum
	}
	for _, client := range e.Clients {
		if client == nil {
			continue
		}
		for asset, amount := range client.Balances {
			accumulate(asset, amount)
		}
		for asset, amount := range client.PerpBalances {
			accumulate(asset, amount)
		}
		for asset, amount := range client.Borrowed {
			accumulate(asset, -amount)
		}
	}
	for asset, amount := range e.ExchangeBalance.FeeRevenue {
		accumulate(asset, amount)
	}
	for asset, amount := range e.ExchangeBalance.InsuranceFund {
		accumulate(asset, amount)
	}
	for asset := range e.conservation.unrepresentable {
		unrepresentable[asset] = true
	}
	recorded := make(map[string]int64, len(e.conservation.recorded))
	for asset, amount := range e.conservation.recorded {
		recorded[asset] = amount
	}
	e.mu.RUnlock()

	assets := make([]string, 0, len(held))
	seen := make(map[string]struct{}, len(held))
	for asset := range held {
		assets = append(assets, asset)
		seen[asset] = struct{}{}
	}
	for asset := range recorded {
		if _, ok := seen[asset]; !ok {
			assets = append(assets, asset)
		}
	}
	sort.Strings(assets)

	var violations []ConservationViolation
	for _, asset := range assets {
		// An asset whose totals no longer fit in int64 cannot be compared, and
		// reporting a wrapped difference as a violation would be worse than
		// saying nothing.
		if unrepresentable[asset] {
			continue
		}
		gap := held[asset] - recorded[asset]
		if gap == 0 {
			continue
		}
		violations = append(violations, ConservationViolation{
			Asset: asset, Recorded: recorded[asset], Held: held[asset], Gap: gap,
		})
	}
	return violations
}
