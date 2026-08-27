# V2-7 P7d holdout results

Status: **complete evidence package; aggregate NOT IDENTIFIED**.

This document reports the three pre-reserved P7d holdout seeds separately.  It
does not invent a cross-seed replication rule that is absent from the P7d
preregistration.  The strongest permitted result is therefore a collection of
orientation-separated screening observations, not a three-seed holdout
verdict.

## Protocol and provenance

The immutable protocol is
[`v2-7-p7d-directional-distress-causal-preregistration.md`](v2-7-p7d-directional-distress-causal-preregistration.md).
The registered cells are C (disabled control), L (+2,000,000,000 raw ABC),
and S (-2,000,000,000 raw ABC).  The development promotion result was
`SUPPORTED (screening)` on seeds 431 and 433 and authorized the pre-reserved
holdout policy.  No financial, population, clock, latency, risk, horizon or
analyzer parameter was changed for these holdouts.

All nine holdout cells use the same recorded simulator source revision
`8b1d013a33129beeb61a578b6f59aefa59cce0c2`, simulator binary SHA-256
`0bcc40ef78f87f08301555bf203366780569c19ed42e84c024e39de34d2ebece`, and
analyzer binary SHA-256
`934993a87a817cf3fcb52eb29e8b1392d0b81e2149b52a4f5706026d57e763ad`.
Every cell is a four-hour, full-evidence run with `greeks.json` and
`latency.json` as completion sentinels.  The six 443/449 launcher statuses and
all nine extraction statuses are zero.  The 439 runs predate the launcher
status convention; their final sentinels, complete evidence, metadata and
fail-closed extraction status are present, but no launcher-status file exists.
That provenance limitation is recorded rather than silently repaired.

The pinned configuration hashes are:

| cell | seed 439 | seed 443 | seed 449 |
|---|---|---|---|
| C | `0ff72d0aae2db4c1bfb22d59742df8be3aa58af01a6268f1a6bb418fdd14b21b` | `0709d24ebce257cd0dda441cda4c0745986a51758e6cd655223b6589f7819cea` | `93cdebfde9e1949637cc61896e889dddf9efef81d42d05ad0d4dc975b3ccc4a5` |
| L | `1089fe48fbf3745d2b39ba939c2531f7ba7d3a2da15ff1a97b271f0e7d66ec5a` | `d58274248399e00cadc73d6c815170e2e0503a7dd3b3752068c80107d3839d83` | `6bb033b6b71a1c4e8d5e1302a7069b748d272af25d01ad217fbbb736a573d9d6` |
| S | `9a2b6dbdd9b1d65525276367d243f221a3ffb18db2f73dca8fe07e016a564c9c` | `00660dd63a588145b164e3affa7ca32336638e943517b8de6eb1adf93c99fb45` | `ccba09c2345c04930f3914e27d4e05a8d1a1d6bfb6ddf036f1763b60ef90fe26` |

The machine-readable package is
[`p7d-holdout-verdict.json`](artifacts/v2-7-p7d/p7d-holdout-verdict.json).
The three per-seed score files are retained beside it.  The package records
all nine cells, all sixteen registered P7d metric artifacts per cell, complete
analysis metadata, runtime/offline evidence-artifact digest equality, and raw
evidence retention.

## Evidence integrity and activation

All cells passed the checked receipt/frontier, order/fill/position, lifecycle,
settlement, conservation and participant replay integrity predicates:

| seed | cell | decisions | enabled | admitted | fills | filled qty (raw ABC) | evidence events | evidence digest |
|---:|:---:|---:|---:|---:|---:|---:|---:|---|
| 439 | C | 21,600 | 0 | 0 | 0 | 0 | 14,242,855 | `48dab1016d114637d78781df792c19fa5c825af31c32cf3651046e2b7d423c10` |
| 439 | L | 21,600 | 21,600 | 246 | 33 | 6,000,000,000 | 14,269,247 | `af7479342837a70fbdb2c1bd0eba57a77504e109d72c9256f58b08d4e37b38e1` |
| 439 | S | 21,600 | 21,600 | 26 | 39 | 6,000,000,000 | 14,381,958 | `333bfd94539128b6cdee0b1848c4f56e59f96d492b40c69a23c523ba8404d58f` |
| 443 | C | 21,600 | 0 | 0 | 0 | 0 | 14,134,038 | `bbe222d8643210d301d116fabb09f5bc14430945e39fa690e0cf431fa542832a` |
| 443 | L | 21,600 | 21,600 | 197 | 35 | 6,000,000,000 | 14,155,150 | `c228c0dbbf4c5566a60ba5645217e5df9f9e2d99943ad78ed2844466f904ebf0` |
| 443 | S | 21,600 | 21,600 | 30 | 66 | 6,000,000,000 | 14,151,831 | `84ecffdbcdc004a03bf970ed27cf05c49972e0a248d9c10dccb38974f47c931d` |
| 449 | C | 21,600 | 0 | 0 | 0 | 0 | 14,245,429 | `36bafbd8addb11df724e2fb2843dab9453e6e73854eb358352262d4f0ba1280e` |
| 449 | L | 21,600 | 21,600 | 297 | 43 | 6,000,000,000 | 14,352,478 | `dcdd0c1a8bbe0125ffc7b30f30bda147960a2f86db413ca2960baecf6f6d8ed3` |
| 449 | S | 21,600 | 21,600 | 20 | 51 | 6,000,000,000 | 14,198,926 | `7054aae81b47a9ff1647b351cbb42f37f37582f704687ab194c1042d93dfb3a6` |

