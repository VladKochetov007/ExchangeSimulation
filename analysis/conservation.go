package analysis

import (
	"sort"
	"strings"
	"sync"
)

// ConservationOptions selects the ledger streams to audit.
type ConservationOptions struct {
	Files         []string
	FilesSelected bool
}

// BalanceDelta is one asset movement inside a logged balance change.
type balanceDeltaRecord struct {
	Asset      string `json:"asset"`
	Wallet     string `json:"wallet"`
	OldBalance int64  `json:"old_balance"`
	NewBalance int64  `json:"new_balance"`
	Delta      int64  `json:"delta"`
}

type balanceChangeRecord struct {
	Timestamp int64                `json:"timestamp"`
	ClientID  uint64               `json:"client_id"`
	Symbol    string               `json:"symbol"`
	Reason    string               `json:"reason"`
	Changes   []balanceDeltaRecord `json:"changes"`
}

// AssetFlow is the audited movement of one asset for one reason.
type AssetFlow struct {
	Reason string `json:"reason"`
	Asset  string `json:"asset"`
	// Credits and Debits are the summed positive and negative deltas.
	Credits int64 `json:"credits"`
	Debits  int64 `json:"debits"`
	// Net is Credits plus Debits. For a transfer between participants it must
	// be zero: nothing is created by two accounts moving value between them.
	Net int64 `json:"net"`
	// Records is how many balance changes contributed.
	Records int `json:"records"`
}

// DeltaConsistency counts balance changes whose reported delta disagrees with
// the difference between the balances they report.
//
// This is the cheapest possible lie detector on the ledger: a change that says
// it moved x while its own before and after differ by y is either a logging
// bug or an accounting one, and either way every downstream audit built on the
// stream inherits it.
type DeltaConsistency struct {
	Checked    int   `json:"checked"`
	Mismatched int   `json:"mismatched"`
	WorstGap   int64 `json:"worst_gap"`
}

// Conservation is the audit of a run's logged balance movements.
//
// It deliberately does not read the account snapshots the exchange reports:
// the point is to reconstruct from the movement stream and see whether the
// movements themselves are self-consistent and whether value is conserved
// where the mechanism says it must be.
type Conservation struct {
	Flows []AssetFlow `json:"flows"`
	// PerVenueNet is the net movement of each asset at each venue across every
	// reason, which is the run's external funding: deposits and borrowing in,
	// nothing else.
	PerVenueNet map[string]map[string]int64 `json:"per_venue_net"`
	Deltas      DeltaConsistency            `json:"delta_consistency"`
	// FundingInstants reports the net funding transfer at each settlement
	// instant. Funding is a transfer between longs and shorts, so each instant
	// must net to zero at a venue.
	FundingInstants []InstantResidual `json:"funding_instants"`
	// ExpiryInstants reports the same sums for settlement, and they are NOT
	// required to be zero.
	//
	// A settlement pays each holder the difference between the settlement
	// price and its own entry price, so the sum over holders is the settlement
	// price times the net open interest, which is zero, minus the sum of entry
	// prices weighted by size, which is not. The instant nets to zero only if
	// every holder entered at the same price. Reading a non-zero value here as
	// value creation is a mistake this comment exists to prevent; the identity
	// that does bind is Identities below.
	ExpiryInstants []InstantResidual `json:"expiry_instants"`

	// FeesLogged is what the venues recorded taking, per asset, from the
	// fee-revenue stream rather than from the account report. It is the
	// independent check on ExchangeTake: two records of the same money that
	// disagree mean one of them is wrong.
	FeesLogged map[string]int64 `json:"fees_logged"`
	// ClassRecords counts the balance changes each class produced, which is
	// the denominator a rounding argument needs: a residual is only rounding
	// if it is small against the number of operations that can round.
	ClassRecords map[string]int `json:"class_records"`
	// ClassNet splits the internal movements by contract class, since the
	// classes conserve differently: a spot book's cash legs cancel against its
	// fees exactly, while a perpetual's do not until every position closes.
	ClassNet map[string]map[string]int64 `json:"class_net"`

	// Identities is the audit that actually binds, per asset.
	Identities []ConservationIdentity `json:"identities"`
	// VenueIdentities is the same audit per venue, which is what localises a
	// residual: venues do not share balances, so a residual that appears at
	// one of them and not the others is a venue-local mechanism.
	VenueIdentities []VenueConservationIdentity `json:"venue_identities"`
}

