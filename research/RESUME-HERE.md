# Resume the frozen audit

Paused mid-campaign for a laptop restart. Nothing here needs re-deriving; it
needs running. Everything below refers to the **2026-08-22 freeze**
(`research/FREEZE.md`, commit `ae13f9a`).

## Where things stand

**Determinism is settled.** Three causes were found and fixed; the acceptance
evidence is in `FREEZE.md`. Independent confirmation arrived afterwards: the
Phase 1 baseline run with **full logging** produced the identical canonical
hash to the logs-off verification run —
`5e9c827e3953faa0bf48973b398d61cc…`, 108,220,950 events. Logging does not
perturb the model.

**Phase 1 is complete as far as the runs go.**

| run | status | events | 24h hash (prefix) |
|---|---|---|---|
| `logs/f2_baseline_101` | complete, raw logs on disk | 108,220,950 | `5e9c827e3953faa0bf48973b398d61cc` |
| `logs/f2_baseline_102` | complete, raw logs on disk | 113,618,719 | `79b7d3f6a00e2f2da606d43123e994a2` |
| `logs/f2_baseline_103` | complete, raw logs on disk | 107,762,992 | `6b8b1571be3c1e648a0fd627fb54b3b6` |

**Metric extraction is partial** and must be finished before anything else:

- `f2_baseline_101`: 14 of 15 metrics extracted (`streamhash` outstanding)
- `f2_baseline_103`: 2 of 15
- `f2_baseline_102`: not started

Interrupted analyzers left empty files; those were deleted, and the stale
`.done` markers were removed, so the reaper resumes cleanly and re-extracts
only what is missing.

**Phase 2 has not produced anything.** Nine arm runs were launched and killed
part-way through their 24 simulated hours; their directories were deleted
because a partial run is not a run. No arm has been re-measured against this
freeze.

## Restart in this order

### 1. Rebuild and confirm the tree

    cd /home/vlad/development/exchange_simulation
    git log --oneline -3
    go build -o bin/multivenue ./cmd/multivenue
    go build -o bin/mvanalyze ./cmd/mvanalyze
    go build -o bin/prunegate ./cmd/prunegate

### 2. Finish extracting the baselines

    setsid nohup bash scratch/reap.sh > scratch/reap.out 2>&1 < /dev/null &

It skips any artifact that is already non-empty, so it picks up exactly where
it stopped, and it now marks a run complete only once every metric has
produced output. If a run was interrupted while extracting, delete its
`.done` marker before starting:

    find research/artifacts/scoreboard/f2_* -name .done -delete

Watch with `tail -f scratch/reap.log`. Expect roughly an hour: the heavy
metrics are `reaction` (it scans every book delta), `optionsurface` and
`streamhash`.

Then confirm nothing is missing:

    ./bin/prunegate | grep -A3 f2_baseline

### 3. Phase 2 — all nine arms, both seeds

Configs are already generated from the new freeze
(`python3 scratch/build_ablation_arms.py` regenerates them if needed). Two
waves, because eighteen 24h runs at ~31GB each will not fit at once:

    setsid nohup bash scratch/parallel_runs.sh scratch/jobs_f2_w1.txt 9 80 > scratch/f2_w1.out 2>&1 < /dev/null &
    # after wave 1 is reaped and pruned:
    setsid nohup bash scratch/parallel_runs.sh scratch/jobs_f2_w2.txt 9 80 > scratch/f2_w2.out 2>&1 < /dev/null &

Keep the reaper running throughout. A run takes ~30 minutes wall for 24
simulated hours when nine share the machine.

**Prune only through the gate**, never by hand:

    ./bin/prunegate --prune

It refuses any run that is not SAFE_TO_PRUNE, which requires every measurement
that arm's own preregistration depends on to exist, parse and be non-empty.

### 4. Phase 4 — stress, on the new freeze

    setsid nohup bash scratch/parallel_runs.sh scratch/jobs_f2_stress.txt 2 80 > scratch/f2_stress.out 2>&1 < /dev/null &

`research/configs/v005-stress-perp-2026-08-22.json` carries exactly the
preregistered delta from the old stress arm — fifty-times uninformed perp flow
and the larger metaorder sizes — and nothing else.

### 5. Phases 3 and 5

Rescore all nine arms from scratch against the new freeze, component by
component, then the provenance table (old diagnostic verdict against new).
Then the frozen stylized-fact scoreboard. Neither needs new runs once Phases 2
and 4 are extracted.

## Rules that still hold

- Never edit either frozen configuration; an arm is a copy with one named
  delta.
- The old freeze's measurements, the eleven earlier reruns and every verdict
  currently in `causal-ablations.md` are **diagnostic artifacts only**.
- Do not revise a preregistered prediction or threshold after seeing a result.
- A result that changes after the determinism fix is itself a finding; do not
  seek reproduction of the old conclusions.
- The economic interpretations in `future-calibration.md` stand and are not to
  be implemented during the audit.

## Note on `scratch/`

`scratch/` is gitignored by project rule, so the run scripts, job lists and the
reaper live on disk rather than in the repository. They survive a restart. The
job lists are also copied into `research/artifacts/` where they are tracked, so
the campaign can be reconstructed even if `scratch/` is lost.

## Useful facts

- 24 simulated hours costs ~31 minutes wall on an idle machine, ~30GB of logs;
  logging is 29% of runtime, the rest is simulation.
- `GOMAXPROCS` no longer affects results, so runs can use whatever the
  scheduler gives them.
- `scratch/dethash_par.sh <label> <duration> <n> [procs]` re-checks determinism
  at any horizon.
- The divergence locator is `checkpoint_interval_seconds` plus
  `trace_from_nano`/`trace_to_nano` in any config; compare `checkpoints.jsonl`
  between runs to bracket a divergence, then trace that window alone.

## 2026-08-22 continuation — provenance and latency measurement

- V-012 was resolved as duplicate checkpoint telemetry only: seed 101's
  2,570,397 excess observations were 2,332,800 `maker_state` and 237,597
  `conservation_violation` observations. Raw evidence is complete.
- Execution checkpoints, normalized evidence multiset, and exact persisted
  JSON-record multiset are now distinct contracts with regression coverage.
- New checkpoints explicitly identify the ordered `execution_observations`
  domain and carry `execution_stream_hash`; legacy `rolling_hash` remains only
  as a compatibility alias for the preserved baseline checkpoint files.
- V-013 made prunegate evaluate its declared manifest predicates rather than
  accept any unrelated nonzero field. No raw logs have been pruned.
- V-014 found that reaction lag is not realized network latency. Compact
  courier `latency.json` instrumentation was added and shown not to perturb a
  5-minute frozen observable stream.
- V-015 made the shared analyzer scanner fail closed on malformed relevant
  evidence. Rescan each baseline's full normalized evidence stream before
  certifying its already-extracted metrics.
- V-016 binds prune-gate verdicts to the exact simulator freeze, so the old
  diagnostic ablation verdicts cannot certify any ae13f9a treatment.
- V-017 makes the independently reconstructed exact JSON-record digest a
  mandatory prune-gate artifact for every run.
- V-018 replaced the reaper's sandbox-incompatible process-name test with
  final-only `greeks.json` + `latency.json` completion sentinels. Wave 1's
  temporary partial derived artifacts were discarded. The interrupted raw
  treatment logs were then discarded too because no run reached final state;
  restart every Wave 1 arm from scratch.

## 2026-08-23 clean stop — restart here

The ae13f9a baseline control evidence remains intact. The last completed
checks were the V-015 strict full-evidence scans:

| seed | normalized persisted evidence events | digest |
|---:|---:|---|
| 101 | 105,650,553 | `91de88d54532c004572670511cfe3b7da76c0cc597c55adccf67bf47fb2f7c69` |
| 102 | 111,048,322 | `cfaf7c52df4ea95ae795ae06c6cd85e8645afd136d05f5be6f4047757963b95a` |
| 103 | 105,192,595 | `42d14cd85a3cefb4c0832a5c891fd5643c25ef8b36394b540c159502f06d5fb3` |

All three compact latency controls are attached to their baseline scoreboard
directories. Seed 102's final compact control was:

```text
event_count=111,048,322
checkpoint=9a0265579fff9e8a1477aaf2e606782889f7842bc512fccd9096ac7a17a6fb05
scheduled=193,376,039 delivered=193,374,477 undelivered=1,562
mean sampled=11.92ms; mean actual delivery=41.20ms
```

Three instrumentation/analyzer-only commits were made after `472004a`:

- `beaacf3 fix(audit): separate execution and evidence provenance`
- `c609b1b fix(audit): bind prune verdicts to simulator freeze`
- `5d3c49a fix(audit): gate exact persisted evidence digest`

The prerequisite regression suite passed, including the fresh-process
`MULTIVENUE_DETERMINISM=1` test across full/no-log and GOMAXPROCS variants.

**Wave 1 was deliberately stopped, not completed.** Nine treatment processes
were interrupted at about 9.5 simulated hours; none emitted final
`greeks.json`, and all nine partial raw directories and their accidentally
created partial scoreboard directories were deleted. They are not evidence.
No treatment or stress result exists at ae13f9a.

Before restarting Wave 1:

1. Rebuild `bin/multivenue`, `bin/mvanalyze`, and `bin/prunegate`.
2. Finish V-017 for all three baselines: calculate and retain
   `evidenceartifacthash.json` from the preserved raw logs. Historic baselines
   predate runtime artifact sidecars, so record their independently scanned
   exact-record digest without pretending it can be compared to a missing
   runtime sidecar.
3. Confirm `./bin/prunegate -json` leaves controls measurement-incomplete only
   for that new required artifact; do not prune the baseline logs.
4. Start `research/artifacts/jobs_f2_w1.txt` again from scratch.
5. Start `scratch/reap.sh` only after the jobs have begun. Its repaired
   completion rule requires both final-only `greeks.json` and `latency.json`,
   and its metric list now includes `evidenceartifacthash`. Never use host
   process-name detection again.

The disk is clean at roughly 395 GB free. The frozen baseline raw logs remain
on disk; no evidence authorized for retention was removed.

### Post-restart update

V-017 is now complete for all three baselines. Exact persisted-record digests
are in each scoreboard directory as `evidenceartifacthash.json`; see V-017 in
`validation-audit.md` for values. Baseline gates pass but their raw logs remain
retained. Wave 1 was restarted from scratch with the completion-sentinel
reaper; no interim treatment metric is evidence until its paired full runs
finish and their contracts pass.
- Baseline 101/102/103 extraction and their compact latency sidecars are
  complete. The stricter V-015 full-evidence revalidation scan is the final
  gate before Phase 2; raw logs remain retained even though prunegate now
  reports the control contract safe.

### 2026-08-23 continuing audit update

- V-005 preregistered stress is complete. It exercised 7,054 forced closes in
  seed 101 and 7,177 in seed 103. Every observed close has an independently
  reconstructed pre-to-post position reduction and zero per-close contract
  residual. No bankruptcy, deficit, or insurance-fund absorption occurred, so
  those paths remain NOT EXERCISED. See `v005-stress-ae13f9a.json`.
- V-022 caught a validator blind spot: a scratch `funding_twice` mutant passed
  the former net/sign audit. The new symbol-scoped, per-account detector caught
  85 duplicate payments over five exercised instants. All 23 retained current
  baseline/treatment/stress derivative artifacts were replayed and are clean;
  pre-fix artifacts are preserved under `historical/v022-pre-duplicate-funding-derivatives/`.
- The ae13f9a baseline scoreboard is appended to
  `research/stylized-facts-baseline.md`. Next: clock-artifact experiments and
  remaining high-value mutations, then frozen-autopsy synthesis. Do not enter
  v2 design yet.

### 2026-08-23 clock and mutation update

- The clock screen is complete and recorded in `clock-artifacts-ae13f9a.md`
  and `artifacts/clock-ae13f9a.json`. It is a screening-level
  CLOCK-SENSITIVE result: 100ms step alone widened the pooled perp basis to
  201.0/188.8 bps (seeds 101/103), while the de-staggered timing package
  compressed it to 2.80/2.37 bps. The driver is an interaction with the clock
  lattice, not a generic benefit of finer resolution; individual clocks remain
  unidentified. All six clock worlds retain raw logs, evidence digests, and
  clean order-lifecycle audits.
- `434eb56` added `mvanalyze -metric orderlifecycle`, which independently
  reconstructs accepted/fill/cancel evidence. All retained 24h baselines pass:
  11.75–11.79M accepted orders per seed and zero missing immediate terminals,
  post-terminal fills, or quantity mismatches. This is analyzer-only and does
  not alter ae13f9a simulator semantics.
- Two new five-hour scratch mutants were caught with raw logs retained:
  `drop_ioc_cancel_log` produced 169,935 missing immediate terminal records
  (`a1decd2`); `double_spot_fee` produced CDF/USD conservation residuals of
  126,548,770,107 and 235,583,218,129 (`870dae4`). The remaining untested
  GTC-cancel-request and future-information paths still lack sufficient
  persisted evidence; do not call them covered.
