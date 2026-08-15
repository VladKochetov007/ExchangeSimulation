# Active frontier

## Target and baseline

Main `6006223` plus pending multi-venue/options instrumentation: `go test ./...`,
focused race suites, and `go vet ./...` pass. Verification tier A.
External-validity gaps remain explicit rather than being treated as bugs fixed
by passing tests. The detailed current conclusion is in
`research/finalization-2026-08-15.md`.

## Supported claims

- Foreign fee assets are reserved per open order rather than merely checked at placement.
- A partial fill that exhausts a foreign fixed-fee balance force-cancels the unbacked remainder.
- Liquidation repayment preserves spot/perp loan attribution after repaying more than the perp-attributed share.
- Terminal venue and actor lifecycles are idempotent; scheduled delayed outputs are dropped after stop.
- Same-timestamp expiries settle in symbol order, before the option risk sweep.
- `make audit FUZZ_COUNT=3` passes vet, static correctness analysis, complete/race test suites, and repeated invariant fuzzing.
- `cmd/reprocheck` preserves fixed-seed replicas and emits a canonical
  metrics-digest manifest; after opting derivsim into deterministic phases,
  three 20-second active replicas now agree.
- Derivative margin admission cannot create negative collateral on integer overflow, and local underlying references must share base, quote, and fixed-point precision until FX conversion is modeled explicitly.
- Dynamically listed derivative events are retained in a symbol-tagged stream;
  `cmd/derivsim` now emits a validated Black-76 Greek report with explicit
  flat-IV and spot-forward-proxy caveats.

## Live mechanism families

| Hypothesis | Mechanism | Evidence | Risk | Next test | Cost |
| --- | --- | --- | --- | --- | --- |
| H-001 | State-machine invariant gap | Two financial accounting defects confirmed and fixed | High | Cross-asset partial-fill/fixed-fee bounds | CPU |
| H-002 | Extension-point contract mismatch | No new defect in this pass | Medium | Interface implementation sweep | CPU |
| H-003/H-006 | Concurrent publication/time ordering | Legacy quiescence falsified; direct phase runtime confirmed | High | Phase-order latency courier | CPU |
| H-007 | Native rewrite as current bottleneck fix | Falsified by profile; orchestration dominates matching | Medium | Optimize verified Go phase/runtime path | CPU |
| H-008 | Long-run one-sided cross liquidity is an artifact | Falsified after deterministic inventory trace | High | Explicit hedged versus finite-inventory policy | CPU |
| H-009/H-010 | Derivative observability and baseline dealer Greeks | Dynamic logs fixed; short-gamma smoke result is model-conditioned | High | Per-position terminal Greek rows | CPU |

## Contradictions and evaluator risks

- Existing regressions are strong but do not exhaust custom instrument, fee model, and lifecycle composition.
- Direct randomwalk and derivsim are reproducible only with deterministic
  phases and direct mounts. Delayed latency couriers are still asynchronous;
  latency-dependent findings are diagnostic rather than research claims.
- Current derivsim now retains a symbol-tagged dynamic derivative stream, but
  timestamp-aligned Epps samples, explicit run balance endpoints, per-position
  Greek rows, a terminal pre-expiry snapshot, and a non-static IV process are
  required before derivative flow, volatility, or PnL claims.
- Cash-carry and parity actors use midpoint/intention logic without complete
  execution ledgers. Their current outputs cannot support profitability or
  equilibrium claims.

## Budget remaining

- Wall time: approximately 10 hours.
- CPU: approximately 9 hours. GPU reserved but not useful for Go tests.

## Next three decisions

1. Phase-order scheduled latency delivery and add an adversarial delayed
   gateway canonical-digest test.
2. Add per-position terminal Greek telemetry, analyzer timestamp joins, and
   explicit initial/terminal balance checkpoints.
3. Build the three-venue options contract with a true A-S linear maker, then
   replace midpoint/intention arb accounting with executable per-leg fills,
   terminal exposure, all-in cost, and deterministic PnL reporting.
