# V2 simulator performance study

Target metric: **simulated market seconds per wall-clock second on the
integrated V2 workload, subject to exact trajectory equivalence.**

A faster run with a different trajectory is not an optimization. Every change
below is held to an identical ordered execution stream hash, an identical
evidence artifact digest, and a byte-identical evidence tree.

## 1. Provenance

| Field | Value |
| --- | --- |
| Scientific HEAD this work is based on | `887899ff05f10dc6fd43d8cd8e88d52d5817c3b3` |
| Performance branch | `autoresearch/v2-performance-research` |
| Go toolchain | `go1.26.7-X:nodwarf5 linux/amd64` |
| Host | 16 CPUs, 62 GB RAM, Linux 7.1.10 |
| C++ | GCC 16.2.1, AVX2 and AVX-512 available |
| External Go dependencies | none, before and after |

Registered holdouts were not read, run, or benchmarked against. Benchmark runs
use the development config `research/configs/v2-integrated-longrun/dev-607.json`
with the non-scientific benchmark seed **900101**, writing only to throwaway
directories outside the repository.

## 2. Benchmark protocol

15 simulated minutes (900 simulated seconds), `GOMAXPROCS=1`, pinned to one core
with `taskset`, five or six alternating A/B repetitions, medians reported with
ranges.
Pinning makes these measurements contention-immune, which matters because up to
three research threads shared this host and earlier unpinned batches had to be
discarded.

### Determinism oracle

Every simulator change must reproduce all four of these:

| Oracle | Value |
| --- | --- |
| Ordered execution stream hash | `51541f91db7c5eae8688235d3961a76af421ab782f05ab62649076cf90aef332` |
| Execution events | 1,163,127 |
| Evidence artifact digest | `7a869b49546a60cba0f5a31f7cbc8236f331d45e73404248096ebf05812739f0` |
| Persisted records | 1,185,184 |
| Evidence tree | 27 files, 442,225,951 bytes, `diff -rq` clean |

The hash matches what `perf/thread-sim` obtained across all 30 of its baseline
runs, spanning both log modes, two filesystems, `GOMAXPROCS` 1 and 4, and
profiled and unprofiled builds.

## 3. Where simulation time goes

Measured after S1 and S2 were accepted and before S3-S6, which is the state the
§4 bounds and the §7 candidate list are computed from. Full evidence, 14.70 s of
CPU samples for 900 simulated seconds, under a profiling build. Percentages are
cumulative and overlap; they are attribution aids, not a partition.

| Subsystem | cum CPU | share |
| --- | ---: | ---: |
| `DrainIngress` (the whole request-processing tree) | 7.07 s | 48.1% |
| `PlaceOrder` | 5.61 s | 38.2% |
| `venueLogger.LogEvent` (both evidence sinks) | 3.95 s | 26.9% |
| `encoding/json.Marshal` (all callers) | 3.21 s | 21.8% |
| `processExecutions` | 2.19 s | 14.9% |
| `checkpointSink.observe` (ordered execution hash) | 1.88 s | 12.8% |
| `settleExecution` | 1.21 s | 8.2% |
| `PositionManager.GetPositionBySide` | 0.97 s | 6.6% |
| `buildAccountMarginProfile` | 0.92 s | 6.3% |
| `CheckLiquidations` | 0.86 s | 5.9% |
| `EventScheduler.ProcessUntil` | 0.82 s | 5.6% |
| `MDPublisher.Publish` | 0.71 s | 4.8% |
| `previewMatchExcluding` (detached clone) | 0.66 s | 4.5% |
| `Matcher.Match` | — | absent from the top 400 |

Cross-cutting totals, summed over every contributing runtime symbol:

| Cross-cutting cost | share |
| --- | ---: |
| Allocator and GC | 13.73% |
| Map operations (hash, probe, iterate) | 11.03% |
| SHA-256 (checkpoint hash) | 5.78% |

**Matching is not a cost centre.** `Matcher.Match` does not reach the top 400
symbols. The 38% attributed to `PlaceOrder` is admission bookkeeping, settlement
accounting, and evidence emission. Any plan that begins by replacing the
matching engine or the order book is optimizing the wrong thing on this
workload; that conclusion is unchanged from the `perf/thread-sim` baseline and is
reconfirmed here after two accepted optimizations.

