# ME-005 development report — valid no-opportunity boundary

Status: **REVIEWED DEVELOPMENT RESULT; NO-OPPORTUNITY BOUNDARY**. The entire
preregistered budget—four economic cells and one ON-1109 technical duplicate—
completed. No parameter, horizon, seed, counterparty, fee, or router policy was
changed after observing an outcome. This is not confirmation, a holdout result,
an arbitrage-profit estimate, or an empirical-market claim.

## Question and exact scope

The [r1c protocol](protocol.md) asked whether the existing finite,
prefunded, two-venue ABC/USD router would encounter a positive one-lot,
fee/depth-adjusted quote edge in its delayed local information; attempt
non-atomic buy/sell FOK legs; and have a reconstructible result after
hypothetical, depth-limited, venue-local terminal inventory restoration. It
also registered a secondary OFF/ON public executable-edge episode count and
duration. The venue-local restoration is **not** an executed transfer or a
claim of realizable intervenue arbitrage. Trade-attributed convergence was
prospectively `NOT_IDENTIFIED`.

The study used the fixed two-venue, price-time ABC/USD ecology; a 0.01-ABC
lot, 5-bp taker fee, 2-s configured courier delay, finite independent router
accounts, one attempt maximum, two development seeds and a five-simulated-
minute horizon per cell. The simulator also constructed background instruments
outside the scored ABC/USD spot pair. ON added a finite router endowment;
total system capital was not held constant. The broader leg-side executable
edge is a diagnostic, not the current router's two-sided policy denominator.

## Retained outcome: opportunity absent in this grid

| Seed | Arm | Two-sided policy episodes / positive duration | Broader leg-side episodes / duration | Router evaluations | Submitted groups |
|---:|:---:|---:|---:|---:|---:|
| 1109 | OFF | 0 / 0 ns | 0 / 0 ns | N/A | N/A |
| 1109 | ON | 0 / 0 ns | 0 / 0 ns | 783 | 0 |
| 1117 | OFF | 0 / 0 ns | 0 / 0 ns | N/A | N/A |
| 1117 | ON | 0 / 0 ns | 0 / 0 ns | 790 | 0 |

All 1,573 retained ON evaluations were classified by the router as
`NO_POSITIVE_POLICY_EDGE`. The public replay independently found zero positive
policy episodes and zero broader leg-side episodes in all four cells. There
were no submitted, admitted, one-leg or matched arbitrage groups; the
per-attempt fill/gross-edge/fee/residual/closeout table is therefore empty,
**not** a table of zero-return trades. The unchanged router-account portfolio
has zero balance delta and a numerical terminal value of zero, but no
attempt-level economic profit is observed. No route-derived opportunity
lifetime, observation/action latency, or residual closeout cost can be
estimated. The fixed configured latency and the receipt-verified local
evaluation path are mechanical facts, not a measured response to an edge.

The registered ON-minus-OFF positive-edge-duration difference is descriptively
0 ns at each seed because both arms had zero episodes. That is an
**identification limitation**, not evidence that arbitrage has no effect on
dispersion, that the router would fail on an edge, or that its matched legs
would be profitable. Equal seed labels and matched background configuration
digests do not prove coupled realized random streams; with no opportunity,
there is no route-mediated contrast to interpret. The two-seed screen cannot
estimate an opportunity frequency or tail-risk rate. No formal equivalence or
trade-attributed convergence claim is issued.

The per-episode delivery caveat in r1c remains relevant: the timeline binds
consumed local sources for actual evaluations, but zero in-episode evaluations
would mean `NO_EVALUATION_OBSERVED / DELIVERY_NOT_DETERMINABLE`, not proven
nondelivery. In these worlds there were no public policy episodes to classify.

## Provenance, validity, resources

Execution/analyzer source C is `442da046f8cc9a6b78ce3900ec13badd3fd2b74a`
(tree `eda8e6d56187c624ce6ce06800371741f91021ee`); governing protocol P
is `96d240ad60bbfb9e3c73c9f0eb21acd665b156ee` (protocol blob
`db7d672bd31a4e1d8f88c95f09458c5c5ebca886`). The four config files and
effective hashes are frozen in P and the four analysis contracts. Both
Go 1.27.0 linux/amd64 binaries report C with `vcs.modified=false`:

- `multivenue` SHA-256 `01422b114b94e1ee654c8e81f45ec85c575e47eae16b4ed521fb9fee7eecb7c1`;
- `me005analyze` SHA-256 `59b3aa4f65cd547828f0f3283eea9e1579c7ccd7b91896e02d2face177fc89bb`.