- Next: continue high-value mutations (especially fill, expiry, collateral,
  and information-boundary paths), then synthesize the frozen autopsy. Do not
  enter v2 design yet and do not prune raw evidence.

### 2026-08-23 delta-sign mutation update

- A five-hour seed-101 `delta_sign` scratch mutant negated `Black76Delta`
  only in the live option-dealer hedge path. It was caught by the
  exchange-owned risk timeline: pooled mean absolute net delta rose from
  0.0170 in the exact-clock control to 1.9264 (113.1x), maximum net delta from
  0.1650 to 10.8592, and drift from 0.000378 to 1.1844 contracts/hour. The
  absolute hedge ratio stayed about one in both worlds, proving that flow or
  absolute-ratio telemetry alone would have missed the reversed sign. The
  mutant source was restored before execution, raw logs remain at
  `logs/mut_delta_sign`, and the artifact is
  `artifacts/mutations/delta-sign.json`.
- Current source regression passed: `go test ./price ./analysis
  ./simulations/multivenue -count=1`. Next remains fill, expiry, collateral,
  and information-boundary coverage; V-005 makes liquidation reachable, but
  an independent contemporaneous mark/equity reconstruction must exist before
  a stale-collateral mutation can be claimed.

### 2026-08-23 fill-to-position mutation update

- V-023 added the analyzer-only `mvanalyze -metric fillpositions`. The
  five-hour control has 248,898 linear (perp/dated-future) fills paired
  one-to-one with 248,898 persisted trade position updates. A scratch mutant
  that settled exactly one north `ABC-PERP` match twice while emitting only
  the original fill produced 248,900 updates; the audit caught the two extras.
  Conservation, terminal linear positions, and order lifecycle all passed,
  demonstrating why the relation is necessary. Artifact:
  `artifacts/mutations/double-perp-settlement-once.json`; raw logs retained.
- Crucial limitation: frozen raw logs lack per-fill option position-update
  evidence. The new audit therefore covers only linear instruments; option
  fill-to-position paths are NOT TESTED. This is a V-023 evidence/audit gap,
  not a simulator defect. Do not claim it covered in the frozen autopsy.
- Full baseline replay is now complete: seeds 101/102/103 have respectively
  1,139,566 / 1,153,404 / 1,134,326 one-to-one linear fill/position pairs,
  each with zero missing, unexpected, or chain-failed transitions. The JSON
  outputs are retained beside their baseline scoreboard artifacts.
- The complementary scratch `drop_perp_settlement_once` mutant preserved the
  first north `ABC-PERP` fill but suppressed its two settlement paths. It had
  248,898 fills and 248,896 position updates; `fillpositions` caught the two
  missing transitions while conservation, terminal positions, and order
  lifecycle all remained clean. Artifact:
  `artifacts/mutations/drop-perp-settlement-once.json`; raw evidence retained.
- V-024 now gives the scheduler-backed courier an executable look-ahead test.
  A 10 ms market-data message is unavailable before its due simulated time;
  negating the courier delay makes the actor inbox receive it at publication
  and fails the test. This validates the message path only: the known direct
  Vanna-Volga dealer-inventory read and historical per-observation evidence
  remain explicit information-boundary limitations. Artifact:
  `artifacts/mutations/future-information-delivery.json`.
- V-025 adds `mvanalyze -metric expiryfills`, anchored to `instrument_listed`
  contract terms rather than to a later settlement event. Its five-minute
  delayed-expiry mutant was caught with 7,326 late fills across 66 exercised
  contracts (six futures, sixty options); the older dated-future-only audit
  saw 1,918. The unit fixture also catches a contract that expires without a
  settlement announcement. Artifact:
  `artifacts/mutations/delay-expiry-settlement.json`; raw logs retained.
- The listing-anchored expiry audit was replayed over the full 24-hour
  baseline logs. Seeds 101/102/103 each have 396 listed contracts and 363
  expired-and-settled contracts, with zero late fill records,
  expired-unsettled contracts, or listing/settlement metadata defects. Their
  inspected fill-record counts are 2,444,064 / 2,448,558 / 2,418,586.

### 2026-08-23 V-026 liquidation-trigger update

- V-026 adds the analyzer-only, coverage-qualified `mvanalyze -metric
  marginchecks` replay for `noise_flow` accounts carrying only ABC-PERP and
  unborrowed USD collateral.  It independently uses persisted initial state,
  ordered derivative positions/balances, and `math/big` arithmetic to compare
  fresh mark, cash, PnL contribution, equity, notional, and maintenance at
  every logged trigger.  V-005 seed 101 matches 8,942/8,942 expected/observed
  breaches across 1,382,076 active mark evaluations; seed 103 matches
  7,533/7,533 across 1,382,128.  All six field mismatch counts are zero.
- A five-hour seed-101 scratch `stale_liquidation_mark` mutation keeps the
  public mark update current but sends the prior stored mark to the liquidation
  sweep.  It leaves the 35 forced-close count unchanged, yet the independent
  replay catches 14 of 39 checks on mark, PnL, equity, notional, and
  maintenance.  This validates the detector, not the unobserved cross-margin
  path; options, FX collateral, isolated margin, and borrowed balances remain
  excluded/unresolved.  Compact evidence:
  `research/artifacts/mutations/stale-liquidation-mark.json`; raw logs remain.
- V-027 records that a finite-horizon run's terminal quiescence tail is not
  equivalent to the same prefix of a longer live run.  The 5h V-026 control
  exactly matches the 24h V-005 control through 5:00 (22,408,046 observations,
  digest `33005d…a5e6f9`), then correctly diverges after the requested horizon.
  Include horizon in every provenance ID and compare only common active
  prefixes.
- Next: commit V-026, then isolate cadence classes behind the existing
  CLOCK-SENSITIVE screen and finish the remaining mutation gaps before frozen
  autopsy synthesis. Do not begin v2 design or prune raw logs.

### 2026-08-23 V-028 evidence-omission mutation update

- A five-hour seed-101 scratch `drop_abc_perp_fill_log` binary suppresses only
  persisted ABC-PERP `OrderFill` records after the actual settlement and
  client notification. The control and mutant have byte-identical terminal
  Greeks and courier-latency sidecars, but the mutant loses 111,398 persisted
  fill records. `fillpositions` catches exactly 111,398 unmatched linear
  position transitions; `orderlifecycle` additionally reports 47,268 missing
  immediate terminals and 28,309 quantity mismatches. Both worlds' runtime and
  offline evidence digests agree with their own preserved records, as they
  should. Artifact: `artifacts/mutations/drop-abc-perp-fill-log.json`; raw
  evidence remains retained.
- The dropped-fill mutation validates the linear evidence contract, not option
  fill-to-position coverage. That V-023 option-path limitation remains a
  frozen-evidence gap.

### 2026-08-23 V-029 clock-factor update

- Ten paired five-hour clock worlds are retained with full logs, runtime/offline
  evidence attestations, and clean order-lifecycle audits. Stage one confirms
  broad cadence sensitivity: publication-only, maker/flow, and
  risk/options/carry packages each compress ABC-PERP basis below the registered
  step-to-destagger midpoint in both seeds. The narrow follow-ups refute a
  one-clock story: quote-only passes only seed 101 (90.99 / 185.00 bps for
  101/103), while dated-carry-only passes only seed 103 (116.27 / 76.28 bps).
- The frozen autopsy must report a coupled cadence-lattice artifact as
  **CLOCK-SENSITIVE**, with individual clock/LCM attribution unresolved. The
  interval tests also change action rate; no phase-offset control is available
  without a new simulator timing feature/freeze. No cadence variant restores
  near-zero raw-return ACF or small triangular dislocations. See
  `clock-artifacts-ae13f9a.md`, `clock-factorial-plan-ae13f9a.md`, and
  `artifacts/clock-factor-ae13f9a.json`. Do not promote any frozen economic
  effect to timing-robust.

### 2026-08-23 V-030 zero-latency mutation update

- A paired five-hour seed-101 scratch mutation makes only the north
  `spot_maker` courier instantaneous while retaining the delayed gateway,
  telemetry, and an unchanged manifest. It is **CAUGHT** directly: control
  north mean drawn delay is about 0.566 ms in all three channels; mutant
  north has exactly zero drawn/delivered ns across 1,040,345 messages, while
  south remains delayed. Runtime and offline persisted-evidence digests match
  within each world; lifecycle and conservation controls remain clean.
- The suggested behavioral proxy is falsified: zero north maker latency
  reduces rather than creates the measured cross-venue edge. Do not infer
  latency correctness from a price-edge direction. The sidecar is the valid
  evidence product, guarded by `TestScheduledZeroLatencyProducesZeroTelemetry`.
  Compact artifact: `artifacts/mutations/zero-north-spot-maker-latency.json`;
  raw logs remain. Remaining frozen mutation gaps are GTC cancellation request
  evidence and cross-margin/option/FX/borrowed-collateral liquidation coverage.

### 2026-08-23 V-031 expired-book quote update

- `expiryfills` now joins `BookSnapshot` records to immutable listing expiry
  terms and separately counts post-expiry snapshots with nonempty depth. The
  pre-existing five-hour delayed-expiry mutant is **CAUGHT** with 19,800
  nonempty post-expiry snapshots across 66 contracts (alongside 7,326 late
  fills); the paired control has zero.
- The extended replay over baseline seeds 101/102/103 is clean: each has 396
  listed and 363 expired-and-settled contracts, with zero late fills, snapshot
  records, or nonempty quotes after contractual expiry. This closes expired
  book/delisting evidence for observable futures/options, but not the absent
  GTC cancel-request path. The analyzer change is freeze-preserving. Next:
  synthesize the frozen autopsy from retained artifacts; do not begin v2 until
  that gate is reviewed.

### 2026-08-23 frozen-autopsy gate

- The ae13f9a frozen autopsy is complete with explicit limits:
  `research/frozen-autopsy-ae13f9a.md` and
  `research/artifacts/frozen-autopsy-ae13f9a.json` distinguish deterministic
  execution/evidence provenance, mechanical support, nine paired causal
  verdicts, V-005 reachability, the stylized scoreboard, cadence sensitivity,
  mutation coverage, and unresolved evidence domains.
- `bin/prunegate -json` classifies the 21 actual `f2_*` baseline/treatment
  directories `SAFE_TO_PRUNE` under the ae13f9a manifest and verdict artifact;
  logs are intentionally still retained. Earlier non-`f2` rows are historical
  directories and must not be mistaken for the frozen campaign.
- It is now legitimate to start the separate v2 design ledger. Do not tune or
  alter the ae13f9a simulator, and preserve all frozen artifacts as the v1
  comparator.

### 2026-08-24 V2-3 P2 clean-stop gate

- P0 and P1 are completed, committed V2 mechanism screens. P2 is the
  preregistered five-minute CDF/USD maker inventory-rebalance screen in
  `v2-3-inventory-rebalance-p2-preregistration.md`; it is **not yet scored**.
- Attempt 0 ran all four immutable A/B × seed-101/103 cells from `6c3cedd`,
  but is invalidated before causal scoring. `exchange.Buy` is numeric zero and
  `MakerInventoryRebalanceDecision.Side` had `omitempty`, so persisted BUY
  decision records omitted their required side. The exchange request/fill
  records prove activity, but cannot retrospectively repair the independent
  decision evidence. Raw logs and every extracted sidecar are retained at
  `artifacts/historical/v2-3-p2-attempt0-buy-side-omission/`; see
  `v2-3-inventory-rebalance-p2-attempt0-invalidation.md`. Do not prune or cite
  them as final results.
- `ce096d2` introduces the explicit persisted `SideEvidence` string without
  changing execution semantics; `b9c271c` adds the negative-inventory BUY
  analyzer regression. Focused normal/race testing and evidence on/off
  execution-hash neutrality passed. The instrumentation correction requires a
  completely fresh four-cell rerun, not a patch of the old evidence.
- Exact restart gate: verify a clean source diff except user-owned artifacts;
  rebuild `bin/multivenue` and `bin/mvanalyze`; run focused normal/race P2
  tests and vet; rerender and byte-diff P2 A/B configs; run all four five-minute
  cells from scratch; use only final `greeks.json` + `latency.json` as run
  sentinels; extract receipts, evidence artifact hash, P2 replay, viability,
  inventory, trade-ratio, and metadata before interpreting anything. If the
  corrected B audit is not valid, preserve that attempt under a new historical
  label and repair only the evidence/auditor defect.
- Pre-shutdown verification at corrected `023e671`: focused normal and race
  P2 tests, `go vet` for analysis/multivenue/mvanalyze, and `go test ./...`
  passed; `multivenue` and `mvanalyze` were rebuilt; renderer output exactly
  matches the committed immutable configs. No simulator or analyzer process was
  left running. Recheck the working tree after reboot, but do not treat the
  pre-shutdown binaries as provenance for the rerun: rebuild again from the
  verified source revision.

