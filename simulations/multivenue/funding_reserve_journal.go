package multivenue

import (
	"fmt"
	"math"

	"exchange_sim/exchange"
)

const fundingReserveRoute = "derivatives.jsonl"

// AppendFundingEndowment records the external source and its 0→K venue
// movement in one ordered canonical stream. It is an opt-in adapter: the
// historical venue logger and funding path never call it.
func (l venueLogger) AppendFundingEndowment(endowment exchange.FundingReserveEndowment, movement exchange.VenueBalanceEvent) (exchange.FundingEndowmentReceipt, error) {
	if l.sequenceMu == nil || l.sequence == nil || l.sink == nil ||
		!l.sink.includesEvidenceOnly() || l.route != fundingReserveRoute ||
		l.venueID != endowment.VenueID || endowment.TimestampNano != movement.Timestamp ||
		movement.Bucket != exchange.VenueFundingRoundingReserve ||
		movement.Symbol != endowment.PerpSymbol || movement.Asset != endowment.QuoteAsset ||
		movement.Reason != "external_endowment" || movement.OldBalance != 0 ||
		movement.Delta != endowment.InitialQuoteAtoms || movement.NewBalance != endowment.InitialQuoteAtoms {
		return exchange.FundingEndowmentReceipt{}, fmt.Errorf("funding reserve endowment has no matching required canonical venue route")
	}
	l.sequenceMu.Lock()
	defer l.sequenceMu.Unlock()
	if *l.sequence > math.MaxUint64-2 {
		return exchange.FundingEndowmentReceipt{}, fmt.Errorf("funding reserve endowment would overflow venue event sequence")
	}
	(*l.sequence)++
	endowmentFrame, err := l.sink.observeRequiredCanonicalEvent(endowment.TimestampNano,
		"funding_reserve_endowment", l.venueID, endowment, l.route, *l.sequence)
	if err != nil {
		return exchange.FundingEndowmentReceipt{}, err
	}
	(*l.sequence)++
	movementFrame, err := l.sink.observeRequiredCanonicalEvent(movement.Timestamp,
		"venue_balance_change", l.venueID, movement, l.route, *l.sequence)
	if err != nil {
		return exchange.FundingEndowmentReceipt{}, err
	}
	return exchange.FundingEndowmentReceipt{EndowmentEventSeq: endowmentFrame, MovementEventSeq: movementFrame}, nil
}
