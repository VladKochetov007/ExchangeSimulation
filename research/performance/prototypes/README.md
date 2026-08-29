# Performance ceiling prototypes

These are **measurement prototypes, not production candidates**. They exist to
put a number on how much headroom a SIMD or native implementation would have,
so a decision about cgo can be made against evidence instead of intuition.

Neither is built by the repository's Makefile and neither is imported by any Go
package. The project has no external Go dependencies and no cgo, before or
after this work.

Build and run:

    g++ -std=c++23 -O3 -march=native -o prefilter prefilter.cpp
    ./prefilter <corpus.jsonl>

    g++ -std=c++23 -O3 -march=native -o stage1 stage1.cpp
    ./stage1 <corpus.jsonl> [key ...]

A corpus is any retained development evidence JSONL file. Do not point these at
registered holdout evidence.

## prefilter.cpp

Multi-needle line prefilter: does any of N quoted event names occur in this
line? Compares the analyzer's former hand-rolled search, `memmem`, and an AVX2
first/second-byte Teddy candidate filter with scalar verification.

Measured on a 101 MiB retained derivatives corpus, twelve registered needles:

| Implementation | Throughput |
| --- | ---: |
| scalar naive (the analyzer's former search) | 222.9 MiB/s |
| `memmem` | 946.3 MiB/s |
| AVX2 Teddy | 1459.3 MiB/s |

All three selected the identical 269,713 lines.

For comparison, in Go over the same corpus and needles: the former hand-rolled
search reached 170.7 MB/s and `bytes.Contains` 339.6 MB/s.

## stage1.cpp

simdjson-style structural pass: compute the quote and structural bitmaps for a
buffer using SIMD plus a carry-less-multiply prefix XOR, then walk the resulting
index to return the byte range of each requested top-level key's value. This is
the shape a coarse-grained cgo submodule would take — one call per buffer, with
Go parsing only the small scalars it actually reads.

Measured over 300,000 retained payloads (60.95 MiB):

| Stage | Throughput |
| --- | ---: |
| structural index only | 2604.0 MiB/s |
| structural index + extract 5 top-level keys | 1181.0 MiB/s |

For comparison, in Go over the same payloads: `json.Unmarshal` into a struct
reached 136 MB/s, and `json.Valid` — which runs exactly the `checkValid` scan
that `json.Unmarshal` performs before decoding anything — reached 343 MB/s.
Validation is therefore about 40% of every `Unmarshal` call.

It does **not** implement full JSON validation, number parsing, or
`encoding/json`'s error classification. That is the point: the benchmark
measures the byte-scanning ceiling, and the semantic-equivalence bar for
replacing `encoding/json` is a separate and much higher one. An earlier
screen in this repository rejected `goccy/go-json` precisely because it
accepted an overflowing `int64` that the standard library rejects.