### 2026-08-24 V2-3 P2 final evidence and score

- The restart gate above is now complete. The narrow zero-valued-enum audit is
  committed at `fb8665d` in `research/v2-zero-valued-enum-evidence-audit.md`.
  It found the BUY omission to be a schema-class hazard, added independent
  zero-member wire fixtures, and found no remaining numeric zero enum with
  `json:",omitempty"` in persisted scientific/provenance records. It makes no
  simulator-economic change. `InstrumentAnnouncement.SettlementPrice` remains
  an explicitly open *price* availability issue, not an enum exception.
- The final full-evidence A/B × seed-101/103 campaign was rebuilt and run from
  source `675e117`, then independently extracted and audited. Raw evidence is
  retained at `research/artifacts/v2-3-p2/{A,B}/seed-{101,103}`; no P2 raw
  evidence is prunable. Completion was determined only from final
  `greeks.json` and `latency.json` sidecars.
- All four V2-0 receipt/frontier audits and all four independent P2 policy /
  request / fill replays are valid. A has exactly 180 `POLICY_DISABLED`
  decisions and zero submissions per seed. B has 46/50 submissions,
  44/48 accepted requests, and 88/96 externally counterparty fills for seeds
  101/103; the two tail requests per seed are declared horizon-censored.
  The replay found zero future/missing/ambiguous receipts, policy mismatches,
  unmatched outcomes, non-IOC terminals, self fills, fee errors, or
  non-reducing local fill transitions.
- `3fa3e2c` records the final P2 score: **SUPPORTED (screening), mechanism
  integrity only**. It supports auditable, capped, locally informed, costly
  external risk transfer; it does **not** support aggregate inventory
  reduction, CDF/USD stabilization, price elasticity, ecology viability, or
  robustness. Attempt 0 remains invalid and attempt 1 pre-audit diagnostic;
  neither may be pooled or cited as final.
- Next: append/commit this updated handoff, then select and preregister the
  next minimal V2 mechanism from `research/v2-design.md` before changing
  economic behavior. Do not tune P2 coefficients, clocks, spreads, demand, or
  population in response to the short mechanism screen.

### 2026-08-24 V2-4 L0 delivery-liability activation screen

- L0 was preregistered before implementation/configuration at `29de0c5`, then
  implemented at `85038fa`. It adds one finite-capital CDF/USD delivery-liability
  hedger per venue; its private, bounded obligation state determines side and
  its public local snapshot supplies only the named executable touch. It has no
  index/mid/shared-book fallback. Explicit side availability and a present
  numeric price are separate fields (`84fb97e`).
- The first 15-second, non-registered B startup smoke exposed a genuine
  terminal-tail evidence defect: an exchange fill can occur at the final
  boundary while its delayed response cannot reach the actor, leaving no local
  fill attestation. It is retained as diagnostic-only at
  `scratch/v2-4-l0-smoke-gsNxBi`; it is not campaign evidence. `ee2a842`
  changes only the new L0 policy to emit an explicit
  `SIMULATION_HORIZON_CENSORED` defer when fewer than two decision intervals
  remain, instead of accepting an unattested fill. `5d5df31` makes the
  independent replay prove that this defer is actually at the horizon.
- The required V2-0 receipt/on-off fresh-process/GOMAXPROCS L0 neutrality test
  passes; L0 recorder telemetry is append-only and does not change execution
  hash. The independent L0 auditor and adversarial fixtures are in `967b6e7`.
- The immutable A/B × seed-101/103 five-minute full-evidence cells ran from
  source `5d5df3175cefd055523b4377d4eee20091d235be`, binary SHA-256
  `692261802d7359c4a4cd297ea6ec90b33a6040a472cd46baec494c80de1c07cc`,
  and `GOMAXPROCS=4`. Raw evidence is retained at
  `research/artifacts/v2-4-l0/{A,B}/seed-{101,103}` and must not be pruned.
  Completion was determined only from final `greeks.json` + `latency.json`.
- All four V2-0 receipt audits, evidence-artifact hashes, and independent L0
  replays are valid with zero checks. Controls have 90 state updates and zero
  L0 requests per seed. Treatments have 310/282 accepted IOC requests,
  480/466 fills, and 15,689,798,578/15,599,737,574 filled CDF units for seeds
  101/103. Every actor has 30 state updates; all treatment directions and
  local fill transitions reduce its signed hedge gap. Seed 103 records six
  explicit tail censors; seed 101 is in-band at the analogous final choices.
- Result: **SUPPORTED (screening), narrow mechanism integrity only**. See
  `research/v2-4-liability-hedger-l0-results.md` and
  `research/artifacts/v2-4-l0/l0-summary.json`. No price-stability, demand
  elasticity, replacement, viability, realism, or robustness claim is
  licensed. Next V2 gate: preregister L1 as a controlled replacement of one
  activity-generator family, with long-run viability, phase, and holdout
  conditions fixed before implementation.

### 2026-08-24 V2-4 L1 matched CDF motive-control screen

- The registered 30-minute full-evidence A/B × paired-{101,103} screen is
  complete. A retains the named CDF/USD-only slot but uses
  `random_side_control`; B differs only by `delivery_liability`. All six
  broad legacy `noise_flow` actors remain in both arms; neither L1 arm is a
  roster-replacement experiment.
- All four cells have final `greeks.json`/`latency.json` sentinels, retained
  raw evidence, a V2 receipt/frontier audit, an exact persisted-evidence
  artifact digest, a valid independent policy replay, 180 state updates for
  each of three slots, accepted requests in each slot, and the weak CDF/USD
  non-collapse floor. The scored provenance is
  `research/artifacts/v2-4-l1/l1-summary.json`.
- Result: **SUPPORTED (screening), local motive only.** In both seeds every
  B fill reduced its independently replayed delivery gap, A contained many
  nonreducing fills, and exact B−A mean absolute gaps were
  −8,898,362,628.476 (101) and −4,334,242,294.419 (103) raw units. See
  `research/v2-4-l1-cdf-motive-control-results.md`. This does not support a
  price-stability, demand-realism, wealth/ecology, robustness, or
  `noise_flow`-replacement claim.
- Two analyzer-only defects were found during retained-evidence replay and
  fixed before final extraction: `4364fed` distinguishes an intended
  no-touch defer (`request_id=0`) from a real request; `1a2641a` accepts the
  exchange's documented exact-zero-fee representation (`amount=0, asset=""`)
  while rejecting missing positive fees. Neither changed simulator behavior or
  raw evidence. Their normal/race fixtures pass.
- Next gate: preregister and run L1-P phase/offset sensitivity, holding the
  L1 roster, obligations, policy, caps, fees, latency, and cadence frequency
  fixed. Do not demote legacy `noise_flow` or tune economic parameters before
  that result.

### 2026-08-24 V2-4 L1-P liability-hedger phase screen

- L1-P is complete and scored at `research/v2-4-l1p-phase-results.md` with
  compact provenance in `artifacts/v2-4-l1p/l1p-summary.json`. It varies only
  `cdf_liability_hedger.decision_phase_offset`: P0 is explicit zero/first
  decision at start+2s; P1 is +1s/first decision at start+3s. All policy,
  population, fees, prices, latency, periods, and other clocks remain fixed.
- `af7a284` provides the phase capability. Explicit zero routes through the
  legacy timer code; `160508a` proves explicit-zero and absent legacy phase
  have identical fresh-process execution hashes at GOMAXPROCS 1/4. The
  nonzero phase evidence-on/off fresh-process matrix is also hash-neutral and
  its independent replay rejects missing/mismatched/off-phase records.
- The full-evidence P0/P1 × seed-{101,103} cells ran 30m from `160508a`,
  `GOMAXPROCS=3`, and multivenue SHA-256
  `cfc920ce12c1aa9790bf43424418fdcb921a5dbbebcd2f22c4109550523515e1`.
  Every completion decision used only final `greeks.json` and `latency.json`.
  All receipt audits, evidence artifact hashes, independent policy/phase
  replays, slot activity gates, and CDF/USD non-collapse floors pass. Raw
  evidence is retained at `artifacts/v2-4-l1p/{P0,P1}/seed-{101,103}` and is
  not prunable.
- Result: **SUPPORTED (screening), narrow local-motive contract.** Every
  exercised delivery fill reduces its independently replayed gap under both
  phases. P1 has much lower descriptive mean gap and more fills in both seeds
  (not a preregistered directional score). This is a clock-interaction
  discovery candidate; it does not establish ecology-wide phase robustness or
  support L2 roster demotion. Next: preregister an L1-P2 phase-decomposition
  screen with holdout replication and one periodic counterpart isolated at a
  time. Do not tune policy, population, price, spread, frequency, or latency.

### 2026-08-24 V2-4 L1-P2 liability/noise phase decomposition

- L1-P2 is complete and scored at
  `research/v2-4-l1p2-noise-phase-results.md`, with exact machine provenance
  at `research/artifacts/v2-4-l1p2/l1p2-summary.json`. It keeps the L1-B
  population and every non-noise clock fixed, then crosses a 0/1-second first
  tick for the delivery-liability hedger and the named broad `noise_flow_*`
  population. No economic coefficient, price, spread, latency, or roster was
  tuned.
- `15405b1` adds the only new behavioral control:
  `noise_flow_decision_phase_offset`. It affects only `noise_flow_*` random
  takers; zero goes through the legacy ticker. `0de1144` tightens the optional
  timing row to be emitted after local planning but before any planned request
  enters its gateway. Fresh-process GOMAXPROCS 1/4 tests prove recorder
  ON/OFF neutrality and absent/explicit-zero equivalence; an independent
  replay catches dropped, duplicated, off-phase, phase-mismatched, and
  phase-omitted rows. A direct old/new two-minute L1-B comparison has the same
  ordered execution hash `6569cfce031a985df8c361261d3a4d3a1b35cb07e8c7e595fbc8c36acae4857c`.
- All eight 30-minute A/B/C/D × seed-101/103 cells have final
  `greeks.json`/`latency.json`, valid receipt and persisted-artifact evidence,
  valid liability and noise timing replays, 540 liability state updates,
  zero nonreducing delivery fills, and passing CDF/USD non-collapse floors.
  Raw evidence is retained at `artifacts/v2-4-l1p2/{A,B,C,D}/seed-{101,103}`
  and is not prunable. An initial shell-terminated attempt has no completion
  sidecars and is retained solely as `NON_EVIDENCE` under
  `artifacts/historical/v2-4-l1p2-attempt0-shell-terminated`.
- Result: **SUPPORTED (screening), narrow relative-clock attribution.** Both
  seeds satisfy the preregistered aligned-greater-than-dealigned direction and
  positive interaction. This says broad `noise_flow_*` timing is a causal
  counterpart for the L1-P local-gap contrast in this population; it does not
  support phase robustness, a unique LCM explanation, price stability, demand
  realism, or L2 roster demotion. Next: preregister and run a fresh 2×2
  holdout-seed replication before any population change.

### 2026-08-24 V2-4 L1-P3 untouched-seed holdout replication

- The fixed 2×2 A/B/C/D phase design was rerun without source, policy,
  population, price, spread, fee, latency, or frequency changes for untouched
  seeds 107/109/113. All 12 full-evidence cells pass final-sidecar, receipt,
  artifact-digest, liability/noise-phase replay, exercised-fill, and CDF/USD
  non-collapse gates. Raw evidence remains retained at
  `artifacts/v2-4-l1p3/{A,B,C,D}/seed-{107,109,113}`.
- Result: **MIXED** under the preregistered rule. Seeds 107 and 109 reproduce
  the aligned-greater-than-de-aligned endpoint and positive interaction; seed
  113 reverses both. Exact raw ratios and digests are in
  `research/v2-4-l1p3-holdout-results.md` and
  `artifacts/v2-4-l1p3/l1p3-summary.json`.
- Consequence: the L1-P2 result is seed-sensitive, not a timing-robust causal
  attribution. It does not support L2 roster demotion, economic tuning, a
  unique LCM mechanism, or any price-stability claim. The next permitted step
  is an explicitly exploratory diagnostic on retained evidence to explain the
  reversal; it cannot revise or rescue the preregistered score.

### 2026-08-24 V2-5 P3e passive finite-term exit — restart handoff

- P3c remains a valid **FALSIFIED** lifecycle result: two genuine finite
  spot/perpetual terms reached their declared end but local opposing touch
  quantities (16,286 and 16,348) were below the venue's legal 100,000 minimum.
  P3d's lower-floor premise is invalid. Do not tune this population or reuse
  its invalid subminimum-order behavior.
- P3e is the committed, opt-in policy `v2_5_p3e_passive_exit_v1`:
  only after independently demonstrated `EXECUTABLE_SIZE_UNAVAILABLE`, it
  posts one legal same-side 100,000-unit GTC post-only child at the participant's
  local passive touch. It has an explicit known deadline, exact cancel request
  identity, no synthetic fill, and no changed venue minimum. Implementation,
  independent replay/mutations, and telemetry-neutrality commits are
  `87d430f`, `2860d5c`, `afc901a`, and `713a9a0`.
