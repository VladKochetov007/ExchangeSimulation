package exchange

import (
	"fmt"
	"math"
	"slices"
)

const FundingReserveEndowmentSource = "E2_ROUNDING_RESERVE_ENDOWMENT"

// FundingReserveBalance is a venue-owned balance for one perpetual. It is not
// fee revenue and cannot be used as credit for an unmatched funding position.
type FundingReserveBalance struct {
	Asset             string `json:"asset"`
	SourceID          string `json:"source_id"`
	Initial           int64  `json:"initial"`
	Balance           int64  `json:"balance"`
	EndowmentEventSeq uint64 `json:"endowment_event_seq"`
}

type FundingReserveEndowment struct {
	VenueID             string   `json:"venue_id"`
	PerpSymbol          string   `json:"perp_symbol"`
	QuoteAsset          string   `json:"quote_asset"`
	TimestampNano       int64    `json:"timestamp_nano"`
	AccountCap          int      `json:"account_cap"`
	RegisteredClientIDs []uint64 `json:"registered_client_ids"`
	InitialQuoteAtoms   int64    `json:"initial_quote_atoms"`
	RateUnitsPerBp      int64    `json:"rate_units_per_bp"`
	SourceID            string   `json:"source_id"`
}

// FundingEndowmentReceipt binds both required records to canonical frame IDs.
// The endowment and its venue movement must be written in that order.
type FundingEndowmentReceipt struct {
	EndowmentEventSeq uint64
	MovementEventSeq  uint64
}

// FundingEndowmentAppender is injected by the evidence owner. It must append
// both records to the same required canonical stream, or return an error.
// Any failed or ambiguous append permanently invalidates this construction.
type FundingEndowmentAppender interface {
	AppendFundingEndowment(FundingReserveEndowment, VenueBalanceEvent) (FundingEndowmentReceipt, error)
}

type fundingOwnedState struct {
	roster         []uint64
	basePrecision  int64
	rateUnitsPerBp int64
	remainders     map[uint64]ScaledFundingAccrualSnapshot
}

