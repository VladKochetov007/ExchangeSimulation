# V2-R2-SV1D implementation checkpoint

Date: 2026-09-09
Scientific branch: `feature/r2-cdf-survival-successor-sv1d`
Predecessor candidate: `1fda960` / SV1C negative activation
Implementation commits: `910cf2c`, `bdacbae`, `a5893f8`, `9c2ab23`
Status: implementation and focused contract testing; no seed 659, development
cell, 24-hour world, or holdout has run from this candidate

## Scientific boundary

The accepted R2/SV1C result remains the archived negative control:
**NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**. SV1D is a separately named,
opt-in successor. It does not alter the eight historical ABC/USD suppliers,
the R2 calendar, historical configs, or retained evidence.

The independent performance branch remains an asynchronous feed. The latest
reviewed refs at this checkpoint are performance `b1847ac`, CDF port
`39768df`, and economic audit `e85e16c`; no newer commits were available.
Its binary evidence and performance changes are not merged into this worktree.

## Implementation contract

The actor's one-sided policy is enabled only by
`quote_on_one_sided_local_book`. It binds its tick and declared admission
minimum to the registered instrument before construction, uses only its delayed
gateway snapshot and private reference, and fails closed for locked, malformed,
stale, or unpriceable local states. A missing-side quote is integer tick-aligned
and bounded by finite cash, gross inventory, position, quantity, and loss
limits. The registered exchange admission minimum is distinct from the
SV1D scientific qualification minimum; the latter is ten admission lots and is
carried in every successor decision.

The actor now rejects same-symbol fill evidence whose order identity, side,
price, quantity, or full/partial flag contradicts its live quote. Rejected
transitions do not mutate position or cash and place the actor in a fail-closed
risk state. A full fill or cancellation records the observation frontier at
which the quote closed; one-sided mode must receive a later observation before
re-entering, preventing forced same-observation replacement.

The analyzer:

- validates the successor's registered admission minimum and larger qualifying
  threshold against the normalized roster and decision evidence;
- accepts missing-side quote provenance only when it matches the declared
  local-touch rule;
- retains public snapshots separately from client-specific observations;
- joins a restoration candidate to the exact accepted order ID and requires
  that order to remain live at the qualifying public snapshot;
- records per-candidate source, acceptance, restoration, order-outcome, and
  partial-fill response evidence;
- rejects malformed fill lifecycle transitions, including side/price changes,
  overfills, and contradictory remaining quantities;
- uses the qualifying threshold for successor weak-side, restoration, and
  supplier-removal measurements while preserving the admission threshold as a
  fallback for historical fixtures;
- rejects self-reference above the preregistered threshold at both aggregate and
  venue scope; and
- retains renderer execution, canonical execution, and full-evidence hashes in
  the CDF audit command output.

## Verification

Passing focused checks at `910cf2c`:

- `go test ./analysis -count=1`
- `go test ./evstream ./evstream/codecs ./evstream/exsim ./types ./exchange ./cmd/cdf-liquidity-audit -count=1`
- one-sided supplier, reentry, fill-reconciliation, tick-binding, and
  finite-risk tests in `simulations/multivenue`
- qualifying-threshold and per-venue self-reference analyzer regressions
- `git diff --check`

A broad multivenue package run reached the repository's default ten-minute
fresh-process timeout in the existing V24-L1 random-side neutrality test; a
previous independent package run passed in approximately 1,010 seconds. This
is a compatibility/runtime gate result, not an activation result. The clean
post-commit gate must rerun the full repository contract with an explicit
longer timeout and separately record any existing fresh-process timeout.

The next promotion boundary is:

1. commit the amended preregistration with the admission/qualification
   distinction;
2. pass clean full test, vet, race, and binary-evidence contract checks;
3. assemble immutable treatment, same-roster mode-off, and no-roster probe
   manifests for seed 659;
4. obtain one fresh independent Sol-xhigh review of the exact clean tree; and
5. only then run the five-minute development activation probe.

Holdouts `619`, `631`, and `641` remain untouched. No result from this
candidate authorizes a full campaign, freeze, or holdout execution.

## Post-review contract repair checkpoint — 2026-09-09

The clean full repository test was run with `go test -timeout=30m ./...` under
`GOMAXPROCS=2` and `GOMEMLIMIT=8GiB`. All Go packages, including the long
fresh-process multivenue suite, passed; the first run failed only because a
repository-path hygiene test found a literal system-temporary path in the
config checker.
That non-scientific issue was removed in commit `b729a9f`, and its focused
regression passes.