- The P3e P0 protocol and immutable config are committed at `e1905d7` and
  `0f006f2`. Config SHA-256 is
  `206fc24ee0bc7f16aacabcbebc794b1fedd611922ef58e4578d5df142b323835`.
  P0 is one seed-107, 98-hour B integrity/activation cell; it is **not** a
  paired market, funding, basis, profitability, or realism screen. Its
  deadline is five seconds after the run horizon, so P0 cannot make a
  deadline-cancellation or closure claim.
- The five-minute full-evidence preflight is complete and documented at
  `research/v2-5-p3e-passive-exit-p0-preflight.md` (commits `9e2c94f`,
  `ac223ca`, `02191c7`). It has valid P4/source/frontier/gateway/actor replay,
  valid observation receipts, clean generic accounting/positions/lifecycle,
  matching runtime/offline persisted-evidence artifact identity, and a passing
  fresh-process `-race` evidence-neutrality test (GOMAXPROCS 1/4). It cannot
  exercise P3e before its 96-hour term end.
- `scripts/extract-v2-5-p3e-metrics.sh` is the fail-closed retained-evidence
  extractor. It requires final nonempty `greeks.json` **and** `latency.json`,
  writes all nine P3e metrics atomically, and proves runtime/offline exact
  persisted-evidence artifact identity. It deliberately does not score the
  P0 activation predicate and never prunes raw evidence. `prunegate` unit
  tests pass; its read-only result for P3e is `MEASUREMENT_INCOMPLETE` because
  the historical ae13f9a manifest has no P3e arm contract. That is the
  intended fail-closed result; never bypass it.
- An initial full P0 launch was explicitly interrupted at
  `2026-08-24T20:29:58+03:00` for laptop shutdown. It has no final
  `greeks.json` or `latency.json`; its incomplete sidecars/checkpoints are
  retained only at
  `research/artifacts/historical/v2-5-p3e-p0-attempt0-interrupted/`.
  It is **NON-EVIDENCE**: never extract, score, compare, cite, or prune it.
  See `research/v2-5-p3e-passive-exit-p0-attempt0-interruption.md`.
- The next launch must be fresh into the now-empty registered path
  `research/artifacts/v2-5-p3e/p0-B-107/`. First verify the worktree/process/
  disk state; do not touch the four user-owned ae13f9a scoreboard edits.
  Rebuild `bin/multivenue`, `bin/mvanalyze`, and `bin/prunegate` from the
  committed head; verify the config SHA above; run with `GOMAXPROCS=4`, full
  evidence, and `-duration 98h`. Completion means only both final sidecars
  exist and are nonempty. Then run the P3e extractor before reading/scoring.
- The pre-score P3e residual-funding analyzer correction is complete at
  `8b8dfa6`, with the rationale frozen in
  `research/v2-5-p3e-residual-funding-audit.md`. The earlier handoff sentence
  that said post-deadline funding must be rejected was wrong: P3e explicitly
  leaves an expired residual economically open, so its ordinary funding must
  remain visible. The replay now separately counts post-term funding before
  the passive deadline and after deadline, but accepts either only with a
  strictly earlier persisted P4 residual state and a nonzero perpetual
  position. Legacy, closed/flat, same-timestamp/future, and unproven residual
  funding remains `OutsideTermFunding` and invalid. P3c is unchanged.
- A fresh five-minute P3e preflight rerun from `8b8dfa6` produced exactly the
  earlier 56,189-observation ordered execution hash and 56,648-record
  persisted-evidence digest; the hardened extractor passes. See
  `research/v2-5-p3e-passive-exit-p0-preflight.md`. This analyzer-only change
  does not alter simulator behavior.
- Current committed head before the next long launch is `8b8dfa6`
  (`fix(analysis): audit P3e residual funding`). The only tracked worktree differences are
  user-owned historical ae13f9a scoreboard artifacts:
  `derivatives.json`, `exposure.json`, `reaction.json`, and `streamhash.json`
  under `research/artifacts/scoreboard/f2_baseline_101/`. No simulator,
  analyzer, config, or research-ledger change is uncommitted. Disk free space:
  about 331 GB. No simulator/analyzer/test process remains.

### 2026-08-24 21:35 EEST — P3e P0 clean stop

- The fresh, registered `P3e` P0 B/107 full-evidence cell is **simulation
  complete and raw evidence is valid**, but its offline analysis was deliberately
  interrupted for this laptop shutdown. Do not score it yet. It ran from source
  `1714c26b6c9253f51cc3da61df52c30956cb4ea8` with the immutable P0 config
  SHA-256 `206fc24ee0bc7f16aacabcbebc794b1fedd611922ef58e4578d5df142b323835`,
  seed 107, 98 simulated hours, full evidence, and `GOMAXPROCS=4`.
- Its final simulator completion line was `sim=98h0m0s`, wall `6m0s`. Both and
  only the completion sentinels are present and nonempty: `greeks.json`
  (397,352,788 bytes) and `latency.json` (2,623 bytes). The terminal ordered
  execution checkpoint is 50,636,477 `execution_observations` with hash
  `e385d5b62666f3ca035d5da45ba01023649eb0e4fd63884326638a52473be7b2`.
- Runtime evidence provenance is retained in
  `research/artifacts/v2-5-p3e/p0-B-107/evidence-artifact-hash.json`. Launch
  executable SHA-256 values were simulator
  `a4326f11a53a4b8c2170797ef045761ac3c601668f5250f2c8d69627aa446583`, analyzer
  `09c7e45ebd6c125cf25e440c857b66c156d13f4fecd7eb4f32a4b05636fa82bf`, and
  prune-gate `cde05218e52dac137b83dc040bc79c5d875c841194c2f6e840da104c23779dbc`.
  The manifest's `modified=true` is solely the four pre-existing user-owned
  ae13f9a scoreboard edits; do not touch them.
- Before the run, the residual-funding replay was corrected and committed:
  `0c77309` documents the P4 contract, `8b8dfa6` classifies only proven
  P4 residual funding separately before/after the passive deadline, `1714c26`
  records an execution/evidence-identical preflight rerun, and `4d9113e` adds
  a regression proving a rejected post-only passive exit preserves the real
  residual instead of silently flattening it. These are analyzer/docs/test
  changes only; no simulator scheduling, RNG, matching, or economics changed.
- The P0 extractor (`scripts/extract-v2-5-p3e-metrics.sh`) was interrupted by
  SIGINT while scanning the completed raw logs. It atomically finished some
  individual metric JSON files but did **not** reach `streamhash`,
  `evidenceartifacthash`, validation, or `analysis-metadata.json`; therefore
  there is no completed P3e analysis contract. Those partial derived files are
  not a verdict and must not be read, cited, or pruned. Leave them in place;
  rerunning the same fail-closed extractor overwrites each metric atomically
  from the preserved raw evidence.
- No simulator, mvanalyze, extractor, or test process remains. Disk free space
  is approximately 330 GB. Raw P0 evidence, both preflights, and historical
  interrupted attempt 0 remain retained. Never manually prune P3e: global
  `prunegate` has no P3e measurement-manifest arm and correctly fails closed.
- At restart: verify the four known user-owned edits only, rebuild
  `bin/multivenue`, `bin/mvanalyze`, and `bin/prunegate`, then rerun exactly:

  ```bash
  P3E_CELL="$PWD/research/artifacts/v2-5-p3e/p0-B-107" \
  MVANALYZE_BIN="$PWD/bin/mvanalyze" \
  scripts/extract-v2-5-p3e-metrics.sh
  ```

  Treat extraction as complete only if it exits zero, prints `extracted V2-5
  P3e evidence`, and writes a complete `analysis-metadata.json`. Then inspect
  the final metrics and score the narrow registered P0 activation/integrity
  predicate. P0 cannot support a market, basis, funding-anchor, cancellation,
  or closure claim because the passive deadline lies after its horizon.

### 2026-08-24 — P3e P0 scored; signed-price hard checkpoint

- The completed P3e P0 raw cell was extracted to a complete fail-closed
  analysis contract and scored in `bdba08d`; see
  `v2-5-p3e-passive-exit-p0-results.md` and
  `artifacts/v2-5-p3e/p0-B-107/p0-verdict.json`. It is **SUPPORTED
  (screening), activation/integrity only**: two owned active finite terms had
  locally delivered opposing perp ask depth 16,286/16,348 below the explicit
  legal 100,000 IOC floor; two exact 100,000 BUY `LIMIT/GTC/post_only` P4
  children were independently attested, accepted, and later filled. Runtime
  and offline exact persisted-evidence identities agree at 51,165,698 records
  / `2e36cc856c71df92624057e48e7aa9e193d6a96b24f0ec9a0c6fa41b26203bf0`.
  Do not promote later `TERM_CLOSED` rows, passive fills, or residual funding
  to closure/funding results: P0 was not registered for those claims and its
  deadline is outside its horizon.
- The required signed-price architectural check is satisfied. Original branch
  `v2/signed-price` closed at `cc91896` and merged at `320262e`; a later
  dedicated hardening branch was integrated at `5afdd45`, with final
  provenance closure `7644b2`. All are ancestors of the active head. The
  authoritative ledger/audit/gate are `v2-signed-price-audit.md`,
  `v2-signed-price-hardening-ledger.md`, and
  `artifacts/v2-signed-price-hardening-gate.json`. Signed numeric values,
  explicit availability, zero-settlement wire preservation, full-range
  midpoint, signed matcher/accounting fixtures, deterministic positive-world
  equivalence, and parent-vs-branch performance are complete. See the
  separate P3e checkpoint document for the exact evidence.
- Next: preregister a fresh same-build P3e lifecycle A/B with the passive
  deadline inside the horizon. Do not use P3c as control; do not tune depth,
  slice, population, spreads, or clocks. Keep activation, partial liquidity,
  cancellation, open residual, actual flat closure, and funding attribution
  as independent endpoints.

### 2026-08-27 — integrated V2 long-run gate paused before simulation

- The active branch is `autoresearch/ffa-ecology-gen0` at `30176e0`
  (`fix(research): harden long-run cell provenance`). No simulator, analyzer,
  extractor, or experiment process is running. No new outcome world was
  launched in this iteration.
- The runner hardening is committed separately. It now registers the
  `dev-607-g8` parity cell, requires the registered `GOMAXPROCS`, hard-codes
  the registered 24-hour horizon, executes the copied `run-config.json`,
  records config/hypothesis identity and binary Go provenance, requires a
  clean binary revision equal to the current repository HEAD, refuses reused
  output directories, and writes an atomic `run-status.json` only after both
  `greeks.json` and `latency.json` are present and the simulator exits zero.
- The integrated 24-hour candidate has **not** started. The next required
  pre-run work is to revise the protocol/extractor/scorer under the independent
  Sol-xhigh review: parse every required JSON; verify analyzer VCS revision and
  clean build; reconcile metadata/config/seed/cell/hypothesis; add late-path
  activation and bounded-integer accounting tolerances; require lifecycle,
  order, settlement, expiry, derivative and margin integrity predicates; add
  the CDF borrow / `PRICE_UNAVAILABLE` cross-asset activation check; classify
  disabled P4/P5/P3 recorders as `OUT_OF_SCOPE`/`RECORDER_NOT_ENABLED`; and
  precommit the three-development-cell aggregate plus seed-607 G4/G8/no-log
  parity scorer. Obtain reviewer acceptance before any 24-hour cell.
- Existing accepted integrated reference smoke remains valid and unchanged:
  source revision `b312336`, six execution checkpoints, 352,099 execution
  observations, execution hash prefix `9d40fd652c0c...`, and persisted
  evidence digest
  `3dc2ac37e4e0d594eb17fada96cdb29ebc2bc2ad82a2caf714019a55fb5d66d2`.
  It is compatibility evidence only, not a long-run realism or funding claim.
- Independent analyzer work found and fixed deterministic ecology-HHI
  accumulation ordering in `1bf1b35`; current replay matches all scientific
  result subobjects from the archived smoke. The old ecology artifact differs
  only in the last floating-point bits of HHI because it was produced before
  that analyzer fix. The analyzer-only performance branch rejected a fused
  decoder prototype (`fccde0e`) after malformed-input/duplicate-key semantics
  diverged despite a measured valid-replay speedup.
- Preserve the four pre-existing user-owned tracked scoreboard edits and all
  untracked retained evidence. Current disk free space is approximately 237 GB.
  Do not stage or delete those files. No holdout seeds have been consumed by
  the integrated candidate; holdout execution remains gated.
