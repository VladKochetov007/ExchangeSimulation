# VNext evidence architecture: a canonical binary event stream

Status: prototype on `autoresearch/v2-performance-research` only. Not merged, not
proposed for r5, and it changes no scientific semantics — the r5 evidence
contract is untouched.

## Why JSON is the wrong hot-path format, established by measurement

Three iterations of trying to make JSON cheaper produced a clear negative
result. On `dev-607-none`, with raw logging switched off entirely:

* `encoding/json.Marshal` is **14.99 % of CPU**, and `pprof -peek` attributes
  **100 % of it to one caller** — the ordered execution-stream hash;
* `sha256.blockSHANI` is a further **3.61 %**, over **978 MB per simulated
  hour**, about 23.5 GB for a 24-hour cell;
* roughly **19 % of CPU is spent proving the run reproducible**, not simulating
  a market.

The obvious repair — a hand-written encoder emitting byte-identical JSON — was
built, differentially tested over 40,000 randomised cases, and **rejected**: it
removed the reflection and changed nothing measurable. That is the finding this
design rests on. **The cost is the byte volume, not the encoder.** JSON cannot
be made cheap enough because the bytes themselves are the expense; the only way
forward is to stop producing them.

## Design

One canonical, deterministic, typed, append-only binary stream. Optional
indexes. No columnar layer unless a measurement demands one.

### Separation of identity from storage

The execution hash chains over **uncompressed canonical frames**. Compression is
applied per block afterwards. A run stored with zstd, with lz4, or raw has the
same hash, so choosing a codec can never change a scientific result. This is
enforced by `TestHashIsIndependentOfCodec`, which round-trips the same events
through all four configurations and requires one digest.

It also has a practical consequence the benchmarks made obvious: since the hash
does not see compression, a run can write **raw** and be compressed later as a
pure storage pass. Compression never has to be on the hot path.

### Canonical bytes

Fixed-width little-endian throughout. No varints — a varint has more than one
representation for a value unless minimality is enforced everywhere, and the
space it saves is recovered by block compression, which is where space belongs.
No floats in payloads: float formatting is platform-dependent and would make
canonical bytes impossible to guarantee. Prices and quantities are signed
`int64` in the instrument's fixed-point precision, which is what they already
are in the simulator.

### Framing

```
StreamHeader  32 B   magic, format major/minor, codec, schema epoch
Block*        20 B header + stored bytes
                    block magic, uncompressed len, stored len,
                    frame count, CRC-32C over UNCOMPRESSED bytes
Frame*        32 B header + payload
                    length, event_seq, sim_ts, schema id, schema version,
                    venue ref, client id
```

* **Ordering.** `event_seq` is gap-free and monotonic and is verified while
  reading, so global causal order is recoverable from the stream alone.
* **Corruption detection and streaming verification.** Block CRC, declared frame
  count and sequence continuity are all checked incrementally as data arrives,
  so damage is caught at the point of damage rather than after buffering a file.
  The CRC covers uncompressed bytes, so it verifies the data rather than the
  compressor's output.
* **Schema evolution.** Every frame is length-prefixed and carries a schema id
  and version, so a reader that does not know a schema skips it exactly instead
  of failing. Adding an event family does not invalidate old streams.
* **Explicit optionality.** A presence bitmap per schema, not a sentinel. This
  is not decoration: the very first round-trip test found real information loss
  because a zero count rendered both a JSON `null` and an empty `[]` the same
  way. Both now carry distinct presence bits.

### Dictionary

Repeated strings — venues, symbols, assets, wallets, reasons — are interned and
referenced by a `uint32`. `ABC-FUT-1735696801` costs four bytes per event
instead of twenty. Ids are assigned in first-use order, which is deterministic
because the event order is, and dictionary entries travel **as frames in the
same stream**, so interning is covered by the execution hash rather than living
in a side channel that could drift from the events referencing it.

### No registry to edit

`evstream` knows nothing about balances, orders or books. Payload encoding lives
on the value (`PayloadAppender`); decoding is supplied by the caller. A new event
family is added in a different package entirely, with no central table to
modify — the extension rule this repository works under.

## Measured, 20,000 representative balance-change events

Encode, including framing, interning and the SHA-256 chain:

| | ns/op | allocs/op | bytes/event | vs JSONL |
|---|---:|---:|---:|---:|
| JSON (current) | 13,001,324 | 40,054 | 276.3 | 1.00x |
| **binary** | **5,441,290** | **27** | **94.3** | **2.93x** |
| binary + s2 | 11,291,390 | 30 | 58.1 | 4.75x |
| binary + zstd | 15,643,998 | 54 | 45.0 | **6.14x** |
| binary + lz4 | 16,024,243 | 28 | 56.3 | 4.91x |

Decode:

| | ns/op | allocs/op |
|---|---:|---:|
| JSON | 60,409,715 | 246,516 |
| **binary** | **1,430,000** | **35** |

**Encode 2.39x faster, decode 42.2x faster, 1,483x and 7,043x fewer
allocations, 2.93x smaller raw and 6.14x smaller with zstd.** The binary encode
figure includes the execution hash; the JSON figure does not, so the real
encode ratio is nearer 2.96x once JSON's own SHA-256 is counted.

