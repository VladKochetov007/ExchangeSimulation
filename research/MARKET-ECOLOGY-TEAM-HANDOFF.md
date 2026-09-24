# ExchangeSimulation: market-ecology research handoff

Status date: 2026-09-24. Source and research state inspected at main commit
6c79eafe3d41b91a35c500f16ca3159172bfe70b; this documentation-only
handoff is a later commit. This is a team entry point,
not a replacement for the authoritative [registry](program/registry.json),
study protocols, immutable evidence or current Git state. Check the remote
branch and owner instructions again before starting work. Historical paragraphs
elsewhere may still speak from an older branch or planning checkpoint.

This document is meant to travel with the repository. Its links are relative
to this file. External raw-evidence paths in linked reports are specific to
the original machine and are not copied or backed up merely because a Git
report names them.

## 1. Mission and scientific standard

We are building an executable laboratory of counterfactual market ecology:

> How do participant objectives, beliefs, capital, information, execution
> rules, financing and risk constraints jointly determine opportunities,
> execution quality, profitability, liquidity, impact and stability?

The target is a map of **conditional** relationships, not one universally best
strategy or a chart that looks realistic. Agents should interact through
observable feeds, orders, queues, trades, balances and contract lifecycles.
Aggregate price, spread, volume, basis, volatility and survival should not be
hard-coded as desired outcomes.

The platform implements much more than any first study should activate:
spot, perpetuals, dated futures, options, multi-venue trading, fees, borrowing,
funding, margin, liquidation, settlement, delayed gateways and binary
evidence. Software presence is not proof of economic activation, causal
identification, replication or empirical realism. A market simulator is not a
forecast of real trading profit.

Keep four entities distinct:

| Entity | Meaning | Common confound |
|---|---|---|
| Policy | Algorithm, parameters and economic objective | A programmed rule mistaken for an emergent law |
| Actor | Instance, endowment, inventory, debt, liabilities and risk budget | More cash that never changes orders called capacity |
| Deployment | Feed, request and response paths; clocks and computation | Strategy identity accidentally determines speed |
| Venue | Matching, admission, fees, market data and lifecycle | Order instruction confused with allocation or routing |

The same policy may be deployed by actors with different resources or delays.
No actor should read an unregistered instantaneous global state. Initial
endowments, private values, liability paths and information processes are
legitimate *assumptions*; their contribution must be distinguished from an
identified endogenous effect.

## 2. Authority and where to look first

- [Research navigation](README.md) and
  [current operational pointer](RESUME-HERE.md) explain the development
  repository and the closed R2/SV1D line.
- The [program guide](program/README.md) and
  [machine registry](program/registry.json) identify each ME study's current
  stage, authorization, next boundary and authoritative files.
- The [policy catalogue](program/policy-catalogue.md) maps 24 proposed policy
  families to source status. IMPLEMENTED means source exists, not that it is
  profitable or fully validated. PARTIAL and UNVERIFIED must remain visible.
- The [consolidation report](REPOSITORY-CONSOLIDATION.md) explains historical
  branches and deferred contributions. Do not repeat whole-worktree
  archaeology for each new idea.
- The repository-local
  [market-ecology research skill](../.agents/skills/market-ecology-research/SKILL.md)
  supplies the reusable cycle, templates and typed result contract.
- The [next-study readiness packet](NEXT-STUDY-READINESS-PACKET.md) and
  [pilot draft](market-ecology-capacity-pilot-protocol-DRAFT.md) explain why the
  first pilot used a one-venue immediate executor instead of treating an
  inventory-sensitive maker as already payoff-ready.

The exact current main commit is a development source identity, not the source
identity of every historical trajectory. Read the manifest and protocol for
each run. Never assign an old trajectory to a later analyzer or merge commit.

## 3. Results that may be cited, with boundaries

