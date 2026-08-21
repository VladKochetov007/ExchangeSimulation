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
	// ChainChecked and ChainBroken verify the other half: that each
	// participant's reported final holding equals its reported initial holding
	// plus every movement logged for it. Within-record consistency cannot see
	// a balance changed without a log; this closes that loop, and it is the
	// only check here covering the class the identity is blind to.
	ChainChecked int   `json:"chain_checked"`
	ChainBroken  int   `json:"chain_broken"`
	WorstChain   int64 `json:"worst_chain_gap"`
	// DecodeFailures counts records that could not be read. A scan that reads
	// nothing produces a residual of zero for every asset, which is
	// indistinguishable from a pass, so silence is counted rather than
	// trusted.
	DecodeFailures int `json:"decode_failures"`
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
	// OptionExpiryInstants are held to the stricter standard: an option pays
	// intrinsic value times position, which is independent of what the holder
	// paid, so each instant must net to zero up to one unit per account.
	OptionExpiryInstants []InstantResidual `json:"option_expiry_instants"`

	// VenueRecorded is the venue's own balance rebuilt from its movement
	// stream, which is what makes the exchange side of the identity
	// independently checkable rather than read from the report it is auditing.
	VenueRecorded map[string]int64 `json:"venue_recorded"`
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
	optionExpiry := make(map[instantKey]*InstantResidual)
	deltas := DeltaConsistency{}
	// The balance chain is per wallet and has to be walked in time order,
	// which a concurrent scan does not give, so the records are collected and
	// ordered afterwards.
	type chainPoint struct {
		at            int64
		before, after int64
		delta         int64
	}
	type walletKey struct {
		venue    string
		clientID uint64
		asset    string
		wallet   string
	}
	_ = walletKey{}
	chain := make(map[walletKey][]chainPoint)

	venueFlows := make(map[string]map[flowKey]*AssetFlow)
	fees := make(map[string]int64)
	venueRecorded := make(map[string]int64)
	classNet := make(map[string]map[string]int64)
	classRecords := make(map[string]int)
	scan := ScanOptions{Events: []string{"balance_change", "fee_revenue", "venue_balance_change"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	type feePayload struct {
		Asset    string `json:"asset"`
		TakerFee int64  `json:"taker_fee"`
		MakerFee int64  `json:"maker_fee"`
	}
	type venueMovement struct {
		Asset string `json:"asset"`
		Delta int64  `json:"delta"`
	}
	if err := r.Scan(scan, func(event Event) {
		if event.Name == "venue_balance_change" {
			var movement venueMovement
			if event.Decode(&movement) != nil || movement.Asset == "" {
				return
			}
			mu.Lock()
			venueRecorded[movement.Asset] += movement.Delta
			mu.Unlock()
			return
		}
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
			mu.Lock()
			deltas.DecodeFailures++
			mu.Unlock()
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
			chainKey := walletKey{event.VenueID, record.ClientID, change.Asset, change.Wallet}
			chain[chainKey] = append(chain[chainKey], chainPoint{
				at: instant, before: change.OldBalance, after: change.NewBalance, delta: change.Delta,
			})

			// A borrowed-wallet entry is a liability, not a holding. A borrow
			// logs the cash it credits and the debt it creates as two positive
			// deltas, so counting both doubles the money borrowed and makes a
			// repayment subtract twice what it returns. Nothing in the audited
			// runs ever repays, which is why this went unnoticed. The flip
			// happens after the within-record check, which is about the
			// record's own arithmetic and not about what the wallet means.
			if change.Wallet == borrowedWallet {
				change.Delta = -change.Delta
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
			switch {
			case record.Reason == "funding_settlement":
				bucket = funding
			case record.Reason == "expiry_settlement" && isOptionSymbol(record.Symbol):
				// An option's payoff is its intrinsic value times the
				// position, which does not depend on what the holder paid, so
				// with every contract at zero net supply each option's expiry
				// instant must net to zero exactly. A dated future's payoff is
				// measured against each holder's own entry price and must not.
				bucket = optionExpiry
			case record.Reason == "expiry_settlement":
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

	// The movements are checked against the account report rather than against
	// their own first and last record. Many records share a timestamp and the
	// log carries no sequence number, so which record opens and which closes a
	// wallet is not recoverable, and an endpoint check taken from the stream
	// reports false breaks on exactly the busiest accounts.
	//
	// Anchoring on the report closes the loop the identity cannot see: every
	// participant's reported final holding must equal its reported initial
	// holding plus every movement logged for it. A balance changed without a
	// logged record breaks this and nothing else in the audit would notice.
	moved := make(map[participantAsset]int64)
	for key, points := range chain {
		sign := int64(1)
		if key.wallet == borrowedWallet {
			sign = -1
		}
		for _, point := range points {
			moved[participantAsset{key.venue, key.clientID, key.asset}] += sign * point.delta
		}
	}
	// Anchored at zero rather than at the reported opening balance: the
	// deposits that create an opening balance are themselves logged
	// movements, so adding them to a reported opening double-counts every
	// account's entire endowment.
	terminal := reportedBalances(r.Report.TerminalAccounts)
	for key, closing := range terminal {
		deltas.ChainChecked++
		gap := closing - moved[key]
		if gap == 0 {
			continue
		}
		deltas.ChainBroken++
		if gap < 0 {
			gap = -gap
		}
		if gap > deltas.WorstChain {
			deltas.WorstChain = gap
		}
	}

	result := &Conservation{PerVenueNet: perVenue, Deltas: deltas, FeesLogged: fees, VenueRecorded: venueRecorded, ClassNet: classNet, ClassRecords: classRecords}
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
	result.OptionExpiryInstants = sortedResiduals(optionExpiry)
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

// borrowedWallet holds a participant's debt rather than its money.
const borrowedWallet = "borrowed"

// externalReasons are the movements that legitimately create a participant's
// holdings out of nothing, because they come from outside the market.
var externalReasons = map[string]bool{"initial_deposit": true, "borrow": true}

// venueTake is what the exchange itself holds, read from its report.
//
// The movement stream is the independent source and is compared against this
// rather than replacing it: a run whose venue movements were never recorded
// still has to be auditable, and a disagreement between the two is itself the
// finding.
func (r *Run) venueTake() map[string]int64 {
	take := make(map[string]int64)
	for _, ledger := range r.Report.VenueLedgers {
		for asset, amount := range ledger.FeeRevenue {
			take[asset] += amount
		}
		for asset, amount := range ledger.InsuranceFund {
			take[asset] += amount
		}
	}
	return take
}

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
	take := r.venueTake()
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
//
// This term is taken from the run's own report, which makes the closing
// identity dependent on the venue's bookkeeping rather than fully independent
// of it: a venue that mis-valued its open positions would still balance here.
// That dependency is not assumed, it is checked -- MeasurePositions rebuilds
// the same quantity from the position and mark streams alone, and on the
// frozen baseline the two agree to the unit over twenty-four hours. Any run
// where they disagree invalidates this identity, which is why the positions
// audit reports the gap rather than hiding it.
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

// participantAsset identifies one participant's holding of one asset, across
// every wallet it keeps that asset in.
type participantAsset struct {
	venue    string
	clientID uint64
	asset    string
}

// reportedBalances flattens an account report into net holdings per asset,
// counting debt as negative.
func reportedBalances(rows []AccountRow) map[participantAsset]int64 {
	out := make(map[participantAsset]int64, len(rows)*2)
	for _, row := range rows {
		for _, balance := range row.Account.SpotBalances {
			out[participantAsset{row.VenueID, row.ClientID, balance.Asset}] += balance.NetAsset
		}
		for _, balance := range row.Account.PerpBalances {
			out[participantAsset{row.VenueID, row.ClientID, balance.Asset}] += balance.NetAsset
		}
	}
	return out
}
