# Validation report for `perf/r2-cdf-survival-port`, 2026-09-05

Four commits applied to `a666d02`, the head of
`feature/r2-cdf-survival-successor`. Nothing is merged into that branch; this
exists to be reviewed and adopted or rejected commit by commit.

An earlier version of this series targeted `230e78f`. That base is 138 commits
stale relative to this branch, and roughly half of it was work this branch had
already done independently and in some places better. Only what is still
outstanding is here.

## Already solved on this branch, so not included

Recorded because the convergence is itself evidence the diagnoses were right,
and because a reader of the older series should know not to apply them.

- **Rendering was quadratic.** `addRenderRecord` re-checked the per-venue
  sequence contract by scanning the route. Gone here.
- **Rendering held the whole run in memory.** `evidence_render_stream.go` is a
  streaming merge here, arrived at independently.
- **The merge's ordering invariant was assumed rather than checked.** This
  branch checks it *and* more strictly than the older series did:
  `renderOutput.append` requires `record.sequence` to equal the next expected
  per-venue sequence exactly, so ordering, gaps and duplicates fall out of one
  streaming test. The older series only enforced per-route monotonicity.
- **The manifest did not record the toolchain.** `BuildInfo` carries `goamd64`
  here.

## What is still outstanding, and what each commit does

### `2797d0f`, `606889c` — market-data fingerprint

`types/market_data.go` marshals a five-field envelope to JSON on every
market-data delivery to every participant, purely to feed SHA-256 and keep
16 bytes. Replaced with a canonical appender rendering exactly the bytes
`encoding/json` would, declining to the reflection path for any payload type or
symbol it does not handle.

Checked by differential fuzz against the reflection path, 24.2M executions, zero
mismatches; execution hash unchanged; all four market-data artifacts
byte-identical.

**On the size of the gain.** `2797d0f`'s message says -5.00%, which is the
number from the branch it was written on. Measured against `230e78f`, arms
alternating by round parity, dev-607/seed 607/20m:

| mode | rounds | median | direction | A/A median |
| --- | ---: | ---: | --- | ---: |
| jsonl | 24 | -1.52% | 22/24, p ~ 1.8e-5 | -0.31%, 7/12 |
| evstream_v3 | 16 | -2.01% | 15/16, p ~ 2.6e-4 | -0.16%, 7/10 |

A profile explains the difference: `MarketDataFingerprint` costs 0.60 s and
5.52% of CPU on the origin branch and 0.63 s and 3.26% there — the same absolute
time, a smaller share, because those runs do roughly 1.8x the total work. **Read
it as about -2%**, not measured against this branch's head.

### `815bda9` — two analyzer metrics are not reproducible

A survey of 52 metrics, four runs each over one run directory at real
parallelism, found exactly two that answer differently every time.

`reaction` gave 5 distinct outputs from 5 runs and was deterministic at
`GOMAXPROCS=1`. `Scan` reads venue files on a worker pool and a book's records
reach it from more than one file, so arrival order varies; the trade tape is
sorted on `sim_ts` alone, which is not a total order over a tie-dense tape, and
`sort.Slice` is not stable, so `sort.Search` returns a different price each run.
Fills are never sorted, and both the fill map and the book map are walked in
Go's randomised map order, so the floating-point sums vary on top of that.

`resting` sorts map-derived names on median alone, so classes at equal medians
swap places.

Fixed by totally ordering on `(File, Ordinal)`, which `analysis.Event` already
carries for this purpose. Both go from 5-of-5 and 4-of-5 distinct outputs to
1-of-5. The new tests fail on the parent commit.

Note the fixed value does **not** equal the old `GOMAXPROCS=1` answer. It
replaces an unspecified tie convention with a principled one and the number
moves.

### `e3c5d64` — `reaction` marks fills against the wrong instrument

Spot records carry no symbol in their data layer; only the derivative nesting
does, and the scanner unwraps only that. `reaction` keys books on
`markKey{VenueID, Symbol}` with no fallback and `mvanalyze` runs it over every
venue file, so all three spot books of a venue share the key `{venue, ""}`.
ABC/USD trades near 50.00 and CDF/USD trades near 3.00 occupy one price tape,
and the price one horizon after a maker's fill is whichever book traded next.

