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

### Noise floor, and which results sit near it

Run-to-run variance on this host depends on its power state, which was not
constant across this work. An A/A control — the **same binary on both sides** of
the alternating harness — establishes the detection threshold directly:

| Setting | A/A reported "change" | ranges |
| --- | ---: | --- |
| `GOMAXPROCS=4`, 12 reps | **-1.11%** | 9.08-9.63 \| 9.12-9.74 |
| `GOMAXPROCS=1`, 8 reps | **-1.24%** | 10.24-11.20 \| 10.32-11.21 |

So **an effect under about 1.2% is indistinguishable, and under about 2.5% is
weak.** This threshold applies to mains power with turbo active. On battery the
CPU sits at a low stable frequency and repeats within 0.6%, which produced
deceptively tight ranges for small effects; those tight ranges reflected a
throttled machine, not a resolved measurement.

Consequences, applied honestly to the results in this document:

* Well above threshold and unaffected: S1 (-16.0% CPU in `none` mode), S2
  (-9.9%), S3 (-11.0%), S11 (-3.6%, and independently corroborated by -21.6%
  allocated objects), and every cumulative figure (-30% to -38%).
* At roughly twice the threshold, so real but not precise: S5 (-2.7%) and S9
  (-2.0%). Both were measured with non-overlapping ranges in the low-variance
  state.
* Below threshold, and accepted on other evidence rather than throughput: S4
  (-16.5% allocation), S10 (-1.4% allocation, explicitly not a throughput
  claim), S12 (structural work reduction only).
* Below threshold and correctly rejected at the time: S6, and the map-payload
  conversion in R11.

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

### S11 — Skip the preview book when the order cannot cross (`a391be6`)

Order admission previews FOK and spot fills against a detached copy of the book.
Instrumenting a 5-minute run showed **65,171 of 105,672 previews were for orders
that cannot cross at all**, and the books are small — 2.2 levels and 4 orders per
side — so the cost was call frequency, not copy size. A matcher that gates
execution on price returns nothing for such an order and leaves both books
untouched, so the copy is waste.

`matching.PriceCrossingMatcher` is an optional promise; both built-in matchers
make it because both gate on `CanMatch`, a price comparison. A midpoint or
auction venue must not, and any matcher that does not implement it gets the full
preview. The short-circuit honours the exclusion set, and market orders always
take the full path.

**-3.6% wall, and the largest allocation reduction in this work**: allocated
bytes 3,334MB to 2,755MB (-17.4%), objects 34,733,214 to 27,232,943 (-21.6%),
clone objects -52.6%.

This had been declined twice on an estimate of about 1.5%. The estimate assumed
the cost was copying large books; instrumenting showed it was frequency.

### S12 — Read all three position sides in one lookup (`fe9b4da`)

Risk work probes every (client, symbol) pair for all three position sides.
**7,882,019 probes in 5 simulated minutes** — the most-called function in the
engine, 75 times more frequent than the preview — and **94.9% find nothing**:
42.7% because the client holds no positions at all. `GetPositionBySide` was
8.96% of CPU, against an earlier estimate of about 2%.

`types.SidedPositionStore` is an optional extension for a store that resolves
the client once and reads all three sides from it. A store that does not
implement it is probed per side as before.

**No reproducible throughput claim.** It was first measured at -1.07% and -1.45%
on battery, but re-measuring on mains power gave -2.25%, -0.40% and -3.99% across
three batches, and the A/A control below puts this host's noise floor at ±1.2%.
The effect is under that threshold. What the change does structurally is not in
doubt — it removes two thirds of the read-lock acquisitions and client-map
lookups in the engine's most-called function — but the wall-clock benefit cannot
be resolved on this hardware. It is kept as strictly-less-work with byte-identical
output, not as a measured speedup.

The reason it is small is visible in the line profile: the remaining per-side
inner lookup, which hashes a string-keyed composite, is 54% of the function and
is unchanged (see R12).

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

## Structural Optimization Frontier

The local phase is exhausted; these are the structural hypotheses, ranked by the
wasted work they target. Wall-clock deltas are reported but are **not** the
acceptance evidence: this host's A/A control ranged from -1.24% to +2.82% across
the same period, so effects under ~3% cannot be resolved by timing. Acceptance
rests on instrumented counts and on single-process profile shares, which host
load cannot move.

| Hypothesis | Measured wasted work | Architecture | Mechanism result | Wall | Semantic risk | Status |
| --- | --- | --- | --- | --- | --- | --- |
| **A. Sparse risk sweep** | 1,415,810 of 1,514,700 (symbol, client) pairs per 5 sim-min do nothing — **93.5%** | per-symbol holder index, sweeps walk holders not all clients | sweep 3.98%→2.31% (-42%), probing 4.57%→2.77% (-39%) | -1.4% to -3.8%, unresolvable | low: body has no side effect before its `continue`; index keeps zero-size entries | **ACCEPTED** `57077e9` |
| **B1. Preview short-circuit, market orders** | 2,925 clones per 5 sim-min built for market orders against an empty opposite side | extend the non-crossing test to "no included liquidity" | closes the last 4.5% of the 65,171 non-crossing previews | below noise | low | **ACCEPTED** (this commit) |
| **B2. Bounded preview clone** | clones copied 413,901 orders where the matcher read 87,067 — **79% waste**; median 10 copied per 1 traversed | truncate the clone to the price-ordered crossing prefix | clone 3.30%→1.99% (-40%), `AddOrder` 2.27%→1.31% (-42%), objects 67.85M→64.53M (-4.9%) | -3.2% at GOMAXPROCS=4, +0.1% at 1 | medium: gated on a widened matcher promise; 30,000-case bounded-vs-unbounded differential plus the 80,000-case preview fuzz | **ACCEPTED** (this commit) |
| **B3. Clone-free preview** | the same 79%, plus the two fixed map allocations per preview | read-only traversal with local `deltaFilled`/`display` state | not built | — | **high** | **REJECTED — see below** |
| **A′. Sparse margin profile** | ~20 symbol iterations per profile, 37,759 profiles per 5 sim-min | iterate only the client's held symbols | not built | — | **blocked by F1** | **REJECTED — see below** |

### B3 rejected: it would be a second matching engine

`MatchingEngine` is a public extension point, and the preview's stated purpose is
to run *the configured matcher* so a user-supplied allocation, iceberg or
self-trade rule cannot disagree with the atomicity check. A read-only traversal
cannot run an arbitrary injected matcher, so it could only ever be an opt-in fast
path with the clone retained as fallback — i.e. a **second implementation of the
matching traversal**, the most correctness-critical code in the simulator.

The audit specified exactly what that second implementation would have to
reproduce, and one item settles it: `PriceTimeMatcher`'s cursor reaches the
virtual tail of an iceberg re-queue **within the same pass**. With queue `A, B, C`
and `A` an iceberg that exhausts, the queue becomes `B, C, A`, the cursor
continues from the already-captured `next = B`, walks `B, C`, then follows
`C.Next → A` and fills `A` a second time on its fresh tranche — before the
rescan flag fires. An implementation that models the refresh as "handle it next
pass" produces a different execution sequence, and the fuzz reaches this case
5,260 times per matcher, so it is not hypothetical. Object pooling adds a second
trap: `RemoveLimit` returns levels to a shared `sync.Pool`, so "run the matcher
and undo it" would hand live levels back to the pool.

B2 captures most of the same waste with no second traversal: the same matcher on
a smaller input, where the omitted levels are provably the ones it would have
broken before reaching.

### A′ rejected: the dense scan is load-bearing, and that is finding F1

`buildAccountMarginProfile` resolves the mark for every perp book in the quote
currency *before* checking whether the client holds a position there, and an
unavailable mark fails the whole profile. Making it sparse would therefore change
behaviour — and the behaviour it would change is the one the risk audit classified
as a **scientific-blocking market-logic bug** (F1). Optimizing through it would
have silently "fixed" a defect while claiming a performance result, which is
exactly the outcome the audit objective exists to prevent. Left dense pending the
scientific owner's ruling.

### Still open

* **Track C**, the high-frequency no-op census, ranked by
  `calls × no-op fraction × cost`. Two instances found so far (previews 62%,
  position probes 94.9%) were both worth acting on, so the census is the best
  remaining lead.
* **Track E**, typed market-data fingerprint hashing: `MarketDataFingerprint` is
  24% of all `json.Marshal` and ~3.8% of CPU. Whether the fingerprint contract
  requires canonical JSON *bytes* or merely a deterministic canonical
  representation is unresolved and is a contract question, not a performance one.
* **Track F**, the research acceleration cache, which is unaffected by any of the
  above and would speed up repeated analysis rather than simulation.