// VenueConservationIdentity closes the books for one asset at one venue.
type VenueConservationIdentity struct {
	VenueID string `json:"venue_id"`
	ConservationIdentity
	// ByReason is the internal movement split by what caused it, so a residual
	// can be attributed rather than merely reported.
	ByReason map[string]int64 `json:"by_reason"`
}

// ConservationIdentity is the closed-system statement for one asset.
//
// Everything a participant holds came from outside the market or from another
// participant. Writing that as a residual:
//
//	ExternalIn + InternalNet + ExchangeTake + OpenPositionValue = 0
//
// where ExternalIn is deposits and borrowing, InternalNet is every other
// logged balance movement summed over participants, ExchangeTake is what the
// venue itself holds in fees and insurance, and OpenPositionValue is the
// unrealised profit of positions still open at the end — the cash that has not
// been paid yet because nobody has closed.
//
// Option positions are deliberately excluded from OpenPositionValue: their
// reported unrealised profit is a Black-76 mark, which is a model's opinion
// rather than a claim on anybody's cash, and folding a model number into a
// cash identity makes the identity untestable.
type ConservationIdentity struct {
	Asset            string  `json:"asset"`
	ExternalIn       int64   `json:"external_in"`
	InternalNet      int64   `json:"internal_net"`
	ExchangeTake     int64   `json:"exchange_take"`
	OpenLinearValue  int64   `json:"open_linear_value"`
	OpenOptionMark   int64   `json:"open_option_mark"`
	Residual         int64   `json:"residual"`
	ResidualRelative float64 `json:"residual_relative"`
}

// InstantResidual is the net movement at one instant, which a transfer
// mechanism requires to be zero.
type InstantResidual struct {
	VenueID   string `json:"venue_id"`
	Timestamp int64  `json:"timestamp"`
	Asset     string `json:"asset"`
	Net       int64  `json:"net"`
	Accounts  int    `json:"accounts"`
}

type flowKey struct {
	reason string
	asset  string
}

type instantKey struct {
	venue     string
	timestamp int64
	asset     string
}

