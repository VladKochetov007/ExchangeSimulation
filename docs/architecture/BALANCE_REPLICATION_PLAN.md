# Balance Replication Enhancement Plan

**Status**: Planned
**Date**: 2026-02-15
**Priority**: P1 (Client-side balance verification capability)

## Problem Statement

Clients need the ability to replicate their balance state independently by consuming event logs. Currently, clients cannot fully verify their balances because:

1. **Gap 1: Missing Reserved Balance Deltas**
   - Reserve/Release operations during order placement/cancellation don't log balance change events
   - Clients can track `Total` but not `Reserved`, breaking: `Available = Total - Reserved`
   - 14+ Reserve/Release call sites in exchange.go don't log

2. **Gap 2: No Sequence Numbers**
   - Events have timestamps but no monotonic sequence numbers
   - Cannot detect missing events (gaps) or duplicates
   - No mechanism for gap detection or event ordering guarantees

3. **Gap 3: No Integrity Verification** (RESOLVED ✅)
   - ~~GetBalanceSnapshot incomplete~~ FIXED: Now returns complete snapshot with spot, perp, borrowed
   - Still missing: event checksums for log integrity verification

4. **Gap 4: No Event Checksum/Hash**
   - No way to verify log integrity (detect corruption, tampering, incomplete writes)
   - Cannot validate event stream hasn't been modified

5. **Gap 5: Symbol vs Client Event Routing**
   - Some events logged to symbol loggers, others to global
   - Client must read multiple log files to reconstruct state

## Design Principles (per CLAUDE.md)

**Library-First Architecture:**
- Users cannot modify library source code
- All customization via dependency injection, interfaces, callbacks
- Open for extension, closed for modification
- Everything configurable, no hard-coded decisions

**Implementation Requirements:**
- Non-breaking changes only
- Injectable interfaces for extensibility
- Default implementations provided
- No changes to existing event structures (add new optional fields)

## Proposed Solution

### Phase 1: Sequence Numbers & Checksums Infrastructure

**Add to `exchange/types.go`:**

```go
// BalanceChangeEventV2 extends BalanceChangeEvent with sequence numbers and checksums
type BalanceChangeEventV2 struct {
	BalanceChangeEvent
	SequenceNum uint64 `json:"sequence_num,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
}
```

**Create `exchange/sequence.go`:**

```go
// SequenceProvider generates monotonic sequence numbers per client
type SequenceProvider interface {
	Next(clientID uint64) uint64
	Reset(clientID uint64)
}

// PerClientSequencer maintains independent sequences per client
type PerClientSequencer struct {
	mu        sync.Mutex
	sequences map[uint64]uint64
}

func NewPerClientSequencer() *PerClientSequencer {
	return &PerClientSequencer{sequences: make(map[uint64]uint64)}
}

func (s *PerClientSequencer) Next(clientID uint64) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequences[clientID]++
	return s.sequences[clientID]
}

func (s *PerClientSequencer) Reset(clientID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sequences, clientID)
}
```

**Create `exchange/checksum.go`:**

```go
// BalanceChecksumCalculator computes integrity checksums for balance events
type BalanceChecksumCalculator interface {
	Calculate(event *BalanceChangeEvent, seqNum uint64) string
}

// SHA256BalanceChecksum implements checksum using SHA256
type SHA256BalanceChecksum struct{}