The independent Sol-xhigh review of commit `5558725` rejected promotion. Its
high-severity objections are accepted and are being repaired before any
activation measurement: source/base-unit conversion and horizon-relative
finite-capital reachability; runtime no-roster topology evidence; exact
terminal-failure comparison binding; dynamic available-memory reserve and
live analyzer supervision; top-level arm-record binding; and closed artifact
namespace checks. The previous `5558725` package therefore remains a rejected
pre-review state, not an activation authorization.

The current repair preserves the R2 calendar and simulator economics. It
introduces one explicit SV1D activation-only roster amendment: the generated
probe roster scales selected finite capital/risk fields from the retained SV1C
source by ten, with the correction preregistered in the append-only amendment
in the successor preregistration. Historical source configs and evidence are
not rewritten. The treatment activation predicate additionally requires
observed filled quantity at least equal to the configured scientific
qualification minimum; initial balances and nonzero utilization are not
treated as proof of economic activation.

Current performance/red-team refs remain performance `b1847ac`, CDF port
`39768df`, and economic audit `e85e16c`; the latest `git fetch origin --prune`
found no newer commits. No seed 659, development cell, 24-hour world, or
holdout has run from this candidate.

## Package repair checkpoint after independent review

The first independent review of the activation package was not treated as a
launch authorization. Its concrete objections were repaired in the working
candidate and are now covered by the package contract: the probe runs a
treatment, a same-roster mode-off control, and a no-roster control; the
comparison uses a successor-specific mode-pair predicate; and the matcher
recognizes the retained eight ABC/USD suppliers separately from the four CDF
specifications replicated across three venues. The one-sided fresh-observation
frontier is recorded only when a one-sided quote closes; a two-sided quote does
not inherit that wait state.

The generator/checker pair now binds the exact clean source parent, registered
contract dependencies, config hashes, normalized-config idempotence, exact
rosters/roles/cadences/thresholds, and the allowed generated-artifact delta.
The runner and scorer require all three arm artifact sets, terminal outcomes,
binary checkpoint attestations, evidence manifests, and an externally stored
accepted exact-tree review before measurement. The preregistration explicitly
limits the five-minute probe's direction coverage: positive initial inventory
can exercise bid-only/missing-ask restoration, while ask-only/positive-inventory
depletion remains unclaimed and requires a separately registered probe.

At this checkpoint no configs, binaries, review attestation, seed-659 output,
development world, or holdout have been produced from the successor. The
default `make test` invocation reached the existing multivenue fresh-process
timeout at 600 seconds; this is recorded as a compatibility/runtime timeout,
not as scientific evidence. The required follow-up is a clean full test with
an explicit longer package timeout, followed by vet/race/evidence-contract
checks and a fresh exact-tree independent review.

## Append-only mark-pass serialization checkpoint

The successor now serializes `CommitMarkEpoch`, perp mark publication, option
derivative refresh, and public liquidation entry points with one
exchange-owned pass mutex. The derivative refresh calls the non-reentrant mark
producer helper when a changed option input requires a complete sibling refresh;
it does not deadlock by reacquiring the public wrapper. This closes the
concurrent producer gap without changing the strict five-argument
`PositionMarginSnapshotter` extension contract already present on this branch.

The regression `TestMarkProducersSerializeRiskEpochCommit` blocks a mark
calculator and verifies that a concurrent derivative refresh waits for the
first pass to complete. This is a concurrency/information-boundary correction,
not an economic retuning. It has been pushed in `a5893f8`; the subsequent
scorer provenance correction is `9c2ab23`. No config, binary, activation
attestation, development world, or holdout has been created from these
revisions. The performance/red-team refs remain `b1847ac`, `39768df`, and
`e85e16c`, with no newer fetched commits; no performance-branch code is
imported.

## Append-only capacity-contract checkpoint — 2026-09-10

The default clean `make test` gate was rerun at the generated-config
revision. All packages before and after `simulations/multivenue` passed; the
repository's ten-minute multivenue package timeout recurred at exactly
`600.015s` in `TestV24L1RandomSideControlEvidenceIsFreshProcessDeterministicAndNeutral`.
The failure is retained as a runtime compatibility limitation, not treated as
activation evidence or silently extended into a green result.