### Allocation

4,395 MB sampled for 900 simulated seconds. Top sources:

| Source | alloc space | share |
| --- | ---: | ---: |
| `encoding/json.Marshal` | 818.72 MB | 18.63% |
| `cloneBookForPreviewExcluding` | 614.07 MB | 13.97% |
| `DelayedGateway.schedulePhaseMarketData` (+ closure) | 644.81 MB | 14.67% |
| `DelayedGateway.DrainDeterministicPhaseEgress` | 368.52 MB | 8.38% |
| `NewClientGateway` | 237.03 MB | 5.39% |
| `NewDelayedGateway` | 231.54 MB | 5.27% |
| `book.(*Book).AddOrder` | 226.52 MB | 5.15% |

The `simulation.DelayedGateway` paths together account for about **1.24 GB, 28%
of all allocation** — more than `json.Marshal`. Market-data delivery, not
serialization, is the largest allocation source, and it feeds the 13.73% spent
in the allocator and GC.

## 4. Amdahl bounds — what any serialization change can be worth

This settles the recurring question of whether to replace JSON with a custom
binary format, or to move serialization into Go assembly, cgo, or a C++ module.

| Block made *free* | Ceiling | sim-s per wall-s |
| --- | ---: | ---: |
| all `json.Marshal` | **1.28x** | 61.2 to 78.2 |
| `json.Marshal` + SHA-256 | 1.38x | 61.2 to 84.6 |
| all map operations | 1.12x | |
| allocator + GC | 1.16x | |
| all four (blocks overlap; optimistic) | <=2.10x | |

**No serialization change can exceed 1.28x end to end**, however fast the
replacement is. A custom binary evidence format would additionally change the
persisted evidence contract and the execution-hash domain — the two artifacts
the r5 gate seals — so it buys at most 1.28x for the highest scientific risk
available. Rejected as a headline optimization.

FFI cost is *not* the obstacle, contrary to the usual objection. The run
persists 1,185,184 records in 14.70 s, or 80,624 events per second. At roughly
60 ns per cgo crossing that is 4.8 ms, 0.03% of runtime — negligible even at
per-event granularity. The obstacle is purely that the work behind such a
boundary is 21.8% of the program.

Getting materially past ~1.4x therefore requires attacking the other 78%:
allocation (28% of which is one subsystem), map lookups, and the accounting
path. That is Go-side algorithmic and data-structure work. A full native rewrite
is the only thing that captures the remainder, and it should follow, not
precede, a correct Go reference.

## 5. Accepted optimizations

Every change below reproduces all four oracles. Equivalence was additionally
confirmed on **three seeds and both log modes**:

| Cell | Evidence tree | Execution hash |
| --- | --- | --- |
| seed 900101, 15 min, `full` | 27 files / 442,225,951 B identical | `51541f91db7c5eae…` |
| seed 900102, 10 min, `full` | 27 files / 272,736,362 B identical | `657758387ac53103…` |
| seed 900103, 10 min, `full` | 27 files / 279,757,752 B identical | `a64a00b4dbe459db…` |
| seed 900101, 10 min, `none` | 4 files / 1,876,201 B identical | `a208e4e4986885c6…` |

### S1 — Cache the deterministic gateway iteration order (`3a32be5`)

Both deterministic drains rebuilt a sorted client-ID slice *and* a parallel
client-to-gateway map on every call, and the ingress drain does so once per pass
while its passes repeat until one is empty. That was 1.51 s of 23.36 s CPU and
511.4 MB of allocation, of which 391.85 MB was the redundant lookup map.

One sorted slice of client/gateway pairs replaces both, invalidated by a
generation counter bumped at the two mutation sites, which already hold `e.mu`.

| Measure | `full` | `none` |
| --- | ---: | ---: |
| CPU | -4.9% | **-16.0%** |
| Peak RSS | -2.3% | -1.6% |

### S2 — High-volume evidence payloads as ordered structs (`eaf6c85`)

Payloads are marshalled **twice** — once by the hash sink, once by the logger —
so a `map[string]any` payload pays for its map and its interface boxes on both.
`mapEncoder` was 2.85 s of CPU and 16.2 M allocated objects (28.2% of all
objects).