| Line | Retained finding | Strongest permissible reading |
|---|---|---|
| [R2/SV1D](v2-r2-sv1d-iteration-closeout.md) | R2 predecessor was NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE. The corrected retained seed-659 supplier probe is evidence-valid and supplier-active, but TREATMENT_NOT_ACTIVATED. | The supplier traded without meeting the registered missing-side restoration predicate. Not a universal supplier failure, tri-arm promotion or completed successor survival campaign. |
| [ME-000](program/ideas/ME-000/report.md) | Provenance/configuration and independent opportunity/execution readiness for the first pilot were closed. | Mechanical readiness, not an economic result. |
| [ME-001](program/ideas/ME-001/report.md) | 27 economic cells plus two controls. Maker/random-taker replacement changed delivered depth and immediate-buy execution response; shortfall direction varied by size. | A simulation-internal, bundled composition response map, not isolated maker profitability or capital capacity. |
| [ME-002](program/ideas/ME-002/report.md) | 24 economic cells plus two controls. Network delay altered order arrival and had mixed-sign 5-ABC fill effects. Processing changed selected information but not arrival under the fixed one-second decision gate. | A bounded timing result, not general speed or geography irrelevance. |
| [ME-003](program/ideas/ME-003/report.md) | 12 economic cells plus two controls. One 5-ABC pair showed a partial IOC fill versus rejected FOK; five other pairs had zero filled-fraction contrast. | One observed instruction-path difference, not a universal IOC/FOK ranking. Rejection reason alone does not isolate book depth. |
| [ME-005](program/ideas/ME-005/report.md) | Four valid five-minute economic cells plus one technical duplicate. No registered two-sided public executable edge or broader leg-side edge in either development seed; routers evaluated but submitted no route. | An opportunity-absent identification boundary. No arbitrage-return, route-on-edge or trade-attributed convergence estimate. |

The completed ME screens are **development** studies, not frozen confirmation,
untouched holdouts, real-market validation or interchangeable replications.
Their exact source, config, analyzer, skill, review and evidence identities are
in each report and machine result. The R2/SV1D line remains separately closed:
the seed-659 raw trajectory came from 971267d, while 8de607d identifies its
accepted queued-fill analyzer correction/rescore. Do not rewrite the old
invalid score or trajectory. The R2 calendar amendment is mechanically tested
but did not itself establish a liquid, surviving 24-hour ecology.

No current line authorizes old reserved holdouts 619/631/641 or automatic
dev-607/dev-613/dev-617. A later study must obtain its own owner-approved
development and confirmation boundaries.

## 4. Market reality and measurement traps

An observed price gap is not automatically executable arbitrage. Directional
bid/ask, reachable depth, lot/tick precision, fee currency, balances,
financing, admission, non-atomic leg timing, venue-local inventory and
closeout cost all matter. A midpoint cycle, matched-leg gross cashflow,
terminal inventory value and final economic result are distinct objects.
Static manufactured positive-edge fixtures establish mechanics, not naturally
occurring alpha.

For execution and information studies, reconstruct this chain:

    public event/publication
      -> actor's delivered information and decision eligibility
      -> decision or documented abstention
      -> request, venue arrival and admission/rejection
      -> exchange-time fills and cancellations
      -> delayed response receipt
      -> cash, asset, debt and fee movements
      -> terminal obligation, risk and declared objective

The actor's sampled quote need not remain executable at request arrival.
An exchange execution before cancellation may be delivered afterward; actor
receipt time is not exchange match time. Do not infer global ordering from
directory traversal. If a causal link or price is missing, classify it as
unknown or invalid under the registered contract rather than substituting
zero, a midpoint or a favorable mark.

Distinguish accounting identities, programmed policies, theoretical
approximations and empirical regularities. A programmed inventory skew is
activation of code, not discovery of an inventory law. Currency/asset/debt
ledgers should reconcile with explicit sources and sinks; aggregate *marked
wealth* need not be conserved. A purchase payment is not automatically buyer
loss. A liability hedger can rationally pay trading costs to reduce exposure,
so trading PnL alone may not be its utility. Signed-price domains can make log
returns, ratios, option inversion and annualization undefined.

The declared R2 compressed calendar lists short/medium/long families every
1/3/6 simulated hours for expiries 2/6/12 hours away. Contracts are keyed
economically by underlying, type and expiry; overlapping families deduplicate.
The [calendar amendment](v2-r5-r2-calendar-amendment-2026-08-30.md)
specifies the lifecycle. It does not encode convergence or prove that term
traders found simultaneous profitable maturities.

