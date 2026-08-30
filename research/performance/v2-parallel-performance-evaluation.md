# V2 parallel performance evaluation

Status: **analyzer line complete and measured; simulator line measured and
recorded separately in `v2-simulator-performance.md`.** Nothing here changes economic behavior, actor policy, RNG
consumption, scheduler ordering, matching, admission, fees, margin, settlement,
funding, instrument lifecycle, experiment configs, thresholds, or evidence
meaning. Every accepted change is held to byte-identical output.

## 1. Base commit and isolation

| Field | Value |
| --- | --- |
| Scientific HEAD at start | `887899ff05f10dc6fd43d8cd8e88d52d5817c3b3` (`feat: add lossless long-run evidence archiving`) |
| Performance branch | `autoresearch/v2-performance-research` |
| Worktree | `/home/vlad/development/exchange_simulation-perf` |
| Helper branches | `perf/thread-sim` (simulator baseline), `perf/thread-pgo`, `perf/thread-arch` |

The active scientific worktree was never modified, reset, or checked out.
Nothing was merged into `autoresearch/ffa-ecology-gen0`.

`research/artifacts/v2-7-p7d/holdout` is the only registered holdout directory
in the tree. It was not read, listed, run, or benchmarked against. All
measurements use retained development evidence under `scratch/` — the same
`signed-price-hardening-20260824` cell the previous performance gate used — or
freshly generated benchmark runs with a non-scientific seed writing only to
throwaway directories outside the repository.

## 2. Machine and toolchain

| Field | Value |
| --- | --- |
| Host | 16 CPUs, 62 GB RAM, Linux 7.1.10 |
| Go | `go1.26.7-X:nodwarf5 linux/amd64` |
| C++ | GCC 16.2.1, AVX2 + AVX-512 (VBMI) available |
| External Go dependencies | none, before and after |

### Benchmark discipline

Three research threads shared one host, and the first timing batches of all
three were contaminated — load average reached 11-13, and one sample came in 70%
above its own minimum. A machine-wide mutex (a flock-based helper on a
session-scoped lock file) now serializes timed measurements; builds, correctness
differentials and profile collection run unlocked.

All numbers below are medians of alternating A/B repetitions with ranges quoted,
warm page cache, fixed `GOMAXPROCS`, and the lock held. Where a sample is
visibly contaminated it is called out rather than averaged away. Simulator
timings are pinned to one core with `taskset`, which makes them
contention-immune.

## 3. Workloads

**Analyzer.** Retained development cell
`scratch/signed-price-hardening-20260824/full-evidence-seed101`: 15 JSONL files,
684,601,507 bytes, 2,126,782 records, 3 venues, spot + derivatives. This is the
same specimen the previous gate and the union-cache evaluation used, so results
are directly comparable to them.

The registered metric set is the 31 derived artifacts driven by
`scripts/extract-v2-integrated-longrun-cell.sh`. On this development cell 5 of
the 31 are inactive because the corresponding recorders were not enabled, so
26 produce artifacts and 19 of those are scan-based. Benchmarks use the 19
scan-based metrics; differentials use all 31.

**Simulator.** `research/configs/v2-integrated-longrun/dev-607.json`, benchmark
seed **900101** (non-scientific; overrides the registered seed 607), 15
simulated minutes, `GOMAXPROCS=1`, output to temporary directories only.

## 4. Baseline

### 4.1 The dominant offline cost is re-reading the same evidence

`MVANALYZE_SCAN_STATS=1` instrumentation counts the physical work of an
extraction. For the 31-metric registered set on the 684MB cell:

| Measure | Value |
| --- | ---: |
| Raw evidence | 0.685 GB |
| Bytes read by extraction | **18.00 GB** |
| Read amplification | **26.3x** |
| Physical scan calls | 30 |
| File opens | 381 |
| Lines scanned | 55.95 M |
| Lines discarded by the prefilter | 46.31 M (83%) |
| Envelope decodes | 9.64 M |
| Events delivered to a reducer | 9.35 M |
| Wall / CPU (26 active metrics) | 90.5 s / 157.9 s |

