package exchange

import (
	"errors"
	"fmt"
	"math/bits"
	"slices"

	etypes "exchange_sim/types"
)

var ErrCollateralInterestArithmetic = errors.New("collateral interest arithmetic overflow")

const collateralInterestDenominator = int64(365 * 24 * 3600 * 10000 / 60)

const collateralInterestIntervalSeconds = int64(60)

type collateralInterestCharge struct {
	client           *Client
	clientID         uint64
	asset            string
	rate             int64
	principal        int64
	interest         int64
	remainderBefore  int64
	remainderAfter   int64
	spotShare        int64
	perpShare        int64
	oldSpot, newSpot int64
	oldPerp, newPerp int64
}

func collateralInterestAccrual(principal, rate, previousRemainder int64) (int64, int64, bool) {
	if principal <= 0 || rate < 0 || previousRemainder < 0 || previousRemainder >= collateralInterestDenominator {
		return 0, 0, false
	}
	// principal*rate can exceed int64 even though the eventual quotient is
	// representable. Keep the exact numerator in 128 bits and reject only when
	// the quotient or carry would exceed the fixed-point contract.
	high, low := bits.Mul64(uint64(principal), uint64(rate))
	low, carry := bits.Add64(low, uint64(previousRemainder), 0)
	var highCarry uint64
	high, highCarry = bits.Add64(high, 0, carry)
	if highCarry != 0 || high >= uint64(collateralInterestDenominator) {
		return 0, 0, false
	}
	quotient, remainder := bits.Div64(high, low, uint64(collateralInterestDenominator))
	if quotient > uint64(^uint64(0)>>1) {
		return 0, 0, false
	}
	return int64(quotient), int64(remainder), true
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
	proposedRemainders := make(map[uint64]map[string]int64)
	proposedTimestamps := make(map[uint64]map[string]int64)
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
			if timestamp <= 0 {
				return fmt.Errorf("%w: invalid timestamp %d", ErrCollateralInterestArithmetic, timestamp)
			}
			if lastTimestamp := e.collateralInterestLastTimestamp(clientID, asset); lastTimestamp >= timestamp {
				return fmt.Errorf("%w: client %d asset %s interval timestamp %d already consumed after %d", ErrCollateralInterestArithmetic, clientID, asset, timestamp, lastTimestamp)
			}
			previousRemainder := int64(0)
			if clientRemainders := e.collateralInterestRemainders[clientID]; clientRemainders != nil {
				previousRemainder = clientRemainders[asset]
			}
			rate := e.collateralInterestRate(asset)
			interest, remainder, ok := collateralInterestAccrual(borrowed, rate, previousRemainder)
			if !ok {
				return fmt.Errorf("%w: client %d asset %s interest", ErrCollateralInterestArithmetic, clientID, asset)
			}
			if proposedRemainders[clientID] == nil {
				proposedRemainders[clientID] = make(map[string]int64)
			}
			proposedRemainders[clientID][asset] = remainder
			if proposedTimestamps[clientID] == nil {
				proposedTimestamps[clientID] = make(map[string]int64)
			}
			proposedTimestamps[clientID][asset] = timestamp

			spotPortion := client.BorrowedSpotPortion(asset)
			if spotPortion < 0 || spotPortion > borrowed {
				return fmt.Errorf("%w: client %d asset %s debt split", ErrCollateralInterestArithmetic, clientID, asset)
			}
			spotShare, perpShare := int64(0), int64(0)
			oldSpot, newSpot := client.Balances[asset], client.Balances[asset]
			oldPerp, newPerp := client.PerpBalances[asset], client.PerpBalances[asset]
			if interest > 0 {
				spotShare, ok = etypes.TryMulDiv(interest, spotPortion, borrowed)
				if !ok {
					return fmt.Errorf("%w: client %d asset %s spot split", ErrCollateralInterestArithmetic, clientID, asset)
				}
				perpShare, ok = etypes.TrySub(interest, spotShare)
				if !ok {
					return fmt.Errorf("%w: client %d asset %s wallet split", ErrCollateralInterestArithmetic, clientID, asset)
				}

				var spotOK, perpOK bool
				newSpot, spotOK = etypes.TrySub(oldSpot, spotShare)
				newPerp, perpOK = etypes.TrySub(oldPerp, perpShare)
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
			}
			charges = append(charges, collateralInterestCharge{
				client: client, clientID: clientID, asset: asset, rate: rate, principal: borrowed,
				interest: interest, remainderBefore: previousRemainder, remainderAfter: remainder,
				spotShare: spotShare, perpShare: perpShare,
				oldSpot: oldSpot, newSpot: newSpot, oldPerp: oldPerp, newPerp: newPerp,
			})
		}
	}

	for clientID, assets := range proposedRemainders {
		if e.collateralInterestRemainders == nil {
			e.collateralInterestRemainders = make(map[uint64]map[string]int64)
		}
		if e.collateralInterestRemainders[clientID] == nil {
			e.collateralInterestRemainders[clientID] = make(map[string]int64)
		}
		for asset, remainder := range assets {
			e.collateralInterestRemainders[clientID][asset] = remainder
		}
	}
	for clientID, assets := range proposedTimestamps {
		if e.collateralInterestLastTimestamps == nil {
			e.collateralInterestLastTimestamps = make(map[uint64]map[string]int64)
		}
		if e.collateralInterestLastTimestamps[clientID] == nil {
			e.collateralInterestLastTimestamps[clientID] = make(map[string]int64)
		}
		for asset, lastTimestamp := range assets {
			e.collateralInterestLastTimestamps[clientID][asset] = lastTimestamp
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
		if charge.interest > 0 {
			e.moveVenueBalance(VenueFeeRevenue, charge.asset, charge.interest, timestamp, "", "margin_interest")
			logBalanceChange(e, timestamp, charge.client.ID, "", "interest_charge", changes)
		}
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
			log.LogEvent(timestamp, charge.client.ID, "margin_interest_accrual", MarginInterestAccrualEvent{
				Timestamp: timestamp, ClientID: charge.client.ID, Asset: charge.asset,
				IntervalSeconds: collateralInterestIntervalSeconds, Principal: charge.principal,
				RateBps: charge.rate, Interest: charge.interest, SpotInterest: charge.spotShare,
				PerpInterest: charge.perpShare, RemainderBefore: charge.remainderBefore,
				RemainderAfter: charge.remainderAfter, Denominator: collateralInterestDenominator,
			})
		}
	}
	return nil
}

