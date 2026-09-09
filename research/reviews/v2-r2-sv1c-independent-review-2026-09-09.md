# SV1C independent promotion review — 2026-09-09

Reviewer: Lovelace, `gpt-5.6-sol`, xhigh reasoning. The reviewer inspected
the clean exact candidate tree, source changes, registered amendments, retained
raw activation evidence, and corrected analysis-only replay without modifying
the repository or running a registered world.

## Exact objects reviewed

* Candidate worktree: `/home/vlad/ExchangeSimulation-r2-sv1c`
* Branch: `feature/r2-cdf-survival-successor`
* HEAD: `7aa9fe27f2fb1b40435bfbba98cd4f6c10fa4d72`
* Git tree object: `b2a902a38287c9c4404dab4ce3d12f02f48b746a`
* Retained raw pair: `/home/vlad/external-scratch/v2-r2-sv1c-activation-643-7ca80ac65067400bbfa2b8ab53fc7793257a77de`
* Final replay: `/home/vlad/external-scratch/v2-r2-sv1c-activation-rescore-643-7aa9fe2`
* Replay attestation SHA-256: `407b0534ddc73a671a22430153cf450c7c9b4ef9e45b08f3b9ef9514611d0072`

The raw treatment and control event streams are respectively
`4e69c909f860a79496c86f8a043069f9511c23c6b8cd6e82822db27949ea3b60` and
`9d1db327295485e2940de28acbbdd1d54009d4b6a7f9199544fc104d2f580f34`.
The original invalid comparison remains preserved with SHA-256
`9261bd175e1c86674128205caa0e5cbac5fde1a2b6eee58058da311e589e2b6c`.

## Verdict

**REJECT SV1C for scientific promotion.** Accept the global-order analyzer
correction as analysis-only/rescore-only, and accept the corrected replay as a
valid negative activation decision. Do not authorize capacity seed 659, the
24-hour development cells, freeze, or holdouts 619/631/641.

This is not conditional. Relaxing the all-supplier activation predicate,
retuning the supplier roster, or rerunning to obtain a favorable result would
be a new successor candidate and requires new preregistration and review.

## Findings accepted by the reviewer

* The R2 calendar amendment is mechanically consistent: calendar-derived
  expiry identity, separate deterministic listing policies, collision
  deduplication, cursor advancement, and shared futures/options expiries. The
  registered schedule is 1h/2h, 3h/6h, and 6h/12h listing-cadence/expiry-lead
  pairs.
* Strict-risk and account-scope liquidation hardening is present and its
  focused tests pass: zero-exposure books are skipped, one unpriceable account
  does not abort the client scan, coherent mark epochs are required,
  settlement-pending exposure fails closed, and same-quote liquidation is
  account-scoped.
* CDF suppliers are separately configured and finite, with bounded balances,
  inventory, positions, quote sizes, and loss budgets; delayed local
  observations; no borrow, recapitalization, global oracle, or mandatory
  two-sided quoting. Raw evidence reports `max_borrowed=0`.
* Routed-event reconstruction correctly uses canonical global sequence across
  evidence files. The original analyzer produced 869 false rest-state
  violations; the corrected replay removes those violations while preserving
  substantive event counts.
* Source and analyzer provenance are explicitly separated: raw source is
  `7ca80ac65067400bbfa2b8ab53fc7793257a77de`, analyzer source is
  `7aa9fe27f2fb1b40435bfbba98cd4f6c10fa4d72`, mode is
  `analysis_only_replay`, and `analyzer_source_modified=false`.
* The correction is rescore-only. No simulator trajectory or economic
  configuration was repaired, and all 68 raw artifacts match their registered
  hashes.

## Promotion blockers

1. Only 7 of 12 supplier/venue instances activated. Five had fills and PnL
   changes but no post-fill inventory-responsive decision:
   `(24,central)`, `(26,central)`, `(26,north)`, `(24,south)`, and
   `(26,south)`.
2. Supplier-removal reconstruction covers only 900 of 939 snapshots, so
   `supplier_removal_counterfactual_valid=false`,
   `supplier_removal_time_weighted_counterfactual_valid=false`, and
   `anti_cheating_satisfied=false`.

The treatment totals are 1,791 decisions, 203 accepted quotes, 95 fills, and
174 cancellations. The corrected evidence is valid, but the preregistered
activation predicate is not satisfied.

## Limitations and next safe action

The five-minute probe contained no liquidation events and no explicit
withdrawal event; liquidity reduction occurred through cancellations/repricing.
Liquidation and loss-limit withdrawal behavior remain source/test validated but
were not activated in this pair. The replay CLI does not itself prove Git
ancestry; the external attestation supplies it and the reviewer independently
verified that `7ca80ac` is an ancestor of `7aa9fe2`.

SV1C should be archived as **VALID EVIDENCE / NEGATIVE ACTIVATION**. Further
simulation requires a separately named, preregistered successor mechanism,
followed by exact-tree review and a new development-only activation gate.