Extrapolated to a 24-hour cell (~48x this 30-minute specimen, ~33 GB raw), the
same architecture would read roughly **860 GB**.

### 4.2 Where analyzer CPU went

`orderlifecycle`, the single most expensive metric, at 15.52 s of samples:

| Component | CPU | Share |
| --- | ---: | ---: |
| `visit` (the metric's own reducer) | 5.76 s | 37.1% |
| envelope `json.Unmarshal` | 2.42 s | 15.6% |
| data-layer `json.Unmarshal` | 1.51 s | 9.7% |
| **speculative nested-payload decode** | 1.44 s | 9.3% |
| prefilter `containsAny` | 1.10 s | 7.1% |

Of the reducer's own 5.76 s, `decodeRequiredJSON` was 5.45 s — 35% of the whole
metric — and it decoded each payload **twice**: once into the target struct, and
again into a `map[string]json.RawMessage` purely to test field presence.

Allocation was even more lopsided. Of 3,208.97 MB sampled:

| Source | Alloc space | Share |
| --- | ---: | ---: |
| `reflect.mapassign_faststr0` (the presence-check map) | 1,356.28 MB | 42.3% |
| `json.(*RawMessage).UnmarshalJSON` (value copies) | 516.12 MB | 16.1% |
| `decodeRequiredJSON` total (cumulative) | 1,977.81 MB | **61.6%** |

Every byte of it is allocated inside `encoding/json` and `reflect`.

### 4.3 Simulator baseline

From `research/performance/v2-simulator-baseline.md` on `perf/thread-sim`
(commit `dcd159b`), 15 simulated minutes at `GOMAXPROCS=1`. Two results invert
the natural reading of the previous gate document:

* **Matching is not a cost centre.** Live `Matcher.Match` is 0.03 s of 23.36 s
  (0.13%). The 39% attributed to `PlaceOrder` is admission bookkeeping,
  settlement accounting and evidence emission.
* **The ordered execution hash dominates the logs-off cost.** With
  `log_mode=none`, `checkpointSink.observe` is still 16.2% of CPU, because it
  marshals every payload a second time purely to hash it.
* **The logger is not I/O bound.** A 5-minute full run writing 117.8 MB of
  venue JSONL issued 1,844 write syscalls totalling 8.12 ms of kernel time. The
  cost is `Marshal` plus allocation, not syscalls, locking or disk.

That baseline also corrects a methodology error in
`research/v2-8-profiling.md`: `dev-607-none.json` disables *both* raw JSON
persistence and the V2 decision recorders, so comparing `full` against it does
not isolate raw JSON logging, which is what the earlier gate reported it as.

Its own wall/CPU table was taken under load 5-10 and is flagged in that document
as carrying unknown one-sided inflation. The CPU and allocation *shares* above
are internally normalized fractions of a single process's own samples and are
not affected by host load.

## 5. Accepted optimizations

All four analyzer changes were validated together by a differential harness that
runs every one of the 31 registered metrics under the reference and candidate
binaries over the same cell and requires identical stdout bytes, identical
stderr bytes and identical exit status. **All 31 matched for every change.**

### A1 — Decode each payload once (`f06a9eb`)

Two decodes per visited record answered questions the raw bytes already answer.

The derivative unwrap decoded every payload in full to discover whether it nests
a second payload, discarding the result for the majority that do not. It now
decodes only when the raw payload could possibly contain a nested `"payload"`
key. The test is a deliberate superset — a literal key, or any backslash, since
a backslash is the only way an escaped spelling such as `"payload"` could
hide one. A false positive costs one decode; a false negative is impossible.

`decodeRequiredJSON`'s second decode into a map was replaced by a single-pass
scanner over the already validated bytes. It reproduces last-key-wins for
duplicate keys, null-as-absent, and required-name ordering, and **declines to
decide** for a non-object or an escaped key, in which case the original decode
runs and supplies its exact error text. That fallback is what preserves failure
semantics, which is the surface that killed two earlier candidates in this
repository.

Differential tests hold the scanner to the original map decode over 200,000
randomized objects spanning duplicate keys, null, whitespace, nested containers,
string escapes, braces inside strings and integer boundaries; and hold the
nesting prefilter to the original unwrap over 100,000 randomized payloads.

Effect (4 alternating reps, `GOMAXPROCS=6`, six heaviest metrics): wall 53.16 s
to 43.17 s (**-18.8%**), CPU 73.44 s to 56.03 s (**-23.7%**). `orderlifecycle`
-42.6% CPU. Allocation on `orderlifecycle` fell from 3,208.97 MB to 1,242.37 MB
(**-61.3%**), with `reflect.mapassign_faststr0` eliminated entirely. `positions`
moved +4% CPU, within this host's noise.

### A2 — Share one evidence pass across independent metrics (`041e31e`)

This is the change that addresses the 26.3x amplification.

`Run.RunFused` serves several metrics from shared physical passes. It shares the
record envelope and **nothing else**: each task still runs its own reducer over
its own immutable `Event` and reaches its own verdict, so no task can observe,
and therefore cannot borrow, another's derived state. Independent reconstruction
is the property that lets these gates catch a bug, and it is preserved exactly.

Per-metric semantics are reproduced rather than approximated:

* a metric receives a record if and only if its own event filter admits it, and
  its own raw prefilter still runs before delivery;
* a metric that asked for one scan worker still sees the whole run from one
  goroutine in file order. **22 analysis files request `Workers: 1`**, relying on
  it for unsynchronized reducer state and for cross-file causal order. Such
  metrics are fused with each other but never with the parallel group, and the
  two groups run concurrently. An early version of this change ignored the
  request and crashed with `concurrent map writes` inside
  `MeasurePostOnlyActivity` — the constraint is real and load-bearing;
* `Ordinal` remains the physical record position, counting filtered records;
* a malformed record fails exactly the metrics whose own prefilter admits it,
  with the same error text, and leaves the others untouched.

`mvanalyze` gains `-fused-out` / `-fused-set` / `-fused-workers` as a research
mode. It reuses the same flag variables as the single-metric switch, so the two
paths cannot diverge on configuration. The registered extraction script is
unchanged.

Equivalence: all 19 fused artifacts are byte-identical to the single-metric
outputs, and the fused pass delivers exactly **7,667,318** visits — the same
count as the separate passes. New tests compare fused and unfused delivery
record by record and cover parse-failure isolation, serial ordering and the
nested-payload unwrap. `go test -race ./analysis` passes.

Physical work, 19 scan-based metrics:

| Measure | separate | fused | change |
| --- | ---: | ---: | ---: |
| Physical passes | 27 | 5 | -81% |
| File opens | 336 | 75 | -78% |
| Bytes read | 15.95 GB | 3.42 GB | -78.5% |
| Lines scanned | 49.57 M | 10.63 M | -78.5% |
| Envelope decodes | 7.96 M | 4.37 M | -45% |
| Events delivered | 7.67 M | 7.67 M | identical |

### A3 — Standard library substring search in the prefilter (`8b00ca1`)

`bytes.Contains` computes the same predicate as the hand-rolled search with
assembly `IndexByte` and a Rabin-Karp fallback.

`research/v2-8-profiling.md` previously measured this swap at **1.11x** and
rejected it as too small to justify. **That measurement understated it.** Over a
101 MiB retained derivatives corpus with the twelve registered event needles,
five repetitions at `GOMAXPROCS=1` give median 170.7 MB/s hand-rolled against
339.6 MB/s standard library — **2.0x**. A test requires both searches to agree
on every line of the corpus, with the previous implementation retained inside
the test as the reference.

### A4 — Cache the deterministic gateway iteration order (`3a32be5`)

Both deterministic drains built a sorted client-ID slice and a parallel
client-to-gateway map on every call, and the ingress drain does this once per
pass while its passes repeat until one is empty — 1.51 s of 23.36 s CPU and
511.4 MB of allocation, of which 391.85 MB was the redundant lookup map.

One sorted slice of client/gateway pairs replaces both structures, and a
generation counter bumped by the only two mutation sites — both already under
`e.mu` — invalidates it. Iteration order is unchanged: ascending client ID.

## 6. End-to-end analyzer result

Pristine one-process-per-metric extraction against the optimized fused
extraction, same 19 scan-based metrics, same 684MB cell, three alternating
repetitions, `GOMAXPROCS=6`, benchmark lock held:

| Measure | pristine | optimized | change | ranges |
| --- | ---: | ---: | ---: | --- |
| Wall | 72.89 s | **26.80 s** | **-63.2%** | 72.52-73.69 \| 26.73-26.93 |
| CPU | 144.80 s | **62.46 s** | **-56.9%** | 144.24-147.10 \| 61.71-62.57 |
| Peak RSS | 103 MB | 219 MB | +112.7% | 102.88-103.26 \| 212.26-219.01 |

**2.72x wall, 2.32x CPU.** The RSS increase is bounded and small: no event is
retained. For contrast, the previously rejected full retained-event union cache
reached 1,137 MB (+11x) for a smaller wall gain and *no* CPU gain at all.

Extrapolated to a 24-hour cell, the read amplification alone falls from roughly
860 GB to about 165 GB.

## 6b. Correction: fused extraction memory scales with evidence size

The A2 measurement and its commit message described the fused path's peak RSS as
"bounded; no event is retained", quoting +112% against the separate path on the
684MB development cell. **The multiplier is roughly right; calling it bounded was
wrong.** Validated on a retained 10.89 GB development cell, 15.9x larger:

| Approach | wall | peak RSS |
| --- | ---: | ---: |
| separate, 14 processes | 463.33 s | 1.19 GB |
| fused, all 14 metrics | 286.21 s | **2.19 GB** |
| batched 3 + 11 | 309.04 s | 1.96 GB |
| **split-heavy, 1 + 13** | **293.67 s** | **1.24 GB** |

Fused peak RSS went 219 MB to 2.19 GB for a 15.9x larger input — essentially
linear. No event is retained, and that part of the claim holds; what scales is
the *reducers'* accumulated state, which grows with the number of distinct
orders and contracts in the evidence. Running them concurrently makes peak RSS
the **sum** of the concurrent states where the separate path pays only the
**maximum**.

The distribution is very skewed: on this cell `orderlifecycle` alone peaks at
1.19 GB while eleven of the fourteen metrics peak under 131 MB. So the fix is
not to abandon fusion but to keep the one dominant reducer out of the shared
group. Splitting `orderlifecycle` into its own invocation recovers nearly all
the speed — 293.67 s against 286.21 s for full fusion, still **-36.6% against
the separate path** — while holding peak RSS to 1.24 GB, within 4% of what the
separate path already needs.

Grouping does not affect results: with `orderlifecycle` split out, all 19
artifacts remain byte-identical to the single-metric reference.

Two further corrections this scaling test forces:

* The **2.72x** analyzer speedup is a 684MB-cell figure. At 10.89 GB the same
  code is **1.62x**, because per-metric reducer work — which fusion does not
  reduce — grows with evidence while the shared envelope work it does reduce is
  amortized across more consumers. The larger number should not be quoted for a
  24-hour cell.
* Extrapolating to a ~33 GB 24-hour cell, expect roughly 3.6 GB peak RSS on the
  separate path and roughly 3.8 GB with the split-heavy grouping. Full fusion
  would be about 6.6 GB, which is affordable on this host but is a real cost
  rather than a rounding error.

**Recommendation for integration**: adopt the fused path with the dominant
reducer split into its own invocation, and re-measure the split on the actual
target cell rather than assuming this cell's skew, since which reducer dominates
depends on the composition.

## 7. Simulator results

Recorded in full in `v2-simulator-performance.md`. Headline: eight accepted
changes take the integrated V2 workload from 50.6 to 79.5 simulated seconds per
wall-clock second at `GOMAXPROCS=1` (**1.57x**, wall -36.4%), and the simulator
turns out to be GOMAXPROCS-invariant — `GOMAXPROCS=4` adds a further -14.3% wall
with byte-identical evidence over twelve verification runs, reaching **88.8
simulated seconds per wall second, 1.75x overall**, with no code change.

### Earlier partial results, superseded

| Change | log mode | wall | CPU | peak RSS |
| --- | --- | ---: | ---: | ---: |
| A4 gateway-order cache | full | -5.3% | -4.9% | -2.3% |
| A4 gateway-order cache | none | (contaminated) | **-16.0%** | -1.6% |

The none-mode wall range (13.24-27.56 s) contains one sample contaminated by a
concurrent test run of my own; the CPU ranges are tight (13.10-14.33 against
11.16-12.00) and the CPU median is the reportable figure. The drain is a larger
share of total work with logging off, which is why the effect is larger there.

### Determinism oracle

Every simulator change is held to this, on `dev-607.json`, seed 900101, 15
simulated minutes, `GOMAXPROCS=1`:

| Oracle | Value |
| --- | --- |
| Ordered execution stream hash | `51541f91db7c5eae8688235d3961a76af421ab782f05ab62649076cf90aef332` |
| Execution events | 1,163,127 |
| Evidence artifact digest | `7a869b49546a60cba0f5a31f7cbc8236f331d45e73404248096ebf05812739f0` |
| Persisted records | 1,185,184 |
| Raw venue JSONL bytes | `ad39919d2f489bf7f8c28420756fa154…` |
| Evidence tree | 27 files, 442,225,951 bytes, `diff -rq` clean |

The hash matches the value `perf/thread-sim` obtained across all 30 of its
baseline runs, spanning both log modes, two filesystems, `GOMAXPROCS` 1 and 4,
and profiled and unprofiled builds.

## 8. Languages and runtimes investigated

### Go arenas — rejected

`GOEXPERIMENT=arenas` still compiles and runs on Go 1.26.7. It is rejected on
two independent grounds.

First, **arenas cannot reach this program's allocation**. An arena only captures
what your own code explicitly allocates into it. 100% of the analyzer's
allocation is inside `encoding/json` and `reflect` (§4.2), which use the normal
heap. An arena would help only after replacing `encoding/json` outright — the
change already rejected twice in this repository on semantic grounds.

Second, it changes recorded provenance: the binary stamps as
`go1.26.7-X:arenas` and records `GOEXPERIMENT=arenas` in its build info, which
is exactly the surface the r5 contract checks.

### C++23 / SIMD / cgo — measured ceiling, not adopted

The prefilter is the only hot loop coarse enough to consider moving. A C++23
`-O3 -march=native` prototype over the same 101 MiB corpus and the same twelve
needles:

| Implementation | Throughput |
| --- | ---: |
| Go hand-rolled (before A3) | 170.7 MB/s |
| Go `bytes.Contains` (after A3) | 339.6 MB/s |
| C++ scalar naive | 222.9 MiB/s |
| C++ `memmem` | 946.3 MiB/s |
| C++ AVX2 first/second-byte Teddy filter | **1459.3 MiB/s** |

So there is genuinely about 4.3x of headroom beyond the adopted Go version, and
a cgo boundary at whole-file granularity would amortize the crossing cost fine.
It is still not worth taking: after A2 the union prefilter runs over 10.63 M
lines instead of 49.57 M, so the prefilter is a single-digit percentage of
analyzer CPU. Spending a cgo dependency, a build-toolchain requirement and a new
failure surface to win a few percent of one of two workloads is a bad trade.
The prototype is retained as the ceiling measurement.

Assembly was not written: the two hot byte loops are `bytes.Contains` and
`bytes.IndexByte`, which are already hand-written assembly in the standard
library. Reimplementing them would be reinventing stdlib.

### Third-party JSON — not revisited

`research/v2-8-profiling.md` already screened `goccy/go-json` (**rejected**: it
accepts an overflowing `int64` that the standard library rejects), `jsoniter`
(2.3x) and `sonic` (3.6x). The measurements in this document supersede the
motivation rather than the screen: A1 and A2 removed most of the decode work
without touching the parser, and `encoding/json` remains the semantic
reference. No Go dependency was added; the module still has none.

## 9. Rejected and superseded hypotheses

| Hypothesis | Verdict | Why |
| --- | --- | --- |
| Retain all decoded events in a union cache | rejected (prior art, confirmed) | 11x RSS, no CPU reduction. `v2-8-analyzer-union-cache-evaluation.md` |
| Fuse the envelope and data-layer decodes into one struct | rejected (prior art, confirmed) | 21.8% faster but diverges on duplicate `data` keys and on error classification. `v2-8-analyzer-fused-data-layer-evaluation.md` |
| `bytes.Contains` is too small to matter | **superseded** | Re-measured at 2.0x, not 1.11x. Adopted as A3 |
| Optimize the matching engine | rejected | `Matcher.Match` is 0.13% of simulator CPU |
| Go arenas | rejected | Cannot reach `encoding/json` allocation; changes build provenance |
| Move the prefilter to C++/cgo | rejected | Real 4.3x headroom, but on a single-digit share of CPU after A2 |
| Hand-written assembly for byte scanning | rejected | The hot loops already are stdlib assembly |
| Replace `encoding/json` with jsoniter or sonic | not adopted | Semantic screen unfinished; A1/A2 removed the motivation |
| Pool the detached preview book | not recommended | `perf/thread-sim` judged the matcher's ownership of queue links and iceberg state too risky for the 3.6% CPU at stake |

## 10. Open work

* **C2, single-marshal of logged payloads.** Implemented and proven exact on
  every oracle in §7 (identical execution hash, evidence digest, raw bytes and
  442 MB tree). Timing in flight. Predicted ~11% simulator CPU in `full` mode
  and none in `none` mode, since the duplicate marshal only exists when the raw
  logger is attached.
* **C3, `map[string]any` log payloads to lexicographically ordered structs.**
  2.85 s CPU / 16.2 M objects (28.2%) in the baseline. Byte-identity depends on
  declaring fields in lexicographic order, since `encoding/json` sorts map keys;
  the codebase already sanctions the pattern in `feesim.persistedEvent`. Not
  started.
* **Production integration of A2.** The fused driver currently duplicates ~19
  metric call expressions rather than refactoring `cmd/mvanalyze`'s 1,400-line
  metric switch into a table. Byte-equality is proven, but a production version
  should do that refactor so the two paths cannot drift.
* **PGO.** `perf/thread-pgo` proved simulator PGO determinism (identical
  execution hash, byte-identical 275 MB evidence tree) and measured binary size
  cost (+1.78% analyzer, +3.23% simulator) before its session ended. Its timing
  batch was discarded as contaminated and was never re-run, so **there is no
  PGO speedup measurement**.
* **Archive and compression.** `perf/thread-arch` produced nothing before its
  session ended. Unmeasured.

## 11. Escalation — not a performance finding

`perf/thread-pgo` reported that `scripts/v2-integrated-longrun-r5-contract.sh`
fails closed unless binaries report **go1.27**, while the host default toolchain
is **go1.26.7-X:nodwarf5**. If correct, a long-run cell launched with the default
`go` would be rejected by its own gate. The exact contract check was not quoted
before that session ended, so this is **reported as unverified** and is for the
scientific owner to confirm. No contract script was modified.

## 12. Commit classification

Against the scientific HEAD this work is based on, `887899f`. Eighteen commits on
`autoresearch/v2-performance-research`, none merged anywhere.

### Safe to cherry-pick — analyzer only, all 31 metrics byte-identical

| Commit | Subject |
| --- | --- |
| `7a44f3f` | `perf: add analyzer scan-cost instrumentation` |
| `f06a9eb` | `perf: decode evidence payloads once per record` |
| `8b00ca1` | `perf: use the standard library substring search in the prefilter` |
| `41b391b` | `perf: split the evidence data layer without revalidating it` |

### Safe to cherry-pick — documentation only

| Commit | Subject |
| --- | --- |
| `07cede0` | `docs: record V2 parallel performance evaluation and SIMD ceiling` |
| `85cb7c2` | `docs: record V2 simulator performance study` |
| `08cc417` | `fix: keep system temporary paths out of the performance evaluation` |
| `d3d8e99` | `docs: record the accepted simulator optimizations and their measurements` |
| `7dd37ca` | `docs: finalize the analyzer result and classify every commit` |
| `bdae26b` | `docs: record the fan-out cache, the lock-free clock, and GOMAXPROCS invariance` |

### Require scientific review — simulator, all oracles identical on three seeds and both log modes

| Commit | Subject | Why review |
| --- | --- | --- |
| `3a32be5` | `perf: cache the deterministic gateway iteration order` | touches deterministic ingress and egress ordering, which is an economic input |
| `eaf6c85` | `perf: build high-volume evidence payloads as ordered structs` | field order is now the evidence contract; a future reorder silently changes persisted bytes |
| `087dd7a` | `perf: persist evidence without marshalling each payload twice` | assembles evidence records without the reflective encoder; largest single gain and largest surface |
| `0ed5180` | `perf: stop reslicing the deterministic phase queues from the front` | changes market-data queue internals |
| `5d1f0d3` | `perf: resolve each client's position map once per lookup` | no measurable gain; take it for clarity or leave it |
| `6725639` | `perf: cache the canonical book and client iteration orders` | caches risk-sweep iteration order; the first version had a missed invalidation that the existing suite caught |
| `26998ae` | `perf: cache each symbol's market-data fan-out order` | fan-out order decides which subscriber reacts first, so it is an economic input; the cache preserves it exactly and the package gained its first tests |
| `5fb87e2` | `perf: read the simulated clock without taking a lock` | replaces a read lock with an atomic load on the most frequently read value in the simulator |

### Require scientific review — new analyzer architecture

| Commit | Subject | Why review |
| --- | --- | --- |
| `041e31e` + `39e3007` | `perf: share one evidence pass across independent metrics` and `refactor: define each shared metric's options once` | adds a second extraction architecture, and must be taken together with the refactor that removes the duplicated option construction. Two conditions remain for the operator rather than the code: split the dominant reducer into its own invocation or peak RSS is the sum of concurrent reducer states rather than the maximum (§6b), and read the RSS expectations from §6b rather than from `041e31e`'s own commit message |

### Do not cherry-pick

None. The two rejected optimizations were reverted rather than committed; both
are recorded with their measurements in the rejected-hypotheses sections.

### The drift risk in `041e31e` is now closed

`041e31e` was held back because its driver duplicated the option construction
that the single-metric switch performs. That is the one way the two paths could
disagree while both still produced well-formed artifacts: a metric run one way
would answer a different question than the same metric run the other way, and
nothing in the output would say so.

`39e3007` gives each of the ten shared option sets a single definition that both
paths call. Behaviour is unchanged in both directions — all 31 registered metrics
still byte-identical through the single-metric path, all 19 fused artifacts still
byte-identical to the single-metric reference.

It also fixed a divergence the driver had introduced: it loaded the run's funding
intervals eagerly, so an unrelated read failure would have failed metrics that
never consult the value, which the single-metric path does not do. They are now
loaded only when the derivative audit is selected.

Two tests keep it closed. One pins each constructor to the settings field it is
supposed to read, using a distinct value per field so a constructor reading the
wrong one yields a visibly wrong option rather than a coincidentally equal
default. The other requires the driver's metric set and the registered set to
match in both directions, so a metric added to one and not the other fails the
build instead of silently falling back to a separate process.

### Integration recommendation

Take the four analyzer commits and the documentation commits directly. Take
the simulator commits as a group after review — they were measured cumulatively
and `087dd7a` depends on the sink returning its encoding, which no earlier
commit provides. `041e31e` is the one change that should not be integrated as
is: it is worth the refactor, but that refactor is a separate piece of work.

Every recommendation is against `887899f`. If the scientific branch has moved,
revalidate on the then-current HEAD as a separate integration step; the
determinism oracle and the 31-metric differential harness are the tools for
that, and both are reusable as committed.