## Market-Logic Findings Exposed by Performance Research

Structural optimization forces implicit assumptions to be stated, and stating
them turned out to be an effective audit of the simulator's causal semantics.
Two independent adversarial audits ran against the risk engine and the matching
preview. Full reports: `v2-risk-semantics-audit.md`, `v2-preview-semantics-audit.md`.
Reproductions are committed as tests, with open findings gated behind
`AUDIT_FINDINGS=1` so the default suite stays green.

**None of these were optimized through.** Every one is recorded as a finding for
the scientific owner to rule on.

### Scientific-blocking

| # | Finding | Measured frequency, dev-607/900101, <=15 min |
| --- | --- | --- |
| F1 | An unmarked book with **zero account exposure** fails the margin profile and suppresses liquidation. The mark is resolved before the position loop, so a book contributing nothing to the profile can fail it | **0** |
| F2 | One account's unpriceable exposure `return`s out of `CheckLiquidations`, skipping **every higher-ID account** at that mark. `CheckPositionMarginerLiquidations` uses `continue` for the identical condition — the two paths disagree | **0** |
| F3 | Cross-margin liquidation decided against a sibling instrument's stale mark; the outcome depends on lexicographic symbol order and can liquidate a solvent account | **0** wrong outcomes, but 33 of 79 clients are cross-margined in USD, so the surface is populated |
| F6 | Positions in a settlement-pending contract contribute 0 equity **and** 0 maintenance, for as long as the pending state lasts — the only unbounded staleness in the graph | **0** (no expiry reached in <=15 min) |

F1 is the finding I hit while proving the sparse risk sweep safe, and it is why
`buildAccountMarginProfile` was left dense. F2 is a strictly better finding that
the audit produced independently: two code paths implementing the same condition
differently is itself strong evidence that at least one is wrong.

Note that `exchange/margin_profile_determinism_test.go:11` **asserts F1's
behaviour as correct** — it builds a profile for a client holding no positions in
either book and requires an error. So F1 was a deliberate choice at least once,
which is what makes it a specification question rather than a plain defect.

### Ambiguous

| # | Finding | Measured frequency |
| --- | --- | --- |
| F8 | Borrow interest truncates to zero below ~10.5 M units with no remainder carry | **100%** — 15 of 15 debts below the threshold, **zero interest charged** |
| F4 | `Transfer` admits withdrawal on `balance - reserved`, ignoring unrealized loss, with no risk re-check | 0 — no caller in the simulation |
| F7 | A single breach closes every leg without re-testing solvency | 0 — netting mode only |
| P1 | The preview silently requires `FilledQty == 0`: its self-check compares this preview's fill sum against the order's *total*, so a partially filled order would be refused | unreachable through today's callers |
| P2 | The clone **repairs** state the live matcher rejects: `AddOrder` grants a fresh tranche to an iceberg with `DisplayRemaining == 0`, so on a corrupt book the preview reports a clean fill where committed matching panics. The clone hides a corruption signal | unreachable through today's callers |

**F8 is the one to look at first.** It is classified ambiguous rather than
blocking because it may be an intended precision floor, but it fires on every
debt in the configuration: the simulation currently charges no borrow interest at
all. That is an economic input, not a rounding detail.

### Does any of this require a rerun?

Per the audit: **no rerun for anything measured.** dev-607 at 5 and 15 minutes
produces zero liquidations, zero margin calls, and zero `liquidation`-path
`price_unavailable` records, so F1, F2, F3 and F6 are defects in decisions that
were never made. The candidate F1/F2 fix reproduces the evidence digest
byte-for-byte, which confirms it directly.

Two conditions would change that, and both are cheap to check against any
long-run artifact before trusting it:

1. **Any `liquidation` event on an account holding positions in two or more
   margined books of the same quote asset** — F3's trigger. Such a run *cannot*
   be rescored, because the counterfactual needs equity re-evaluated against the
   full refreshed mark set and the evidence does not contain it. It would need a
   rerun.
2. **Any `expiry_settlement_pending` record** — F6's trigger, and plausible in
   the multi-hour runs that do cross dated-future expiries.

A `price_unavailable` record whose operation is `liquidation` or
`option_liquidation` is F1/F2 firing directly.

Neither audit was permitted to read the registered holdouts, so nothing here
speaks to them.

### The preview path itself is clean

Worth stating alongside the findings: 80,000 differential scenarios across both
matchers — FIFO and ProRata, exclusions, partial fills, market/IOC/GTC/FOK/
post-only, iceberg refreshes reached through real matcher passes rather than
forged, queue depths past three, levels the order cannot reach, and levels
holding only the incoming client's own orders — produced **no divergence in
executable behaviour** between the preview and committed matching. The preview is
correct where it is reachable. P1 and P2 are latent asymmetries in unreachable
territory.

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

## 5c. Long-horizon profile — the shape does not change

Every simulator profile above is a 15-minute cell. The analyzer's numbers turned
out to be misleading at scale (see the parallel evaluation, §6b), so the same
check was owed here: a 4-hour run, profiled.

| Measure | 15 min | 4 hours |
| --- | ---: | ---: |
| Wall (`GOMAXPROCS=1`) | 11.24 s | 183.26 s |
| Evidence written | 442 MB | 6.2 GB |
| Retained heap after the run | 553 MB | 605 MB |
| `sha256.blockSHANI` | 7.5% | 6.6% |
| `structEncoder.encode` | 11.6% | 10.2% |
| map operations | ~15% | ~14% |
| allocator and GC | ~13% | ~13% |
| locking (mutex + RWMutex atomics) | ~6.4% | ~7.2% |

**No new hotspot appears and no cost grows superlinearly.** Retained heap is
still flat and still dominated by the two gateway constructors, which allocate
once at setup. This is the result that de-risks the 24-hour cell, and unlike the
analyzer it needed no correction.

The one share that grew is locking, which is why the three candidates below were
examined and all three declined.

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

### R8 — Batch the three-side position probe

`GetPositionBySide` is **96.7% of all RWMutex read-lock cost** — 2.64 s of the
2.72 s in `atomic.Int32.Add`, and about 2.8% of CPU once `RUnlock` is counted.
Four call sites probe it three times per symbol, once per position side, and
most probes return nothing.

A batched accessor taking the lock and the outer map lookup once would recover
part of that. It is declined: the four sites are in the risk sweep and the
liquidation path, they pass the results around as `*Position`, and a batched
form returning into a shared buffer introduces aliasing where none exists today.
Calibrated against S9 — which predicted ~5% and measured 2.0% — the realistic
gain is about 2% for a change in the two most correctness-critical paths in the
engine.

### R9 — Return positions by value instead of by pointer

`GetPositionBySide` heap-allocates a copy on every call: **1,745 MB and
28,591,824 objects** over a 4-hour run, 3.15% of allocated bytes and 4.27% of
objects. Returning `(Position, bool)` would remove all of it with no aliasing
risk at all.

Declined on the API contract rather than on safety. `GetPositionBySide` is part
of the exported `PositionStore` interface, which is an extension point users
implement, so its signature may not change. Working around it means an optional
extension interface plus a cached type assertion — the pattern
`ExactLinearPositionStore` already establishes — for a gain that S4's calibration
(-16.5% of allocation bought -1.45% of wall) puts under 1%.

### R10 — Skip the phase-drain lock when nothing is queued

`ClientGateway.DrainDeterministicEgress` and `Idle` together are about 40% of
mutex cost. The phase barrier polls every mount and actor in a loop until a
round makes no progress, so each poll takes a gateway's lock even when both
phase queues are empty. An atomic pending counter would let the drain return
early without the lock.

Declined on risk. The phase barrier is what makes this simulator deterministic
and `GOMAXPROCS`-invariant, and the loop's own comment records that idleness can
flip under it. Narrowing the window in which a late arrival is observed is
exactly the kind of change that passes three seeds and fails on the fourth. Worth
1-2%; not worth being wrong about.

### R11 — Convert the remaining map log payloads to ordered structs

S2 converted the three highest-volume `map[string]any` payloads and measured
-9.9% wall. Five map payloads remain, and an allocation profile by **object
count** rather than by bytes — a view this study had under-used — put
`reflect.unsafe_New` at 7.5% of allocated objects over a 4-hour run, reached
entirely through `reflect.copyVal` from `MapIter`, which is `encoding/json`
walking those maps.

The two highest-volume remainders were converted: the IOC-expiry `OrderCancelled`
payload and `maker_state`. Evidence stayed byte-identical on both seeds, so the
field ordering was right.