**Every codec costs more than it saves on the hot path.** LZ4 and zstd both make
encoding *slower than JSON*. Since the hash ignores compression, the right
arrangement is to write raw during the run and compress as a separate pass.

### What this implies for the simulator, and the caveat

JSON marshal (14.99 %) plus SHA-256 (3.61 %) is 18.6 % of CPU, replaced by
something about 2.96x cheaper, saving roughly 12 percentage points:
**about 1.14x**, under the ~1.28x Amdahl ceiling established earlier for any
serialization change.

That is a projection from a microbenchmark, not a result. This campaign has
repeatedly seen projected wins evaporate in a full A/B — most recently the JSON
appenders, which promised ~11 pp and delivered zero. The number to trust is the
one from an end-to-end run with an A/A control, which is the next step.

## Validation

The brief's requirement is bidirectional: canonical JSON, typed semantic event,
binary frame, typed event, canonical JSON, with every field and the ordering
preserved. `TestJSONToBinaryToJSONIsLossless` and `TestRoundTripRandomised`
implement exactly that, rendering the decoded event back to JSON and requiring
byte equality with `encoding/json` on the original — across quote, backslash,
control, HTML-significant, non-ASCII and invalid-UTF-8 strings, integer
extremes, present and absent optional fields, and nil versus empty slices.

It found a genuine defect on its first run, which is the reason it exists.

## Open architecture question: selective queries

The next question is whether the binary stream plus optional indexes can serve
selective analytics without a columnar layer. Planned, in order:

1. **Block index** — per block: offset, first/last `event_seq`, min/max
   timestamp, event-family bitmap. Held in a footer or sidecar so blocks can be
   skipped without being read, and so reads can be mmap-friendly and random.
2. **Benchmark four query classes separately**, never blended:
   A selective metric (few columns, one symbol, narrow window);
   B broad aggregate (all symbols, one family);
   C exact causal replay (full ordered reconstruction);
   D cross-family event study around a window.
3. Only if a class shows a concrete deficiency, prototype Parquet or Arrow for
   that class alone.

**The tension to resolve honestly** is class B. In one globally ordered stream,
families interleave, so nearly every block contains nearly every family and a
family bitmap skips almost nothing. A columnar layout wins class B by reading 3
of 30 columns; a row-major stream reads whole records whatever the index says.

The candidate answer that avoids a columnar layer is **derived family-partitioned
streams in the same binary format** — same frames, filtered, each still ordered
by `event_seq`, with global order recoverable by a k-way merge over roughly
twenty families. That keeps one format, one decoder and one set of guarantees,
and makes completeness checkable exactly as a derived layer should be: the union
of `event_seq` across derived streams must equal the set in the canonical
source. Whether that beats Parquet on class B is a measurement, not an opinion,
and it has not been taken yet.

## Block index, and the answer on Parquet

A block descriptor is 64 bytes — offset, stored and uncompressed length, first
and last `event_seq`, min and max timestamp, an **exact** event-family bitmap and
a **probabilistic** symbol filter. Exactness in the family bitmap and one-sided
error in the symbol filter are deliberate: a false positive costs a wasted block
read, a false negative would silently drop events from a scientific result.

The index is written as a sidecar rather than a footer, so a stream truncated by
a crash keeps a usable index for the blocks that landed, and an index can be
rebuilt without rewriting evidence.

### Correctness before speed

`TestQueryClassesAgree` runs every query three ways — JSON scan, binary full
scan, binary indexed scan — and requires identical match counts. An indexed scan
that skipped a block containing a match would look excellent and be wrong, so
this gate comes first and the benchmarks are only meaningful behind it.

### Measured, 120,000 mixed-family events

| class | JSON scan | binary full | binary indexed | blocks skipped |
|---|---:|---:|---:|---:|
| A selective (one symbol, 1 % window, one family) | 414.6 ms | 4.87 ms | **0.379 ms** | 8 of 9 |
| B broad aggregate (all symbols, one family) | 402.7 ms | 5.18 ms | 4.28 ms | **0 of 9** |
| C causal replay (every event in order) | 396.6 ms | 4.98 ms | 4.07 ms | 0 of 9 |
| D cross-family window | 416.9 ms | 4.93 ms | **0.513 ms** | 8 of 9 |

Allocations across a full pass: **4,274,441 for JSON, 3 for the indexed scan.**

**The predicted weakness appeared exactly where predicted, and it is smaller
than expected.** Class B skips no blocks at all: families interleave in a
globally ordered stream, so nearly every block contains every family and the
family bitmap is worthless there. That is the structural argument for a columnar
layer. But B is already **94x faster than the JSON baseline** in absolute terms,
so the argument does not translate into a practical deficiency.

**Provisional conclusion: the indexed binary format satisfies all four classes,
and Parquet is not justified on this evidence.** The residual option, if class B
ever becomes the bottleneck, is derived family-partitioned streams in the same
format — same frames, filtered, each ordered by `event_seq`, global order
recovered by a k-way merge — which keeps one format and one decoder instead of
importing a second storage engine.

### The caveat that matters for quoting these numbers

