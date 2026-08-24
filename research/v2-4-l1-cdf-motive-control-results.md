# V2-4 L1 results — matched CDF/USD motive-control screen

Status: **SUPPORTED (screening), local motive only.** This is the registered
30-minute A/B × seed-{101,103} experiment from
[`v2-4-l1-cdf-motive-control-preregistration.md`](v2-4-l1-cdf-motive-control-preregistration.md).
It identifies the effect of the declared side-selection motive within one
otherwise matched CDF/USD-only slot. It is not evidence of price stability,
empirical demand elasticity, legacy `noise_flow` replacement, ecological
viability, or phase robustness.

## Provenance and evidence gate

The immutable run cells used source revision
`a99245f54d445ff2f8a4464f65d6785283cc6d20`, a rebuilt `multivenue` binary
with SHA-256 `65234f4f204ca14d28999944bfbdb3e85a5802735b5854b431a85935bb1fa33d`,
`GOMAXPROCS=4`, full persisted evidence, and a 30-minute simulated horizon.
Their four configs are in `research/configs/v2-4-l1/`; after excluding the
per-arm `experiment_id`, the sole economic A/B difference is
`cdf_liability_hedger.policy_mode`:
`random_side_control` in A and `delivery_liability` in B.

Raw evidence remains retained under
`research/artifacts/v2-4-l1/{A,B}/seed-{101,103}`. Completion was established
only by each cell's final `greeks.json` and `latency.json`. The extractor then
retained a V2 receipt/frontier audit, a persisted-evidence artifact digest, an
independent policy/state/fill replay, full and post-warmup book viability, the
preregistered non-collapse floor, and analysis metadata. None of this
authorizes pruning.

The final offline analysis revision is `1a2641ae180b46f71f9ca667d4238ee26b8b9088`
(`mvanalyze` SHA-256
`c12a5dbd8e37da90a1753fb8dfb9eeb1dbfc69f49f9ec8dc1e9ed45052f25888`).
Two evidence-auditor defects were found while replaying the retained cells and
were fixed before final extraction, without changing the simulator, raw logs,
scheduler, RNG, actor, or economy:

- `4364fed` corrected a generic replay check that treated a retained intended
  quantity on `LOCAL_EXECUTABLE_PRICE_UNAVAILABLE` as an actual request even
  when the decision's `request_id` was zero. The decision-level replay already
  validated that explicit no-touch defer shape.
- `1a2641a` corrected a replay assumption that every fill has a positive USD
  fee. The exchange's documented `normalizedExecutionFee` represents an exact
  integer-truncated zero fee as `amount=0, asset=""`; the replay now requires
  that exact representation and still rejects zero where a positive fee is due.

Both corrections have focused normal/race fixtures and were checked against
the raw A/101 and B/101 records. They are analysis-only provenance fixes, not
post-hoc economic changes.

| arm / seed | evidence events / digest prefix | receipt replay | policy replay | slot updates | accepted | fills | CDF non-collapse floor |
| --- | --- | --- | --- | ---: | ---: | ---: | --- |
| A / 101 | 2,419,537 / `85f15445` | valid | valid | 180 × 3 | 2,666 | 4,287 | pass |
| A / 103 | 2,328,033 / `f670b748` | valid | valid | 180 × 3 | 2,644 | 3,438 | pass |
| B / 101 | 2,398,538 / `b33fb884` | valid | valid | 180 × 3 | 1,982 | 1,944 | pass |
| B / 103 | 2,396,423 / `a1098866` | valid | valid | 180 × 3 | 2,014 | 2,068 | pass |

Each CDF/USD venue cleared the prespecified floor: at least 150 trades, at
least two taker roles, at least one maker role, and at least 95% two-sided
snapshots after the 10-second warmup. The lowest measured two-sided share was
96.5% (A/101 south). This is deliberately only a non-collapse floor.

## Preregistered motive score

`absolute_gap_sum` is retained as an exact integer over 2,700 decision
samples (900 per venue); displayed means are derived only as
`absolute_gap_sum / 2700`. No float state is used by the replay.

| seed | A exact sum / mean abs gap | B exact sum / mean abs gap | paired B − A mean | A reducing / nonreducing fills | B reducing / nonreducing fills |
| --- | ---: | ---: | ---: | ---: | ---: |
| 101 | 24,924,305,180,184 / 9,231,224,140.809 | 898,726,083,300 / 332,861,512.333 | −8,898,362,628.476 | 1,520 / 2,767 | 1,944 / 0 |
| 103 | 13,012,936,825,726 / 4,819,606,231.750 | 1,310,482,630,795 / 485,363,937.331 | −4,334,242,294.419 | 1,475 / 1,963 | 2,068 / 0 |

The registered primary criterion is met in both seeds:

1. all exercised B fills reduce their independently reconstructed absolute
   signed delivery gap;
2. A has at least one exercised nonreducing fill (2,767 and 1,963); and
3. B's exact time-average absolute gap is lower than A's in both paired seeds.

This supports only the narrow causal claim:

> Holding the CDF/USD-only slot's local feed, cadence, obligation path,
> quantity cap, IOC execution, fee, deposits, and terminal policy fixed,
> selecting side from the signed delivery-liability gap reduces that slot's
> own exposure more reliably than an independent random-side control.

The control's large nonreducing share is expected by its declared design and
is evidence that the scoring did not silently impose delivery direction on A.
Two paired seeds are screening evidence, not a robustness result.

## Boundaries and next gate

The screen does not compare prices, spreads, market impact, wealth, or
population substitution. Its viable-looking CDF/USD activity floor also does
not establish an unconcentrated or realistic ecology. In particular, the six
legacy broad `noise_flow` participants remain present by design, so L1 says
nothing about replacing them.

Before any L2 allocation/replacement claim, the next high-value experiment is
the separately preregistered L1-P cadence phase/offset test. It must vary phase
without retuning flow, spreads, inventory, latency, or population, and retain
the same independent policy/receipt replay contract.
