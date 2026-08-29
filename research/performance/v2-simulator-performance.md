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
with `taskset`, five alternating A/B repetitions, medians reported with ranges.
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

## 3. Where simulation time goes — current HEAD

Full evidence, 14.70 s of CPU samples for 900 simulated seconds. Percentages are
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

### S1 — Cache the deterministic gateway iteration order (`3a32be5`)

Both deterministic drains rebuilt a sorted client-ID slice *and* a parallel
client-to-gateway map on every call, and the ingress drain does so once per pass
while its passes repeat until one is empty. That was 1.51 s of 23.36 s CPU and
511.4 MB of allocation in the baseline profile, of which 391.85 MB was the
redundant lookup map and 550 ms was the sort.

The gateway set changes only when a client connects or disconnects, and both
mutations already hold `e.mu`. A single sorted slice of client/gateway pairs
replaces both structures, invalidated by a generation counter bumped at those two
sites. Iteration order is unchanged: ascending client ID.

| Measure | `full` | `none` |
| --- | ---: | ---: |
| Wall | -5.3% | (one sample contaminated) |
| CPU | -4.9% | **-16.0%** |
| Peak RSS | -2.3% | -1.6% |

The drain is a larger share of total work with logging off, which is why the
effect is larger there.

### S2 — High-volume evidence payloads as ordered structs (`eaf6c85`)

Every payload is marshalled **twice** — once by the ordered-execution hash sink
and once by the raw evidence logger — so a `map[string]any` payload pays for its
map allocation and sixteen interface boxes on both. The baseline attributed
2.85 s of CPU and 16.2 M allocated objects (28.2% of all objects) to
`mapEncoder`.

The three highest-volume map payloads became structs: `OrderFill` (149,594
events per 15-minute run), `OrderCancelled` (111,393) and `BookSnapshot`
(70,581). Fields are declared in lexicographic order of their JSON names, which
is the order `encoding/json` emits for a map, so the persisted bytes are
unchanged. Each type carries a comment recording that the order is the evidence
contract.

| Measure | `full` |
| --- | ---: |
| Wall | 14.40 s to 12.98 s (**-9.9%**) |
| CPU | 14.27 s to 12.85 s (**-10.0%**) |
| Peak RSS | unchanged |

Ranges 14.33-15.15 against 12.93-13.44.

### Combined result

Pristine base against S1+S2, `log_mode=full`, five alternating pinned
repetitions:

| Measure | base | S1+S2 | change |
| --- | ---: | ---: | ---: |
| Wall | 17.24 s | **13.88 s** | **-19.5%** |
| CPU | 17.05 s | 13.71 s | -19.6% |
| Peak RSS | 760,540 KiB | 755,828 KiB | -0.6% |

Ranges 16.94-18.11 against 13.11-13.93.

**Target metric: 52.2 to 64.8 simulated seconds per wall-clock second, +24.2%,
with all four determinism oracles identical.**

## 6. Rejected optimizations

### R1 — Reuse the hash sink's encoding as the logged payload

The sink marshals every payload to hash it, then the logger marshals the same
payload again. Passing the sink's bytes to the logger as a `json.RawMessage`
looked like it should remove the second marshal outright, and the baseline
predicted about 11% of CPU.

**Measured: +1.15% CPU — a regression.** Five alternating pinned repetitions,
ranges 14.62-14.92 against 14.69-14.88, so the sign is not noise.

The mechanism does not work. `encoding/json` emits a `RawMessage` through
`compact()`, which is a full scan of the value with an HTML-escape check. That
costs about what marshalling the payload costs, so the change trades a marshal
for a compaction and gains nothing. Reverted.

The correct version of this idea is to skip the nested encoder entirely and
append the sink's bytes into a manually built envelope, since the persisted line
format is fixed and its key order is already known. That has not been built yet.

### R2 — Replace JSON evidence with a custom binary format

Bounded at 1.28x by §4, and it changes both the persisted evidence contract and
the execution-hash domain. Rejected on risk-per-gain.

### R3 — Move serialization into cgo, C++, or Go assembly

Same 1.28x bound, plus a toolchain dependency and a new failure surface. The FFI
crossing cost is genuinely negligible at 80,624 events/s (0.03% of runtime), so
the objection is not FFI overhead — it is that the addressable block is too
small to justify the boundary.

### R4 — Optimize the matching engine or the order-book representation

`Matcher.Match` is absent from the top 400 CPU symbols. There is nothing to win
here on this workload.

### R5 — Pool the detached preview book

`cloneBookForPreviewExcluding` is 13.97% of allocation and 4.5% of CPU, so it is
a real target, but the matcher mutates queue links and iceberg state on the
clone. `perf/thread-sim` judged the ownership proof too risky for the gain and
did not recommend it. Not attempted; a pooling change that introduced aliasing
would be a correctness failure, not an optimization.

## 7. Ranked next candidates

| Idea | Hotspot | Current | Proposed | Expected | Risk | Status |
| --- | --- | --- | --- | ---: | --- | --- |
| Manual envelope encoder appending the sink's payload bytes | `venueLogger.LogEvent`, 26.9% | two full marshals per event | one marshal + byte append into a fixed line format | ~8-10% | medium: touches evidence writing, but oracle-checkable | next |
| Reduce `DelayedGateway` market-data allocation | 1.24 GB, 28% of allocation | per-delivery allocation and closures | preallocated per-symbol buffers | up to ~6% via GC | medium | not started |
| Integer instrument/venue handles in hot paths | map ops 11.03% | `map[string]` lookups per event | resolve once at setup, index dense slices | up to ~8% | medium, wide diff | not started |
| Incremental margin state | `buildAccountMarginProfile` 6.3% + `CheckLiquidations` 5.9% | recomputed per sweep | cached, invalidated on position change | up to ~8% | high: risk semantics | not started |
| `GetPositionBySide` lookup | 6.6% | map lookup per call | dense per-instrument index | up to ~4% | low-medium | not started |
| Go PGO | whole binary | no profile | `default.pgo` from a representative run | unmeasured | low semantic, proven deterministic | blocked: `perf/thread-pgo` proved byte-identical output and measured binary size, but its timing batch was discarded as contaminated and never re-run |

## 8. Open escalation — not a performance finding

`perf/thread-pgo` reported that `scripts/v2-integrated-longrun-r5-contract.sh`
fails closed unless binaries report **go1.27**, while this host's default
toolchain is **go1.26.7-X:nodwarf5**. If correct, a long-run cell launched with
the default `go` would be rejected by its own gate. The exact contract check was
not quoted before that session ended, so this is recorded as **unverified** and
is for the scientific owner to confirm. No contract script was modified.