**The JSON baseline here is `map[string]any` unmarshal, which is the worst
case.** The production analyzer does not do that: it uses typed structs behind
`decodeRequiredJSON`, a needle prefilter and envelope reuse, and that work
already bought 2.83x over the naive path. So the fair comparison against the
*current optimized* analyzer is perhaps a third of the ratios above — roughly
30x rather than 95x on class C, and still very large on A and D where the index
does the work rather than the decoder.

The ratios that do not depend on the baseline are the ones between binary
variants: the index is worth **12.8x on class A** and **9.6x on class D** over a
binary full scan, and **1.2x on class B**, which is the number that decides the
columnar question.

### Block granularity is untuned

120,000 events produced only 9 blocks at the 1 MB default, so selectivity is
coarse — a query touching one event still reads a whole megabyte. Smaller blocks
would sharpen A and D further at some cost in compression ratio. This has not
been swept, and the A and D figures should be read as a floor rather than a
best case.

## The Amdahl ceiling, measured rather than projected

The 1.14x estimate for the simulator came from a microbenchmark ratio, and this
campaign has produced three projected wins that measured zero. Before building
schemas for twenty event families to get an end-to-end number, the cheap
falsifier is to measure the **ceiling**: a build whose `observe` skips
`json.Marshal` entirely and hashes a constant. If removing *all* of the
serialization cost yields X, then no serialization change can ever exceed X.

`dev-607-none`, seed 607, `GOMAXPROCS=4`, taskset-pinned, five alternating
repetitions with an inline A/A control:

```
A1  median 28.32s   28.32 28.07 27.96 28.38 29.07
A2  median 28.85s   28.58 29.51 28.85 29.16 28.70
B   median 23.69s   23.05 23.71 23.93 23.69 23.44

A/A control  : +1.85%
B vs A pooled: -17.29%
```

Every B sample lies below every A sample. **The ceiling is −17.29 %**, or about
1.21x, for removing serialization and its hashing from the evidence path
entirely.

### Two independent measurements agree

The CPU profile put `encoding/json.Marshal` at 14.99 % and `sha256.blockSHANI`
at 3.61 %, totalling **18.6 %**. The wall-clock ceiling probe says **17.29 %**.
A profile and a stopwatch, measuring different things by different means,
landing within 1.3 points of each other is the kind of corroboration this
campaign has mostly lacked.

### What the binary format should therefore deliver

The microbenchmark puts the binary path at 5.44 ms against roughly 16.1 ms for
JSON once JSON's own SHA-256 is included — it retains about **33.7 %** of the
cost, because it still hashes, just over 2.93x fewer bytes. So it should capture
roughly two thirds of the ceiling:

    0.663 x 17.29 % = about 11.5 %, or 1.13x

**That is now a bounded estimate corroborated by two independent methods, not a
microbenchmark extrapolation.** It is also larger than anything else this
campaign has found: the accepted `bookDeltaEvidence` change was −3.89 %, and
every other candidate measured zero.

### What this does not yet establish

The probe destroys the execution hash, so it is a measurement build and nothing
more. It bounds the opportunity; it does not demonstrate that a real binary sink
achieves it, and the remaining third of the cost — hashing 94 bytes per event
instead of 276 — is real work that stays. The end-to-end A/B against a genuine
binary sink is still the number that decides this, and it needs schemas for the
event families that make up the bulk of the stream.

The order of work that follows from this is clear: the five families covering
76 % of hashed bytes first, then the end-to-end A/B, and only then a decision
about retiring JSON from the hot path.

## Decomposing the ceiling: the bytes do not matter, the reflection does

The ceiling probe removed both the marshal and the hashed volume, so it could
not say which of the two mattered. A second probe separates them: same removal
of reflection and per-event allocation, but hashing **88 bytes** of appended
integers instead of one — the byte profile a real binary encoder would produce.

```
              A/A control    vs baseline
ceiling  (hash 1 B)   +1.85%      -17.29%
emulated (hash 88 B)  +0.88%      -17.93%
```

**The two are indistinguishable.** Cutting hashed volume from 276 bytes per
event to 88 buys nothing measurable. With hardware-accelerated SHA-256 the
digest is not the constraint; the reflection-based marshal and its 40,054
allocations per 20,000 events are.

This corrects an assumption stated earlier in this campaign — that the binary
format wins partly by removing bytes, and that the JSON appenders failed
*because* they kept the byte volume. That reasoning was wrong in its mechanism.
The appenders failed because their coverage was small: `balance_change` is only
13.5 % of events by count, and at that coverage the expected effect was about
1.5 %, below the 0.62 % A/A control's reliable detection threshold. The two
results are consistent, not contradictory — but the explanation offered at the
time was not the right one.

### The value proposition, correctly split

| dimension | gain | source |
|---|---|---|
| simulator CPU | **~11.5 %** | removing reflection and per-event allocation |
| storage | 2.93x raw, 6.14x zstd | byte-volume reduction |
| analytics decode | 42.2x | typed decoding, no parse step |

These are three separate wins with three separate mechanisms, and only the first
is bounded by the 17.9 % ceiling. Designing the format for compactness in order
to make the *simulator* faster would be optimizing the wrong variable — a
smaller encoding helps disk and nothing else. What makes the simulator faster is
that the encoder is typed, branch-free and allocation-free.

