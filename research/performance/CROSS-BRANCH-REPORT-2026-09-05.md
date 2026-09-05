# Cross-branch findings, 2026-09-05

## What changed

The scientific branch `autoresearch/ffa-ecology-gen0` (`230e78f`) has independently
built its own binary evidence stack — canonical stream, typed schemas, route
preservation, a renderer, fail-closed gates, a CLI flag — from the same
merge-base as this branch, `887899f`. Neither branch's commits appear in the
other's history:

```
$ git merge-base autoresearch/v2-performance-research github/autoresearch/ffa-ecology-gen0
887899f feat: add lossless long-run evidence archiving
$ git merge-base --is-ancestor 6901edb github/autoresearch/ffa-ecology-gen0; echo $?
1
```

Their format is `evstream_v3`; this branch's is `evstream_v1`. Two independent
implementations of one mechanism is the strongest verification surface this
campaign has had, and it is the reason this report exists: everything below is
their implementation tested from outside it, or a defect found in code both
branches share.

## 1. Their reconstruction is exact — independently verified

Their own test suite checks their renderer. This is an outside check: run
dev-607/seed 607/20m twice from the same binary, once in JSON mode and once as
`evstream_v3`, render the binary run, and compare the rebuilt tree to the JSON
tree file by file.

| check | result |
| --- | --- |
| file set | 15 / 15 match |
| byte-identical to the JSON-mode run | 0 / 15 |
| byte-identical after removing the `"sequence":N` their renderer adds | **15 / 15**, 1,669,063 records |

The reconstruction is sound. The caveat is the field: their renderer emits
`"sequence":N` into every record's `data` object and their JSON-mode logger does
not, so their rebuilt tree cannot be byte-compared against the historical
format without normalising it first. Analyzers ignore the extra field, so
nothing is broken; the attestation is simply weaker than it could be.

## 2. Their renderer was quadratic and held the whole run in memory — both fixed

`simulations/multivenue/evidence_render.go`, `addRenderRecord`, re-checked the
per-venue sequence contract by scanning every record already appended to the
route, once per record.

| run | frames | `events.evs` | wall, before | wall, after | speedup | peak RSS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 5m | 382,498 | 68.9 MB | 7.83 s | 1.18 s | 6.6x | 223-228 MB |
| 20m | 1,640,870 | 294.3 MB | 132.90 s | 4.71 s | **28.2x** | 909-923 MB |
| 60m | ~4.9 M | 875.8 MB | 1118.75 s | 14.67 s | **76.3x** | 2.20-2.60 GB |

Before the fix the wall-time exponent is 1.95 over both intervals — 17.0x for
4.29x the records, then 8.42x for 2.98x more. After it, wall time is linear and
the speedup grows with the run, which is what a quadratic-to-linear fix does.

Extrapolating to the 24-hour runs their campaign needs — their own 24-hour
capacity probe emitted 30,212,381,584 bytes of binary output, so this length is
real — rendering one run goes from about **6.4 days to about 6 minutes**.

The scan was also redundant, and weaker than what already followed it:
`validateRenderRecords` enforces the same contract with a set, per venue rather
than per route, so a sequence claimed by two routes of one venue passes the
removed scan and fails the surviving check. Nothing tested either of them.

All 15 output files are byte-identical to the previous renderer's and the
execution hash is unchanged at `afe320fe…0895`. Four tests now cover the
contract directly.

Peak RSS was untouched by that and was the second, separate blocker: linear in
frames, because the renderer held the whole rendered run before writing any of
it. 2.6 GB at one hour extrapolates to roughly 62 GB at 24 hours, the total RAM
of the machine these runs are measured on — fast enough and still not fitting.

**Also fixed, as a streaming merge.** Frames arrive in per-route sequence order
and each sidecar file is written in sequence order, so both inputs are already
sorted. Measured on a real run: of 1,640,870 frames across 15 routes, zero
arrive out of order within a route or within a venue, and zero of 28,193 sidecar
lines are out of order within their file. Two structures made the buffering look
necessary and did not — the evidence-only digest is an addition of per-record
hashes declared `unordered_multiset`, so it does not care what order records are
folded in, and the per-venue sequence contract was enforced with a set holding
every sequence where a bitmap answers the same questions in one bit each.

