# SV1 asynchronous performance-feed review — 2026-09-02

This is an append-only review of the independent performance/red-team branch.
It is not an authorization to merge that branch, change the SV1 economics, or
consume a later development cell or holdout.

## Revisions and inspection boundary

- Scientific worktree: `feature/r2-cdf-survival-successor`.
- Scientific source inspected: `df1057d` after the compressed route-storage
  checkpoint (the treatment trajectory itself remains bound to `3f73f30`).
- Performance branch: `origin/autoresearch/v2-performance-research`.
- The configured `github` remote was absent, so the branch was fetched from
  the existing `origin` remote without switching worktrees.
- Previous reviewed performance revision: `feeb6f9`.
- New branch tip reviewed: `c4434ad`.
- Last-reviewed marker for the next checkpoint: `c4434ad`.

The relevant new history after `feeb6f9` included the binary-pipeline
promotion memo, the analyzer-readability fail-closed change, performance
profiling/fingerprint work, and the F1 risk-semantics port. The branch was
read with `git show`/`git diff`; no performance commit was cherry-picked.

## Independent findings adjudicated against SV1

### F1/F2 — zero-exposure marks and sweep aborts

The performance branch's `385eaf7` port confirms the previously reported
finding: a margin profile must resolve marks only for positions the account
actually holds, and an unpriceable account must not abort the sweep for later
clients. The current SV1 scientific tree already has the corresponding
semantics: `buildAccountMarginProfile` collects nonzero positions before
resolving a perpetual mark, and `CheckLiquidations` continues after a
profile-price failure. The current tree's existing risk hardening, not the
performance branch, is the source of truth. No branch code was imported.

Classification for this checkpoint: **already fixed on the scientific tree**;
the completed treatment-607 trajectory predates this review and will be
checked for activation proxies during extraction.

### F3 — cross-margin same-timestamp marks

The current SV1 tree collects successful perpetual marks, installs the batch,
and only then evaluates liquidation. This is the coherent-mark-set invariant
described by the red-team reproduction; it removes lexicographic symbol-order
dependence from the risk decision. This was independently present before the
performance-feed review and was not imported from `385eaf7`.

Classification: **already fixed on the scientific tree**. A raw treatment
result remains subject to the existing liquidation/margin evidence checks.

### F6 — settlement-pending exposure

The current SV1 tree does not treat a retained settlement-pending position as
zero risk. A nonzero pending position causes the account profile to fail closed
until the declared settlement source resolves it; active siblings cannot use a
silently incomplete portfolio. This is the current lifecycle contract and is
covered by the scientific branch's pending-exposure tests.

Classification: **already fixed / fail-closed on the scientific tree**. The
treatment-607 evidence still needs a direct `expiry_settlement_pending`
activation check; no historical trajectory is reinterpreted here.

### F8 — collateral-interest truncation

The performance branch correctly leaves this as an economic-specification
question rather than calling integer arithmetic a bug automatically. The
current implementation charges only whole fixed-point interest units at its
declared minute cadence and carries no fractional remainder. The SV1 raw
trajectory includes collateral borrowing, so extraction must measure the
borrow principal, rate, interval, mathematical interest, posted interest, and
the 24-hour cumulative loss before deciding whether the registered CDF
mechanism depends materially on financing cost.

Classification at this checkpoint: **ambiguous specification; activation
measurement pending**. It is not a reason to rerun the already completed
treatment trajectory before its retained evidence is measured. If the
registered successor's supplier economics are materially altered by this
truncation, the existing trajectory cannot be promoted as corrected-semantic
evidence and a new development treatment will be required.

### F4/F7 and other adjacent findings

The feed records wallet-withdrawal risk re-evaluation and hedge-mode
liquidation-leg semantics as latent/ambiguous cases. They are not part of the
registered SV1 actor path unless the retained evidence shows a corresponding
transfer or hedge-mode activation. They remain documented red-team inputs and
are not silently promoted or fixed for scoreboard purposes.

## Performance-only work

The branch's sparse holder-index, preview-bound, fingerprint, allocation, and
binary-analyzer work remains deferred. The binary evidence prototype is not
the SV1 evidence contract; SV1 uses the already accepted `evstream_v3` raw
stream and the current analyzer contract. The only imported-adjacent change in
this scientific worktree is the separately designed compressed route-storage
adapter (`df1057d`), which preserves the uncompressed JSON-record semantics.

## Decision and next review

No new performance-branch semantic defect blocks the immediate bounded
treatment-607 extraction because F1/F2/F3/F6 are already represented by the
scientific tree's risk/lifecycle hardening, while F8 is not yet shown to alter
a registered decision. This is a triage disposition, not a freeze decision.

