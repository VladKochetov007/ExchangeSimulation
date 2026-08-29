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

### S6 — Resolve each client's position map once per lookup (`5d1f0d3`)

Three `PositionManager` methods indexed `pm.positions[clientID]` twice. Strictly
less work and provably identical, but **-0.34% wall with overlapping ranges**:
reported as no measurable gain. The string-keyed `positionKey` hash is the real
cost here, and replacing it with an integer instrument handle is a wider change.

### Combined result

Pristine base against S1-S6, `log_mode=full`, six alternating pinned
repetitions:

| Measure | base | optimized | change |
| --- | ---: | ---: | ---: |
| Wall | 17.475 s | **11.605 s** | **-33.6%** |
| CPU | 17.345 s | 11.505 s | -33.7% |
| Peak RSS | 755,852 KiB | 745,774 KiB | -1.3% |

Ranges 17.39-17.53 against 11.53-11.66.

> **Target metric: 51.5 to 77.6 simulated seconds per wall-clock second, a
> 1.51x speedup, with all four determinism oracles identical on three seeds and
> both log modes.**

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

### R6 — Pool the detached preview book

13.97% of allocation and 4.5% of CPU, so a real target, but the matcher mutates
queue links and iceberg state on the clone. A pooling change that introduced
aliasing would be a correctness failure, not an optimization. Not attempted.

## 7. Ranked next candidates

| Idea | Hotspot | Proposed | Expected | Risk | Status |
| --- | --- | --- | ---: | --- | --- |
| Integer instrument/venue/client handles | map ops 11.0% | resolve once at setup, index dense slices | up to ~8% | medium, wide diff | not started |
| Incremental margin state | margin 6.3% + liquidation 5.9% | cache, invalidate on position change | up to ~8% | high: risk semantics | not started |
| Reduce remaining `DelayedGateway` allocation | ~0.9 GB after S4 | checked-out scratch buffers | ~1% | medium | R2 attempt failed |
| Preview-book reuse | 14% of allocation | proven-ownership reuse | up to ~4% | high | R6 |
| Go PGO | whole binary | `default.pgo` from a representative run | unmeasured | low semantic | blocked: determinism and binary size proved on `perf/thread-pgo`, timing batch discarded as contaminated and never re-run |

## 8. Open escalation — not a performance finding

`perf/thread-pgo` reported that `scripts/v2-integrated-longrun-r5-contract.sh`
fails closed unless binaries report **go1.27**, while this host's default
toolchain is **go1.26.7-X:nodwarf5**. If correct, a long-run cell launched with
the default `go` would be rejected by its own gate. The exact contract check was
not quoted before that session ended, so this is recorded as **unverified** and
is for the scientific owner to confirm. No contract script was modified.
