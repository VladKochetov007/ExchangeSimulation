# V2-1c — one remote public-feed maker smoke

## Status

Implemented and evidence-gated. This is one target maker, one remote source,
one symbol, and one fixed weight. It validates a participant-local
cross-venue information path. It does not test price discovery, convergence,
or any economic benefit of remote information.

## Mechanism

An ABC/USD maker on `north` keeps its own delayed local-book cache and receives
a separate public snapshot session from `south`. The source session is a
`FeedOnlyGateway`: subscribe/unsubscribe are permitted; any order request
panics in its unit test. It has no participant row, mandate, inventory,
capital, or order entry. The target computes

```text
(1 - remote_weight) * local_mid + remote_weight * remote_mid
```

only after both copied, two-sided cache views exist. It never falls back to
the historical consensus index. Information delay is separate from the target
maker's request/response delay.

The initial configuration is intentionally narrow:

| field | value |
|---|---|
| target/source | `north` / `south` |
| symbol | `ABC/USD` |
| local `spot_maker` delay | constant 10 ms |
| remote feed delay | constant 20 ms |
| remote weight | 0.5 |
| evidence roles | `spot_maker`, `v2_remote_feed` |

The config rejects an instrumented remote feed unless scalar V2-0 receipts,
V2-1b vectors, both role declarations, an explicit local `own_mid` cache, and
a nonzero remote delay are present. An evidence-off form exists only for the
fresh-process neutrality test. It is never a retained scientific run.

## Preregistered smoke checks

| check | prediction | result |
|---|---|---|
| target cache activation | target remote cache contains only `south` ABC/USD snapshots | PASS |
| shared-value exclusion | target maker is not index anchored | PASS |
| feed account isolation | remote feed session cannot submit an order | PASS |
| scalar evidence | V2-0 receipts and decisions audit valid/nonempty | PASS |
| vector evidence | every target scalar order has one vector with exactly local and remote components | PASS |
| component future/drop mutations | independent audit rejects altered prefix or missing component | CAUGHT |
| dropped decision-vector mutation | independent audit rejects a removed vector after count/digest rewrite | CAUGHT |
| fresh-process determinism | same remote world exact across GOMAXPROCS 1/4 | PASS |
| instrumentation neutrality | evidence OFF/ON has same execution hash across fresh processes | PASS |
| persisted evidence determinism | ON scalar and vector counts/digests match across GOMAXPROCS 1/4 | PASS |

The live smoke is 20 simulated seconds, seed 101. Fresh-process controls use
two simulated minutes. They are construction/provenance tests, not a seed
replication claim.

Both compact artifacts can be independently invoked with `mvanalyze` without
raw JSON logs or `greeks.json`; V-034 fixed an incorrect generic-loader
prerequisite discovered by this smoke.

## Profiling result

A bounded two-minute GOMAXPROCS=1 remote smoke retained 21,644 schedules,
21,591 receipts, and 378 scalar decisions. The vector extension contains 103
decision rows (7,416 bytes) and 206 component rows (11,536 bytes): 18,952
bytes, versus about 3.84 MiB for the scalar schedule/receipt/decision files.

That run's sampled allocation profile remains dominated by preview-book
allocation (`book.NewBook`, about 55% of allocation space). The evidence-on
profile includes a post-run audit and must not be used as an ON/OFF timing
estimate; it is only a structural cost check. Existing command-level V2-0
profiles remain the valid timing comparison. Therefore `goccy/go-json` stays
deferred: V3 is not a serialization hotspot, and swapping evidence JSON needs
a byte-corpus/digest/mutation campaign first.

## Interpretation and next gate

Supported narrow claim: one maker can consume a delayed remote public feed,
form a composite from two actor-owned copied views, and leave independently
auditable evidence that both delivered frontiers preceded every remote-based
order.

Unsupported claims: heterogeneous informed makers, realistic price discovery,
quote-mediated synchronization, explicit arbitrage, or a stylized fact. Next
mechanism is a small fixed roster with distinct source membership, weights,
and horizons, followed by activation tests before the V2-1/V2-2 2x2.
