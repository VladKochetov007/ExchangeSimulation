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