### Revised prediction for the real sink

The emulation performs no dictionary lookups and appends eleven fixed integers,
so it is optimistic. Estimating the real encoder at about 34.6 % of JSON's
marshal cost — 4.5 ms of the microbenchmark's 5.44 ms once SHA-256 over 1.9 MB
is subtracted — gives

    0.654 x 17.93 % = about 11.7 %

Two independent routes now converge on **11.5 % to 11.7 %**, and the mechanism
behind the number is understood rather than assumed.

## Interning priced, and a contradiction that must not be averaged away

The emulation appended fixed integers with no dictionary lookups, so it could
not price interning — the one cost a real typed encoder adds that the emulation
omitted. A third probe adds six map lookups per event against a dictionary of
realistic size and string shape, which is what a `balance_change` costs: symbol,
reason, optional side, plus asset and wallet per delta.

| probe | A/A control | vs baseline |
|---|---:|---:|
| ceiling — no marshal, hash 1 B | +1.85% | −17.29% |
| emulated — no marshal, hash 88 B | +0.88% | −17.93% |
| **interned** — no marshal, 6 lookups, hash 88 B | −0.64% | **−16.96%** |

**Interning costs at most about one percentage point, and possibly nothing** —
the spread across the three probes (17.0 % to 17.9 %) is comparable to the
spread of their own A/A controls. The specific design risk flagged in the
previous iteration is retired.

### The contradiction

Two methods now disagree about what the real sink will deliver, and the gap is
large enough to matter.

* **The probes** say the remaining cost after removing reflection is close to
  zero, implying the real sink lands near **17 %**.
* **The microbenchmark** says the binary path costs 5.44 ms per 20,000 events
  against 13.0 ms for `json.Marshal` alone — it retains about **42 %** of the
  marshal cost, implying a gain nearer **10 %**.

The explanation is that the probes are optimistic in a way the microbenchmark is
not. A probe does six lookups and fourteen integer appends into a reused buffer.
The real encoder additionally builds a frame header, walks a nested `changes`
slice field by field, emits dictionary frames on first use, patches frame
lengths, and maintains block state. None of that appears in a probe.

So the honest position is a **range of 10 % to 17 %**, with the microbenchmark
the more faithful of the two because it exercises a real implementation rather
than a sketch of one. It is recorded as a range rather than collapsed to a point
estimate, because averaging two measurements that disagree for an understood
reason would manufacture a precision neither supports.

What the probes do establish, and the microbenchmark cannot, is the **shape** of
the cost: reflection dominates, byte volume is irrelevant to simulator speed,
and interning is free. Those three facts constrain the design regardless of
where in the range the final number falls.

## Retention by family complexity, and the cost that turned out to be mine

The probes and the microbenchmark disagreed, so the retention ratio — real
binary encode over `json.Marshal` — was measured for the two families at
opposite ends of structural complexity.

| family | JSON ns/event | binary ns/event | retention |
|---|---:|---:|---:|
| `balance_change` — nested slice, optional field, six interned strings | 619 | 244 | 39.4 % |
| `BookDelta` — flat integers, one interned string | 272 | 174 | **63.9 %** |

**The simple family retains more cost, not less.** `encoding/json` is cheap on a
flat five-field struct, while the binary path pays a fixed per-event overhead
whatever the payload contains. Since the stream is dominated by simple events,
that pushed the expected gain to the bottom of the 10–17 % range.

Pricing the fixed overhead found the culprit, and it was a design decision of
mine rather than anything inherent:

| | with per-frame SHA-256 | without | hashing share |
|---|---:|---:|---:|
| `balance_change` | 4,878,599 ns | 2,221,540 ns | **54.5 %** |
| `BookDelta` | 3,472,636 ns | 1,082,852 ns | **68.8 %** |

The writer was doing `Reset(); Write(rolling); Write(frame); Sum()` for every
event — roughly two compression rounds plus reset overhead, 4.6 million times
per simulated hour. It was the single largest cost in the binary path, larger
than the encoding it existed to protect.

### The tempting fix, rejected

Hashing once per block would be cheaper still. It is also wrong: block
boundaries depend on the configured block size, so the digest would depend on a
storage parameter, and the central guarantee of this format is that storage
choices cannot change trajectory identity. A faster hash that breaks the reason
the hash exists is not a faster hash.

### What was adopted

One long-lived hasher streaming every frame, summed by cloning the state when a
digest is needed. Order-dependent, tamper-evident, and a pure function of the
canonical byte sequence — independent of block size, codec and buffering.

| | before | after | encode delta |
|---|---:|---:|---:|
| `balance_change` | 4,878,599 ns | 3,732,147 ns | **−23.5 %** |
| `BookDelta` | 3,472,636 ns | 2,244,306 ns | **−35.4 %** |

Retention improves to 30.1 % and 41.3 %, giving an expected simulator gain of
about **11.4 %** — converging with the original estimate, on a better
implementation. `TestHashIsIndependentOfCodec` still passes.

## Correction: decode is 42.2x, not 15.9x