// EndowFundingRoundingReserve fixes the complete account roster and records
// exactly K external quote atoms for K persistent identities. It is an opt-in
// construction step, not a funding payment or a way to replenish the reserve.
func (e *DefaultExchange) EndowFundingRoundingReserve(endowment FundingReserveEndowment, appender FundingEndowmentAppender) error {
	if e == nil || e.Clock == nil || appender == nil || endowment.VenueID == "" ||
		endowment.PerpSymbol == "" || endowment.QuoteAsset == "" || endowment.SourceID != FundingReserveEndowmentSource ||
		endowment.TimestampNano < 0 || endowment.AccountCap <= 0 ||
		len(endowment.RegisteredClientIDs) != endowment.AccountCap ||
		int64(endowment.AccountCap) != endowment.InitialQuoteAtoms || endowment.RateUnitsPerBp <= 0 {
		return fmt.Errorf("funding reserve: invalid finite endowment, account cap or required journal")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ExchangeBalance == nil {
		return fmt.Errorf("funding reserve: venue balance ledger is unavailable")
	}
	if failure := e.fundingEndowmentFailures[endowment.PerpSymbol]; failure != nil {
		return failure
	}
	_, alreadyEndowed := e.ExchangeBalance.FundingRoundingReserves[endowment.PerpSymbol]
	if e.ID != endowment.VenueID || e.Clock.NowUnixNano() != endowment.TimestampNano ||
		e.fundingStates[endowment.PerpSymbol] != nil ||
		alreadyEndowed || e.venueBalanceSequence == math.MaxUint64 ||
		len(e.Clients) != endowment.AccountCap {
		return fmt.Errorf("funding reserve: venue, clock, roster or one-time endowment changed")
	}
	perp := e.Instruments[endowment.PerpSymbol]
	book := e.Books[endowment.PerpSymbol]
	if !fundingInstrumentBindingMatches(perp, book) || book.Symbol != endowment.PerpSymbol ||
		perp.InstrumentType() != "PERP" || !perp.IsPerp() ||
		perp.QuoteAsset() != endowment.QuoteAsset || perp.BasePrecision() <= 0 {
		return fmt.Errorf("funding reserve: perpetual instrument or quote asset is not bound to this venue")
	}
	roster := slices.Clone(endowment.RegisteredClientIDs)
	slices.Sort(roster)
	for index, clientID := range roster {
		if index > 0 && roster[index-1] == clientID ||
			e.Clients[clientID] == nil || e.Clients[clientID].ID != clientID {
			return fmt.Errorf("funding reserve: incomplete, duplicate or misbound frozen client roster")
		}
	}
	accrual, err := NewScaledFundingAccrual(perp.BasePrecision(), endowment.RateUnitsPerBp)
	if err != nil {
		return fmt.Errorf("funding reserve: %w", err)
	}
	canonicalEndowment := endowment
	canonicalEndowment.RegisteredClientIDs = slices.Clone(roster)
	movement := VenueBalanceEvent{
		Timestamp: endowment.TimestampNano, Sequence: e.venueBalanceSequence + 1,
		Bucket: VenueFundingRoundingReserve, Asset: endowment.QuoteAsset,
		Symbol: endowment.PerpSymbol, Reason: "external_endowment",
		OldBalance: 0, NewBalance: endowment.InitialQuoteAtoms,
		Delta: endowment.InitialQuoteAtoms,
	}
	receipt, err := appender.AppendFundingEndowment(canonicalEndowment, movement)
	if err != nil || receipt.EndowmentEventSeq == 0 || receipt.MovementEventSeq <= receipt.EndowmentEventSeq {
		failure := fmt.Errorf("funding reserve: required endowment evidence failed or has no canonical sequence: %w", err)
		if err == nil {
			failure = fmt.Errorf("funding reserve: required endowment/movement frame sequence is absent or reversed")
		}
		if e.fundingEndowmentFailures == nil {
			e.fundingEndowmentFailures = make(map[string]error)
		}
		e.fundingEndowmentFailures[endowment.PerpSymbol] = failure
		return failure
	}
	if e.Clock.NowUnixNano() != endowment.TimestampNano {
		failure := fmt.Errorf("funding reserve: clock advanced during required endowment append")
		if e.fundingEndowmentFailures == nil {
			e.fundingEndowmentFailures = make(map[string]error)
		}
		e.fundingEndowmentFailures[endowment.PerpSymbol] = failure
		return failure
	}
	state := &fundingOwnedState{roster: roster, basePrecision: perp.BasePrecision(),
		rateUnitsPerBp: endowment.RateUnitsPerBp,
		remainders:     make(map[uint64]ScaledFundingAccrualSnapshot, len(roster))}
	for _, clientID := range roster {
		state.remainders[clientID] = accrual.Snapshot()
	}
	if e.fundingStates == nil {
		e.fundingStates = make(map[string]*fundingOwnedState)
	}
	e.fundingStates[endowment.PerpSymbol] = state
	if e.ExchangeBalance.FundingRoundingReserves == nil {
		e.ExchangeBalance.FundingRoundingReserves = make(map[string]FundingReserveBalance)
	}
	e.ExchangeBalance.FundingRoundingReserves[endowment.PerpSymbol] = FundingReserveBalance{
		Asset: endowment.QuoteAsset, SourceID: endowment.SourceID,
		Initial: endowment.InitialQuoteAtoms, Balance: endowment.InitialQuoteAtoms,
		EndowmentEventSeq: receipt.EndowmentEventSeq,
	}
	// The required journal already contains the 0→K movement. Do not send it
	// through the optional, errorless venue logger a second time.
	e.venueBalanceSequence = movement.Sequence
	e.conservation.recordVenue(endowment.QuoteAsset, endowment.InitialQuoteAtoms)
	return nil
}
