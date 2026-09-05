# Validation report for `perf/ffa-gen0-port`, 2026-09-05

Ten commits applied to `230e78f`. Nothing is merged into
`autoresearch/ffa-ecology-gen0`; this branch exists to be reviewed and adopted
or rejected commit by commit.

The work came from `autoresearch/v2-performance-research`, which built a binary
evidence format independently from the same merge-base `887899f`. That branch's
own format is superseded by `evstream_v3` and was dropped rather than pushed.
What is here is the part that survives: one optimization, three correctness
fixes, two performance fixes to the renderer, one provenance addition, and two
commits that correct claims made earlier in this series.

## What each commit changes and how it was checked

### `a5fd14a`, `d1b8c01` — market-data fingerprint

`types/market_data.go` marshalled a five-field envelope to JSON on every
market-data delivery to every participant, purely to feed SHA-256 and keep
16 bytes. Replaced with a canonical appender that renders exactly the bytes
`encoding/json` would, declining to the reflection path for any payload type or
symbol it does not handle.

Checked: differential fuzz against the reflection path, 24.2M executions, zero
mismatches. Execution hash unchanged; all four market-data artifacts
byte-identical.

**Read `a5fd14a`'s -5.00% as the origin branch's number.** See `20acb8e`.

### `2290019`, `ad9a561` — two analyzer metrics were not reproducible

A survey of 52 metrics, four runs each over one run directory at real
parallelism, found exactly two that answer differently every time.

`reaction` gave 5 distinct outputs from 5 runs and was deterministic at
`GOMAXPROCS=1`. `Scan` reads venue files on a worker pool and a book's records
reach it from more than one file, so arrival order varies; the trade tape was
then sorted on `sim_ts` alone, which is not a total order over a tie-dense tape,
and `sort.Slice` is not stable, so `sort.Search` returned a different price each
run. Fills were never sorted, and both the fill map and the book map were walked
in Go's randomised map order.

`resting` sorted map-derived names on median alone, so classes at equal medians
swapped places.

Fixed by totally ordering on `(File, Ordinal)`, which `analysis.Event` has
always carried for this purpose. Both went from 5-of-5 and 4-of-5 distinct
outputs to 1-of-5. The new tests fail on the parent commit.

`ad9a561` corrects a claim in `2290019`: the fixed value does **not** equal the
old `GOMAXPROCS=1` answer. It replaces an unspecified tie convention with a
principled one and the number moves.

### `aa9d7b4` — `reaction` marked fills against the wrong instrument

Spot records carry no symbol in their data layer; only the derivative nesting
does, and the scanner unwraps only that. `reaction` keyed books on
`markKey{VenueID, Symbol}` with no fallback and `mvanalyze` runs it over every
venue file, so all three spot books of a venue shared the key `{venue, ""}`.
ABC/USD trades near 50.00 and CDF/USD trades near 3.00 occupied one price tape,
and the price one horizon after a maker's fill was whichever book traded next.

`reaction` was the only book-keyed metric in the package without a fallback.
`arbitrage`, `crossvenue` and `triangular` use `symbolFromPath(event.File)`;
`post_only` uses `symbolFromSpotFile`; `hedging`, `roleaudit`, `viability`,
`term_carry_p4`, `exposure` and `options_p6` fall back to a payload symbol;
`surface` rejects non-options. Of the six metrics `mvanalyze` runs without
narrowing the file set, the other five return early or fall back.

Measured on dev-607/seed 607/20m:

| role | before | after |
| --- | ---: | ---: |
| `cdf_spot_maker` | -37,763.985 bps | -4.477 bps |
| `imbalance_maker` | -31,305.874 bps | -4.009 bps |
| `fixed_distance_maker` | -24,940.754 bps | -3.417 bps |
| `abc_cdf_spot_maker` | -1,159.630 bps | -1.570 bps |
| `futures_maker` (derivative) | 3.463 bps | 3.463 bps |
| `option_dealer` (derivative) | -8,985.718 bps | -8,985.718 bps |

Pooled markout goes from -13,950.28 to -762.26 bps. Every fill count is
unchanged and the derivative rows are bit-identical, which is the control: those
records carry a symbol and must not move. The book table gains exactly the six
rows it should, 72 to 78.

One pre-registered prediction **failed**: the lag arm was expected to rise off
zero and did not. 312,597 observations and a pooled mean of
`3.199007028218441e-06` seconds, bit-identical before and after. The lag is not
zero because books were pooled; it is zero because orders arrive within
microseconds of the changes they follow inside a single book too. That is a
separate emptiness in the lag arm, left as found.

**Consequence for the record:** every published spot-maker markout was computed
against a pooled tape. Anything resting on that figure needs re-deriving. This
branch does not do that, because those are not its conclusions.

### `ebe33b4`, `1b03fa2`, `4aea437` — rendering

`addRenderRecord` re-checked the per-venue sequence contract by scanning every
record already appended to the route, making rendering quadratic. The check was
redundant and weaker than `validateRenderRecords`, which uses a set per venue
rather than per route, so a sequence claimed by two routes of one venue passed
the removed scan and fails the surviving check. Nothing tested either; the
contract is now tested directly.

Rendering then held the whole reconstructed run in memory. Frames arrive in
per-route sequence order and each sidecar file is written in sequence order, so
both inputs are already sorted and merge as they are read. Two structures made
buffering look necessary and did not: the evidence-only digest is an addition of
per-record hashes declared `unordered_multiset`, so fold order is irrelevant,
and the sequence set became a bitmap.

