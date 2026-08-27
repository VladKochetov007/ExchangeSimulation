# V2-8 analyzer union-cache evaluation

Status: **rejected for adoption; benchmark-only candidate** (2026-08-27).

## Question and isolation

The analyzer was benchmarked separately from the simulator on the retained
signed-price-hardening full-evidence seed-101 workload.  The candidate was
implemented only in an isolated worktree at benchmark revision `c40bb31`; no
working-tree source, simulator binary, evidence encoder, or scientific config
was changed.  The benchmark used Go `go1.26.6-X:nodwarf5`, `GOMAXPROCS=8`,
sequential runs, and stayed below the 20-GB memory budget.

The baseline is the six physical reducer scans used by the selected mechanical
analyzers (conservation once, positions twice, fill-position twice, and order
lifecycle once).  The candidate decodes the event envelope/data layer once,
retains all decoded events, and feeds the otherwise unchanged reducers.  This
is a deliberately generous cache comparison, not a proposed production
implementation.

Workload: 15 JSONL files, 684,592,532 raw event bytes.  Baseline and candidate
produced byte-identical combined output and identical component digests:

    combined SHA256: b9c599bb1b82bba2d9324cd6836d453c73496ccd292c82ce4d995b05ac1a78ad
    conservation:   269ecd5e56a3c3b2c7ed19039dd06b69f4ed133f34d6c4bdb2507f86ffec03b2
    positions:       1338a4890213448a30baf782e976aa4cc68feb4a62b4a8e5e490ce49217c774d
    fillpositions:   7805d3378d9f73e38d1f95b49092e78712c8124c806cc2166a1cfeadf0891734
    orderlifecycle:  546b579d51ceb78a8b6d9172ee17220ebe65795d7d57f4dfaec76f7802ee967b

The complete raw benchmark files and profiles are retained under
`research/artifacts/v2-8-profiling/analyzer-union-cache-20260827/`, with
`SHA256SUMS`.

## Result

| measure | repeated scans | retained union cache | change |
|---|---:|---:|---:|
| wall seconds | 14.224 | 8.183 | −42.5% |
| CPU user+system seconds | 28.79 | 29.35 | +1.9% |
| peak RSS | 102,216 KiB | 1,136,968 KiB | +11.1× (+1.01 GiB) |
| total allocated bytes | 3,104,773,312 | 3,611,190,656 | +16.3% |
| malloc count | 46,833,264 | 44,657,356 | −4.65% |
| GC cycles | 192 | 24 | −87.5% |

The wall-time reduction is an I/O/redecode trade: CPU work does not decrease,
and the retained event representation causes a large resident-memory and
allocation increase.  A full retained-event cache is therefore unsafe for
large campaign extraction under the declared memory budget.  It is rejected,
despite exact output equivalence on this workload.

## Next bounded hypothesis

The result justifies investigating a streaming single-decode dispatcher or
fused reducer pipeline, not retaining the entire event stream.  Any future
candidate must preserve per-file ordering and parser failure behavior, use a
bounded queue/state, and pass malformed-record/error-order, duplicate-key,
integer-boundary, `RawMessage`, trailing-garbage, full-output, digest, and
fresh-process determinism differentials.  It must be benchmarked with repeated
medians and an explicit peak-RSS ceiling.  No JSON dependency replacement is
licensed by this result; `encoding/json` remains the semantic reference.
