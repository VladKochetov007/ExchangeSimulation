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