func (e *DefaultExchange) collateralInterestRate(asset string) int64 {
	if e.BorrowingMgr != nil {
		// BorrowingConfig is the authority for an enabled loan: the same rate
		// must appear in the borrow receipt and every later accrual record.
		return e.BorrowingMgr.getRate(asset)
	}
	// Tests and legacy callers may inject debt without enabling the borrowing
	// manager; retain the automation-level rate for that explicit compatibility
	// path.
	return e.CollateralRate
}

func (e *DefaultExchange) collateralInterestLastTimestamp(clientID uint64, asset string) int64 {
	if timestamps := e.collateralInterestLastTimestamps[clientID]; timestamps != nil {
		return timestamps[asset]
	}
	return 0
}

// closeCollateralInterestRemainderLocked applies the explicit terminal policy
// for a debt whose sub-unit interest cannot be posted. The fraction is written
// off rather than attached to an unrelated future loan, and the event makes
// that loss visible to a replay auditor.
func (e *DefaultExchange) closeCollateralInterestRemainderLocked(clientID uint64, asset string, timestamp int64, reason string) {
	clientRemainders := e.collateralInterestRemainders[clientID]
	if clientRemainders == nil {
		return
	}
	remainder := clientRemainders[asset]
	delete(clientRemainders, asset)
	if len(clientRemainders) == 0 {
		delete(e.collateralInterestRemainders, clientID)
	}
	if remainder == 0 {
		return
	}
	if log := e.getLogger("_global"); log != nil {
		log.LogEvent(timestamp, clientID, "margin_interest_remainder_closed", MarginInterestRemainderClosedEvent{
			Timestamp: timestamp, ClientID: clientID, Asset: asset,
			RemainderBefore: remainder, RemainderAfter: 0,
			Denominator: collateralInterestDenominator, Reason: reason,
		})
	}
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
