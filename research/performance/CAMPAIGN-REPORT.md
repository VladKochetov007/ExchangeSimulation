# V2 performance campaign: final report

Branch `autoresearch/v2-performance-research`. Nothing here is merged into the
scientific branch and no scientific semantics were changed by any performance
commit.

## 1. Contract and verification regime

**Question.** Maximize simulated market seconds per wall-clock second on the
integrated V2 workload, subject to exact trajectory equivalence, and determine
whether JSON should be retired from the simulator's evidence hot path.

**Tier A.** A deterministic simulator with an exact acceptance oracle: identical
seed and config must reproduce an identical ordered execution-stream hash. That
oracle is stronger than a benchmark, because a change that alters behaviour
fails outright rather than scoring worse.

**Model-to-reality gap.** The oracle proves trajectory identity, not that a
change is beneficial. Every performance claim rests on wall-clock measurement,
which on this host is noisy — measured A/A controls ranged from **+0.06 % to
+5.52 %** across the campaign. That variance is the single largest threat to
every number here and is why every accepted result carries its own A/A control
measured in the same session.

**Invariants never violated.** Byte-identical execution stream hash on at least
two seeds; full test suite green; no change to the r5 evidence contract.

**Falsifier for the leading claim.** If removing serialization entirely yielded
less than a few percent, the whole VNext line was worthless. It yielded 17.29 %.

## 2. Strongest supported claims

| claim | evidence | status |
|---|---|---|
| Serialization ceiling is **17.0–17.9 %** of simulator CPU | three independent probes, each with its own A/A control | supported |
| Byte volume contributes **~0** to simulator CPU | ceiling (hash 1 B) and emulation (hash 88 B) indistinguishable | supported |
| Dictionary interning costs **~0** | third probe adds six map lookups per event, −16.96 % vs −17.93 % | supported |
| Per-frame SHA-256 was **54–69 %** of binary encode cost | encode with and without hashing | supported |
| Binary decode is **42.2x** faster than JSON | isolated, three repetitions each | supported |
| Binary is **2.84x** smaller on real evidence | 304,877 real records | supported |

### The end-to-end result, and the correction that reframes it

| claim | evidence | status |
|---|---|---|
| Binary sink is **-19.95 %** wall under `log_mode: full`, replace mode | 3 rounds, A/A +0.96 % | self-measured, not yet independently graded |
| Binary sink is **-15.84 %** wall under `log_mode: none`, discard mode | 5 rounds, A/A -0.06 % | self-measured; an earlier build graded independently at -13.2 % |
| Binary sink was **+8.18 %** — a REGRESSION — under `log_mode: full`, dual-write | 3 rounds, A/A +0.49 % | measured, and fixed |
| A real run's evidence corpus rebuilds **byte-identically** from the binary alone | 1,569,110 records rendered and diffed | supported |
| On-disk output falls **2.18x**, or **4.79x** with zstd | same config and seed, real files | supported |
| No codec can change the execution hash | four codecs, identical digest | supported |

**The correction that matters most in this campaign.** Every number published
before the last day came from `log_mode: none`. **Six of the seven registered
configs use `log_mode: full`.** In that regime the binary sink was not merely
less effective — it was *slower than the thing it replaced*, because the sink
returned no reusable bytes and the venue logger marshalled every payload a
second time, storing two complete copies of the run's evidence.

An independent reproducer predicted this in one sentence. The prediction was
recorded as a "design gap" and not tested. It was testable in twenty minutes,
and testing it turned a headline win into a regression, then — once the
dual-write was removed — into a larger win than the original claim.

The transferable lesson is not about serialization. **A speedup measured in a
configuration nobody runs is not a speedup**, and the cheapest way to find that
out is to measure the configuration people actually run, first, before
optimising anything.

| The matching engine is **0.45 %** of CPU | CPU profile | supported |
| A native-language rewrite offers nothing | encoder at MOV speed, hash already on SHA-NI, matching 0.45 % | supported |
| `bookDeltaEvidence` map→struct: **−8.46 % mallocs, −3.89 % wall** | exact `MemStats`, A/B with A/A control | supported, accepted |
| A real binary sink would deliver **~11.5 %** | ceiling x measured retention, two routes converging | projected, bounded |