At the next natural checkpoint, fetch only commits newer than `c4434ad`, read
new risk or evidence-contract reports, and independently reproduce any finding
that could affect actor decisions, liquidation, balances, funding, settlement,
or lifecycle. Do not inspect or consume holdouts before the separately
authorized freeze boundary.

## Append-only checkpoint: provenance-bound activation comparison — 2026-09-08

The scientific successor is now at `4dd15d3` on
`feature/r2-cdf-survival-successor`, pushed to `origin`. This is a protocol
hardening change only. It preserves the R2 calendar/lifecycle semantics, the
finite CDF roster, the historical ABC/USD suppliers, the predecessor negative
boundary, and the holdout boundary.

The exact-tree review of `a666d02` identified a concrete substitution gap: a
valid seed-607, one-venue producer comparison could be placed under an outer
seed-643 activation attestation and pass the old four-boolean check. The fix
raises the activation-pair contract to v4 and binds the comparison to the
registered treatment/control source-config hashes, seed, horizon, nanosecond
endpoints, venue set, experiment and hypothesis IDs, `evstream_v3`/`full`
mode, reviewed source revision, simulator/analyzer hashes and linux/amd64/v1
build identity. The expected supplier population is derived from the
registered treatment config (currently 4 suppliers × 3 venues = 12),
rather than supplied by an unbound fixture argument. The runner emits the same
identity fields from its effective registered configs.

The activation-contract test now keeps the real analysis producer fixture as a
negative seed/config substitution, builds a production-shaped positive
seed-643 comparison, and rejects nested and outer mutations for seed, revision,
binary/config/analyzer hashes, evidence/log mode, venues, experiment identity,
time bounds, pair booleans and supplier population. Focused `analysis`,
`types`, `exchange`, and `simulations/multivenue` tests passed. Clean
`GOMAXPROCS=2 make test` passed all Go packages and integrated, SV1, archive and
parity fixtures after the commit. No activation, capacity, development,
freeze, or holdout world was run.

The performance branch was fetched through `b1847ac`, reading only the delta
after `c4434ad`. Its `f153e12` reaction analyzer fix is a confirmed analyzer
book-key bug candidate: symbolless spot records were pooled across a venue's
spot files, producing cross-instrument markouts. This is not simulator
semantics and is not used by the SV1B activation predicate; it is deferred
from the scientific tree. Any historical reaction-dependent claim would need
its retained evidence rescored/replayed before reuse. The binary evidence,
ordering-key, and analyzer reproducibility changes remain a separate deferred
VNext line and were not merged.

The current tree has no accepted exact-tree review attestation. The remaining
gate is `go vet`, targeted race/evidence/determinism validation, and one fresh
independent Sol-xhigh review of the complete exact tree before pinned binaries
or the five-minute activation probe. Capacity, development, freeze and
holdout execution remain closed; holdouts `619`, `631`, and `641` remain
untouched. The next performance marker is `b1847ac`.

## Append-only feed checkpoint: no new performance commits; scientific protocol hardening — 2026-09-08

`git fetch origin autoresearch/v2-performance-research` found no commit after
the last reviewed marker `b1847ac`; the asynchronous performance feed is
unchanged. The deferred `f153e12` reaction-analyzer book-key correction and the
separate VNext binary-evidence line remain unimported. No performance branch
code was merged into the scientific successor.

The scientific branch advanced to `c676b8d` for a fail-closed protocol repair,
not a performance change. The activation runner stages a positive provenance
record, validates it against complete producer artifacts and deterministic
analyzer replay, and only then publishes the accepted filename. A failed final
validation is retained under an invalid diagnostic filename. Focused activation
tests, clean `GOMAXPROCS=2 make test`, `go vet ./...`, shell syntax, and
`git diff --check` pass; the bounded targeted race/evidence/determinism gate
also passes. This does not change any economic model or historical result.

The exact-tree review of `30e2bf0` remains a historical rejection. The current
tree still requires a fresh independent exact-tree Sol-xhigh review before
build or activation. No activation, capacity, development, freeze, or holdout
run has occurred, and holdouts `619`, `631`, and `641` remain untouched. The
next performance marker remains `b1847ac`.

## Append-only feed checkpoint: terminal-diagnostic protocol repair — 2026-09-08

The fresh exact-tree Sol-xhigh review of `23594b2` found a protocol-only
terminal-failure integration bug: a valid sealed economic terminal endpoint was
recognized by the runner but rejected by the completed-only arm validator
before reaching `UNAVAILABLE_TERMINAL_FAILURE`. This was independently
reproduced and repaired on the scientific branch in `758b10e`.

The repair does not import performance code or alter market economics. It
separates completed-success and valid terminal-diagnostic arm validation,
retains the complete producer/hash chain for the latter, and stages its pair
provenance until self-validation succeeds. The new contract regression covers
the diagnostic pair and a mutated outcome hash. Clean full tests, vet, syntax,
diff-check, and bounded targeted race/evidence checks pass at the repair.

