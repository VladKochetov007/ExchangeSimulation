# r5 amendment: risk-semantics correction before dev-607

Date: 2026-08-30
Status: blocking amendment to `research/v2-integrated-longrun-r5-protocol-2026-08-28.md`.
No freeze authorization, no holdout authorization.

## Why the r5 candidate is blocked

The r5 protocol registered its execution order as: commit the amendment, get an
independent review, build from a clean go1.27 worktree, then run only dev-607
and review it. Before dev-607 was launched, an independent market-logic audit
from the isolated performance branch was triaged against the exact scientific
HEAD `887899f`. The triage is
`research/v2-integrated-longrun-r5-risk-triage-2026-08-30.md`.

One finding is not merely latent. `buildAccountMarginProfile` resolved a mark
for every same-quote margined book *before* establishing whether the account
held anything in it, so an unmarked book failed the whole profile and the
account's liquidation decision was discarded. In the **registered** dev-607
configuration this fires at every dated-future listing instant, because the
listing scheduler runs inside the expiry automation job and the option
liquidation sweep runs in that same job — one tick before the price job can
mark the new contract.

Measured over 6h30m of dev-607 seed 607, three venues, `log_mode=full`:
**917 liquidation-eligibility evaluations discarded**, on exactly two simulated
seconds (t=7201 s and t=21601 s), every record reading
`cross-margin mark for ABC-FUT-…: no usable price`. A 24-hour cell has roughly
twelve such instants per venue.

That is inside the stop rule for liquidation eligibility, so the candidate is
blocked and the defect is repaired before any registered cell runs. No
liquidation was actually missed in the measured window — dev-607 produced zero
liquidations of any kind — but the evidence cannot say what the discarded
decisions would have been, so it is not rescorable.

`dev-613` and `dev-617` differ from `dev-607` only in `experiment_id` and
`seed`, so all three registered cells were affected identically.

## What changed in the simulator

One scientific commit, three changes, all in `exchange/exchange.go`. No
performance code was merged or cherry-picked from
`autoresearch/v2-performance-research`; the performance commits `af8535d`,
`57077e9` and `2989934` remain unintegrated.

1. `buildAccountMarginProfile` establishes position relevance before resolving
   a mark. Fail-closed is retained for held exposure whose mark is genuinely
   unavailable.
2. `CheckLiquidations` `continue`s rather than `return`s on a profile failure,
   matching its sibling `CheckPositionMarginerLiquidations`. One account's
   unpriceable exposure no longer cancels every higher-ID account's decision.
3. `updateAllPerpPrices` commits every mark the tick produced and then sweeps
   once, instead of interleaving mark application and sweep per symbol. A
   cross-margined leg is no longer valued at the previous tick's mark, so a
   liquidation outcome no longer depends on lexicographic symbol order. Mark
   and funding publication order is unchanged.

Two findings are recorded and deliberately left as they stand: settlement-
pending positions being invisible to risk (mechanically unreachable in the
registered world, now written into `docs/realism-gaps.md`), and the borrow
interest accrual floor (already documented there, and absent at r5 debt scale
after warm-up).

## Provenance consequence

* The simulator revision changes, so every r5 binary identity is superseded.
  Nothing built from `887899f` may be used for a registered cell.
* Any dev-607 evidence produced under the previous semantics is superseded, not
  rescored. None had been produced in the v5 root for this candidate.
* The v4 preserved archive and its SHA-256 are untouched by this amendment.
* No holdout was read, listed, parsed or run: not
  `research/artifacts/v2-7-p7d/holdout`, and not the configs `holdout-619.json`,
  `holdout-631.json`, `holdout-641.json`.

## Registered execution order for the successor candidate

1. Commit this amendment plus the semantic fix and its regression tests, with
   `make test` and `scripts/test-v2-integrated-longrun-contract.sh` passing.
2. Obtain an independent high-effort review of the committed semantic change —
   specifically of the three risk-path changes and of the claim that mark and
   funding publication order is unchanged.
3. Build simulator, analyzer and prune-gate binaries from one clean,
   provenance-pinned Go 1.27 worktree with `-trimpath`, `CGO_ENABLED=0` and
   `vcs.modified=false`.
4. Run only `dev-607` in a successor v6 root, extract it, and independently
   review the exact output. Confirm as part of that review that the evidence
   contains **zero** `price_unavailable` records with `operation` of
   `liquidation` or `option_liquidation`, which is the direct acceptance test
   for the F1 repair.
5. After that review accepts the dev-607 gate, run only registered `dev-613`
   and `dev-617`, plus the registered 607 G4/G8/no-log controls and the parity
   check. No holdout directory is read or launched.
6. A separate freeze authorization artifact must precede any use of holdouts
   619, 631 or 641.

Everything else in the r5 protocol — the evidence observability changes, the
fail-closed analyzers, the strict position/settlement predicates, the manifest
and sibling attestation — carries forward unchanged. This amendment changes
simulator risk semantics only.
