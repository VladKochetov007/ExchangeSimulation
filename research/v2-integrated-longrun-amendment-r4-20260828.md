# V2 integrated long-run protocol amendment r4

Date: 2026-08-28  
Parent source commit: `1aa8ec9` (`fix: close exact linear accounting conservation gap`)

## Status and scope

This amendment is the registered successor to the integrated long-run
candidate-v3 protocol. Candidate-v3 remains an immutable historical failure:
its fresh seed-607 run reached completion, but failed the conservation contract
with a bounded integer weighted-basis residual. Its evidence and failure note
are retained; no v3 output is retroactively promoted.

The r4 gate tests whether the corrected exact linear accounting contract can
complete the already registered development program. It is a mechanical,
determinism, provenance, lifecycle, and evidence-contract gate. A pass is not a
claim that market realism, funding anchoring, executable price discovery,
liquidation reachability, option-surface emergence, or ecology realism has been
established.

The registered r4 cells are full development cells `dev-607`, `dev-613`, and
`dev-617`, followed by the seed-607 controls `dev-607-none` and `dev-607-g8`.
Reserved holdouts `619`, `631`, and `641` remain unread and unconsumed until a
separate freeze authorization is written and independently reviewed.

## Contract identities

The future artifacts use these new identities:

| Artifact | r4 identity |
|---|---|
| runner metadata | `v2-integrated-longrun-runner-v4` |
| extracted candidate/integrity/activation metadata | `v2-integrated-longrun-candidate-v4` |
| parity attestation | `v2-integrated-longrun-parity-v2` |
| development score | `v2-integrated-longrun-scorer-v3` |

Runner metadata schema is version 4. The protocol does not mutate or reuse
candidate-v3 evidence directories. The r4 output root must be fresh and must
refuse overwrite of any existing cell or derived artifact.

The canonical r4 evidence root is
`/home/vlad/v2-integrated-longrun-candidate-20260828-v4`. The runner, extractor,
parity checker, and scorer reject any other root and reject symlinked roots or
cell paths. This prevents a historical directory or an alternate filesystem
alias from being silently treated as r4 provenance.

## Corrected accounting contract

For margined linear instruments only, the position store uses an exact integer
average-cost lattice. The aggregate basis numerator is bounded-width and all
transitions are preflighted before public position or balance state changes.
Partial close basis allocation uses deterministic toward-zero integer division;
realized cash is the cumulative toward-zero result, and the unapplied
remainder is carried across flat transitions, flips, and later reopening. The
carry is not silently discarded.

Exact settlement and liquidation use the same accounting state. Liquidation
prices are solved on the discrete price lattice with the toward-zero equity
boundary and an adverse-neighbour verification. The strict exchange composition
requires the exact all-or-nothing store operations for linear margined
instruments; there is no legacy fallback in the strict path. Options retain
their existing legacy accounting and are not registered with the linear exact
store.

Expiry settlement is atomic. It previews terminal carry, preflights all client
and venue cash movements, settles each linear position through the exact store,
then compare-and-clears the expected carry before applying the terminal rounded
cash adjustments. A mismatch leaves the carry untouched and fails closed.

Every terminal adjustment is represented by a `position_rounding` event with
client, venue, symbol, timestamp, asset, cash adjustment, remainder numerator,
and precision. The analyzer requires `abs(remainder_numerator) < precision`,
unique terminal keys including venue/client/symbol/timestamp/asset, exact client
wallet linkage in the `perp` bucket, exact venue linkage in `fee_revenue`, and
checked aggregate arithmetic. The candidate integrity predicate requires every
rounding-audit counter to be zero.

The candidate conservation tolerance remains at most 1000 fixed units. A
residual outside that contract is a failure; the bound is not relaxed to rescue
a run.

## Build and execution provenance

Before the first r4 cell, a clean detached performance/build worktree will
produce `multivenue`, `mvanalyze`, and `prunegate` with Go 1.27,
`CGO_ENABLED=0`, `-trimpath`, and VCS stamping enabled. Each binary must report
`vcs.modified=false`, the exact clean source revision, and the recorded build
settings; the runner and extractor verify the actual embedded Go version begins
with `go1.27`. The parity attestation carries the simulator binary SHA-256 and
embedded Go version, plus the prunegate SHA-256, revision, and toolchain. The
runner, extractor, parity checker, and scorer require these values to match the
clean pinned build and each development/control cell. The gate worktree must be
clean before launch and extraction.

Execution is limited to the registered r4 development cells and seed-607 parity
controls. `GOMAXPROCS=4` is used for full and no-log cells; the registered G8
control uses `GOMAXPROCS=8`. Resource monitoring remains active: use bounded
parallelism, keep RAM below the available safe margin, and refuse a launch when
the evidence filesystem has less than the protocol free-space threshold.

Raw logs and evidence are retained until every applicable measurement contract
passes. No historical result is deleted. No holdout path is read by the
development runner, extractor, parity checker, or scorer.

## Review gates

An independent Sol-xhigh review is required after the r4 source/protocol
change, after the clean pinned build, after the fresh dev-607 extraction, and
before the three-cell/parity freeze decision. A review may block progression on
any provenance, accounting, analyzer, statistical, or contract defect.

The prior closed lines remain closed: `ae13f9a`, P3e, signed-price, P4/P5,
P6, P7d, and the mixed-timing line are not reopened without a new concrete
regression against their existing evidence.