## 3. Rejected, with the evidence that rejected them

Preserved so they are not rediscovered under different wording.

| candidate | why rejected |
|---|---|
| Byte-identical JSON appenders | identical mallocs, +0.93 % bytes, +0.44 % wall against +0.62 % A/A. Removed reflection and changed nothing |
| No-op sweep watermarks (`CheckExpiries`, funding, listings) | 100 %, 99.995 %, 99.97 % no-op rates, and **absent from the top 200 profile nodes**. Frequency alone ranks nothing |
| Inlining the scheduler heap's ordering key | **+1.33 %** against a +0.06 % A/A control. The pointer chase was not the cost; `Swap` tripled |
| Clone-free matching preview | would be a second implementation of matcher traversal; iceberg re-queue reaches the virtual tail within one pass |
| Parquet / columnar analytics layer | indexed binary answers all four query classes; the index is worth only 1.2x on the one class columnar would win |
| Sparse margin profile (A′) | blocked by market-logic finding F1 — optimizing through it would have silently repaired a scientific defect while claiming a performance result |

**The one unsound rejection has been re-tested, and the answer split in two.**
The `instrumentLogEvent` wrapper appender was rejected on a +3.3 % malloc
regression measured *across* the introduction of a census probe that itself
allocates on every fallback event. Re-measured in isolation, byte-identity
verified first:

| arm | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| reflection, both fixtures | 1196-1231 | 592 | 13 |
| `json.Marshal` + `MarshalJSON` on the wrapper | 2737-2793 | 1072 | 21 |
| reflection, struct fixture only | 353-398 | 160 | 2 |
| call-site appender, struct fixture only | 46.9-73.5 | 0 | 0 |

The rejection stands for the form that was tested, for a mechanism reason the
contaminated run never identified: `encoding/json` seeing a `json.Marshaler`
calls it and then **compacts the returned bytes into its own buffer**, so the
candidate pays for a second buffer and a copy that reflection never pays. It is
**2.27x slower**. Implementing `MarshalJSON` cannot win here and could not have
won.

The form that was never tested does win: **7.6x and zero allocations**, by
bypassing `encoding/json` at the call site rather than inside it. This is a
competing option against the binary format and is recorded as such in
[v2-simulator-performance.md](v2-simulator-performance.md).

## 4. Method: what this campaign learned about measuring

Four instrument errors were made and caught, all by re-measuring rather than by
reasoning. They are the most transferable output here.

1. **`-allocprofile` is sampled.** Its run-to-run spread on this workload is
   **1.9 %** — wider than the −1.76 % "win" it was used to claim. It also
   invented a +5.8 % "regression" in the opposite direction. Replaced with
   `runtime.MemStats.Mallocs`, exact, spread under 0.001 %.
2. **Benchmarks in one process share a heap.** A decode benchmark following
   heavy encode benchmarks measured 4.18 ms; isolated it measures 1.43 ms. The
   published 15.9x was really **42.2x**.
3. **Instrumentation is not free.** A census probe formatting `%T` on every
   fallback event contaminated every allocation measurement taken after it, and
   produced a wrapper "regression" that never existed.
4. **A/A controls must be measured in the same session.** One A/B returned
   −0.40 % against an A/A of **+5.52 %** — the host had become an order of
   magnitude noisier within a single session. That run was discarded.

The rule that emerges: **sampling is a property of the instrument, not of
whether the number prints as an exact integer.** `alloc_objects` reads like a
census and is not one.

## 5. Independent reproduction status

**One build was independently graded; the current build has not been.**

