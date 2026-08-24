# V2-3 P3 R1 — confirmed perpetual passive-quote replenishment result

Status: **NOT EXERCISED.** This is a valid replacement screen under the R1
evidence amendment, not a favorable or unfavorable causal result about
perpetual liquidity, carry completion, funding, price stability, or realism.

The original economic preregistration remains unchanged in
[`v2-3-perp-quote-replenishment-p3-preregistration.md`](v2-3-perp-quote-replenishment-p3-preregistration.md).
R1 changes only fill evidence identity, as declared before configuration
rendering and simulation in
[`v2-3-perp-quote-replenishment-p3-r1-evidence-amendment.md`](v2-3-perp-quote-replenishment-p3-r1-evidence-amendment.md).
Attempt 0 remains invalid historical evidence; it is neither pooled with nor
used to score R1.

## Immutable screen and provenance

The short full-evidence screen compares the fixed policy arms on seeds 101 and
103:

| arm | `perp_maker_replenish_below_bps` | policy |
| --- | ---: | --- |
| A | 0 | disabled control |
| B | 5,000 | strict confirmed residual below one half of target |

All four cells used source revision
`e79eb37541b1c9af14e06e9f12ef042ec80a091e`, multivenue binary SHA-256
`916fbcbbee24fe07c98dfd66979e227486ed958139773350e5b75a6d509a0333`,
`GOMAXPROCS=3`, a five-minute horizon, full logging, and only final
`greeks.json` plus `latency.json` sidecars as completion sentinels. Raw
evidence is retained at `research/artifacts/v2-3-p3-r1/{A,B}/seed-{101,103}`
and is not prunable.

The compact, reproducible scorecard is
[`p3-r1-summary.json`](artifacts/v2-3-p3-r1/p3-r1-summary.json). It records
the per-cell config and evidence digests, replay results, ordinary order
lifecycle, information-boundary result, and ABC-PERP viability gate.

## Evidence and ordinary-viability gates

Every cell passes the R1 validity contract:

- each persisted-evidence artifact has a nonempty unordered-multiset digest;
- every P3 decision/lifecycle replay is valid, with zero structural,
  threshold, outcome, duplicate, and lifecycle mismatches;
- every V2-0 receipt/frontier replay is valid; and
- the independent ordinary order-lifecycle reconstruction has accepted orders
  and zero lifecycle error counters.

The ordinary ABC-PERP viability prerequisite also holds without asserting that
the market is globally realistic:

| seed | snapshots across venues | two-sided share | trades | viable 60s windows |
| ---: | ---: | ---: | ---: | ---: |
| 101 | 900 | 98.22% | 761 | 14 / 18 |
| 103 | 900 | 97.78% | 791 | 15 / 18 |

Both arms are identical for each seed on these observables. That is expected
when no treatment action occurs; it is not an estimate of a treatment effect.

## Preregistered activation score

| arm / seed | P3 decisions | policy-enabled decisions | `refresh_due` | observed action |
| --- | ---: | ---: | ---: | --- |
| A / 101 | 49 | 0 | 0 | `POLICY_DISABLED` ×49 |
| A / 103 | 22 | 0 | 0 | `POLICY_DISABLED` ×22 |
| B / 101 | 49 | 49 | 0 | `ABOVE_THRESHOLD` ×49 |
| B / 103 | 22 | 22 | 0 | `ABOVE_THRESHOLD` ×22 |

The preregistration requires an independently confirmed partial own
perpetual-quote fill whose remaining quantity is *strictly* below half target
at the next otherwise unchanged decision. Neither B cell contains such a
decision (`refresh_due = 0`). Therefore neither treatment seed activates the
mechanism, and the correct result is **NOT EXERCISED**.

This does **not** falsify the local replenishment policy. It says only that
this fixed five-minute, two-seed screen did not expose the state needed to
test it. The threshold, population, timing, price, and size policy are not
retuned after this result. No additional P3 screen is licensed by this
preregistration.

## Evidence correction carried by R1

The retained attempt-0 rows could not uniquely identify two same-time,
same-quantity fills. R1 persists the exact venue `trade_id` for each fill.
`trade_id: 0` is accepted as the first valid venue trade, while non-fill rows
also serialize zero and are distinguished by their transition. The independent
replay joins exact venue, participant, order, trade, symbol, side, quantity,
exchange-time, and actor-observation-time identities. R1 has zero such join
failures, so its NOT EXERCISED classification is evidence-valid rather than a
logging limitation.

## Consequence

P3 closes as a negative activation result. It provides no basis to claim that
the policy repairs the historical finite-term exit-capacity failure, identifies
funding/carry economics, or improves V2 ecology. The next research step must
be selected independently from the V2 design ledger; retained P3 evidence
must not be pruned or used to tune a new threshold.
