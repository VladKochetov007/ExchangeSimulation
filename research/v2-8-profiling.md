# V2-8 performance gate — 2026-08-24

## Decision

Gate passed for methodology, not for a production optimization. Profiles locate
material work; no simulator, scheduler, RNG, matching, or evidence semantics
changed in this gate. V2 market experiments may resume only with the profile
artifacts retained. Do not replace `encoding/json` yet.

Profiles use Go `go1.26.6-X:nodwarf5` on this host. Percentages below are
inclusive pprof percentages and overlap; they are attribution aids, not a
partition summing to 100%.

## Simulator sample

| Field | Full evidence | Raw JSON logs disabled |
| --- | ---: | ---: |
| Config | `frozen-baseline-2026-08-22.json` | same |
| Config SHA-256 | `ca933bf2244eec8e104d4313456bed386809703bb7a1179125b8d9255f1b1036` | same |
| Seed / simulated horizon | 101 / 30 minutes | 101 / 30 minutes |
| `GOMAXPROCS` | 1 | 1 |
| V2 receipt evidence | `spot_maker`, enabled | `spot_maker`, enabled |
| Wall time | 32.25s | 24.92s |
| Simulated seconds per wall second | 55.8 | 72.2 |
| Peak RSS | 802MB | 811MB |
| Sampled allocation | 10.77GB | 9.37GB |
| GC cycles | 24 | 21 |

`log_mode=none` still computes the ordered execution hash/checkpoints and
receipt sidecars. Thus this difference isolates raw JSON evidence persistence,
not all observation infrastructure. Full raw evidence costs about 7.33s wall
and 1.40GB sampled allocation in this sample. The run's code baseline was
`c5947b7`; all passive-refresh P0 switches remained false.

Raw profiles and timing data are retained under
`research/artifacts/v2-8-profiling/simulator-current-30m-{full,none}-receipts-seed101/`.
For example, full CPU and allocation profile SHA-256 values are
`ab3d3aadd1d9d0416c948ced90e093c17a5faa5a3dfdb94aa16c94692ca26467` and
`4900fdb34b3b063887ed522fe38795e1dd47447534fe6322ab00eef0f2cff46d`.

| Component | CPU / allocation evidence | Wall contribution | Candidate | Scientific / semantic risk |
| --- | --- | --- | --- | --- |
| Raw evidence JSON encode and write | `encoding/json.Marshal` 23.7% CPU, 20.2% allocation; logger 15.1% CPU | 7.33s of 32.25s relative to logs-off | Decoder/encoder replacement only after byte-contract proof | High: changes persisted bytes and evidence digest unless proved identical |
| Ordered execution checkpoint | 14.0% CPU, 9.5% allocation inclusive | present in both modes | Canonical binary encoder only after a new hash-domain contract | High: reproducibility attestation input |
| Order admission and matching | `PlaceOrder` 39.2% CPU inclusive; `processExecutions` 20.6% CPU inclusive | core engine work | preserve detached matcher proof; explore reuse only with digest-equivalence tests | High: fills, fee preflight, FOK and self-trade semantics |
| Detached preview clone | 4.5% CPU, 12.9% allocation; clone itself 4.0% CPU, 11.7% allocation | material allocation, not primary CPU cost | pooled/resettable detached books, only after ownership proof | High: matcher mutates queue links and iceberg state |
| Market-data courier/cache | 3.5% CPU; 6.8% allocation | modest | reuse copies only if receipt/frontier evidence stays identical | High: information boundary and delivery ordering |
| Scheduler/event queue | 4.7% CPU, 5.9% allocation | modest | no current change | High: ordering is economic input |
| Risk/margin/liquidation | position-margin sweep 4.0% CPU; liquidation 3.3% CPU | modest | profile again under reachable distress | Medium: current population rarely stresses path |
| Actor decisions / option Greeks | actor tick 2.8% CPU; no Greek hot node in this sample | not material here | profile option-heavy O-stage separately | Medium: current sample underweights option chain |
| Mutex / block | no application mutex contention; block profile is trace/runtime sleep | no action | none | N/A |

GC is material but allocation-driven: full run allocates about 334MB/s and the
CPU profile assigns 14.5% cumulatively to `runtime.mallocgc`; no pause-driven
application lock bottleneck appears. The trace's scheduler delay is dominated
by runtime background sweep activity, so it is not a simulator queue delay.

Historical profile data from before `d291525` allocated about 41GB for a
similar 30-minute full run. Current 10.77GB is consistent with the detached
book capacity fix being valuable, but this is not a controlled before/after
claim because source revisions differ. Do not use it as a performance result.

