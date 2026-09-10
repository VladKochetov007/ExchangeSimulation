package exchange

import (
	"fmt"
	"slices"
)

// ValidateNoBorrowingDebt verifies the explicit no-debt boundary used by
// strict scientific successors. It checks both the aggregate liability and
// the spot-attributed portion: a zero aggregate with a nonzero attribution
// would otherwise make a transient auto-borrow invisible to the audit.
func (e *DefaultExchange) ValidateNoBorrowingDebt() error {
	if e == nil {
		return fmt.Errorf("exchange is nil")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	clientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)
	for _, clientID := range clientIDs {
		client := e.Clients[clientID]
		if client == nil {
			return fmt.Errorf("client %d is nil", clientID)
		}
		assets := make([]string, 0, len(client.Borrowed))
		for asset := range client.Borrowed {
			assets = append(assets, asset)
		}
		slices.Sort(assets)
		for _, asset := range assets {
			if debt := client.Borrowed[asset]; debt != 0 {
				return fmt.Errorf("client %d has nonzero %s borrowing debt %d", clientID, asset, debt)
			}
		}
		assets = assets[:0]
		for asset := range client.BorrowedSpot {
			assets = append(assets, asset)
		}
		slices.Sort(assets)
		for _, asset := range assets {
			if debt := client.BorrowedSpot[asset]; debt != 0 {
				return fmt.Errorf("client %d has nonzero %s spot borrowing attribution %d", clientID, asset, debt)
			}
		}
	}
	return nil
}
