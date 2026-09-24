package crossvenue

import (
	"fmt"

	"exchange_sim/analysis"
)

func validateConservation(result *analysis.Conservation) error {
	if result == nil || len(result.Identities) == 0 || len(result.VenueIdentities) == 0 {
		return fmt.Errorf("ME-005 conservation: missing asset or venue identities")
	}
	deltas := result.Deltas
	if deltas.Checked == 0 || deltas.ChainChecked == 0 || deltas.Mismatched != 0 || deltas.ChainBroken != 0 ||
		deltas.DecodeFailures != 0 || deltas.MalformedVenueRecords != 0 || deltas.MalformedFeeRecords != 0 ||
		deltas.VenueBalanceMismatches != 0 || deltas.FeeRevenueMismatches != 0 || deltas.TradingFeeMismatches != 0 ||
		deltas.MarginInterestMismatches != 0 || deltas.MarginInterestFailures != 0 || deltas.FundingRemainderMismatches != 0 ||
		deltas.FundingWalletMismatches != 0 || deltas.UnsupportedRevenueRecords != 0 || deltas.MalformedInterestRecords != 0 ||
		deltas.DuplicateFeeIdentities != 0 || deltas.DuplicateFeeMovements != 0 || deltas.MalformedVenueLedgers != 0 ||
		deltas.VenueTerminalSequenceMissing != 0 || deltas.VenueOrderMismatches != 0 ||
		deltas.VenueSequenceMismatches != 0 || deltas.VenueChainMismatches != 0 || deltas.ArithmeticFailures != 0 {
		return fmt.Errorf("ME-005 conservation: movement or venue ledger mismatch: %+v", deltas)
	}
	if result.PositionRounding.Events != 0 && !result.PositionRounding.Valid {
		return fmt.Errorf("ME-005 conservation: invalid position rounding")
	}
	for _, identity := range result.Identities {
		if identity.Residual != 0 {
			return fmt.Errorf("ME-005 conservation: asset %s residual %d", identity.Asset, identity.Residual)
		}
	}
	for _, identity := range result.VenueIdentities {
		if identity.Residual != 0 {
			return fmt.Errorf("ME-005 conservation: venue %s asset %s residual %d", identity.VenueID, identity.Asset, identity.Residual)
		}
	}
	for _, instant := range result.FundingInstants {
		if instant.Net != 0 {
			return fmt.Errorf("ME-005 conservation: venue %s funding residual %d", instant.VenueID, instant.Net)
		}
	}
	return nil
}
