# ME-002 — prospective network × actor-processing development protocol (draft)

Status: **DRAFT, NOT LOCKED, NO ME-002 WORLD RUN**. The owner’s 2026-09-23
continuation authorizes a bounded ME-002 development study, but this draft is
not an execution plan until the deployment implementation, independent replay,
tests, exact candidate and prospective review are complete. No confirmation
partition or historically reserved holdout is involved.

## Question, mechanism and permitted claim

Holding one immediate ABC/USD buy policy, actor endowments, C0=4 maker/8
random-taker ecology, venue rules, fees, 1 ms policy polling and 1-second
earliest decision fixed, how do directed network delivery and actor-side
processing delay change the filled fraction of a finite target? The strongest
possible result is a three-seed simulation-internal **causal development
response map for these synthetic deployment assignments**. The policy does
not optimize its latency; network and processing belong to deployment, not to
its economic objective. No city, empirical connectivity or general latency
irrelevance claim is licensed.

The local mechanism predicts that longer realized action delay can miss
short-lived displayed-depth opportunities, especially at 5 ABC. Competing
explanations include periodic snapshot sampling, 1 ms clock phase, unchanged
arrivals despite different nominal delays, endogenous book divergence after
the focal order, and terminal marking of unfilled quantity. A null with
separated realized paths is a finite-grid null, not proof of equivalence. A
nominal contrast without separated realized paths is an identification limit.

## Why these delays and this ecology

The predecessor [ME-001 report](../ME-001/report.md) is a completed development
screen, not a confirmation set. A new Go-only profile of its retained C0
pre-decision evidence is being versioned separately; no ME-002 outcome is used
to choose this design. The source clock steps and focal polls every 1 ms,
the baseline focal feed/request/response delay is 1 ms each, and the venue’s
default periodic snapshot interval is 100 ms. In the three retained C0 seeds,
the profile found 22 pre-decision focal snapshots each, 1 ms publication-to-
receipt lag, 100 ms median *sampled* best-touch lifetime and complete 5-ABC
displayed-depth episodes of approximately 73–200 ms. These episodes are
snapshot-observed proxies: the evidence does not establish continuous
between-snapshot quote lifetime. Some episodes are right-censored.

Prospectively choose fast network = 1 ms and slow network = 90 ms on each of
the focal market-data, request and response paths. Fast processing = 0 ms;
slow actor-side processing = 120 ms from inbox receipt until that snapshot
is eligible for the unchanged 1 ms policy poll. These are synthetic logical
delays selected around the observed sampling/episode timescales, not measured
geographical links or machine CPU profiles. Newer snapshots arriving during a
fixed processing wait remain queued in receipt order; the actor processes
each due snapshot at its next policy tick. One-sided snapshots retain the
existing last usable two-sided quote policy, but must remain visible in
evidence. The focal client remains ID 13 in every arm. All background actors,
capital, seeds, fee assignments and their own transport remain unchanged.

| Arm | Feed/request/response network | Actor processing | Nominal feed+processing+order path, excluding poll wait |
|---|---:|---:|---:|
| F/F | 1/1/1 ms | 0 ms | 2 ms |
| F/S | 1/1/1 ms | 120 ms | 122 ms |
| S/F | 90/90/90 ms | 0 ms | 180 ms |
| S/S | 90/90/90 ms | 120 ms | 300 ms |

Response delay is recorded separately and is not part of the pre-execution
effective-action-latency formula. `effective_action_latency` is the actual
selected public-message publication-to-order-arrival interval, decomposed
into inbound delivery, actor processing, poll wait and outbound delivery.
Report it divided by a clearly labelled *observed snapshot opportunity
duration* only when such a duration is complete and measurable. The configured
processing-delay/1-ms decision-period ratio is 0 or 120; also report realized
values. Do not imply that these components sum to the same number when the
actor used a different, older selected snapshot.

## Fixed matrix, seeds, outcomes and contrasts

One ABC/USD price-time venue, one immediate buy parent, C0 background,
0.5 and 5 ABC targets, and development seeds `12001`, `12011`, `12017`:
**4 deployments × 2 targets × 3 seeds = 24 economic worlds**. The seeds
were selected before any ME-002 outcome by taking these three new ME-002
development labels after a tracked-registry/config search found no matching
reserved holdout label; that check is not a claim about all private machines.
Two fresh-process F/F technical duplicates, with `GOMAXPROCS=1` and `7`,
raise the maximum to **26 executions**. The equal/equal control and a
two-identical-actor deployment-to-ID swap are *synthetic fixtures*, not
additional economic worlds. No decision-cadence variation is included.

