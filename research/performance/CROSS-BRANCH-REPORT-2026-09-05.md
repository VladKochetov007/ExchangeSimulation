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

## 2. Their renderer was quadratic — fixed, 25.2x

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

Peak RSS is untouched by this and is the **second, separate blocker**: it is
linear in frames because the renderer holds the whole rendered run before
writing any of it, and 2.6 GB at one hour extrapolates to roughly 62 GB at
24 hours — the total RAM of the machine these runs are measured on. Rendering a
24-hour run is now fast enough and still will not fit. A streaming merge would
not need to hold it; this branch's superseded renderer held only the
evidence-only records, some tens of thousands of lines.

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
conflicting, and their full suite is green with it applied. A paired A/B on
their branch to confirm the figure in their configuration has not been run yet.

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

- Whether the markout tie convention should be written order at all (§3).
- Peak render RSS is linear in frames and untouched (§2).
- The -5.00% figure is measured on this branch, not theirs (§4).

## 7. Reproduction

```bash
# independent verification of their reconstruction
git worktree add --detach /tmp/sci github/autoresearch/ffa-ecology-gen0
cd /tmp/sci && go build -o /tmp/mv ./cmd/multivenue && go build -o /tmp/evsrender ./cmd/evsrender
CFG=research/configs/v2-integrated-longrun/dev-607.json
/tmp/mv -config $CFG -duration 20m -seed 607 -logdir /tmp/json
/tmp/mv -config $CFG -duration 20m -seed 607 -evidence-format evstream_v3 -logdir /tmp/bin
/tmp/evsrender -dir /tmp/bin -out /tmp/rebuilt
for f in $(cd /tmp/json && find venues -name '*.jsonl'); do
  cmp /tmp/json/$f <(sed -E 's/("data":\{"venue_id":"[^"]*",)"sequence":[0-9]+,/\1/' /tmp/rebuilt/$f)
done

# the analyzer determinism survey
for i in 1 2 3 4 5; do ./bin/mvanalyze -metric reaction /tmp/json > /tmp/r$i; done
md5sum /tmp/r* | awk '{print $1}' | sort -u | wc -l   # 5 before the fix, 1 after
```

The four commits are on `perf/ffa-gen0-port`, applied to `230e78f` in review
order: the fingerprint optimization, its fuzz test, the analyzer determinism
fix, and the renderer complexity fix. Nothing is merged into
`autoresearch/ffa-ecology-gen0`.

## 8. Next experiment with the highest information gain

A paired A/B of the fingerprint port on their branch, with an A/A control in the
same session, to state the -5.00% in their configuration rather than transfer it
from this one.
