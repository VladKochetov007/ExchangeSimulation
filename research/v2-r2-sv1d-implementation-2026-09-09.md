# V2-R2-SV1D implementation checkpoint

Date: 2026-09-09
Scientific branch: `feature/r2-cdf-survival-successor-sv1d`
Predecessor candidate: `1fda960` / SV1C negative activation
Implementation commit: `910cf2c`
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