// MeasureConservation audits the logged balance movements of a run.
func (r *Run) MeasureConservation(opts ConservationOptions) (*Conservation, error) {
	var mu sync.Mutex
	flows := make(map[flowKey]*AssetFlow)
	perVenue := make(map[string]map[string]int64)
	funding := make(map[instantKey]*InstantResidual)
	expiry := make(map[instantKey]*InstantResidual)
	deltas := DeltaConsistency{}

	venueFlows := make(map[string]map[flowKey]*AssetFlow)
	fees := make(map[string]int64)
	classNet := make(map[string]map[string]int64)
	classRecords := make(map[string]int)
	scan := ScanOptions{Events: []string{"balance_change", "fee_revenue"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	type feePayload struct {
		Asset    string `json:"asset"`
		TakerFee int64  `json:"taker_fee"`
		MakerFee int64  `json:"maker_fee"`
	}
	if err := r.Scan(scan, func(event Event) {
		if event.Name == "fee_revenue" {
			var payload feePayload
			if event.Decode(&payload) != nil || payload.Asset == "" {
				return
			}
			mu.Lock()
			fees[payload.Asset] += payload.TakerFee + payload.MakerFee
			mu.Unlock()
			return
		}
		var record balanceChangeRecord
		if event.Decode(&record) != nil || len(record.Changes) == 0 {
			return
		}
		class := contractClass(record.Symbol)
		instant := record.Timestamp
		if instant == 0 {
			instant = event.SimTS
		}
		mu.Lock()
		defer mu.Unlock()
		classRecords[class]++
		for _, change := range record.Changes {
			deltas.Checked++
			if gap := change.NewBalance - change.OldBalance - change.Delta; gap != 0 {
				deltas.Mismatched++
				if gap < 0 {
					gap = -gap
				}
				if gap > deltas.WorstGap {
					deltas.WorstGap = gap
				}
			}
			key := flowKey{record.Reason, change.Asset}
			flow := flows[key]
			if flow == nil {
				flow = &AssetFlow{Reason: record.Reason, Asset: change.Asset}
				flows[key] = flow
			}
			flow.Records++
			if change.Delta >= 0 {
				flow.Credits += change.Delta
			} else {
				flow.Debits += change.Delta
			}
			flow.Net += change.Delta

			if !externalReasons[record.Reason] {
				if classNet[class] == nil {
					classNet[class] = make(map[string]int64)
				}
				classNet[class][change.Asset] += change.Delta
			}

			if perVenue[event.VenueID] == nil {
				perVenue[event.VenueID] = make(map[string]int64)
			}
			perVenue[event.VenueID][change.Asset] += change.Delta

			if venueFlows[event.VenueID] == nil {
				venueFlows[event.VenueID] = make(map[flowKey]*AssetFlow)
			}
			venueFlow := venueFlows[event.VenueID][key]
			if venueFlow == nil {
				venueFlow = &AssetFlow{Reason: record.Reason, Asset: change.Asset}
				venueFlows[event.VenueID][key] = venueFlow
			}
			venueFlow.Net += change.Delta
			venueFlow.Records++

			var bucket map[instantKey]*InstantResidual
			switch record.Reason {
			case "funding_settlement":
				bucket = funding
			case "expiry_settlement":
				bucket = expiry
			default:
				continue
			}
			instantID := instantKey{event.VenueID, instant, change.Asset}
			residual := bucket[instantID]
			if residual == nil {
				residual = &InstantResidual{VenueID: event.VenueID, Timestamp: instant, Asset: change.Asset}
				bucket[instantID] = residual
			}
			residual.Net += change.Delta
			residual.Accounts++
		}
	}); err != nil {
		return nil, err
	}

	result := &Conservation{PerVenueNet: perVenue, Deltas: deltas, FeesLogged: fees, ClassNet: classNet, ClassRecords: classRecords}
	result.Identities = r.conservationIdentities(flows)
	result.VenueIdentities = r.venueIdentities(venueFlows)
	for _, flow := range flows {
		result.Flows = append(result.Flows, *flow)
	}
	sort.Slice(result.Flows, func(i, j int) bool {
		if result.Flows[i].Reason != result.Flows[j].Reason {
			return result.Flows[i].Reason < result.Flows[j].Reason
		}
		return result.Flows[i].Asset < result.Flows[j].Asset
	})
	result.FundingInstants = sortedResiduals(funding)
	result.ExpiryInstants = sortedResiduals(expiry)
	return result, nil
}

func sortedResiduals(bucket map[instantKey]*InstantResidual) []InstantResidual {
	out := make([]InstantResidual, 0, len(bucket))
	for _, residual := range bucket {
		out = append(out, *residual)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].VenueID != out[j].VenueID {
			return out[i].VenueID < out[j].VenueID
		}
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].Asset < out[j].Asset
	})
	return out
}

// WorstResidual reports the largest absolute net across a set of instants,
// which is the number an audit lives or dies by.
func WorstResidual(residuals []InstantResidual) (InstantResidual, bool) {
	worst := InstantResidual{}
	found := false
	for _, residual := range residuals {
		magnitude := residual.Net
		if magnitude < 0 {
			magnitude = -magnitude
		}
		current := worst.Net
		if current < 0 {
			current = -current
		}
		if !found || magnitude > current {
			worst, found = residual, true
		}
	}
	return worst, found
}

// externalReasons are the movements that legitimately create a participant's
// holdings out of nothing, because they come from outside the market.
var externalReasons = map[string]bool{"initial_deposit": true, "borrow": true}

// conservationIdentities closes the books per asset.
func (r *Run) conservationIdentities(flows map[flowKey]*AssetFlow) []ConservationIdentity {
	external := make(map[string]int64)
	internal := make(map[string]int64)
	for key, flow := range flows {
		if externalReasons[key.reason] {
			external[key.asset] += flow.Net
			continue
		}
		internal[key.asset] += flow.Net
	}
	take := make(map[string]int64)
	for _, ledger := range r.Report.VenueLedgers {
		for asset, amount := range ledger.FeeRevenue {
			take[asset] += amount
		}
		for asset, amount := range ledger.InsuranceFund {
			take[asset] += amount
		}
	}
	linear, optionMark := r.openPositionValue()

	assets := make(map[string]struct{})
	for asset := range external {
		assets[asset] = struct{}{}
	}
	for asset := range internal {
		assets[asset] = struct{}{}
	}
	for asset := range take {
		assets[asset] = struct{}{}
	}
	names := make([]string, 0, len(assets))
	for asset := range assets {
		names = append(names, asset)
	}
	sort.Strings(names)

	identities := make([]ConservationIdentity, 0, len(names))
	for _, asset := range names {
		identity := ConservationIdentity{
			Asset: asset, ExternalIn: external[asset], InternalNet: internal[asset],
			ExchangeTake: take[asset],
		}
		// Only the quote asset carries derivative settlement, so the open
		// value belongs to it alone.
		if asset == settlementAsset {
			identity.OpenLinearValue = linear
			identity.OpenOptionMark = optionMark
		}
		identity.Residual = identity.InternalNet + identity.ExchangeTake + identity.OpenLinearValue
		scale := identity.ExternalIn
		if scale < 0 {
			scale = -scale
		}
		if scale > 0 {
			identity.ResidualRelative = float64(identity.Residual) / float64(scale)
		}
		identities = append(identities, identity)
	}
	return identities
}

