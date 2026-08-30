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
| JSON | 66,452,830 | 246,516 |
| **binary** | **4,181,613** | **35** |

**Encode 2.39x faster, decode 15.9x faster, 1,483x and 7,043x fewer
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