`OrderFill` (149,594 events per 15-minute run), `OrderCancelled` (111,393) and
`BookSnapshot` (70,581) became structs whose fields are declared in
lexicographic order of their JSON names — the order `encoding/json` emits for a
map — so the persisted bytes are unchanged.

**-9.9% wall, -10.0% CPU.**

Three tests asserted the payload's Go type rather than its evidence, and two
skipped silently when the assertion failed, which is how a byte-identical change
produced a measured PnL of zero instead of an error. They now read the field
from the payload's persisted JSON and fail loudly on a missing field.

### S3 — Persist evidence without marshalling each payload twice (`087dd7a`)

`JSONLinesLogger.LogEncodedEvent` assembles a record from segments of already
encoded JSON, appending integers with `strconv` into one reused buffer. The
venue envelope prefix is built once per venue with `encoding/json`, and event
names are cached the same way, so no escaping rule is reimplemented by hand.
Every path that cannot supply bytes falls back to the reflective encoder, so the
fast path is opt-in per record.

A differential test drives the real logger and requires byte-identical output
*and* an identical evidence digest against `LogEvent` across the cross product
of 5 venue identifiers, 5 event names, 3 client IDs, 5 timestamps and 18 payload
shapes — HTML escaping, U+2028/U+2029, non-ASCII, embedded quotes and
backslashes, control characters, int64/uint64 boundaries, floats, nested
structs, arrays, scalars and nil — plus buffer reuse over 500 records,
write-failure propagation and short writes.

**-11.0% wall, -11.0% CPU.** The single largest change.

### S4 — Stop reslicing the deterministic phase queues from the front (`0ed5180`)

Both phase queues advanced with `s = s[1:]`, which abandons the consumed prefix:
the slice keeps only its tail capacity, so every refill reallocates and copies.
A small FIFO advances a head index over one reused backing array instead, zeroing
the popped slot so a delivered message is not kept alive.

| Measure | Change |
| --- | ---: |
| Allocated bytes | 3,793.98 MB to 3,167.14 MB (**-16.5%**) |
| Allocated objects | 36,716,252 to 30,880,881 (**-15.9%**) |
| Wall / CPU | -1.45% / -1.46% |
| Peak RSS | -1.3% |

The wall gain is small because GC was not the binding constraint at 13.7% of
CPU. The value is the allocation and resident-memory reduction, which is what
makes long runs predictable.

### S5 — Cache the canonical book and client iteration orders (`6725639`)

`buildAccountMarginProfile` rebuilt and re-sorted the whole book symbol list on
every call — it runs once per account per breach — and `CheckLiquidations` and
the cross-book sweep rebuilt and re-sorted the client list. 250 ms of a 920 ms
margin-profile build was the symbol sort alone, over an instrument set that is
static between listings and expiries.

Two cached accessors, each invalidated by a generation counter bumped at every
mutation site. Iteration order is unchanged.

**-2.7% wall, -2.6% CPU**, ranges non-overlapping (11.79-11.97 against
11.52-11.56).

The first version bumped the counter on book insertion but not on expiry
deletion, and the existing suite caught it as a nil dereference in the risk
sweep. Both accessors now also compare cached length against map length, which
is O(1) and converts a future missed bump into a rebuild rather than a stale
entry handed to code that dereferences it.

### S7 — Cache each symbol's market-data fan-out order (`26998ae`)

`Publish` rebuilt and re-sorted the subscriber client-ID list on every message —
250 ms of its 570 ms — over a subscription set that changes only on subscribe or
disconnect, and it re-resolved `p.gateways[symbol]` once per subscriber inside
the fan-out loop.

Fan-out order is an economic input: the subscriber handed a message first reacts
first, which is what a latency experiment measures. The cache preserves
ascending client-ID order exactly. The package previously had **no tests**; four
were added covering fan-out order, tracking of all three mutation paths,
per-symbol isolation, and recovery from a missed invalidation.

### S8 — Read the simulated clock without taking a lock (`5fb87e2`)

Reading the clock is the most frequent operation in the simulator, and each read
took an RWMutex read lock — two atomic read-modify-writes. Those atomics were
3.37% of CPU.