- Safe continuation sequence: inspect `30176e0`; update and commit the
  protocol hardening, extractor hardening, and precommitted scorer as separate
  logical commits; run shell/tests and obtain the independent reviewer result;
  rebuild from a clean source tree with VCS metadata; only then launch the
  registered development cells sequentially with disk/RAM monitoring. Do not
  infer completion from host process names and do not consume holdouts before
  the candidate gate qualifies.

### 2026-08-31 — R2 calendar gate remains blocked after independent review

- The active scientific branch is `autoresearch/ffa-ecology-gen0`; code
  correction `494d696` is pushed. It follows the rejected `83dc7b1` review by
  Banach (Sol-xhigh), which found unbound/empty venue identity, a shell
  timeline test that did not exercise the extractor's default expectation,
  and an unsafe 5 GiB disk launch floor.
- `494d696` now rejects missing/empty calendar venue identity, binds the exact
  registered `central,north,south` set in the analyzer/R2 extractor and
  activation/integrity predicates, compares the literal timeline with the
  maintained helper before exercising the default checker, and derives the
  launch floor from retained evidence measurements (approximately 51 GiB
  free). Focused calendar/exchange tests and the R2 contract pass.
- A dirty-tree full `make test` reached the Go and R2 contract tests but was
  correctly rejected by the clean-worktree parity/archive guard; a clean full
  rerun, vet, targeted race check, and a fresh exact-tree Sol-xhigh review of
  the post-correction HEAD remain required. Current free space is about 28
  GiB, so no development cell can safely launch yet. No R2 development,
  parity, archive/prune, or holdout `619/631/641` evidence has run.
- Continue: commit/push this append-only state record, run the clean mechanical
  gates, obtain independent acceptance, then rebuild pinned Go 1.27 binaries.
  Resolve the measured disk-capacity stop condition before dev-607. Preserve
  the incomplete temporary evidence tree and all historical results.

### 2026-08-31 — corrected clean gate passed; promotion and capacity still block

- The venue/fixture/capacity correction is `494d696`; the append-only state
  record is pushed in `558fe21`. On the clean exact tree, `GOMAXPROCS=4 make
  test`, `go vet ./...`, and `GOMAXPROCS=4 go test -race
  ./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests` all pass. The earlier
  dirty-suite failure remains only a diagnostic for the clean-worktree guard.
- Banach's review of `83dc7b1` remains a historical rejection. Fresh
  independent Sol-xhigh acceptance of exact current HEAD is still required;
  no binary rebuild or development cell may start before it.
- The runner’s measured capacity floor is approximately 51 GiB free; the host
  currently has approximately 28 GiB. Do not lower the floor or launch a
  partial run. No R2 evidence, parity control, archive/prune operation, or
  holdout `619/631/641` has been consumed.

### 2026-08-31 — current stop after safe cache cleanup

- The regenerable Go build/test cache was cleaned without touching research
  evidence, recovering approximately 4.85 GiB. Free space is now about 32
  GiB, still below the approximately 51 GiB R2 launch floor. The incomplete
  derivative-proxy tree and all retained historical evidence remain preserved.
- The exact corrected code is covered by clean full tests, vet, and targeted
  race checks. Fresh Sol-xhigh review of the post-correction HEAD remains
  mandatory, but the reviewer service currently refuses new threads at its
  limit; prior reviews of superseded commits are not acceptance.
- Do not lower the capacity floor, compress/delete uncontracted evidence, build
  promoted binaries, or launch dev-607 while either the reviewer or capacity
  gate is unresolved. Resume with fresh exact-tree review, then pinned build,
  only after both gates clear.

## Append-only operational update: F3 epoch correction — 2026-09-09

- The active scientific branch is `autoresearch/ffa-ecology-gen0` at pushed
  HEAD `90d0ffbf32a07f3393a5be903b30fa9c954f723a`, with a clean worktree and
  no cherry-pick in progress. This commit is the minimal simulator-semantic
  correction for reachable F3 cross-margin stale-sibling marks; it preserves
  the prior R2 calendar and the historical negative survival result.
- The correction commits a coherent exchange-owned mark epoch across
  perpetuals, dated futures, and options, fails closed on unavailable sibling
  marks, serializes mark producers with risk sweeps, and provides the optional
  `PositionMarginSnapshotter` extension for mark-dependent maintenance. The
  built-in instrument path is covered by focused regressions and an independent
  Avicenna Sol-xhigh review, which returned conditional acceptance with no
  built-in-instrument blocker. The review limitations (epoch availability is
  not a freshness/TTL guarantee; custom hidden maintenance inputs require the
  snapshot interface) remain explicit.
- Clean post-commit `make test`, `go vet ./...`, focused tests, focused race
  tests, and `git diff --check` pass. No development cell, parity control,
  binary launch, or holdout was consumed. The latest fetch found no new
  performance-port or economic-red-team commits beyond the recorded refs.
- The retained historical dev-607 archive remains unchanged. Its multi-symbol
  exposure topology makes F3 reachable, but it has no observable liquidation
  checks, liquidations, or margin calls; corrected semantics therefore require
  a successor development rerun rather than offline trajectory repair.
- The current R2 candidate remains explicitly `NON-VIABLE AT THE 24H
  MARKET-SURVIVAL GATE`. The separately named CDF-liquidity successor remains
  a negative, off-branch development result and is not merged by this fix.
  Before any new cell, finish the binary-evidence successor promotion/review
  boundary, build the pinned candidate, and use development cells only. Holdout
  seeds `619/631/641` remain behind explicit freeze authorization.

## Append-only operational update: SV1C asynchronous closure — 2026-09-09

- The separate CDF successor branch is now at `1fda960` and was inspected
  without switching or merging it. Its Lovelace Sol-xhigh review rejects SV1C
  for scientific promotion while accepting the corrected global-order replay
  as valid analysis-only negative evidence.
- The rejection is substantive: only 7/12 supplier–venue instances had the
  required post-fill inventory-responsive decision, and supplier-removal
  reconstruction covered 900/939 snapshots. No activation predicate was
  relaxed, no supplier was retuned, and no historical trajectory was repaired.
  SV1C therefore does not authorize capacity seed 659, full development,
  freeze, or holdouts `619/631/641`.
- This branch remains the corrected scientific baseline at `90d0ffb` plus its
  F3 documentation commit. Any further CDF work must be a separately named,
  independently motivated preregistered successor; selecting the seven
  successful instances or weakening the all-roster criterion would be post hoc
  rescue. No new CDF code is being imported here.

## Append-only operational update: SV1D asynchronous closure — 2026-09-10

The newer `feature/r2-cdf-survival-successor-sv1d` branch was inspected
without switching or merging it, through pushed tip `039f008`. Its seed-659
five-minute activation attempt is **INVALID ARM EVIDENCE / NON-ADVANCING
GATE**: treatment, same-roster mode-off, and no-roster arms all reached an
early scheduled South option-risk failure while the underlying CDF/USD book
was transiently empty. None reached a valid endpoint, none recorded a
one-sided supplier decision, and treatment/mode-off execution streams were
identical. Lagrange's independent review rejects interpreting this as either
activation or a valid negative result.

The branch also rejected its first outcome-bearing capacity procedure and
re-registered a synthetic outcome-neutral binary workload. Neither that
capacity path nor SV1D code is part of the authoritative branch. No main-tree
development cell, freeze, or holdout was run; `619/631/641` remain untouched.
The next permissible SV1D action is source-level diagnosis plus a minimal
non-economic producer/fixture repair, then exact-tree review and a fresh
seed-659 activation probe only if the repaired contract can produce a valid
endpoint. Do not weaken strict valuation or force one-sided activation.

## Append-only operational update: strict CDF equity correction — 2026-09-10

The exact scientific branch is now clean and pushed at
`3fe893b5626a33994bdf47015f1a1b306a8a7fdc`. This successor correction follows
the binary-evidence integration and CDF balance reconciliation. Strict CDF
decision validation now distinguishes a current local risk mark from cached
equity, reconstructs marked equity and peak from initial account state plus
actor-visible fill deltas, and preserves cached risk state through pending,
missing, or unavailable observations. The binary decision schema is explicitly
versioned (`v2`); v1 decoding remains supported.

The bounded clean gate `GOMAXPROCS=4 GOMEMLIMIT=8GiB make test` passed on this
exact revision, including all package tests and the integrated long-run, R2,
and archive contract suites. No development cell, capacity attestation,
parity control, or holdout was run. `619/631/641` remain untouched. The
latest fetch of `origin/autoresearch/v2-performance-research` found no commit
after reviewed `b1847ac`; no performance code was imported.

This correction is not yet independently accepted for promotion. Before a
valid SV1D activation probe, the successor still needs strict end-to-end CDF
evidence auditing and the preregistered concentration/lifecycle diagnostics:
supplier attribution, quote lifetimes, inventory/PnL exposure,
withdrawal/reprice behavior, and removal-counterfactual coverage. The old R2,
SV1C, and invalid SV1D outcomes remain historical negative/non-advancing
records and are not rewritten.

## Append-only current checkpoint: exact-tree CDF lifecycle hardening — 2026-09-10

The authoritative scientific branch is clean and pushed at `c2b0ad7`
(`test: cover CDF lifecycle edge cases`), with semantic parents
`1298a7f` (`fix: enforce causal CDF quote lifecycle`) and `fbdec9d`
(`fix: bind rendered payloads to binary evidence`). The predecessor
`4965ada` lifecycle checkpoint was independently reviewed by Goodall the 2nd
(Sol-xhigh) and rejected for promotion. Its seven findings are preserved in
the triage note; this checkpoint addresses them with attributable changes and
adversarial regressions rather than treating the rejected verdict as
acceptance.

- Epoch-4 binary frames now carry a SHA-256 commitment to the canonical JSON
  payload, the renderer verifies it, and strict CDF source/render identity
  comparison includes the payload digest. Epoch-3 historical streams remain
  readable, while the registered successor contract is epoch 4.
- CDF supplier decisions carry an explicit `replaces_order_id` in decision
  schema v3 (v1/v2 decoding remains supported). Strict analysis requires the
  replacement link and changed terms before counting a completed reprice.
  The audit rejects multiple simultaneous live supplier orders and terminal
  order-ID reuse.
- Forced exchange cancellation evidence is emitted after the public
  `BookDelta`, allowing strict depth reconstruction to observe the post-removal
  state. A legitimate actor-cancel/forced-cancel `ORDER_NOT_FOUND` race is
  accepted only for a previously recorded forced terminal order.
- Strict decision reasons are restricted to the preregistered economic
  lifecycle vocabulary. Per-supplier activation now requires an actual
  withdrawal or reprice cancellation; aggregate arbitrary cancellations no
  longer satisfy the lifecycle gate.
- Focused tests cover payload mutation after attestation regeneration,
  explicit replacement identity, one-live-order enforcement, terminal-ID
  reuse, forced-cancel races, partial-to-full fills, same-timestamp sequence
  inversion, and producer BookDelta ordering.

The exact clean bounded gate `GOMAXPROCS=4 GOMEMLIMIT=8GiB make test` passed
on `c2b0ad7`, including package, integrated-long-run, R2, and archive suites;
the focused analyzer/exchange/multivenue suites and `git diff --check` also
passed. The performance branch was fetched at this natural semantic
checkpoint and has no commit after reviewed `b1847ac`; no performance code was
merged. RAM remained below the 8 GiB process limit with approximately 24 GiB
host-available, and approximately 38 GiB disk was free.

This is still a promotion checkpoint, not launch authorization. No binary
rebuild, capacity run, development cell, freeze authorization, or holdout was
consumed; holdouts `619/631/641` remain untouched. Next: fresh exact-tree
Sol-xhigh review of the complete successor, `go vet`, targeted race and
fresh-process evidence-neutrality checks, an actual binary-evidence capacity
measurement, then a provenance-pinned Go 1.27 build and the development-only
SV1D activation probe before any registered 24-hour cell.

## Append-only current checkpoint: Hooke review rejection and strict-audit correction — 2026-09-11

Hooke the 2nd (Sol-xhigh) independently reviewed exact clean tree `b4f8bd4`
without running experiments and **REJECTED** promotion. The review identified
six concrete blockers: strict digest binding used the inner derivative payload
instead of the wrapped frame payload; legitimate actor wait reasons were
rejected and reasons were self-attested; rejected replacement requests could
lose `replaces_order_id`; no strict test exercised `events.evs` through the
production renderer into the complete strict audit; concentration/removal
diagnostics lacked exact per-supplier event-time depth evidence; and the
activation predicate required per-supplier withdrawal/reprice although the
preregistration required only a global lifecycle event.

The response is committed and pushed in `cde2e62` and `146bc02`. The strict
audit now hashes the exact wrapped frame payload, derives/validates the
decision reason from observable state, accepts the registered actor lifecycle
vocabulary, restores replacement lineage after rejected submissions, records
event-time per-supplier depth, computes side-specific concentration and
removal-counterfactual diagnostics, and applies the preregistered global
withdrawal/reprice criterion. `146bc02` also adds a production-renderer
strict end-to-end binary fixture with sidecar removal and corruption-sensitive
identity checks.