The decode figure published earlier was measured in a process where the encode
benchmarks had already run, so the binary decode was paying for their GC state.
Measured in isolation, three repetitions each:

| | median |
|---|---:|
| JSON decode | 60.41 ms |
| binary decode | **1.430 ms** |

**42.2x, not 15.9x.** The correction is in the favourable direction, which makes
it no less a correction. It is the same class of error as the sampled allocation
profile earlier in this campaign: an instrument was trusted without checking
what else was sharing it. Benchmarks in one process share a heap, and a decode
benchmark that follows heavy encode benchmarks is not measuring the same
machine.

## On real evidence, not a synthetic corpus

Every analytics figure above came from a corpus generated by the same code that
reads it, which is the weakest form of evidence available. `cmd/evsbench`
repeats the comparison on bytes a real simulation wrote — the same file the
production analyzer consumes, with the same field shapes, string cardinality and
record ordering.

Input: `dev607-6h30m/venues/north/spot/ABC-USD.jsonl`, **304,877 `BookDelta`
records drawn from 2,065,331 lines**.

| | JSONL | binary |
|---|---:|---:|
| bytes per record | 195.9 | **69.0** |
| total | 59,719,933 | 21,037,012 (**2.84x smaller**) |
| encode | — | 72 ms (238 ns/record) |
| blocks | — | 21 |

| query | JSONL | binary | matched |
|---|---:|---:|---:|
| A — 1 % time window, one side | 1.810 s | **529 µs** | 1,017, counts agree |
| B — whole-run aggregate | 1.742 s | **14.5 ms** | 304,877, counts agree |

**Size is a fair comparison and holds: 2.84x on real data against 2.93x
synthetic.** Encode at 238 ns/record on real data is close to the 174 ns
measured synthetically, the difference being real symbol lookups and real value
distributions.

### The query comparison is not apples to apples

Stated plainly, because the ratios are large enough to mislead. The JSONL side
scans all **2,065,331** lines to find the 305k it wants. The binary stream
contains only those 305k, because the conversion already did the family
filtering. A large part of both ratios is that pre-filtering, not decoding
speed.

Two honest readings, and they answer different questions:

* **As a pipeline**, the comparison is the right one: an analyzer converts once
  (72 ms here) and then queries many times, so every subsequent query pays the
  filtered cost. That is the 120x to 3,400x figure, and it is real for a
  workload that queries more than once.
* **As a decoder**, the fair number is the isolated one measured earlier —
  **42.2x**, JSON 60.41 ms against binary 1.430 ms for the same 20,000 records.
  That is the number to quote when comparing formats rather than pipelines.

The block index contributes the difference between the two queries: A skips
almost every block and lands at 529 µs, B can skip none and pays 14.5 ms for the
same records. That ratio, 27x between a windowed and an unwindowed query over
one stream, is the index working, and it is measured on real data rather than a
generated one.

## What the simulator looks like after serialization is gone

The ceiling build — serialization removed entirely — was profiled to find the
bottleneck that VNext will expose. This is the post-VNext landscape, measured
rather than guessed.

| block | share |
|---|---:|
| map operations (`matchH2`, `aeshashbody`, `Iter.Next`, `mapaccess1_fast64`) | **~11 %** |
| GC (`scanObject` 8.89 % cum, `mallocgcSmallScanNoHeader` 8.53 % cum) | ~9–12 % |
| mutex lock + unlock | 5.9 % |
| scheduler heap (`heap.down` 3.21 % cum, `Less` 1.47 %) | ~4.7 % |
| `memmove` | 3.0 % |

**The next bottleneck is the map layer, not the market logic.** Hash-map access
is the largest single block once serialization is removed — symbol lookups,
client lookups, order-id indexes and the per-symbol logger lookup, all keyed by
string or uint64.

Two things follow. First, the dictionary interning that VNext introduces is on
the right side of this: replacing repeated string keys with dense `uint32` ids
is exactly the direction that shrinks this block, so the format's design
already leans against the next bottleneck rather than into it. Second, the
scheduler's ~4.7 % survives, and its layout has already been tested and
rejected — reducing it needs a calendar or ladder queue, which remains the one
identified but unattempted algorithmic change.

Recorded now so the post-implementation profile has a baseline to be compared
against rather than being interpreted fresh.

## Determinism under concurrency

Canonical bytes are only canonical if concurrency cannot reorder them. The same
run at three thread counts, `dev-607-none`, seed 607, ten simulated minutes:

| GOMAXPROCS | bytes | SHA-256 prefix |
|---|---:|---|
| 2 | 84,309,184 | `f94b9a5b879e3a840dd209bb` |
| 4 | 84,309,184 | `f94b9a5b879e3a840dd209bb` |
| 8 | 84,309,184 | `f94b9a5b879e3a840dd209bb` |

Byte-identical. This is stronger than the earlier same-thread-count check: it
says the ordering comes from the simulation's deterministic phase structure
rather than from the scheduler happening to interleave the same way twice.

Typed coverage also shrinks the stream. The identical run produced 108,229,701
bytes before the order-lifecycle schemas and 84,309,184 after — **22 % smaller**
purely from families moving off opaque JSON onto typed encodings.

## A design gap worth stating