`current` is now an `atomic.Int64`. Readers load it; writers still hold `mu`,
because advancing the clock is a compare-and-set against `goal` that must stay
atomic between writers. A reader observes exactly the set of values it observed
under the read lock. `go test -race` passes.

S7 and S8 together: **-3.7% wall, -3.6% CPU**, ranges non-overlapping
(11.59-11.87 against 11.28-11.38).

### S6 — Resolve each client's position map once per lookup (`5d1f0d3`)

Three `PositionManager` methods indexed `pm.positions[clientID]` twice. Strictly
less work and provably identical, but **-0.34% wall with overlapping ranges**:
reported as no measurable gain. The string-keyed `positionKey` hash is the real
cost here, and replacing it with an integer instrument handle is a wider change.

### S10 — Reuse the checkpoint hasher and its scratch buffers (`b5ebf04`)

The hash sink allocated per observed event: a fresh SHA-256 state, the slice
`Sum(nil)` returns, and two string-to-`[]byte` conversions. The hasher is now
created once and `Reset` per event, `Sum` writes into a retained slice, and the
two identity writes are concatenated into one reusable buffer — exactly
equivalent, since `Reset` restores the state `New` returns and SHA-256 hashes a
stream.

Allocated bytes -1.4%, objects -1.2%, peak RSS -1.2%. **Wall is not
distinguishable from noise** (ranges overlap), so this is accepted for the
memory reduction rather than as a throughput claim. The saving is smaller than
four removed allocations suggests because escape analysis was already
stack-allocating part of the hash state.

### S9 — Index resting orders by owner (`c9c9cbf`)

Order admission answers three questions that each concern one client's own
resting orders — position exposure, hedge-reduce headroom, and self-crossing
quotes — and each scanned the entire book side to find them: O(book) work per
placement to examine a handful of orders. Two of the three were 3.48% and 1.66%
of CPU.

`Book` now indexes resting orders by owner, maintained at the single insertion
and single removal site. The call sites narrow their candidate set and keep
their ownership filters unchanged, so both paths select identically. Detached
preview books deliberately carry no index — a preview is built and discarded
without being queried, and indexing it would add allocation to the largest
allocation site in the simulator to serve nobody.

**-2.0% wall, -2.1% CPU.** Predicted ~5%; the gap is that these functions also
do non-iteration work. Peak RSS rose 3.2% in that batch, and removing the map
capacity hints did not change it, so it is GC pacing rather than the index's own
footprint; against the pristine base peak RSS is level.

### Combined result

Pristine base against S1-S10, `log_mode=full`, `GOMAXPROCS=1` pinned to one
core, six alternating repetitions:

| Measure | base | optimized | change |
| --- | ---: | ---: | ---: |
| Wall | 18.015 s | **11.24 s** | **-37.6%** |
| CPU | 17.85 s | 11.135 s | -37.6% |
| Peak RSS | 763,810 KiB | 765,232 KiB | +0.2% |
| Allocated bytes | 4,395 MB | 3,242 MB | -26.2% |

> **Target metric at `GOMAXPROCS=1`: 50.0 to 80.1 simulated seconds per
> wall-clock second, a 1.60x speedup**, with all four determinism oracles
> identical on three seeds and both log modes.

Batch-to-batch medians move by roughly 1-2% on this host even under the
benchmark lock, so the figures quoted per change and the cumulative figure agree
to about that tolerance rather than exactly.

### The simulator is GOMAXPROCS-invariant, and that is worth another 14%

The benchmark protocol pins `GOMAXPROCS=1` because it makes measurements
contention-immune. It is not the fastest setting. Measured on the final binary:

| `GOMAXPROCS` | wall | CPU | peak RSS | execution hash |
| ---: | ---: | ---: | ---: | --- |
| 1 | 11.84 s | 11.77 s | 760,588 KiB | `51541f91db7c5eae…` |
| 2 | 10.29 s | 11.19 s | 634,036 KiB | `51541f91db7c5eae…` |
| 4 | 10.22 s | 11.49 s | 624,176 KiB | `51541f91db7c5eae…` |
| **8** | **10.14 s** | 11.41 s | 624,008 KiB | `51541f91db7c5eae…` |