func (c *SHA256BalanceChecksum) Calculate(event *BalanceChangeEvent, seqNum uint64) string {
	// Hash: clientID + timestamp + seqNum + sum(delta amounts)
	h := sha256.New()
	binary.Write(h, binary.LittleEndian, event.ClientID)
	binary.Write(h, binary.LittleEndian, event.Timestamp)
	binary.Write(h, binary.LittleEndian, seqNum)
	for _, delta := range event.Changes {
		binary.Write(h, binary.LittleEndian, delta.Delta)
	}
	return hex.EncodeToString(h.Sum(nil))
}
```

**Update `ExchangeConfig`:**

```go
type ExchangeConfig struct {
	MaxClients         int
	Clock              Clock
	SequenceProvider   SequenceProvider           // Optional, defaults to PerClientSequencer
	ChecksumCalculator BalanceChecksumCalculator  // Optional, defaults to SHA256BalanceChecksum
}
```

### Phase 2: Log Reserved Balance Changes

**Add helper in `exchange/balance_logger.go`:**

```go
func (t *BalanceChangeTracker) LogReserveChange(
	timestamp int64,
	clientID uint64,
	symbol string,
	asset string,
	wallet string,
	reserveDelta int64,
	reason string,
) {
	if t.logger == nil {
		return
	}

	// Log the change in reserved balance (available changes inversely)
	event := BalanceChangeEvent{
		Timestamp: timestamp,
		ClientID:  clientID,
		Symbol:    symbol,
		Reason:    reason,
		Changes: []BalanceDelta{
			{
				Asset:      asset,
				Wallet:     wallet,
				OldBalance: 0, // Reserved balance, not total
				NewBalance: reserveDelta,
				Delta:      reserveDelta,
			},
		},
	}

	// Add sequence number and checksum if providers configured
	if t.seqProvider != nil {
		seqNum := t.seqProvider.Next(clientID)
		// Use extended event type
		eventV2 := BalanceChangeEventV2{
			BalanceChangeEvent: event,
			SequenceNum:        seqNum,
		}
		if t.checksumCalc != nil {
			eventV2.Checksum = t.checksumCalc.Calculate(&event, seqNum)
		}
		t.logger.LogEvent(timestamp, clientID, "balance_change_v2", eventV2)
	} else {
		t.logger.LogEvent(timestamp, clientID, "balance_change", event)
	}
}
```

**Update all Reserve/Release call sites** (14+ locations):

```go
// Example for order placement:
if !client.ReservePerp(instrument.QuoteAsset(), initialMargin) {
	// ... borrowing logic ...
}
// After reserve:
if tracker := e.BalanceTracker; tracker != nil {
	tracker.LogReserveChange(
		e.Clock.NowUnixNano(),
		clientID,
		req.Symbol,
		instrument.QuoteAsset(),
		"perp",
		initialMargin,
		"order_reserve",
	)
}
```

### Phase 3: Client-Side Verification Tools

**Create `exchange/balance_verifier.go`:**

```go
// BalanceReplicator reconstructs client balance from event log
type BalanceReplicator struct {
	spotBalances map[string]int64
	spotReserved map[string]int64
	perpBalances map[string]int64
	perpReserved map[string]int64
	borrowed     map[string]int64

	lastSeqNum   uint64
	checksumCalc BalanceChecksumCalculator
}

func NewBalanceReplicator(initialSnapshot *BalanceSnapshot) *BalanceReplicator {
	r := &BalanceReplicator{
		spotBalances: make(map[string]int64),
		spotReserved: make(map[string]int64),
		perpBalances: make(map[string]int64),
		perpReserved: make(map[string]int64),
		borrowed:     make(map[string]int64),
	}

	// Initialize from snapshot
	for _, bal := range initialSnapshot.SpotBalances {
		r.spotBalances[bal.Asset] = bal.Total
		r.spotReserved[bal.Asset] = bal.Reserved
	}
	for _, bal := range initialSnapshot.PerpBalances {
		r.perpBalances[bal.Asset] = bal.Total
		r.perpReserved[bal.Asset] = bal.Reserved
	}
	for asset, amount := range initialSnapshot.Borrowed {
		r.borrowed[asset] = amount
	}

	return r
}

// ApplyEvent processes a balance change event
func (r *BalanceReplicator) ApplyEvent(event *BalanceChangeEventV2) error {
	// Verify sequence number
	if event.SequenceNum != 0 && event.SequenceNum != r.lastSeqNum+1 {
		return fmt.Errorf("gap detected: expected seq %d, got %d", r.lastSeqNum+1, event.SequenceNum)
	}

	// Verify checksum if present
	if event.Checksum != "" && r.checksumCalc != nil {
		expected := r.checksumCalc.Calculate(&event.BalanceChangeEvent, event.SequenceNum)
		if expected != event.Checksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, event.Checksum)
		}
	}

	// Apply balance changes
	for _, delta := range event.Changes {
		switch delta.Wallet {
		case "spot":
			r.spotBalances[delta.Asset] += delta.Delta
		case "perp":
			r.perpBalances[delta.Asset] += delta.Delta
		case "spot_reserved":
			r.spotReserved[delta.Asset] += delta.Delta
		case "perp_reserved":
			r.perpReserved[delta.Asset] += delta.Delta
		case "borrowed":
			r.borrowed[delta.Asset] += delta.Delta
		}
	}

	r.lastSeqNum = event.SequenceNum
	return nil
}

// Verify compares replicated state against a snapshot
func (r *BalanceReplicator) Verify(snapshot *BalanceSnapshot) error {
	// Compare spot balances
	for _, bal := range snapshot.SpotBalances {
		if r.spotBalances[bal.Asset] != bal.Total {
			return fmt.Errorf("spot %s mismatch: expected %d, got %d", bal.Asset, bal.Total, r.spotBalances[bal.Asset])
		}
		if r.spotReserved[bal.Asset] != bal.Reserved {
			return fmt.Errorf("spot reserved %s mismatch: expected %d, got %d", bal.Asset, bal.Reserved, r.spotReserved[bal.Asset])
		}
	}

	// Compare perp balances
	for _, bal := range snapshot.PerpBalances {
		if r.perpBalances[bal.Asset] != bal.Total {
			return fmt.Errorf("perp %s mismatch: expected %d, got %d", bal.Asset, bal.Total, r.perpBalances[bal.Asset])
		}
		if r.perpReserved[bal.Asset] != bal.Reserved {
			return fmt.Errorf("perp reserved %s mismatch: expected %d, got %d", bal.Asset, bal.Reserved, r.perpReserved[bal.Asset])
		}
	}

	// Compare borrowed
	for asset, amount := range snapshot.Borrowed {
		if r.borrowed[asset] != amount {
			return fmt.Errorf("borrowed %s mismatch: expected %d, got %d", asset, amount, r.borrowed[asset])
		}
	}

	return nil
}
```

### Phase 4: Extension Points for Custom Wallets

**Library-first design:** Users can add custom wallet types without modifying library code.

**Add to `exchange/types.go`:**

```go
// WalletType identifies the wallet in a balance change
type WalletType string