## Offline analyzer sample

`mvanalyze -metric crossvenue` replayed retained 30-minute full JSON evidence
from `scratch/v20-profile.aBp4H3/full-on/run` using current analyzer code,
`GOMAXPROCS=14`, 3 venues and `ABC-USD`.

| Field | Result |
| --- | ---: |
| Evidence files / raw size | retained 30-minute full-evidence run / about 644MB per pass |
| Wall / CPU | 0.95s / 3.56s |
| Peak RSS | 69MB |
| Sampled allocation | 367MB |
| Allocation objects | 5.18M |
| JSON unmarshal CPU | 64.0% inclusive |
| Event prefilter CPU | 20.4% |
| File reads CPU | 6.6% |
| Metric-specific payload decode CPU | 21.7% inclusive |
| Mutex contention | none attributable to analyzer logic |

Raw profile data: `research/artifacts/v2-8-profiling/analyzer-crossvenue-30m-full-on-seed101/`.
`mvanalyze` now has opt-in CPU, allocation, block, and mutex profile flags;
these profile only the offline command process.

## JSON decoder differential screen

Tool: `tools/jsonbench` at commit `c5947b7`, retained input
`scratch/v20-profile.aBp4H3/full-on/run`, three cached passes, 6,380,073
records / 1.93GiB total. Before timing a candidate, it must match
`encoding/json` for every retained record and fixtures covering malformed JSON,
large payload, signed/unsigned integer boundaries, and `RawMessage` copy
behavior.

| Decoder | Compatibility screen | Throughput | Allocation | Decision |
| --- | --- | ---: | ---: | --- |
| `encoding/json` | reference | 107.1 MiB/s | 3.79GiB | retain |
| `goccy/go-json` v0.10.6 | **rejected**: accepts overflowing `int64` fixture standard rejects | not timed | N/A | exclude |
| `json-iterator/go` compatible v1.1.12 | passed this bounded screen | 247.1 MiB/s | 3.06GiB | no adoption yet |
| `sonic.ConfigStd` v1.15.2 | passed this bounded screen | 388.7 MiB/s | 4.27GiB | no adoption yet |

Results: `research/artifacts/v2-8-profiling/json-decoder-30m-full-on-seed101-r2/`.
This does **not** establish full `encoding/json` equivalence. In particular, a
production logger replacement must additionally prove raw record bytes,
map ordering, malformed-record behavior, error handling, and runtime/offline
evidence-digest agreement across a deterministic run. Until then, speed is not
permission to change an evidence contract. External implementation claims also
warn that compatibility differs on invalid input and raw values; local
differential tests are controlling evidence.

The structural prefilter check compared current `analysis.bytesContains` with
`bytes.Contains` over 10,633,910 retained lines. Both selected 704,655 lines;
standard library search was 1.11x faster (1409.5 vs 1271.7 MiB/s). This is too
small at whole-analyzer scale to justify a production change alone. Results:
`research/artifacts/v2-8-profiling/filter-30m-full-receipts-seed101/`.

## Follow-up rules

1. Do not rewrite scheduler, matching, or evidence encoding from this profile.
2. Do not add `go-json`, `jsoniter`, or Sonic to simulator runtime imports.
3. Any preview-book reuse prototype needs identical execution hashes, raw
   evidence digests, accounting/lifecycle tests, and a measured before/after
   profile.
4. Re-profile option-heavy O-stage and liquidation-reachable worlds before
   optimizing those paths.
5. Continue V2-3 P0 only with its A/B/C causal separation and fixed viability
   gates; profiling does not alter its economics or preregistration.

## Reprofile after signed-price merge — 2026-08-24

The signed-price branch merged as `320262e`. The relevant V2-8 profiles were
re-run from that merged code, rather than relying on pre-merge percentages.
This is the same retained 30-minute seed-101 baseline workload, with full raw
logs, `spot_maker` receipt evidence, 60-second execution checkpoints, and
`GOMAXPROCS=1` for the simulator. The terminal ordered execution hash remained
`1eb482c7d5a21a08092c751252ca31dc6e4a0b8decf50fedefa08b2904afb2c7` at
2,126,782 observations; the persisted evidence digest remained
`c530af24a5c75950e2090b95f858c271d70ab47e55b1e44d66ec531885f7bb75`.