// settlementAsset is the asset derivative profit is paid in.
const settlementAsset = "USD"

// openPositionValue sums the unrealised profit of positions still open at the
// end of the run, separating the linear instruments, whose unrealised profit
// is somebody else's unpaid cash, from options, whose reported figure is a
// model mark.
func (r *Run) openPositionValue() (int64, int64) {
	linear, options := int64(0), int64(0)
	for _, row := range r.Report.TerminalAccounts {
		for _, position := range row.Account.Positions {
			if isOptionSymbol(position.Symbol) {
				options += position.UnrealizedPnL
				continue
			}
			linear += position.UnrealizedPnL
		}
	}
	return linear, options
}

// isOptionSymbol reports whether a contract is an option, by the call or put
// suffix the listing scheduler writes.
func isOptionSymbol(symbol string) bool {
	return strings.HasSuffix(symbol, "-C") || strings.HasSuffix(symbol, "-P")
}

// venueIdentities closes the books at each venue separately.
func (r *Run) venueIdentities(venueFlows map[string]map[flowKey]*AssetFlow) []VenueConservationIdentity {
	take := make(map[string]map[string]int64)
	for _, ledger := range r.Report.VenueLedgers {
		if take[ledger.VenueID] == nil {
			take[ledger.VenueID] = make(map[string]int64)
		}
		for asset, amount := range ledger.FeeRevenue {
			take[ledger.VenueID][asset] += amount
		}
		for asset, amount := range ledger.InsuranceFund {
			take[ledger.VenueID][asset] += amount
		}
	}
	openLinear := make(map[string]int64)
	for _, row := range r.Report.TerminalAccounts {
		for _, position := range row.Account.Positions {
			if isOptionSymbol(position.Symbol) {
				continue
			}
			openLinear[row.VenueID] += position.UnrealizedPnL
		}
	}

	venues := make([]string, 0, len(venueFlows))
	for venue := range venueFlows {
		venues = append(venues, venue)
	}
	sort.Strings(venues)

	var out []VenueConservationIdentity
	for _, venue := range venues {
		perAsset := make(map[string]*VenueConservationIdentity)
		for key, flow := range venueFlows[venue] {
			identity := perAsset[key.asset]
			if identity == nil {
				identity = &VenueConservationIdentity{VenueID: venue, ByReason: make(map[string]int64)}
				identity.Asset = key.asset
				perAsset[key.asset] = identity
			}
			if externalReasons[key.reason] {
				identity.ExternalIn += flow.Net
			} else {
				identity.InternalNet += flow.Net
				identity.ByReason[key.reason] += flow.Net
			}
		}
		assets := make([]string, 0, len(perAsset))
		for asset := range perAsset {
			assets = append(assets, asset)
		}
		sort.Strings(assets)
		for _, asset := range assets {
			identity := perAsset[asset]
			identity.ExchangeTake = take[venue][asset]
			if asset == settlementAsset {
				identity.OpenLinearValue = openLinear[venue]
			}
			identity.Residual = identity.InternalNet + identity.ExchangeTake + identity.OpenLinearValue
			if scale := identity.ExternalIn; scale > 0 {
				identity.ResidualRelative = float64(identity.Residual) / float64(scale)
			}
			out = append(out, *identity)
		}
	}
	return out
}

// contractClass groups a symbol by how its cash conserves.
func contractClass(symbol string) string {
	switch {
	case symbol == "":
		return "none"
	case isOptionSymbol(symbol):
		return "option"
	case strings.Contains(symbol, "-PERP"):
		return "perp"
	case strings.Contains(symbol, "-FUT-"):
		return "dated"
	default:
		return "spot"
	}
}