Most of the gain arrives at 2 and the curve is flat from 4 onward, so 4 is the
sensible setting: **-13.7% wall and -17.9% peak RSS** against 1. CPU time falls
slightly too, because GC assist work overlaps instead of serializing onto the one
runnable core — which is also why the heap stays smaller.

Determinism was verified, not assumed: twelve runs, four each at `GOMAXPROCS` 2,
4 and 8, produced evidence trees **byte-identical** to the `GOMAXPROCS=1`
reference — all 27 files and 442,225,951 bytes, `diff -rq` clean — and the
execution hash is unchanged at every setting in every sweep since. Ordering comes
from the deterministic phase barrier, not from goroutine timing.

Adopting it is the scientific owner's call, because the r5 protocol may register
`GOMAXPROCS`. If it is adopted:

> **Target metric at `GOMAXPROCS=4`: 50.0 to 88.1 simulated seconds per
> wall-clock second, a 1.76x total speedup, with peak RSS down 18%.**

## 5b. Long-horizon behaviour

Every figure above is a 15-minute cell. The registered run is 24 hours, and a
cost that is superlinear in run length would be invisible at 15 minutes and
dominant at r5 scale. It was worth checking rather than assuming.

### Throughput is flat in run length

| Horizon | wall | peak RSS | sim-s per wall-s | execution events |
| --- | ---: | ---: | ---: | ---: |
| 15 min | 12.15 s | 763,464 KiB | 74.0 | 1,163,127 |
| 30 min | 24.01 s | 821,168 KiB | 74.9 | 2,317,837 |
| 60 min | 46.55 s | 904,304 KiB | 77.3 | 4,522,662 |
| 120 min | 90.42 s | 1,028,012 KiB | 79.6 | 8,802,887 |

Throughput does not decay; it drifts slightly upward as fixed setup cost
amortizes. Event count grows sub-linearly with horizon, so per-event work is
constant. **There is no superlinear cost in this simulator**, which is the
result that matters for a 24-hour cell.

### The growing peak RSS is not a leak

Peak RSS rises 763 MB to 1,028 MB across those runs, which looks like a leak
and is not one. Retained heap measured after the run is **553 MB at 15 minutes
and 559 MB at 60 minutes** — flat. Live heap is dominated by fixed setup:
`NewClientGateway` at 237 MB and `NewDelayedGateway` at 225 MB, together 84% of
it, allocated once.

Peak RSS is an extreme-value statistic over transient GC headroom, so a longer
run simply samples that maximum more often. It is bounded by pacing, not
unbounded growth.

### GC pacing is a real throughput knob, and tuning it is a trap above one core

`GOGC` and `GOMAXPROCS` are runtime knobs with no semantic effect. Swept jointly
on a 60-minute horizon; **the execution hash is identical in all eight
combinations** (`f961a66193488e5a`):

| `GOMAXPROCS` | `GOGC` | wall | peak RSS | sim-s per wall-s |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 100 | 44.28 s | 911,440 KiB | 81.3 |
| 1 | 200 | 42.12 s | 1,515,248 KiB | 85.4 |
| 1 | 400 | 40.42 s | 2,689,520 KiB | 89.0 |
| 1 | 800 | 40.61 s | 3,976,800 KiB | 88.6 |
| **4** | **100** | **39.12 s** | **753,464 KiB** | **92.0** |
| 4 | 200 | 39.04 s | 1,291,208 KiB | 92.2 |
| 4 | 400 | 40.01 s | 2,360,932 KiB | 89.9 |
| 4 | 800 | 40.08 s | 3,339,724 KiB | 89.8 |

At one core, raising `GOGC` buys real throughput — +9.5% at `GOGC=400` — because
GC serializes onto the only runnable core, so trading memory for fewer
collections pays. **At four cores it buys nothing**: `GOGC=200` is +0.2%
throughput for +71% peak RSS, and 400 and 800 are slower than the default.
GC work already overlaps, so extra heap headroom only adds page-fault and
cache-footprint cost.

The recommended operating point is therefore **`GOMAXPROCS=4` with `GOGC` left
alone** — which is also the lowest peak RSS of any setting that reaches 90+
simulated seconds per wall second. `GOMEMLIMIT` behaves as the same trade from
the other side: 900 MiB costs 11% wall for 576 MB peak, 700 MiB costs 29% wall
for 381 MB, both with the hash unchanged. Useful for fitting a constrained host,
not for speed.

