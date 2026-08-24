# V2-4 L1-P3 — post-hoc seed-heterogeneity diagnostic

Status: **exploratory retained-evidence diagnostic; not preregistered and not
a revision of the L1-P3 causal score.**

## Question and method

The registered L1-P3 replication is MIXED: seeds 107/109 have the original
relative-phase direction, while 113 reverses it. This diagnostic asks only
whether that reversal is plausibly an evidence/activation failure or instead
appears in the actual local execution path.

For every `liability_hedger_decision` row in the retained full JSONL evidence,
I independently summed `abs(hedge_gap)` by venue, excluding only the declared
`NOT_SUBSCRIBED` and `SIMULATION_HORIZON_CENSORED` rows. I also counted
`SUBMIT_IOC`, `IN_BAND`, and `LOCAL_EXECUTABLE_PRICE_UNAVAILABLE`, then summed
`liability_hedger_fill.qty` by venue. These are descriptive decompositions of
the existing endpoint, not new outcome substitutions.

## Findings

- This is not missing-evidence or inactive-mechanism noise. Every cell already
  passed receipt, phase, policy, state-update, and reducing-fill replay gates.
  The unavailable-touch count is only 0–9 out of about 2,700 decisions per
  cell, so unavailable local books cannot explain the sign reversal.
- The high-gap phase combinations are associated with low local fill
  throughput despite continued submission—not with hedgers ceasing to act.
  The timing relationship therefore reaches the endpoint through actual
  venue-local fills/partial executions, but this decomposition cannot identify
  which counterparty or policy state caused it.

| seed / arm / venue | local gap sum | share of arm gap | IOC submissions | filled quantity |
| --- | ---: | ---: | ---: | ---: |
| 107 A north | 332,700,000,000 | 67.395% | 707 | 13,060,000,000 |
| 107 B north | 77,377,500,000 | — | 555 | 35,295,000,000 |
| 107 C north | 72,765,000,000 | — | 519 | 35,500,000,000 |
| 107 D north | 67,965,000,000 | — | 467 | 34,820,000,000 |
| 109 D north | 233,510,000,000 | 51.164% | 562 | 26,460,000,000 |
| 109 B north | 76,320,000,000 | — | 547 | 35,200,000,000 |
| 109 C north | 83,365,000,000 | — | 566 | 34,230,000,000 |
| 113 B central + south | 177,678,804,824 | 71.670% | 1,382 | 67,134,574,100 |
| 113 C central + south | 171,437,824,633 | 70.202% | 1,326 | 68,185,626,470 |
| 113 A central + south | 131,155,597,342 | 67.467% | 879 | 70,300,000,000 |
| 113 D central + south | 132,156,233,311 | 66.088% | 889 | 70,300,000,000 |

In seed 113 specifically, the de-aligned B/C cells submit substantially more
central/south IOC requests yet fill less quantity there than the aligned A/D
cells. That is consistent with a phase-dependent local liquidity/queue path,
not with the evidence rows being absent. It is **not** proof that broad noise
flow is the sole counterparty, nor that a particular least-common-multiple
explanation is correct: all actor interactions and the resulting books differ
after the first phase-dependent events.

## Consequence

The diagnostic reinforces, rather than weakens, the registered **MIXED**
verdict. The current population has seed- and venue-specific execution regimes
under this clock intervention. No phase, price, spread, roster, latency, or
liability parameter may be tuned from these rows. If the mechanism is revisited
later, the useful experiment is a separately preregistered, controlled
one-venue counterparty/liquidity perturbation with the same phase manipulation
and a direct fill-through endpoint; it must not be presented as a continuation
of L1-P3.
