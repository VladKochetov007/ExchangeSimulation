# V2-3 P1 — inventory-asymmetric quoted-size result

Status: **completed screening experiment; primary mechanism activation
supported.** This is not a price-stability, calibration, or robustness claim.

P1 holds the completed P0-R1 C passive-refresh contract fixed:
exchange-enforced post-only admission and actor cancel-before-replace. It
compares only the scoped spot-Stoikov displayed-size coefficient:

| arm | seeds | size skew |
| --- | --- | ---: |
| P1-A | 101, 103 | 0 bps |
| P1-B | 101, 103 | 5,000 bps |

The exact inputs are
[`v2-3-p1`](configs/v2-3-p1/), rendered from P0-R1 C. Their structural
renderer allows only identity labels, the retained decision-evidence flag, and
this coefficient to differ. All worlds used simulator revision
`5c24635e70d4646c8e0f45704845f728efc2d775`, binary SHA-256
`7163428d9c5ad3564fbf5aa7bcfd24cd4245d8040732ee1d2f69056329a64cc2`, a
five-minute horizon, full raw JSONL retention, and `GOMAXPROCS=3` per cell.
The retained machine-readable score input is
[`p1-summary.json`](artifacts/v2-3-p1/p1-summary.json).

## Evidence and activation

All four cells pass the required information, policy, join, and viability
gates. The participant-information replay reports zero future decision use and
zero bad decision frontier in every cell. The P1 checker now independently
joins every pre-send quantity decision to an accepted/rejected order request by
`(venue, client, request_id)`, and separately retains terminal-horizon-censored
sides. It found no missing, duplicate, policy, request-field, or censoring
failure.

The first checker incorrectly pooled same-named numbered makers across venues.
This was an analyzer defect, not evidence loss: raw logs identify each event's
venue. Commit `90829d2` splits those rows by `(venue_id, maker)`, adds an
adversarial regression fixture, and all artifacts were regenerated. Each cell
now has 24 active scoped venue-maker rows (three venues × eight spot Stoikov
makers); none is hidden by a role aggregate.

| arm / seed | decisions | nonzero risk | nonzero adjustment | long `ask>bid` | short `ask<bid` | nonzero risk, zero skew | accepted / rejected | terminal censored sides |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A / 101 | 1,386 | 1,350 | 0 | 0 | 0 | 1,350 | 2,731 / 37 | 4 |
| A / 103 | 1,429 | 1,393 | 0 | 0 | 0 | 1,393 | 2,784 / 62 | 12 |
| B / 101 | 3,090 | 3,059 | 3,059 | 704 | 2,355 | 0 | 6,036 / 120 | 24 |
| B / 103 | 2,939 | 2,906 | 2,906 | 398 | 2,508 | 0 | 5,844 / 10 | 24 |

Zero-risk decisions were symmetric in all four cells; wrong-direction size
rows were zero. Thus P1-B's every nonzero-risk decision had a representable
nonzero adjustment in the declared direction, while P1-A's every nonzero-risk
decision remained symmetric. The raw checker derives the integer formula with
`math/big`, not the actor's checked multiplication helper; side-swap,
coefficient-strip, request-quantity, missing-outcome, terminal-delivery, and
venue-maker-pooling mutations are caught.

**Primary verdict: SUPPORTED (screening).** The intervention is active in both
paired seeds and the observed signed quantity association is exactly the
preregistered local policy. This does not establish realism or stability.

## Viability and descriptive outcomes

All scoped books retained snapshots, a positive two-sided share, trades, and
volume in all three venues. The table below is deliberately descriptive; it
does not alter the activation verdict.

| seed | book | A two-sided / trades / volume | B two-sided / trades / volume | B − A trades |
| ---: | --- | --- | --- | ---: |
| 101 | ABC/USD | 0.990 / 11,300 / 87,723,985,018 | 0.990 / 10,149 / 77,346,097,995 | -1,151 |
| 101 | CDF/USD | 0.968 / 5,450 / 792,121,832,188 | 0.990 / 5,191 / 654,831,509,560 | -259 |
| 101 | ABC/CDF | 0.909 / 2,929 / 299,057,123,878 | 0.943 / 3,238 / 295,590,891,317 | +309 |
| 103 | ABC/USD | 0.990 / 12,527 / 91,779,097,526 | 0.990 / 7,997 / 77,817,222,799 | -4,530 |
| 103 | CDF/USD | 0.964 / 4,642 / 838,327,632,005 | 0.988 / 3,219 / 724,413,584,026 | -1,423 |
| 103 | ABC/CDF | 0.917 / 2,902 / 280,080,094,957 | 0.902 / 2,434 / 357,003,924,001 | -468 |

P1-B increased the pooled absolute maker net-delta path in both pairs:
`+3.374e9` (seed 101) and `+1.231e9` (seed 103) raw units. Mean per-series
lag-one net-delta autocorrelation changed by `-0.0021` and `-0.0146`,
respectively. This is evidence that the short-horizon ecology responds to the
redistributed displayed size; it is neither a predeclared improvement target
nor evidence of worse or better risk management by itself.

First-to-terminal executed-trade ratios are retained as a descriptive,
explicitly defined statistic. Across all nine venue/book rows, they remain
near one (A ranges 0.99933–1.01350; B ranges 0.99933–1.01026). A ratio is
reported unavailable rather than numeric zero if a book has no executed trade
or an opening trade price of zero. No conclusion about runaway suppression is
licensed by these five-minute paths.

The treatment also changes the number of maker decisions and requests. That is
an expected endogenous consequence of quantity changes becoming actionable at
the existing refresh cadence—not an unregistered clock change. It means the
later explicit-rebalance experiment must keep request/cadence accounting in
its activation contract rather than attribute any price outcome to a passive
coefficient alone.

## Interpretation and next gate

P1 establishes one narrow fact: with P0-C passive refresh fixed, the configured
spot maker family can visibly reduce bid appetite and increase ask appetite
when long, with the reverse when short, without killing the short-screen books.
It does **not** show that this policy stabilizes CDF/USD, improves inventory
risk, produces a realistic ecology, or replaces a costly aggressive rebalance.

The next V2-3 gate is therefore a separately preregistered P2 explicit
inventory-rebalance policy. It must introduce its own order telemetry, fee and
slippage accounting, participation/cooldown/risk limits, activation test, and
causal comparison. It must not retune P1's size coefficient, spreads, clocks,
latency, or demand after this screen.

Raw evidence remains retained. The extraction script neither prunes evidence
nor marks any P1 cell safe to prune.