The performance branch was fetched at this semantic checkpoint and still has
no commit after `b1847ac`; the deferred `f153e12` analyzer correction and VNext
binary evidence remain unmerged. A fresh review of the post-repair exact tree
is required before any pinned build or activation. No activation, capacity,
development, freeze, or holdout run occurred; holdouts `619`, `631`, and `641`
remain untouched.

## Append-only feed checkpoint: no new commits; terminal comparison hash hardened — 2026-09-08

The performance branch was fetched again with `b1847ac` as the last-reviewed
marker. There is no new commit to inspect. The deferred `f153e12`
reaction-analyzer book-key correction and the separate VNext binary-evidence
prototype remain unmerged and do not alter the current evidence contract.

The scientific branch advanced to `f817823` with a narrow protocol regression:
terminal-diagnostic pair validation now recomputes and binds the comparison
JSON SHA-256, and the focused contract rejects a mutated comparison digest.
The clean full test and static checks pass at this exact tree. This is not a
performance import and does not change simulator economics or historical
results. A new exact-tree independent Sol-xhigh review remains required before
the pinned build/activation boundary.

## Append-only feed checkpoint: no new commits; terminal evidence review repair — 2026-09-08

Fetching `origin/autoresearch/v2-performance-research` again found no commit
after the last-reviewed marker `b1847ac`. The deferred `f153e12`
reaction-analyzer correction and VNext binary-evidence prototype remain
unmerged and did not affect this checkpoint.

At the scientific promotion boundary, independent Sol-xhigh reviewer Faraday
rejected exact tree `a2b3cc1`. One finding was a semantic analyzer admission
bug, not performance work: legal CDF supplier waits with `loss_limit` and
`equity_unavailable` were rejected. `5d98985` adds regression coverage and
acceptance for those registered reasons. The second finding was a terminal
diagnostic contract weakness: the fixture did not use a valid sealed evstream
and the validator lacked renderer-backed reconstruction and several exact
identity bindings. The scientific branch repaired this in `c0dc4c5`, `f5d2ee0`,
`aa4d4f7`, and `0475c74`; no performance-branch code was imported.

The repaired terminal contract builds real Go 1.27 test binaries, generates a
valid `evstream_v3` stream with a completion trailer, independently renders
each terminal arm, and rejects provenance/tree/renderer/resource/comparison/
outcome and stream-truncation mutations. Clean full tests, vet, shell syntax,
diff-check, and bounded race/evidence gates pass at `0475c74`. No activation,
capacity, development, freeze, or holdout run occurred. A fresh exact-tree
review remains required; the performance feed is still deferred at `b1847ac`.

## Append-only feed checkpoint: no new performance commits; Godel protocol rejection — 2026-09-08

`origin/autoresearch/v2-performance-research` was fetched again and has no
commit after the last reviewed marker `b1847ac`. The deferred `f153e12`
reaction-analyzer correction and the VNext binary-evidence prototype remain
outside the scientific branch.

The fresh independent Sol-xhigh review of exact scientific HEAD `9b9abc4`
rejected the current promotion boundary for two protocol reasons, not for
performance work: terminal-pair validation accepted a one-row checkpoint file
that the real runner rejects, and successful activation provenance did not
validate its recorded renderer identity. These findings were independently
reproduced. Scientific commit `8953b51` adds shared checkpoint validation to
all SV1B arms, makes the terminal fixture runner-compliant with four negative
checkpoint mutations, and pins/binds the successful-path renderer identity.

The repair does not alter R2 economics, calendar/lifecycle behavior, CDF
supplier logic, or historical evidence. Clean focused contracts and full
`make test` passed at `8953b51`; no simulator activation, capacity probe,
development cell, freeze, or holdout was run. A new exact-tree independent
review is required before promotion. No performance patch was merged.

## Append-only feed checkpoint: Russell checkpoint-integrity rejection — 2026-09-08

The performance branch was fetched through unchanged marker `b1847ac`; no new
performance commit was found and no performance implementation was imported.

Independent Sol-xhigh reviewer `Russell` rejected scientific HEAD `305a01c`
for a new protocol issue: the checkpoint validator accepted non-integer
counters, non-`evstream_v3` representation, nonzero unencodable payloads,
mismatched rolling hashes, and terminal count/hash values inconsistent with
the binary attestation. The finding was independently reproduced and does not
alter R2 economics or historical evidence.

Scientific commits `7980619` and `6016dc9` add strict SV1B binary checkpoint
validation, final-row/attestation binding, integer binary frame validation,
and effective mutation coverage while preserving legacy JSON callers. Clean
focused/full gates passed; no activation, capacity, development, freeze, or
holdout run occurred. Another exact-tree independent review remains required.