**Rejected: no measurable benefit.** Six alternating pinned repetitions put wall
and CPU at -0.49% with *identical* ranges (10.02-10.26 on both sides). Reflect
map-iteration objects did fall 17.6% as predicted, but total allocated objects
did not move: three passes gave a median of 33,378,280 before and 33,374,538
after. Boxing a nine-field struct into the logger's `any` parameter costs about
what the map iteration it replaced did, where S2's payloads won because their
event volume was an order of magnitude higher.

A first single-sample measurement showed +4.4% objects and was not trusted;
repeating it showed the difference was noise. Reverted rather than shipped,
because two type declarations and a test rewrite for a measured no-op is churn.

The useful conclusion is the negative one: **the remaining map payloads are too
low-volume in this composition to be worth converting**, so the S2 exercise
should not be repeated on them without first measuring the event counts of the
composition in question.

### R12 — Integer instrument keys for the position map

After the batched read (S12), the remaining cost in the position path is the
inner lookup itself: line-level profiling puts `clientPositions[positionKey{...}]`
at 54% of `PositionsAcrossSides` and the heap copy at 18%. `positionKey` is
`{symbol string, side}`, and hashing it is 47% of all `aeshashbody` time.

Keying the inner map by an interned integer instrument identifier would make
that hash trivial. It is declined: the interning table must itself be consulted
by symbol, so the batched read would trade three string-composite hashes for one
string hash plus three integer hashes — roughly half of the 54%, which is about
1% of simulator CPU, for a change to the core position-storage structure.

### R13 — Return positions by value from the batched read

The heap copy is 18% of `PositionsAcrossSides`, and unlike `GetPositionBySide`
this method is new, so the public-interface objection that blocked R9 does not
apply: it could return values and allocate nothing.

Declined on aliasing. Two of the four call sites append the returned pointers to
a slice that outlives the loop iteration, so returning a stack array and taking
its element addresses would let a later symbol or client overwrite positions the
liquidation path is still holding. Making those callers copy reintroduces the
allocation; making them hold values changes their downstream signatures. About
0.7% of CPU for pointer-lifetime risk in the liquidation path is the worst
risk-per-gain remaining.

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

### Map operations are diffuse, not concentrated

Map work is 11.89% of CPU, which invites the assumption that some hot lookup is
responsible. It is not. The largest single consumer of map *accesses* is
`OrderBook.FindOrder` at 52% of them — and `FindOrder` is **1.47% of CPU**. The
rest of the 11.89% is Swiss-table internals (`matchH2`, `matchFull`,
`Iter.Next`) spread across the whole engine, with no caller worth attacking on
its own.

The one concentration is `positionKey` hashing at 47% of `aeshashbody`, which
R12 prices at about 1%.

### What is no longer worth attacking

Two blocks now dominate and both are the hash contract rather than
implementation: `encoding/json.structEncoder` at 11.6% and SHA-256 at 7.5%,
together about 19% of CPU. After S3 the payload is encoded exactly once, and
that one encoding is the input to the ordered execution hash. Removing it means
changing the hash domain, which is the reproducibility attestation the r5 gate
seals. Everything else in the profile is now diffuse: no single remaining
function outside those two exceeds 5%.

## 8. Toolchain: the go1.27 gate, resolved

`perf/thread-pgo` flagged, without evidence, that the r5 contract might reject
binaries built with this host's default toolchain. That is now verified and
resolved.

**The gate is real.** `v2_r5_is_go_127` requires `go1.27*` and is enforced at six
places: twice in `run-v2-integrated-longrun-cell.sh` (simulator, prunegate),
three times in `extract-v2-integrated-longrun-cell.sh` (analyzer, simulator,
prunegate) and once in `check-v2-integrated-longrun-parity.sh`. This host's
default `go` stamps binaries `go1.26.7-X:nodwarf5`, which fails all six.

**It is satisfiable, not blocking.** `GOTOOLCHAIN=go1.27.0` fetches and uses the
pinned toolchain, and a binary built that way reports `go1.27.0` and passes the
gate. No script change is needed; the requirement is on the build invocation.

**The gate is protecting something specific.** Built from the same clean tree,
go1.27 populates the run manifest's VCS provenance while the host default does
not:

    go1.26.7-X:nodwarf5   "revision": "unknown",  "time": ""
    go1.27.0              "revision": "e4f0c5f2…", "time": "2026-08-30T04:21:07Z"

So a cell built with the host default would carry an empty provenance stamp in
its own manifest. The pin is not arbitrary and should not be relaxed.

**The toolchain does not change the trajectory.** Built from one tree under both
toolchains, the two binaries produce an identical ordered execution stream hash
(`51541f91db7c5eae8688235d3961a76af421ab782f05ab62649076cf90aef332`) and
byte-identical venue evidence (`94672cc58bef43a82c57392be0b01782`). The only
difference in the whole 27-file tree is the manifest provenance above, which is
the field doing its job.

**The optimizations hold under the required toolchain.** Base against optimized,
five alternating pinned repetitions under each:

| Toolchain | base wall | optimized wall | change |
| --- | ---: | ---: | ---: |
| `go1.26.7-X:nodwarf5` | 18.21 s | 11.26 s | -38.2% |
| `go1.27.0` | 19.49 s | 11.93 s | **-38.8%** |

**But go1.27 itself costs about 6% on this workload.** The same source is
consistently slower under it — base 18.21 s to 19.49 s (+7.0%), optimized
11.26 s to 11.93 s (+5.9%), with non-overlapping ranges in both pairs. That is a
toolchain cost the project is already committed to for provenance reasons, not
something this work introduced, but it is worth knowing: roughly a sixth of the
speedup measured here is spent paying for the newer compiler.

Every other measurement in this document was taken under `go1.26.7-X:nodwarf5`.
The relative results carry over — the speedup is 38.8% rather than 38.2% under
go1.27 — but absolute wall times in the tables above should be read as about 6%
optimistic relative to a real r5 build.


## Track C — the high-frequency no-op census

Ranking waste by `calls x no-op fraction x cost per call` needs a count of
useless invocations, which a CPU profile cannot give: a profile says where time
goes, not whether the work was needed. The `census` package supplies that. It is
off unless `EXSIM_CENSUS` is set, the disabled path is one branch on an
immutable global, and sites register themselves so no central table has to be
edited to add one.

Measured on `dev-607-none` (the registered logging-parity control: identical
population to dev-607 with recorders off), seed 607, **2 simulated hours**,
three venues, `GOMAXPROCS=4`.

| site | calls | useless | useless % | verdict |
|---|---:|---:|---:|---|
| `marketdata.Publish` (all types) | 2,959,320 | 1,554,626 | 52.5 % | see breakdown |
| `marketdata.Publish[Delta]` | 1,682,047 | **1,387,675** | **82.5 %** | **act** |
| `marketdata.Publish[Trade]` | 610,963 | 62,511 | 10.2 % | leave |
| `marketdata.Publish[Funding]` | 21,604 | **21,604** | **100 %** | **act** |
| `marketdata.Publish[Snapshot]` | 561,981 | 171 | 0.03 % | **hypothesis rejected** |
| `marketdata.Publish[Instrument]` | 66 | 6 | 9.1 % | negligible |
| `exchange.logSnapshots.book` | 561,354 | 323,712 | 57.7 % | ambiguous, see below |
| `exchange.CheckExpiries` | 21,600 | 21,600 | 100 % | **act** |
| `exchange.CheckAndSettleFunding` | 21,600 | 21,599 | 99.995 % | **act** |
| `exchange.CheckListings` | 21,600 | 21,594 | 99.97 % | **act** |
| `exchange.UpdateDerivativeMarks.contract` | 475,002 | 176,946 | 37.3 % | **not waste — my error** |
| `marketdata.PublishDelta.alloc` | 0 | 0 | — | **dead code** |

### C1 — book deltas built for an audience that is not there

`publishBookUpdate` (`exchange/order_handling.go:537`) heap-allocates a
`BookDelta` and calls `Publish`. In two simulated hours **1,387,675 of those
1,682,047 fan-outs reach no subscriber** — roughly **16.6 M wasted allocations
and publisher-lock acquisitions per 24-hour cell**. This is the single largest
no-op class found so far, an order of magnitude above the preview-matching and
position-probe classes that were worth acting on earlier.

`Publish[Funding]` is smaller but total: **100 % of 21,604 funding fan-outs
reach nobody**, because nothing in this population subscribes to `MDFunding`.

**Economically expected?** Yes — an actor subscribing to trades on a symbol has
no reason to want its deltas, and the publisher is right to deliver nothing.
The waste is not the empty delivery, it is that the payload is constructed
before anyone asks whether it is wanted. **PERFORMANCE ONLY.**