On the corrected exact tree, clean `make test`, `go vet ./...`, focused
analysis/exchange/multivenue tests, targeted race tests, and fresh-process
determinism/evidence-neutrality tests pass; `git diff --check` passes. The
review rejection and its corrections are not an acceptance verdict: a fresh
independent review of `146bc02` is now the promotion boundary. No capacity
floor, binary rebuild, development cell, freeze authorization, or holdout has
been consumed. Performance remains deferred at reviewed ref `b1847ac`; no
performance code was imported. Holdouts `619/631/641` remain untouched.

## Append-only current checkpoint: second exact-tree review rejection — 2026-09-11

Two independent Sol-xhigh reviewers examined exact clean tree `764f10d` and
rejected promotion without running capacity, development, or holdout work.
They independently reproduced a reachable strict-evidence failure: production
exchange cancellation emits the post-removal `BookDelta` before
`OrderCancelled`, while the analyzer keeps the supplier order live until the
latter event. A supplier-only side can therefore make reconstructed supplier
depth exceed public depth on a legitimate withdrawal/reprice. The current
strict E2E fixture bypasses this production event sequence.

The review also identified a false-positive risk in post-fill activation:
market/reference movement can change `TargetPosition` and satisfy the
inventory-response count without an inventory-attributable quote/risk change;
and a strict completion check currently verifies hashes without validating the
required sidecar schemas/content through a production activation adapter. A
secondary reachable false rejection was found for `limit_or_touch_unavailable`
when an existing quote leaves positive stale quote fields in `baseDecision`.

These are promotion blockers, not economic results. No historical trajectory
was changed and no experiment was consumed. The next correction must preserve
exchange event order and economics, make depth attribution coherent across the
documented cancellation transition, make post-fill response independently
inventory-attributable, validate complete arm artifacts fail-closed through the
actual production extraction path, and cover the real binary/renderer/audit
route. The branch remains blocked before capacity, pinned build, SV1D probe,
development cells, freeze, and holdouts `619/631/641`.

## Append-only current checkpoint: CDF strict-audit correction `66f5781` — 2026-09-11

The exact scientific HEAD is pushed as `66f5781` on
`autoresearch/ffa-ecology-gen0`, with a clean worktree and no cherry-pick in
progress. This is a minimal evidence/audit correction after the two independent
Sol-xhigh rejections of `764f10d`; it does not alter R2 economics, actor
rosters, exchange matching, seeds, or any historical trajectory.

The strict CDF audit now reconciles the documented production ordering of a
public removal `BookDelta` followed by `OrderCancelled`: removal deltas are
held at their exact event-time public state until the lifecycle frame updates
the supplier order map, while additive depth transitions remain observable
immediately. The production-like binary fixture now exercises shared aggregate
levels, fill/cancel ordering, one-sided posting/restoration, and the complete
renderer-to-audit path. Post-fill response credit compares the delayed public
market/risk state and requires changed quote terms; private target/reference
movement or a new request ID alone cannot count. The stale positive quote-term
failure path has a direct regression.

Completion evidence is now fail-closed beyond hashes: run-status schema and
sentinels, Greek/latency sidecar structure, monotonic terminal checkpoints,
fixed-file manifests, binary/evidence-only artifacts, and retained raw-evidence
manifest accounting are validated. `mvanalyze -metric cdfactivation` and
`scripts/audit-v2-r2-sv1d-activation.sh` are the production adapter path.

Verification at this checkpoint:

- `go test ./analysis -count=1` passed, including the strict production
  renderer, sidecar corruption, post-fill causality, and stale-quote tests.
- `go test ./evstream ./types ./exchange ./simulations/multivenue ./cmd/mvanalyze
  -count=1` passed; the multivenue portion completed in 204.350 seconds.
- `go vet ./...`, `git diff --check`, shell syntax checks, and clean
  `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test` passed; the latter included the
  integrated-long-run, R2, and archive contract suites.
- Targeted race/fresh-process checks and the new exact-tree independent review
  are still pending; no acceptance is inferred from this mechanical gate.

The performance branch was fetched at this checkpoint and remains unchanged
after reviewed `b1847ac`; no performance code was imported. Host state was
approximately 38 GiB free disk and 24 GiB available RAM. No capacity
measurement, pinned Go 1.27 build, development cell, freeze authorization, or
holdout `619/631/641` was consumed. Next: targeted race and fresh-process
evidence-neutrality checks, then one fresh exact-tree Sol-xhigh review. Only an
accepted review permits binary-evidence capacity measurement and the pinned
build/activation sequence.

## Append-only current checkpoint: exact-tree mechanical gates complete — 2026-09-11

The exact scientific tree at `50210cd` is clean and pushed. The previously
pending bounded checks are now complete: targeted `go test -race` for
`./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests` passed in
the retained `exsim-v2-race-50210cd.log` file in the system temporary
workspace, and the fresh-process
determinism/evidence-neutrality matrix for `simulations/multivenue` passed in
the retained `exsim-v2-fresh-50210cd.log` file in that workspace. Both used `GOMAXPROCS=2` and
`GOMEMLIMIT=4GiB`.

Together with the earlier focused suites, clean bounded `make test`, `go vet`,
shell syntax, and diff checks, this closes the mechanical verification gate
for the current successor tree. It is not a scientific promotion verdict:
one fresh independent exact-tree Sol-xhigh review remains required. The next
permitted actions after acceptance are a measured binary-evidence capacity
floor, a clean provenance-pinned Go 1.27 build, and the smallest SV1D
activation probe. No capacity measurement, development cell, freeze, or
holdout `619/631/641` has been consumed. The performance feed was re-fetched
and remains at reviewed `b1847ac`; no performance code was imported.

## Append-only current checkpoint: fresh exact-tree review rejection — 2026-09-11

Two independent Sol-xhigh reviewers examined clean exact HEAD `144d151` and
rejected promotion. They agreed that the R2 calendar, risk/economic
semantics, opt-in finite CDF roster, and anti-oracle constraints remain intact;
no capacity, binary launch, development, or holdout work was performed.

The rejection identified reachable evidence/provenance blockers:

- a shared-price cancellation can emit a nonzero aggregate `BookDelta` before
  `OrderCancelled`, while the analyzer still attributes the canceled supplier
  order;
- the strict fixture models full-fill removal before `OrderFill`, contrary to
  the producer path, and can leak later replacement state into earlier depth
  observations;
- post-fill response credit can still arise from private reference/target or
  order-identity changes rather than an inventory-attributable response;
- raw manifest records and checkpoint terminal binding are incomplete in the
  Go strict validator;
- the shell adapter derives “expected” hashes from the artifact under audit
  and does not authenticate the analyzer/renderer or immutable probe
  identities;
- the binary-only runner, manifest, run-status, and strict-audit requirements
  are inconsistent, and no immutable SV1D treatment/mode-off/no-roster probe
  configs are registered under `research/configs`.

These are evidence-contract and promotion defects, not a scientific result
about CDF liquidity or a reason to rewrite R2/SV1C/invalid-SV1D history. The
next correction must align the actual producer/runner/renderer/audit path,
register the probe identities, repeat mechanical gates, and obtain another
fresh exact-tree review before capacity or execution.

## Append-only current checkpoint: binary evidence/parity hardening — 2026-09-11

The exact scientific tree is clean and pushed at `7e8d9fa`. The binary
successor contract now explicitly declares `evidence_contract_version: 2` in
all registered R2 development and reserved config identities; the simulator
therefore emits the epoch-4 wrapped payload/global-order evidence required by
the successor rather than silently defaulting to the legacy binary envelope.

The completion contract binds the four market-data receipt sidecars to both
the evidence manifest and run status, validates every checkpoint against the
corresponding binary event prefix, rejects fixed-file symlinks, and requires
nonempty microstructure and courier-latency observations for strict CDF
completion. New regressions cover sidecar mutation, intermediate checkpoint
hash mutation, and fixed-file symlink substitution.

Parity controls now invoke the production `evsrender` binary for both full-log
seed-607 arms, validate its report and rendered attestation against runtime
event/frame hashes, compare actual reconstructed venue files, and bind the
renderer clean build identity into the parity attestation. The hermetic
archive fixture uses a contiguous epoch-4 venue sequence so this is a real
renderer-path test rather than a copied runtime attestation.

The clean bounded `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test` passed on this exact
tree, including package, integrated-long-run, R2, archive, and renderer parity
suites. The performance feed was fetched again and has no commit after reviewed
`b1847ac`; no performance implementation was imported. No capacity measurement,
pinned scientific binary, SV1D probe, development cell, freeze, or holdout was
run; holdouts `619/631/641` remain untouched.

This is a mechanical evidence checkpoint, not promotion. Remaining gates are
targeted race/fresh-process checks, one fresh exact-tree independent Sol-xhigh
review, a measured binary capacity floor, a clean pinned Go 1.27 build, and the
development-only SV1D activation probe with its separate treatment/mode-off/
no-roster scoring. Historical R2/SV1C/invalid-SV1D claims remain unchanged.

## Append-only current checkpoint: current-tree mechanical closure — 2026-09-11

At exact clean HEAD `0bfb809`, targeted race coverage
(`go test -race ./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`)
passed with `GOMAXPROCS=2 GOMEMLIMIT=4GiB`. The fresh-process baseline
determinism and binary evidence/log-mode neutrality checks also passed; their
small retained logs are `exsim-race-0bfb809.log`, `exsim-fresh-0bfb809.log`,
and `exsim-binary-neutral-0bfb809.log` in the system temporary workspace.
`go vet ./...`, diff hygiene, and the renderer-backed clean `make test` gate
remain green. No development or holdout world has run.

The exact promotion boundary is now one fresh independent Sol-xhigh review of
the complete clean tree. If accepted, the next actions are an actual binary
capacity measurement, clean Go 1.27 provenance-pinned builds, and the
development-only SV1D treatment/mode-off/no-roster activation probe. Capacity,
probe, freeze, and holdout execution remain unauthorized; `619/631/641` are
untouched. The performance feed remains at reviewed `b1847ac`.

## Append-only checkpoint: SV1D provenance/runner mechanical closure — 2026-09-11

Exact scientific HEAD `301a131` is clean and pushed. The SV1D development-only
adapter now binds the immutable tri-arm plan and every arm result to the
simulator, analyzer, and renderer SHA-256 identities. The production
`evsrender` command emits a self-attested clean-build identity bound to the
rendered binary-evidence attestation; strict analysis verifies that identity
before reconstructing activation metrics. A guarded runner accepts only the
registered seed-659 treatment/mode-off/no-roster arms, a clean same-revision
Go 1.27 linux/amd64/v1 trimpath CGO-disabled tool set, an explicit review and
probe authorization, and a separately measured binary-capacity attestation.
It refuses holdouts and refuses to overwrite any output namespace.

The exact tree passed clean bounded `make test`, `go vet ./...`, targeted
`go test -race ./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`,
and the fresh-process determinism/binary-evidence neutrality checks. A
temporary non-campaign 3-second binary run and the actual `evsrender` binary
also passed: 942 event frames, 15 reconstructed routes, and a clean
Go 1.27/self-attested renderer bound to the rendered attestation. The first
dirty-tree make test was intentionally rejected by the parity guard; the
clean rerun passed. No capacity measurement, development cell, freeze, or
holdout `619/631/641` was consumed, and no historical evidence was changed.

The performance feed was fetched from `origin` through reviewed `b1847ac` and
has no newer commit; no performance implementation was imported. The next
boundary is one fresh exact-tree independent Sol-xhigh review of `301a131`.
Only acceptance permits the actual binary-capacity measurement, pinned
scientific build, and registered SV1D activation probe.

## Append-only checkpoint: fail-closed SV1D observation coverage — 2026-09-11

The exact scientific tree is clean and pushed at `e4d069c`. This correction
responds to the fresh exact-tree Sol-xhigh rejection of `b1d2a3a`: strict CDF
activation and control audits now require the registered public-book sampling
cadence, detect missing/shifted opening or terminal coverage, and reconstruct
the initial empty state only when the first snapshot is exactly one interval
after the registered start with no earlier public transition. Strict depth
observations retain whether they came from snapshots or deltas, so the cadence
check cannot be satisfied by arbitrary transitions alone.

Persistence now measures one uninterrupted run across adjacent non-two-sided
states, including `bid_only -> empty -> ask_only`, as required by the registered
kill predicate. The production fixture was made a complete one-second grid and
its synthetic order timeline was moved so every public-depth transition remains
causally consistent; no production economic rule was changed.

Verification passed: focused strict renderer audit, full `go test ./analysis`,
and clean bounded `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test`, including all package,
integrated-long-run, R2, archive, and renderer-parity suites. The dirty-tree
make-test attempt was rejected only by the required clean parity/archive
guards; the committed rerun passed. No capacity measurement, pinned campaign
build, SV1D arm, development cell, freeze, or holdout `619/631/641` was run.

