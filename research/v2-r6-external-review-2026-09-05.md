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

**And it is not one cell, nor one sign.** The same before/after on the other two
development cells, each at its own seed:

| cell | role | before | after |
| --- | --- | ---: | ---: |
| dev-613 | `cdf_spot_maker` | -19,973.165 bps | -3.979 bps |
| dev-613 | `fixed_distance_maker` | -15,601.116 bps | -2.867 bps |
| dev-613 | `abc_cdf_spot_maker` | 161.072 bps | 0.433 bps |
| dev-617 | `cdf_spot_maker` | **+68,800.736 bps** | -6.515 bps |
| dev-617 | `fixed_distance_maker` | +18,778.950 bps | -2.525 bps |
| dev-617 | `abc_cdf_spot_maker` | 1,526.707 bps | 1.216 bps |

Fill counts are unchanged in every row. The sign is not stable either: dev-607
and dev-613 read strongly negative and dev-617 strongly positive, because the
answer depends on which foreign book happened to trade next. Across the three
cells the defective metric spans -37,540 to +68,801 basis points — a +688%
markout on a spot maker — and every corrected value falls within ±7 bps.

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

### The attestation's frame counts cannot detect a semantic change

The event frame count is identical across those 138 commits while the stream is
3.3% larger. That was first recorded as a curiosity; it is not one.

Re-run at 20 minutes, same config and seed, unpatched builds of both revisions:

| | `230e78f` | `a666d02` |
| --- | ---: | ---: |
| event frames | 1,640,870 | 1,640,870 |
| stream frames | 1,640,940 | 1,640,940 |
| `events.evs` | 294,335,937 B | 306,079,525 B |
| execution hash | `afe320fe…` | `4504d198…` |

Identical at 20 minutes and at 24 hours. The count is structurally determined by
the configuration rather than by the economics, so 138 commits of CDF depth,
concentration and activation work move it by zero.

`validateBinaryAttestation` compares `EventFrames` and `StreamFrames`. Both are
therefore **integrity checks — did the stream survive intact — with essentially
no power to detect a semantic change.** The one field that does move is the
execution hash, and §11 shows the hash also moves for a pure `GOAMD64` rebuild
with nothing semantic behind it.

So the two fields fail in opposite directions and neither separates the case
that matters:

| | moves on a semantic change | moves on a toolchain change |
| --- | --- | --- |
| frame counts | no | no |
| execution hash | yes | yes |

Nothing in the attestation distinguishes "the science changed" from "the build
changed". That is what makes the toolchain fields load-bearing rather than
housekeeping, and it is an argument for recording them even though this branch
already does — a reader comparing two runs needs to know that frame-count
agreement is not evidence of equivalence.

### And the hash moves on an evidence-format change too

Rendering both 20-minute runs and comparing them record by record settles what
the differing hash actually represents:

| | |
| --- | --- |
| records compared | 1,669,063 |
| value differences on fields present in both | **0** |
| new field occurrences | 1,922,654, all of them one field: `event_seq` |

Across those 138 commits, on this cell, every pre-existing field of every record
holds the same value, no event appears or disappears, and nothing is reordered.
One field was added to every event type. The 4.0% byte growth and the entirely
different execution hash are explained by that field alone.

**So the hash moved between two revisions whose simulated trajectory is
identical.** It conflates three different causes — a semantic change, a
toolchain change, and an evidence-format change — while the frame counts detect
none of the three. A hash comparison answers "were these bytes produced the same
way", not "did the simulation do the same thing", and only a content-level
comparison like this one separates them.

### 230e78f was not log-mode neutral, and this branch fixed it

`dev-607-none` is the logging parity control — `log_mode: none`, the `record_*`
recorders off, described in the config as "no economic treatment". Its whole
purpose is to show that logging does not perturb the trajectory, so its
execution hash must equal `dev-607`'s.

Four fresh 20-minute runs, seed 607:

| revision | `dev-607` | `dev-607-none` | neutral |
| --- | --- | --- | --- |
| `230e78f` | `afe320fe…` | `cda604d6…` | **no** |
| `a666d02` | `4504d198…` | `4504d198…` | yes |

All four produce 1,640,870 event frames. On `230e78f` turning logging off changed
the digest, so the parity control was not a parity control there: a cell and its
own control were different trajectories. Something in the 138 commits fixed it,
and this branch holds the invariant.

The consequence is archaeological rather than actionable — it is already fixed
here — but it bears on anything concluded on `230e78f` or earlier that used the
parity cell as a control or as a logging-neutrality check.

For completeness, the branch these commits came from holds the invariant in both
of its modes: `dev-607` and `dev-607-none` hash `b1f5e3a8…` in JSON mode and
`e1ad48f5…` in replace mode.

### It is not one cell: no development cell shows a trajectory difference

dev-607 was checked first, so the obvious question is whether it is unusual. It
is not. Every non-holdout cell, each at its own seed, 20 minutes, rendered and
compared record by record between the two revisions:

| cell | seed | records | value differences on shared fields | new fields |
| --- | ---: | ---: | ---: | --- |
| dev-607 | 607 | 1,669,063 | 0 | `event_seq` only |
| dev-613 | 613 | 1,600,210 | 0 | `event_seq` only |
| dev-617 | 617 | 1,607,487 | 0 | `event_seq` only |

Zero, on all three. Across those 138 commits the CDF depth, concentration and
activation work changes gates, audits and recording, and does not move the
simulated trajectory on any cell available here.

That is compatible with the work being exactly right — defensive gates and
fail-closed paths *should* be inert on runs that never violate them, and their
value is in what they would reject. What it means is that none of these cells
demonstrates the behaviour those gates exist to govern, so none of them can
serve as evidence that the gates do what they are meant to do.

**The holdouts are the gap and they were not touched.** 619, 631 and 641 are
excluded by operator instruction; they were neither read nor run. If the CDF
survival mechanism binds anywhere in this campaign, that is where to look, and
this report cannot say whether it does.

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
