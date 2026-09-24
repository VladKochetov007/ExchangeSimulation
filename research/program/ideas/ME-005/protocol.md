# ME-005 development protocol r1 — two-venue first-attempt screen

Status: **PREREGISTERED, CONDITIONAL**. This document was written before any
ME-005 economic outcome. Execution is permitted by the owner's conditional
ME-005 instruction only after both bounded independent reviews accept the
exact code/config/protocol package and the resource preflight below succeeds.
The four readiness requirements remain defined by
[readiness-20260924.md](readiness-20260924.md); the prospective local-closeout
choice is [prepare-contract-20260924.md](prepare-contract-20260924.md).
Historical V2-2b, ME-001/002/003 and R2/SV1D claims retain their own sources.

## Question, claim and falsifier

Can the existing finite, prefunded two-venue router identify a positive
fee/depth-adjusted ABC/USD quote opportunity in its delayed local information,
attempt two non-atomic market-FOK legs, and produce a reconstructible net
economic result after *hypothetical venue-local terminal restoration*? A
positive quote edge or matched cashflow is not by itself transferable profit.
The primary output is the full public-to-local-to-admission-to-fill-to-local-
value funnel, including valid zero-attempt, unmatched, negative and unavailable
states. The strongest primary claim is a two-seed **development mechanical and
economic response map**, not general arbitrage profitability. A router that
receives a feasible positive local quote but does not attempt a route, a
failure at venue arrival, a negative terminal value, and no opportunity are
distinct possible results. No sign is required for promotion of a valid result.

The secondary OFF/ON outcome is the per-world total half-open duration and
count of one-lot positive public executable-edge episodes, reported per seed
and as paired differences only when both cells are valid. It is a bounded
router-enable comparison in this ecology, not trade-attributed convergence:
`trade_attribution = NOT_IDENTIFIED`. Identical seeds and normalized
background configs do not prove identical realized random streams. No
equivalence margin, p-value, empirical corridor, tail-risk or general market-
convergence claim is registered. If there are no public positive episodes,
the mechanism's opportunity set is absent in this small screen, not falsified.

## Candidate, policy, ecology and exact cells

Executable/analyzer source commit C is
`b9ed591e255ff951b53dc2c001c6ac25a90b7b0e` (tree
`05352204d1254c7436ea11318ef2e0857741a2de`), built cleanly with
Go 1.27.0 linux/amd64. Skill directory tree at C is
`60b91e73f1000a285e003bed122648636a737b2d`. This later documentation
commit governs the protocol but does not change C. Build the simulator and
`me005analyze` from C; the run manifest must identify exactly C as clean.
Any source, config, estimator, endpoint or protocol change needs a named
successor amendment and affected cells rerun. Existing outcomes are retained.

The policy is the current `CrossVenueArb` two-sided-book, one-lot,
positive-after-fee touch-edge rule. It observes two delayed venue-local feeds,
checks its own prefunded venue balances, and submits a buy FOK then a sell FOK
through the existing courier. Max one group/world; it may rationally abstain.
Both venues use price-time matching. There is no transfer/shared wallet and no
router borrowing. Each ON venue account begins with 1 ABC (`100000000` atoms)
and USD 100000 (`10000000000` quote atoms); the order lot is 0.01 ABC
(`1000000` atoms). Taker fee is 5 bp in USD atoms (`100000` atoms/USD),
charged under the existing exchange rounding rule. The router's configured
base courier delay is 2 simulated seconds with tier 1, unchanged across ON
cells. Record realized publication→receipt→decision→request→venue placement→
execution→response times; do not infer action latency from configuration.

Target market: ABC/USD spot on `north` and `south`; no other symbol is scored.
The multivenue simulator still constructs its fixed background spot,
perpetual, futures and options actors/instruments. This is **not** a pure
one-asset market. Background code/defaults, two venue rules, maker/noise
roster, information paths, clocks, fees, capital and dealer-hedge mode are
held fixed. Derivative tenors begin at one hour or longer, beyond the
five-minute horizon; presence is not a lifecycle result. The ON intervention
adds exactly one router with two funded accounts, so total system capital
differs by that declared finite endowment. It is not a fixed-total-capital
replacement experiment. The OFF arm has no router account or evaluations.

Four economic cells, each five simulated minutes from 2025-01-01 00:00 UTC to
00:05 UTC (terminal `1735689900000000000` ns), one-second step:

| Cell | Seed | Raw config SHA-256 | Effective config SHA-256 |
|---|---:|---|---|
| [OFF-1109](config-off-1109.json) | 1109 | `494ed63730a898aafa9288d60f0d94bd30ed6a4a94d8993ed7377a3eb5e429f5` | `c121ae21036019d97edf1ed0ca60e8587250e8e842582b768f12447455016603` |
| [ON-1109](config-on-1109.json) | 1109 | `a1e59a0e36bc02493c9178befba7a19c265bf470952eca5caf18f780457bde1d` | `3a4e6a37f269dba160bafd5bbe354d53d84431f9deca3306a088a589558ab0e2` |
| [OFF-1117](config-off-1117.json) | 1117 | `8327b63e8ab1967a93a6302f8a0b5a825f59f341327209b221f152bb173f76cf` | `fb7d72c1d8c0bae6ea8cb005d450df9b02ad7eca6765c45d830e9f5cc148c1b5` |
| [ON-1117](config-on-1117.json) | 1117 | `e8d3bb80b93b75de6415107aa89031000e1d54ed5d9d952ec14c5357f5b94347` | `ec5a09120d64880a684086549a7970d24098dcb46f92f4214741515dd11192b9` |