**The constraint that makes this non-trivial.** `Publish` increments `p.seqNum`
whenever the symbol has any subscriber, *before* the per-subscriber MDType
filter. So a fan-out with subscribers but no type match still consumes a
sequence number. Skipping such a call outright would renumber the market-data
stream that other subscribers observe, which can move actor behaviour and
therefore the trajectory. Any fix must suppress **construction** while leaving
sequence-number consumption exactly as it is.

### C2 — fixed-cadence sweeps that rescan everything to discover nothing

Three automation jobs run once per simulated second and answer "is anything
due?" by scanning every instrument:

* `CheckExpiries` — 21,600 sweeps, **100 % found nothing**, 561,420 instrument
  visits in two hours;
* `CheckAndSettleFunding` — 21,600 sweeps, **one** settlement; each sweep ranges
  all instruments, allocates a slice, sorts it, then takes the exchange mutex
  once per perp only to compare `now < NextFunding` and continue;
* `CheckListings` — 21,600 sweeps, six listings.

**Economically expected?** The *outcome* is: expiries and funding settlements
are genuinely rare. The *cost* is not — every due time is known in advance, so
a next-due watermark answers the same question with one comparison and gives
byte-identical events in the same order. **PERFORMANCE ONLY.**

### C3 — rejected hypotheses, recorded

* **Public snapshots built for nobody: false.** I expected
  `logSnapshots` to build two `GetPublicSnapshot` slices per book per second for
  symbols with no snapshot subscriber. Measured: **171 of 561,981**, 0.03 %.
  Snapshots are consumed almost everywhere. Hypothesis dead.
* **`MDPublisher.PublishDelta` is dead code**: **0 calls**. Every live delta
  goes through `publishBookUpdate` calling `Publish` directly. My first
  instrumentation was attached to the unused function and reported zero waste
  while 1.68 M deltas flowed past it. Worth recording as a warning: a census
  site on the wrong function reports "no problem here" just as confidently as
  one on the right function.

### C4 — a measurement error of mine, corrected

I first counted `UpdateDerivativeMarks` calls where the underlying price was
unchanged since the previous tick (37.3 %) as repeated work. **That is wrong.**
`Black76Premium(underlying, strike, iv, yearsLeft, isCall)` also takes time to
expiry, which advances every tick, so an unchanged underlying still produces a
different premium. The site is kept, with its note corrected to say so, because
the count is still useful context — but it is not waste and must not be
optimized away.

### C5 — ambiguous: 57.7 % of book snapshots repeat the previous record

`exchange.logSnapshots.book` shows **323,712 of 561,354** snapshots byte-identical
to the previous snapshot of the same book, covering 2,513,286 serialized price
levels. That is real redundancy in the evidence stream.

It is **not** a free optimization. The r5 lifecycle analyzers consume
`BookSnapshot` records directly — `analysis/expiry_fill.go` decodes every
snapshot during a contract's listed lifetime specifically so that a malformed
active snapshot cannot hide the last executable-depth transition. Emitting
fewer records changes what those predicates see. Classified **AMBIGUOUS /
REQUIRES SPECIFICATION**: the redundancy is measured and recorded, the decision
whether a periodic snapshot stream may elide unchanged records belongs to the
evidence contract, not to a performance pass.

### C1 implemented — build the delta only for an audience that exists

`MDPublisher.PublishBuilt(symbol, mdType, build func() any, timestamp)` calls
`build` at most once, and only when a running subscriber will actually receive
the message. `publishBookUpdate` now uses it.

**What it deliberately does not change.** `Publish` increments `seqNum` whenever
the symbol has any subscriber, *before* the per-subscriber MDType filter, so a
fan-out with subscribers but no type match still consumes a sequence number.
Suppressing the call would renumber the market-data stream other subscribers
observe and could move the trajectory. Only construction is suppressed;
sequence-number consumption is untouched. That is why the change is
trajectory-neutral rather than merely "probably safe".

**The closure is free.** The obvious objection is that a builder closure trades
a `BookDelta` allocation for a closure allocation. Escape analysis says
otherwise — `go build -gcflags=-m` reports
`exchange/order_handling.go:534: func literal does not escape`, because `build`
is called but never stored. Verified rather than assumed.

| measurement | before | after | delta |
|---|---:|---:|---|
| allocated objects / sim-hour | 137,780,800 | 135,358,257 | −1.76 % — **RETRACTED, see below** |
| peak RSS (median of 2) | 742.6 MB | 739.5 MB | −0.43 % (2 samples, weak) |
| wall clock, 1 h sim, 4 reps | 25.96 s | 26.03 s | +0.26 % (A/A control +0.20 %) |
| execution stream hash, seeds 900101 / 900102 | — | — | **byte-identical** |

**Accepted on allocation and peak-RSS grounds, explicitly not on wall time.**
The wall-clock difference is smaller than the A/A control measured in the same
session, so this workload at `GOMAXPROCS=4` is not allocation-bound and the
change must not be quoted as a speedup. The allocation reduction is a
deterministic count, not a timing, and the RSS drop was consistent across both
measured pairs.

The honest reading is that the census found a large *count* of useless work
whose *unit cost* is low. That is worth knowing in itself: it corrects the
assumption, carried since the preview-matching result, that a big no-op class
is automatically a big win. Frequency alone does not decide it — the earlier
wins removed no-ops that each dragged a book clone or a mark resolution behind
them, while a dropped `BookDelta` is 32 bytes and a lock acquisition.

### C2 rejected — the 100 % no-op sweeps are too cheap to matter

`CheckExpiries`, `CheckAndSettleFunding` and `CheckListings` are the purest
no-ops in the census: 21,600 sweeps each per two simulated hours, finding
nothing 100 %, 99.995 % and 99.97 % of the time, and `CheckExpiries` alone
visits 561,420 instrument entries to do it. A next-due watermark would remove
all of it and is easy to write.

**I did not implement it, because the profile says it is invisible.** In a
one-hour CPU profile at `GOMAXPROCS=4`, none of the three functions appears
anywhere in the top 200 nodes by cumulative time. Their combined cost is below
roughly 0.1 %.

This is the C1 lesson applied before spending effort rather than after: a 100 %
no-op rate ranks nothing on its own. `wasted = calls x no-op fraction x cost per
call`, and when the third factor is a map range over 26 instruments, the product
stays near zero. Recorded as rejected so the census's most eye-catching rows do
not get re-litigated.

### The profile's actual answer: the integrity hash, not the market logic

The same profile, taken on `dev-607-none` where **logging is off entirely**:

| | share |
|---|---:|
| `encoding/json.Marshal` (cum) | **14.99 %** |
| `crypto/.../sha256.blockSHANI` (flat) | 3.61 % |
| `runtime.mallocgc` (cum) | 12.96 % |
| `runtime.scanObject` (cum) | 8.22 % |
| mutex lock + unlock (flat) | 6.02 % |
| scheduler heap (`eventHeap.Less` + `heap.down`) | 6.22 % |

`pprof -peek` attributes **100 % of `json.Marshal` to a single caller**:
`(*checkpointSink).observe` in `simulations/multivenue/divergence.go`. Every
execution event is marshalled to JSON and SHA-256'd to feed the ordered
execution-stream digest — and with logging off those bytes exist *only* for the
hash. Roughly **19 % of CPU is spent proving the run is reproducible**, not
simulating a market.

Censusing that call by event name gives the size of the job and where it is
concentrated:

| event | calls / sim-hour | bytes hashed | share | cumulative |
|---|---:|---:|---:|---:|
| `balance_change` | 627,564 | 197,490,082 | 20.2 % | 20.2 % |
| `OrderFill` | 625,528 | 183,205,813 | 18.7 % | 38.9 % |
| `OrderAccepted` | 610,350 | 173,911,957 | 17.8 % | 56.7 % |
| `BookDelta` | 869,857 | 98,986,502 | 10.1 % | 66.8 % |
| `BookSnapshot` | 281,181 | 91,958,423 | 9.4 % | **76.2 %** |
| `venue_balance_change` | 300,261 | 54,942,982 | 5.6 % | 81.8 % |
| `OrderCancelled` | 409,359 | 37,428,976 | 3.8 % | 85.6 % |
| `fee_revenue` | 299,928 | 36,801,302 | 3.8 % | 89.4 % |
| `Trade` | 312,764 | 36,767,535 | 3.8 % | **93.2 %** |
| remaining 12 types | 306,510 | 66,795,000 | 6.8 % | 100 % |

**978.4 MB marshalled and hashed per simulated hour**, 4,643,655 events — about
**23.5 GB per 24-hour cell**, with logging off.

**Five event types are 76 % of it; nine are 93 %.** That concentration is what
makes this tractable: a hand-written encoder that emits *byte-identical* JSON
for those types keeps every published hash valid while removing the reflection
that `structEncoder.encode` spends 10.25 % of CPU on. The pattern already exists
in this tree — `simulations/feesim/logger.go` assembles records from
pre-encoded segments with `strconv.Append*` into a reused buffer.