| Component | Merged measurement | Interpretation |
| --- | ---: | --- |
| simulator wall time | 30.22 s / 59.56 simulated seconds per wall second | Within the prior full-log profile range; no signed-price regression signal. |
| simulator peak RSS / sampled allocation | 808,916 KiB / 10.83 GB | Logging allocation remains material. |
| order admission | `PlaceOrder` 41.1% cumulative CPU | Still the largest core-engine target; no safe optimization proposed. |
| matching/settlement | `processExecutions` 23.0% cumulative CPU | Coupled to accounting and preview correctness. |
| JSON logging | `encoding/json.Marshal` 25.7% cumulative CPU; 19.8% sampled allocation | Still material, but persisted byte semantics rule out an intuition-driven encoder swap. |
| detached preview | 12.5% cumulative sampled allocation | Material allocation, but pooling remains high semantic risk. |
| scheduler/courier | no new standalone material hotspot | Do not alter event ordering or receipt paths. |
| block/mutex | no application contention | No synchronization optimization justified. |

Offline `mvanalyze -metric crossvenue` was separately replayed at
`GOMAXPROCS=14` over the merged run: 0.87 s wall, 3.56 s CPU, 69,120 KiB peak
RSS, and 394.45 MB sampled allocation. `encoding/json.Unmarshal` consumed
73.0% cumulative CPU; `bytesContains`/event prefiltering 16.6%; payload decode
23.3% cumulative. This is the same qualitative bottleneck as the original
gate. The small whole-analyzer filter opportunity remains insufficient to
justify a change, and the signed representation creates no decoder reason to
adopt a dependency.

Raw merged artifacts are retained under
`scratch/v2-8-signed-merged.xD8xvN/`. Decision: retain `encoding/json`, make
no simulator or analyzer optimization in this step, and reprofile only when a
new workload (option-heavy or liquidation-reachable) changes the measured
hotspot mix.

## Post-hardening reprofile — `5afdd45`

The signed-price *hardening* branch was subsequently fast-forwarded at
`5afdd45`.  This is a new measurement rather than a claim based on the earlier
`320262e` profile.  The simulator used the retained 30-minute baseline config,
seed 101, `GOMAXPROCS=1`, 60-second checkpoints, and `spot_maker` receipt
evidence with raw JSON logging disabled.  It produced 2,126,782 observations
and the expected ordered execution hash
`5db76448ebb8c5ca60d04366a5fe89540e745564c7fb86cc328be7515989e5f6`.

| Component | Measurement | Decision |
| --- | ---: | --- |
| Simulator wall / speed | 22.25 s / 80.9 simulated seconds per wall second | No regression signal; not an improvement claim from one timing specimen. |
| Simulator RSS / allocation / GC | 812,756 KiB / 9.246 GB sampled / 21 cycles (418.6 MB/s) | Allocation remains material. |
| Core order path | `PlaceOrder` 32.6% cumulative CPU; `processExecutions` 14.7% | Do not alter matching/admission without a separate ownership and exact-equivalence proof. |
| Checkpoint/logging | checkpoint observation/logger 18.0%; `encoding/json.Marshal` 15.6% | Still material but semantic-risky; no encoder swap. |
| Detached preview | 13.6% inclusive sampled allocation | Potential future candidate only after matcher ownership/fill equivalence proof. |
| Scheduler/courier/locks | no new standalone hotspot; block is pprof shutdown and mutex profile is empty | No ordering or synchronization change. |
| Analyzer replay | 0.73 s wall, 64,968 KiB RSS, 410.85 MB sampled allocation | `encoding/json.Unmarshal` 70.6% cumulative CPU; still a material offline-only bottleneck. |
| Analyzer structural scan | prefilter 19.4% cumulative CPU; worker block is only the terminal `WaitGroup` join | Existing whole-workload `bytes.Contains` comparison was too small to justify a change. |

The analyzer replay was `crossvenue` over the retained 707 MB full-evidence
specimen.  The data reconfirms that an analyzer-only decoder is the only
plausible future JSON optimization surface, but it does **not** authorize a
dependency: `goccy/go-json` remains rejected for overflow incompatibility;
Sonic/jsoniter need a broader malformed/duplicate-key/UTF-8/RawMessage/
integer-boundary differential and identical end-to-end analyzer output before
any adoption.  No simulator JSON encoding, evidence digest, event domain, or
economic logic changed here.  Compact machine-readable provenance and pprof
hashes are in
[`v2-8-signed-hardening-reprofile.json`](artifacts/v2-8-signed-hardening-reprofile.json);
the profile binaries are retained under `scratch/signed-price-hardening-20260824/`.