const (
	WalletSpot         WalletType = "spot"
	WalletPerp         WalletType = "perp"
	WalletSpotReserved WalletType = "spot_reserved"
	WalletPerpReserved WalletType = "perp_reserved"
	WalletBorrowed     WalletType = "borrowed"
)

// BalanceDeltaV2 uses WalletType for extensibility
type BalanceDeltaV2 struct {
	Asset      string     `json:"asset"`
	Wallet     WalletType `json:"wallet"`
	OldBalance int64      `json:"old_balance"`
	NewBalance int64      `json:"new_balance"`
	Delta      int64      `json:"delta"`
}
```

**Users can define custom wallets:**

```go
const (
	WalletCustomStaking WalletType = "staking"
	WalletCustomRewards WalletType = "rewards"
)
```

### Phase 5: Documentation

**Create `docs/observability/balance-replication.md`:**
- How to use BalanceReplicator
- Gap detection protocol
- Checksum verification
- Custom wallet types
- Example: replaying logs to verify balances

**Update `docs/core-concepts/exchange-architecture.md`:**
- Balance event versioning (v1 vs v2)
- Sequence number guarantees
- Checksum algorithm

## Testing Strategy

**Unit Tests:**
- `TestPerClientSequencer`: sequence generation, reset, concurrency
- `TestSHA256BalanceChecksum`: checksum calculation, determinism
- `TestBalanceReplicator`: apply events, verify state, detect gaps
- `TestReserveLogging`: verify all Reserve/Release operations log

**Integration Tests:**
- `TestEndToEndBalanceReplication`: place orders, execute trades, verify replication
- `TestGapDetection`: simulate missing events, verify error
- `TestChecksumFailure`: corrupt event, verify detection
- `TestCustomWalletTypes`: extend with custom wallets, verify compatibility

## Migration Path (Non-Breaking)

1. **Phase 1**: Add optional fields to events (backward compatible)
2. **Phase 2**: ExchangeConfig uses default providers if not set (backward compatible)
3. **Phase 3**: Existing logs remain valid (v1 events still work)
4. **Phase 4**: Users opt-in to v2 events by configuring SequenceProvider
5. **Phase 5**: Documentation shows both v1 (simple) and v2 (verified) approaches

## Benefits

**For Users:**
- Independent balance verification (trust but verify)
- Detect missing events (gap detection)
- Detect log corruption (checksums)
- Audit trail for regulatory compliance
- Custom wallet types without library changes

**For Library:**
- Maintains library-first principles (injectable interfaces)
- Non-breaking changes (backward compatible)
- Extensible for future wallet types
- Testable and verifiable

## Implementation Checklist

- [ ] Phase 1: Add SequenceProvider and BalanceChecksumCalculator interfaces
- [ ] Phase 1: Update ExchangeConfig with optional providers
- [ ] Phase 2: Add LogReserveChange to BalanceChangeTracker
- [ ] Phase 2: Update all 14+ Reserve/Release call sites
- [ ] Phase 3: Implement BalanceReplicator
- [ ] Phase 3: Add gap detection and checksum verification
- [ ] Phase 4: Add WalletType extensibility
- [ ] Phase 5: Write documentation
- [ ] Phase 5: Write comprehensive tests
- [ ] Phase 6: Review with anti-ai-slop
- [ ] Phase 6: Run make test, make lint

## Timeline Estimate

- Phase 1: 2 hours (interfaces, config, tests)
- Phase 2: 3 hours (logging all reserve/release sites, tests)
- Phase 3: 2 hours (replicator, verification, tests)
- Phase 4: 1 hour (wallet type extensibility)
- Phase 5: 2 hours (documentation, examples)
- Phase 6: 1 hour (review, cleanup)

**Total: ~11 hours** (1.5 days)

## Future Enhancements (Out of Scope)

- Snapshot compression/delta encoding
- Multi-client snapshot batching
- Real-time balance streaming via WebSocket
- Balance reconciliation service
- Automated gap recovery protocol