This is the largest single lead in the campaign so far and the next thing to
build. The acceptance test is the one already in use: identical seed and config
must reproduce the identical execution stream hash, which is exactly the
property a byte-identical encoder preserves and a faster-but-different encoder
would destroy.

### Correction — the allocation profile is sampled, and its noise floor is 1.9 %

I accepted C1 partly on "−2,422,543 allocated objects (−1.76 %)", and called it
a deterministic count rather than a timing. **That was wrong, and the claim is
retracted.**

`-allocprofile` is a *sampled* profile: Go records one sample per
`MemProfileRate` bytes (512 KB by default) and scales up, so `alloc_objects` is
an estimate, not a count. Running the identical binary on the identical seed and
config three times:

```
mv-lazy run 1   137,718,484
mv-lazy run 2   136,911,770
mv-lazy run 3   139,533,169     spread 2,621,399 = 1.9 %
```

**The noise floor is larger than the effect I reported.** The −1.76 % is not
evidence of anything.

This is the same discipline that was already applied to wall clock — establish
A/A before believing a small delta — and I simply failed to apply it to a
profile I had labelled "deterministic" in my own head. The lesson generalizes:
*sampling* is a property of the instrument, not of whether the number looks like
an exact integer. `alloc_objects` prints as `137,780,800` and reads as a census;
it is not one. The census counters in this branch are exact, because they are
atomic increments on every call. Profiles are not.

What survives for C1: the trajectory is byte-identical, the test suite is green,
the census count of 1,387,675 fan-outs reaching nobody is exact (it is an atomic
counter, not a profile), and wall clock is unchanged within an A/A control.
**No performance benefit is demonstrated.** C1 removes provably redundant work,
and that is the honest justification for keeping it — not a measured speedup.

### Track E, first increment — byte-identical hand encoding, not yet justified

`types.JSONAppender` is an optional interface for payloads on the hot evidence
path: a type that can append its own JSON, byte-for-byte identical to
`encoding/json.Marshal`. `checkpointSink.observe` uses it when the payload
implements it and falls back to `encoding/json` otherwise, so adding a type
costs nothing elsewhere and no registry is edited.

Implemented for `BalanceChangeEvent` and `BalanceDelta` — the largest single
event type at 20.2 % of hashed bytes. Byte identity is enforced by a
differential test against `encoding/json`, not by inspection: 20,000 randomized
events covering quote, backslash, control, HTML-significant (`<`, `>`, `&`),
non-ASCII and invalid-UTF-8 strings, integer extremes, `omitempty` on and off,
and the nil-versus-empty slice distinction (`null` versus `[]`). Strings that
the fast path cannot reproduce verbatim fall back to `encoding/json` for that
string, which is what makes byte identity a guarantee rather than an assumption
about the data.

| measurement | before | after |
|---|---:|---:|
| execution stream hash, seeds 900101 / 900102 | — | **byte-identical** |
| `encoding/json.Marshal`, cumulative CPU | 14.99 % | **13.14 %** |
| allocated objects, median of 3 | 135,779,557 | 143,615,708 (**+5.8 %**) |
| wall clock, 1 h sim, 4 reps | 25.78 s | 26.11 s (+0.84 %, A/A control +0.73 %) |

**Not accepted.** The reflection cost does fall — 1.85 percentage points off
`json.Marshal` for a type worth 20.2 % of the bytes, which is the right
direction and roughly the right size. But allocations rise 5.8 %, well outside
the 1.9 % sampling floor, and wall clock does not move.

The cause is buffer sizing. `encoding/json.Marshal` encodes into a pooled
`encodeState` and then allocates a result of exactly the right length; the
appender allocates a fixed hint up front. At a 256-byte hint most events grew;
raising it to 512 stopped the growth but now over-allocates against a 315-byte
mean. Next increment: carry a per-event-name size hint learned from the previous
encoding so the single allocation is right-sized, then extend to the remaining
four types that make up 76 % of hashed bytes and measure once with a signal big
enough to clear the noise. Kept wired, and explicitly marked unaccepted, because
the next step builds directly on it.

### The allocation instrument, replaced

The sampled profile misled in **both** directions. Replacing it with
`runtime.MemStats.Mallocs`, which is an exact cumulative count, and re-running
the appender question:

| | mallocs (exact) | bytes allocated |
|---|---:|---:|
| baseline | 161,801,929 / 161,802,115 | 13.108 GB |
| appender, fixed 512 B hint | 161,802,245 / 161,801,598 | 13.207 GB (+0.75 %) |
| appender, learned size hint | 161,802,620 / 161,802,371 | 13.140 GB (+0.25 %) |

**The object count is identical to within 600 allocations out of 161.8 million.**
The "+5.8 % objects" that made me reject the appender was an artifact: the
appender allocated more *bytes*, the sampled estimator saw more samples, and it
inferred more objects that were never there. The same instrument had earlier
invented C1's −1.76 % "win". One sampled instrument, two false readings in
opposite directions.

`runtime.MemStats.Mallocs` is now printed by the runner under `EXSIM_CENSUS`.
Every future allocation claim in this campaign should use it. Run-to-run spread
is under 0.001 %, against 1.9 % for the profile.

The learned size hint (previous encoded length for that event name, plus an
eighth and 16 bytes, held in a `sync.Map` of atomics) cuts the byte overhead
from +0.75 % to +0.25 %. It cannot reach zero: `encoding/json` encodes into a
pooled buffer and then allocates a result of exactly the right length, which a
single up-front allocation cannot match without knowing the size in advance.

### Why the appender still does not show: coverage is 17 %, not 76 %

The census now reports, per event type, whether the payload took the appender or
fell back to reflection. That turned an assumption into a measurement, and the
assumption was wrong:

| event | calls | still on reflection | bytes |
|---|---:|---:|---:|
| `balance_change` | 627,564 | 108,320 (**17.3 %**) | 197.5 MB |
| `OrderFill` | 625,528 | **100 %** | 183.2 MB |
| `OrderAccepted` | 610,350 | **100 %** | 173.9 MB |
| `BookDelta` | 869,857 | **100 %** | 99.0 MB |
| `BookSnapshot` | 281,181 | **100 %** | 92.0 MB |
| all others | — | **100 %** | 232.8 MB |

I wrote appenders for `types.BookDelta`, `types.PriceLevel` and
`types.BookSnapshot` and they are never reached, because **the event payloads
are different types**: the `BookDelta` event logs a `map[string]any`
(`deltaLog` in `publishBookUpdate`), and the `BookSnapshot` event logs
`bookSnapshotEvidence`, not `types.BookSnapshot`. Only `balance_change` was ever
diverted, so actual coverage is about **17 % of hashed bytes, not the 76 % the
five-type plan assumed** — which is exactly why the wall clock did not move.

Two things follow. First, the appenders that are written are correct and tested
but currently dead on this path; they stay, because the market-data publisher
also encodes those types. Second, the remaining work is not "write four more
appenders for the types I already named" — it is to find the *actual* payload
type behind each event name first. The `map[string]any` payloads are the most
expensive of all to marshal, since `encoding/json` sorts map keys on every call,
so `BookDelta` at 869,857 events per simulated hour is probably the single best
target left.

Recorded as the next increment rather than guessed at: the coverage census now
answers the question directly, so the next pass can be aimed rather than hoped.

### The map payload converted — and the appender rejected

Two changes were tested together and had to be separated, because only one of
them works.

**`bookDeltaEvidence` replaces the last high-volume `map[string]any` payload**
(869,857 events per simulated hour) with a struct whose fields are declared in
lexicographic order of their JSON names — the order `encoding/json` emits for a
map. The test that matters is not "the struct matches `encoding/json`", which
would be true of any field order, but "the struct matches **the map literal it
replaced**", with the original map kept in the test as the oracle, plus a test
that states the expected byte string outright.

Measured like-for-like, census off, same tree otherwise, two runs each:

| | mallocs (exact) | bytes allocated |
|---|---:|---:|
| map payload | 159,212,011 | 13.038 GB |
| struct payload | 145,749,922 | 12.436 GB |
| | **−8.46 %** | **−4.62 %** |

Wall clock, on the one run whose A/A control was clean (+0.67 %): **−3.89 %**.

**This does not contradict `68297a7`**, which rejected the same class of change.
That commit converted *lower-volume* payloads and said so explicitly: "S2's
payloads won because their event volume was an order of magnitude higher".
`BookDelta` at 869,857 events per simulated hour is that order of magnitude.
Its conclusion — that volume decides — is confirmed here, not overturned.

