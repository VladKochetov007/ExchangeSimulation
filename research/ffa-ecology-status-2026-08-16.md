# FFA Ecology Handoff Status

**State:** implementation is frozen at this commit boundary. This document is
an evidence ledger and continuation plan, not a claim that the requested market
ecology already exists.

## Executive Summary

The repository now has a reproducible three-venue derivative control and the
scaffolding for a three-asset spot graph. It does **not** yet have a defensible
free-for-all ecology, a triangle-arbitrage result, a payoff matrix, an
evolutionary rule, or a rock-paper-scissors conclusion.

The correct scope is:

- **Accepted:** deterministic phase execution and the one-underlying strict
  USD evaluator control (E-038).
- **Expected invalid control:** E-039 demonstrates that strict valuation fails
  closed when a required `CDF/USD` terminal mark is one-sided.
- **Superseded, not accepted:** E-040 has reproducible strict account output
  but used USD quote units for an `ABC/CDF` CDF-quoted maker. It cannot support
  an economic conclusion about cross-pair liquidity or triangular arbitrage.
- **Pending code correction:** `ABC/CDF` now receives CDF quote precision and
  its control variance is converted under the explicitly frozen-CDF synthetic
  assumption. It needs complete verification and a fresh E-041/E-042 run
  after this pause.

No result in this branch says a strategy is profitable, persistent, dominant,
or part of a non-transitive equilibrium.

## Implemented Research Substrate

| Component | Current state | Boundary |
| --- | --- | --- |
| Phase runtime | Accepted deterministic direct-runtime control | Delayed courier semantics still need their own canonical digest before latency claims. |
| Venue allocation | `VenueRules` selects exact existing price-time or pro-rata matcher per venue | This is not CME-style hybrid, top, split, leveling, or auction allocation. |
| Three venues | `north`, `central`, `south`, separately funded books | Venue identity is not a clearing or transfer network. |
| Instruments | `ABC/USD`, perpetual, rolling dated futures, European options; optional `CDF/USD` and `ABC/CDF` | Cross-listed CDF derivatives, generic FX paths, and a full contract graph are absent. |
| Agents | Stoikov-inspired spot/perp makers, futures maker, option dealer, random spot/option flow, optional one-symbol router | No population selection, mutation, re-entry, learning, or broad heterogeneous agent population. |
| Reporting | Strict initial/terminal participant accounts in USD; option-risk telemetry | Fitness is not yet population-wide marked PnL under an explicit selection policy. |

## Evidence Ledger

### E-038: accepted evaluator control

- **Question:** can the one-underlying, three-venue control produce a complete
  strict USD report independently of host parallelism?
- **Hypothesis:** deterministic phases plus complete terminal two-sided marks
  produce the same manifest and risk report at `GOMAXPROCS=1` and `14`.
- **Result:** supported for this narrow evaluator scope.
- **Evidence:** 27 initial and 27 terminal account rows; manifest SHA-256
  `a1f81dfffdca0c249591cc23aa62941e546342006e746a37ed1a611a0951adc4`;
  Greek report SHA-256
  `bda88d8bdf9bc6f21ef545e36a7d75f757eec4855603278573471acb6bd58c27`.
- **Artifacts:**
  `logs/research/ffa-ecology-control-e038-canonical-gomax1-2026-08-15` and
  `logs/research/ffa-ecology-control-e038-canonical-gomax14-2026-08-15`.
- **Why it matters:** it makes later population accounting falsifiable.
- **Why it is insufficient:** one underlying, static policies/IV, no CDF, no
  triangle actor, no selection, and no external calibration.

### E-039: invalid strict-mark probe

- **Question:** does one liquidity provider per new `CDF/USD` and `ABC/CDF`
  pair sustain an auditable terminal conversion mark?
- **Result:** no. South `CDF/USD` was one-sided at five minutes.
- **Correct behavior:** strict terminal accounting failed rather than omitting
  CDF exposure or falling back to a last trade.
- **Evidence:** manifest SHA-256
  `a4da2c343496c4aaa95494a449b54294d32cf22abab1303c2a5cde81ae258c4e`;
  error `terminal_post_mark participant valuation requires two-sided CDF/USD
  mark on venue south`.
- **Interpretation:** measurement gate worked. This is neither a PnL result
  nor evidence that one maker is generally insufficient in a real market.

### E-040: reproducible but superseded cross-asset probe

- **Question:** does using two independently funded makers per added spot pair
  give a complete strict report across host parallelism?
