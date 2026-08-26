# V2-6 P6-R1 — cross-asset collateral-mark viability

Status: **frozen before the P6-R1 repair cells**.  This is a bounded
instrument/valuation viability screen.  It does not reopen the completed P6
O0/O1/O2 activation results and it does not license an option-surface causal
claim by itself.

## Why this protocol exists

The preserved incomplete P6 O3/O4 worlds fail at the simulator's strict
terminal risk boundary, not at evidence extraction.  In the failed venue a
CDF/USD book becomes durably one-sided.  At the same time, the CDF maker's
ordinary sell-side request is rejected with `PRICE_UNAVAILABLE` while
cross-margin auto-borrow tries to value CDF.  The inherited borrowing oracle
contains USD and ABC but no CDF entry.  Thus a non-zero CDF balance cannot be
used as collateral for an ordinary sell that needs borrowing, and a durable
one-sided book then leaves strict marked-account capture without a usable CDF
mark.

This is a configuration/economic contract omission in the cross-asset
population, not evidence loss and not a reason to substitute zero, last trade,
or a remote/global price.  The failed O3/O4 directories remain historical and
are not P6 evidence.

## Hypothesis and intervention

**Hypothesis:** an explicit positive bootstrap CDF/USD collateral mark and a
finite CDF borrow cap will let the declared CDF maker collateral path operate
without changing actor information, quote references, matching, clocks,
spreads, liquidity, option participants, or evidence semantics.  Dealer-risk
capture should require CDF only when the captured dealer account actually has a
CDF balance/debt/collateral or a position whose quote asset is CDF.

The sole implementation delta is a new opt-in configuration contract:

* `cross_asset_collateral_marks=true` requires `cross_asset_spot_graph=true`;
* CDF/USD collateral is marked at the inherited bootstrap value
  `3,000 USD/CDF` (fixed-point `mvCDFBootstrap`);
* CDF collateral precision is the inherited base precision;
* CDF maximum cross-margin borrow is the inherited base-asset cap of
  `20,000` units (`20_000 * mvBasePrecision`), matching the existing ABC cap;
* the default remains disabled, so all pre-R1 configurations retain their
  exact contract;
* this mark is used only for collateral authorization/accounting.  It is not a
  maker fair-value/index input and is not delivered to any participant.

The finite cap is an ex-ante balance-sheet policy, not a value selected from
the failed worlds: it is the existing per-base-asset borrow limit applied to
the newly declared cross-asset collateral asset.  It prevents the repair from
turning an omitted CDF limit into unlimited credit.

## Cells and held-fixed inputs

The development cells are a fresh P6-R1 family, O0–O4 × seeds 211 and 213,
using the existing eight-hour P6 horizon, full evidence, receipt/frontier
sidecars, and final `greeks.json`/`latency.json` completion sentinels.  Every
P6 field is byte-identical within a seed and stage except the new explicit
collateral-mark field and experiment/provenance identifiers.  The historical
P6 files are immutable; R1 configs live in a separate directory.

No P6 holdout seed (223, 227, or 229) is consumed for debugging or viability
repair.  Holdouts remain unauthorized until a complete, identifiable staged
screen exists under the R1 contract.

## Required activation and integrity observables

Before interpreting any stage, the independent extractor must establish:

1. final completion sentinels and successful simulator exit;
2. exact runtime/offline persisted-evidence event-count and digest equality;
3. receipt/frontier, conservation, position/fill, lifecycle, settlement and
   expiry checks;
4. CDF maker sell-side borrowing attempts, explicit collateral-price lookup,
   and borrow-limit outcomes;
5. no CDF collateral mark is requested for a dealer account with no CDF
   balance/debt/collateral or CDF-quoted position;
6. strict population marked accounts have an explicit CDF mark whenever a
   participant actually carries CDF exposure;
7. no actor receives the collateral oracle or any hidden/global price;
8. fresh-process deterministic execution and evidence hashes.

The first four items are activation/integrity diagnostics.  They are not a
claim that the option surface or basis became more realistic.

## Falsification and classification

* **SUPPORTED (screening):** both development seeds complete the declared
  evidence contract, the explicit CDF collateral path is exercised, and the
  stage's registered participant-activation gates pass.
* **NOT IDENTIFIED:** the CDF path is not exercised, or a required stage is
  incomplete, even if another stage completes.
* **FALSIFIED AT VIABILITY:** the explicit mark/cap is active but the declared
  CDF account still cannot execute for a reason within this contract, or strict
  accounting fails.

No R1 result may be used to claim O3−O2 or O4−O3 option-surface causality unless
both stages are newly paired, valid, and their separate option deltas are
scored under a preregistered R1 comparison.  The original incomplete P6 O3/O4
cells remain historical only.

## Adversarial tests before long cells

* Remove the CDF oracle entry: a CDF auto-borrow must fail explicitly with the
  collateral-price error, never with a numeric-zero fallback.
* Set the CDF mark to zero or negative: the positive collateral-domain error
  must be preserved.
* Exceed the finite CDF cap: the borrow must reject without mutating wallet or
  debt state.
* Make a dealer hold no CDF while the venue has a CDF/USD mark history: dealer
  risk capture must not demand an unrelated CDF mark.
* Give a dealer a CDF balance or an ABC/CDF position: missing CDF valuation
  must fail closed.
* Toggle evidence recording and compare execution hashes; telemetry must not
  alter scheduling, RNG, matching, or actor decisions.

## Stop rule

If R1 only makes the option stages run but does not activate the declared
participant or evidence gates, record the stage as **NOT EXERCISED**.  Do not
change demand, liquidity, clocks, spreads, funding, collateral cap, or option
parameters after seeing outcomes.  A complete R1 viability screen is a
prerequisite for any later P6 holdout decision; it is not itself a market-level
success criterion.