**The byte-identical appenders are rejected.** `types.JSONAppender` and the
`AppendJSON` implementations encoded correctly, were differentially tested
against `encoding/json` over 40,000 randomised cases including HTML escaping,
invalid UTF-8, `omitempty` and the `null`-versus-`[]` distinction, and kept the
execution stream hash byte-identical. They also do nothing:

| | mallocs | bytes | wall |
|---|---:|---:|---:|
| struct, appenders off | 145,750,169 | 12.436 GB | — |
| struct, appenders on | 145,749,563 | 12.552 GB (+0.93 %) | +0.44 % (A/A +0.62 %) |

Identical allocation count, 0.93 % more bytes, no wall-clock effect. The whole
measured win belongs to removing the map; bypassing reflection on top of that
adds nothing. Reverted rather than kept, because an encoder that must be held
byte-identical to `encoding/json` forever is a permanent maintenance liability
and it buys nothing.

### Two instrument errors, both mine

**The wrapper "regression" that never existed.** An appender on
`instrumentLogEvent` measured as +3.3 % mallocs, and no change to it helped. The
cause was not the wrapper: the census probe I had added to identify fallback
types formats a `%T` string on **every** reflection-path event, so every
measurement taken after adding it included the probe's own allocations. I was
comparing two builds across a change in instrumentation. The exact-allocation
counter is now gated on its own `EXSIM_ALLOC` variable rather than on
`EXSIM_CENSUS`, so allocations can be measured without the counter that measures
them.

**A/A discipline applies to the host, not just the change.** The final
struct-versus-map wall-clock A/B returned −0.40 % against an A/A control of
**+5.52 %** — the machine had become an order of magnitude noisier than the
+0.62 % earlier in the same session. That run is discarded, not reported. The
wall-clock attribution to the struct therefore rests on elimination: the
combined change measured −3.89 % on a clean host, and the appender half measured
nothing on a clean host. A direct struct-versus-map A/B should be repeated when
the machine is quiet.

## Where the time is not: the exchange logic itself

The recurring question in this campaign has been whether core exchange logic,
or a rewrite of it in C++/Rust/assembly, is worth pursuing. The profile answers
it, and the answer is no.

Taken on `dev-607-none` with raw logging off, so every figure below is market
mechanics rather than evidence writing:

| subtree | share of CPU |
|---|---:|
| `checkpointSink.observe` — serialize + hash for the execution digest | **21.35 %** |
| `settleExecution` — position, cash and fee settlement | 7.57 % |
| `ProRataMatcher.Match` + `PriceTimeMatcher.Match` | **0.45 %** |

**The matching engine — the thing an exchange nominally is — costs 0.45 % of
CPU.** Order admission, settlement, fee accounting and evidence production
around it cost roughly seventy times as much. Optimizing matching, in any
language, cannot matter.

These are not additive: `settleExecution` writes events, so part of its 7.57 %
is inside the 21.35 %. The two unambiguous statements are that evidence
production is the largest single subtree and that matching is negligible.

### The no-op census has no third instance here

The two structural wins in this campaign both came from finding an operation
that ran constantly and usually did nothing — preview matching at 62 % useless,
position probes at 94.9 %. The settlement path was searched for a third:

* `recordFeeRevenue` already returns immediately when both fees are zero;
* `moveVenueBalance` already returns immediately on a zero delta or empty asset;
* `restoreFeeHeadroom` already returns for market orders.

**Every hot guard is already present.** This is recorded as a negative result so
the settlement path is not re-searched for the same pattern: the cost there is
real work — balance updates and event emission — not waste.

### C++, Rust and assembly: no win available

Stated with measurements rather than intuition, now that the hot paths are
known:

* The **binary encoder** after removing hashing runs at 111 ns/event for the
  most complex family and 54 ns for the simplest. That is
  `binary.LittleEndian.AppendUint64` compiling to a MOV against a reused buffer;
  there is no instruction-level headroom for another language to take.
* The **hash** is `crypto/sha256`, which Go already dispatches to SHA-NI
  hardware instructions — `sha256.blockSHANI` is what appears in the profile.
  Hand-written assembly would be reimplementing the same instruction.
* The **matching engine** is 0.45 %, so even an infinitely fast rewrite is
  invisible.
* What remains is dominated by allocation, GC scanning and map access, which
  are runtime and data-structure properties. A language change would have to
  bring a different memory model to help, and that is a rewrite of the
  simulator, not an optimization of it.

The conclusion is that the remaining opportunity is **structural, not
linguistic**: stop producing the bytes rather than encode them faster, which is
what the VNext binary format does and why its ceiling is 17.9 %.

## Rejected: inlining the scheduler heap's ordering key

**Hypothesis.** The event scheduler's priority queue costs about 5 % of CPU
(`heap.Pop` 4.06 %, `heap.Push` 1.0 %), and `eventHeap.Less` alone is **2.37 %
flat** for what is two integer comparisons. The heap is a `[]*ScheduledEvent`,
so every comparison dereferences a pointer into a separately allocated event and
every sift walks a different cache line. Storing the ordering key inline —
`{at int64, id uint64, event *ScheduledEvent}` — should let a sift read
contiguous memory.

**Result: rejected. It is slower.**

```
A1  median 28.88s   28.87 30.29 28.88 28.60 29.82
A2  median 28.90s   28.65 28.90 28.86 29.54 29.08
B   median 29.27s   28.65 30.62 29.27 29.19 29.68

A/A control  : +0.06%   <- the cleanest control measured in this campaign
B vs A pooled: +1.33%
```

Evidence was byte-identical on seeds 900101 and 900102 and the full suite passed,
so the change was correct — just worse. Reverted.

**Why the hypothesis was wrong.** `Less` is expensive because it is called
constantly, not because it misses cache. Scheduled events are allocated close
together in time and therefore close together in memory, so the pointers were
already resident and the dereference was nearly free. Meanwhile `Swap` went from
moving 8 bytes to moving 24, and a heap sift does many more swaps than the
locality saved.

**What this rules out.** The scheduler's ~5 % is not a data-layout problem, so
it will not yield to a cheaper representation of the same algorithm. Reducing it
requires an algorithmic change — a calendar or ladder queue exploiting the fact
that event timestamps are clustered on a 1 ms grid — which is a much larger
change and is not attempted here. Recorded so the layout idea is not retried.

## The competing option: JSON appenders at the call site

The binary evidence format has until now been measured against the JSON path
as it stands. That is the wrong baseline for a promotion decision, because a
cheaper intervention on the JSON path exists and is now measured.

Re-testing the one unsound rejection in this campaign (the `instrumentLogEvent`
wrapper appender) produced two results rather than one. Both were checked for
byte-identical output against the reflection path first, because the ordered
execution-stream digest is taken over exactly these bytes.

**Implementing `json.Marshal` cannot win.** A `MarshalJSON` on the wrapper is
**2.27x slower** and allocates 8 more objects per event. The cause is
structural, not incidental: the encoder calls the `Marshaler` and then compacts
its output into the encoder's own buffer, so a hand-written marshaller pays for
a second buffer and a copy that the reflection walk never pays. No amount of
tuning inside `MarshalJSON` reaches parity.

**Bypassing `encoding/json` at the call site does win: 7.6x, zero
allocations.** A `AppendJSON(dst []byte) []byte` interface, type-switched in
`checkpointSink.observe` instead of calling `json.Marshal`, writes straight into
a reusable scratch buffer.

### What this means for the promotion decision

The two options are not as far apart as the headline numbers suggest, and the
comparison must be stated in terms of what each one addresses:

| | JSON appenders | canonical binary |
| --- | --- | --- |
| addresses the 14.99 % marshal share | yes | yes |
| addresses the 3.61 % per-event digest | no | yes, one continuous hasher replaces per-event `Sum256` |
| bytes written | unchanged | 2.92x smaller |
| typed decode, block index, selective queries | no | yes |
| evidence format changes | no | yes |
| per-family hand-written encoders required | yes | yes, the same set |
| map payloads | cannot be byte-identical, must fall back | ride as opaque JSON, still typed frames |

The appender path is bounded above by the marshal share it addresses, so
roughly **13 % of wall time** against binary's measured **15.73 %**. That gap is
real but it is not the gap between 15.73 % and zero, and an honest promotion
recommendation has to say so.

The decisive asymmetry is not the wall-clock difference. It is that the appender
path buys speed while leaving the evidence in a format that is still 2.92x
larger, still requires a full sequential parse to answer any question, and still
has no typed reader. The binary format's case rests on those, with the wall
clock as a secondary benefit — not the other way round.

### Caveat on the 7.6x