- **Observed:** 39 initial and 39 terminal USD account rows; same manifest and
  report SHA-256 at `GOMAXPROCS=1` and `14`.
- **Archived artifacts:**
  `logs/research/ffa-ecology-control-e040-cross-asset-canonical-gomax1-2026-08-15`
  and
  `logs/research/ffa-ecology-control-e040-cross-asset-canonical-gomax14-2026-08-15`.
- **Invalidating discovery:** the `ABC/CDF` instrument has CDF precision
  `1e8`, but its Stoikov configuration used USD precision `1e5`. Consequently
  its price scale and risk term were not in the contract's quote units.
- **Disposition:** retain the artifacts and reproduction observation, but do
  not reuse them for cross-pair, triangle, liquidity, or ecology claims.

## Current Unverified Correction

`simulations/multivenue/sim.go` now creates `ABC/CDF` makers with:

1. `QuotePrecision = mvBasePrecision`, matching the CDF quote currency.
2. `InitialVariancePerSec = USDVariance / (CDFUSDPrice^2)` under a frozen
   `CDF/USD = 3000` control. This is a units conversion only; it does not model
   dynamic CDF volatility or correlation.
3. A regression in `simulations/multivenue/sim_test.go` that checks precision,
   converted variance, and a finite positive bootstrap quote.

A focused test of this correction passed before the requested stop. The full
suite, race suite, vet, and a new deterministic five-minute artifact are
**intentionally not run at this boundary**.

## Debug Findings That Block Economic Claims

### Triangle and cross-currency execution

- The existing `feesim` triangle actor converts `Q/ABC` using the wrong base
  precision, a 100x sizing defect for the configured units. It also uses stale
  cached snapshots, unbounded market/GTC legs, and does not reconcile
  partial/cancelled legs or actual fees.
- The `randomwalk` triangle actor has the same non-atomic partial/cancel and
  residual-accounting limitations.
- A same-book FOK order is not cross-instrument atomic. A valid replacement
  must submit sequential, bounded **limit FOK** legs based only on coherent
  delayed public snapshots, and ledger actual per-fill quote fees and all
  residual ABC/CDF inventory.
- The initial triangle control must restrict fees to known quote-denominated
  percentage fees. Fixed, base, and foreign fees require fragmentation-aware
  reservation semantics and are not safe for an economic baseline.
- CDF is not yet a complete spot-margin collateral/price-source asset.

### Information and venue semantics

- Market-data sequence numbers are global across symbols/types. They are not
  valid per-book recovery signals, and high-rate routers need per-feed sequence
  semantics plus resnapshot behavior.
- Direct strategy access to exchange state must remain prohibited. Agents may
  use delayed public book/trade/instrument events and private own-order events
  only.
- Price-time and basic pro-rata are available. They must not be labeled as
  real-world hybrid allocation rules until minimums, caps, residual policy,
  rounding, and priority rules are specified and tested.

### Derivatives and valuation

- Option terminal equity needs exchange-owned mark hierarchy: a premium-paid
  option's wealth is mark value plus wallet, not futures-style entry-to-mark
  PnL added again. A real matched-fill conservation regression is required.
- Pre-expiry Greek capture must occur at a positive time-to-expiry and use an
  exchange-consistent underlying mark. Actor-local stale spot state is not
  adequate for causal risk attribution.
- Terminal execution metrics need a two-sided terminal mark or an explicit
  last-trade label; a hidden fallback cannot be treated as executable value.

### Previously found engine hardening work

Earlier commits introduced deterministic phases, actor lifecycle buffering,
safe aggregate arithmetic, fee-reservation hardening, and side-aware quoting.
Those changes are retained. Their details and experiment references remain in
the prior research documents; none is a substitute for a new cross-asset
economic experiment.

## Preserved Prototype Policy

Rejected experimental code and tests are retained by default. A prototype is
classified as `inactive` in research metadata when it has a semantic defect;
it is not deleted merely because it is unsuitable for evidence. Deletion is
allowed only for generated artifacts or code explicitly superseded by a tested
replacement, and the reason must be recorded in the relevant research note.

The first in-progress multivenue triangle draft was stopped before it was
committed because it used simultaneous market legs and therefore could not
meet the FOK/residual contract. Its requirements are preserved here and in
`FFA-02-R2`; it must be rebuilt as the sequential limit-FOK design below, not
silently reintroduced as an active strategy.

## Next Implementation Approach

### 1. Revalidate the corrected graph