`4aea437` then checks the ordering the merge rests on rather than assuming it.
The invariant was measured, not guaranteed — a violation would have produced a
plausible file with no error, since the duplicate and gap checks cover which
sequences appear and not their order.

| run | frames | wall before | after | RSS before | after |
| --- | ---: | ---: | ---: | ---: | ---: |
| 5m | 382,498 | 7.83 s | 0.99 s | 223 MB | 13.8 MB |
| 20m | 1,640,870 | 132.90 s | 4.22 s | 922 MB | 12.7 MB |
| 60m | 4,859,415 | 1118.75 s | 12.83 s | 2.60 GB | 14.2 MB |
| 6h | 26,682,865 | 83.7 s* | 77.3 s | 15.14 GB | 21.9 MB |
| 24h | 96,241,273 | not attempted | **285.6 s** | ~60 GB projected | **47 MB** |

\* the 6h "before" column is after the complexity fix, so it isolates memory.

Output is byte-identical at every size, 15 of 15 files, up to 98,181,443
records at 24 hours, with matching frame counts, route counts and execution
hashes. The ordering check never fired across 26,682,865 frames.

Together these take a 24-hour render from roughly 6.4 days and ~60 GB to
4 minutes 46 seconds and 47 MB.

### `9c39449` — toolchain in the manifest

The same revision and seed run for 24 simulated hours at `GOAMD64=v3` hashes
`2f00c608…a001f` where the default hashes `2806c51c…b26dc`. Both emitted exactly
96,241,273 event frames and both passed every gate. The streams first differ at
byte 12,584,428 of 17.4 GB, 0.072% in, and never reconverge; total sizes differ
by 2,703 bytes.

The drift is numerical, not structural — nothing added, dropped or reordered,
values move in the last bits and the digest moves with them. Everything a person
would inspect agrees and only the number the attestation compares disagrees.

`BuildInfo` now carries `go_version`, `goarch`, `goos` and `goamd64`, with
`goamd64` omitted when unreported so older manifests are unchanged. Verified on
real runs: the two builds that hash differently now produce manifests that say
why. Provenance only — no semantics change, no execution hash moves.

## Independent checks of the existing implementation

Not changes, but findings from testing `230e78f` from outside it.

**The reconstruction is exact.** Running dev-607/seed 607/20m twice from one
binary, once in JSON mode and once as `evstream_v3`, rendering the binary run
and comparing file by file: 15 of 15 byte-identical after removing the
`"sequence":N` the renderer adds to every record, 1,669,063 records. The
reconstruction is sound; the caveat is that the added field means a rebuilt tree
cannot be byte-compared against the historical format without normalising first.

**A 24-hour run completes.** The `2026-08-31 capacity probe outcome` note
records a 24-hour run exiting at the terminal population mark on an unpriceable
CDF/USD mark and blocks a capacity measurement on adjudicating it. On
dev-607/seed 607 the simulation exits 0 after a full `sim=24h0m0s`, 15m30s wall,
17,434,418,871 bytes of `events.evs`.

**This is not a fix.** Between `95596f1`, the revision that probe used, and
`230e78f`, the diff is two documentation files and 159 insertions — no code. The
binary that succeeded is the binary that failed, so the difference is in the
setup. `GOAMD64` is ruled out by the ablation above: the gate passes at both v1
and v3. The remaining candidates are the compiler version (that note says Go
1.27; these runs used `go1.26.7`), the seed the note does not record, and the
copied configuration.

## Open, and not this branch's to close

- Re-deriving whatever depended on spot-maker markout.
- What `reaction`'s lag arm is meant to measure, given it reports 3.2 µs.
- Which of the three remaining differences explains the capacity probe. The
  compiler-version arm needs a toolchain not installed here.

## Reproduction

`WORK` is a scratch directory outside the repository.

```bash
go build -o "$WORK/mv" ./cmd/multivenue && go build -o "$WORK/evsrender" ./cmd/evsrender
CFG=research/configs/v2-integrated-longrun/dev-607.json

# reconstruction is exact, modulo the added sequence field
"$WORK/mv" -config $CFG -duration 20m -seed 607 -logdir "$WORK/json"
"$WORK/mv" -config $CFG -duration 20m -seed 607 -evidence-format evstream_v3 -logdir "$WORK/bin"
"$WORK/evsrender" -dir "$WORK/bin" -out "$WORK/rebuilt"
for f in $(cd "$WORK/json" && find venues -name '*.jsonl'); do
  cmp "$WORK/json/$f" <(sed -E 's/("data":\{"venue_id":"[^"]*",)"sequence":[0-9]+,/\1/' "$WORK/rebuilt/$f")
done

# the analyzer determinism survey, 5 before the fix and 1 after
for i in 1 2 3 4 5; do ./bin/mvanalyze -metric reaction "$WORK/json" > "$WORK/r$i"; done
md5sum "$WORK"/r* | awk '{print $1}' | sort -u | wc -l

# the toolchain ablation
GOAMD64=v3 go build -o "$WORK/mv-v3" ./cmd/multivenue
"$WORK/mv-v3" -config $CFG -duration 24h -seed 607 -evidence-format evstream_v3 -logdir "$WORK/v3"
```
