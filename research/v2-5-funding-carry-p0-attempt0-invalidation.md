# V2-5 P0 activation attempt 0 — invalidated evidence campaign

Status: **INVALIDATED BEFORE INTERPRETATION.** The preserved world at
`research/artifacts/v2-5-p0/activation-101/` is historical diagnostic
evidence, never an activation result and never a funding/carry result.

## What ran

| field | value |
| --- | --- |
| immutable input | `configs/v2-5-p0/activation-101.json` |
| seed / horizon | 101 / 5 simulated minutes |
| terminal event count | 56,415 |
| execution hash | `feef18193965147a02437b935af3a2fdc9ceabe889f86dcef0b81ac00a6fa5ec` |
| raw decision evaluations | 450 |
| submitted legs / accepted orders / fills | 27 / 27 / 32 |
| V2 receipt base audit | valid |

The activity counts are diagnostic only. They must not be quoted as successful
P0 activation because the decision-to-observation linkage below was invalid.

## Defect and scope

Version-one `funding_carry_decision` evidence wrote each cached spot book,
perpetual book, and funding update as though it were the *current* frontier of
the actor's shared delayed gateway. At a busy same-time delivery boundary,
`MarketDataFrontier()` reports the final delivered prefix, not the individual
message that populated a cache entry. Thus one decision could label three
different source identities with a single unrelated terminal receipt ordinal.

The raw public observations were retained once and the simulator state is not
missing data. This is an **evidence-schema/instrumentation defect**, not an
economic or execution defect. It cannot be repaired by guessing source
identities from later cache state or order outcomes.

The original P0 analyzer reported 891 source-link mismatches (and dependent
gateway checks) under the v1 contract. After the explicit v2 schema, the
attempt is rejected at the policy-version boundary, so an obsolete field layout
cannot accidentally appear valid.

## Correction and rerun criterion

Commits `bf4e879` and `c574676` replace the false per-cache frontier claims
with:

1. one exact decision-level gateway frontier; and
2. per-cache source identities `(type, sequence, publication time)` replayed
   independently within that frontier prefix.

For every present cached source, the corrected analyzer requires one matching
receipt on the decision link, ordinal no later than the declared frontier, and
publication and delivery no later than the decision. It also supports an
explicit empty registered frontier before the first receipt, which cannot
justify a cached input. Commit `c84c367` establishes fresh-process evidence
ON/OFF neutrality and GOMAXPROCS 1/4 determinism for the P0 actor.

A replacement uses the separately committed `activation-r1-101.json` with
only provenance labels changed. It must run into a new directory, retain all
raw evidence, produce final `greeks.json` and `latency.json`, and pass:

- V2 receipt audit;
- `mvanalyze -metric fundingcarry` with `valid: true`;
- persisted-evidence artifact digest; and
- exact run/binary/config/hash provenance.

Only then can P0 be called an activation gate. It still cannot support a claim
about basis anchoring, profitability, or market realism.