Run the focused, full, race, and vet gates with repository-local Go caches,
then run E-042 with the exact E-040 topology under `GOMAXPROCS=1` and `14`.
Require byte-identical manifest/account/risk digests, 39 complete initial and
terminal rows, and a two-sided direct `ABC/USD` plus `CDF/USD` terminal mark.
Archive both runs under `logs/research/` and add an immutable experiment row.

### 2. Build a triangle null harness before a population

Create one group per attempted cycle with immutable group/leg IDs, local
snapshot timestamps, request/order IDs, quote-asset fees, executed notional,
and residual inventory. Gate signals on the same public snapshot timestamp,
top-level displayed depth, known quote fee, and a positive all-in expected USD
edge.

Direction A, starting from ABC quantity `q`, is:

```text
ABC -> CDF at ABC/CDF bid
CDF -> USD at CDF/USD bid
USD -> ABC at ABC/USD ask
```

Direction B is the reverse. Submit one limit-FOK leg at a time. Derive the
next amount from actual prior fills, not desired intent. Do not submit later
legs after a reject. A bounded CDF/USD unwind is permitted only to remove
rounding dust; otherwise record the group as incomplete with explicit
residuals. A complete group must have zero non-USD residual within declared
rounding bounds.

### 3. Build independent mechanism controls

Only after the triangle ledger passes fixtures should the project add local
basis/funding/parity agents to multivenue. Each family needs a no-strategy
control, fee/latency/label permutations, residual and completion metrics, and
held-out seeds before it joins a fixed population mixture.

### 4. Introduce an ecology evaluator last

Freeze initial capital, risk limits, bankruptcy, recapitalization, re-entry,
and mutation in a new manifest version. Estimate directed invasion growth
`g(A | population without A)` in risk-normalized, USD marked-equity share.
Only call rock-paper-scissors when held-out confidence intervals support all
three directed edges and survive seed, label/ID, registration-order, and
population-mixture permutations.

## Deferred Verification Commands

These are deliberately deferred, not executed in this handoff:

```bash
env TMPDIR="$PWD/.cache/go-tmp" GOTMPDIR="$PWD/.cache/go-tmp" \
  GOCACHE="$PWD/.cache/gocache" go test ./simulations/multivenue -count=1
env TMPDIR="$PWD/.cache/go-tmp" GOTMPDIR="$PWD/.cache/go-tmp" \
  GOCACHE="$PWD/.cache/gocache" go test ./... -count=1
env TMPDIR="$PWD/.cache/go-tmp" GOTMPDIR="$PWD/.cache/go-tmp" \
  GOCACHE="$PWD/.cache/gocache" go test -race ./simulations/multivenue ./exchange ./simulation -count=1
env TMPDIR="$PWD/.cache/go-tmp" GOTMPDIR="$PWD/.cache/go-tmp" \
  GOCACHE="$PWD/.cache/gocache" go vet ./...
```

Then run the new E-042 control twice, with `GOMAXPROCS=1` and `14`, using
different persistent `logs/research/` directories and compare canonical
outputs before advancing the hypothesis ledger.

## Research File Index

| File | Purpose |
| --- | --- |
| `research/ffa-ecology-contract-2026-08-15.yaml` | Scope, information contract, invariants, stop conditions, and mutation boundary. |
| `research/ffa-ecology-deepresearch-2026-08-15.md` | Literature-backed design constraints for A-S, option inventory risk, venue allocation, fragmentation, and invasion claims. |
| `research/ffa-ecology-plan-2026-08-15.md` | Promotion ladder from mechanics through population ecology. |
| `research/ffa-ecology-budget-2026-08-15.md` | CPU/storage guardrails and generation gates. |
| `research/ffa-ecology-previous-work-2026-08-15.md` | Retained baseline, known limitations, and historic prototype boundaries. |
| `research/ffa-ecology-hypotheses-2026-08-15.jsonl` | Append-only hypothesis cards, falsifiers, and status changes. |
| `research/ffa-ecology-experiments-2026-08-15.jsonl` | Append-only commands, hashes, artifacts, and invalidations. |
| `research/ffa-ecology-findings-2026-08-15.md` | Short findings board with accepted, invalid, and pending evidence. |
| `research/ffa-ecology-control-2026-08-15.json` | Declarative control configuration used by the prior E-038/E-040 runs. |
| `research/ffa-ecology-status-2026-08-16.md` | This handoff: current code state, evidence disposition, blockers, and exact resumption plan. |

