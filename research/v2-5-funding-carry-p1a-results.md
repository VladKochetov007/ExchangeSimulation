# V2-5 P1a result — fee-aware funding/carry feasibility

Status: **NOT EXERCISED.** This is the registered single-cell feasibility
gate from
[`v2-5-funding-carry-p1a-feasibility-preregistration.md`](v2-5-funding-carry-p1a-feasibility-preregistration.md),
not a paired causal test of funding, basis convergence, profitability, or
realism. The complete machine-readable record is
[`p1a-verdict.json`](artifacts/v2-5-p1a/fee-aware-107/p1a-verdict.json); raw
logs and sidecars remain retained in the same directory.

## Provenance and evidence gate

| field | value |
| --- | --- |
| config / seed / horizon | `configs/v2-5-p1a/fee-aware-107.json` / 107 / 30 minutes |
| simulator and analyzer source | `e1a31559dabaa40d29f1cb79a4c574e38aa4c130` |
| terminal events / ordered execution hash | 312,700 / `37bda2da23fa7d9cce6dcf99f1b175f4a9ac5fb8778c503c297ad81b7e609315` |
| persisted-evidence artifact | 315,400 records / `65298ade0b6451e2f934a2d420162dd30a054d000998814ee725355fb74eaa7c` |
| process setting | `GOMAXPROCS=12` |

The final `greeks.json` and `latency.json` sentinels were both non-empty
before extraction. The fresh build manifest reports `modified=true` only
because the shared worktree had user-owned changes beneath
`research/artifacts/scoreboard`; the tracked diff contained no source-path
change and no untracked source-like file was present. This is recorded rather
than hidden in the provenance record.

Both independent evidence gates pass:

- V2 receipt replay: **valid** — 16,179 schedules and 16,170 deliveries;
  schedule, receipt, and empty scalar-decision sidecar digests match, with no
  bad decision frontier, future decision use, or missing due receipt.
- Funding-carry replay: **valid** — all 2,700 policy evaluations replay from
  declared local sources with zero receipt, future-use, arithmetic, sign,
  decision-field, gateway, or actor-outcome mismatch. There are no policy
  checks because no request was emitted, not because outcomes were omitted.
- Terminal delta accounting: 18,150 records checked, zero mismatches and zero
  broken chains; aggregate ABC and USD residuals are both zero. The generic
  funding-semantic audit has no funding payment instant because the
  30-minute horizon is shorter than the configured eight-hour settlement
  interval.

## Registered feasibility result

The exchange fee and policy estimate were both explicitly 5 bps per
aggressive leg. The policy estimates four legs (entry plus eventual exit), so
the recorded fee component is 20 bps. It also records one bps each for
balance-sheet, margin-risk, leg-risk, and its minimum post-cost return. The
declared 500 annual-bps borrow estimate rounds to zero over the one
eight-hour-horizon at the policy's current whole-bps resolution; that is a
present, measured zero cost—not an unavailable input or a free-loan
assumption.

| observed fresh funding range | costed net-carry range | required gross funding | submitted / accepted / fills |
| ---: | ---: | ---: | ---: |
| 1–3 bps | −24 to −20 bps | at least 24 bps | 0 / 0 / 0 |

The 3-bps maximum therefore remains 21 bps below the registered threshold.
All potential action paths explicitly deferred; none was silently skipped:

| venue | `NET_CARRY_BELOW_MINIMUM` | zero premium | local reference unavailable | terminal censored |
| --- | ---: | ---: | ---: | ---: |
| central | 885 | 1 | 10 | 2 |
| north | 5 | 880 | 11 | 2 |
| south | 890 | 3 | 3 | 2 |

The exact total has 1,780 cost-based defers, 884 zero-premium defers, 24
local-reference defers, 3 initial funding-unavailable defers, 3 subscription
events, and 6 terminal-censored evaluations. The per-venue rows establish
that the negative feasibility result is neither a missing feed nor an absent
actor: all three desks evaluated fresh delivered funding, but none reached the
declared post-cost hurdle.

## Interpretation and next gate

P1a falsifies feasibility of the *current whole-bps, fee-aware one-interval
policy in this retained P0 population*, not the broader proposition that
funding can matter. The paired P1b A/B market screen is therefore not started:
there is no inventory/order intervention to compare, so a basis result would
be **NOT IDENTIFIED** by design.

The useful design constraint is more specific than “increase funding” or
“lower fees”: the policy combines an eight-hour holding interval with integer
bps, making annual borrow round to zero while all four execution fees remain
integer whole bps. Any later V2-5 P2 must be separately preregistered as a
representation/participant-economics change—e.g. exact fractional carry
accounting tied to observed notional and a clearly motivated horizon or
participant—then pass its own source, activation, and causal gates. It may not
reinterpret this P1a cell or tune the funding rate, spread, clock, demand, or
population to make it trade.
