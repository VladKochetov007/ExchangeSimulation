# V2-5 P3e — passive-exit lifecycle causal screen

Status: **preregistered before analyzer implementation, preflight execution,
full-cell execution, or lifecycle outcome inspection.** P0 remains **SUPPORTED
(screening)** for activation/integrity only. The signed-price migration remains
closed at merge `320262e` and hardening `5afdd45`; neither result is reopened.

## Fixed cells and provenance

All cells use the committed experiment source, `GOMAXPROCS=4`, full evidence,
98 simulated hours, and paired development seeds 107 and 109. Completion is
defined only by nonempty final `greeks.json` and `latency.json` sidecars.
Process-name inference is prohibited.

| cell | config SHA-256 | raw artifact path |
| --- | --- | --- |
| A/107 | `bb11a68bec082305d6c5245d1ef47b4f1103f36ea8d001e2d7b6ab0b2dff715f` | `research/artifacts/v2-5-p3e/lifecycle-A-107/` |
| A/109 | `2a799bde450c4eb69da271843c1e770e50cbcfc6dcb3603c13efdbdfd28c1d04` | `research/artifacts/v2-5-p3e/lifecycle-A-109/` |
| B/107 | `cfcf9ede724cd9a6b3f2c5f36c1c0e6c0ddc6272d73287295808ae69ad6f21e1` | `research/artifacts/v2-5-p3e/lifecycle-B-107/` |
| B/109 | `136e63ddc19491879fb3be26dc7e27a3d3feefa220d2e6494324254af5742cc1` | `research/artifacts/v2-5-p3e/lifecycle-B-109/` |

The A arm retains the legacy defer-only exit. The B arm adds only:

```json
"passive_exit": {
  "slice_qty": 100000,
  "deadline_at_nano": 1736038805000000000
}
```

The slice equals the validated 100,000-unit venue minimum. The deadline is
3,600 seconds after the expected term end and 3,595 seconds before simulation
termination. `1736038805000000000` is also A's analysis cutoff, but A receives
no simulator policy at that timestamp.

`scripts/check-v2-5-p3e-lifecycle-configs.sh` is the preregistered structural
guard. For each seed it removes only `experiment_id` and `description`, then
proves that deleting B's `term_carry_allocator.passive_exit` makes the complete
A/B JSON structures identical. It also proves seed cells differ only by seed,
provenance metadata, and the already-declared treatment.

## Independent lifecycle evidence contract

The registered `termcarrylifecycle` metric must replay persisted evidence and
must not trust actor state labels as proof of lifecycle facts. It reconstructs
each owned term from canonical entry fills and continuously applies canonical
spot/perpetual fills to the independently derived positions. It joins passive
decisions to their local receipt frontier, gateway request, canonical venue
admission, fills, cancel request, venue acknowledgement, and actor outcome.

Every term record reports these endpoints separately:

1. owned-term activation and expected term end;
2. locally reconstructed aggressive eligibility;
3. passive eligibility, submission, and canonical admission;
4. canonical acceptance-to-terminal resting duration;
5. partial and full passive fills;
6. spot and perpetual residual positions at term end, first reduction,
   registered deadline/cutoff, and terminal observation;
7. cancellation request, identity, venue acknowledgement, and deadline state;
8. independently proven flatness and time to flat;
9. later `TERM_CLOSED` transition count and identity;
10. funding before term end, on a residual before the deadline, on a residual
    after the deadline, and after real close; and
11. fill/position/funding conservation and actor-vs-terminal-position agreement.

Absence and numeric zero have different meanings. A-side passive endpoints are
`not_applicable`. A present eligibility condition with no fill has observed
zero filled quantity. A missing owned term or missing sub-minimum aggressive
condition is `not_exercised` rather than zero.

A term is proven closed only when all of the following independently hold:

1. reconstructed spot and perpetual positions both become exactly zero;
2. exactly one later `TERM_CLOSED` transition names that same term;
3. flatness and the transition occur no later than
   `1736038805000000000`; and
