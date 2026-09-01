# V2-R2-SV1 24-hour development preregistration

Date: 2026-09-01  
Candidate: `V2-R2-SV1-24H-CDF-LIQUIDITY`  
Predecessor: R2 at `230e78fd202c78ea34ac3f0857089a2344a7cebd`  
Stage: development only; no freeze or holdout authorization

## Boundary and prior activation

The predecessor R2 candidate remains archived as **NON-VIABLE AT THE 24H
MARKET-SURVIVAL GATE**. Its configs, evidence, and conclusion are not
rescored under this candidate. The accepted R2 calendar/lifecycle semantics,
risk hardening, and binary evidence contract remain unchanged.

This is a separately named economic successor. It carries the existing eight
ABC/USD elastic suppliers unchanged and adds the finite, separately configured
CDF/USD roster already specified in
`v2-r5-cdf-liquidity-successor-preregistration-2026-08-31.md`. The roster is
off in every historical R2 config.

The five-minute seed-607 treatment/control activation probe under candidate
revision `d85bfb1bcac20a8c22b3d2629ecb5da83c17abd3` was independently accepted
for activation only. It showed 12/12 suppliers trading, PnL-changing, and
inventory-responsive, with finite quote cash reconciliation. The result did
not establish survival, 24-hour viability, or stressed withdrawal behavior.

## Mechanism hypothesis

Finite economically motivated CDF/USD liquidity suppliers, acting only on
delayed local observations and bearing finite inventory/cash/PnL risk, can
reduce persistent one-sided CDF collapse often enough that a full ecology
remains strictly valuatable over a 24-hour compressed research world.

The calendar does not encode a target price, spread, volume, survival result,
or valuation availability. No supplier has a two-sided quote obligation,
forced fill, forced replenishment, unlimited capital, or external live price
anchor.

## Registered cells

Primary development is three matched treatment/control pairs. Each pair uses
the same seed and the same base R2 configuration; the only economic treatment
delta is the preregistered CDF/USD supplier roster and its required decision /
receipt evidence settings. All primary cells use a 24-hour horizon, full
`evstream_v3` evidence, and `GOMAXPROCS=4`.

| cell | seed | population | purpose |
|---|---:|---|---|
| `treatment-607` | 607 | R2 + CDF roster | primary development treatment |
| `control-607` | 607 | R2, no CDF roster | matched counterfactual |
| `treatment-613` | 613 | R2 + CDF roster | primary development treatment |
| `control-613` | 613 | R2, no CDF roster | matched counterfactual |
| `treatment-617` | 617 | R2 + CDF roster | primary development treatment |
| `control-617` | 617 | R2, no CDF roster | matched counterfactual |

Two additional seed-607 treatment runs are registered only as instrumentation
parity controls after the primary `treatment-607` completes:

| cell | process/evidence mode | purpose |
|---|---|---|
| `treatment-607-g8` | `GOMAXPROCS=8`, full evidence | process-width execution parity |
| `control-607-none` | `GOMAXPROCS=4`, no persisted raw/evidence-only logs | matched no-CDF log-mode execution neutrality |

CDF decision recording requires persisted evidence by the simulator contract,
so a CDF-enabled no-log cell would be invalid rather than a valid parity run.
The no-log parity cell therefore uses the matched no-CDF control population.
The parity cells are not extra economic treatments and are not used as
independent seeds. `treatment-607-g8` must match the primary treatment on the
registered execution-stream and terminal-state parity predicates; the
no-log control must match `control-607` on the log-neutral predicates. If host
pressure makes `GOMAXPROCS=8` unsafe, the parity cell is deferred; it is never
run by silently changing the registered process setting.

No holdout, including `619`, `631`, or `641`, is in this namespace or may be
read before a separate freeze authorization.

## Fixed configuration delta

For each seed, the treatment config is generated from the corresponding
immutable R2 full config. The control is generated from the same source with
only successor identity metadata changed. The treatment additionally declares:

- the four CDF/USD supplier specs per venue through the generic configurable
  roster interface;
- `record_elastic_liquidity_supplier_decisions: true`;
- the `cdf_elastic_supplier` market-data receipt role.