`reaction` is the only book-keyed metric in the package without a fallback.
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
unchanged and the derivative rows are bit-identical — the control, since those
records carry a symbol and must not move. The book table gains exactly the six
rows it should, 72 to 78.

One pre-registered prediction **failed**: the lag arm was expected to rise off
zero and did not. 312,597 observations and a pooled mean of
`3.199007028218441e-06` seconds, bit-identical before and after. The lag is not
zero because books were pooled; it is zero because orders arrive within
microseconds of the changes they follow inside a single book too. A separate
emptiness in the lag arm, left as found.

**Verified on this branch, not only on the one these commits came from.** The
numbers above were first measured against `230e78f`. Re-measured on `a666d02`
with a run generated by this branch's own binary, dev-607/seed 607/20m:

| role | before | after |
| --- | ---: | ---: |
| pooled | -13,292.239 bps | -762.258 bps |
| `cdf_spot_maker` | -37,539.981 bps | -4.477 bps |
| `fixed_distance_maker` | -25,046.377 bps | -3.417 bps |
| `imbalance_maker` | -31,420.427 bps | -4.009 bps |
| `abc_cdf_spot_maker` | -1,556.454 bps | -1.570 bps |
| `futures_maker` (derivative) | 3.463 bps | 3.463 bps |

Every corrected value is identical to the one measured 138 commits earlier. The
uncorrected ones are not — pooled reads -13,292.239 here against -13,950.28
there — because the unfixed metric answers differently on every run. That is the
point restated: the fix makes this metric reproducible across revisions, and
without it a comparison between two branches measures the tie order as much as
it measures the simulation.

**Consequence for the record:** every published spot-maker markout was computed
against a pooled tape. Anything resting on that figure needs re-deriving. This
branch does not do that, because those are not its conclusions.

## Two findings that are not changes

**A 24-hour run completes.** The `2026-08-31 capacity probe outcome` note
records a 24-hour run exiting at the terminal population mark on an unpriceable
CDF/USD mark and blocks a capacity measurement on adjudicating it. Run on
`230e78f`, dev-607/seed 607, the simulation exits 0 after a full `sim=24h0m0s`,
15m30s wall, 17,434,418,871 bytes of `events.evs`.

That is not a fix: between `95596f1`, the revision the probe used, and
`230e78f`, the diff is two documentation files and no code. The difference is in
the setup, and `GOAMD64` is ruled out — see below. The remaining candidates are
the compiler version (the note says Go 1.27; these runs used `go1.26.7`), the
seed the note does not record, and the copied configuration.

**Re-measured on this head.** The first run was against `230e78f`, 138 commits
behind, which include the CDF depth and concentration work — exactly the
mechanism the probe failed on — so the result could not be assumed to carry.
Run again on `a666d02` with an unpatched build of this branch:

| | `230e78f` | `a666d02` |
| --- | --- | --- |
| simulation | exit 0, `sim=24h0m0s` | exit 0, `sim=24h0m0s`, wall 18m0s |
| event frames | 96,241,273 | 96,241,273 |
| `events.evs` | 17,434,418,871 B | 18,006,037,027 B |
| execution hash | `2806c51c…b26dc` | `f8bc3bc4…` |

**The failure does not reproduce here either.** The condition the capacity gate
is blocked on is not a property of this code at 24 hours on the default cell.

The event frame count is identical across those 138 commits while the stream is
3.3% larger, which is what schema changes adding fields to frames would look
like rather than a moved trajectory. That is an observation, not a claim — it
was not chased, and nothing above depends on it.

**The toolchain moves the hash without moving anything else.** The same revision
and seed run for 24 simulated hours at `GOAMD64=v3` hashes `2f00c608…a001f`
where the default hashes `2806c51c…b26dc`. Both emitted exactly 96,241,273 event
frames and both passed every gate. The streams first differ at byte 12,584,428
of 17.4 GB — 0.072% in — never reconverge, and their total sizes differ by 2,703
bytes. The drift is numerical, not structural: everything a person would inspect
agrees and only the number the attestation compares disagrees. This branch
already records `goamd64`, which is what makes that diagnosable.

## Open, and not this branch's to close

- Re-deriving whatever depended on spot-maker markout.
- What `reaction`'s lag arm is meant to measure, given it reports 3.2 µs.
- Which of the three remaining differences explains the capacity probe. The
  compiler-version arm needs a toolchain that was not installed.