The remaining rejected-review items are unchanged: measured outcome-neutral
binary capacity, exact registered tri-arm plan identities and digest,
content-bound external review attestation, per-arm corpus/result hashes and
immutable manifest, failure-continuing runner semantics, metadata identity,
content-addressed launch binaries, and a separately named 24-hour SV1D config
set. The performance feed remains at reviewed `b1847ac`; no performance code
was imported.

## Append-only checkpoint: SV1D capacity/activation runner closure (`1c870f7`) — 2026-09-11

The exact scientific tree is clean, pushed, and now at `1c870f7`. The
capacity verifier binds every retained resource trace to the attested
filesystem identity and rejects an unbound trace. The activation adapter now
retains a typed incomplete result when a simulator, renderer, manifest,
terminal valuation, or strict audit step fails, then scores all three arms and
exits nonzero; a failed arm cannot be silently omitted or counted as a
successful control. Capacity-arm record and filesystem mutations have direct
regressions.

Verification at this exact tree:

- bounded `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test` passed, including package,
  integrated-long-run, R2, archive, and renderer-parity contracts;
- `GOMAXPROCS=2 GOMEMLIMIT=4GiB go vet ./...` passed;
- targeted `go test -race ./analysis ./cmd/sv1dprobe ./cmd/sv1dresource
  ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1` passed;
- fresh-process `TestRunsOfOneSeedProduceOneEventStream` plus binary evidence
  log-mode neutrality passed; shell syntax and `git diff --check` passed.

The pre-commit dirty-tree gate was retained as a diagnostic failure only; the
clean committed gate above is the authoritative result. No capacity
attestation, pinned Go 1.27 campaign build, SV1D probe, development cell,
freeze, or successor holdout `619/631/641` has run. The current next boundary
is one fresh independent Sol-xhigh review of this complete exact tree. Only an
accepted review permits the finite-cgroup capacity preflight, pinned build,
and seed-659 activation probe. The performance branch was refreshed through
reviewed `b1847ac` with no newer commit and no performance implementation was
imported.

## Append-only operational update: strict SV1D launch provenance (`d96fa1c`) — 2026-09-11

The code-bearing scientific tree is clean, pushed, and pinned at `d96fa1c`;
this entry is documentation-only. The checkpoint preserves the R2
calendar/lifecycle, risk, actor, and historical
negative semantics. The SV1D successor adapter now binds strict arm audits and
tri-arm scoring to the reviewed tree, canonical plan, parent registration,
amendment, review attestation/report/key, measured capacity attestation and
records, capacity runner, activation runner, activation metadata, simulator,
analyzer, renderer, evidence epoch, and bounded runtime envelope. The runner
also performs live filesystem-identity and cgroup/OOM/memory/disk checks while
the simulator and renderer execute, and continues to retain typed incomplete
arm results on failure. Legacy historical JSON audit identities remain
accepted only through their legacy path.

Verification at this exact tree passed: clean bounded `GOMAXPROCS=2
GOMEMLIMIT=4GiB make test` including package, integrated-long-run, R2,
archive, and parity contracts; `go vet ./...`; targeted `go test -race
./analysis ./cmd/sv1dprobe ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`;
fresh-process determinism and binary-evidence/log-mode neutrality checks;
shell syntax and `git diff --check`. The first pre-commit full test was
rejected only by the intentional dirty-tree archive/parity guard; the clean
committed rerun passed.

No capacity attestation, pinned Go 1.27 campaign build, SV1D activation,
development cell, freeze, or successor holdout `619/631/641` has run. The
performance branch was refreshed through last-reviewed `b1847ac`, with no
newer commit and no imported performance code. The next boundary remains one
fresh exact-tree independent Sol-xhigh review; only acceptance permits the
actual binary-capacity measurement and seed-659 activation probe.

## Append-only operational update: retained capacity revalidation (`7d92e73`) — 2026-09-11

The fresh independent Sol-xhigh review of exact tree `ccc29c2` rejected one
valid promotion blocker: strict arm-audit and final-score checks validated the
copied capacity attestation but did not reopen the retained capacity manifest,
resource traces, capacity tools/configs, arm evidence, or rendered evidence
under `capacity_root`. The review accepted the R2 calendar/lifecycle and
current correctness semantics and authorized no capacity or scientific run.

The code-bearing scientific tree is now clean, pushed, and pinned at `7d92e73`;
this entry is documentation-only. Strict retention now binds both
`capacity_root` and `capacity_records_root` from activation metadata to the
attestation and invokes the full `VerifySV1DCapacityAttestation` contract at
each strict arm audit and tri-arm score. That rechecks retained input hashes,
measurement-record manifest and traces, resource aggregates, capacity-arm
artifacts, binary evidence, rendered evidence, and tool identities. A
regression deletes the measurement manifest and confirms the retained bundle
is rejected.

Verification at this code-bearing tree passed: clean bounded `make test`
including all package, integrated-long-run, R2, archive, and parity contracts;
`go vet ./...`; targeted `go test -race ./analysis ./cmd/sv1dprobe
./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`; fresh-process
determinism/binary-evidence neutrality; shell syntax and `git diff --check`.
No capacity attestation, pinned Go 1.27 campaign build, SV1D activation,
development cell, freeze, or holdout `619/631/641` was consumed. The
performance branch remains reviewed through `b1847ac` with no newer commit or
imported implementation. Promotion again awaits one fresh exact-tree
independent Sol-xhigh review; only acceptance permits capacity preflight and
the seed-659 activation probe.

## Append-only pause checkpoint: SV1D scoring provenance correction in progress — 2026-09-11

Following the second fresh exact-tree Sol-xhigh rejection, the worktree retains
an uncommitted correction on scientific HEAD `96c34065`. Complete arm scoring
now re-audits the retained run/rendered evidence and compares the result with
the retained arm record; strict result records are copied by the runner under
the metadata-bound `provenance/arm-results/<arm>-<sha256>.json` layout and read
with descriptor-bound no-symlink traversal. Scientific rejection statuses now
populate venue diagnostics, aggregate durations/fractions, and failed
predicates before branching to `TREATMENT_NOT_ACTIVATED` or
`ANTI_CHEATING_REJECTED`. The activation metadata contract records the result
root. No economic semantics, historical evidence, capacity artifact, or
experiment was changed.

The focused `analysis`/`cmd/sv1dprobe` tests and the focused
`evstream`/`types`/`exchange`/`simulations/multivenue` suites passed; shell
syntax and `git diff --check` passed. A new bounded full `make test` reached
the package suites through `simulations/latencylab` but was deliberately
stopped with SIGINT at the quota boundary before completion, so this patch is
not yet committed or promoted. No child test process remains; latest resource
check showed about 24 GiB available RAM and 34 GiB free disk. Next session:
rerun cleanly in tmux, then vet/race/fresh-process checks, commit and push this
checkpoint, append the review correction record, and obtain one fresh exact-tree
independent review. Do not run capacity, SV1D activation, development cells,
freeze, or holdouts `619/631/641` before that review.

## Append-only checkpoint: SV1D scoring provenance correction mechanically green — 2026-09-12

The previously paused correction was revalidated after resuming. The full
bounded `make test` completed every package, integrated-long-run, R2,
multivenue, and test-suite check; its only nonzero result was the intentional
dirty-worktree archive/parity refusal. `go vet ./...`, targeted race coverage,
fresh-process determinism/evidence-neutrality, shell syntax, and
`git diff --check` all pass. The added regression now also proves a valid
per-venue threshold failure is retained alongside a treatment activation
failure. The performance branch was fetched again and has no commit after
reviewed `b1847ac`; no performance code was imported.

The correction is ready for one attributable commit/push. It changes no
economic mechanism or historical artifact. After the commit, rerun the clean
full/archive gate, then obtain the next exact-tree independent Sol-xhigh review;
capacity preflight, SV1D activation, development cells, freeze, and holdouts
`619/631/641` remain unauthorized.

## Append-only pause checkpoint: resource-bound SV1D runner (`aaf751e`) — 2026-09-12

The exact scientific tree is now clean, pushed, and pinned at `aaf751e`. This
checkpoint adds only operational evidence hardening to the development-only
SV1D runner. Each monitored simulator, renderer, audit, and score stage now
retains a fail-closed resource record containing sample count, cgroup peak,
host/disk minima, swap, and OOM deltas. A successful tri-arm run additionally
publishes a content-bound ten-stage resource manifest linked to activation
metadata, score, score-corpus manifest, and the measured resource policy. A
resource manifest is not emitted for an incomplete or out-of-envelope run.

The pre-commit bounded full gate completed all Go packages and contract tests;
the only failure was the intentional dirty-worktree archive/parity guard. The
post-commit clean `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test` passed, as did the
focused `cmd/sv1dprobe` test, shell syntax, and `git diff --check`. Earlier
exact-tree vet, targeted race, fresh-process determinism, and evidence
neutrality checks remain attached to the preceding code checkpoint; they must
be rerun or explicitly revalidated on this final tree before review. No
economic mechanism, R2 calendar, configuration, historical evidence, or
binary trajectory changed.

The last resource observation was approximately 33 GiB filesystem free, 27 GiB
host memory available, and zero swap; this is an operational snapshot, not a
capacity attestation. The performance feed was fetched through reviewed
`b1847ac` and has no newer commit. No fresh exact-tree Sol-xhigh review,
capacity measurement, pinned campaign build, SV1D activation probe,
development cell, freeze authorization, or holdout `619/631/641` has run.

This is the deliberate weekly quota pause. Next session, use the smallest
promotion loop: verify the clean exact tree and performance delta; rerun the
remaining final-tree vet/race/fresh-process checks; obtain one independent
Sol-xhigh review; only if accepted perform the binary-capacity preflight and
build pinned Go 1.27 binaries; then run the development-only seed-659 SV1D
activation probe before considering `dev-607`. Do not spend another large
experiment or touch holdouts until those gates are accepted. Preserve failed
and historical artifacts; prune only files whose retention/measurement
contract has already passed.

## Append-only checkpoint: final-tree mechanical gate complete (`24f3ee1`) — 2026-09-12

The source tree remains unchanged from `aaf751e`; `24f3ee1` contains only the
pause documentation. The final-tree checks are now complete: `GOMAXPROCS=2
GOMEMLIMIT=4GiB go vet ./...` passed; targeted race coverage for
`analysis`, `cmd/sv1dprobe`, `cmd/sv1dresource`, `cmd/mvanalyze`,
`cmd/prunegate`, and `tests` passed; and the focused fresh-process matrix for
`TestRunsOfOneSeedProduceOneEventStream`,
`TestBinaryEvidenceIsFreshProcessDeterministicAndNeutral`, and
`TestPerpExposureEvidenceIsFreshProcessDeterministicAndNeutral` passed. The
clean bounded `make test` at the preceding source commit passed all package,
integrated-long-run, R2, archive, and parity contracts. Shell syntax and diff
hygiene also pass.

No economic semantics, configuration, historical evidence, or experiment
authorization changed. The performance feed still has no commit after
`b1847ac`. The final resource observation is approximately 33 GiB free disk,
27 GiB available RAM, and zero swap. No capacity attestation, pinned campaign
build, SV1D activation, development cell, freeze, or holdout has run.

This closes the mechanical work for the week, not scientific promotion. The
next action is one independent Sol-xhigh review of exact tree `24f3ee1` using
the raw R2/SV1D contracts and provenance changes. If accepted, measure binary
capacity once, build pinned Go 1.27 binaries, and run only the development
seed-659 activation probe. Do not rerun completed gates or consume `dev-607`
until that probe is independently inspected; do not read holdouts `619/631/641`
before explicit freeze authorization.

## Append-only quota-stop checkpoint: descriptor-bound SV1D locking (`fe33dd8`) — 2026-09-12

The exact scientific tree is clean, pushed, and pinned at `fe33dd8`. This
code-bearing checkpoint adds the reusable Linux `sv1dlock` adapter and wires it
into the development-only SV1D activation and binary-capacity runners. The
adapter opens the existing namespace lock without truncation, rejects symlinked
parents/final paths using descriptor-relative `openat`, takes a non-blocking
exclusive `flock`, and passes the held descriptor to the child. Resource
manifest validation now binds the exact ordered ten-stage path/stage sequence.
The lock binary digest is retained in capacity and activation provenance.
No R2 economics, SV1D participant behavior, historical evidence, or experiment
configuration changed.

Focused package tests, shell syntax, and `git diff --check` passed. The bounded
full `make test` run completed every package and ordinary contract suite; its
only reported nonzero conditions were the intentional dirty-worktree parity and
archive guards, so a clean post-commit full gate remains deferred. The quota
pause began immediately after this commit; no clean full rerun, vet/race pass,
independent review, capacity measurement, pinned campaign build, SV1D probe,
development cell, freeze, or holdout `619/631/641` was run.