With the binary sink enabled *and* raw logging on, the run currently writes
both: `observe` returns nil, so the venue logger falls through and marshals the
payload a second time for the JSONL. Both files are produced, and the run pays
for both.

That is why the bytes comparison above used separate runs rather than one. In
the intended end state the binary stream replaces the JSONL and
`RenderPayloadJSON` supplies the compatibility path on demand, so nothing needs
to be written twice. Leaving both in place is deliberate for now — it keeps the
old evidence available for differential checking while the format is under
review — but it means the measured wall-clock gain is the encode-and-hash saving
only, and does not yet include the write-side saving that retiring the JSONL
would add.

## Block compression, measured on a real run

Every earlier binary measurement ran uncompressed, because the sink constructed
`WriterOptions{}` and offered no way to ask for anything else. With the codec
made configurable, one hour of `dev-607-none`, seed 607, writing real files
rather than discarding:

| codec | bytes | ratio | elapsed | execution hash |
| --- | ---: | ---: | ---: | --- |
| none | 520,087,073 | 1.00x | 21.19 s | `59215e9bfc7da65d` |
| lz4 | 149,042,164 | 3.49x | 22.31 s | `59215e9bfc7da65d` |
| s2 | 131,841,722 | 3.94x | 21.38 s | `59215e9bfc7da65d` |
| zstd | 78,402,501 | **6.63x** | 23.25 s | `59215e9bfc7da65d` |

**All four hashes are identical.** That is the point of hashing uncompressed
canonical frames, and it is now demonstrated on a real run rather than argued
from the design. A codec is a storage decision: two runs compressed differently
are the same run, and a result never has to name its codec.

**s2 is close to free**: 3.94x for +0.9 % wall, inside the A/A noise floor of
0.06-0.8 %. zstd buys 6.63x for +9.7 %.

Against the JSON baseline the two changes stack. JSONL runs about 1.48 GB per
simulated hour; binary is 520 MB, and zstd takes it to 78 MB — **18.9x**
overall. A 24-hour cell falls from roughly 35.5 GB to 1.9 GB.

The cost figures are measured against the uncompressed *binary* arm, both
writing real files. They are not a comparison against JSON, which this
particular run did not include.

### Recommended default

s2, on the evidence: it is a 3.94x reduction for a cost that cannot be
distinguished from noise. zstd is the right choice when a campaign is
capacity-bound rather than time-bound, and the 9.7 % is worth naming rather
than absorbing silently.

## The adoption blocker, removed

`replace` mode makes the binary stream stand in for the venue JSONL, which
raises the question that decides whether the format can ship: are the typed
frames enough to rebuild the JSON *exactly*, or only nearly? Nearly is a
failure — the records are evidence.

`cmd/evsrender` reconstructs the venue log records from `events.evs` alone.
Measured on dev-607, seed 607, 20 simulated minutes, against a JSON-mode run of
the same config and seed:

```
json-mode JSONL lines    1,597,303
binary frames            1,569,110
replace-mode JSONL          28,193
                         ---------
1,569,110 + 28,193    =  1,597,303
```

The partition is exact. The 28,193 are `LogEvidenceOnly` events, which are
excluded from the execution hash by design because they must not perturb the
digest; they were never in the binary stream and replace mode still writes them.
The JSONL degrades to an instrumentation-only sidecar rather than disappearing.

Rendering the 1,569,110 hashed events and comparing them against the same events
from the JSON-mode run, sorted, byte for byte:

**IDENTICAL.** Every rendered line matches a JSON-mode line exactly.

This is a stronger statement than the field-level losslessness the independent
reproducer verified. That test asked whether each schema preserves its fields
through a round trip. This asks whether a real run's entire evidence corpus can
be rebuilt from the binary alone and come out byte-identical to what the
simulator would have written — 1.57 million records, every field, every
escaping decision, every numeric formatting choice.

### The stronger form of the test, and a caveat on comparing digests

An independent grader ran a better version of this check: instead of comparing
only the rendered subset against its counterpart, it sorted the **union** of
(rendered + the 28,193 leftover JSONL) against the whole JSON-mode JSONL. All
1,597,303 records, bit-identical. That is a superset of the original claim and
it proves something the original did not: **replace mode does not perturb the
trajectory**, since both arms produce the same complete record multiset.

The union form is now the reproduction below, and it was re-run after the
block-granularity digest change: still identical, 1,597,303 on both sides.

**Do not compare sorted-file digests across machines or agents.** `sort` is
locale-dependent, so two correct runs can produce different orderings of the
same multiset and therefore different file hashes. The grader's sorted union
hashed to `32054054...` and mine to `d26b831a...`; the content comparison inside
one consistent locale is what carries the result. Pin `LC_ALL=C` if a digest is
wanted as the artifact.

### Reproduction

```bash
EXSIM_BINARY_EVIDENCE=replace ./multivenue -config dev-607.json -duration 20m -seed 607 -logdir bin/
env -u EXSIM_BINARY_EVIDENCE      ./multivenue -config dev-607.json -duration 20m -seed 607 -logdir json/
# the union form: no family filter needed, because nothing is excluded
cat <(go run ./cmd/evsrender -dir bin/) bin/venues/*/*.jsonl bin/venues/*/*/*.jsonl |
  LC_ALL=C sort > union.sorted
cat json/venues/*/*.jsonl json/venues/*/*/*.jsonl | LC_ALL=C sort > original.sorted
cmp union.sorted original.sorted
```