### End-to-end on a 60-minute horizon

| Configuration | wall | peak RSS | sim-s per wall-s |
| --- | ---: | ---: | ---: |
| base, `GOMAXPROCS=1` | 72.44 s | 899,284 KiB | 49.6 |
| base, `GOMAXPROCS=4` | 62.41 s | 749,952 KiB | 57.6 |
| optimized, `GOMAXPROCS=1` | 43.77 s | 914,248 KiB | 82.2 |
| **optimized, `GOMAXPROCS=4`** | **38.99 s** | **752,556 KiB** | **92.3** |

Code changes alone are **1.66x**; with `GOMAXPROCS=4` the total is **1.86x** at
19% lower peak RSS than the base's own single-core configuration.

For the registered 24-hour cell that is roughly 29 minutes of wall time reduced
to roughly 16.

## 6. Rejected optimizations

### R1 — Reuse the hash sink's encoding as a `json.RawMessage`

Predicted about 11% of CPU. **Measured +1.15% CPU — a regression**, ranges
non-overlapping. `encoding/json` emits a raw value through `compact()`, which
rescans and re-escapes it for about the cost of the marshal it replaced, so the
change traded a marshal for a compaction. Reverted. S3 is the form that works:
skip the nested encoder entirely.

### R2 — Capacity-hint the receipt slice in the phase-egress drain

`received := make([]T, 0)` grows through 371,586 allocations totalling 264.75 MB,
so sizing it from the exact queue-depth bound looked free. **Measured: allocated
objects rose from 30,880,881 to 34,454,337** with total bytes flat.

`make([]T, 0)` does not allocate — a zero-capacity slice points at `zerobase` —
whereas `make([]T, 0, n)` always does. The hint therefore converted a free slice
into a real allocation on every drain that delivers nothing. Reverted. Removing
this allocation properly needs a checked-out scratch buffer, whose concurrency
guard is not worth ~0.4% of wall time.

### R3 — Replace JSON evidence with a custom binary format

Bounded at 1.28x by §4, and it changes both the persisted evidence contract and
the execution-hash domain — the two artifacts the r5 gate seals. Worst
risk-per-gain available.

### R4 — Move serialization into cgo, C++, or Go assembly

Same 1.28x bound. FFI crossing cost is genuinely negligible at 80,624 events/s
(0.03% of runtime), so the objection is not FFI overhead — the addressable block
is simply too small to justify a toolchain dependency and a new failure surface.

### R5 — Optimize the matching engine or the order-book representation

`Matcher.Match` is absent from the top 400 CPU symbols.

### R7 — Go profile-guided optimization

**Rejected: PGO is a regression on this workload.**

A merged profile was collected from three representative cells — seeds 900101
and 900102 in `full`, and 900101 in `none`, so the profile is not tuned to one
cell — and placed as `cmd/multivenue/default.pgo`.

| Measure | non-PGO | PGO | change |
| --- | ---: | ---: | ---: |
| Wall | 11.165 s | 11.36 s | **+1.74%** |
| CPU | 11.075 s | 11.275 s | +1.80% |
| Peak RSS | 766,112 KiB | 799,506 KiB | +4.35% |
| Binary size | 7,604,120 B | 7,834,289 B | +3.03% |

Six alternating repetitions pinned to one core; the ranges do not overlap
(11.15-11.23 against 11.31-11.47), so the sign is not noise.

Correctness was not the problem: the PGO binary produced byte-identical evidence
trees on both seeds. It is simply slower here. The plausible reason is that this
simulator's hot path is dominated by standard-library work — `encoding/json`
reflection and SHA-256 — that PGO cannot devirtualize, so what it mostly does is
inline more aggressively, growing the binary 3% and costing instruction-cache
locality without buying call-site specialisation.

`default.pgo` was removed rather than retained. This closes the open item left
by `perf/thread-pgo`, whose timing batch was discarded as contaminated: PGO does
have a measurable effect on this workload, and it is negative.

### R6 — Reuse the detached preview book

Now the largest single allocation site at 544.56 MB of 3,220.84 MB (16.9%).
`perf/thread-sim` declined it because the matcher mutates queue links and
iceberg state on the clone and the ownership question was unresolved.

