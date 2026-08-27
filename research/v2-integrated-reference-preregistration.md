# Integrated V2 reference configuration: pre-run contract

Status: **pre-registered before integrated-world outcomes**  
Date: 2026-08-27

## Purpose

The long FROZEN-2 candidate gate established exact execution/evidence
reproducibility for a historical control, but that control did not enable the
V2 mechanisms already screened individually.  This document defines the
smallest integrated reference composition needed for a compatibility and
freeze-preparation gate.  It is not a stylized-fact tuning target and it does
not assert that any market-level effect will occur.

## Base and sole overlay

The base is the registered P6-R1 O4 configuration
`research/configs/v2-6-p6r1/O4-223.json`, with its seed and experiment identity
replaced by the development identity in
`research/configs/v2-integrated/reference-dev-601.json`.

The only economic/information overlay is copied exactly from the registered
V2-2b I1R1 configuration (`research/configs/v2-2b/I1R1-101.json`):

- local maker cache enabled and maker anchor set to `own_mid`;
- the three declared remote ABC/USD maker feeds, including their source
  venues, weights, confidence, age limits, and constant delays;
- one cross-venue router tier, 1-second router latency, 1,000,000-lot edge
  size, and 100 maximum attempts;
- constant 10 ms spot-maker market-data latency;
- decision-frontier recording enabled;
- the union of receipt roles required by the O4 actors, remote feeds, and
  router.

All O4 values remain unchanged: post-only and cancel-before-replace passive
refresh, inventory-size/rebalance policy, CDF liability hedger, option
liability user, cross-asset collateral authorization, one SABR value taker,
one Vanna--Volga desk, ordinary six-member metaorder population, venue rules,
funding, fees, and clocks.  P7d directional distress is deliberately not
unioned into this normal reference; its C/L/S configurations remain the
registered stress overlays.

## Ex-ante classifications

| choice | classification | rationale |
|---|---|---|
| O4 base population and numeric values | inherited | last viable staged option ecology with complete holdout activation evidence |
| P2 passive/size/rebalance values | inherited | last registered V2-3 mechanism values; no post-outcome tuning |
| I1R1 feed/router values | inherited | registered participant-local information and executable-router contract |
| development seed 601 and 5-minute smoke horizon | design choice | fresh non-holdout development identity; long enough to cross startup marks while cheap enough for compatibility checks |
| full evidence, 60-second checkpoints | design choice | required to audit receipts/frontiers and execution/evidence hashes; observational only |

## Required pre-freeze checks

The first integrated screen is a 5-minute full-evidence/no-log pair from the
same clean simulator binary, followed by a fresh-process repeat.  It must
verify, without market-level interpretation:

1. constructor/config validation and no unknown-field drift;
2. exact ordered execution-hash equality for full versus no-log and across
   allowed GOMAXPROCS values;
3. runtime/offline persisted-evidence digest equality for the full case;
4. all three remote-feed receipt/frontier chains obey delivery-before-decision;
5. router requests, account separation, and non-atomic leg evidence, while
   allowing a zero-action router to remain `NOT EXERCISED`;
6. post-only admission and cancel-before-replace evidence;
7. O4 liability, option, SABR, and Vanna--Volga activity classification;
8. strict population accounting, collateral authorization, and conservation.

The smoke cannot license a market-level realism, convergence, funding, or
emergence claim.  A failed activation or evidence contract blocks promotion
of this composition and is recorded without changing its values.  A passing
smoke licenses only a separately declared integrated long-run candidate; it
does not convert O3/O4 SABR/Vanna--Volga structure into an emergent finding.

## Explicit non-goals

No P7d directional participant, shock, capital, risk threshold, funding
intervention, option prior, spread, liquidity, clock, or latency value may be
changed after observing integrated outcomes.  No holdout seed is consumed by
this development smoke.