The independent-review repair exposed one contract-layout defect before any
capacity measurement: the dedicated 24-hour capacity runner creates a
resource attestation and terminal evidence but intentionally does not create
the activation arm's `run-status.json`. Commit `ca1a722` gives the capacity
validator its own closed expected-file set rather than inheriting the
activation set. The derived-config retirement/regeneration sequence is
`6454fe7` then `c8ff967`; the final normalizer is a clean Go 1.27.0,
CGO-disabled build whose VCS revision is `c8ff967`.

The capacity probe remains a prerequisite, not a scientific result. It is
registered for treatment seed 659 over 24 simulated hours with `evstream_v3`,
full logs, `GOMAXPROCS=2`, a 20 GiB address-space limit, an 18 GiB Go memory
limit, a 4 GiB free-disk reserve, a 4 GiB safety margin, live available-RAM
monitoring, and a one-hour wall limit. Its output and attestation are bound
to the exact source revision, normalized treatment hash, accepted exact-tree
review, checkpoint validator, evidence manifest, terminal outcome, binary
hashes, and observed resource measurements. No capacity probe, activation
probe, development cell, 24-hour world, or holdout has run from this
revision.

## Post-review contract repair checkpoint — 2026-09-10

Schrodinger's independent Sol-xhigh review of the immutable pre-repair
candidate at `2d31abb` rejected promotion. It identified three contract gaps:
the scorer could not classify valid terminal-negative evidence because it
required `valid == true` first; the capacity attestation did not close the
measured probe root or bind its root-level normalization stderr; and the
scorer did not bind all published provenance claims to the embedded comparison
and its actual file/hash.

The minimal repair is committed in `789e2bf`. The activation contract now has
one classifier for ordinary and terminal-negative comparisons, the scorer
requires coherence between recomputed and published comparison claims, and
the capacity runner/validator enforce the exact probe-root inventory and
`config.stderr.log` hash. The subsequent generated-config retirement and
regeneration commits are mechanical provenance maintenance; the R2 calendar,
finite CDF supplier economics, historical evidence, and SV1C negative control
are unchanged.

The repaired source passed the expanded SV1D checker/contract fixtures,
`go vet ./...`, the uncached focused package suite (including multivenue in
1002.515 seconds), exchange/tests/selected-multivenue race gates, and
`git diff --check`. The default ten-minute `make test` timeout remains a
known runtime limitation of the pre-existing fresh-process multivenue test;
the final gate will use and record the explicit longer timeout. No review
attestation, capacity measurement, seed-659 activation probe, development
cell, 24-hour world, freeze, or holdout has been created from the repaired
candidate.

## Final uncached mechanical gate before exact-tree review — 2026-09-10

The exact clean candidate at `da57f8e28f89b6c51615ecec513e763fff352439`
completed the deliberately uncached full repository gate:

```text
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local GOMAXPROCS=2 \
GOMEMLIMIT=8GiB GOFLAGS="-count=1 -timeout=30m" make test
```

It passed with `FULL_MAKE_UNCACHED_EXIT=0`; the long
`simulations/multivenue` package completed in `1000.377s`. The SV1C failure
sentences printed during the shell contract checks are expected diagnostics
from malformed/negative fixtures, and the checks concluded with passing
SV1C, SV1D, survival-summary, and integrated-long-run archive contracts.

The uncached run was performed after two non-economic promotion defects were
found and repaired. The SV1D comparison fixture was completed with the
required supplier-volume claim in `69a6a60`; then the production cardinality
validator was corrected in `4b9c08a` so it measures the `suppliers` and
`venues` arrays rather than the enclosing objects. The generated-artifact
retirement/regeneration pairs are `655c89`/`c45aa67` and
`1fb69ac`/`da57f8e`. These repairs preserve the R2 calendar, SV1D economics,
the SV1C negative control, and all historical evidence.

The prior focused vet, targeted race, and fresh-process evidence-neutrality
gates remain green on the unchanged Go source lineage; the final promotion
record will rerun the required exact-tree vet/race/contract matrix after the
documentation-bound artifact regeneration. No review attestation, binary
capacity measurement, seed-659 activation probe, development cell,
24-hour world, freeze, or holdout has been created from this candidate.

The next boundary is a fresh independent Sol-xhigh review of the complete
exact tree. Review acceptance is required before the binary-capacity probe;
capacity is a launch measurement, not scientific activation evidence. The
activation probe remains the only permitted SV1D world after that boundary,
and development cells remain gated on its independent grading.
