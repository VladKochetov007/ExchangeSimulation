package analysis

import (
	"sort"
	"sync"
)

// RoleAuditOptions configures the economic-role audit.
type RoleAuditOptions struct {
	Files         []string
	FilesSelected bool
}

// RoleBehaviour is what one participant class actually did, as distinct from
// what its configuration says it is for.
//
// The distinction matters because a class can be present, funded and busy
// while doing something other than its stated job: a market maker that crosses
// the spread is a taker, an arbitrageur that trades one leg is a directional
// punter, and a hedger that never trades the hedge instrument is not hedging.
type RoleBehaviour struct {
	Role string `json:"role"`
	// Participants is how many accounts carry the role.
	Participants int `json:"participants"`
	// MakerFills and TakerFills separate liquidity provided from liquidity
	// taken. A class calling itself a maker with a high taker share is not
	// making a market in the sense its name claims.
	MakerFills int     `json:"maker_fills"`
	TakerFills int     `json:"taker_fills"`
	TakerShare float64 `json:"taker_share"`
	// Symbols is how many distinct books the class traded, and TopSymbolShare
	// the fraction of its fills in its busiest one. A desk whose job spans
	// several books and whose activity is concentrated in one is not doing
	// the job.
	Symbols        int     `json:"symbols"`
	TopSymbolShare float64 `json:"top_symbol_share"`
	// SignedQty and GrossQty give the class's directionality. A market maker
	// or an arbitrageur should be close to flat over a long run; a signed
	// share near one is a directional position wearing another name.
	SignedQty   int64   `json:"signed_qty"`
	GrossQty    int64   `json:"gross_qty"`
	SignedShare float64 `json:"signed_share"`
	// Rejected counts orders the venue refused, which is how a class that
	// cannot afford its own strategy shows up.
	Rejected int `json:"rejected"`
	// FeesPaid is what the class paid to trade, in quote units.
	FeesPaid int64 `json:"fees_paid"`
}

// RoleAudit is the population's behaviour, one row per class.
type RoleAudit struct {
	Roles []RoleBehaviour `json:"roles"`
}

// MeasureRoles reports what each participant class did.
func (r *Run) MeasureRoles(opts RoleAuditOptions) (*RoleAudit, error) {
	type fillPayload struct {
		Symbol    string `json:"symbol"`
		Qty       int64  `json:"qty"`
		Side      string `json:"side"`
		Role      string `json:"role"`
		FeeAmount int64  `json:"fee_amount"`
		FeeAsset  string `json:"fee_asset"`
	}

	var mu sync.Mutex
	type accumulator struct {
		makerFills, takerFills int
		signed, gross          int64
		rejected               int
		fees                   int64
		symbols                map[string]int
		participants           map[uint64]struct{}
	}
	perRole := make(map[string]*accumulator)
	at := func(role string) *accumulator {
		state := perRole[role]
		if state == nil {
			state = &accumulator{symbols: make(map[string]int), participants: make(map[uint64]struct{})}
			perRole[role] = state
		}
		return state
	}

	scan := ScanOptions{Events: []string{"OrderFill", "OrderRejected"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(scan, func(event Event) {
		role := r.Role(event.VenueID, event.ClientID)
		if role == "" {
			return
		}
		if event.Name == "OrderRejected" {
			mu.Lock()
			at(role).rejected++
			mu.Unlock()
			return
		}
		var fill fillPayload
		if event.Decode(&fill) != nil || fill.Qty <= 0 {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = fill.Symbol
		}
		if symbol == "" {
			symbol = symbolFromSpotFile(event.File)
		}
		signed := fill.Qty
		if fill.Side == "SELL" {
			signed = -fill.Qty
		}
		mu.Lock()
		state := at(role)
		state.participants[event.ClientID] = struct{}{}
		state.symbols[symbol]++
		state.signed += signed
		state.gross += fill.Qty
		state.fees += fill.FeeAmount
		if fill.Role == "taker" {
			state.takerFills++
		} else {
			state.makerFills++
		}
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	audit := &RoleAudit{}
	for role, state := range perRole {
		behaviour := RoleBehaviour{
			Role: role, Participants: len(state.participants),
			MakerFills: state.makerFills, TakerFills: state.takerFills,
			Symbols: len(state.symbols), SignedQty: state.signed, GrossQty: state.gross,
			Rejected: state.rejected, FeesPaid: state.fees,
		}
		if fills := state.makerFills + state.takerFills; fills > 0 {
			behaviour.TakerShare = float64(state.takerFills) / float64(fills)
			top := 0
			for _, count := range state.symbols {
				if count > top {
					top = count
				}
			}
			behaviour.TopSymbolShare = float64(top) / float64(fills)
		}
		if state.gross > 0 {
			signed := state.signed
			if signed < 0 {
				signed = -signed
			}
			behaviour.SignedShare = float64(signed) / float64(state.gross)
		}
		audit.Roles = append(audit.Roles, behaviour)
	}
	sort.Slice(audit.Roles, func(i, j int) bool { return audit.Roles[i].Role < audit.Roles[j].Role })
	return audit, nil
}