The disabled C controls have only the expected roster observations and policy
disabled decisions.  Every enabled orientation reached the registered target
through ordinary admitted/fill-linked IOC orders, with zero terminal target
gap.  Thus target activation is valid in all six active orientations.  This
is an activation/integrity statement only; it is not a liquidity, profitability
or realism statement.

Receipt replay reported zero future decision use, bad frontiers and receipt
errors in every cell.  The analyzer metadata reports the same event count and
digest as the run-time `evidence-artifact-hash.json` for every cell.  Analysis
revisions differ historically between 439 (the previously reviewed scorer
pass), 443 and 449, but all use the same analyzer bytes and SHA; this is
recorded in each cell's `analysis-metadata.json`.

## Participant-specific risk replay

The primary risk endpoint is the role/account-scoped replay in
`perpexposurerisk.json`.  It reconstructs contemporaneous marks, wallet and
borrow balances, position continuity, maintenance checks and participant risk
events.  The registered integrity counters were zero in all cells, and
observed checks equal independently expected breaches.

| seed | cell | candidates | mark updates | expected breaches | observed checks | participant liquidations | primary risk status |
|---:|:---:|---:|---:|---:|---:|---:|---|
| 439 | C | 3 | 43,190 | 0 | 0 | 0 | not exercised |
| 439 | L | 3 | 43,188 | 12 | 12 | 11 | exercised |
| 439 | S | 3 | 43,188 | 0 | 0 | 0 | not exercised |
| 443 | C | 3 | 43,191 | 0 | 0 | 0 | not exercised |
| 443 | L | 3 | 43,188 | 9 | 9 | 9 | exercised |
| 443 | S | 3 | 43,188 | 26 | 26 | 1 | exercised |
| 449 | C | 3 | 43,185 | 0 | 0 | 0 | not exercised |
| 449 | L | 3 | 43,190 | 7 | 7 | 7 | exercised |
| 449 | S | 3 | 43,184 | 1 | 1 | 1 | exercised |

The per-seed classifications are:

| seed | directional activation | participant-specific risk | interpretation |
|---:|---|---|---|
| 439 | SUPPORTED (screening) | MIXED | long risk exercised; short risk not exercised |
| 443 | SUPPORTED (screening) | SUPPORTED (screening) | both signs exercised |
| 449 | SUPPORTED (screening) | SUPPORTED (screening) | both signs exercised |

Long and short are never pooled to let one sign compensate for the other at a
seed.  The seed-439 scorer was written after its metrics existed and was
reviewed as an orientation-separated factual replay with the narrower claim
recorded in
[`v2-7-p7d-holdout-scorer-439-independent-review.md`](reviews/v2-7-p7d-holdout-scorer-439-independent-review.md).
The same pinned per-seed predicate was then applied unchanged to 443 and 449.

## Deficit, insurance and bankruptcy boundaries

The current ancillary `liquidations.json` field is ecology-wide and is not a
participant-specific reconstruction.  It combines separate deficit/insurance
quantities and has no registered participant-bankruptcy evidence contract.
Accordingly, deficit and insurance are **NOT SCORED** as P7d participant
endpoints here, and bankruptcy is **NOT EXERCISED / not identified**.  A
non-zero ecology-wide liquidation or insurance value must not be promoted to a
claim about the directional desk.  A future participant-scoped accounting
protocol would be a separate experiment, not a reinterpretation of this one.

## Aggregate verdict and permitted claim

The preregistration reserves seeds 439, 443 and 449 but defines no all-seed,
majority, or other aggregate replication rule.  The machine package therefore
records:

    aggregate classification: NOT IDENTIFIED
    preregistered aggregate rule defined: false

This is a missing contract, not a failed risk result.  It would be post-hoc to
declare “two of three,” “all three,” or any other aggregate rule after seeing
the seed outcomes.

The strongest permitted statement is:

> Under the pinned P7d holdout configurations, all six enabled orientations
> validly activated and passed evidence-integrity checks.  Participant-specific
> maintenance-risk replay was mixed at seed 439 and supported at screening
> level in both signs at seeds 443 and 449.  No cross-seed aggregate inference
> is licensed.

The following claims are explicitly prohibited by this package:

- three-seed replication or generalization;
- participant-specific deficit or insurance coverage;
- bankruptcy;
- full-ecology liquidation realism;
- funding or basis anchoring;
- profitability;
- market stability or any stylized fact.

## Invalid earlier attempts and evidence retention

Two pre-review launches for C/L/S-443 and C/L/S-449 were interrupted before
the scorer-review gate.  They reached partial metadata/checkpoint state, had
status 143, and had no final completion sentinels, raw event evidence or
valid score.  They are archived under
`research/artifacts/historical/v2-7-p7d-pre-scorer-review-interrupted-2026-08-27/`
and are not part of this package.  The valid 443/449 worlds were run only
after the scorer review and with unchanged pinned source, binary, configs and
evidence semantics.  No raw P7d holdout evidence has been pruned.

## Machine-artifact checks

The package was regenerated after detecting a construction bug that duplicated
seed entries and emitted null seed values.  The corrected artifact contains
exactly three unique per-seed records, `[439, 443, 449]`, and was revalidated by
running `scripts/score-v2-7-p7d-holdout-seed.sh` for each seed into temporary
outputs; each output was byte-identical to its retained score file.

The aggregate remains `NOT IDENTIFIED` until a future protocol explicitly
defines an aggregate rule.  This result closes the registered P7d holdout
execution package without authorizing distress retuning.  The next licensed
scientific gate is the V2-6 causal option disposition, or a documented closure
of that question if no clean single-mechanism contrast is identifiable.
