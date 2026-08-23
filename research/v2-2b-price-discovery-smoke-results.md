# V2-2b — informed makers × executable routers smoke results

## Scope and provenance

This is the outcome record for the preregistration in
[V2-2b](v2-2-price-discovery-smoke-preregistration.md). It is a two-seed,
five-minute Tier-C market interpretation screen, not a new freeze, calibration
result, realism claim, or robust price-discovery result.

- Simulator/input revision: `69b2537`; all eight immutable inputs are under
  `research/configs/v2-2b/`.
- Analysis revision: `51f46e0`; extractor:
  `scripts/extract-v2-2b-metrics.sh`; summary builder:
  `scripts/summarize-v2-2b.sh`.
- Raw evidence and compact outputs:
  `research/artifacts/v2-2b/{arm}/seed-{seed}/`.
- Machine-readable aggregate:
  `research/artifacts/v2-2b/summary.json`, SHA-256
  `da73de7a94f9d0d43e05e655cec0fe8fe9ac0d40084ea78d56499877c2ba51b1`.
- Every completed world has final `greeks.json` and `latency.json`; these are
  the only completion sentinels. Raw logs are retained. No world was rerun
  after an analyzer correction.

The offline midpoint statistic uses fresh, two-sided ABC/USD snapshots from
all three venues at a two-second staleness bound. Its after-fee edge companion
is an omniscient 5-bps bid/ask upper-bound diagnostic, not a router information
set or a router PnL estimate. Router cashflow is in quote atomic units and is
separately reconstructed from its non-atomic group legs.

## Evidence and activation gate

All eight V2-0 receipt replays and V3 frontier-vector replays are valid. The
new per-link activity report closes V-037: every declared remote feed in I1
cells delivered 361 receipts in R0 and 362 in R1; feed-only links emitted zero
orders. Remote-maker vectors are nonempty (821 for seed 101 and 775 for seed
103) and valid in both I1 arms. Thus the remote information treatment is
activated, not merely configured.

Every cell has three fully completed 100-ABC metaorders with a positive,
available VWAP and 20 or 21 child orders. Each parent overlaps 37--39 sampled
positive cross-venue-dispersion observations. This establishes an active local
execution population and observed dislocation during parents, but does **not**
causally attribute the dislocation to those parents: this feasibility design
has no metaorder-off source control.

The direct router report reconciles leg counts, quantities, notionals, fees,
cashflow-qualified completed groups, and residual inventory in every cell. In
I0R1 it submits/completes 72/71 groups for seed 101 and 8/7 for seed 103, with
one terminal pending group per seed and zero base residual. I1R1 has zero
signals and zero submitted groups in both seeds. This is observed mechanism
inactivity, not a router failure or a negative cashflow being hidden.

## Paired measurements

| arm | seed | mean midpoint range (bps) | after-fee edge observations | longest edge run (s) | router submitted/completed |
| --- | ---: | ---: | ---: | ---: | ---: |
| I0R0 | 101 | 8.804 | 252 | 134 | 0 / 0 |
| I1R0 | 101 | 5.560 | 0 | 0 | 0 / 0 |
| I0R1 | 101 | 8.801 | 252 | 134 | 72 / 71 |
| I1R1 | 101 | 5.560 | 0 | 0 | 0 / 0 |
| I0R0 | 103 | 7.488 | 30 | 14 | 0 / 0 |
| I1R0 | 103 | 5.001 | 0 | 0 | 0 / 0 |
| I0R1 | 103 | 7.488 | 30 | 14 | 8 / 7 |
| I1R1 | 103 | 5.001 | 0 | 0 | 0 / 0 |

Remote informed makers lower mean dispersion by 3.244 and 2.488 bps with the
router off. They also remove 252 and 30 sampled post-fee edge observations,
respectively. Router-on minus router-off with informed makers off changes mean
dispersion by -0.003 and 0.000 bps, with zero change in edge observations or
longest sampled edge run. With informed makers on, the router never activates,
so its conditional effect is unmeasured.

The small route lot nevertheless produces both favorable and unfavorable
realized groups after the one-second non-atomic leg delay. Its seed-101
completed cashflow is 435,701 quote atoms and seed-103 cashflow is 8,590 quote
atoms. These values are direct route accounting, not an argument that the
omniscient scanner's quoted edge was captured at every fill.

## Component verdicts

| component | verdict | interpretation |
| --- | --- | --- |
| delayed remote-feed activation | SUPPORTED (screening) | exact feed-only sessions delivered data; scalar and vector evidence is valid |
| quote-mediated reduction in this screen | SUPPORTED (screening) | both paired seeds have lower fresh midpoint dispersion and no post-fee scanner edge under I1 |
| local execution population active | SUPPORTED (feasibility) | all parents complete and overlap sampled dislocation; causal source attribution remains unresolved |
| router activation without remote feeds | SUPPORTED (screening) | 72/8 submitted groups, reconciled direct reports |
| router reduces residual snapshot edge under I0 | FALSIFIED (screening) | both paired controls have unchanged sampled edge counts and lifetimes despite completed routes |
| router effect conditional on informed feeds | NOT IDENTIFIED | remote feeds eliminate observed post-fee route signals, so no R1 orders exist |
| quote-mediated versus trade-mediated decomposition | MIXED / incomplete | the quote channel is active; the trade channel cannot be estimated when I1 suppresses its own activation |

Two seeds do not establish robustness. The result is also cadence-specific:
ten-millisecond/heterogeneous remote feed delays, one-second quote/snapshot
clocks, and one-second router legs remain explicit inputs, not incidental
implementation details.

## Consequence and next discriminating experiment

Do not tune the router after this screen. Its small lot visibly executes yet
does not move the registered snapshot statistic, while remote quotation removes
the route opportunity itself. The next test must preregister a router-capacity
and action-clock identification design with the remote-maker channel held
fixed, report route participation relative to displayed depth, and measure
event-time edge decay in addition to one-second snapshots. It must retain
non-atomic leg-risk accounting and a source-off execution control before any
claim about trade-mediated convergence.