| run | frames | peak RSS before | after | wall before | after |
| --- | ---: | ---: | ---: | ---: | ---: |
| 5m | 382,498 | 223 MB | 13.8 MB | 1.06 s | 0.99 s |
| 20m | 1,640,870 | 922 MB | 12.7 MB | 4.80 s | 4.22 s |
| 60m | 4,859,415 | 2.60 GB | 14.2 MB | 14.75 s | 12.83 s |
| 6h | 26,682,865 | **15.14 GB** | **21.9 MB** | 83.7 s | 77.3 s |
| 24h | 96,241,273 | not attempted, ~60 GB | **47 MB** | — | **285.6 s** |

Output is byte-identical at every size — 15 of 15 files, 27,173,903 records at
six hours, same event frame counts, route counts and execution hashes.

The six-hour point is what makes this more than an extrapolation. The buffered
renderer needs 15.14 GB there, which scales to about 60 GB at 24 hours and so
confirms the earlier estimate rather than softening it. The streaming renderer
needs 21.9 MB, a 690x reduction, and its slow growth from 14.2 MB is the
sequence bitmap and nothing else.

It also tests the invariant the merge rests on at 16x the scale it was measured
at. Rendering now refuses a route whose sequences go backwards rather than
assuming they cannot, and across 26,682,865 frames the check never fired.

## 3. Two analyzer metrics answered differently every time — fixed

52 metrics, four runs each over one run directory, at real parallelism. Exactly
two are not reproducible, and both are live on `230e78f`:

**`reaction`** — 5 distinct outputs from 5 runs; deterministic at
`GOMAXPROCS=1`. `Scan` reads venue files on a worker pool and a book's records
reach it from more than one file, because `Symbol` comes from the record and not
the path. The trade tape was then sorted on `sim_ts` alone, which over a
tie-dense tape is not a total order, and `sort.Slice` is not stable, so
`sort.Search` returned a different price each run. Fills were never sorted, and
both the fill map and the book map were walked in Go's randomised map order, so
the floating-point sums varied on top of that.

**`resting`** — `RolesByDistance` sorted map-derived names on median alone, so
classes at equal medians swapped places. Presentation only, but it breaks
artifact comparison.

Both fixed on this branch (`7a931f4`) by totally ordering on `(File, Ordinal)`,
which `analysis.Event` has always carried for this purpose. Each metric went
from 5-of-5 and 4-of-5 distinct outputs to 1-of-5. The fix cherry-picks onto
`230e78f` cleanly and is verified there against their own run directory.

### The value moves, and that is the more interesting result

The fix does not restore a previous answer. On dev-607/seed 607/20m,
`central/abc_cdf_spot_maker`:

| | mean markout | picked off |
| --- | ---: | ---: |
| pre-fix, `GOMAXPROCS=1`, 3 runs | -2194.549 bps | 0.491 |
| pre-fix, default, 12 runs | -1277 … -2301 bps | 0.484 … 0.513 |
| fixed, any `GOMAXPROCS` | **+82.368 bps** | 0.408 |

The corrected value lies outside the whole pre-fix distribution and on the other
side of zero. An earlier commit message on this branch claimed the fixed value
equalled the `GOMAXPROCS=1` answer; it does not, and the check behind that
sentence compared the fixed binary against itself. Corrected in `f9c77a3`.

What this says is that the markout statistic is extremely sensitive to which of
several same-instant trades is called "the price at the horizon" — a ~2280 bps
swing and a sign change from a tie convention that was never specified. Ordering
by the order records were written is defensible and is what the fix does, but
whether that is the right definition is a question about the metric, not about
the fix, and belongs to whoever owns it.

## 4. A -5.00% optimization their branch does not have

`types/market_data.go:19` still marshals a struct to JSON on every market-data
delivery to every participant, purely to feed SHA-256. This branch replaced that
with a canonical appender in `43f86f2`: -5.00%, all four market-data artifacts
byte-identical, execution hash unchanged, differential-fuzzed against the
reflection path for 24.2M executions.