Primary endpoint in every valid assigned world:

```text
Y = independently reconstructed filled ABC / assigned target ABC
```

For a valid no-send, admission rejection or accepted-unfilled world, `Y=0`;
partial fills retain their actual fraction. Process or evidence failure is
`UNASSESSABLE`, not zero. Report the full-completion indicator separately.
For each seed and target, publish F/F, F/S, S/F and S/S values plus:

```text
network effect    = ((S/F + S/S) - (F/F + F/S)) / 2
processing effect = ((F/S + S/S) - (F/F + S/F)) / 2
interaction       = S/S - S/F - F/S + F/F
```

These are finite paired-world contrasts, not event-row standard errors.
Publish each seed’s contrast, three-seed median and range; no asymptotic
p-value or equivalence declaration. If a matched quartet is invalid, show
all assigned statuses and leave that quartet’s contrast undefined. A new
source/evidence correction requires a new pinned candidate and rerunning
affected comparable cells; never selectively rerun a valid unfavorable seed.

Secondary outcomes: target shortfall (only if decision and terminal marks
are valid), completion time, fill count, quoted fees, rejected/accepted/
partial state, residual, latest delivered/processed quote age, and the
publication→receipt→processing→decision→send→arrival→fill/response funnel.
Lower marked shortfall with an unfilled obligation is not successful
execution. Markout is not causal impact. Signed-price ratios are restricted
to this positive spot book; unavailable marks are explicit missing values.

## Readiness and evidence gates

Before any world, bind exact network/processing fields in the effective
runtime contract; demonstrate that a zero-processing legacy world preserves
ME-001 economics; verify exact logical timestamps in four arm fixtures,
including a quote withdrawn before slow arrival, neighboring tick phase,
same-ID assignment and independent processing of multiple queued snapshots.
Confirm that both delayed responses and all due processing work drain before
the four-second terminal hook. No wall-clock sleep may represent model delay.

Use a new versioned evidence schema for the actor-processing event. The
analyzer must independently join the exchange publication and publisher
delivery status to focal receipt, processing completion and policy tick;
then request ID, venue admission, exchange-time fills, delayed responses,
cash/ABC ledger and terminal mark. A publication is not an executable
opportunity merely because its midpoint moved. The only prospective
opportunity proxy here is a sampled positive two-sided public snapshot with
displayed ask depth for the assigned target; it is not a continuous venue
state or guarantee of fill at later arrival. Report absent publication,
not-sent/dropped feed, received-but-unprocessed, processed-but-no-decision,
send, admission and fill separately. Preserve one-sided/stale observations.

Mutation fixtures must fail closed on omitted/duplicated/misordered processing
events, wrong snapshot sequence or receipt time, altered processing delay,
request/response identity, late/unanchored fill, fee, ledger and terminal
mark. Independent reconstruction may reuse ME-001 arithmetic only after
its timing assumptions are parameterized and verified; the accepted ME-001
trajectory, score and schema remain historical identities.

## Resources, ordering, review and stop

Build clean pinned Go 1.27.0 simulator/analyzer binaries from the exact
accepted protocol/source commit. First run F/F controls and require byte-
identical canonical evidence and reconstructed outcomes. Preflight their
wall/RSS/disk use before the economic matrix. Use at most one world process
at a time, `GOMAXPROCS≤7`, four simulated seconds per world, 30-second
per-world timeout, 15-minute total batch ceiling, 4 GiB peak process RSS
and 1 GiB retained evidence. Stop if the actual preflight exceeds any cap.
Run target 0.5 then 5, and within each target F/F, F/S, S/F, S/S in ascending
seed order. Stop a matched seed block for infrastructure/evidence failure;
valid negative outcomes do not trigger retuning. Outputs are exclusive fresh
paths outside the source checkout; preserve incomplete attempts.

Obtain one prospective bounded independent Sol-6 medium design/evidence
review before execution. After raw replay and all-assigned analysis, obtain
two fresh read-only Sol-6 medium reviews: (A) timing/execution/accounting/
evidence and (B) causal/statistical claim scope. Service unavailability is
`UNAVAILABLE / NOT_ISSUED`, never an acceptance. No ME-001 confirmation,
ME-002B execution, city topology, ME-003 or historical holdout use follows
from this protocol. No new economic world starts after 06:15 UTC on
2026-09-24; all bounded work stops by 07:00 UTC.
