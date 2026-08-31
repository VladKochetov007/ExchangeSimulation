# V2 successor design: CDF/USD market-survival amendment

Status: **design decision pending; no simulator, configuration, binary, or
registered cell changed.** This document follows the failed non-cell 24-hour
capacity probe recorded in
`v2-r5-binary-evidence-successor-2026-08-31.md`.

## Problem boundary

The current R2 successor keeps all eight `elastic_supplier` participants on
`ABC/USD` because `elastic_supplier_symbols` is unset. It also has two
inventory-skewed CDF/USD spot makers and a finite CDF liability hedger per
venue. In the isolated dev-607-shaped 24-hour probe, South CDF/USD remained
one-sided for about 11.9 simulated hours while the liability hedger retained a
short CDF position. Strict terminal accounting correctly rejected the run.

This is a market-survival failure, not an evidence-format failure and not a
reason to weaken the mark contract. A new intervention would be an explicit
economic successor, not a mechanical R2 correction. The calendar/lifecycle,
risk, binary evidence, and historical JSON contracts remain fixed.

## Evidence available before a new decision

The retained V-002 causal ablation found that the CDF runaway requires the
combination of inventory-skew amplification and absent level-caring demand.
Its price-elastic-supplier arm used the same existing supplier family with
four of eight suppliers cycling on `CDF/USD`; the 24-hour CDF/USD terminal
ratio was 2.07x for seed 101 and 1.05x for seed 103, versus 49.41x and 45.00x
for the control. That is a prior about a mechanism, not a guarantee for the
R2 population: the V-002 configuration, maker/reference setup, provenance,
and seed are different.

The retained P2 screen also shows why inventory rebalance alone is not a
sufficient repair. P2's capped IOC action is local and costly, but it can only
trade against an available contra-touch. It cannot create aggregate CDF supply
when the book is already bid-only.

## Candidate interventions

| candidate | economic change | scientific advantage | principal risk |
| --- | --- | --- | --- |
| R2 unchanged | none; classify the 24h candidate as non-viable | preserves the exact registered ecology and gives an honest negative survival result | no 24h capacity attestation, dev-607, or holdout program can proceed under this candidate |
| reassign existing suppliers | cycle some of the existing eight between ABC/USD and CDF/USD, as V-002 did | smallest code/config change and directly grounded in prior causal evidence | changes ABC/USD supply and the composition of the existing population; not a clean preservation of R2 |
| dedicated CDF suppliers | retain the eight ABC/USD suppliers and add a separately named finite CDF/USD elastic-supplier roster | isolates the intended missing damping mechanism while preserving the existing R2 population; uses delayed local book observations | adds participants and balance-sheet supply, so it is a material ecology amendment requiring new preregistration and scoring |
| remove inventory skew | disable or scope out the CDF maker inventory-skew amplifier | removes the identified feedback amplifier | removes a real maker risk response and changes the maker mechanism rather than testing ecology composition |
| mark fallback | use last trade, stale mark, remote mark, or bootstrap value | would let the run finish | economically invalid for live CDF exposure and contradicts the fail-closed risk contract; rejected |

The dedicated-roster candidate is the cleanest successor if the scientific aim
is to study a viable multi-asset ecology rather than to retune the existing
R2 run. It should not be applied to the current R2 configs silently.

## Conditions for a defensible dedicated-roster successor

If authorized, the implementation should expose a generic configurable roster
of elastic-supplier policies rather than hard-code a new CDF branch. Each
roster entry must declare its symbol, finite balances, reference initialization
and adaptation, elasticity, position limit, rebalance lot, cadence, role
identity, and evidence settings. The existing eight ABC/USD entries must remain
byte-identical in the successor control. New CDF entries must:

1. trade only from delayed local CDF/USD snapshots;
2. have finite CDF/USD and USD balances and the existing borrowing limits;
3. use no global index, last-trade fallback, or hidden instantaneous reference;
4. retain explicit evidence of decisions, requests, fills, fees, positions, and
   terminal mark age;
5. be added symmetrically to every venue;
6. preserve the accepted R2 calendar, derivative lifecycle, risk, ordering,
   and binary evidence semantics; and
7. be compared with a matched no-new-roster control under a new immutable
   successor configuration.

A fixed bootstrap reference with zero adaptation would be an exogenous price
anchor. It must not be introduced merely to make CDF/USD survive. If the
elastic supplier is used, its private reference adaptation and half-life must
be preregistered before any successor outcome and treated as part of the
economic mechanism.

## Required promotion sequence for an amended successor

No unregistered viability run is authorized by this design note. A defensible
sequence would be:

1. choose and preregister the roster and its private-reference policy;
2. add focused unit, information-boundary, accounting, and evidence-neutrality
   tests without changing current R2 behavior when the roster is empty;
3. obtain independent review of the exact semantic/configuration amendment;
4. regenerate the immutable successor configs and clean Go 1.27 binaries;
5. run only its registered development cells and controls, beginning with
   dev-607; and
6. repeat the full binary capacity measurement before any cell launch if the
   candidate completes strict terminal accounting.

The existing current-R2 failed probe is not rescored under any amended
successor. No holdout is touched during design, and the current R2 configs and
historical results remain immutable.

## Decision

No option in this document is selected implicitly. The current scientific tree
therefore remains unchanged and stopped before capacity attestation. A new
CDF liquidity/inventory policy requires explicit scientific authorization as a
successor candidate; absent that authorization, the correct result is to
retain the current R2 non-viability finding rather than alter the economics or
weaken risk accounting.