`113189a` was graded from a clean workspace by a separate agent with its own
harness. It confirmed the wall-clock mechanism (-13.2 % paired against an A/A of
-0.8 %, sign test p ~ 0.0002), confirmed event-count parity and RSS, confirmed
field-level losslessness, and confirmed determinism across GOMAXPROCS at both
5 m and 1 h. It refuted one claim of mine outright ("every binary sample lies
below every JSON sample", which does not survive a contended host) and
re-attributed the allocation figure, though that re-attribution was itself
wrong: it removed a disabled-instrumentation line from the JSON arm only, not
knowing the binary sink carried an equivalent one. With both guarded the
original 8.22 % stands, re-measured at 8.30 %.

It found four defects. Three were real and are fixed: a false attestation
(binary runs wrote `event_count: 0` and an all-zero hash, so any two runs
compared identical in the tool built to tell runs apart), a stream that
truncated on an unencodable payload, and an encoding that depended on how the
caller boxed a value. Its fourth finding — that the speedup would not survive a
logging run — was the most valuable of all and is discussed above.

Everything after `113189a` is self-graded, including the replace-mode result
that is now the headline. A second grading pass is open.

**The remaining performance results have not been independently reproduced.** They
were produced and graded by the same agent, which the research protocol
explicitly warns against. Two independent adjudications were obtained during
this campaign, but both were of the *market-logic* findings, not the performance
work.

The end-to-end binary-sink measurement, when it happens, should be graded by a
separate reproducer from a clean workspace.

## 6. Counterevidence and limits

* Every wall-clock number depends on a host whose noise floor moved by two
  orders of magnitude within one session.
* The 11.5 % projection has never been measured end to end; it rests on a
  ceiling probe plus a retention ratio from two event families.
* The analytics ratios on real evidence (120x–3,400x) compare a converted
  single-family stream against a scan of mixed JSONL, so much of the ratio is
  pre-filtering rather than decode speed. The format-to-format number is 42.2x.
* Class B of the query taxonomy is the one place a columnar layout would win,
  and the argument against it is that B is already fast in absolute terms — not
  that a columnar layer would be slower.
* The binary format has two event families implemented out of roughly twenty.

## 7. Reproduction

```
# ceiling and emulation probes: patch checkpointSink.observe, then
BASE=<baseline> CAND=<probe> REPS=5 DURATION=1h bash bench_ab.sh

# format round trip, codec independence, query classes
go test ./evstream/... -count=1

# encode/decode ratios, isolated to avoid shared-heap contamination
GOMAXPROCS=4 go test ./evstream/exsim/ -run XXX -bench BenchmarkDecodeJSON -benchtime=5x -count=3
GOMAXPROCS=4 go test ./evstream/exsim/ -run XXX -bench BenchmarkDecodeBinary -benchtime=5x -count=3

# real-evidence conversion and query comparison
go run ./cmd/evsbench -in <venue>/spot/ABC-USD.jsonl -event BookDelta -symbol ABC/USD

# exact allocations, census off so the probe is not measured
EXSIM_ALLOC=1 GOMAXPROCS=4 ./multivenue -config dev-607-none.json -duration 1h -seed 607 -logdir <dir>

# no-op census
EXSIM_CENSUS=1 GOMAXPROCS=4 ./multivenue -config dev-607-none.json -duration 1h -seed 607 -logdir <dir>
```

Artifacts: `evstream/` (format, index, codecs), `evstream/exsim/` (schemas and
differential tests), `census/`, `cmd/evsbench/`,
`research/performance/vnext-evidence-format.md`,
`research/performance/v2-simulator-performance.md`.

## 8. Next experiment, and the stop assessment

**The research question is answered; what remains is engineering.** Three of
four mechanism families are closed with measurements — exchange logic, native
language, scheduler layout. The fourth, evidence production, has a measured
ceiling, an understood mechanism and a bounded expectation. Further probing of
it has lower expected information gain than the threshold, because the remaining
uncertainty is the gap between a sketch and an implementation, and only an
implementation closes that.

**Highest expected information gain:** schemas for the families covering ~76 %
of hashed bytes, wired into `checkpointSink`, measured end to end with an A/A
control and graded by an independent reproducer. Predicted 10–17 %; a result
below 10 % would mean framing and nested-field costs exceed the microbenchmark,
which is the one thing no method here has tested.

**Decisions that are not the researcher's to make:** whether to spend the
engineering budget on that implementation, and whether to grant an independent
reproducer for the run that grades it.
