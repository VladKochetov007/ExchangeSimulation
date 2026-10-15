# Funding Batch Public-Composition Probe — Mechanics Only

Status: exploratory test-only probe; ME-014 remains preparation-only and `run=false`.

## Question

Can a consumer outside package `exchange` compose the public funding-account snapshot and pure funding-batch preview across repeated settlements while retaining per-account fractional state and a finite residual reserve?

This is narrower than asking whether the simulator has a production-ready funding lifecycle. In particular, it does not test scheduled settlement, concurrent venue activity, canonical payment evidence, or economic funding behavior in a market world.

## Exact source and artifact identity

- Scientific baseline inspected: `a9dcd1f564138e97645574239427e9fd512f7c9d`.
- Working change: `research/me014composition/funding_composition_test.go` (new external-package test; no production source change).
- Relevant public APIs: `DefaultExchange.CaptureFundingAccountSnapshot`, `exchange.PreviewFundingBatch`, `ScaledFundingAccrualSnapshot`, and `Client.MutatePerpBalance`.
- Reproduction after the test is committed: `GOMAXPROCS=2 go test ./research/me014composition -run 'TestConsumer(OwnedFundingState|FundingShortfall)' -count=1`.

## Test construction and observed arithmetic

The fixture uses one perpetual, fixed opposing positions of +1, +1, and −2 base-precision units, positive funding of one quantized unit per basis point, a constant mark, and a caller-owned reserve initialized to 3 quote atoms. The adapter captures the complete venue account snapshot, combines it with caller-owned accrual snapshots and reserve state, obtains the pure preview, rechecks that the source and simulated timestamp have not changed, and then applies the result under a test-only quiescence assumption.

For each term, with denominator `base_precision × 10,000 × rate_units_per_bp = 10,000`, the signed cash numerators are −100, −100, and +200. This makes the sub-atom claims visible without floating-point arithmetic.

Across 100 repeated synthetic settlement terms:

| Checkpoint | Client quote cash (long 1, long 2, short 3) | Consumer reserve | Sum |
|---|---:|---:|---:|
| Initial | 100, 100, 100 | 3 | 303 |
| 50 terms | 100, 100, 101 | 2 | 303 |
| 100 terms | 99, 99, 102 | 3 | 303 |

All three carried fractional remainders are zero after 100 terms. A separate payer-shortfall case starts all three wallets at zero and raises the mark so both long accounts owe one atom. The preview returns the typed shortfall for clients `[1, 2]`; the fixture's cash and caller-owned reserve/accrual state remain unchanged.

These values validate this synthetic arithmetic path and its failure-before-mutation behavior. They are not a funding-rate calibration, market result, or validation of the modeled settlement interval.

## Interpretation and limits

**Supported narrowly:** the public snapshot and pure preview can be composed by external test code with persistent caller-owned accrual/reserve state for repeated deterministic calculations. The typed payer-shortfall result is usable by such an adapter as a pre-commit failure.

**Not established:** a safe production settlement adapter. The test does not enter the runner's verified pre-ingress phase, hold exchange locks across preview and commit, serialize against trades or other risk work, write canonical account/reserve movement evidence, or atomically bind account balances, venue reserve, accrual state, and funding schedule. The test's `Client.MutatePerpBalance` calls are serialized fixture mutations, not a concurrency-safe or evidence-complete commit protocol. The reserve is deliberately external to `DefaultExchange.ExchangeBalance`; its conservation check is therefore test-owned, not the exchange ledger's invariant.

The current legacy `SettleFunding` / `CheckAndSettleFunding` path remains distinct and unchanged. This probe neither replaces nor revalidates that path, and does not show that production already uses the fractional batch preview.

## Independent check and validation

- Fresh Sol-6 medium reviewer Bernoulli: **ACCEPT**, limited to arithmetic/accounting accuracy and claim boundaries. The reviewer checked the actual external test, preview implementation, and legacy funding path; found no must-fix issue; did not run tests or simulations.
- `GOMAXPROCS=2 go test -race ./research/me014composition -count=1`: passed.
- `GOMAXPROCS=2 go vet ./research/me014composition`: passed.
- `GOMAXPROCS=2 go test ./...`: passed, exit 0. The exact candidate test was included; most unrelated package results were cached.
- `git diff --check`: passed.
- The full `make test` gate remains pending until this exact candidate is committed, because its retained-archive/parity scripts reject a dirty worktree. No economic worlds or holdouts were run or inspected.

## Decision

Record a partial mechanical composition result and stop at the production-commit boundary. More external arithmetic tests cannot establish atomicity or evidence completeness that the public contract does not provide. A follow-up would require a separately authorized production adapter/contract with a canonical journal, exclusive phase ownership, whole-source revalidation under the commit lock, fault injection across every write boundary, and repeated-term reconstruction from emitted evidence. No such implementation, ME-014 readiness acceptance, or economic run is authorized by this probe.
