# V2-3 P2 results — explicit CDF/USD inventory rebalance

Status: **SUPPORTED (screening), mechanism integrity only.** This is a
five-minute, two-seed identification result, not a stability, calibration, or
aggregate-risk claim. The machine-readable record is
`research/artifacts/v2-3-p2/p2-summary.json`; all raw evidence remains
retained in its final A/B seed directories.

## Valid campaign

The final A/B × seed-101/103 cells ran after the zero-enum audit and the
P2 spot-outcome analyzer correction, from source revision `675e117` with full
logs. `greeks.json` and `latency.json` were the sole completion sentinels.
The independent receipt replay and the independent P2 policy/order/fill replay
are valid in all four cells. Every persisted artifact has an unordered evidence
multiset digest; no digest is misrepresented as a global ordered stream.

| arm | seed | decisions | submissions | accepted | censored tail | fills | filled quantity | P2 audit |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| A | 101 | 180 | 0 | 0 | 0 | 0 | 0 | valid |
| A | 103 | 180 | 0 | 0 | 0 | 0 | 0 | valid |
| B | 101 | 180 | 46 | 44 | 2 | 88 | 5,665,000,000 | valid |
| B | 103 | 180 | 50 | 48 | 2 | 96 | 9,150,000,000 | valid |

Both A worlds recorded exactly 180 `POLICY_DISABLED` decisions and no
submission. B had only the preregistered `SUBMIT_IOC`, `COOLDOWN`, and
`IN_BAND` actions. The two B submissions per seed at the finite-horizon tail
are explicitly marked simulation-horizon censored; they have no invented
venue outcome and are not counted as rejected or accepted.

## Primary causal score

The primary P2 claim is supported at screening level:

> An eligible, out-of-band CDF/USD maker can submit an auditable, capped,
> local-information IOC risk-transfer order that pays ordinary taker fees and
> reduces its own inventory only through an external counterparty.

The intervention activated in both treatment seeds while submitting zero
orders in both controls. Independent replay found zero missing/ambiguous/future
receipts, decision-policy mismatches, request-field mismatches, duplicate or
missing outcomes, non-IOC terminals, fill-evidence mismatches, self fills,
free/non-taker fills, fee mismatches, or non-reducing local fill transitions.

## Descriptive non-primary measurements

Pooled CDF-maker mean absolute inventory changed by +307,049,322 raw units in
seed 101 and +195,133,386 in seed 103 (B minus A). Terminal pooled inventory
sum moved by -6,816,507,368 and -1,753,627,783 respectively. These numbers do
not contradict the per-fill local reduction: P2 transfers risk between agents,
while all other flow remains active. They must not be reinterpreted as an
aggregate-risk or stability score.

CDF/USD five-minute quote/activity viability was already limited by
concentrated flow in both arms. It is reported in the retained viability
sidecars but was not a P2 gate and cannot rescue or defeat the primary
mechanism claim.

## Limits and lineage

This result does not show that P2 fixes the CDF/USD runaway, stabilizes prices,
creates price-elastic demand, improves long-horizon viability, or survives a
larger seed set. It validates only the costly, rate-limited, locally informed
external risk-transfer mechanism needed for later ecology work.

Attempt 0 remains invalid because valid BUY was omitted from decision evidence;
attempt 1 remains an unscored pre-audit diagnostic run. Their raw evidence is
preserved under `research/artifacts/historical/` and is never pooled with this
final campaign.
