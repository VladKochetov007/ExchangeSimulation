package exchange

import (
	"errors"
	"fmt"
	"slices"

	etypes "exchange_sim/types"
)

var ErrCollateralInterestArithmetic = errors.New("collateral interest arithmetic overflow")

const collateralInterestDenominator = int64(365 * 24 * 3600 * 10000 / 60)

type collateralInterestCharge struct {
	client           *Client
	asset            string
	interest         int64
	spotShare        int64
	perpShare        int64
	oldSpot, newSpot int64
	oldPerp, newPerp int64
}

// ChargeCollateralInterest charges one declared minute of interest. The
// preflight keeps all client debits and the venue credit untouched until every
// representability check succeeds, so a later asset cannot observe a partial
// sweep after an earlier one fails.
func (e *DefaultExchange) ChargeCollateralInterest() {
	timestamp := e.Clock.NowUnixNano()
	e.mu.Lock()
	err := e.chargeCollateralInterestLocked(timestamp)
	e.mu.Unlock()
	if err != nil {
		e.reportCollateralInterestFailure(timestamp, err)
	}
}

func (e *DefaultExchange) chargeCollateralInterestLocked(timestamp int64) error {
	clientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)

	charges := make([]collateralInterestCharge, 0)
	proposedRevenue := make(map[string]int64)
	for _, clientID := range clientIDs {
		client := e.Clients[clientID]
		assets := make([]string, 0, len(client.Borrowed))
		for asset := range client.Borrowed {
			assets = append(assets, asset)
		}
		slices.Sort(assets)
		for _, asset := range assets {
			borrowed := client.Borrowed[asset]
			if borrowed <= 0 {
				continue
			}
			interest, ok := etypes.TryMulDiv(borrowed, e.CollateralRate, collateralInterestDenominator)
			if !ok {
				return fmt.Errorf("%w: client %d asset %s interest", ErrCollateralInterestArithmetic, clientID, asset)
			}
			if interest <= 0 {
				continue
			}

			spotPortion := client.BorrowedSpotPortion(asset)
			if spotPortion < 0 || spotPortion > borrowed {
				return fmt.Errorf("%w: client %d asset %s debt split", ErrCollateralInterestArithmetic, clientID, asset)
			}
			spotShare, ok := etypes.TryMulDiv(interest, spotPortion, borrowed)
			if !ok {
				return fmt.Errorf("%w: client %d asset %s spot split", ErrCollateralInterestArithmetic, clientID, asset)
			}
			perpShare, ok := etypes.TrySub(interest, spotShare)
			if !ok {
				return fmt.Errorf("%w: client %d asset %s wallet split", ErrCollateralInterestArithmetic, clientID, asset)
			}

			oldSpot := client.Balances[asset]
			newSpot, spotOK := etypes.TrySub(oldSpot, spotShare)
			oldPerp := client.PerpBalances[asset]
			newPerp, perpOK := etypes.TrySub(oldPerp, perpShare)
			if !spotOK || !perpOK {
				return fmt.Errorf("%w: client %d asset %s wallet balance", ErrCollateralInterestArithmetic, clientID, asset)
			}

			venueRevenue := e.ExchangeBalance.FeeRevenue[asset]
			if previous, present := proposedRevenue[asset]; present {
				venueRevenue = previous
			}
			venueRevenue, ok = etypes.TryAdd(venueRevenue, interest)
			if !ok {
				return fmt.Errorf("%w: venue revenue for %s", ErrCollateralInterestArithmetic, asset)
			}
			proposedRevenue[asset] = venueRevenue
			charges = append(charges, collateralInterestCharge{
				client: client, asset: asset, interest: interest,
				spotShare: spotShare, perpShare: perpShare,
				oldSpot: oldSpot, newSpot: newSpot, oldPerp: oldPerp, newPerp: newPerp,
			})
		}
	}

	for _, charge := range charges {
		if charge.spotShare > 0 {
			charge.client.Balances[charge.asset] = charge.newSpot
		}
		if charge.perpShare > 0 {
			charge.client.PerpBalances[charge.asset] = charge.newPerp
		}
		changes := make([]BalanceDelta, 0, 2)
		if charge.perpShare > 0 {
			changes = append(changes, perpDelta(charge.asset, charge.oldPerp, charge.newPerp))
		}
		if charge.spotShare > 0 {
			changes = append(changes, spotDelta(charge.asset, charge.oldSpot, charge.newSpot))
		}
		e.moveVenueBalance(VenueFeeRevenue, charge.asset, charge.interest, timestamp, "", "margin_interest")
		logBalanceChange(e, timestamp, charge.client.ID, "", "interest_charge", changes)
		if log := e.getLogger("_global"); log != nil {
			if charge.perpShare > 0 {
				log.LogEvent(timestamp, charge.client.ID, "margin_interest", MarginInterestEvent{
					Timestamp: timestamp, ClientID: charge.client.ID, Asset: charge.asset, Wallet: "perp", Amount: charge.perpShare,
				})
			}
			if charge.spotShare > 0 {
				log.LogEvent(timestamp, charge.client.ID, "margin_interest", MarginInterestEvent{
					Timestamp: timestamp, ClientID: charge.client.ID, Asset: charge.asset, Wallet: "spot", Amount: charge.spotShare,
				})
			}
		}
	}
	return nil
}

func (e *DefaultExchange) reportCollateralInterestFailure(timestamp int64, err error) {
	if err == nil {
		return
	}
	log := e.getLogger("_global")
	if log != nil {
		log.LogEvent(timestamp, 0, "margin_interest_failed", map[string]any{
			"timestamp": timestamp,
			"reason":    err.Error(),
		})
	}
}
