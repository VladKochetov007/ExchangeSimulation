# V2-3 P3 attempt 0 — invalid retained evidence

Status: **INVALID; historical only; not scored.**

## Completed worlds retained

On 2026-08-24, the initial short full-evidence A/B screen completed all four
five-minute worlds at implementation revision `b4e4f190e88bc4421bce38dcd142a5c6fa132447`:

| arm | seed | retained evidence directory |
| --- | ---: | --- |
| A | 101 | `research/artifacts/v2-3-p3/A/seed-101` |
| A | 103 | `research/artifacts/v2-3-p3/A/seed-103` |
| B | 101 | `research/artifacts/v2-3-p3/B/seed-101` |
| B | 103 | `research/artifacts/v2-3-p3/B/seed-103` |

Each directory has the registered final `greeks.json` and `latency.json`
sentinels, copied immutable config and run metadata, and retained raw venue
JSONL. Completion does not make it valid scientific evidence.

## Why it is invalid

The first lifecycle schema recorded a fill's `(order ID, exchange timestamp,
side, quantity)` but omitted its exchange trade ID. A pro-rata match can create
two distinct fills with exactly those four values at one timestamp. In
`A/seed-101`, for example, central order 5935 has two distinct equal
104,437,701-unit partial fills at one timestamp, and south order 2940 has two
distinct equal 150,323,507-unit partial fills. The independent replay correctly
refused to guess which fill the delayed actor had received.

This is an observation-schema defect, not evidence loss and not a simulator
economic or determinism defect. The actor's execution trajectory was preserved;
the persisted lifecycle evidence could not uniquely reconstruct it. The first
extractor also addressed the evidence-artifact wrapper at `.result.events`, not
the obsolete top-level path. That extractor defect was found before any score.

The A/101 independent replay recorded four ambiguous-fill failures. Extraction
stopped at that first invalid cell; no A/B result, viability comparison, or
policy-activation claim was calculated from attempt 0. The other three complete
raw worlds remain retained but are likewise not eligible for the corrected
contract because their lifecycle rows do not carry `trade_id`.

## Corrective action and boundary

Commit `4cdbe31` adds `trade_id` to the evidence-only lifecycle row and joins
each fill exactly by trade identity. It also proves that `trade_id = 0` is a
valid first venue trade, not an unavailable sentinel. Collision, wrong-trade,
duplicate-fill, zero-valued-first-trade, fresh-process, and GOMAXPROCS
neutrality tests pass after the correction.

The correction changes only append-only P3 evidence and its analyzer. It adds
no scheduler event, RNG draw, actor-visible value, quote rule, timing, price,
size, matching, fee, or population change. The original economic A/B threshold
remains 0 versus 5,000 bps. A separate R1 evidence amendment is preregistered
before any replacement world runs.

Raw evidence is retained. Nothing in this attempt is prunable or scoreable.