### What is still not proven

The renderer emits one globally ordered stream. It does not reproduce the
*file layout* — which venue and symbol file each record was written to — because
that routing lives in the logger tree rather than in the events. An analyzer
that opens `venues/north/spot/ABC-USD.jsonl` by path still needs either that
routing rebuilt or a change to read the stream. The content is proven; the
directory shape is not.

## The digest algorithm: measured, and deliberately not changed

The hash is the one part of the format that would be expensive to change after
promotion, so it was worth pricing before the format freezes rather than after.

`crypto/sha256` on this host runs at **1,650 MB/s** (median of three, isolated
on a spare core; SHA-NI, which is what `sha256.blockSHANI` in the profile
means). The stream is 520 MB per simulated hour and a one-hour run takes about
24.6 s in replace mode, so:

```
520 MB / 1650 MB/s = 0.315 s  =  1.28 % of wall
```

**An infinitely fast hash would buy 1.28 %.** BLAKE3 at the 2-3x it typically
achieves over SHA-NI would save 0.6-0.85 %, which is below this host's A/A noise
floor. There is no case for changing the digest, and none for an assembly
implementation of it either: the existing one already dispatches to hardware.

### This supersedes an earlier finding rather than contradicting it

The campaign previously recorded that per-frame SHA-256 was **54-69 % of binary
encode cost**. That was true of per-frame hashing, which paid call overhead once
per event. The continuous hasher replaced it with one long-lived hasher over the
whole stream, moving the regime from overhead-bound to throughput-bound. Same
algorithm, different economics, and the earlier number should not be quoted as
if it still described the current design.

### Where the evidence cost now sits

With `replace` mode, the evidence subsystem is: typed encode (small), hashing
(1.28 %), and the write. **The write is now the largest remaining evidence
cost**, which is what makes the codec choice matter more than any further
encoder work — zstd removes 4.79x of it for 2.4 % wall.

That is a plateau on this path. Further optimisation of the evidence subsystem
has lower expected value than the diffuse remainder of the simulator, where
matching is 0.45 % of CPU and map work has no single caller above 1.5 %.

## Pre-registered cost model for the replace-mode hot path

Every profile grounding this campaign was taken on the **JSON** path. After the
format change the ranking has certainly moved, and the honest way to look is to
predict first and measure second, so the profile can contradict a stated model
rather than confirm whatever it happens to show.

Per event, replace mode does exactly this, read off the code rather than
guessed:

| work | count per event | estimated cost | share of a 24.6 s hour |
| --- | ---: | ---: | ---: |
| `binaryEvidence.record` mutex | 1 pair | ~15-20 ns | ~0.35 % |
| `checkpointSink.observe` mutex | 1 pair | ~15-20 ns | ~0.35 % |
| `Intern(eventName)` map lookup | 1 | ~22 ns | ~0.4 % |
| `Intern(venueID)` map lookup | 1 | ~22 ns | ~0.4 % |
| scratch-to-block payload copy | 1 | ~5 ns (tens of bytes) | ~0.1 % |
| continuous SHA-256 | 1 | measured | **1.28 %** |
| frame header append + length patch | 1 | ~5 ns | ~0.1 % |

At 4.64 M events per simulated hour, **no single item exceeds about 1.3 %, and
the whole enumerated list is roughly 3 %.** The remainder of the run is market
mechanics.

### The prediction, and what would falsify it

**Prediction: a CPU profile of a replace-mode run will show no evidence-path
function above ~2 % of CPU, and the evidence subsystem in total below ~5 %.**

If the profile shows any sink or writer function above 5 %, this model is wrong
and the cause is worth finding — most likely a cost not visible in the per-event
enumeration, such as block flush behaviour, allocator pressure from the growing
block buffer, or write-side stalls that the discard-mode measurements hid by
construction.

Two candidate optimisations follow from the table and are recorded as *not worth
doing on current evidence*: merging the two mutex pairs into one, and caching
the interned event-name reference at the call site. Together they price at about
1.5 %, against an A/A noise floor of 0.06-0.96 %. They are marginal rather than
free, and neither should be attempted before the profile either confirms the
model or overturns it.

## The pre-registered model was wrong, and how

The prediction was: no evidence-path function above ~2 % of CPU in replace mode,
subsystem below ~5 %. An independent profile refuted it. `checkpointSink.observe`
is **7.1-8.2 % cum**. The attribution:

```
checkpointSink.observe                     8.20%  cum
└─ binaryEvidence.record                   7.96%
   ├─ Writer.AppendInterning               6.56%
   │  ├─ appendFrame                       3.63%
   │  │  └─ sha256.Digest.Write            2.69%   <- predicted 1.28%
   │  ├─ OpaqueJSON.AppendPayloadInterning 1.17%
   │  └─ typed appenders + misc           ~1.75%
   └─ Writer.Intern                        1.41%   <- predicted 0.8%
      └─ Dictionary.Lookup (flat)          1.17%
```