**The ownership proof is now available and it is favourable.** The two clone
books are locals of `previewMatchExcluding` and are never returned. The only
value that escapes is `*matching.MatchResult`, which is
`{Executions []*etypes.Execution, FullyFilled bool}`, and `etypes.Execution`
holds only IDs, prices and quantities — no pointer into the clone. The clone is
therefore provably dead when the function returns, and its executions are
already pooled and explicitly released on every rejection path.

It is still declined here, on return rather than on safety: reusing it needs a
`Reset` on `book.Book` that clears both indexes and returns level objects to the
freelist, touching internals the live matcher shares. Against S4's calibration —
-16.5% of allocation bought -1.45% of wall — 16.9% of allocation is worth
roughly 1.5% of wall. That is the worst risk-per-gain remaining, so the proof is
recorded for whoever revisits it rather than acted on.

## 7. Ranked next candidates

| Idea | Hotspot | Proposed | Expected | Risk | Status |
| --- | --- | --- | ---: | --- | --- |
| Adopt `GOMAXPROCS=4` | run configuration | none — no code change | **-14.3% wall, -15.7% RSS** | none technically; byte-identical over 12 runs | **measured; awaiting a protocol decision** |
| Integer instrument/venue/client handles | string-keyed lookups only ~1.6% | resolve once at setup, index dense slices | **<2%** | medium, wide diff | **deprioritized by measurement, see below** |
| Incremental margin state | margin 6.3% + liquidation 5.9% | cache, invalidate on position change | up to ~8% | high: risk semantics | not started |
| Preview-book reuse | 16.9% of allocation | reuse with a `Book.Reset` | ~1.5% | high; ownership now proved, see R6 | declined on return, not safety |
| Reduce remaining `DelayedGateway` allocation | 338 MB after S4 | checked-out scratch buffers | ~0.4% | medium | R2 attempt failed |
| ~~Go PGO~~ | whole binary | `default.pgo` from a representative run | **-1.7% (a regression)** | low semantic | **rejected by measurement, see R7** |

### Integer handles: the hypothesis measurement did not support

Map operations are ~15% of CPU, and the obvious reading is that string-keyed
lookups on symbols and venue identifiers are the cost, so integer handles would
be the big win. **The profile says otherwise.** Broken down by runtime symbol:

| Map cost | share |
| --- | ---: |
| iteration (`Iter.Next`, `matchFull`, `mapIterNext`) | ~5.4% |
| hashing and probing (`matchH2`, `aeshashbody`, `memhash64`) | ~5.0% |
| **string-keyed lookups** (`mapaccess*_faststr`, `getWithoutKeySmallFastStr`) | **~1.6%** |

The string lookups are spread across twelve callers at 0.01-0.03 s each, none
individually worth a refactor. The dominant cost was *iteration*, and the single
largest iterator was `positionExposureViolation` at 42.9% of all iteration
starts — which S9 addressed directly, by not iterating the wrong collection,
rather than by changing what the keys are.

The one remaining concentrated string-hash cost is `positionKey`, a
`{symbol string, side}` composite that is 62.5% of `aeshashbody` — about 1.2% of
CPU. That is the entire realistic prize for instrument handles here.

### What is no longer worth attacking

Two blocks now dominate and both are the hash contract rather than
implementation: `encoding/json.structEncoder` at 11.6% and SHA-256 at 7.5%,
together about 19% of CPU. After S3 the payload is encoded exactly once, and
that one encoding is the input to the ordered execution hash. Removing it means
changing the hash domain, which is the reproducibility attestation the r5 gate
seals. Everything else in the profile is now diffuse: no single remaining
function outside those two exceeds 5%.

## 8. Open escalation — not a performance finding

`perf/thread-pgo` reported that `scripts/v2-integrated-longrun-r5-contract.sh`
fails closed unless binaries report **go1.27**, while this host's default
toolchain is **go1.26.7-X:nodwarf5**. If correct, a long-run cell launched with
the default `go` would be rejected by its own gate. The exact contract check was
not quoted before that session ended, so this is recorded as **unverified** and
is for the scientific owner to confirm. No contract script was modified.