The appender benchmark reuses one scratch buffer and so allocates nothing. The
real `observe` path returns its encoded bytes to the raw-evidence logger for
reuse, so a production appender could not reuse a single buffer unconditionally
without either copying at the handoff or retiring the JSONL writer. The 7.6x is
therefore an upper bound on the call-site appender option, and closing that gap
requires the same JSONL retirement the binary path requires.


## What independent grading found, and what it cost

A separate reproducer graded `113189a` from a clean workspace with its own
harness. Its timing method was stronger than mine: two JSON arms as the A/A
control with rotating order, paired within-round deltas rather than pooled
medians.

**The speedup reproduces.** −13.26 % on medians and −13.19 % paired, against an
A/A of −0.82 % / +0.30 %; all 12 paired deltas negative, sign test p ~ 0.0002; a
second 8-round run agreed at −13.10 %. It also built a census-free JSON arm and
confirmed the probe does not confound the timing (−12.49 % against it).

**It withdrew a claim of mine.** "Every binary sample lies below every JSON
sample" does not survive a contended host. The paired analysis carries the
result; the separation sentence was overreach and is retracted.

**It found four defects. Three were real and are fixed.**

1. **A false attestation.** Binary mode wrote `checkpoints.jsonl` with
   `event_count: 0` and an all-zero hash, so any two binary runs compared
   identical in the tool whose purpose is telling runs apart, while
   `executionHash()` had no caller. This is the most serious defect the campaign
   produced, and it would have shipped.
2. **An unencodable payload truncated the stream.** 102 events with one bad
   payload recorded 1 and dropped 101, reported only at close. Now substituted
   per event, as the JSON path always did, and counted in the checkpoint.
3. **Encoding depended on caller boxing.** `Trade` had a pointer receiver, so a
   `Trade` value took the opaque path and a `*Trade` took the typed path: one
   logical event, two encodings, two digests. Value receivers now; `Trade` was
   the only pointer-receiver appender in the tree.

**One finding was itself wrong, and measurement settled it.** The reproducer
re-attributed the allocation win from 8.22 % to 5.20 %, on the grounds that a
disabled-census line at `divergence.go` allocates one string per event. That is
true, and it also had no way to see that `binarysink.go` carried an equivalent
unguarded concat, so it removed the probe from one arm only. Both guarded and
re-measured: **136,403,401 to 125,083,537 mallocs, −8.30 %**. The original
figure stands.

**One overstatement of mine, correctly caught.** The GOMAXPROCS determinism
evidence was a 10-minute run presented among 1-hour figures. The reproducer
checked determinism at 1h itself and it holds — 659,881,373 bytes identical
across three runs at GOMAXPROCS 4 and 8 — but the original wording implied a
check that had not been run.

Losslessness held at field level under its own reflection test, including
invalid UTF-8, NUL, emoji, nil-vs-empty and `omitempty`. It also noted the
binary path *fixes* a JSON-path weakness: the old digest hashed
`eventName+venueID` unseparated, so `("ab","c")` and `("a","bc")` collide.

## The result that matters: logging on

Every figure in this campaign until now came from `log_mode: none`. That was
the right isolation for measuring encode-and-hash cost, and it is the wrong
regime to conclude from: **six of the seven registered configs use
`log_mode: full`**. The one I optimised and measured is the one barely used.

The independent reproducer predicted the speedup would not carry, because with
the binary sink on and raw logging on, `observe` returns nil and the venue
logger marshals the payload a second time for the JSONL. Measured on dev-607,
seed 607, 20 simulated minutes, both arms writing real evidence:

| arm | median | disk |
| --- | ---: | ---: |
| JSON | 9.99-10.04 s | 594,722,408 |
| binary, dual-write | 10.83 s | 769,457,467 |

**+8.18 % against an A/A of +0.49 %, and 29 % more bytes.** Not "the speedup
fails to carry" — the binary path is *slower* than the thing it replaces,
because the run encodes its evidence twice and stores it twice. The prediction
was right and the reality was worse than the prediction.

### The fix is the design that was always intended

`EXSIM_BINARY_EVIDENCE=replace` lets the binary stream stand in for the JSONL
rather than accompany it. Keeping both is right while the format is under
review, because the JSONL is what the binary stream is differentially validated
against; it was never the shipped configuration. The mode is explicit rather
than inferred, because it changes which artefacts a run produces and an
analyzer expecting venue JSONL should not find it silently absent.

| arm | median | disk |
| --- | ---: | ---: |
| JSON | 10.21-10.30 s | 594,722,408 |
| binary, replace | **8.21 s** | **272,576,389** |
| binary, replace + zstd | 8.63 s | **124,174,797** |

**-19.95 % against an A/A of +0.96 %**, and disk falls 2.18x, or **4.79x** with
zstd for a further 2.4 % wall.

This is a larger gain than the -15.84 % measured with logging off, and for a
reason worth stating: with logging off the binary format only replaces the
encode; with logging on it replaces the encode *and* the write. The headline
number and the useful number were never the same number, and the useful one is
bigger.

### What this corrects

The `-15.84 %` figure stands for what it measured and is now the *secondary*
result. The primary result is `-19.95 %` under full logging, which is the
regime the research cells actually run in. Any earlier statement that implied
the campaign's speedup applied to registered runs was measuring the wrong
configuration.

## The bottleneck moved: a profile of the replace-mode build

Every profile in this campaign until now was taken on the JSON evidence path.
After the format change, the ranking is different, and the difference is the
point: **the evidence subsystem is no longer the largest cost, and no
application function is.** dev-607, seed 607, 20 simulated minutes, replace mode:

| | flat | cum |
| --- | ---: | ---: |
| `runtime.mallocgcSmallScanNoHeader` | 1.81 % | **10.39 %** |
| `runtime.scanObject` (GC mark) | 4.83 % | 8.33 % |
| `internal/sync.(*Mutex).Lock` + `Unlock` | **7.00 %** | — |
| `sha256.blockSHANI` | 3.86 % | — |
| `aeshashbody` + `maps.ctrlGroup.matchH2` | 5.08 % | — |
| `PositionManager.PositionsAcrossSides` | 1.21 % | 5.07 % |
| `encoding/json.structEncoder` | 0.97 % | 4.95 % |

**Allocation and GC together are now the dominant theme**, and the profile says
where the objects come from. Attribution by allocation site — sampling is sound
for *which site*, though not for deltas:

| site | flat | cum |
| --- | ---: | ---: |
| `actor.(*BaseActor).decodeMarketData` | 9.08 % | 9.08 % |
| `exchange.cloneBookForPreviewBounded` | 4.62 % | **11.67 %** |
| `marketdata.(*MDPublisher).publish` | 4.00 % | **11.28 %** |
| `simulation.(*DelayedGateway).schedulePhaseMarketData` | 3.88 % | **11.13 %** |
| `simulation.(*EventScheduler).Schedule` | 6.02 % | 6.02 % |
| `actor.(*BaseActor).decodeResponse` | 4.50 % | 6.69 % |
| `multivenue.(*binaryEvidence).record` | 3.35 % | 5.61 % |
| `math/big.nat.make` | 2.46 % | 2.46 % |

**The dominant allocation theme is market-data fan-out, not evidence.** Publish,
schedule, decode and the preview clone are the top of the list; the binary sink
is 5.61 %, below all of them.

### Three targets this opens, and what each is worth

1. **Market-data fan-out.** `publish` → `schedulePhaseMarketData` →
   `decodeMarketData` is one pipeline, and each stage allocates. Every venue
   fans a snapshot to every subscribed actor, which decodes it into fresh
   objects. This is the largest allocation theme in the simulator and it has
   never been attacked.

2. **Mutex traffic at 7.00 % flat.** Traced to the deterministic-phase
   machinery — `DrainIngress`, `PlaceOrder`, the `DelayedGateway` pumps — not to
   the evidence sink. It is worth stating plainly that **the earlier estimate in
   this campaign was wrong by an order of magnitude**: the sink's two mutex
   pairs were priced at ~0.7 % and dismissed, and mutex operations are 7 % of
   CPU. They are not the sink's. Whether they are removable is a real question,
   because the phase barriers already serialise execution — GOMAXPROCS
   invariance is evidence that ordering does not depend on this locking.

3. **`math/big` inside an actor tick.** `dated_term_carry.go` allocates
   `big.Int` per call from `processTick`. **Do not replace it with int64**: it is
   there for exactness in carry economics, and swapping it would trade a silent
   correctness risk for 2.46 % of allocations. Reusing the values instead of
   `new(big.Int)` per call preserves the arithmetic exactly and removes the
   allocations.

### What this means for the campaign's conclusion