**Two estimates were wrong, and the second error is the instructive one.**

1. The untyped tail. This one went wrong twice and the second time was mine to
   catch. A grader corrected the carried 6.6 % down to **2.43 %**, and I
   recorded the correction without checking it. The two opaque counters are
   **disjoint by construction** — a comment in `evstream_schema.go` that I wrote
   says a sink-level census *cannot see* wrapper-level fallbacks — so counting
   only `binary.sink[*]` misses the larger half. Verified by running the census
   myself:

   | tail | events | share of 1,568,215 |
   | --- | ---: | ---: |
   | sink-level, outer payload untyped | 38,103 | 2.43 % |
   | wrapper-level, inner payload untyped | 64,147 | 4.09 % |
   | **total** | **102,250** | **6.52 %** |

   **The original 6.6 % was right.** What does not change is the CPU
   conclusion: completing schema coverage still recovers about **1.4 %**,
   because two thirds of the residual marshal cost is `MarketDataFingerprint`
   in the market-data receipt subsystem, which runs identically in both arms
   and is not the evidence path at all.

   The two tails cost wildly different amounts per event — sink-level
   **2.62 µs**, wrapper-level **0.31 µs**, a factor of 8.4 — so 4.09 % of events
   carry 0.02 s while 2.43 % carry 0.10 s. The schema worth writing is
   **`maker_state` (32,400 events)**, then `balance_snapshot` (5,100). The
   64,147 wrapper-opaque events are barely worth typing.

2. **The streaming digest was priced with the wrong instrument.** A 1 MB-block
   benchmark measured 1,650 MB/s and the conclusion drawn was that a continuous
   hasher is throughput-bound, giving 1.28 %. The profile says 2.69 %. Writes
   here are one frame at a time — on the order of 150 bytes — so the cost is
   partly per-call and buffering overhead, not streaming throughput. A block
   size chosen for the benchmark's convenience answered a different question
   than the one asked.

The general form: **an instrument that measures the right operation can still
measure it under the wrong conditions.** Benchmark at the size the code
actually uses.

### What the profile makes worth doing, and what it does not

| candidate | measured share | verdict |
| --- | ---: | --- |
| per-event `os.Getenv` in `venueLogger.LogEvent` | 1.12 % | **fixed** — resolved once at construction |
| `Writer.Intern`, two map lookups per event | 1.41 % | declined; removing one is worth ~0.6 %, inside the A/A band |
| completing schema coverage | ~1.4 % | declined for now; larger than it looks only because it also removes the last reflection from the path |
| a faster digest | ≤2.69 % | declined; see the digest section, and it is a format-identity change |

The `os.Getenv` call was a regression introduced with replace mode itself: a
syscall-backed read, on the hottest path in the simulator, of a value that
cannot change during a run. A direct A/B of the fix returned **-1.21 % against
an A/A control of +1.63 %**, so the wall-clock effect is **not resolvable on
this host today** and is not claimed. The change stands on the profile
attribution and on being obviously correct, not on that measurement.

### The digest is absorbed per block, and my own comment had ruled it out

An earlier comment in `writer.go` rejected hashing at block granularity: "block
boundaries depend on the configured block size, so the digest would depend on a
storage parameter." That conflated two different designs. A per-block digest
that is then *chained* would indeed depend on the block size. Absorbing the same
bytes into the same continuous hasher in larger `Write` calls does not —
SHA-256 is a stream, so `Write(a); Write(b)` and `Write(a||b)` are identical.

Frames now accumulate and are absorbed by `flushBlock`; `ExecutionHash` clones
the running state and absorbs the unflushed tail, so it stays correct at any
point. **Verified by the strongest available test: the same config and seed
produce the bit-identical digest before and after —
`45f0f6826226c46dde4a55c34672ba42bd98970fd4557fa4389d78118dcb6b1b`.**

Measured at **-0.44 % against an A/A control of +1.15 %**, so the effect is the
predicted size but below what this host resolves today; it is not claimed as a
measured win. It is accepted because it is provably digest-neutral and strictly
less work — fewer `Write` calls and no per-frame copy into the hasher's 64-byte
staging buffer.

### Why the digest cost was under-predicted, settled

An independent bench varied only the `Write` size while hashing one real
stream's 174,723,743 bytes: 1 MB gives 1,716 MB/s, and the ~111-byte mean frame
gives 1,367 MB/s. So the write-size penalty is real but only **1.26x**, moving
the prediction from 1.19 % to 1.50 % against a measured 2.69 %.

**Write size was therefore about 0.3 of the 1.5-point miss, not the miss.** The
in-situ rate is 174.7 MB / 0.23 s = **760 MB/s, 44 % of the microbenchmark rate
on the same core**. Stripping the `memmove` still leaves compression at ~878
MB/s. The gap is working-set and GC pressure — the same profile shows
`runtime.scanObject` at 8.6 % against a 645 MB RSS.

The lesson is sharper than "benchmark at the right size": **a tight loop over a
hot buffer on an idle core is the wrong instrument even at the right size.** A
111-byte benchmark would have reached 1.50 %, still under half the truth.