No other economic field may differ between a treatment and its matched control.
The source R2 config hashes and activation-roster hash are recorded by the
config checker and in the generated provenance. The rendered effective config
must be byte-identical to the registered config before a run can start.

The source identities for this preregistration are fixed as follows:

| source | SHA-256 |
|---|---|
| R2 `dev-607.json` | `3fef36431a1a62d5a9b59aeda96798bff033cba8dac88fdebe2c30a73183e7bd` |
| R2 `dev-613.json` | `e5134fc9d4af3ab07326ea5bdc5639a965740e5e002bea989980e249108ef253` |
| R2 `dev-617.json` | `7d990c4aee55ee2d76b7041001f3fba89819635dec0d0f92c58385850a2de0fa` |
| accepted CDF activation roster | `1c9f1094b2b8619e3ad7965547dc34bf9ee9e134bc349a1f5deaaeb644cf94ff` |

The generated registered config hashes are: `treatment-607`
`0e267d1e737ed232564651b8f9e65b137d3f4ddb59235e826633f4b972ae35ae`,
`treatment-613`
`aec261dbe10407fe49bdd9ae1b5e4b3c58c85990050f76ceff75f146c2ffcc1c`,
`treatment-617`
`f9df3afd4d8e5ffe8a6828d2f7e98ad3d2c7eaf32eda3fce0b11041ce71ec8f2`,
`control-607` `b0c51e32a5811adc60af4bf4def5d521cb94a32ed93a3d22f892daff73fe914d`,
`control-613`
`2dd09a40f466b49f36fd6eafe3d91141b0ba3cab16fe237afca76e171de83bc0`,
`control-617`
`9347be95ae776f698895e0051bba386bff1d8024b795c0eec8c3e2c13bf6878c`, and
`control-607-none`
`9f9e363a6d434003cab9c223e039fd7e850d394e01ed1d52cdcc386210f9ee1d`.

## Required evidence and scoring

Every primary arm must pass the existing full mechanical, conservation,
position, order-lifecycle, derivative, calendar, settlement, margin, and
evidence contracts. Treatment arms must additionally pass the independent CDF
cash/inventory/PnL audit and retain its per-supplier diagnostics. A malformed,
missing, reordered, incomplete, or provenance-mismatched evidence stream is an
invalid cell, not a negative economic result.

The development report will separately record, by arm and seed:

- strict terminal valuation and side availability after warm-up;
- CDF supplier activity, volume/depth concentration, inventory, cash,
  borrowing, PnL, quote lifetime, cancellation, withdrawal, and repricing;
- treatment/control side-absence and terminal-mark comparisons where the
  paired evidence is identifiable;
- derivative/lifecycle activation and all existing integrity predicates.

The 24-hour result is not declared viable merely because a process exits zero.
No broad realism claim is licensed unless the relevant mechanism is active,
the evidence is valid, and the preregistered endpoint is identifiable.

## Kill criteria

Close this successor without holdouts if any primary treatment relies mainly on
mechanically guaranteed liquidity, hidden price information, forced two-sided
quoting, forced replenishment, unbounded capital, or supplier concentration
above the preregistered 75% diagnostic threshold. Also close it if strict
valuation fails, CDF remains persistently one-sided, or the supplier's risk
and PnL cannot be reconstructed. A negative result is retained as the
successor's result and does not rewrite R2.

## Promotion sequence

The sequence is:

1. validate the immutable configs and runner/scorer contracts;
2. obtain one fresh independent Sol-xhigh review of this complete 24-hour
   successor tree;
3. build clean Go 1.27 `linux/amd64/v1` binaries;
4. perform an actual full-run binary-evidence capacity measurement and retain
   its evidence until its measurement contract passes;
5. run primary treatment/control cells sequentially, beginning with
   `treatment-607` and `control-607`;
6. extract and independently review each completed pair; run the registered
   parity cells only after their base treatment evidence is accepted;
7. obtain explicit freeze authorization before any untouched holdout.

The activation probe's acceptance authorizes none of steps 3--7 by itself;
each later gate remains fail-closed and all historical evidence remains
untouched.
