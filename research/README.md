# Research navigation

This repository is the authoritative **development** baseline after the
repository consolidation recorded in
[`REPOSITORY-CONSOLIDATION.md`](REPOSITORY-CONSOLIDATION.md). It is not a
scientific freeze and it is not a certificate of market realism.

## Current development state

The consolidated code baseline is the former scientific tip
`f507c7ee2b11fd05e1adb9f17fa0a66faf889ea1`, descended from the published
`main` tip `ffe1434cfc60b5f79b5289b610d3d5137286f514`. The consolidation adds
navigation and provenance records only; it does not enable an experimental
roster or change the default economics.

The single current operational pointer is
[`RESUME-HERE.md`](RESUME-HERE.md), whose current entry closes the R2/SV1D
development line. The companion audit is
[`V2-CURRENT-STATE-AUDIT.md`](V2-CURRENT-STATE-AUDIT.md).

## Closed research lines

- [R2/SV1D iteration closeout](v2-r2-sv1d-iteration-closeout.md) — the R2
  predecessor is `NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`; the corrected
  retained seed-659 treatment is evidence-valid but
  `TREATMENT_NOT_ACTIVATED`. The supplier traded, but the registered
  restoration predicate was not met.
- [Machine closeout summary](artifacts/v2-r2-sv1d-closeout-summary.json) —
  preserves original-run, analyzer, evidence, and review identities.
- [Next-study design brief](market-ecology-next-study-brief.md) — explicitly
  **NOT AUTHORIZED TO RUN**.
- [R2 calendar amendment](v2-r5-r2-calendar-amendment-2026-08-30.md) — the
  calendar is mechanically specified and tested; no completed 24-hour ecology
  campaign is implied by that result.

No `dev-607`, `dev-613`, `dev-617`, parity, freeze, or holdout
`619/631/641` was launched from the closed boundary. Historical results retain
their original source, configuration, analyzer, and evidence identities.

## Implemented platform and remaining uncertainty

The development baseline contains the reusable exchange library and research
adapters for matching, balances, margin, funding, borrowing, liquidation,
settlement, delayed gateways, market data, derivatives, multi-venue actors,
and canonical/binary evidence streams. The current R2 lifecycle contract uses
calendar expiries with short/medium/long listing policies of 1/3/6 simulated
hours and 2/6/12 simulated hours to expiry; schedule collisions are
deduplicated by `underlying + contract type + expiry`.

These are software mechanisms and contracts, not evidence that the simulated
ecology produces realistic prices, liquidity, option surfaces, funding basis,
or survival. The closed SV1D probe does not identify a general limitation of
finite liquidity suppliers, and the proposed capacity/composition study still
requires a separate owner decision and preregistration.

## Historical lines and deferred contributions

The consolidation record and machine summary account for the divergent SV1C,
SV1D, latency, performance, port, and economic-red-team branches. Their
commits remain reachable through named branches, remote refs, or private
archival refs; they were not merged wholesale into `main`. Performance and
red-team work is deferred unless a future, separately authorized change
adopts a specific contribution.

Large retained evidence and private review bundles remain at their original
external paths. Git contains compact reports, manifests, and hashes; those
records do not claim that external evidence is backed up or byte-verified by
this repository.

## Ordinary development checks

From a clean checkout, inspect the target before running it. The ordinary
mechanical gate is:

```bash
make test
make vet
go test -race ./analysis ./cmd/mvanalyze ./cmd/prunegate ./cmd/sv1dprobe ./cmd/sv1dresource ./tests -count=1
git diff --check
```

`make test` runs Go tests and the four integrated-long-run contract/archive
fixture scripts; those scripts construct temporary fixtures and do not launch
registered cells. Commands and scripts named `run-v2-*`, `capacity`,
`activation`, `holdout`, `dev-607`, `dev-613`, or `dev-617` are research
campaign adapters, not ordinary integration checks. Do not invoke them under
this development baseline without an explicit study authorization.

Use the project’s pinned Go toolchain where provenance matters. Python in
`.venv` is for visualization/supporting inspection only; data-intensive
simulation and analysis remain in Go.

## Reproducing a retained result

The bounded SV1D rescore can be reproduced from the retained raw namespace and
the exact Go 1.27 analyzer/renderer bundle using the commands in section 9 of
[`v2-r2-sv1d-iteration-closeout.md`](v2-r2-sv1d-iteration-closeout.md). It
does not require a simulator rerun and does not overwrite the original invalid
score.

## Safe checkout

```bash
git clone https://github.com/VladKochetov007/ExchangeSimulation.git
cd ExchangeSimulation
git fetch origin --no-prune
git checkout main
git worktree add ../ExchangeSimulation-work main
```

The public checkout reproduces source, tests, reports, and compact manifests.
Private external evidence and review bundles require the recorded local paths
and access permissions; their absence must not be silently replaced by a new
run.