It cherry-picks onto `230e78f` with only a file that does not exist there
conflicting, and their full suite is green with it applied.

Measured on their branch, 24 paired rounds with the arm order alternating by
round parity, dev-607/seed 607/20m in **JSON mode**:

| | |
| --- | --- |
| base median | 19.355 s |
| ported median | 18.980 s |
| paired delta | median **-1.52%**, mean -1.62%, range -5.12% to +5.70% |
| ported faster in | 22 of 24 rounds |
| execution hash | `fb16fc252a051ced` in both arms |

The pairing holds up under the checks that would break it. Splitting by which
arm ran first gives -1.46% (12 rounds, base first) against -1.85% (12 rounds,
ported first), so there is no material order effect for the alternation to have
missed; splitting by time gives -1.77% for rounds 1-12 against -1.49% for
13-24, so there is no drift.

An A/A control of 12 rounds in the same session, same binary in both arms, gives
a median of `-0.31%` and a mean of `+0.28%`, with the second arm faster in 7 of
12 rounds — directionless, as a control should be. Per-round noise is large: the
A/A median absolute delta is 1.15%, comparable to the effect itself, so the
median alone would not carry this claim. The sign test does: 22 of 24 in one
direction is `p ~ 1.8e-5`, against 7 of 12 for the control. The effect is real
and small.

That is a third of the -5.00% claimed here, and the two figures are not in
conflict: the original was measured in **replace mode**, where the profile put
`MarketDataFingerprint` at 3.16% of CPU precisely because the JSON evidence
encoding it competes with had been removed. Measuring it in JSON mode
understates it by construction. So the transferability question is about
`evstream_v3` mode, which is the configuration their campaign will ship.

Measured there, 16 paired rounds, same alternation:

| | |
| --- | --- |
| base median | 14.747 s |
| ported median | 14.466 s |
| paired delta | median **-2.01%**, mean -2.13%, range -5.30% to +1.09% |
| ported faster in | 15 of 16 rounds |
| base-first / ported-first | -2.00% / -2.10%, so again no order effect |
| execution hash | `c172d74803571a17` in both arms |

An A/A control of 10 rounds in the same mode and session gives a median of
`-0.16%`, a mean of `-0.24%` and a median absolute delta of 0.50%, with the
second arm faster in 7 of 10 — directionless. The effect is four times that
floor and 15 of 16 rounds is `p ~ 2.6e-4`, so this is the cleaner of the two
measurements.

**The -5.00% measured on this branch does not transfer to theirs; on their code
the same patch is worth about 2%.** A profile of both baselines says exactly why:

| branch | mode | `MarketDataFingerprint` | measured gain |
| --- | --- | --- | ---: |
| this branch at `43f86f2^` | replace | 0.60 s, 5.52% cum | -5.00% |
| `230e78f` | evstream_v3 | 0.63 s, 3.26% cum | -2.01% |

The function costs **the same absolute time on both branches**. It is a smaller
fraction of theirs because their run does roughly 1.8x the total CPU work. Each
gain sits at or below its own function's share, so nothing here exceeds its
ceiling and the mechanism survives intact — what does not transfer is the
percentage, because a percentage is a property of the denominator.

That also retracts a suspicion raised while measuring this: -5.00% looked
impossible against the 3.16% this campaign recorded for the same function, and
the resolution is that 3.16% was an under-sampled figure. A fresh profile of
that same commit puts it at 5.52%. The commit message's number should be read as
one sample, not as the function's cost.

## 5. What of this branch's work is superseded

The `evstream_v1` routing work in progress here — a stream reference in the sink
envelope plus a layout renderer — solved the same problem their `evstream_v3`
already solved, in a shipped and better-tested form. It is dropped rather than
pushed, to avoid forking the format. Two things from it survive as observations
worth their attention: their renderer buffers the whole rendered run in memory
where a streaming merge does not have to, and their added `sequence` field costs
them byte-comparability against the historical format.

