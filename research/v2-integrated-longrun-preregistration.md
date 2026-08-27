# Integrated V2 long-run candidate: pre-run contract

Status: **pre-registered before long-run outcomes**
Date: 2026-08-27

## Purpose and scope

The integrated V2 reference smoke established a five-minute compatibility
contract.  This protocol is the next candidate gate requested by the
independent freeze-readiness review: it exercises the same integrated ecology
over a horizon long enough to expose periodic funding, contract listing/
expiry/settlement, cross-asset collateral, and late lifecycle paths.

This is a **development/candidate qualification** protocol, not a causal
ablation and not a realism score.  No parameter, population, clock, spread,
latency, or risk value may be selected or changed after inspecting these
outcomes.  A passing candidate only licenses construction of the later
immutable freeze bundle; it does not license a market-level claim.

## Frozen simulator composition

Each full-evidence cell is a byte-for-byte derived configuration from
`research/configs/v2-integrated/reference-dev-601.json`.  The economic and
information fields are inherited unchanged from the registered P6-R1 O4 plus
V2-2b I1R1 overlay.  Only provenance identity (`experiment_id`,
`hypothesis_id`, `description`) and `seed` differ between cells.  The parity
configuration is the corresponding derived `log_mode=none` configuration with
optional evidence recorders disabled; it is a logging/instrumentation control,
not an economic treatment.

No P4/P5 funding or dated-carry intervention, P7d distress overlay, option
prior, demand, liquidity, spread, clock, or latency value is introduced here.

## Ex-ante design choices

| choice | classification | rationale fixed before outcomes |
|---|---|---|
| O4 cumulative ecology plus I1R1 local-feed/router overlay | inherited | last integrated composition with completed activation/evidence smoke |
| 24-hour simulated horizon | free experimental-design choice | covers three 8-hour north funding periods and multiple six-hour future/option tenors while remaining a bounded candidate run |
| development seeds 607, 613, 617 | free experimental-design choice | fresh odd development identities selected before this protocol's outcomes; 601 remains historical smoke-only |
| untouched holdout seeds 619, 631, 641 | free experimental-design choice | fresh identities reserved before candidate outcomes; they are not consumed by this gate |
| full persisted evidence for all development cells | inherited contract | required for independent lifecycle, settlement, receipt/frontier, accounting, and risk reconstruction |
| 60-second execution checkpoints | inherited | existing integrated evidence contract; observational only |
| canonical `GOMAXPROCS=4` | inherited operational setting | existing deterministic campaign setting on this host; host parallelism is recorded, not an economic input |
| one seed-607 full/none/g8 parity set at the same 24-hour horizon | free experimental-design choice | a bounded long-horizon determinism/logging check; `g8` is a fresh-process parallelism check and is not a treatment |
| completion sentinels `greeks.json` and `latency.json` | inherited evidence rule | only final sidecars are accepted as run completion; process names or partial files are insufficient |

The development/holdout partition is fixed as follows:

* Development: full-evidence seeds **607, 613, 617**.
* Determinism/logging control: seed **607**, full `GOMAXPROCS=4`, no-log
  `GOMAXPROCS=4`, and full `GOMAXPROCS=8`, all 24 simulated hours.
* Untouched holdout reservation: **619, 631, 641**.  These seeds must not be
  run, inspected, or used for analyzer debugging until a later immutable
  freeze and a separately pinned holdout protocol.
* Seed 601 is prior development-smoke evidence only and is not counted as a
  long-run development replication.

## Required provenance and evidence contract

Every cell must be launched in a fresh directory using a clean binary built
from one declared source revision.  The directory must contain pre-run
`run-config.json` and `run-metadata.json`, and post-run `greeks.json` and
`latency.json` written only after successful simulator completion.  The runner
must record source revision, binary SHA-256, config SHA-256, Go toolchain,
`GOMAXPROCS`, horizon, command, and holdout status.

The fail-closed extractor must independently produce, at minimum:

* `evidenceartifacthash.json` and runtime/offline digest equality;
* `streamhash.json` with its explicit persisted-evidence domain label;
* observation receipts and frontier vectors;
* mechanical, conservation, positions, fill/position, order-lifecycle,
  lifecycle, expiry/fill, and settlement audits;
* option surface, option liability/value-taker/Vanna--Volga activity;
* maker refresh/rebalance, liability hedger, cross-venue, arbitrage, role and
  ecology reports;
* derivatives, liquidations, margin checks, and dated/funding diagnostics,
  including explicit zero-activity results where a path is not exercised.

Required artifacts are considered complete only when every registered file is
present, parses, has a complete `analysis-metadata.json`, and records the
analyzer revision/SHA.  Raw evidence is retained; this protocol grants no
prune authority.

## Candidate qualification predicates

The candidate is qualified only if all of the following hold:

1. All three development full cells exit successfully and contain both final
   completion sentinels.
2. Runtime and offline persisted-evidence event counts and digests agree for
   every full cell.
3. All required evidence and accounting/lifecycle analyzers pass their
   fail-closed integrity predicates.  A mechanism with zero activity is
   recorded `NOT EXERCISED`, never promoted by a generic non-empty artifact.
4. Seed-607 full, no-log, and full-g8 runs have identical ordered execution
   checkpoints and deterministic sidecars at the common horizon.  Persisted
   evidence content/digest is compared only for the full evidence pair; the
   no-log cell intentionally has a different evidence domain.
5. Receipt/frontier replay confirms delivery-before-decision for every audited
   role and no required evidence class is missing.
6. The complete provenance bundle is reproducible from the declared clean
   source revision and analyzer revision.

Failure of any predicate blocks promotion and is reported as an evidence or
mechanical defect, inactive mechanism, or unexplained nondeterminism according
to the exact failed predicate.  No numeric or economic rescue is permitted.

## Interpretation boundary

This candidate can establish that the integrated composition survives a
meaningful deterministic/evidence/lifecycle gate.  It cannot by itself claim:

* funding anchoring or basis convergence;
* dated-future carry or maturity convergence;
* executable trade-mediated price discovery;
* liquidation/bankruptcy reachability when no event occurs;
* endogenous option smile/skew (SABR/Vanna--Volga priors remain inherited);
* ecological realism, wealth concentration, or stylized-fact success.

Those questions require their existing causal protocols or the later frozen
holdout validation.  This protocol therefore has no market-level PASS/FAIL
thresholds and no post-outcome tuning rule.