The source/config normalization-only test fixes the effective digests; actual
manifests must match. These two seed labels were selected before any ME-005
outcome after a tracked `research/program` and `research/configs` metadata
search found no ME-005 or reserved-holdout collision. That limited search is
not a claim of universal seed novelty. No 619/631/641 holdout is touched.

## Estimators, denominators and validity

Use canonical `evstream_v3` contract version 2. Renderer verifies the complete
ordered stream; the analyzer checks clean source C, effective config, seed,
raw/rendered reports and execution attestations, exact terminal horizon,
strict population accounting and run-wide ledger conservation. Any missing,
malformed, duplicated, misordered or contradictory required identity is
`INVALID_EVIDENCE`, not a market null. Retain its raw incomplete attempt.
No output is interpreted merely because the process exited zero.

For both arms, independently replay public book snapshots/deltas in global
event-frame order. A public opportunity is a half-open interval with positive
one-lot bid/ask/depth/fee edge in either direction; same-time transitions and
horizon censoring remain explicit. The router policy additionally requires
both sides of each local book. For ON, audit received local publication
prefixes and each actual evaluation, including no-action; join submitted
vectors, gateway placement, exchange-time FOK fill/cancellation, actor inbox
receipt, account movements and terminal venue-local book. A public episode
with no aligned evaluation is not an actor rejection. Quote-time funding is
not proof of arrival-time depth/admission. Preserve unknown disappearance
cause where event identities cannot distinguish price, depth, other execution
or latency. Do not read a periodic midpoint as executable opportunity.

For a matched group of quantity `q`, report actual sell proceeds minus buy
cost before fees, charged USD leg fees, and matched cashflow after fees.
Report venue-local ABC and USD changes even if global ABC residual is zero.
At the common terminal horizon, hypothetically sell each positive local ABC
delta into that venue's displayed bids and repurchase each negative delta
from that venue's asks, walking finite depth and applying taker fees. This
does **not** send additional orders. `final_net_value = sum(quote_delta[v]) +
sum(local_restoration_cashflow[v])` because the configured router is
non-borrowing and movement evidence must prove no financing/transfer.
The full value is unavailable on insufficient, invalid, out-of-domain or
over-age book evidence; no midpoint/infinite-depth/zero substitution.
The selected last-publication recency bound is 10 simulated seconds, chosen
before results against the 1-second quote cadence and 2-second courier.
No attempt has no matched-edge observation; it is not zero arbitrage profit.

Report all four cells and every attempted group (at most one per ON world),
including losing and unpriceable attempts. Classify process failure,
incomplete binary run, valid no public opportunity, public opportunity not
delivered while active, delivered but infeasible/no-action, order refusal,
one-leg fill, matched legs, unavailable local closeout, negative/zero/positive
terminal value separately. The primary response map is not conditioned only
on surviving profitable attempts. For valid paired edge-duration results,
publish both raw seed values and ON-minus-OFF differences; two seeds support
only a screening range/median, not significance or reliable tail inference.
If either cell of a seed is invalid, its paired contrast is `NOT_DEFINED`.
No post-result parameter, seed, horizon or capital rescue is allowed.

## Execution boundary and resource stop

After both independent reviews accept, use one clean checkout of C and hash
the Go 1.27-built simulator/analyzer binaries. Record binary digests and raw
config/contract digests before the first cell. Use new exclusive external
namespaces; never overwrite or delete an old raw/rendered/result tree.
First execute ON-1109 twice as a fresh-process determinism control,
`GOMAXPROCS=1` then `7`, with otherwise identical candidate/config. The
second execution is the preselected ON-1109 economic cell; the first is a
technical duplicate, not a third seed. Require identical canonical execution
hash and reconstructed result aside from artifact-path/build metadata before
proceeding. Then execute OFF-1109, OFF-1117, ON-1117 in that order. Maximum
five simulator processes, four economic cells, 300 simulated seconds each.
Do not rerun a valid losing or inactive cell selectively.

One process at a time. Each simulator and analyzer process runs with at most
7 Go workers, finite cgroup `MemoryMax=12G`, `MemorySwapMax=0`, `CPUQuota=700%`
and 10-minute wall timeout. Total batch stop at 60 minutes of simulator wall
time, measured rather than assumed. Before the first cell require at least
55 GiB free disk and 16 GiB available RAM; before every next run require at
least 25 GiB free disk and 16 GiB available RAM. Stop if a cell's combined
raw+rendered tree exceeds 10 GiB or if projected retained/staging volume
would leave less than 25 GiB free. These are capacity stop rules, not
incentives to prune protected evidence or lower strict checks. Verify actual
scope placement, resource settings and exit-code propagation with harmless
process controls; preserve incomplete attempts. If a hard cap/timeout or
evidence defect occurs, stop comparable cells, diagnose prospectively and
amend/review the candidate before rerunning affected assignments. Valid
economic losses/nulls do not stop the batch.

Only four development cells and the stated technical duplicate are licensed.
No holdouts, confirmation, ME-002B/006/007, transfer-policy successor,
capacity frontier, trade-mediated convergence or empirical claim follows.
After analysis, two independently scoped reviews of the result/claim may
challenge provenance and interpretation; they cannot change this protocol
after outcomes. Preserve all attempts and publish the standard report and
typed result, then recommend one next study without running it.