The evidence-only ordering key (`b5c2a25`) also becomes moot on their branch:
they give every record a per-venue sequence, which subsumes it.

## 6. Live uncertainties

- Nothing blocks a 24-hour binary render. It is measured, not extrapolated
  (§2, §10): 4 minutes 46 seconds and 47 MB.
- Why their 24-hour probe failed and this one did not is open (§10). The
  toolchain is the leading candidate and is not recorded in the manifest, so
  the artifacts cannot settle it.
- Whether the markout tie convention should be written order at all (§3). This
  matters far less now that fills are marked against their own book — the
  remaining ties are between trades in one instrument — but it is still an
  unspecified choice.
- The lag arm of `reaction` reports a pooled mean of 3.2 microseconds and did
  not move when the books were separated (§9). Nothing here explains what it is
  supposed to measure on a simulator whose orders arrive in the same instant as
  the changes they follow; that question belongs to whoever owns the metric.
- Every markout figure the campaign has published for a spot maker was computed
  against a pooled tape. The corrected values are single-digit basis points
  where the old ones were tens of thousands, so any conclusion that rested on
  spot maker markout needs re-deriving. Nothing in this report does.

## 7. Reproduction

`WORK` is any scratch directory outside the repository, and `SCI` a worktree of
the scientific branch.

```bash
# independent verification of their reconstruction
git worktree add --detach "$SCI" github/autoresearch/ffa-ecology-gen0
cd "$SCI" && go build -o "$WORK/mv" ./cmd/multivenue && go build -o "$WORK/evsrender" ./cmd/evsrender
CFG=research/configs/v2-integrated-longrun/dev-607.json
"$WORK/mv" -config $CFG -duration 20m -seed 607 -logdir "$WORK/json"
"$WORK/mv" -config $CFG -duration 20m -seed 607 -evidence-format evstream_v3 -logdir "$WORK/bin"
"$WORK/evsrender" -dir "$WORK/bin" -out "$WORK/rebuilt"
for f in $(cd "$WORK/json" && find venues -name '*.jsonl'); do
  cmp "$WORK/json/$f" <(sed -E 's/("data":\{"venue_id":"[^"]*",)"sequence":[0-9]+,/\1/' "$WORK/rebuilt/$f")
done

# the analyzer determinism survey
for i in 1 2 3 4 5; do ./bin/mvanalyze -metric reaction "$WORK/json" > "$WORK/r$i"; done
md5sum "$WORK"/r* | awk '{print $1}' | sort -u | wc -l   # 5 before the fix, 1 after
```

The four commits are on `perf/ffa-gen0-port`, applied to `230e78f` in review
order: the fingerprint optimization, its fuzz test, the analyzer determinism
fix, and the renderer complexity fix. Nothing is merged into
`autoresearch/ffa-ecology-gen0`.

## 8. Next experiment with the highest information gain

Re-derive whichever published results depended on spot-maker markout, now that
the figure has moved by four orders of magnitude (§9). That is a larger change
to the scientific record than anything else here, and it is the one thing this
branch cannot check for itself, because it does not own those conclusions.

After that, the same 24-hour cell built with the Go toolchain their probe used.
That is the one experiment that would separate a toolchain-dependent
market-survival outcome from a configuration difference, and the answer changes
whether a scientific gate can be trusted across build environments.

## 9. The reaction metric marks fills against the wrong instrument

Spot records carry no symbol in their data layer; only the derivative nesting
does, and both branches' scanners unwrap only that. `reaction` keys books on
`markKey{event.VenueID, event.Symbol}` with no fallback, and `mvanalyze` invokes
it over every venue file. So all three spot books of a venue share the key
`{venue, ""}`: ABC/USD trades near 50.00, CDF/USD trades near 3.00 and ABC/CDF
trades occupy one price tape, and the price one horizon after a maker's fill is
whichever book happened to trade next.

`(3.00 - 50.00) / 50.00 x 10000 = -9400 bps` is the scale of the error, and a
live dev-607/seed 607/20m measurement on the scientific branch reports per-role
markouts of `-36,992` and `-24,514` bps — magnitudes no maker on a spot book
can produce. The `f2_baseline_101` scoreboard artifact was checked only for the
empty-symbol book, which it has, with 2,324,523 observations under
`{central, ""}`; its markout values were not re-derived here.

