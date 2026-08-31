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
	"bytes"
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
	Symbol string `json:"symbol"`
	// PositionSide is retained as raw JSON because the simulator's marked
	// account schema serializes the enum numerically while older reports omit it.
	// Strict audits decode it with presence preserved instead of treating absent
	// evidence as BOTH.
	PositionSide         json.RawMessage `json:"position_side"`
	Size                 int64           `json:"size"`
	EntryPrice           int64           `json:"entry_price"`
	MarkPrice            *int64          `json:"mark_price"`
	UnrealizedPnL        int64           `json:"unrealized_pnl"`
	UnrealizedPnLPresent bool            `json:"-"`
}

// UnmarshalJSON preserves whether the report actually carried unrealized_pnl.
// A missing zero-valued field is not equivalent to a measured zero in a
// strict terminal reconciliation.
func (p *Position) UnmarshalJSON(data []byte) error {
	type positionWire Position
	var decoded positionWire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*p = Position(decoded)
	pnl, present := fields["unrealized_pnl"]
	p.UnrealizedPnLPresent = present && !bytes.Equal(bytes.TrimSpace(pnl), []byte("null"))
	return nil
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
	VWAP           *int64  `json:"vwap,omitempty"`
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

	// fuse, when set, routes Scan through a fused extraction coordinator so
	// several metrics share one physical pass. It is nil for the ordinary
	// one-metric-per-process path.
	fuse *fusedPass
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
	// A run whose venue JSONL was replaced by a binary evidence stream still
	// has a venues directory, because the LogEvidenceOnly families are still
	// written there. It therefore reads as a valid but much quieter run, and
	// every metric below silently produces wrong numbers: measured against a
	// JSON run of the same seed, 26 of 32 metrics differ while exiting 0,
	// `viability` reports zero books and reads as a pass, and `conservation`
	// reports 720 broken chain links that do not exist.
	//
	// Refuse rather than analyse. A metric that cannot see the evidence must
	// say so, not report an absence of findings.
	if err := refuseUnreadableEvidence(dir); err != nil {
		return nil, err
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

// refuseUnreadableEvidence fails a run whose manifest declares an evidence
// format this package cannot read. A manifest with no such field is the JSONL
// default and is always readable, so runs written before the field existed are
// unaffected.
func refuseUnreadableEvidence(dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		// A run without a manifest predates provenance recording; it cannot be
		// in a replaced format, because that format postdates the manifest.
		return nil
	}
	var manifest struct {
		EvidenceFormat string `json:"evidence_format"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	if manifest.EvidenceFormat == "" {
		return nil
	}
	return fmt.Errorf(
		"analysis: run %s stores its execution evidence as %q, which this analyzer cannot read; "+
			"the venue JSONL holds only the evidence-only families, so every metric would report "+
			"a quieter run rather than an error", dir, manifest.EvidenceFormat)
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