External retained namespace: `/home/vlad/ExchangeSimulation-me005-evidence-20260924`.
It contains five distinct retained, unmodified raw run directories, five rendered trees,
five individual Go-analyzer results, pinned binaries, and `preflight.json`.
The machine [result](result.json) records the four economic result SHA-256s,
canonical execution hashes, cell counts and evidence IDs. This external path
is an index of retained local evidence, **not** proof of a remote backup.

The runner observed zero exit status for each simulator and analyzer process;
the retained result files do not independently attest those shell statuses.
The analyzer verified full
ordered `evstream_v3` rendering, exact source/effective-config/report binding,
strict terminal horizon, accounts, venue movements and run-wide conservation;
an incomplete or contradictory cell would have been invalid, not scored as a
no-opportunity world. The ON-1109 technical and economic runs, invoked with
recorded `GOMAXPROCS=1` and `GOMAXPROCS=7` settings respectively, have identical
canonical execution hash
`bf0d19765bbc0b1289c5c269562103ca9c2490b592c05967edb9ef407c17b740`
and byte-identical reconstructed result SHA-256
`8abb776e46e1fd3419b28407314ca951e7e9c5e1673ca49533353d54829b3c41`.

The [independent preregistration reviews](../../../reviews/me005-r1c-prerun-review-20260924.md)
accepted C/P before any economic outcome. The clean full `make test`, `go vet
./...`, targeted race, skill-validator and whitespace gates passed on C's
unchanged executable tree. The finite-cgroup preflight recorded actual child
placement, `MemoryMax=12 GiB`, zero swap, `CPUQuota=700%`, and exit-status and
timeout propagation. Available disk and RAM before first execution were
75,952,738,304 and 31,663,923,200 bytes, above the 55-GiB/16-GiB floor.
Each raw-plus-rendered economic cell used 33.1–35.4 MB of disk at inspection;
together they used 136,876,032 bytes. The entire retained namespace used
188,973,056 bytes at first report inspection. No resource-limit failure was
observed, but final disk size does not attest peak staging, and the result
files do not independently prove per-process CPU/memory settings or exit
statuses. The simulator printed coarse `wall=0s`; no precise per-process peak
RSS or wall attestation was retained, so none is invented. Manifests are
content-bound, not cryptographically signed shell attestations.

## Offline reproduction and boundaries

Use a clean checkout of C and Go 1.27.0 to rebuild the two binaries, checking
their hashes and build VCS fields above. For each existing raw run, render and
analyze into **new**, non-overlapping output paths with the corresponding
`contract-<arm>-<seed>.json` from P, then compare the result SHA-256 in
[result.json](result.json). For example, without launching another world:

```bash
me005analyze \
  -raw /home/vlad/ExchangeSimulation-me005-evidence-20260924/on-1109 \
  -rendered <new-empty-rendered-path> \
  -contract <P-checkout>/research/program/ideas/ME-005/contract-on-1109.json \
  -out <new-result-path>
```

The command renders and checks existing immutable evidence; it does not run
the simulator. The GOMAXPROCS control, all four result hashes and raw
execution hashes are in the machine result. Retain every raw and initially
invalid attempt; none was removed here. Historical three-venue router runs,
ME-001/002/003, and R2/SV1D retain their original code/population/results.

No new cell, alternative seed, longer horizon, reduced fee, injected economic
edge, larger capital allocation or background retuning is licensed by this
no-opportunity result. A synthetic positive-edge **fixture** remains a
mechanical test of matching and accounting, not evidence of naturally
occurring arbitrage. Confirmation/holdouts and empirical comparison were not
run. The finite ME-005 development budget is exhausted.

## One recommended next study, not authorized

Among the planned ideas, [ME-002-B](../ME-002-B/idea.md)—decision cadence
versus transport/compute delay—is the best next bounded question: ME-002's
fixed decision gate already produced a timing-channel non-activation, so a
prospectively specified phase/cadence contrast can discriminate clock
resolution from actual latency irrelevance with comparatively low semantic
and compute cost. It is not an ME-005 rescue and must receive its own owner
authorization and protocol. ME-006's informed-maker × arb factorial is more
directly ecological but needs an exercised arbitrage opportunity set;
ME-007's three-asset conversion has higher asset/fee/residual semantics cost;
ME-008 still needs a verified capital→orders/exposure/payoff channel. None is
started by this report.

No plot is generated: four zero-valued episode series and no attempts are
clearer in the complete table above than in a graph. Three bounded
[post-result independent reviews](../../../reviews/me005-post-result-review-20260924.md)
accepted this no-opportunity conclusion with limitations and required precise
wording about operational settings and duplicate-run identity. Neither C, P,
nor the raw trajectories changed.