4. no later outcome reopens or mutates the term.

Cancellation alone never proves closure. Resting duration starts at canonical
acceptance and ends at the first terminal event, full fill, cancellation, or
run censoring. Any term-attributed funding after independently proven close is
an integrity failure. The metric emits per-term records, cell aggregates, and
named integrity failures; no single boolean may replace those records.

The adversarial suite must reject dropped or duplicated fills, a partial fill
reported as close, forged or duplicate `TERM_CLOSED`, cancellation reported as
closure, wrong cancellation identity, residual erasure at the deadline,
missing post-deadline funding, funding attributed after real close, future
receipt use, and terminal-position disagreement.

## Mechanical extraction gate

The lifecycle extractor must produce `termcarrylifecycle` plus the existing
`termcarry`, `observationreceipts`, `derivatives`, `conservation`, `positions`,
`orderlifecycle`, `lifecycle`, `streamhash`, and `evidenceartifacthash`
metrics before any result is inspected. Every evidence, accounting, frontier,
canonical-chain, and terminal-position check must pass. The runtime and offline
exact persisted-artifact event count and digest must match byte-for-byte.

All raw evidence remains retained and uncommitted. The historical `ae13f9a`
prune manifest grants no P3e authority and must not be bypassed.

## Preflight and full execution

After the experiment source is committed, build `multivenue`, `mvanalyze`, and
`prunegate` once and record the source revision and all binary SHA-256 values.
Short A/B preflights may establish only config parsing, evidence schema,
structural delta, final-sidecar production, extractor behavior, and manifest
serialization of A without P4 versus B with the declared P4 policy. They cannot
score lifecycle behavior. Preflight provenance is committed separately.

The four full cells run in bounded parallel waves while disk use and RSS are
monitored. No outcome may be inspected until every registered metric has been
extracted for that cell.

## Paired endpoints and classification

For each seed, score:

- eligible terms reaching the registered aggressive-ineligibility condition;
- proven-closed eligible terms and `closure_fraction`;
- `all_eligible_terms_closed_by_deadline`;
- time from term end to first residual reduction and to flatness;
- passive filled quantity and terminal residual magnitude;
- residual funding exposure; and
- paired B-minus-A deltas.

The verdict is fixed as follows:

- **INVALID**: any evidence, digest, accounting, receipt-frontier,
  canonical-chain, conservation, or terminal-position failure.
- **NOT EXERCISED**: no paired owned term reaches the registered
  aggressive-ineligibility condition.
- **FALSIFIED AT ACTIVATION**: B reaches that condition but cannot emit and
  admit a valid ordinary passive child.
- **SUPPORTED (screening)**: both exercised seed pairs have positive B-minus-A
  closure effects and B proves actual closure.
- **MIXED**: seed directions disagree, only one pair exercises, or B reduces
  exposure without consistently closing.
- **FALSIFIED**: activation passes but both exercised pairs show no closure
  improvement. Any residual reduction remains a separately reported secondary
  result.

No basis, funding-anchor, profitability, stability, liquidity-realism, broader
market, or population-robustness claim is permitted.

## Conditional V2-5 continuation

If P3e proves closure with valid evidence, it may be promoted only as the
finite-term execution contract before a separate funding/carry causal screen
is preregistered. If it reduces but does not close, that partial-liquidity
result is retained without changing demand, depth, spread, slice, clocks, or
venue rules, and this participant cannot support a market-level funding-anchor
claim. If activation or integrity fails, the inference stops; only the
demonstrated mechanism or evidence defect may be repaired under a new
preregistration.

A later funding/carry screen must identify, in order: delivered premium and
funding observation; independently recomputed expected funding; exact net carry
after fees, borrow, balance-sheet, margin, and leg risk; changed target
inventory; actual submitted and filled spot/perpetual orders; and resulting
basis dynamics. A missing link is **NOT IDENTIFIED**. Funding never writes a
target market price. Dated carry requires a later, separate
time-to-expiry/settlement protocol.