Empirical comparison is separate from more simulator seeds. The
[LOBSTER sample](https://data.lobsterdata.com/info/DataSamples.php) supplies
message and synchronized order-book files, with an
[official structure description](https://data.lobsterdata.com/info/DataStructure.php).
Compatible spreads, displayed depth and hypothetical sweep costs are plausible
future benchmarks; those files do not identify the hidden objectives or
population classes in our simulator. Pin symbol, date, regime, units, sampling,
costs and corporate-action rules before an empirical claim.

## 5. Research cycle: one hypothesis at a time

The [skill cycle](../.agents/skills/market-ecology-research/references/research-cycle.md)
is the default. Its modes are natural prompt conventions: INTAKE, PLAN,
PREPARE, RUN, ANALYZE, REPORT and SYNTHESIZE. Invoking the skill is not
authorization to execute the registry.

1. Start from one question and its historical predecessors. Identify the
   strongest intended claim: mechanical, descriptive, activation, causal,
   replication, empirical or game-theoretic.
2. Write the actor objective, resources, actual information, local mechanism,
   predicted aggregate effect, competing explanations and a discriminating
   contrast. Separate policy, actor, deployment and venue.
3. Register formulas, units, valid price domain, rounding, fees and the
   opportunity/risk-set denominator. Trace scale to orders, exposure, utilized
   capital and outcome. A no-trade decision may be rational.
4. Prove the smallest production path with independent arithmetic, adversarial
   fixtures, event joins and fail-closed evidence. Do not start an economic
   batch because one synthetic happy-path test passes.
5. Prospectively lock source, effective config, protocol, endpoint, contrasts,
   seeds/conditions, horizon, resources, stop rule, missingness, censoring and
   confirmation policy. Get bounded independent mechanics/evidence and
   causal/statistical reviews when the claim warrants them.
6. Run only the owner-authorized finite development budget from a clean,
   pinned candidate into new immutable output namespaces. Use fresh-process
   determinism/evidence-neutrality checks where required.
7. Independently reconstruct all assigned cells. Separate process completion,
   evidence validity, opportunity presence, actor activity, registered
   activation, economic verdict and empirical comparison.
8. Preserve valid losses, nulls, absent opportunities and falsifications.
   Do not choose a replacement seed, longer horizon or easier market after
   seeing the result. Confirmation on uninspected conditions requires
   separate authorization.
9. Publish one technical report and typed result with exact evidence links,
   uncertainty, limitations, resource usage and unrun stages. Synthesize only
   comparable source/population/endpoint contracts.

The [formula/evidence reference](../.agents/skills/market-ecology-research/references/evidence-and-formulas.md),
[timing reference](../.agents/skills/market-ecology-research/references/latency-and-clocks.md),
[capacity/game reference](../.agents/skills/market-ecology-research/references/capacity-and-games.md)
and [statistics/record reference](../.agents/skills/market-ecology-research/references/statistics-and-reporting.md)
are loaded as applicable, not pasted into every protocol.

An analyzer defect can justify a versioned rescore of sufficient immutable
raw evidence. A simulator-semantic defect that could have changed orders,
balances, risk or actor decisions requires a successor trajectory for the
affected study; an old trajectory cannot be repaired offline. Preserve the
original failed or invalid attempt.

## 6. Current idea map and suggested attacks

The [registry](program/registry.json) gives live stage and authorization;
the following is dependency order, **not** a batch schedule.

| Idea | Current position | High-value discriminating question |
|---|---|---|
| [ME-002-B](program/ideas/ME-002-B/idea.md) | PLAN; recommended next, not authorized | Did the one-second decision gate and phase hide a processing-delay effect? Separate cadence/phase from transport without claiming a general threshold. |
| [ME-004](program/ideas/ME-004/idea.md) | INTAKE | What changes when venue allocation is FIFO versus pro-rata with policy/resources fixed? Test ties, rounding and account fragmentation. |
| [ME-006](program/ideas/ME-006/idea.md) | INTAKE | Are informed maker quotes and executable arb trades substitutes or complements for synchronization and liquidity? Needs a genuinely exercised route opportunity set first. |
| [ME-007](program/ideas/ME-007/idea.md) | PLAN | Can a three-asset conversion be executed and closed after charged-asset fees, depth, precision and residual risk? Mechanics first; endogenous incidence later. |
| [ME-008](program/ideas/ME-008/idea.md) | INTAKE | Does assigned capital actually change orders, exposure and payoff, and how does crowding alter the admissible region? |
| [ME-009](program/ideas/ME-009/idea.md) | INTAKE | Which tested roles substitute or complement in a small resource-normalized ensemble? |
| [ME-010](program/ideas/ME-010/idea.md) | INTAKE umbrella | Separate named perp carry, dated carry/roll or option child studies only after financing/lifecycle readiness; do not rescue historical P4/P5/P6 by relabeling them. |
| [ME-011](program/ideas/ME-011/idea.md) | INTAKE | On reliable population-conditioned payoffs, what are the restricted responses, regret bounds and invasion relations? |
| [ME-012](program/ideas/ME-012/idea.md) | INTAKE | What explicit entry/exit/reinvestment rule produces stable shares, metastability or cycles, distinct from imposed calendar periodicity? |

Other falsification axes may become separate studies: capital redistribution
versus new capital; actor-count fragmentation; replacement versus removal;
same policy under swapped deployments; latency phase and transport ordering;
matching allocation versus IOC/FOK/GTC instruction; fee and financing
precision; adverse selection and inventory withdrawal; cross-currency
collateral; settlement-pending risk; or a mechanism under a different but
prospectively specified counterparty mix. Change one axis initially. Do not
silently alter defaults or combine every derivative and venue into a first
screen.

For economic capital capacity, define a tested admissible set conditional on
an ecology, objective, risk and exit rule. Report the entire grid, including
loss, no-trade, forced exit and unpriceable cells. Passing the largest tested
scale does not locate the upper boundary; no passing point is not universal
zero capacity. Host runtime/RAM/disk capacity, executable flow capacity and
risk-adjusted capital capacity are different quantities.

## 7. Game theory and publication path

The staged path is:

    population-conditioned objective/payoff surfaces
      -> restricted policy/resource best responses
      -> rare-policy invasion tests and response graph
      -> explicit allocation, entry, exit or learning mechanism
      -> long-run coexistence, concentration, instability or cycles

Market payoffs need not be zero-sum. Pairwise rankings do not determine an
arbitrary multi-policy game. Non-transitive invasion does not by itself
produce capital cycles. Restricted low regret is not global Nash. Stable
market-statistic distributions, stable wealth shares after perturbation,
strategic equilibrium, metastability and scheduled calendar effects must be
reported separately. Adaptation or replicator-like rules bring new
assumptions and balance-sheet sources/sinks; they are not implicit in today's
fixed-population worlds.

Useful primary reading already curated in the
[readiness packet](NEXT-STUDY-READINESS-PACKET.md#13-primary-literature-and-empirical-reference-plan):
[Farmer on market force and ecology](https://doi.org/10.1093/icc/11.5.895),
[Scholl, Calinescu and Farmer on ecology and malfunction](https://doi.org/10.1073/pnas.2015574118),
[Almgren and Chriss on execution cost/risk](https://doi.org/10.21314/JOR.2001.041),
[Bouchaud, Farmer and Lillo on order flow and liquidity](https://doi.org/10.1016/B978-012374258-2.50006-3),
[ABIDES on message-level simulation](https://doi.org/10.1145/3384441.3395986),
[the empirical game-theory survey](https://doi.org/10.1613/jair.1.16146),
and [alpha-Rank](https://doi.org/10.1038/s41598-019-45619-9).
These motivate design questions; they do not certify this simulator.

A paper could eventually report conditional execution capacity, a ranking
reversal, substitutability, or a well-supported limitation. It need not
claim stable capital cycles or reproduce the whole market. Stronger
emergence language requires robust replication across declared conditions,
the predicted causal ablation/restoration, and a compatible empirical
comparison. Three development seeds are a screen, not rare-tail evidence.

## 8. Existing tooling and what to build only when needed

Go 1.27.0 is the current project toolchain. Core exchange/simulation code
lives in [exchange](../exchange), [simulation](../simulation) and
[simulations](../simulations); Go reconstruction lives in
[analysis](../analysis) and [experiment](../experiment).
[evstream](../evstream) supplies canonical binary evidence with ordering and
corruption tests. Existing command adapters include
[multivenue](../cmd/multivenue),
[evsrender](../cmd/evsrender),
[mvanalyze](../cmd/mvanalyze),
[ME-005 analysis](../cmd/me005analyze),
and ME-001/002/003 study-specific runners and aggregators under
[cmd](../cmd). Scripts should parse arguments, call reusable Go library
functions and serialize results; avoid embedding all analytics in one-off
command main functions.

Use ripgrep for source search, Go tests and test-race for focused contracts,
go vet for static checks, and Go pprof/benchmarks **after** locating a real
performance bottleneck. The ordinary [Makefile](../Makefile) test target
runs Go tests and synthetic contract/archive fixtures; inspect targets before
execution. Campaign, activation, capacity, dev-607 and holdout adapters are
not ordinary smoke tests. The local research-skill validator is:

    go run ./.agents/skills/market-ecology-research/scripts/validate.go --root .

Python in the repository .venv is for visualization, not data-intensive
processing. Use uv to manage that environment; matplotlib/seaborn are already
part of the local visualization setup. Plots should come from verified,
machine-readable Go results. Good examples are opportunity funnels,
paired response maps, quantity versus completion/cost, per-attempt gross
edge versus final net result, residual inventory/closeout and timing
distributions. Do not replace source-bound evidence with a plot.

Before creating analytics, look for reusable existing paths. The most
valuable missing *study-specific* tools are often a small event-ordered
opportunity/risk-set replay, a public-to-delivered-to-action join, an
independent fee/ledger/terminal-closeout reconstruction, an all-assigned
cell aggregator with explicit missing/censored states, or a treatment/control
comparability check. Add each only for a declared endpoint and test it with
wrong identity, missing event, duplicate fill, misordering, fee-unit,
overflow, signed-price-domain and incomplete-output mutations. Avoid a new
dashboard, database, storage migration or generalized workflow engine
without a concrete need. The binary-evidence format is already in the
scientific development tree; historical JSON hashes retain their identity.

## 9. Branch collaboration and permissions

Start a new direction from the current remote main in its own branch/worktree.
Do not edit another agent's active checkout. Existing historical and
performance branches are independent evidence/idea sources, not wholesale
merge targets. Keep source, tests, config/protocol, reports and optimizations
attributable in small conventional commits. Preserve raw evidence and old
invalid results. Push only intended branches; reconcile main normally before
publication, never force-push or reset someone else's work.

The repository [AGENTS.md](../AGENTS.md) asks for a library/framework-first
design: extension through external configuration, composition, interfaces or
callbacks, not a hard-coded registry or enum that users must edit. It also
requires descriptive Go names and make test before code commits. It mentions
anti-ai-alop and tmux skills; they were not available in the inspected local
skill locations, so do not claim to have loaded them unless your own
environment actually supplies them.

Explicitly invoke the repo
[market-ecology skill](../.agents/skills/market-ecology-research/SKILL.md)
with the needed mode. Its version should be recorded at protocol lock. For
separately authorized long-horizon hypothesis search, the optional
[conduct-creative-research skill](../.agents/skills/conduct-creative-research/SKILL.md)
separates proposal, adversarial critique, grading and reproduction. It is
not permission to run every backlog item. Recent bounded independent
reviews used fresh Sol-6 medium contexts; respect the current owner's
review-model restrictions, never self-approve, and report service failure as
UNAVAILABLE / NOT_ISSUED rather than a scientific verdict.

Suggested first prompt for a collaborator:

    $market-ecology-research
    Mode: PLAN. Study: <one ID or a new ID>.
    Start from current main, registry and relevant historical reports.
    No simulations. Return one falsifiable question, competing explanations,
    minimal fixtures, opportunity denominator, estimand, finite matrix,
    resource estimate, review gates and prospective stop rule.

An eventual RUN prompt must resolve the placeholders to an exact owner-approved
protocol, candidate, worlds, seeds/conditions, horizon, CPU/RAM/disk bounds,
output namespace and stop rules. A registry entry or planning branch is not
authorization. Keep review responses and operational status separate from a
pinned executable candidate; source/config/protocol changes require an
explicit successor gate.

## 10. What would count as progress

Progress is a reproducible conditional answer, including a valid null or
identification limit. Examples: a maker class changes an executor's
opportunity set; a deployment swap changes arrival but not outcome because
of a declared decision gate; a finite arb desk takes residual risk even
when matched legs show a gross edge; or a proposed mechanism has no relevant
opportunity in the registered ecology. Each answer should narrow the next
question.

Do not confuse green tests with realism, static arbitrage fixtures with
emergent alpha, replicated simulated seeds with empirical validation,
software ledger transfers with trading PnL, or an implemented policy with an
economically optimizing actor. After months of collaboration, the durable
asset is the versioned chain of questions, negative findings, independent
reconstructions and exact limitations—not a collection of positive plots.
