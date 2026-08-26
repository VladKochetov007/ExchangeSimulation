# V2-7 P7c — multi-day fixed-liability risk horizon

Status: **preregistered design; no P7c outcome or preflight world has been
inspected**.

P7b established that the unit-corrected fixed-liability actor can enter and
hold a declared perpetual hedge with a near-initial-margin capital level, but
the participant margin/forced-close path was not exercised during one day.
P7c tests a different economic risk horizon rather than lowering a trigger or
altering the market path:

> With the same persistent physical liability, ordinary execution, and the
> already registered near-initial-margin capital, does a two-day risk window
> make a participant margin/forced-close event reachable?

This is a mechanism-identification experiment, not a crisis-realism,
funding, basis, profitability, or price-path target. No price shock,
threshold override, synthetic fill, atomic leg, funding read, or direct price
anchor is added.

## Fixed participant and information contract

The participant is the audited `v2_7_fixed_liability_v1` local-feed actor. It
has a fixed off-exchange physical exposure of `-1,000,000,000` raw ABC (10
ABC), so its declared perpetual target is `+1,000,000,000` raw ABC on each of
the three venues. It submits ordinary IOC orders at the last locally delivered
executable touch, capped at `250,000,000` raw ABC per request. Once the target
is filled it holds it and does not reopen after an exchange-side close. The
P7a/P7b receipt/frontier, position, fill, lifecycle, settlement, expiry,
conservation and risk evidence requirements remain in force.

## Development design

| cell | participant | initial perp margin | horizon | purpose |
|---|---|---:|---:|---|
| C | installed, disabled | 5,500,000,000 raw ($55,000) | 48 h | roster/evidence control |
| T | enabled fixed liability | 5,500,000,000 raw ($55,000) | 48 h | two-day risk-horizon treatment |

The capital is inherited unchanged from P7b's registered near-initial-margin
level. At the inherited approximately $50,000 opening reference, 10 ABC has
about $500,000 notional, the 10% initial requirement is about $50,000 and the
configured five-basis-point entry fee is about $25. The $55,000 balance is
therefore a fixed finite buffer chosen before P7b outcomes, not a value fitted
to a P7c path. The new intervention is the **48-hour observation/risk
horizon**, a natural two-day liability-management window; no margin or price
threshold is changed.

All other fields inherit `research/configs/v005-stress-perp.json` and the P7b
participant settings exactly: three venues, cross-asset spot graph disabled,
projected maker rosters, ordinary fees/borrowing/funding, local 40-ms public
feed delay, 2-second actor decisions, one-second runner step, full evidence
and strict risk checks. The registered P3e exit parameters, population,
liquidity, spreads, latency distributions and clock phases are unchanged.

The registered full-run horizon is **48 simulated hours**. This covers six
north funding instants, 48 central funding instants and 24 south funding
instants under the inherited 28,800/3,600/7,200-second venue schedules. The
run is censored only at the declared 48-hour endpoint; risk checks and
funding/lifecycle evidence before that endpoint are retained. A mechanics-only
preflight may use at most 15 simulated minutes and cannot score distress.

Development seeds are **367 and 371**, reserved before any P7c run. Untouched
holdouts are **373, 379 and 383**, also reserved before any P7c outcome. No
holdout may be run unless both treatment seeds pass activation/evidence gates
and at least one treatment seed contains an independently reconstructed
participant risk event.

## Endpoints and classification

The analyzer reports independently:

1. participant decisions, local receipt frontier and ordinary IOC outcomes;
2. target entry, admitted/fill-linked quantity and terminal hedge gap;
3. contemporaneous participant margin checks and expected breaches;
4. participant-specific margin-call/forced-close events and position reduction;
5. residual position/collateral after any close;
6. deficit, insurance transfer and bankruptcy, each with exact balance and
   conservation reconstruction;
7. generic liquidations for other accounts, explicitly not substituted for
   participant distress;
8. funding events before and during any participant residual exposure.

The primary paired development statistic is the exact indicator that a
participant-specific expected margin breach and corresponding independently
linked venue risk event occur by 48 hours, conditional on valid activation.
Supporting continuous endpoints are time-to-first-risk breach, forced-close
quantity and terminal residual exposure. Deficit, insurance and bankruptcy are
separate zero/nonzero endpoints and are never required for a forced-close
activation.

The control must remain valid and disabled. The treatment must reach the full
three-venue target with zero terminal target gap and pass all evidence and
accounting checks. If activation or evidence fails, classify
`FALSIFIED AT ACTIVATION` or `FALSIFIED AT EXECUTION`. If activation is valid
but both treatment cells have zero participant risk events, classify
`NOT EXERCISED`; do not tune or consume holdouts. A single valid risk event
with a clean independent reconstruction supports only a screening-level
reachable-distress claim and licenses the registered holdout policy. A
seed-discordant risk predicate is `MIXED`. Generic liquidations alone never
satisfy treatment risk activation.

No P7c result licenses claims about funding anchoring, basis convergence,
profitability, market stability, liquidation realism of the full ecology, or
holdout realism.

## Adversarial, provenance and retention contract

Before scoring, the fail-closed extractor must verify the immutable config
delta, full evidence, receipt/frontier causality, ordinary order admission,
position/fill links, lifecycle, settlement, expiry, conservation and all risk
arithmetic. Runtime and offline persisted-evidence artifact digests must agree.
Metadata records the simulator revision, binary, config hash, analysis
revision, completion sentinels, exact horizon and raw-log retention.

The mutation suite includes reversed liability sign, dropped/duplicated
decisions or fills, future/delayed/duplicated/reordered receipts, wrong local
touch, request-cap violations, dropped/duplicated liquidation evidence, stale
marks/collateral and synthetic balance resets. A mutation that survives is a
validator weakness. A zero trigger count is `NOT TESTED/NOT EXERCISED`, never
a pass.

All raw worlds remain retained; this protocol has no prune authority. The
P7b cells and their `NOT EXERCISED` outcome remain historical evidence and are
not silently rewritten or used as P7c controls.
