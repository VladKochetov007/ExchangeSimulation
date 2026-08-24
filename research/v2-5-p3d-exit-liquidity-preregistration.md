# V2-5 P3d — exit-liquidity policy preregistration

Status: **preregistered before a P3d preflight, P3d full world, or P3d
raw-evidence inspection.** P3c is retained historical evidence that the v2
finite-term policy cannot submit exits below its actor-local 100,000-unit
materiality floor. P3d tests exactly one replacement participant-policy
mechanism: whether a separately declared, local-touch-bounded risk-reduction
floor allows those already-owned terms to unwind.

The causal design and its limitations are fixed in
[`v2-5-p3d-exit-liquidity-design.md`](v2-5-p3d-exit-liquidity-design.md).
P3d does not revise P3a–P3c or make a funding, carry-profit, basis,
price-quality, or realism claim.

## Fixed paired cells

| arm | config | seed | horizon | output root |
| --- | --- | ---: | --- | --- |
| A — legacy exit floor | `configs/v2-5-p3d/term-exit-legacy-107.json` | 107 | 98 simulated hours | `artifacts/v2-5-p3d/term-exit-legacy-107/` |
| B — exchange-unit exit floor | `configs/v2-5-p3d/term-exit-unit-107.json` | 107 | 98 simulated hours | `artifacts/v2-5-p3d/term-exit-unit-107/` |

Both cells use P3 v3 evidence and all P3c market, financial, timing, capital,
latency, finite-mandate, and population values. Each explicitly persists
`unwind_min_order_size`, so both use the same schema and no absent-versus-zero
wire ambiguity remains. After removing provenance strings (`experiment_id`,
`causal_arm`, `description`) the sole economic-policy delta is:

```text
term_carry_allocator.unwind_min_order_size: 100000 (A) -> 0 (B)
```

Zero means the exchange's positive-unit quantity admission floor; it is not a
price, price fallback, absent observation, missing liquidity, or unlimited
quantity. Entry children still use the shared 100,000-unit
`min_order_size`; every unwind child remains an IOC capped by the current
locally delivered touch, 10m lot cap, and residual exposure.

P3d changes an economic actor policy for these new cells. It changes neither
the historical ae13f9a freeze nor any P3a–P3c artifact. The v3 evidence field
is observer-only; it consumes no random draws and adds no event scheduling.

## Preflight and raw-evidence contract

Before either full cell, run one five-simulated-minute full-evidence preflight
for **each exact configuration**, at the corresponding `preflight-*` output
root. A preflight may establish only: configuration decoding, v3 policy
serialization (including explicit zero for B), valid receipt/frontier replay,
and required sidecar generation. It cannot test a 96-hour exit and cannot
justify modifying either configuration.

Run full cells with `GOMAXPROCS=4`. Completion means nonempty final
`greeks.json` **and** `latency.json` only. A host process, event checkpoint,
partial JSONL, or partial sidecar is not a sentinel. Raw JSONL, manifest,
checkpoints, receipt/decision sidecars, and all extracts below remain retained;
P3d is outside the frozen-prune manifest and is explicitly **not eligible for
manual pruning**.

Before scoring either arm, independently extract and retain:

```text
termcarry, observationreceipts, derivatives, conservation, positions,
orderlifecycle, lifecycle, streamhash, evidenceartifacthash
```

The v3 term replay must have zero source/frontier/gateway/venue/actor,
arithmetic, decision-policy, lifecycle, first-exposure, position-continuity,
and terminal-account mismatch counters. Generic receipt, balance, position,
order-lifecycle, derivative funding, and lifecycle counters must pass. The
latency output must show 40-ms market data and 20-ms request/response delivery.
Persist both the ordered execution hash and exact JSON-record artifact digest.

## Activation, prediction, and kill criteria

The primary comparison is **B minus A**, not B versus historical P3c:

1. A must reproduce the control mechanism: after an active term reaches its
   recorded end with an observed positive touch below 100,000 units, it emits
   `EXECUTABLE_SIZE_UNAVAILABLE` and submits no undersized unwind child.
2. B must, from a present locally delivered executable touch, emit at least one
   receipt-bounded `SUBMIT_UNWIND_PERP_IOC`; its request side, price, and size
   must match the independent v3 replay.
3. Completion is separate: every active term must reach exactly one later
   `TERM_CLOSED`; `closed_terms == active_terms`, `open_terms == 0`, and P3
   terminal spot/perpetual ownership is flat. No funding settlement may occur
   outside a still-active declared term.

P3d is **SUPPORTED (development lifecycle screen)** only if A and B pass their
mechanical/evidence gates, A remains size-deferred when the registered
activation condition occurs, B submits the predicted small child, and B closes
all active terms without accounting or information-boundary failure.

It is **FALSIFIED** if B cannot submit a legal small child from a present
touch, if the independent replay cannot predict a submitted child, or if an
active B term remains open in the registered post-term window. It is **NOT
EXERCISED** if either cell forms no active term. If activity differs across
arms before expiry for any reason other than downstream effects of the declared
exit policy, record that as a validity problem rather than a causal result.

The existing P3 mutations—missing/duplicate close, dropped unwind fill,
funding outside active term, delayed/reordered receipt, forged first exposure,
and terminal balance mismatch—remain required. P3d adds the committed v3
mutations: forged effective exit floor, entry using the exit exception, and an
oversized local-touch child. A pass still does not establish funding anchoring,
positive realized carry after exit, or realistic execution quality.

## Outcome addendum — invalid attempt, 2026-08-24

The preceding preregistration is immutable historical text. Its premise that
`unwind_min_order_size=0` meant the venue's positive-unit admission floor was
false: both ABC instruments have an actual 100,000-unit minimum. The B arm
submitted 16,286/16,348-unit children and was rejected as `INVALID_QTY`; it is
therefore **INVALID / NOT SCORED**, rather than a causal falsification of the
actor-only exit-floor hypothesis. Full provenance and the independent replay
are recorded in [`v2-5-p3d-exit-liquidity-results.md`](v2-5-p3d-exit-liquidity-results.md).