`mvanalyze` runs six metrics with no file narrowing — `makerquotesize`,
`makerrefresh`, `basis`, `reaction`, `roles` and `lifecycle` — and `reaction` is
the only one of them that keys a book on a symbol with no fallback. `basis` and
`lifecycle` return early on an empty symbol, `roles` falls back to the fill's
symbol, and both maker-quote metrics fall back to `symbolFromSpotFile`. Every
other book-keyed analyzer guards it too. `arbitrage`, `crossvenue`
and `triangular` fall back to `symbolFromPath(event.File)`; `post_only` to
`symbolFromSpotFile`; `hedging`, `roleaudit`, `viability`, `term_carry_p4`,
`exposure` and `options_p6` to a payload symbol; `surface` rejects anything that
is not an option. `reaction` was the only one with no fallback, so this is an
omission against a convention the package applies everywhere else, not a design
choice. The fix uses the majority form.

On a fixture where the foreign book sorts first, the markout reads **+9400.000
bps before and -1.000 bps after**.

This also reframes §3. The tie order mattered by thousands of basis points
*because* the tied candidates were different instruments. Making the metric
reproducible was necessary and was not sufficient.

### Pre-registered predictions for the real-data check

Stated before measuring on dev-607/seed 607/20m, so the check can fail:

1. The lag arm currently reports `mean 0.0000s, p50 0.0000s, min 0.0000s`
   because a pooled tape almost always has an order from *some* book at the same
   instant. After the fix the pooled lag should rise above zero.
2. Per-role markout magnitudes should fall from tens of thousands of basis
   points to plausible ones, since a fill is no longer marked against a book at
   a different price level.
3. Fill counts should not change at all: the fix does not touch which records
   are fills.

A result that leaves the lag at zero, or leaves markouts in the tens of
thousands, falsifies the diagnosis rather than confirming it.

### Outcome: two confirmed, one falsified

Measured on dev-607/seed 607/20m.

| role | before | after |
| --- | ---: | ---: |
| `cdf_spot_maker` | -37,763.985 bps | **-4.477 bps** |
| `imbalance_maker` | -31,305.874 bps | **-4.009 bps** |
| `fixed_distance_maker` | -24,940.754 bps | **-3.417 bps** |
| `abc_cdf_spot_maker` | -1,159.630 bps | **-1.570 bps** |
| `spot_maker` | — | -0.340 bps |
| `futures_maker` (derivative) | 3.463 bps | 3.463 bps |
| `option_dealer` (derivative) | -8,985.718 bps | -8,985.718 bps |

Prediction 2 holds: spot markouts fall to single-digit basis points, and pooled
markout goes from `-13,950.28` to `-762.26` bps. Prediction 3 holds: every fill
count is unchanged. The derivative rows are bit-identical, which is the control
that matters — those records carry a symbol, so the fix must not touch them, and
it does not. The book table gains exactly the six rows it should: 72 to 78, with
the `""` bucket replaced by `ABC-USD`, `CDF-USD` and `ABC-CDF` in each of three
venues.

**Prediction 1 is falsified.** The lag arm is bit-identical either way —
312,597 observations and a pooled mean of `3.199007028218441e-06` seconds before
and after. The lag was never zero because books were pooled; it is zero because
orders arrive within microseconds of the book changes they follow, which the
simulator's phase structure makes true within a single book as well. That is a
separate emptiness in this metric's lag arm and the book key does not touch it.
It is recorded here rather than quietly dropped.

## 10. A 24-hour run completes, and their capacity probe's failure does not reproduce

Their `2026-08-31 capacity probe outcome` note records a 24-hour run exiting
with status 1 at the terminal population mark:

```text
multivenue: terminal_post_mark participant valuation requires two-sided CDF/USD
mark on venue south: no usable price
```

It calls this "a durable unpriceable-exposure/market-survival condition, not
merely the expected transient cancellation at the final ingress boundary", and
blocks a new capacity measurement on adjudicating it.

