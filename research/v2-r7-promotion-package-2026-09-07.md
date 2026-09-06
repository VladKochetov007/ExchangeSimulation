# Promotion package — performance and correctness, 2026-09-07

**Base revision:** `a666d02`, head of `feature/r2-cdf-survival-successor`, the
current scientific branch. Verified 0 behind / 12 ahead at the time of writing.
Nothing here is merged into that branch.

**Branch:** `perf/r2-cdf-survival-port`.

Three patches. Two are recommended for adoption as they stand; one is a
confirmed defect whose fix changes published metric values and is therefore not
mine to approve.

## Benchmark methodology

Everything below was measured on this base, not on an earlier one.

- **Isolation.** `cmd/multivenue` has no dependency on `analysis/`
  (`go list -deps` reports zero). Of the eight files this branch changes, only
  `types/fingerprint_fast.go` and `types/market_data.go` are in the simulator
  binary's dependency set, so an A/B between the two binaries measures the
  fingerprint patch and nothing else.
- **Pairing.** Each round runs both arms back to back, alternating which goes
  first by round parity, so drift and order effects cancel. Reported as the
  median of per-round relative deltas.
- **A/A control.** The same binary in both arms, same session, same cell. It
  bounds what the harness reports when nothing changed.
- **Mode.** `evstream_v3`, the configuration this branch ships.
- **Duration.** 20 simulated minutes per run, seeds matched to their cells.
- **Semantic gate.** Every arm's execution stream hash is recorded. A patch that
  moves it is not a performance patch.

## 1. Market-data fingerprint — `2797d0f`, `606889c`

**Finding.** `types/market_data.go` marshals a five-field envelope to JSON on
every market-data delivery to every participant, purely to feed SHA-256 and keep
16 bytes. Replaced with a canonical appender rendering exactly the bytes
`encoding/json` would, declining to the reflection path for any payload type or
symbol it does not handle.

**Benchmark.** Paired rounds, alternating order, `evstream_v3`, 20m:

| cell | seed | A/B median | direction | A/A median | A/A median abs |
| --- | ---: | ---: | --- | ---: | ---: |
| dev-607 | 607 | -2.01% | 15/16 faster | -0.16% | 0.50% |
| dev-613 | 613 | **-0.76%** | 10/12 faster | +0.51% | 0.51% |
| dev-617 | 617 | **-2.26%** | 12/12 faster | -0.02% | 1.35% |

Pooled direction: **37 of 40 paired rounds faster**. Splitting each cell by
which arm ran first leaves both halves negative, so this is not an order
artifact.

**Read it as roughly 1-2%, not as a single number.** The magnitude varies by
cell from 0.76% to 2.26%, and on dev-613 the effect is only about 1.5x its own
A/A median absolute delta. The direction is what is solid; the size is
workload-dependent. An earlier revision of this report quoted -5.00%, which was
the number from the branch the patch was written on and does not hold here.

**Semantic impact: none.** Execution hash identical across all four arms on each
cell — `2ee4b03c…` on dev-613, `4e248dcb…` on dev-617. All four market-data
artifacts byte-identical. Differential fuzz against the reflection path,
24.2M executions, zero mismatches. The fast path declines rather than guesses:
an unknown payload type or a symbol needing escaping takes the reflection path.

**Recommendation: adopt.** No semantic risk, no hash movement, and the direction
survives three seeds. The gain is modest and should be described as such.

## 2. Analyzer determinism — `815bda9`

**Finding.** Of 52 metrics surveyed at four runs each, exactly two answer
differently every time they are asked the same question. `reaction` gave 5
distinct outputs from 5 runs and was deterministic only at `GOMAXPROCS=1`;
`resting` reordered classes at equal medians. Causes: `Scan` reads venue files
on a worker pool and a book's records reach it from several files, so arrival
order varies; the trade tape was sorted on `sim_ts` alone, which is not a total
order over a tie-dense tape, and `sort.Slice` is not stable. Fills were never
sorted, and both the fill map and the book map were walked in randomised map
order.