## Append-only feed checkpoint: no new commits; exact checkpoint precision repair — 2026-09-08

`origin/autoresearch/v2-performance-research` was fetched again with
`b1847ac` as the last-reviewed marker. There are no newer commits; the
performance branch remains an asynchronous red-team feed only. Its deferred
`f153e12` analyzer correction and VNext binary-evidence work were not merged.

At the scientific promotion boundary, independent Sol-xhigh reviewer
`Aristotle` rejected exact scientific tree `efeaaf7` because jq's floating-point
`floor` check could accept a fractional production-epoch checkpoint timestamp.
This was independently reasoned as a protocol verifier defect, not a
performance or simulator-semantic change. `cc71a49` moves strict checkpoint
number and duplicate-key validation into a Go library/adapter using exact JSON
number lexemes, binds strict terminal checkpoints to binary attestation, and
adds the effective production-epoch fractional regression. Clean full tests,
vet, terminal contract, and targeted race/evidence checks passed. No scientific
or holdout run occurred and no historical artifact was rewritten.

The current successor remains closed at the fresh-review boundary: obtain one
new exact-tree Sol-xhigh review of R2/CDF semantics, correctness hardening, and
the binary evidence contract before pinned builds or activation.

## Append-only feed checkpoint: Bacon rejection and pinned-validator repair — 2026-09-08

The performance branch was fetched through unchanged marker `b1847ac`; no new
performance commit was found and no performance implementation was imported.

Independent Sol-xhigh reviewer `Bacon` rejected exact scientific tree
`afbbdbe0a16750704f28828cd190e3d172ab0979`. The checkpoint helper allowed the
environment-selected `V2_R2_CHECKPOINT_GO` executable, so `/bin/true` could
bypass strict binary checkpoint validation. The exact helper was also absent
from successor capacity/cell revalidation paths and used mutable `go run`.

The scientific repair is `8fbb725`: a canonical, registered
`cmd/checkpointvalidate` Go 1.27.0 binary is pinned by package, revision,
reproducible-build metadata, and SHA-256, rechecked at every invocation;
activation, capacity, 24-hour cell, and extraction paths use exact validation;
and arm/pair/capacity/run provenance binds the validator identity. `66206cb`
adds the direct override/unregistered-validator regression. Clean full tests,
vet, focused suites, and targeted race checks pass at `66206cb`. No SV1B or
holdout run occurred; a new exact-tree Sol-xhigh review remains required.

## Append-only feed checkpoint: no new performance commits; SV1B capacity identity repair — 2026-09-08

The performance branch was fetched with `b1847ac40e8b7483e6e8a3f94b3705b4058884b0`
as the last-reviewed marker. There are no newer commits, so no new report or
diff was imported. The deferred analyzer correction and VNext binary-evidence
prototype remain outside the scientific branch.

Exact scientific HEAD `5766dff` exposed a local successor-manifest mismatch:
the pinned-validator contract script required capacity contract v5 while the
committed manifest still named v4. Aligning the field then revealed the
renderer SHA-256 also needed refreshing. Commit `235babf` records the exact
renderer digest and v5 identity; registered config bytes and economics are
unchanged. Both SV1 24-hour configuration checkers pass. This is a protocol
provenance repair only, with no activation/capacity/development/freeze/holdout
run. A fresh exact-tree Sol-xhigh review remains required before promotion.

## Append-only feed checkpoint: no new performance commits; Gauss accepted scientific tree — 2026-09-08

The performance feed remains unchanged after reviewed commit `b1847ac`; no
performance implementation was imported. Fresh independent Sol-xhigh reviewer
`Gauss` inspected exact scientific HEAD `67924ed` and returned **ACCEPT WITH
NONBLOCKING NOTES**, with no verified blocker and no experiment rerun required.
The review ran no simulator or holdout. The scientific successor is now at the
pinned-build boundary; the next permitted world is the paired seed-643
activation probe, subject to its own evidence review.

## Append-only feed checkpoint: no new performance commits; activation config identity repair — 2026-09-08

The performance branch was fetched through unchanged marker `b1847ac`; no new
commit or report was found and no performance implementation was imported.

The first valid SV1B seed-643 preflight on scientific tree `07b394b` rejected
the hand-authored activation config before simulation because the pinned
simulator's effective JSON was not byte-identical. The retained external
diagnostic root contains no world result. Scientific commits `658412e` and
`b11f21a` added the regression, canonicalized both treatment/control configs,
and updated the registered hashes; the final config checker and clean
mechanical gates pass at `b11f21a`. This is a scientific protocol/provenance
repair only. The prior review/build artifacts are not reused; fresh exact-tree
review and pinned rebuild remain required.