Run here on dev-607/seed 607 for 24 hours in `evstream_v3` mode, it does not
happen:

| | |
| --- | --- |
| simulation | exit 0, `done: sim=24h0m0s wall=15m30s`, peak RSS 1.65 GB |
| `events.evs` | 17,434,418,871 bytes; whole run directory 23 GB |
| render | exit 0, **4 min 45.63 s**, peak RSS **47 MB** |
| reconstructed | 96,241,273 event frames, 15 routes, 98,181,443 records, 32 GB |
| execution hash | `2806c51c30039e46ed4f9cf5ab0fe7f6e03f0b958e957fed90768187493b26dc` |

**This is not a fix, and must not be read as one.** Between `95596f1`, the
revision their probe used, and `230e78f`, the head this ran on, the diff is two
documentation files and 159 insertions. Zero lines of code. The binary that
succeeded is the binary that failed.

So the difference is in how the two runs were set up, and the candidates are:

- **The Go toolchain.** Their note says Go 1.27; this used `go1.26.7`. This
  campaign has already shown the toolchain changes the trajectory —
  `GOAMD64=v3` fuses multiply-add, shifts `log_variance` by two ulp and changes
  the execution hash. A compiler version can do the same, and a market-survival
  outcome sits downstream of exactly those floating-point paths. Only one
  toolchain is installed here, so this was not testable.
- **The seed.** Their note does not record which seed the probe used. This ran
  seed 607.
- **The configuration.** Their note says the dev-607 config was "copied only to
  redirect its log directory". This used the in-tree file unmodified.

**The artifacts cannot distinguish these.** Their manifest records
`config.seed` but no `go_version`, `GOARCH`, `GOOS` or `GOAMD64` — checked
directly on this run's `manifest.json`. That is the provenance gap §7 of the
promotion memo already described, and here it is load-bearing: a scientific gate
was declared blocked on a durable market-survival mechanism, the same code does
not reproduce it, and nothing in the evidence says why.

What is established: the failure is not a property of this code at 24 hours on
the default cell. What is not established: which of the three differences causes
it. Deciding that is worth more than the capacity number it was blocking.

## 11. GOAMD64 is not the explanation, and the hash divergence is now quantified at 24 hours

§10 left three candidates for why their probe failed where this run did not, and
named the toolchain as the leading one. The compiler version could not be tested
— only `go1.26.7` is installed — but the codegen level could, because this
campaign already showed `GOAMD64=v3` fuses multiply-add and changes the
execution hash at three minutes.

The same 24-hour cell, same source, same seed, built at each level:

| | `GOAMD64=v1` (default) | `GOAMD64=v3` |
| --- | --- | --- |
| simulation | exit 0, `sim=24h0m0s`, wall 15m30s | exit 0, `sim=24h0m0s`, wall 15m26s |
| event frames | 96,241,273 | **96,241,273** |
| `events.evs` | 17,434,418,871 bytes | 17,434,421,574 bytes |
| execution hash | `2806c51c…b26dc` | **`2f00c608…a001f`** |

**The market-survival gate passes under both.** So codegen does not explain
their probe's failure, and the toolchain hypothesis survives only as the
compiler *version*, which remains untested. The candidates are now the Go
version, the seed their note does not record, and their copied configuration.

What the ablation does establish is a much tighter characterisation of the
divergence than the three-minute observation this campaign recorded. The two
streams first differ at byte 12,584,428 of 17.4 GB — 0.072% in, so within the
opening minutes of simulated time — and never reconverge. Yet across
twenty-four simulated hours they produce **exactly the same number of events**,
and their total sizes differ by 2,703 bytes, 0.0000155%.

The drift is numerical, not structural. No event is added, dropped or reordered;
values move in the last bits and the digest that covers them moves with it. That
is the worst shape this could take for a reproduction check: two builds of one
revision agree on everything a person would look at and disagree on the one
number the attestation compares. Without `GOAMD64` in the manifest — and it is
not there — a failed reproduction is indistinguishable from a semantic change.