Next session: run the clean final-tree mechanical checks once, fetch only
performance commits newer than `b1847ac`, obtain one independent exact-tree
Sol-xhigh review, and proceed only if accepted to the registered binary
capacity preflight and seed-659 activation probe. Do not launch `dev-607` or
touch holdouts before those gates and explicit freeze authorization.

## Append-only mechanical promotion update: binary contract hardening (`fdbf077`) — 2026-09-12

The exact scientific tree is clean, pushed, and pinned at `fdbf077`. Two
code-bearing commits close the latest independent-review findings without
changing R2 economics, SV1D behavior, registered configurations, historical
evidence, or experiment authorization. `f1d5807` makes required binary
dictionary references fail closed, reserves id zero for optional absence,
rejects empty dictionary definitions, bounds index allocations before reading,
versions optional CDF decision strings to schema v4 and global balance symbols
to schema v2, and moves rendered-route duplicate/gap validation out of the
per-record O(n^2) path. `fdbf077` additionally validates the inherited lock
descriptor at the private capacity `--internal-arm` entrypoint, closing the
environment-marker bypass.

Verification at this exact tree:

- bounded `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test`: passed all Go packages and
  integrated-long-run, R2, archive, and parity contracts;
- `GOMAXPROCS=2 GOMEMLIMIT=4GiB go vet ./...`: passed;
- prescribed targeted race matrix for `analysis`, `cmd/sv1dprobe`,
  `cmd/sv1dresource`, `cmd/mvanalyze`, `cmd/prunegate`, and `tests`: passed;
- race coverage for changed evidence packages and exchange: passed;
- fresh-process matrix for baseline determinism, binary evidence neutrality,
  and perp-exposure evidence neutrality: passed;
- shell syntax, R2 contract checks, focused corruption/reference tests, and
  `git diff --check`: passed.

The performance feed was fetched through `b1847ac` and has no newer commit;
no performance implementation was imported. The host currently has about
32 GiB free disk, about 26 GiB available RAM, and no swap. No capacity
attestation, pinned campaign build, SV1D activation, `dev-607`, `dev-613`,
`dev-617`, freeze, or holdout `619/631/641` has run.

The sole remaining promotion action is one fresh independent Sol-xhigh review
of exact `fdbf077`, covering the R2 calendar/lifecycle semantics, current risk
hardening, binary evidence contract, strict scoring/provenance, resource
manifests, and both lock entrypoints. Only an accepted review permits the
measured binary-capacity preflight and pinned Go 1.27 build; the next scientific
execution remains the development-only seed-659 activation probe.

## Append-only review-closure checkpoint: binary evidence and arm handoff (`11da855`) — 2026-09-12

The exact scientific tree is clean, pushed, and pinned at `11da855`. The fresh
Sol-xhigh review of `81a6665` rejected two concrete promotion defects: CDF v4
numeric presence/value contradictions were silently normalized, and a direct
caller could enter the capacity internal arm with only a forged marker and
caller-opened lock descriptor. These were reproduced independently and closed
in separate commits `0d9c5e1` and `11da855`. The CDF decoder now rejects both
contradiction directions. The capacity path now requires the one-shot
pipe-backed handoff supplied by `sv1dresource`, validates its parent identity,
and has a regression proving direct forged entry cannot create an arm.

The exact post-fix tree passed focused package/control suites, clean bounded
`make test`, vet, prescribed race tests, changed evidence-package race tests,
fresh-process determinism/evidence-neutrality checks, shell syntax, and diff
hygiene. The performance feed was fetched through `b1847ac` with no newer
commit. No economics, configurations, historical evidence, capacity, binary,
activation, development cell, freeze, or holdout changed.

The weekly quota stop is at the next promotion boundary: obtain one fresh
independent Sol-xhigh review of exact `11da855`. Only acceptance permits the
measured binary-capacity preflight, pinned Go 1.27 build, and development-only
seed-659 activation probe. Do not launch `dev-607` or touch holdouts
`619/631/641` before those steps and explicit freeze authorization.

## Append-only quota-stop checkpoint: reviewed-arm authorization remediation (`86854e3`) — 2026-09-12

The exact scientific HEAD is clean, pushed, and pinned at `86854e3` with tree
`1e3cb97daadc770a32785978bb3fe1117fff8a8d`. A fresh Sol-xhigh review of the
prior candidate `0c19cdf` rejected the old public-wrapper/pipe-token arm
handoff: an arbitrary caller could reach the private capacity arm using a
caller-opened lock descriptor and environment marker. No scientific run had
used that path, so there is no historical impact.

The remediation is complete in `49dbcd4`, with wrapper attack coverage in
`b33f089` and `86854e3`. FD3 now remains the namespace lock through
`sv1dresource`; the internal arm revalidates the clean exact tree, signed
review and plan, review documents, target config identity, pinned
simulator/analyzer/renderer/runner identities, and the actual resource-parent
binary before any arm directory is created. The CDF v4 contradiction test now
covers every optional numeric field in both directions. Bounded focused Go
tests, the complete `simulations/multivenue` package, shell syntax, diff
hygiene, and the clean integrated-long-run contract passed. The public-wrapper
regression reaches the missing signed-review identity gate and creates no arm.

The performance feed was fetched through `b1847ac` and has no newer commit;
the VNext binary prototype remains deferred. No capacity measurement, pinned
campaign build, seed-659 activation, `dev-607`, `dev-613`, `dev-617`, parity
control, freeze, or holdout `619/631/641` has run. Disk is about 31 GiB free,
RAM about 26 GiB available, and swap is absent.

This is a deliberate quota stop at the next promotion boundary. Next session:
verify the clean tree and feed delta, run the remaining final-tree vet/race/
fresh-process/evidence-contract checks, obtain one fresh exact-tree Sol-xhigh
review, and only on acceptance run the registered binary-capacity preflight,
then pinned Go 1.27 builds and the development-only seed-659 activation probe.
Do not launch `dev-607` or inspect holdouts before those gates and explicit
freeze authorization. Preserve all prior review rejections and historical
artifacts.

## Append-only current-tree pointer after authorization remediation — 2026-09-12

The code-bearing remediation remains `49dbcd4`, with regression commits
`b33f089` and `86854e3`; the current clean branch also contains only the
append-only state records that followed them. The next exact-tree review must
resolve `git rev-parse HEAD` at launch and include every current documentation
record. The authorization gate remains closed: no capacity, activation,
development cell, freeze, or holdout has run.

## Append-only sampler review remediation — 2026-09-12

The fresh Sol-xhigh review of exact `f258af9` was independently rejected for
`SV1D-RSRC-001`: the resource sampler could record a maximum inter-sample gap
of `250894382` ns against the registered `250000000` ns ceiling because a
250-ms ticker was followed by variable-cost resource collection. The finding
was reproduced with a non-campaign three-second `/bin/sleep`; it is an
operational capacity-measurement defect with no historical impact because no
capacity or scientific run used this successor path.

Commit `5cd7b4d` corrects the sampler without changing R2 economics, SV1D
behavior, evidence format, or registered configs. The recorded interval stays
the strict maximum permitted observed gap; actual post-collection timestamps
are retained, and fixed half-interval deadlines provide collection headroom.
The strict validator remains fail-closed. A real multi-tick regression passes
five repetitions, and the repaired CLI reproduction reports 26 samples with a
maximum gap of `125773682` ns.

At the exact clean `5cd7b4d` tree, bounded `make test`, `go vet ./...`, the
targeted race matrix, fresh-process determinism/evidence-neutrality checks,
focused binary-evidence suites, and `git diff --check` pass. The performance
feed remains unchanged after reviewed `b1847ac`; no performance code was
imported. No capacity, pinned build, activation, development cell, freeze, or
holdout `619/631/641` was consumed.

Promotion remains closed pending one fresh exact-tree Sol-xhigh review of the
complete candidate. Acceptance is required before binary capacity and the
seed-659 activation probe; all development cells and holdouts remain
unauthorized. Full details are in
`research/v2-r5-sv1d-sampler-cadence-fix-2026-09-12.md`.

## Append-only review-availability stop — 2026-09-12

The fresh Sol-xhigh promotion review was attempted twice against exact clean
HEAD `82f452028740d535253ce2d822be750250466ff2` (tree
`ebd2e711618eb421ec8cb66c1828ed779f6388cd`) and both launches failed before
inference with backend HTTP 403. No ACCEPT or REJECT verdict is recorded; the
reviewer explicitly declined to fabricate one. The branch remains clean and
synchronized with origin.

This is an external review-availability stop, not scientific acceptance. No
capacity measurement, pinned build, activation probe, development cell,
freeze, or holdout `619/631/641` was run. When Sol-xhigh service/quota is
available, retry the same complete exact-tree review first; only an explicit
acceptance can authorize the binary-capacity preflight and seed-659 probe.

## Append-only repeated review-availability stop — 2026-09-12

The next fresh Sol-xhigh review was attempted twice against exact clean HEAD
`b1d66fec96281e82bafcc3c7b052cd9c68c3281a` (tree
`1475134b48e9cc05476938154689d5184c9d9835`). Both launches failed before
inference with backend HTTP 403, first through WebSocket and then HTTPS. No
scientific verdict was produced. The branch remains clean and synchronized
with origin.

This is the same external review-availability blocker, not acceptance or a
candidate rejection. No capacity, pinned build, activation, development cell,
freeze, or holdout `619/631/641` was run. Retry the complete exact-tree review
when Sol-xhigh service/quota is available; keep the promotion gate closed.

## Append-only blocked-gate record — 2026-09-12

The same external blocker recurred on three consecutive continuation turns:
each fresh Sol-xhigh/xhigh review launch failed before inference with HTTP 403
over the available transports. The latest attempt targeted exact HEAD
`1db902a3c2327da204f2e1b80ee271700ceffdf3` (tree
`b019a8f68b60e56b65e9716b6d506b0bb69f249e`). No scientific verdict exists.

The objective is left active but blocked at the independent-review boundary.
No capacity, build, activation, development cell, freeze, or holdout action is
authorized. Resume by retrying the complete exact-tree review after the
Sol-xhigh backend/quota becomes available.

## Append-only launch-provenance correction — 2026-09-12

The fresh exact-tree review eventually accepted candidate `023dfc3`, but its
independent signed-bundle revalidation found a real launch-gate defect before
any capacity or scientific execution. Go 1.27 emits the toolchain version on
the first `go version -m` line (`binary: go1.27.0`), while the SV1D capacity,
activation, and audit adapters searched for an obsolete `go` metadata row.
Valid pinned binaries were therefore rejected. The finding is reachable but
has no historical activation: no successor capacity arm, activation probe,
development cell, freeze, or holdout ran.

The minimal correction is pushed as `527d55a` and changes all three adapters
to parse the first metadata line. The R2 contract test covers the first-line
form and rejects the obsolete parser. Clean bounded `make test`, `go vet`, the
targeted race matrix, fresh-process determinism/evidence-neutrality checks,
shell syntax, and diff checks pass at that commit. Full details are in
`research/v2-r5-go-1.27-launch-parser-fix-2026-09-12.md`.

The pre-correction pinned tools and provisional signed bundle are not accepted
for launch. Rebuild the tools at `527d55a`, obtain one fresh exact-tree review
and a valid bundle bound to those tools, then run only the outcome-ineligible
binary capacity preflight. No activation, development cell, freeze, or
holdout `619/631/641` is authorized.

## Append-only exact-tree review rejection — 2026-09-13

Fresh reviewer `Gauss` (`01a09810-a506-7bf3-bed8-7d8ca07ef6e4`) reviewed exact
HEAD `054c60a` and returned **REJECT** for the narrow promotion step. The new
Go 1.27 provenance note contained an illustrative tracked system-temporary
binary path,
which the repository path-contract test correctly rejects. This is a
documentation-only gate defect with no economic or historical impact; no
capacity, activation, development, freeze, or holdout ran.

The path is being replaced with `example-binary`, followed by a genuinely
uncached full `make test`. A fresh exact-tree review is required afterward.

## Append-only uncached verification — 2026-09-13

The placeholder correction is committed as `5ce2c7b`. Its fresh uncached full
`make test` passed: all packages, fresh-process multivenue evidence checks,
integrated-long-run contracts, R2 contracts, and archive contracts. The
negative resource-contract diagnostic was expected and the command exited
zero; no OOM event occurred. The next gate is a fresh exact-tree independent
review of `5ce2c7b`.

## Append-only capacity-runner executable-bit correction — 2026-09-13

The first capacity launch reached the lock handoff but exited 126 before any
output because the tracked capacity runner was `0644` and its direct `$0`
re-entry required executable mode. No capacity arm or evidence was produced.
Commit `7d35958` sets the runner to `0755` and adds a contract assertion for
that requirement. Focused R2 checks and a clean full `make test` pass at the
committed tree, including parity and archive contracts. The previous signed
bundle is no longer valid for this tree; obtain a fresh exact-tree review and
rebuild before retrying capacity. No activation, development, freeze, or
holdout ran.