The earlier finding stands — matching is 0.45 % of CPU and a native-language
rewrite buys nothing. But "the remaining profile is diffuse" was **wrong**. It
was diffuse on the JSON path, where evidence dominated everything. With evidence
removed, a clear structure appears: allocation and GC, fed mostly by market-data
fan-out. That is a new structural bottleneck and it is the honest answer to
whether there is more to do here: **there is, and it is not in serialization,
not in matching, and not in any language choice.**

### GC tuning is a dead end: collecting less is slower

Before attacking allocation, the cheap probe: if GC mark is 8.33 % of CPU, how
much of it can be bought back by collecting less? `GOGC` answers that without
writing any code, and the answer is none.

| | wall | RSS | execution hash |
| --- | ---: | ---: | --- |
| `GOGC=100` (default) | **7.47 s** | 644,884 KB | `e1ad48f5f35e0f12` |
| `GOGC=400` | 7.65 s | 1,643,604 KB | `e1ad48f5f35e0f12` |
| `GOGC=off` | **8.44 s** | 4,003,652 KB | `e1ad48f5f35e0f12` |

**Turning the collector off costs 13 % wall and 6.2x the RSS.** The mark cost is
not recoverable by marking less: with a multi-gigabyte heap, allocation touches
progressively colder memory and the locality loss exceeds everything the
collector was charging. `GOGC=400` is already the wrong direction.

The conclusion is sharper than "reduce allocations": **the cost is in allocating,
not in collecting**, so the only lever is not creating the objects. Any future
proposal to tune the collector on this workload is answered here.

The execution hash is identical at every setting, which is worth recording on its
own: the collector is not part of trajectory identity, unlike the
microarchitecture level.

## -5.00 %: the market-data fingerprint, found by the new profile

The replace-mode profile put `simulation.(*DelayedGateway).scheduleMarketDataReceipt`
at **8.21 % of CPU**, and `types.MarketDataFingerprint` — reached 100 % from it —
at **3.16 %**, all of it inside `encoding/json.Marshal`.

The function marshals a five-field envelope with an `any` payload to JSON purely
to feed SHA-256, and keeps 16 bytes of the digest. It runs **once per market-data
delivery per participant**, so the venue fan-out multiplies it. It is the same
marshal-to-hash pattern the binary evidence format removed, in a subsystem that
was never in this campaign's scope.

**The fix is byte-identity, not a new format.** A payload may implement
`CanonicalJSONAppender` and render exactly the bytes `encoding/json` would; the
envelope is assembled by hand around it; and the digest is taken over the same
bytes as before. Nothing observable changes, so no re-baselining and no
scientific decision is involved.

The fast path **declines** rather than guesses: a payload with no canonical
encoder, or a symbol containing anything Go would escape (`<`, `>`, `&`, quotes,
backslashes, control bytes, non-ASCII) takes the reflection path. Refusing is
always safe; producing subtly different bytes silently changes an identity that
feeds delivery evidence and decision attestations.

| | |
| --- | ---: |
| wall clock | **-5.00 %** |
| A/A control, same session | -0.25 % |
| execution stream hash | **unchanged**, `e1ad48f5f35e0f12` |
| `market-data-receipts-v2.bin` | byte-identical |
| `market-data-decisions-v2.bin` | byte-identical |
| `market-data-evidence-v2.json` | byte-identical |
| `evidence-artifact-hash.json` | byte-identical |

Verified by differential test over every payload type with `MinInt64`/`MaxInt64`
/`MaxUint64` edges, nil versus empty slices, and symbols requiring escaping;
plus the fallback path proven correct for an unknown payload type, which is what
lets a user add a market-data type without touching this package.

**This is the largest single win in the campaign after the format change itself,
and it is in core exchange logic rather than in evidence writing.** It exists
because the profile was retaken after the bottleneck moved. Every earlier
profile was dominated by evidence serialization, which hid it.

### Profile after the fingerprint win: what is left, and why each is left

Retaken on the current build, which is the discipline this campaign learned late:
a change that moves 5 % of the workload invalidates the ranking that motivated it.

| target | cum | status |
| --- | ---: | --- |
| `schedulePhaseMarketData` | 7.62 % | near floor — receipt path is now 3.75 %, half of it SHA-256 |
| `buildAccountMarginProfile` | 6.99 % | **blocked**, see below |
| `previewMatchExcluding` | 6.24 % | previously rejected: a clone-free preview is a second implementation of matcher traversal |
| `checkpointSink.observe` | 5.99 % | the evidence sink, already reduced twice |
| `updateAllPerpPrices` | 5.62 % | not probed |
| map probing (`matchH2` + `aeshashbody`) | ~8.6 % flat | diffuse: largest single consumer is `OrderBook.FindOrder` at 2.75 %, everything else under 1 % |

**The fingerprint path is now near its floor.** `encoding/json` is gone from it
entirely; what remains is 50 % SHA-256 and 42 % the hand-written encoder.
Shrinking the digest would buy about 1.5 % and would change the fingerprint,
which is an identity in delivery evidence — declined on the same grounds as
changing the stream digest.

**`buildAccountMarginProfile` is the one with a real prize and it is blocked.**
It walks every book symbol for one client, which is the shape both of this
campaign's structural wins had, and a no-op census is the obvious next probe.
The campaign already rejected the sparse version once, recorded as "blocked by
market-logic finding F1 — optimizing through it would have silently repaired a
scientific defect while claiming a performance result".

F1 has since been fixed — on the **scientific** branch (`26fbe7d`). It is not on
this one:

```
scientific branch  anyHeld occurrences: 3
perf branch        anyHeld occurrences: 0
```

So the block stands here for the original reason. Making the loop skip symbols
the client has no position in changes *when marks are resolved*, and on a tree
without F1 that is the semantic repair, not the optimisation. Unblocking is a
branch decision: port the scientific commit first, or do this work on the
scientific branch after the freeze.

### Three allocation hypotheses, three failures, and the pattern they make

The profile says allocation and GC are the dominant theme. Three plausible ways
to act on that were tested and all three lost:

| hypothesis | expectation | measured |
| --- | --- | ---: |
| collect less (`GOGC=400`) | fewer marks, faster | **+2.4 %** |
| collect never (`GOGC=off`) | no marks at all | **+13 %**, 6.2x RSS |
| stop pre-sizing bounded preview maps | a bounded clone copies ~10 orders, not the whole side, so sizing its maps for the source wastes buckets | **+2.27 %** (A/A +0.02 %) |

The last is the most instructive. `cloneBookForPreviewBounded` sizes its maps to
`len(source.Orders)` while a bounded clone copies a price-ordered prefix — the
code's own comment says the median preview copies ten orders. Allocating buckets
for hundreds to hold ten looks obviously wasteful. It is not: growing a map from
empty pays rehashing on every growth step, and that costs more than the wasted
buckets. **One oversized allocation beats several correctly sized ones.**

**The pattern across all three:** re-shaping allocation loses on this workload,
whether by collecting less, deferring collection, or right-sizing. Every
allocation-related win in this campaign came instead from **removing the work
entirely** — deleting the reflection, deleting the JSON, deleting the second
encode — not from making the same work allocate more cleverly.

That is the rule to carry forward: on this simulator, do not try to allocate
better. Find the allocation that does not need to exist.

### The fingerprint fast path, fuzzed rather than sampled

A fixed corpus proves the cases its author thought of, which is the weakest
possible evidence for a byte-identity claim: the whole risk is inputs the author
did not imagine. The property is asymmetric and that is what makes it fuzzable —
**declining is always safe, accepting and differing is a silent corruption** of
an identity that appears in delivery evidence and decision attestations.

`FuzzFingerprintFastPathMatchesReflection` generates symbols, sequence numbers,
timestamps, prices, quantities, sides and `MDType` values, builds each of the
four payload shapes including one with no canonical encoder, and asserts only
this: **if the fast path accepts an input, its bytes equal `encoding/json`'s.**

**24.2 million executions across two runs, zero mismatches**, 130 inputs
retained as interesting. That covers `MinInt64`/`MaxInt64`/`MaxUint64` edges,
`MDType` values outside the three encoded types, symbols requiring HTML and
unicode escaping, and payloads with no encoder — all of which the fast path
declines rather than guesses at.

**Correction to an earlier claim in this file's history:** the commit that added
this said the interesting corpus is committed and becomes a regression test. It
is not. Go persists only *failing* inputs to `testdata/`, and there were none;
the 130 interesting inputs live in the build cache and vanish with it. The
fuzzer must therefore be re-run to have value — it is not a regression test that
runs with `go test`.

What does run every time is the seeded corpus in `f.Add`, plus the fixed-corpus
differential test beside it. Those cover the cases worth pinning: the integer
extremes, nil versus empty slices, escaped symbols and an unencodable payload.