**Fix.** Totally order on `(File, Ordinal)`, which `analysis.Event` already
carries for this purpose. Both metrics go from 5-of-5 and 4-of-5 distinct
outputs to 1-of-5.

**Benchmark: not applicable.** `analysis/` is not in the simulator binary. This
changes no runtime path.

**Semantic impact: the reported values move.** The fix replaces an unspecified
tie convention with a principled one; it does not restore a previous answer.

**Recommendation: adopt before the freeze.** A metric that answers differently
on each invocation cannot be frozen, and a comparison between two revisions
using it measures the tie order as much as the simulation. The value change is a
consequence of removing the nondeterminism, not a separate decision.

## 3. `reaction` marks fills against the wrong instrument — `e3c5d64`

**Finding.** Spot records carry no symbol in their data layer; only the
derivative nesting does, and the scanner unwraps only that. `reaction` keys
books on `markKey{VenueID, Symbol}` with no fallback and `mvanalyze` runs it
over every venue file, so all three spot books of a venue share the key
`{venue, ""}`. ABC/USD trades near 50.00 and CDF/USD trades near 3.00 occupy one
price tape, and the price one horizon after a maker's fill is whichever book
traded next.

`reaction` is the only book-keyed metric in the package without such a fallback;
`arbitrage`, `crossvenue` and `triangular` use `symbolFromPath(event.File)`,
`post_only` uses `symbolFromSpotFile`, six others fall back to a payload symbol,
and `surface` rejects non-options.

**Magnitude, three cells:**

| cell | role | before | after |
| --- | --- | ---: | ---: |
| dev-607 | `cdf_spot_maker` | -37,539.981 bps | -4.477 bps |
| dev-613 | `cdf_spot_maker` | -19,973.165 bps | -3.979 bps |
| dev-617 | `cdf_spot_maker` | **+68,800.736 bps** | -6.515 bps |

The sign is not stable across cells, because the answer depends on which foreign
book traded next. Across the three cells the defective metric spans -37,540 to
+68,801 basis points; every corrected value falls within 7 bps of zero. Fill
counts are unchanged in every row, and the derivative rows are bit-identical —
those records carry a symbol and must not move, and do not.

**Benchmark: not applicable.** Analyzer-only.

**Semantic impact: large, and it reaches published results.** Every spot-maker
markout on record was computed against a pooled tape.

**Recommendation: the defect is confirmed; adoption is the metric owner's
call.** Marking a fill against a different instrument is wrong on any reading,
and the fix uses the convention the rest of the package already uses. But it
changes published values by four orders of magnitude, and deciding what the
markout statistic should mean — and re-deriving whatever rested on it — is a
scientific judgement this package does not make.

## Not in this package

**Binary evidence remains experimental and is deliberately excluded.** The
render fixes developed on `perf/ffa-gen0-port` are superseded: this branch
already removes the quadratic scan, already renders as a streaming merge, and
already records the toolchain in the manifest — in the ordering check, more
strictly than that series did. Nothing from that work is proposed for the
freeze, and the full semantic and evidence contract has not been independently
validated.

## Reproduction

`WORK` is a scratch directory outside the repository.

```bash
git worktree add --detach "$BASE" a666d02
git worktree add --detach "$FIXED" perf/r2-cdf-survival-port
(cd "$BASE"  && go build -o "$WORK/base"  ./cmd/multivenue)
(cd "$FIXED" && go build -o "$WORK/fixed" ./cmd/multivenue)

# paired A/B, alternating order; A/A replaces "fixed" with a second "base"
for i in $(seq 1 12); do
  for arm in base fixed; do
    "$WORK/$arm" -config research/configs/v2-integrated-longrun/dev-617.json \
      -duration 20m -seed 617 -evidence-format evstream_v3 -logdir "$WORK/r"
    # record wall time and execution_stream_hash from
    # "$WORK/r/binary-evidence-attestation.json", then remove "$WORK/r"
  done
done
```

The isolation claim is checkable directly:

```bash
go list -deps ./cmd/multivenue | grep -c exchange_sim/analysis   # 0
```
