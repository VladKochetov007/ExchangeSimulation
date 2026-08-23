// Package analysis reads multivenue simulation logs and computes market-quality
// metrics over them.
//
// The logs are large: a single eight-hour run writes millions of events across
// several venues and instruments, and a campaign compares dozens of runs. The
// work is a streaming scan with per-event decoding, which is why it lives here
// rather than in a scripting language — the same pass that takes minutes
// interpreted takes seconds compiled and parallelised across files.
//
// The package is a library: metrics are ordinary functions over a Run, and a
// caller that wants a metric this package does not provide can implement it
// with Scan without modifying anything here.
package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Participant identifies one client at one venue.
type Participant struct {
	VenueID  string
	ClientID uint64
}

// Account is the subset of a logged account this package reads.
type Account struct {
	Timestamp    int64      `json:"timestamp"`
	SpotBalances []Balance  `json:"spot_balances"`
	PerpBalances []Balance  `json:"perp_balances"`
	Positions    []Position `json:"positions"`
	Equity       int64      `json:"equity"`
}

// Balance is one asset's net holding.
type Balance struct {
	Asset    string `json:"asset"`
	Borrowed int64  `json:"borrowed"`
	NetAsset int64  `json:"net_asset"`
}

// Position is one derivative position.
type Position struct {
	Symbol        string `json:"symbol"`
	Size          int64  `json:"size"`
	EntryPrice    int64  `json:"entry_price"`
	UnrealizedPnL int64  `json:"unrealized_pnl"`
}

// AccountRow is one participant's account at one phase of the run.
type AccountRow struct {
	VenueID  string           `json:"venue_id"`
	ClientID uint64           `json:"client_id"`
	Role     string           `json:"role"`
	Marks    map[string]int64 `json:"marks"`
	Account  Account          `json:"account"`
}

// Metaorder is one parent order recorded by an execution desk.
type Metaorder struct {
	ID             int     `json:"id"`
	TraderID       uint64  `json:"trader_id"`
	VenueID        string  `json:"venue_id"`
	Side           string  `json:"side"`
	ParentQty      int64   `json:"parent_qty"`
	FilledQty      int64   `json:"filled_qty"`
	StartTimestamp int64   `json:"start_timestamp"`
	EndTimestamp   int64   `json:"end_timestamp"`
	StartMid       int64   `json:"start_mid"`
	EndMid         int64   `json:"end_mid"`
	VWAP           int64   `json:"vwap"`
	ChildCount     int     `json:"child_count"`
	Completed      bool    `json:"completed"`
	Participation  float64 `json:"realized_participation"`
}

// RequestBudget is one participant's admission accounting.
type RequestBudget struct {
	VenueID     string `json:"venue_id"`
	Role        string `json:"role"`
	Admitted    int64  `json:"admitted"`
	RateLimited int64  `json:"rate_limited"`
	Overloaded  int64  `json:"overloaded"`
}

// Report is the run's summary document.
type Report struct {
	InitialAccounts  []AccountRow    `json:"initial_accounts"`
	TerminalAccounts []AccountRow    `json:"terminal_accounts"`
	Metaorders       []Metaorder     `json:"metaorders"`
	RequestBudgets   []RequestBudget `json:"request_budgets"`
	VenueLedgers     []VenueLedger   `json:"venue_ledgers"`
}

// VenueLedger is what an exchange itself holds: the fees it took and whatever
// its insurance fund absorbed. A conservation identity that ignores it reports
// every fee as value destroyed.
type VenueLedger struct {
	VenueID       string           `json:"venue_id"`
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

// Run is one simulation output directory.
type Run struct {
	Dir    string
	Report Report

	roles map[Participant]string
	files []string
}

// Open reads a run's report and indexes its event logs.
func Open(dir string) (*Run, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "greeks.json"))
	if err != nil {
		return nil, fmt.Errorf("analysis: read report: %w", err)
	}
	run := &Run{Dir: dir}
	if err := json.Unmarshal(raw, &run.Report); err != nil {
		return nil, fmt.Errorf("analysis: decode report: %w", err)
	}
	run.roles = make(map[Participant]string, len(run.Report.TerminalAccounts))
	for _, row := range run.Report.TerminalAccounts {
		run.roles[Participant{row.VenueID, row.ClientID}] = RoleGroup(row.Role)
	}
	venues := filepath.Join(dir, "venues")
	// A run logged with log_mode none has a report and no event logs. That is a
	// valid run for every report-derived metric, so a missing directory is an
	// empty file list rather than an error.
	if _, err := os.Stat(venues); os.IsNotExist(err) {
		return run, nil
	}
	if err := filepath.WalkDir(venues, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".jsonl") {
			run.files = append(run.files, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("analysis: index logs: %w", err)
	}
	sort.Strings(run.files)
	return run, nil
}

// Files returns the indexed event logs, sorted so a scan is deterministic.
func (r *Run) Files() []string { return append([]string(nil), r.files...) }

// BookFiles selects the log files belonging to one venue's book.
//
// Every metric that reads a single book needs this selection, and an empty
// result means the venue or symbol does not exist rather than "measure
// everything" — which is why callers pass FilesSelected alongside it.
func (r *Run) BookFiles(venueID, symbol string) []string {
	var files []string
	for _, path := range r.files {
		if pathHasSymbol(path, venueID, symbol) {
			files = append(files, path)
		}
	}
	return files
}

// Role returns the participant class that owns a client at a venue.
func (r *Run) Role(venueID string, clientID uint64) string {
	return r.roles[Participant{venueID, clientID}]
}

// RoleGroup collapses numbered participants of one class, so spot_maker_3
// reports as spot_maker.
func RoleGroup(role string) string {
	index := strings.LastIndex(role, "_")
	if index <= 0 || index == len(role)-1 {
		return role
	}
	for _, char := range role[index+1:] {
		if char < '0' || char > '9' {
			return role
		}
	}
	return role[:index]
}
