# V2 current-state reconciliation audit

**Audit cut:** 2026-08-27, after inspection of the working tree and retained
artifacts on branch `autoresearch/ffa-ecology-gen0` at `061493a`.  This is a
read-only reconciliation of the project against the persistent objective in
`/home/vlad/.codex/attachments/67638de2-09ea-4c2f-9032-44accadb5bd6/goal-objective.md`.
No simulator, treatment, or holdout world was launched for this audit; no
parameter, configuration, raw evidence, or historical result was changed.  A
previously licensed P7d seed-439 campaign was already present when the audit
began.  Its existing extraction material is reported below but is not scored
or completed by this document.

This document is intended to be sufficient for a reviewer who has not followed
the recent session.  It reconciles committed documents and immutable result
artifacts rather than copying the older resume note.  `research/v2-design.md`
is the current V2 design ledger; individual preregistrations and their
machine-readable artifacts are the authority for each experiment.  A negative
or inactive result is retained as evidence and is not silently upgraded to a
model failure or repaired by this audit.

## 1. Exact repository state

### Git and filesystem

| item | observed state |
|---|---|
| branch | `autoresearch/ffa-ecology-gen0` |
| HEAD | `061493a769832204a6845248f40a04d96cc4bc6c` — `experiment(v2): pin P7d holdout policy` |
| first-parent context | V2 design `8109530`; ae13f9a autopsy `7404b51`; signed-price implementation `320262e`, hardening `5afdd45`, provenance closure `7644b2`; P3e lifecycle result `e1ae6f3`; P4 protocol `7cade86`; latest simulator-economy addition `6d09268`; latest P7d development/provenance chain through `061493a` |
| tracked dirty files | Four pre-existing user-owned scoreboard extracts: `research/artifacts/scoreboard/f2_baseline_101/{derivatives,exposure,reaction,streamhash}.json` |
| staged files | none |
| untracked files | Raw evidence, generated artifacts, historical runs, local profiles and other workspace material (including P7d holdout 439); these were not staged or deleted. The audit does not treat untracked evidence as committed provenance. |
| disk/process at cut | `/home`: 1.3 T total, 249 G available (81% used); no `multivenue`, `mvanalyze`, or `prunegate` process was running. |

The dirty scoreboard files are the same worktree edits explicitly documented
by earlier P3e/P7d manifests.  They are not simulator source changes, but a
clean freeze build must still avoid relying on a dirty worktree.

### Semantic, analyzer, documentation, and binary revisions

| class | revision(s) and meaning |
|---|---|
| latest simulator-semantic revision | `6d09268c58c1e720d6725190543ced127daf4a44`, `feat(v2): add directional perp mandate`. It changes the P7d directional participant and its simulation wiring. No simulator source path changed after this revision. |
| signed-price semantic lineage | `320262e` merged signed representation/availability; `5afdd45` closed hardening; `7644b2` closed provenance. These are ancestors of HEAD and are complete, not an outstanding gate. |
| latest analyzer implementation revision | `abc42a164679c5aded881be14cb08f63d9fae0b8`, role-scoped borrow replay. `4b2856e` added the independent P7d risk analyzer and `cmd/mvanalyze` wiring. These are analyzer-only after the simulator revision. |
| latest analyzer/script/result state | `34a54c4`, `dae67ae`, `71f1626`, `59b63ec`, and `061493a` record provenance, scores, configs, and policy. `d1555e1` is semantics-neutral script housekeeping plus a historical-note update. |
| historical V1 autopsy | `7404b51`, `research/frozen-autopsy-ae13f9a.md`; complete with explicit limits and not reopened. |
| current design ledger | `research/v2-design.md`; later than the persistent goal and the operative V2 status ledger. |

The distinction matters: P7d worlds were run with an immutable simulator
binary built before analyzer-only commits. Their run metadata records source
revision `8b1d013` and binary SHA
`0bcc40ef78f87f08301555bf203366780569c19ed42e84c024e39de34d2ebece`.  The
binary contains the `6d09268` simulator behavior; the later changes used to
score it are analyzer/provenance changes.  This is valid recorded provenance,
but it is not an exact clean-HEAD build attestation.

Current binaries are:

| binary | SHA-256 | embedded build revision | assessment |
|---|---|---|---|
| `bin/multivenue` | `0bcc40ef78f87f08301555bf203366780569c19ed42e84c024e39de34d2ebece` | `4b2856e...+dirty` | simulator paths are unchanged after the P7d semantic revision, but the artifact is not a clean HEAD rebuild |
| `bin/mvanalyze` | `934993a87a817cf3fcb52eb29e8b1392d0b81e2149b52a4f5706026d57e763ad` | `abc42a1...+dirty` | matches the latest analyzer path; dirty build stamp reflects the shared worktree |
| `bin/prunegate` | `151982bfc845990fde903413420c2eb1da27e6ec3efb3e6827dc01b1c4844535` | `4b2856e...+dirty` | no source-path change after the stamped revision; clean freeze rebuild still required |

The persistent objective file has timestamp `2026-08-24 11:34:56 +0300`, size
9,453 bytes, and SHA-256
`e7d196499f5b35896ec0487158bd09eafae78d733c0411153f18a80a53ea9673`.  It
still says that signed-price is the “next architectural gate,” which is false
at this HEAD.  `research/RESUME-HERE.md` has timestamp `2026-08-24 22:02:04
+0300` and remains an older ae13f9a frozen-audit resume note.  Neither file is
an authoritative description of the current V2 campaign.

This audit did not run `go test`, race tests, or any simulation smoke.  Earlier
result documents record focused/full-suite passes at their own revisions, but
that is not a fresh current-HEAD green attestation; the clean freeze package
must perform it explicitly.

### Completed architectural gates visible from HEAD

* `research/v2-signed-price-audit.md` records signed `int64` representation,
  explicit `ErrNoPrice` availability, contract-specific `PriceDomain`,
  full-range midpoint handling, signed matcher/settlement fixtures, zero
  settlement serialization, positive-world equivalence, determinism, and
  performance.  Its hardening artifact is
  `research/artifacts/v2-signed-price-hardening-gate.json`.
* `research/v2-0-information-boundary.md`, the V2-1 cache/frontier/remote-feed
  documents, and the V2-2 router evidence document establish the compact
  receipt/frontier contract.  They explicitly limit the claim to audited
  emitted decisions; no-op ticks and some direct internal reads are not
  universally logged.
* The historical V1 `research/causal-ablations.md`,
  `research/validation-audit.md`, `research/measurement-manifest.json`, and
  `research/artifacts/validation-summary.json` are provenance for ae13f9a (or
  earlier audit contracts), not a replacement V2 verdict.  In particular,
  V-012 records the exact 2,570,397 duplicate checkpoint observations
  (`maker_state` 2,332,800 plus `conservation_violation` 237,597); the
  persisted evidence was present once, so this was a hash-domain/
  instrumentation defect rather than scientific evidence loss.
* `research/v2-8-profiling.md` records both the pre-signed and post-hardening
  profiles.  The post-hardening 30-minute simulator specimen took 22.25 s
  (80.9 simulated seconds per wall second), peaked at 812,756 KiB RSS, sampled
  9.246 GB of allocations and 21 GC cycles; `PlaceOrder` was 32.55% inclusive
  CPU, checkpoint/logger 17.98%, `encoding/json.Marshal` 15.62%, and detached
  preview 13.58% of sampled allocation.  The retained analyzer replay took
  0.73 s, 64,968 KiB RSS and 410.85 MB sampled allocation, with
  `encoding/json.Unmarshal` at 70.57% inclusive CPU.  No JSON dependency was
  adopted, and this profile is a methodology/performance gate rather than a
  new economic result.

## 2. Reconciliation with the persistent goal objective

The persistent objective remains scientifically appropriate, but its “Current
state” and “Immediate sequence” are stale.  The classifications below refer to
the objective as written, not to whether every eventual scientific question is
positive.

| goal instruction/state claim | status now | reconciliation/evidence |
|---|---|---|
| Keep ae13f9a historical and do not retune it | **DONE / CURRENT** | `research/frozen-autopsy-ae13f9a.md` at `7404b51` is complete; V2 results do not rewrite it. |
| V2-0 evidence prerequisites | **DONE, narrow scope** | Receipt, schedule, decision, frontier-vector, mutation, and logging-neutrality contracts are recorded in `v2-0-information-boundary.md`; no-op decisions and some uninstrumented paths remain explicit limitations. |
| V2-1 participant-local information | **PARTIALLY DONE** | Single cache, remote-feed smoke, heterogeneous three-maker construction and frontier vectors pass. No broad heterogeneous price-discovery claim is licensed. |
| V2-2 executable router / price discovery | **PARTIALLY DONE** | Router information/non-atomic evidence passes; V2-2b supports delayed quote-mediated dispersion reduction and router activation without remote feeds, but trade-mediated convergence and the informed-maker/router conditional channel are unmeasured or falsified in the smoke. |
| V2-3 passive making/inventory | **PARTIALLY DONE** | P0 admission supported, C−B ordering mixed, P1 size activation supported, P2 costly rebalance integrity supported, P3-R1 not exercised. No long-horizon stability claim. |
| V2-4 liability/ecology and L1-P3 | **PARTIALLY DONE / CURRENT LIMITATION** | L0/L1/L1-P/L1-P2 narrow local mechanisms supported; L1-P3 untouched replication is MIXED (107/109 repeat, 113 reverses). No roster demotion/replacement result. |
| “signed-price is the next gate” | **SUPERSEDED / DONE** | Signed-price merged at `320262e`, hardened at `5afdd45`, provenance closed at `7644b2`; audit and positive-world equivalence are complete. |
| V2-8 profiling before signed-price | **SUPERSEDED** | The original profile and post-merge/hardening reprofiles are both recorded in `v2-8-profiling.md`; no third-party JSON library was adopted. |
| V2-5 funding/carry | **PARTIALLY DONE** | P3/P3e lifecycle mechanics pass narrowly; P4 is FALSIFIED at the registered basis endpoint, P4b is FALSIFIED AT EXECUTION, and P5 dated carry is NOT EXERCISED. No holdout promotion. |
| V2-6 staged options | **PARTIALLY DONE** | Original P6 is incomplete; P6-R1 viability and all O0–O4 stage activations replicate on holdouts. No causal option-surface/emergence claim. |
| V2-7 distress | **PARTIALLY DONE / CURRENT** | P7a–P7c participant distress is NOT EXERCISED; P7d development supports directional activation/risk replay and authorized holdouts. Seed-439 holdout worlds are now consumed but not fully packaged/scored; 443/449 remain untouched. |
| Replace activity generators incrementally | **OUTSTANDING** | L0/L1 prove local liability motivation, but no registered broad replacement/demotion result exists. Existing broad `noise_flow` population remains. |
| Explicit interval and phase timing | **PARTIALLY DONE** | Phase controls and L1-P/L1-P2/L1-P3 evidence exist. The L1-P3 robustness result is MIXED, not a license to tune timing. |
| Preserve evidence boundary and mutate auditors | **PARTIALLY DONE** | V2-0/1/2 and several P2–P7 contracts are independently mutated; known gaps remain for no-op decisions, GTC cancellation, broader cross-margin and some direct strategy state. |
| Declare immutable V2 freeze and regenerate controls | **OUTSTANDING** | There is no new immutable V2 freeze or regenerated final V2 control set at current HEAD. |
| Final V2 autopsy and untouched realism validation | **OUTSTANDING** | `frozen-autopsy-ae13f9a.md` is V1 history; no `frozen-autopsy-v2`/final holdout scoreboard exists. |

The objective’s overall philosophy—mechanism first, independent evidence,
paired interventions, explicit negative results, and holdout validation—remains
**CURRENT**.  The signed-price ordering in its immediate sequence is the only
major architectural claim that is simply stale; P4–P7 and the options work
have moved the actual frontier materially further.

## 3. Full V2 mechanism ledger

“OOS” below means a registered out-of-sample replication, not merely a second
development seed. “Holdout reserved/consumed” is experiment-specific: a seed
may be a holdout for one protocol and a development seed for another without
automatically contaminating either contract. A freeze license is always the
narrow claim stated; code existence never upgrades a mechanism to a market
claim.

### V2-0 through V2-3

| mechanism / scientific question | status; strongest permitted claim; exact classification | development / holdout accounting | result/preregistration and machine artifact | relevant commits; eventual-freeze license |
|---|---|---|---|---|
| **Signed-price architecture:** can signed numeric values and availability be separated without changing positive worlds? | Complete. Signed `int64`, explicit unavailable state, contract-level domains, signed-safe arithmetic and zero settlement wire are audited. **SUPPORTED** (architectural gate; not an economic-price claim). | Positive-world equivalence uses parent `99ce69c`/candidate `0f6a3c6`; no holdout concept. Commodity negative-price ecology was not instantiated. | `research/v2-signed-price-audit.md`; `research/artifacts/v2-signed-price-hardening-gate.json`. | `320262e`, `5afdd45`, `7644b2`. Licensed for representation/availability, not for unsupported Black–76 negative forwards or uninstantiated commodity economics. |
| **V2-0 evidence prerequisites:** can audited participant observations be reconstructed from delivered receipts? | Complete for declared delayed links and emitted order/quote decisions; future/drop/delay/duplicate/reorder mutations caught; no-op strategy ticks are not universal evidence. **SUPPORTED** (narrow information contract). | Construction/baseline smoke seeds 101 and fresh-process GOMAXPROCS 1/4; no separate holdout policy. | `research/v2-0-information-boundary.md`; compact sidecar manifests and `market-data-evidence-v2.json` under retained run artifacts. | `59d4d80` and preceding V2-0 instrumentation commits. Freeze license only for claims whose local decision frontier is recorded. |
| **V2-1 local cache/frontier/remote feed:** can makers use delayed copied public views rather than a hidden consensus value? | Single cache, vector frontier, one remote maker, and three-maker heterogeneous construction all pass. **SUPPORTED (screening)** for participant-local information construction; no economic price-discovery claim. | Smokes use seed 101 (20 s/2 min); three-maker roster is a construction test, not a holdout. | `research/v2-1-single-feed-cache.md`, `v2-1-frontier-vectors.md`, `v2-1-remote-feed-smoke.md`, `v2-1d-roster-preregistration.md`; sidecar artifacts retained with each smoke. | `c6a3f14`, `e809b61`, `6152c9b`. Licensed for information-boundary tests, not broad heterogeneous-maker realism. |
| **V2-2 executable router:** can a router use delayed feeds and non-atomic legs with independently auditable information? | Router frontiers, executable bid/ask selection, fees, FOK leg identity, residual-leg accounting and feed-only isolation pass. The generic five-minute population initially had no route; a targeted metaorder probe activated it. **SUPPORTED (screening)** for construction/activation; no convergence claim. | Construction/probe seed 101; no holdout policy. | `research/v2-2-router-evidence-preregistration.md`; router artifacts and V3 frontier vectors under `research/artifacts/v2-2b/`. | `35c2927` result lineage plus V-035–V-038 fixes in `validation-audit.md`. Licensed for auditable non-atomic router participation only. |
| **V2-2b informed makers × router smoke:** which channel changes cross-venue dispersion? | Delayed remote makers reduce fresh midpoint dispersion by 3.244 bps (101) and 2.488 bps (103); router activates without remote feeds but does not change the registered snapshot edge endpoint. **SUPPORTED (screening)** for quote-mediated reduction; router residual-edge effect **FALSIFIED (screening)**; conditional informed+router channel **NOT IDENTIFIED**; quote/trade decomposition **MIXED / incomplete**. | Development cells I0/I1 × R0/R1, seeds 101/103; no untouched holdouts. | `research/v2-2b-price-discovery-smoke-results.md`; `research/artifacts/v2-2b/summary.json` (SHA `da73de...51b1`). | `35c2927`, analysis `51f46e0`, simulator/input `69b2537`. Freeze license: narrow quote-mediated screening only; trade-mediated price discovery remains open. |
| **V2-3 P0 passive admission and actor ordering:** does post-only admission separate from cancel-before-replace? | B−A exchange post-only admission is supported mechanically; C−B actor ordering is mixed (rejects and trades rise, volume falls, two-sided sign disagrees). No stability inference. **SUPPORTED (mechanical screening)** for B−A; **MIXED** for C−B. | A/B/C × seeds 101/103, 5 min; no holdout. | `research/v2-3-passive-making-p0-results.md`, `v2-3-passive-making-p0-causal-refinement.md`; `research/artifacts/v2-3-p0-r1/p0-summary.json`. | `198a3b2` and P0 refinement lineage. Freeze license: venue passive contract and explicit actor-ordering limitation only. |
| **V2-3 P1 inventory size:** does inventory change displayed size asymmetrically? | Every nonzero-risk treatment decision adjusts size in the registered direction; books remain viable in the short screen. **SUPPORTED (screening)** for local policy activation, not stabilization. | A/B × 101/103, 5 min; no holdout. | `research/v2-3-inventory-size-p1-results.md`; `research/artifacts/v2-3-p1/p1-summary.json`. | `953f80f`, checker correction `90829d2`. Freeze license: local size policy only; no price-stability or ecology claim. |
| **V2-3 P2 explicit rebalance:** can a costly, capped, local-information IOC transfer maker risk to external counterparties? | Enabled CDF makers submit ordinary capped IOC orders, pay fees, and all fills reduce local gap; controls submit none. **SUPPORTED (screening)** for mechanism integrity. | A/B × 101/103, 5 min; no holdout. Attempts 0/1 are historical invalid/unscored. | `research/v2-3-inventory-rebalance-p2-results.md`; `research/artifacts/v2-3-p2/p2-summary.json`. | `3fa3e2c`, analyzer/schema corrections in the P2 lineage. Freeze license: existence of explicit costly rebalance; no aggregate stability claim. |
| **V2-3 P3-R1 replenishment:** does a confirmed partial perpetual fill trigger replenishment? | Evidence and viability are valid, but `refresh_due=0` and no qualifying partial fill occurs. **NOT EXERCISED**, not falsified. | A/B × 101/103, 5 min; no holdout. | `research/v2-3-perp-quote-replenishment-p3-r1-results.md`; `research/artifacts/v2-3-p3-r1/p3-r1-summary.json`. | `86f5e7e`, source `e79eb...`. No freeze license beyond a valid inactive-test record. |

### V2-4 ecology and timing

| mechanism / scientific question | status; strongest permitted claim; exact classification | development / holdout accounting | result/preregistration and machine artifact | relevant commits; eventual-freeze license |
|---|---|---|---|---|
| **L0 liability hedger:** can finite stateful delivery exposure create a local hedge-gap action? | Enabled actor is locally executable and finite; control policy is matched. **SUPPORTED (screening)** for narrow local hedge-gap activation; not replacement or stability. | A/B × 101/103, 5 min; no holdout. | `research/v2-4-liability-hedger-l0-results.md`; `research/artifacts/v2-4-l0/l0-summary.json`. | `91503bc`. Licensed only for a follow-up motive screen. |
| **L1 matched motive control:** does delivery liability change side selection versus random side? | Delivery-liability fills reduce the independent gap; random-side control has nonreducing fills. **SUPPORTED (screening)** for the local motive distinction; broad legacy `noise_flow` retained. | A/B × 101/103, 30 min; no holdout. | `research/v2-4-l1-cdf-motive-control-results.md`; `research/artifacts/v2-4-l1/l1-summary.json`. | `4dd3423` and analyzer fixes documented in the result. No roster replacement license. |
| **L1-P phase:** does a half-period liability phase change the local result? | Policy correctness and viability survive; phase changes descriptive gaps/fills. **SUPPORTED (screening)** for the narrow local-motive/phase contract; descriptive effect not a market claim. | P0/P1 × 101/103, 30 min; no holdout. | `research/v2-4-l1p-phase-results.md`; `research/artifacts/v2-4-l1p/l1p-summary.json`. | `d60d82f`; phase capability `af7a284`/`160508a`. Licensed only for explicit phase controls. |
| **L1-P2 liability/noise relative phase:** is broad noise cadence a counterpart clock? | Both development seeds satisfy aligned>dealigned and positive interaction. **SUPPORTED (screening)** for one local-gap relative-phase effect; not a unique LCM or ecology timing claim. | A/B/C/D × 101/103, 30 min; holdout policy required. | `research/v2-4-l1p2-noise-phase-results.md`; `research/artifacts/v2-4-l1p2/l1p2-summary.json`. | `76de7cd`; no roster/demotion license until holdout. |
| **L1-P3 untouched timing replication:** does the relative-phase effect replicate? | Seeds 107/109 repeat the direction; 113 reverses it. **MIXED** under the registered all-three rule. The retained heterogeneity diagnostic is exploratory only. | A/B/C/D × 107/109/113; holdouts consumed for this protocol; no new seed selected. | `research/v2-4-l1p3-holdout-results.md`, `v2-4-l1p3-heterogeneity-diagnostic.md`; `research/artifacts/v2-4-l1p3/l1p3-summary.json`. | `9fd9276`. No timing tuning, L2 demotion, or ecology-wide freeze license. |
| **Later V2-4 replacement/demotion:** can broad activity generators be replaced by liabilities/value/execution objectives? | No later preregistered replacement/demotion experiment is completed. **NOT IDENTIFIED / OUTSTANDING** as a broad ecology claim. | Existing L0/L1 development only; no registered replacement holdout. | `v2-design.md` V2-4 section and L0/L1 records. | No result commit. Not licensed for V2 freeze as a completed ecology. |

### V2-5 funding, carry, and lifecycle

| mechanism / scientific question | status; strongest permitted claim; exact classification | development / holdout accounting | result/preregistration and machine artifact | relevant commits; eventual-freeze license |
|---|---|---|---|---|
| **P0 funding-aware activation attempt 0:** can a carry desk use delivered funding and local books? | The original world is **INVALIDATED BEFORE INTERPRETATION**: the decision evidence attached unrelated terminal receipt frontiers, so no activation or funding claim is licensed. | Seed 101, 5 min; no holdout. | `research/v2-5-funding-carry-p0-attempt0-invalidation.md`, `research/v2-5-funding-carry-p0-preregistration.md`; retained `research/artifacts/v2-5-p0/activation-101/`. | Historical invalidation only; never promoted to a result. |
| **P0-R1 replacement activation:** can the corrected desk consume delivered funding/local books and submit ordinary non-atomic legs? | Receipt/frontier, exact carry, gateway, order/fill and terminal exposure checks pass; **SUPPORTED (screening)** for activation/integrity only, not funding anchoring or basis. | Seed 101, 5 min; no holdout. | `research/v2-5-funding-carry-p0-r1-results.md`; `research/v2-5-funding-carry-p0-preregistration.md`; `research/artifacts/v2-5-p0/activation-r1-101/p0-verdict.json`. | Replacement is append-only telemetry; no economic/freeze claim. |
| **P2 signal readiness:** is a public perpetual mark/funding signal present in retained exposure evidence? | All required venues/rates/mark pairs are present in A/B seeds, including present zero rates. **SUPPORTED (descriptive readiness)**; it does not identify a funding response. | P2a A/B × 101/103, 5 min; no holdout. | `research/v2-5-p2-signal-readiness-preregistration.md`; `research/artifacts/v2-5-p2/p2-signal-readiness-verdict.json`. | `8c29024` analyzer-only. Licensed only as a public-input readiness prerequisite. |
| **P2a physical exposure:** can bounded physical exposure produce ordinary local perpetual hedges? | State, delayed feed, ordinary orders/fills, gap reduction, conservation and positions replay. **SUPPORTED (screening)**, narrow activation only. | A/B × 101/103, 5 min; no holdout. | `research/v2-5-p2-perp-exposure-results.md`; `research/artifacts/v2-5-p2/p2a-verdict.json` (with the separate signal-readiness record). | `1eed365` lineage. Licensed as a fixed independent flow source, not funding/basis. |
| **P1a fee-aware feasibility:** can current whole-bps economics cross the declared hurdle? | All venues evaluate funding/costs but no action reaches the 24-bps hurdle. **NOT EXERCISED**. | One development seed 107, 30 min; paired 101/103 reserved for the later design but not consumed by P1a. | `research/v2-5-funding-carry-p1a-results.md`; `research/artifacts/v2-5-p1a/fee-aware-107/p1a-verdict.json`. | P1a result lineage; no P1b license. |
| **P3a/P3b term entry and realized funding:** can a finite term form and survive a funding instant? | P3a one short activation world and P3b one 9-hour realization world pass local delayed entry/funding-transfer integrity. **SUPPORTED (development screening)**, no basis or profitability claim. | P3a seed 107, 5 min; P3b seed 107, 9 h; no holdout. | `research/v2-5-p3a-term-carry-results.md`, `research/v2-5-p3a-term-carry-preregistration.md`, `research/v2-5-p3b-term-realization-results.md`, `research/v2-5-p3b-term-realization-preregistration.md`; corresponding `p3a-verdict.json` and `term-realization-107-verdict.json`. | `55cd06a` analysis lineage and prior P3 commits. Licensed only for local entry/funding-transfer mechanics. |
| **P3c finite term completion:** can the fixed minimum-size unwind close? | Two legitimate terms reach end, but displayed asks are below the actor's legal minimum; no unwind, residual remains, and one post-term funding transfer is observed. **FALSIFIED (development lifecycle screen)** for that close policy. | Seed 107, 98 h; no holdout. | `research/v2-5-p3c-term-completion-results.md`; `term-completion-107-verdict.json`. | Historical P3c result; do not use as P3e causal control. |
| **P3d exit-liquidity attempt:** does setting actor exit floor to zero solve the problem? | Exchange minimum was misdescribed; B submits sub-minimum orders that the venue rejects. **INVALID / NOT SCORED**; raw attempt retained. | Seed 107, 98 h; no holdout. | `research/v2-5-p3d-exit-liquidity-results.md`; retained historical P3d directories. | `2cb51bf`, `21d17ae` corrections. No economic inference. |
| **P3e P0 passive activation:** can a bounded passive child be admitted when aggressive depth is below legal minimum? | Two ordinary post-only children are admitted through gateway/venue evidence; activation and integrity pass. **SUPPORTED (screening)** for the narrow activation contract only. | B/107, 98 h; no holdout. | `research/v2-5-p3e-passive-exit-p0-results.md`; `research/artifacts/v2-5-p3e/p0-B-107/p0-verdict.json` and the `p0-B-107/` evidence directory. | `bdba08d`; no closure or market claim. |
| **P3e lifecycle A/B:** does passive exit close finite terms by deadline versus defer-only? | B closes both terms in both seeds; A retains 40m residual per term. **SUPPORTED (screening)** narrowly for finite-term execution/closure. | A/B × 107/109, 98 h; development comparison, no holdout. | `research/v2-5-p3e-lifecycle-results.md`; `research/artifacts/v2-5-p3e/lifecycle-verdict.json`. | Result originally `e1ae6f3`; later note/script housekeeping `d1555e1`. No funding/basis/profitability claim. |
| **P4 six-link funding/carry:** does changed funding produce target inventory, real orders, and basis response? | Links 1–5 pass in both seeds: funding 1→3 bps, expected 12→36, carry −16.4785→+7.5215 bps, target/real fills. Exact paired basis remains zero in every qualifying venue. **FALSIFIED** at registered market-basis endpoint. | Development A/B × 107/109; holdouts 127/131/137 reserved, **not consumed**. | `research/v2-5-p4-funding-carry-results.md`, `v2-5-p4-funding-carry-causal-preregistration.md`; `research/artifacts/v2-5-p4/p4-verdict.json`. | `c3bda00`, source `2d36b90`, analyzer `b6b58...`. No holdout promotion; no general funding-irrelevance claim. |
| **P4b independent perp flow:** does a fixed independent flow source make funding's basis effect identifiable? | Funding and first four links activate; seed 401 fails matched ordinary execution, seed 409 executes but exact basis effect is zero. **FALSIFIED AT EXECUTION** (development screening). | A/B × 401/409; holdouts 419/421/431 reserved for P4b, **not consumed for P4b** (431 is later P7d development). | `research/v2-5-p4b-independent-perp-flow-results.md`; `research/v2-5-p4b-independent-perp-flow-preregistration.md`; `research/v2-5-p4b-independent-perp-flow-numeric-addendum.md`; `research/artifacts/v2-5-p4b/p4b-development-score.json`. | `248d525`, source `5fdb0c`, analyzer `f1d93...`. No holdout license. |
| **P5 dated carry/convergence:** can exact-cost dated terms activate and converge? | 126,888 candidate evaluations per seed but zero eligible terms, target changes, submissions, or measurable basis. **NOT EXERCISED**. Dated convergence/carry is not causally tested. | A/B × 117/119, 26 h; holdouts 139/149/151 reserved, **not consumed**. | `research/v2-5-p5-dated-carry-results.md`; `research/v2-5-p5-dated-carry-causal-preregistration.md`; `research/v2-5-p5-dated-carry-numeric-addendum.md`; `research/artifacts/v2-5-p5/development-verdict.json`. | `18bdd26`, source `9a9c590`. No parameter rescue or holdout promotion. |

### V2-6 options, V2-7 distress, and V2-8 performance

| mechanism / scientific question | status; strongest permitted claim; exact classification | development / holdout accounting | result/preregistration and machine artifact | relevant commits; eventual-freeze license |
|---|---|---|---|---|
| **Original P6 O0–O4:** can the staged option ecology run under the initial cross-asset mark contract? | O0/O1 activation passes; O2 activation and hedge flow pass but directional transmission sign was not preregistered; O3 only seed 213 valid; O4 neither valid. **DEVELOPMENT INCOMPLETE**, with O2 directional component **NOT IDENTIFIED** and O3/O4 paired stages **NOT EXERCISED**. | Development seeds 211/213; holdouts 223/227/229 were not authorized/consumed for original P6. | `research/v2-6-p6-options-results.md`, `v2-6-p6-options-causal-preregistration.md`; `research/artifacts/v2-6-p6/development-summary.json`. | `39f37d5` result lineage. No option causal or freeze license. |
| **P6-R1 viability repair:** does explicit positive CDF collateral marking plus finite borrow make all stages executable without exposing an oracle? | All O0–O4, CDF borrow, O2 liability/hedge, O3 SABR and O4 VV activities pass independent evidence. **SUPPORTED (screening)** for viability/stage activation only. | Development O0–O4 × 211/213; holdouts 223/227/229 reserved and then consumed. | `research/v2-6-p6r1-viability-results.md`; `research/v2-6-p6r1-cross-asset-mark-viability-preregistration.md`; `research/artifacts/v2-6-p6r1/development-summary.json`. | `724a4fa`, source `bf4927b`, analyzer `a17e40...`. No surface-emergence license. |
| **P6-R1 untouched stage replication:** do viability and stage activations survive untouched seeds? | All 15 cells pass receipts, accounting, CDF borrow, market-price option activity, O2 hedge, O3 SABR and O4 VV activation. **SUPPORTED (screening)** out of sample for viability/stage activation only. | Holdout O0–O4 × 223/227/229; **consumed** and valid. | `research/v2-6-p6r1-holdout-results.md`; `research/artifacts/v2-6-p6r1/holdout-summary.json`. | `7b9e925`, closure `07ef1a7`. No causal smile/skew/hedge/emergence claim. |
| **Option structure before/after priors:** does smile/skew emerge without SABR/VV beliefs? | O0/O1/O2 show descriptive non-flat surfaces, but no registered causal corridor; O3 includes explicit SABR and O4 explicit VV, so their structure is inherited. IV/parity and hedge evidence are independently inferred, but O2 transmission sign is **NOT IDENTIFIED**. | P6-R1 dev and holdouts support stage viability only; no causal surface holdout exists. | Surface tables in `v2-6-p6-options-results.md` and R1 reports; no causal machine verdict. | No qualifying causal result commit. Not licensed as an emergent option claim. |
| **P7a distress:** can fixed physical-liability participants exercise margin/forced close/deficit? | Fixed-liability activation passes, but participant-specific distress, forced close, deficit, insurance and bankruptcy never trigger. **NOT EXERCISED** for distress. | Development C/H/L × 307/311; holdouts 313/317/331 reserved, not consumed. | `research/v2-7-p7a-results.md`; `research/v2-7-p7-distress-causal-preregistration.md`, `research/v2-7-p7-numeric-addendum.md`; `research/artifacts/v2-7-p7a/p7a-development-score.json`. | `14d85e8`, unit erratum `ea3ee32`. No risk-path validation beyond no-debt arithmetic. |
| **P7b corrected capital:** does the unit correction make fixed-liability distress reachable? | Activation passes; participant risk and deficit/insurance/bankruptcy remain zero. **NOT EXERCISED**. | Development C/H/L × 337/341; holdouts 347/349/353 reserved, not consumed. | `research/v2-7-p7b-results.md`; `research/v2-7-p7b-distress-causal-preregistration.md`, `research/v2-7-p7b-numeric-addendum.md`; `research/artifacts/v2-7-p7b/p7b-development-score.json`. | `fb1e627`, provenance correction `dea1177`. |
| **P7c longer horizon:** does 48 h fixed-liability exposure exercise risk? | Activation passes; participant-specific breaches/forced closes/deficit/insurance/bankruptcy remain zero. **NOT EXERCISED**. | Development C/T × 367/371; holdouts 373/379/383 reserved, not consumed. | `research/v2-7-p7c-results.md`; `research/v2-7-p7c-distress-causal-preregistration.md`, `research/v2-7-p7c-numeric-addendum.md`; `research/artifacts/v2-7-p7c/p7c-development-score.json`. | `e5e8e74`; no distress holdout license. |
| **P7d directional distress development:** can finite-capital unhedged directional participants reach risk events? | Long and short targets activate through ordinary IOC fills; participant-specific breach/replay is observed; long has deficits/insurance, short has forced close without deficit. **SUPPORTED (screening)** for directional activation and risk replay. Deficit/insurance is observed but separately audited; bankruptcy is not claimed. | Development C/L/S × 431/433; holdouts 439/443/449 reserved before outcomes. | `research/v2-7-p7d-results.md`; `research/v2-7-p7d-directional-distress-causal-preregistration.md`, `research/v2-7-p7d-directional-distress-numeric-addendum.md`; `research/artifacts/v2-7-p7d/p7d-development-score.json`. | Simulator `6d09268`/run binary source `8b1d013`; analyzer `abc42a1`; result `59b63ec`, policy `061493a`. Licensed to execute the registered holdout policy, not to claim full-ecology distress realism. |
| **P7d holdout policy:** does the directional risk path replicate? | Seed 439 C/L/S worlds are physically present. C/L have extraction status 0; S has complete required metric files and analysis metadata but no `.extract.status` sentinel observed. No holdout verdict has been generated. **NOT SCORED / INCOMPLETE**, not a risk null. | Holdout C/L/S × 439: **consumed**; 443/449 remain untouched. Runtime evidence: C 14,242,855 / `48dab101...423c10`; L 14,269,247 / `af747934...7b38e1`; S 14,381,958 / `333bfd94...4d58f`. | Configs pinned in `research/configs/v2-7-p7d/{C,L,S}-439.json`; existing raw/evidence under `research/artifacts/v2-7-p7d/holdout/`; no score artifact. | Holdout configs/policy `061493a`; binary/analyzer stamps as above. No freeze license until the registered fail-closed holdout contract is completed or explicitly declared incomplete. |
| **V2-8 performance/timing contract:** where is cost, and has an optimization changed semantics? | Profiling methodology passes; no unsafe dependency adopted. Post-hardening profile still shows matching/order path and JSON logging/analyzer decode as material; no broad optimization or C++ rewrite. **SUPPORTED** as a performance-gate/methodology result. | Baseline/profile seed 101 and retained merged workloads; no holdout concept. | `research/v2-8-profiling.md`, `research/v2-performance-methodology.md`; `research/artifacts/v2-8-signed-hardening-reprofile.json` and pprof trees. | `5afdd45`/profile lineage. Licensed for performance accounting only; current-head clean reprofile remains freeze preparation. |

### What is inherited versus actually endogenous

The V2-2b dispersion change is the strongest market-level screening signal, but
it is attributable to delayed remote maker quotation in the tested cells; the
router did not activate when informed makers were on. P6-R1 surfaces are
descriptive and O3/O4 explicitly encode SABR/Vanna–Volga priors. L1-P2 is a
local timing effect that fails the all-three untouched-seed replication. P4's
changed funding/carry and real fills are genuine participant responses, but the
registered basis endpoint is exactly unchanged. These distinctions prevent
the ledger from turning activated code or an attractive surface into an
emergence claim.

## 4. Mandatory reconstruction of V2-5 and V2-6

### V2-5 funding/carry

1. **Did the final six-link experiment run?** Yes. P4 ran four complete 98-hour
   full-evidence cells (A/B × 107/109) from source `2d36b90`; P4b ran its
   conditional independent-flow screen (A/B × 401/409). P5 ran its dated-carry
   development cells (A/B × 117/119). P3e was the preceding lifecycle gate.
2. **Exact verdict:** P4 is **FALSIFIED** at the registered market-basis
   endpoint; P4b is **FALSIFIED AT EXECUTION** in aggregate development
   scoring; P5 is **NOT EXERCISED**. These are not generic claims that funding
   is irrelevant.
3. **Six-link chain:** P4 independently verifies delivered funding, expected
   funding, exact costed carry, target inventory, and ordinary non-atomic
   spot/perpetual fills in both seeds. Funding changes 1→3 bps per interval,
   expected funding 12→36 bps per term, and exact net carry about
   −16.4785→+7.5215 bps. The treatment changes to long spot/short perp and
   reaches a matched 0.1-ABC exposure. Link 6 fails: every qualifying venue
   has `pre = post = 20000/9999` bps, so the paired basis response is exactly
   zero. P4b independently activates the first four links, but one seed fails
   matched ordinary execution and the other has exact-zero basis. Thus the
   missing causal step is not participant activation; it is a measurable
   funding-driven market response at the registered scale.
4. **Untouched replication:** none. P4 holdouts 127/131/137 remain unconsumed;
   P4b holdouts 419/421/431 remain unconsumed for P4b; no holdout was licensed
   after the negative development outcomes. Seed 431 is used as P7d
   development, which does not contaminate the P4b-specific untouched policy.
5. **Dated convergence:** P5 did not produce a single exact-cost eligible term
   in either development seed. It therefore measured no target, execution, or
   pre-settlement basis window. Dated-future instruments and lifecycle paths
   exist, but the registered dated-carry participant/mechanism did not
   activate; convergence/carry has no positive or negative causal result.
6. **What remains:** an explicit disposition is required before a freeze can
   claim funding/carry economics. The existing negative results may remain as
   limitations; a new treatment may not be selected merely to cross a hurdle or
   rescue the basis endpoint. Dated carry remains inactive rather than
   falsified.

### V2-6 options

1. **Implemented stages:** O0–O4 exist in the P6 staged configs. The original
   P6 O0/O1/O2 development cells are valid; O3 is only valid at seed 213 and
   O4 is invalid/incomplete under the original mark contract. P6-R1 repairs
   viability with an explicit positive CDF collateral mark and finite borrow
   cap for accounting, without exposing a hidden fair value. All O0–O4 then
   pass development and holdout stage activation.
2. **Mechanical versus causal:** Original P6 and P6-R1 are primarily
   mechanical/activation screens. O2 demonstrates liability demand, dealer
   option inventory, and hedge-tagged underlying flow, but its transmission
   sign was never preregistered. O3/O4 demonstrate active SABR/VV participant
   paths; they are not causal tests of surface emergence. No paired causal
   O2-versus-O3/O4 surface experiment is complete.
3. **Holdouts:** P6-R1 uses untouched seeds 223/227/229 and all 15 O0–O4 cells
   pass the fixed viability/stage contract. There is no untouched causal
   surface holdout.
4. **Pre-prior structure:** O0/O1/O2 all exhibit descriptive non-flat surface
   values from market-price IV inversion, but no registered effect-size or
   causal corridor identifies why. O3's contrast contains an explicit SABR
   prior; O4 contains explicit Vanna–Volga risk-transfer beliefs. Those
   structures are inherited by design, not emergent evidence.
5. **Independent audits:** IV is inferred from market quotes/trades, not agent
   model IV; parity, settlement, post-expiry fills, dealer exposure and
   evidence digests are independently audited in valid cells. This supports
   measurement integrity, not a surface claim. Black–76 remains explicitly
   positive-forward-domain only.
6. **Freeze license:** options are licensed only for mechanical/stage viability
   (including P6-R1 OOS activation). Smile/skew/term-structure emergence,
   hedge-feedback direction, and VV causal attribution are unresolved.

## 5. V2-7 current state: P7a through P7d

The sequence is not rewritten. P7a's post-run unit erratum identified a raw-USD
precision error: the stated 4.17×/8.33× leverage was actually about 0.42×/0.83×.
P7b corrected capital units; P7c extended the horizon. All three retained
fixed-liability activation but never exercised participant-specific risk,
forced close, deficit, insurance, or bankruptcy. Their generic venue
liquidations are diagnostic only.

P7d changed the *economic mechanism*, not just a threshold. It introduced a
separate finite-capital, unhedged directional desk with a declared +2e9 or
−2e9 raw-ABC perpetual target, 5.5e9 raw-USD own margin, capped ordinary
`auto_perp` borrow, 500e6 maximum IOC child, delayed local book feed, and no
synthetic close/reset. The development result is **SUPPORTED (screening)** for
finite-capital directional activation and independent participant maintenance
risk replay:

| development orientation | decisions | accepted / fills | filled quantity | expected→observed breaches | participant liquidations | deficit / insurance observation |
|---|---:|---:|---:|---:|---:|---:|
| C control, 431/433 | 21,600 each | 0 / 0 | 0 | 0→0 | 0 | 0 / 0 |
| L long, 431/433 | 21,600 each | 306/40; 266/40 | 6,000,000,000 each | 16→16; 14→14 | 10; 12 | 3 / 1, insurance deficit 3,970,335,945 and 823,797,845 |
| S short, 431/433 | 21,600 each | 46/47; 22/51 | 6,000,000,000 each | 1→1 each | 1 each | 0 / 0 |

The P7d development score explicitly separates long/short activation and risk
from deficit/insurance/bankruptcy. No participant bankruptcy claim is present.
The development score authorized the pre-pinned holdout policy because both
orientations had valid activation and clean participant-specific risk events.

### Existing seed-439 holdout material (not a verdict)

The previous licensed campaign has consumed C/L/S seed 439. It is not an
untouched seed anymore, and no holdout score has been generated:

| cell | config SHA-256 | decisions / enabled | accepted / fills / quantity | expected→observed breaches | participant liquidations | deficits / insurance | runtime evidence |
|---|---|---:|---:|---:|---:|---:|---|
| C-439 | `0ff72d0aae2db4c1bfb22d59742df8be3aa58af01a6268f1a6bb418fdd14b21b` | 21,600 / 0 | 0 / 0 / 0 | 0→0 | 0 | 0 / 0 | 14,242,855; `48dab1016d114637d78781df792c19fa5c825af31c32cf3651046e2b7d423c10` |
| L-439 | `1089fe48fbf3745d2b39ba939c2531f7ba7d3a2da15ff1a97b271f0e7d66ec5a` | 21,600 / 21,600 | 246 / 33 / 6,000,000,000 | 12→12 | 11 | 0 / 0 | 14,269,247; `af7479342837a70fbdb2c1bd0eba57a77504e109d72c9256f58b08d4e37b38e1` |
| S-439 | `9a2b6dbdd9b1d65525276367d243f221a3ffb18db2f73dca8fe07e016a564c9c` | 21,600 / 21,600 | 26 / 39 / 6,000,000,000 | 0→0 | 0 | 0 / 0 | 14,381,958; `333bfd94539128b6cdee0b1848c4f56e59f96d492b40c69a23c523ba8404d58f` |

All three directories have valid run metadata, final `greeks.json` and
`latency.json`, all required P7d metric JSON files, and complete
`analysis-metadata.json` with analysis revision
`061493a769832204a6845248f40a04d96cc4bc6c` and analyzer SHA
`934993a87a817cf3fcb52eb29e8b1392d0b81e2149b52a4f5706026d57e763ad`.
C-439 and L-439 have root extraction status files with successful exit.  S-439
has a successful extraction log and the same required outputs, but no
`S-439.extract.status` was present at the audit cut.  That missing completion
sentinel means the holdout package is **not yet fail-closed complete**, even
though the visible metrics are valid-looking.  The raw evidence is retained;
this audit did not create the missing sentinel or score the cell.  Holdout
seeds 443 and 449 remain untouched.

## 6. Holdout accounting

The table lists every seed set declared by the V2 protocols, including
single-cell development/feasibility seeds, development seeds that later serve
as a holdout in a different experiment, and historical invalid attempts.  A
“consumed” entry is always relative to the named protocol.

| protocol / use | role | declared seeds | consumed under that protocol? | reconciliation |
|---|---|---|---|---|
| ae13f9a frozen baseline and V1 causal controls | development/control | 101, 102, 103 | yes | Baseline worlds and old ablations are historical control evidence; not V2 holdouts. |
| V2-1 construction / V2-2b smoke | construction/development | 101, 103 (some one-maker smokes use 101) | yes | No holdout policy. |
| V2-3 P0/P1/P2/P3-R1 | development paired | 101, 103 | yes | P0 attempt-0 and P2 attempts 0/1 are historical invalid/unscored; final records use same seeds under their own contracts. |
| V2-4 L0/L1/L1-P/L1-P2 | development paired | 101, 103 | yes | These are development seeds for the local motive/phase screens. |
| V2-4 L1-P3 | untouched replication | 107, 109, 113 | yes | Valid registered holdout; 107/109 repeat, 113 reverses; final classification MIXED. The retained heterogeneity file is exploratory only and uses no new seed. |
| V2-5 P1a feasibility | single development | 107 | yes | Explicitly not an untouched holdout for V2. |
| V2-5 P0 / P0-R1 funding activation | development/history | 101 | yes | Attempt 0 is invalidated before interpretation; R1 is a valid activation/integrity replacement. No holdout was declared. |
| V2-5 P2a physical exposure | development paired | 101, 103 | yes | Narrow activation only. |
| V2-5 P3a/P3b/P3c/P3d | development/history | 107 | yes | P3d is INVALID/NOT SCORED; P3c is a valid negative lifecycle result. |
| V2-5 P3e P0 and lifecycle | development | 107; 107, 109 | yes | P0 and lifecycle are complete development evidence; no holdout seed was declared. |
| V2-5 P4 | development | 107, 109 | yes | Holdouts 127, 131, 137 were reserved but **not consumed** after the falsified endpoint. |
| V2-5 P4b | development | 401, 409 | yes | Holdouts 419, 421, 431 remain unconsumed **for P4b**. Seed 431 is separately used as P7d development; this is cross-experiment reuse, not P4b holdout evidence. |
| V2-5 P5 dated carry | development | 117, 119 | yes | Holdouts 139, 149, 151 remain untouched; no eligible development terms. |
| V2-6 original P6 | development | 211, 213 | yes | Holdouts 223, 227, 229 were not authorized/consumed for the incomplete original stage screen. |
| V2-6 P6-R1 | development | 211, 213 | yes | P6-R1 development all O0–O4 valid. |
| V2-6 P6-R1 | untouched replication | 223, 227, 229 | yes | All 15 cells consumed and valid; OOS support is only for viability/stage activation. |
| V2-7 P7a | development | 307, 311 | yes | Holdouts 313, 317, 331 remain untouched. |
| V2-7 P7b | development | 337, 341 | yes | Holdouts 347, 349, 353 remain untouched. |
| V2-7 P7c | development | 367, 371 | yes | Holdouts 373, 379, 383 remain untouched. |
| V2-7 P7d | development | 431, 433 | yes | Seed 431 is also a reserved-but-unconsumed P4b holdout; this P7d development use is allowed by the experiment-specific contract. |
| V2-7 P7d | untouched replication | 439, 443, 449 | **439 consumed; 443/449 untouched** | C/L/S-439 evidence exists without a final package/verdict; 443 and 449 have not been run. |

No accidental reuse is hidden by the table.  The important cross-protocol cases
are intentional: 107/109 are L1-P3 holdouts but P4/P3e development seeds;
431 is a P4b holdout but a P7d development seed.  They are not valid reasons to
invalidate either protocol unless its own preregistered holdout rule forbids
the cross-use, which these documents do not.

The broad legacy `ffa-2026-08-*` configuration directories are historical
exploratory calibration material, not V2 holdout contracts.  They are retained
but are not silently counted as V2 development or validation seeds.

## 7. What is actually missing before a V2 freeze?

This section uses the persistent objective and current design ledger, not a new
ambition that every metric must pass.

### A. Must complete before claiming a clean, current V2 freeze

1. **Close the already-authorized P7d holdout contract.**  The 439 material
   must pass the fail-closed completion/extraction contract or be explicitly
   classified incomplete; if the preregistered all-three policy is retained,
   443 and 449 must then be run under their already-pinned configs. No score
   may use the visible S-439 JSON in place of its missing status sentinel.
2. **Create an exact freeze provenance package.**  Rebuild simulator, analyzer,
   and gate from a clean declared source revision; record configs, seeds,
   binary hashes, GOMAXPROCS, execution hashes, evidence-artifact hashes, and
   analysis revision. Current binaries are semantically useful but stamped
   dirty/older than HEAD and cannot be the final immutable V2 package.
3. **Run the final freeze validation contract.**  Fresh-process determinism,
   positive-world equivalence where appropriate, accounting/lifecycle/
   matching/position/settlement/funding/risk/information-boundary gates,
   high-value mutations, and final controls must be regenerated from the
   declared V2 freeze rather than inherited from mixed historical revisions.
4. **Publish the V2 autopsy and holdout partition.**  The current branch has no
   final `frozen-autopsy-v2`/equivalent artifact or complete calibration versus
   untouched-holdout scoreboard. A freeze without an explicit negative-result
   and limitation ledger would not satisfy the objective.

### B. Should complete before freeze if the corresponding claim is intended

These are high-value unresolved identification questions, but a negative result
may be frozen honestly instead of being tuned until positive:

* **Trade-mediated cross-venue discovery:** V2-2b never estimates the router
  channel when informed makers are on, and the router-on/off snapshot endpoint
  is falsified in the no-informed arm. A clean event-attributed trade-channel
  experiment is needed to claim trade-mediated convergence.
* **Activity-generator replacement:** L0/L1 support a local liability motive,
  not broad demotion/replacement. A V2 freeze that claims a coherent ecology
  should either complete a separately preregistered replacement slice or label
  the retained `noise_flow` population as a design limitation.
* **Funding/carry:** P4 and P4b do not identify a basis response; P5 has no
  eligible term. Further work must be an ex-ante protocol, not a post-outcome
  magnitude rescue. Alternatively, close these as falsified/inactive and do not
  claim funding anchoring or dated convergence.
* **Options:** P6-R1 has OOS viability and stage activation, but no causal
  option-surface or hedge-feedback comparison. A freeze may include the stages
  with explicit “inherited/uncausal” limits, but cannot call smile/skew
  emergent.
* **Distress:** P7d development is screening-only until the holdout package is
  complete. Bankruptcy remains unexercised; deficits/insurance were observed in
  the long development orientation and need independent accounting treatment.
* **Evidence/mutation coverage:** close or explicitly scope known gaps in GTC
  cancellation state transitions, broader cross-margin portfolios, per-fill
  option paths, run-level priority wiring, and no-op/direct strategy information
  frontiers before making universal information/mechanical claims.
* **Clock robustness:** L1-P3 is MIXED and must not be tuned. It can remain a
  seed-sensitive limitation if no ecology-wide timing claim is made.

### C. Can remain explicit limitations rather than be tuned away

* P4's exact-zero basis response and P4b's execution failure;
* P5's inactive dated-carry development screen;
* P3-R1's unexercised replenishment trigger;
* P7a–P7c participant-risk non-exercise;
* absent bankruptcy under the registered stress;
* O3/O4 structure inherited from SABR/Vanna–Volga priors;
* lack of robust stylized facts in the historical ae13f9a autopsy;
* two-seed screening strength where no holdout was registered;
* current performance trade-offs and the decision to retain `encoding/json`.

The distinction is deliberate: a mechanism can be falsified or inactive and
still be a scientifically useful part of a frozen autopsy. What cannot remain
ambiguous is whether a claimed mechanism was actually exercised, whether its
evidence was independently reconstructible, or whether a holdout was consumed.

## 8. Freeze-readiness scorecard

| category | readiness | basis and limitation |
|---|---|---|
| matching/mechanics | **READY WITH EXPLICIT LIMITATION** | Strong matcher/accounting fixtures and signed-price tests; run-level priority and some broad portfolio paths remain mutation-limited. |
| accounting/conservation | **READY WITH EXPLICIT LIMITATION** | Core conservation, fees, funding, settlement and truncation residuals are audited; broader cross-margin/option-per-fill coverage is incomplete. |
| price semantics | **READY** | Signed-price branch, explicit availability, midpoint proof/tests, zero settlement wire and positive-world equivalence are complete. |
| lifecycle | **READY WITH EXPLICIT LIMITATION** | P3e finite-term closure is supported; P3c failure and P7/option lifecycle paths are scoped; pending/unusual cancellation coverage remains. |
| information boundaries | **READY WITH EXPLICIT LIMITATION** | V2-0/1/2 receipt/frontier contracts are strong for audited emitted decisions, not every no-op/internal strategy state. |
| cross-venue information | **READY WITH EXPLICIT LIMITATION** | Delayed remote feed and quote-mediated screening support exist; no universal heterogeneous price-discovery claim. |
| cross-venue arbitrage | **NOT READY** | Router construction/activation exists, but trade-mediated convergence is unmeasured and the registered snapshot effect is falsified in one arm. |
| passive/inventory mechanics | **READY WITH EXPLICIT LIMITATION** | P0/P1/P2 activation supported; P3 replenishment not exercised; no stability claim. |
| economic demand/ecology | **NOT READY** | Liability motive is narrow; no broad activity-generator replacement, wealth, concentration or ecological survival screen is complete. |
| perp funding | **NOT READY** | P4 falsified at basis; P4b falsified at execution; no out-of-sample causal funding result. |
| dated futures | **NOT READY** | P5 has no eligible terms; convergence/carry not exercised. |
| options | **READY WITH EXPLICIT LIMITATION** | P6-R1 stage viability is OOS supported; causal surface/hedge/emergence claims are not ready. |
| distress | **NOT READY** | P7d development supports the risk path, but seed-439 packaging/scoring is incomplete and 443/449 remain untouched; bankruptcy is unexercised. |
| timing robustness | **READY WITH EXPLICIT LIMITATION** | Explicit phases and mutation evidence exist; L1-P3 is MIXED and not robust. |
| mutation coverage | **READY WITH EXPLICIT LIMITATION** | Many high-value mutations are CAUGHT; GTC cancel, broad cross-margin, per-fill options, run-level priority and no-op information remain limited/NOT TESTED. |
| determinism | **READY WITH EXPLICIT LIMITATION** | Historical fresh-process/GOMAXPROCS gates pass; current binaries are dirty/stamped before HEAD, so a clean V2 freeze rebuild is still required. |
| performance | **READY WITH EXPLICIT LIMITATION** | V2-8 methodology and post-signed reprofile pass; no unsafe JSON dependency; new freeze workload needs a clean reproducible profile. |
| holdout integrity | **NOT READY** | P6-R1 and L1-P3 holdouts are valid, but P7d 439 is consumed without a final score/status package and 443/449 remain reserved. |

**Is it scientifically defensible to freeze V2 at current HEAD?**

NO

The reason is not that every chart or stylized fact fails. The current
holdout package is incomplete, current binaries are not a clean freeze
attestation, and the declared V2 freeze/autopsy contract has not yet been
regenerated at one immutable source. Funding, dated carry, broad ecology and
trade-mediated discovery would have to be either completed under existing
protocol discipline or explicitly frozen as limitations before a defensible
freeze claim.

## 9. Next three actions from current HEAD

These are the only three recommended next actions. They are ordered by
scientific value and do not authorize tuning or new experiments inside this
audit.

1. **Finish and score the already-authorized P7d holdout policy.**
   *Type:* holdout validation/provenance repair, not economic redesign.
   *Why:* P7d development `SUPPORTED (screening)` explicitly licenses the
   fixed 439/443/449 policy; 439 is already consumed and cannot be silently
   called untouched. Complete the fail-closed extraction package for C/L/S-439
   (or record it invalid/incomplete), then run 443/449 only under the pinned
   configs if the registered all-three holdout policy requires them.
   *Stop condition:* no score unless every required cell has successful exit,
   final sentinels, all metrics, runtime/offline digest agreement and complete
   analysis metadata; no parameter, population, horizon or stress change.

2. **Close the highest-value causal option/funding disposition under a new
   preregistration, without consuming holdouts during design.**
   *Type:* development protocol decision followed by one registered causal
   experiment, not calibration.
   *Why:* P4/P4b/P5 leave funding and dated carry negative/inactive, while
   P6-R1 licenses only option-stage viability. The next scientifically useful
   extension is an ex-ante, independently measured comparison of the existing
   P6-R1 O2 surface/hedge path against the explicit-prior stages, or a formal
   closure of that question if no clean intervention is available. Funding and
   dated results must remain negative/inactive unless every causal link and
   endpoint is met.
   *Stop condition:* activation/evidence failure yields `NOT IDENTIFIED`,
   `FALSIFIED AT ACTIVATION`, or `NOT EXERCISED`; no hurdle, capital, spread,
   clock or SABR/VV prior is selected after outcomes.

3. **Assemble a clean immutable V2 freeze candidate and run the final audit
   package.**
   *Type:* freeze preparation and validation.
   *Why:* the goal requires one exact simulator/config/seed trajectory,
   regenerated controls, mutation/evidence/risk/lifecycle gates, a complete
   calibration-versus-holdout scoreboard, and the V2 autopsy. This is where
   unresolved P4/P5/options/ecology/timing findings become explicit
   limitations rather than hidden blockers.
   *Stop condition:* freeze only after clean binaries, fresh-process hashes,
   evidence digests, required mutation classifications, holdout accounting and
   final machine-readable artifacts are internally consistent; do not tune
   economics to improve the scoreboard.

## 10. Proposed updated goal state

The following is a replacement draft for only the stale `Current state:` and
`Immediate sequence:` portions of the persistent objective.  The objective
file itself is intentionally not modified by this audit.

### Current state:

* The ae13f9a frozen autopsy remains complete and historical at `7404b51`.
* V2-0 receipt/frontier evidence, V2-1 participant-local feed construction,
  V2-2 router evidence and the V2-2b quote-mediated smoke are complete at
  screening scope.  Quote-mediated dispersion reduction is supported in the
  tested cells; trade-mediated convergence is not identified.
* Signed-price is complete and merged: implementation `320262e`, hardening
  `5afdd45`, provenance closure `7644b2`.  Positive-world determinism,
  evidence equivalence, arithmetic, settlement, matcher and performance gates
  passed.  Do not recreate this branch absent a new regression.
* V2-3 P0 post-only admission is mechanically supported; actor ordering is
  mixed; P1 asymmetric size and P2 explicit rebalance are supported for narrow
  local activation; P3-R1 replenishment is not exercised.
* V2-4 L0/L1/L1-P/L1-P2 local liability/timing screens are supported narrowly;
  L1-P3 untouched replication is MIXED (107/109 repeat, 113 reverses).  No
  broad activity-generator replacement is complete.
* V2-5 P3e passive finite-term lifecycle is supported for its narrow closure
  contract.  P4 funding/carry is FALSIFIED at the registered basis endpoint;
  P4b is FALSIFIED AT EXECUTION; P5 dated carry is NOT EXERCISED.  Their
  untouched holdouts remain unconsumed.
* V2-6 original P6 is incomplete.  P6-R1 development and untouched seeds
  223/227/229 support cross-asset viability and O0–O4 stage activation only;
  IV/parity/hedge measurement is independent, but surface emergence and
  O2-directional transmission are not identified and O3/O4 structure is
  explicitly prior-driven.
* V2-7 P7a–P7c participant distress is NOT EXERCISED.  P7d development
  (`431/433`) supports finite-capital directional activation and participant
  risk replay.  Holdout seed 439 C/L/S worlds are consumed and not yet scored
  as a complete package; 443/449 remain untouched.
* V2-8 profiling and post-signed reprofile are complete as methodology gates;
  `encoding/json` remains the reference and no JSON dependency has been
  adopted.
* The current branch is `autoresearch/ffa-ecology-gen0` at `061493a`, with
  pre-existing dirty scoreboard files and retained untracked evidence.  A
  clean current-head build has not yet replaced the recorded P7d binaries.

### Immediate sequence:

1. Complete the registered P7d holdout evidence package without rerunning or
   retuning seed 439; run 443/449 only if required by the unchanged holdout
   policy, then publish a fail-closed holdout verdict.
2. Keep P4/P4b/P5 negative or inactive unless a separately preregistered,
   independently measurable funding/dated-carry causal extension is justified;
   complete the existing P6-R1 option-surface/hedge causal disposition without
   calling explicit SABR/VV structure emergent.
3. Build a clean immutable V2 candidate, regenerate controls and all required
   accounting/lifecycle/information/risk/mutation/determinism artifacts, and
   publish the final calibration/untouched-holdout/discovery scoreboard and V2
   autopsy.  Treat broad ecology replacement, trade-mediated discovery,
   bankruptcy, and mixed timing as explicit limitations unless their existing
   protocols produce identifiable evidence.

## Post-audit update: P7d holdout package

This append-only update supersedes the P7d portion of the reconciliation above
without rewriting its historical state.  At result commit `c84d671` and review
commit `d4d31b8`, the pre-reserved P7d holdout seeds 439, 443 and 449 have all
been consumed under the pinned source revision, binary, configs and evidence
contract.  All nine C/L/S cells have complete extraction status, final
`greeks.json`/`latency.json` sentinels, all sixteen registered metric artifacts,
analysis metadata and runtime/offline evidence-artifact digest equality.  Raw
evidence remains retained.

Per-seed result classifications are:

| seed | activation | participant-specific risk |
|---:|---|---|
| 439 | `SUPPORTED (screening)` | `MIXED` (long exercised, short not exercised) |
| 443 | `SUPPORTED (screening)` | `SUPPORTED (screening)` |
| 449 | `SUPPORTED (screening)` | `SUPPORTED (screening)` |

The machine package is
`research/artifacts/v2-7-p7d/p7d-holdout-verdict.json`.  It now contains
exactly one per-seed record for each of 439/443/449 and explicitly records
aggregate `NOT IDENTIFIED`: the P7d preregistration reserved three seeds but
did not define an all-seed, majority or other aggregate rule.  No aggregate
replication claim is licensed.  The seed-439 scorer was written after its
metrics existed and was reviewed with a narrower factual claim; that
provenance is retained.  The 443/449 cells were run only after that scorer
review and the pinned scorer was applied unchanged.

The current package does **not** score participant-specific deficit or
insurance, because the available ancillary liquidation field is ecology-wide;
bankruptcy remains not exercised/not identified.  It also makes no funding,
basis, profitability, market-stability or full-ecology liquidation claim.
Earlier pre-review 443/449 attempts are archived as status-143 invalid runs
and are excluded from the valid package.  The fallback red-team review
(`research/reviews/v2-7-p7d-holdout-results-independent-review.md`) records
`ACCEPT WITH NARROWER CLAIM`; configured Sol-xhigh review agents were
unavailable due to the usage limit, so this is explicitly not an independent
Sol review.

The next licensed scientific gate is the V2-6 causal option disposition (or a
formal closure as `NOT IDENTIFIED` if no clean contrast exists), followed by a
freeze-readiness review.  P4/P4b/P5 and the mixed L1-P3 timing line remain
explicit limitations; no P7d distress retuning is authorized.

## Post-audit update: V2-6 causal option disposition

At commits `08b236c` and `7882310`, the V2-6 causal question was reviewed after
the complete P6-R1 viability/stage-activation development and untouched
replication.  The disposition is `NOT IDENTIFIED` for an emergent option
surface or directional option-to-underlying hedge-response claim.  P6-R1
remains `SUPPORTED (screening)` for O0--O4 viability and participant
activation on development seeds 211/213 and consumed holdout seeds 223/227/229.

The reason is contractual, not an activation failure: O1→O2 bundles liability
demand with dealer delta hedging and never fixed the O2 transmission sign or
effect-size corridor; O3→O2 and O4→O3 add explicit SABR and Vanna--Volga
priors, respectively, so any incremental surface structure is inherited by
construction.  No matched prior removal/restoration or belief-permutation
contrast was preregistered.  Market IV, parity, dealer exposure and hedge
evidence are independently measurable, but their causal surface/feedback
interpretation is not licensed.  The machine disposition is
`research/artifacts/v2-6-option-causal-disposition.json`; the detailed record
is `research/v2-6-option-causal-disposition.md`.

Configured Sol-xhigh reviewers were unavailable due to the model-usage limit;
the fallback red-team review is explicitly labeled non-independent in
`research/reviews/v2-6-option-causal-disposition-review.md`.  No new option
simulation or holdout was consumed.  This closes the option-emergence question
as an explicit V2 limitation and licenses a freeze-readiness review rather
than a post-outcome options experiment.

## Post-audit update: evidence durability and freeze preparation

At `930c313` the evidence-durability review recorded the Sol-family verdict
`ACCEPT WITH NARROWER CLAIM` for the earlier transport changes.  The follow-up
correctness commit `2068d9d` now enforces the required fresh evidence sink at
the `multivenue.NewSim` boundary, retains the first `Sim.Close` result, writes
latency before the raw-artifact attestation, and atomically publishes compact
sidecars.  The successful scientific contract is therefore:

    fresh output directory (optionally containing only run-config/run-metadata)
    + zero simulator exit
    + fresh greeks.json and latency.json
    + runtime/offline evidence-digest equality

This does not claim crash-durable streaming JSONL or directory `fsync`; those
remain explicit limitations.  The correction is instrumentation/evidence
handling only and does not change successful-run economic state, scheduling,
RNG consumption, or event ordering.  A clean-source candidate determinism and
mechanical-gate run remains outstanding before the immutable V2 freeze.

## Append-only operational update: corrected clean gate (2026-08-31)

The corrected code commit is `494d696`, with its state-record documentation in
`558fe21`. The clean exact-tree mechanical gates now pass: full `GOMAXPROCS=4
make test`, `go vet ./...`, and targeted race coverage for the analyzer/gate
packages and `tests`. This does not promote the candidate: Banach rejected the
predecessor `83dc7b1`, and a fresh independent Sol-xhigh review of the current
HEAD is still mandatory.

The evidence-capacity preflight intentionally requires approximately 51 GiB
free from a retained 35,341,880,370-byte complete historical tree with a
1.5x-plus-2-GiB reserve. The host has approximately 28 GiB free, so the next
hard stop is external capacity, not a scientific failure. No R2 cell, parity
control, archive/prune operation, or holdout `619/631/641` has run.

## Append-only operational update: R2 calendar gate review (2026-08-31)

The current R2 successor must not be inferred from the older freeze-readiness
sections above. Exact code commit `494d696` follows Banach's independent
Sol-xhigh rejection of `83dc7b1`. The rejection was valid and pre-launch: the
calendar audit accepted empty or renamed venue identities, the shell timeline
fixture was not connected to the extractor's default helper path, and the
runner's 5 GiB free-space check was contradicted by retained full-run size.

The correction adds exact `central,north,south` venue binding, empty/missing/
renamed venue regressions, independent literal/helper timeline comparison and
default-path testing, and a measured capacity floor of approximately 51 GiB
free. Focused tests and the R2 contract pass. The dirty full-suite attempt is
retained only as a diagnostic because the parity/archive tests correctly
require a clean worktree; clean full, vet, race, and fresh exact-tree review
remain pending. The host currently has approximately 28 GiB free, so the
capacity preflight is intentionally a hard stop before dev-607.

No R2 development cell, parity control, archive/prune operation, or holdout
`619/631/641` has run. The incomplete temporary derivative-proxy evidence tree
is preserved. The next valid progression is clean gate -> independent review
-> provenance-pinned build -> dev-607 -> extraction/review -> remaining
registered development cells and controls; holdouts remain behind explicit
freeze authorization.

## Append-only operational update: reviewer and capacity stop (2026-08-31)

After the clean gates passed, safe removal of the regenerable Go build/test
cache recovered approximately 4.85 GiB; current free space is approximately
32 GiB. This remains below the R2 runner’s evidence-based approximately 51 GiB
floor. No retained raw evidence, incomplete derivative-proxy tree, historical
result, or holdout was deleted or altered.

The exact post-correction candidate still needs a fresh independent Sol-xhigh
review. The review service currently reports its agent-thread limit even after
all prior review threads completed, so no prior superseded verdict is being
used as acceptance. Binary rebuild and every R2 cell remain gated on both
fresh review and safe capacity.

## Append-only current checkpoint — F3 correction `90d0ffb` (2026-09-09)

The active scientific tree is clean at
`90d0ffbf32a07f3393a5be903b30fa9c954f723a`. The F3 simulator correction is
committed and pushed after an independent Avicenna Sol-xhigh conditional
acceptance of the exact code diff. It establishes an exchange-owned coherent
mark epoch for the built-in perpetual, dated-future, and option risk path;
missing sibling marks fail closed; producer/risk entry points are serialized;
and built-in option maintenance consumes the committed underlying/premium
pair. The optional snapshotter interface preserves extensibility for custom
position-margin instruments.

The exact post-commit mechanical evidence is green: focused regressions,
focused race tests, clean full `make test` (including the integrated long-run
and R2 contract/archive suites), `go vet ./...`, and `git diff --check`. No
development cell, parity cell, or holdout was launched. Holdouts `619/631/641`
remain untouched. Host capacity at this checkpoint is approximately 48 GiB
free and 23 GiB available RAM; no launch capacity claim has been made from
that observation.

The retained old trajectory was not rewritten. Multi-symbol positions make F3
reachable in the historical topology, while retained evidence contains zero
observable `liquidation_check`, `liquidation`, or `margin_call` records. This
is not proof that unlogged internal solvent checks never occurred, so the old
trajectory is not promoted under corrected semantics and must be rerun only in
the authorized successor development sequence. The old R2 candidate remains
closed as `NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`; the CDF-liquidity
successor remains separately named and negative, and is not merged into this
scientific branch.

The asynchronous refs were fetched at this checkpoint with no new commits
after performance `b1847ac`, performance-port `39768df`, or economic-red-team
`e85e16c`. The binary evidence work remains a separately gated infrastructure
successor; no performance-branch code was imported by `90d0ffb`. The next
promotion boundary is binary-evidence successor reconciliation, exact review
where required, pinned build, and development-only execution. Freeze
authorization is still absent.

## Append-only checkpoint: SV1C asynchronous closure — 2026-09-09

The separate `feature/r2-cdf-survival-successor` branch was fetched and
inspected at `1fda960`. Its exact-tree Lovelace Sol-xhigh report
(`research/reviews/v2-r2-sv1c-independent-review-2026-09-09.md`) rejects SV1C
for scientific promotion, while accepting its corrected global-order replay
as a valid analysis-only negative activation result. Seven of twelve
supplier/venue instances activated, but five lacked a post-fill
inventory-responsive decision; supplier-removal reconstruction covered only
900 of 939 snapshots. Consequently the preregistered activation and
anti-cheating gates are not satisfied.

The rejection is preserved as a negative result. No predicate was relaxed, no
supplier was selected or retuned after the fact, no capacity seed 659 or
24-hour development was run, and no freeze or holdout authorization exists.
The active scientific branch remains the independent F3-corrected tree
`90d0ffb`/`2dc7acd`; SV1C implementation changes remain off-branch. A future
CDF successor, if scientifically justified, requires a new name, an
independently motivated finite roster and preregistration, fresh exact-tree
review, and a new development-only activation probe. Holdouts `619/631/641`
remain untouched.

## Append-only checkpoint: SV1D asynchronous closure — 2026-09-10

The separate `origin/feature/r2-cdf-survival-successor-sv1d` branch was
inspected read-only through pushed tip `039f008f1aa9f3a748bf3fac33d0bbb7a2e1892e`.
It remains outside the authoritative scientific tree. Its seed-659 activation
attempt is **INVALID ARM EVIDENCE / NON-ADVANCING GATE**, not a valid
activation-negative result.

The registered five-minute treatment, same-roster mode-off control, and
no-roster control all stopped at simulated `00:01:01` on a scheduled South
option-risk pass because the underlying CDF/USD book was transiently empty.
The producer correctly rejected the nonterminal `SIMULATION_FAILURE`: none of
the three arms reached a valid registered endpoint, and no arm had a
`local_book_mode == "one_sided"` supplier decision. Treatment and mode-off
execution streams were identical. The retained activation root and its
Lagrange Sol-xhigh review classify this as a repair-required producer/fixture
boundary, not evidence for or against the CDF supplier hypothesis.

The SV1D capacity-protocol review also rejected an earlier outcome-bearing
capacity procedure. The branch replaced it with a preregistered synthetic,
outcome-neutral `evstream_v3` workload, but that capacity path and the invalid
activation result remain branch-local. No SV1D code, binary, capacity
attestation, development cell, freeze, or holdout was imported or launched on
this branch. Holdouts `619/631/641` remain untouched.

The only permitted continuation for SV1D is source-level diagnosis and a
minimal non-economic producer/fixture repair, followed by focused/full gates
and a fresh exact-tree independent review. Any change to risk semantics,
fallback valuation, roster economics, warm-up, or seed is a new successor
amendment. The authoritative branch remains at `22c414d`; its R2 candidate is
closed as `NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`.

## Append-only current checkpoint: strict CDF equity correction `3fe893b` — 2026-09-10

The live scientific worktree is clean at
`3fe893b5626a33994bdf47015f1a1b306a8a7fdc`, with the same revision pushed to
`origin/autoresearch/ffa-ecology-gen0`. The correction separates a current
local risk mark from cached CDF equity, reconstructs strict marked equity and
peak from initial account state and actor-visible fill deltas, and keeps the
cached state unchanged for pending/missing/unavailable observations. The
binary CDF decision envelope is now schema version 2 with backward-compatible
v1 decoding.

`GOMAXPROCS=4 GOMEMLIMIT=8GiB make test` passed cleanly on this exact tree,
including the integrated long-run/R2/archive contract suites. No new
scientific trajectory or capacity measurement exists. The performance feed
was fetched and has no commits after `b1847ac`; its changes remain deferred.
The remaining promotion work is strict CDF end-to-end evidence coverage and
the preregistered supplier concentration, quote-lifecycle, inventory/PnL, and
removal-counterfactual diagnostics, followed by fresh exact-tree review. No
holdout may be touched before freeze authorization.

## Append-only current checkpoint: Goodall review response and lifecycle hardening — 2026-09-10

The exact scientific HEAD is `c2b0ad7`, pushed on
`autoresearch/ffa-ecology-gen0`, with a clean worktree. The predecessor
`4965ada` was rejected by the independent Goodall the 2nd Sol-xhigh review.
The rejection identified: missing payload-content binding, unenforced
one-live-order semantics, an unaccepted forced-cancel race, producer/log depth
ordering, inferred rather than linked repricing, self-attested cancellation
motivation, and insufficient strict adversarial coverage.

The successor response is split across `fbdec9d`, `1298a7f`, `0c0266f`, and
`c2b0ad7`. Epoch-4 binary evidence binds the renderer/analyzer payload to the
source digest; CDF decision schema v3 carries `replaces_order_id`; strict
analysis enforces one live order and no terminal identity reuse; allowed
economic reasons are explicit; forced cancellation is logged after the public
book update; forced-cancel/cancel-rejection races are reconciled; and
per-supplier activation requires a real withdrawal or reprice cancellation.
The tests include a strict end-to-end source/render payload-tamper fixture and
the lifecycle/order/ordering adversarial cases listed in `RESUME-HERE.md`.

The clean bounded `GOMAXPROCS=4 GOMEMLIMIT=8GiB make test` passed on the exact
HEAD, including all package tests and integrated-long-run/R2/archive guards.
Focused suites and `git diff --check` pass. Performance ref
`origin/autoresearch/v2-performance-research` was fetched at this checkpoint;
there is no commit after `b1847ac`, and no performance implementation was
imported. The host currently has approximately 38 GiB free and 24 GiB
available RAM. No development/capacity cell, binary launch, freeze, or
holdout `619/631/641` has run.

Promotion remains blocked until one fresh independent review accepts this
exact complete tree, followed by vet/race/fresh-process checks, a measured
binary-evidence capacity floor, and a clean provenance-pinned Go 1.27 build.
The old R2 candidate remains the archived negative control
`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`; no historical result has been
rewritten.

## Append-only current checkpoint: Hooke review rejection and correction `146bc02` — 2026-09-11

Hooke the 2nd (Sol-xhigh) reviewed exact clean predecessor `b4f8bd4` and
rejected promotion. This was a substantive review, not a request for cosmetic
changes. The six blockers were: incorrect inner-versus-wrapped payload digest
binding; incomplete/overly strict actor wait-reason handling with self-attested
reasons; loss of replacement lineage after a rejected order request; absence
of a strict production `events.evs` to renderer to complete-audit test; missing
exact per-supplier event-time depth/concentration/removal evidence; and a
strict activation predicate stronger than the preregistered global lifecycle
criterion.

The exact-tree correction is split into `cde2e62` (wrapped payload identity)
and `146bc02` (lifecycle, reason derivation, lineage, production E2E, and
diagnostic hardening). The current HEAD is `146bc02`, pushed and clean. Strict
analysis now binds identity to the exact rendered outer frame, derives valid
decision reasons from reconstructed state, supports the registered actor wait
vocabulary, preserves `replaces_order_id` across rejected replacements,
retains event-time supplier depth, calculates side-specific time-weighted and
removal-counterfactual diagnostics, and requires global rather than
per-supplier withdrawal/reprice as preregistered.

Post-correction evidence: clean `make test`, `go vet ./...`, focused
analysis/exchange/multivenue tests, targeted race tests, fresh-process
determinism/evidence-neutrality checks, and the strict production-renderer
binary E2E audit all pass. This is only a mechanical correction gate. The
fresh exact-tree independent review is still required before capacity,
provenance-pinned build, or the SV1D activation probe. No development cell,
freeze, or holdout ran; `619/631/641` remain untouched. The performance feed
still has no commit after reviewed `b1847ac`, and no performance change was
merged.

## Append-only current checkpoint: second exact-tree review rejection — 2026-09-11

Two independent Sol-xhigh reviewers reviewed exact clean `764f10d` and rejected
promotion. The common blocker is production cancellation ordering: the exchange
removes public depth and emits `BookDelta` before emitting `OrderCancelled`,
but strict analysis attributes the intermediate delta while the supplier order
is still live. This can invalidate a legitimate supplier withdrawal/reprice in
the one-sided sparse-book state under investigation. The strict production
renderer fixture did not include that actual producer sequence.

Additional blockers are a post-fill response predicate that can credit
market-driven target movement rather than an inventory-attributable response,
hash-only validation of required completion sidecars without schema/content
checks through a production activation adapter, and a `limit_or_touch_unavailable`
predicate that can reject a valid existing-quote withdrawal because stale
positive quote fields survive the producer's early failure path.

No capacity, development, freeze, or holdout action was taken. No historical
result was rewritten. These findings supersede the prior mechanical-ready
checkpoint; the next code correction and fresh exact-tree review are required
before any binary capacity attestation or SV1D activation probe.

## Append-only current checkpoint: strict-audit correction `66f5781` — 2026-09-11

Exact scientific HEAD `66f5781` is pushed and clean. It responds to the two
independent Sol-xhigh rejections of `764f10d` without changing economic
semantics. Strict depth attribution now models the real producer transition
`BookDelta` (public removal) then `OrderCancelled` (supplier-order removal),
with additive deltas recorded immediately and removal deltas flushed only after
the corresponding lifecycle state change. The binary fixture covers aggregate
same-price siblings, fills, cancellations, one-sided posting/restoration, and
production binary rendering into the strict audit.

The post-fill activation predicate now requires a same delayed public
market/coherent risk state and changed quote terms. It excludes private
reference/target movement and request-ID churn as sufficient evidence. A
direct stale-positive-quote regression covers `limit_or_touch_unavailable`.
The completion contract now validates sidecar schemas/content and terminal
checkpoint order in addition to hashes, and the production `mvanalyze` plus
shell adapter invokes the strict activation audit with externally supplied
provenance.

Mechanical evidence: analysis tests, the requested evstream/types/exchange/
multivenue/mvanalyze suites, `go vet ./...`, shell syntax, diff checks, and a
clean bounded `make test` all passed. Targeted race/fresh-process checks and a
fresh exact-tree independent review remain pending. The performance feed still
has no commit after reviewed `b1847ac`; no performance implementation was
merged. Disk/RAM were approximately 38 GiB free and 24 GiB available. No
capacity or binary build, SV1D activation probe, development cell, freeze, or
holdout `619/631/641` was run.

## Append-only current checkpoint: exact-tree mechanical gates complete — 2026-09-11

Exact clean HEAD is `50210cd`. The bounded targeted race suite
`go test -race ./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`
completed with exit status 0, and the fresh-process
determinism/evidence-neutrality matrix completed with exit status 0. Logs are
retained as `exsim-v2-race-50210cd.log` and
`exsim-v2-fresh-50210cd.log` in the system temporary workspace; both runs were bounded by
`GOMAXPROCS=2 GOMEMLIMIT=4GiB`.

The current successor therefore has a complete mechanical gate record
alongside the focused/full/vet checks recorded above. Promotion remains
blocked only by the required fresh independent exact-tree Sol-xhigh review at
this boundary; acceptance is not inferred from passing tests. After an
accepted review, measure binary-evidence capacity from an actual run, build
the pinned Go 1.27 binaries, and run the smallest development SV1D activation
probe before any registered 24h cell. No capacity, development, freeze, or
holdout action occurred. The performance branch still has no commit after
`b1847ac`, and no performance code was merged.

## Append-only current checkpoint: fresh exact-tree review rejection — 2026-09-11

Two independent Sol-xhigh reviewers rejected clean exact `144d151` before any
capacity, pinned binary, SV1D probe, development cell, freeze, or holdout.
They independently confirmed that R2 economics/calendar/risk behavior is
unchanged and that the CDF successor remains finite, opt-in, delayed-local,
and free of forced two-sided quoting or hidden global price access.

Promotion blockers are preserved here rather than silently treating the
passing fixtures as acceptance: nonzero shared-level cancellation deltas are
not deferred until `OrderCancelled`; full-fill producer ordering is not
represented by the strict fixture; post-fill response can be credited from
reference/target or order-ID changes; raw manifest records and checkpoint
terminal binding are not fully validated; the shell adapter derives expected
hashes from the artifact under test and lacks immutable analyzer/renderer/probe
identities; and the binary-only runner/manifest/status contract does not match
the strict validator. No immutable SV1D treatment, mode-off, and no-roster
configs currently exist under `research/configs`.

The required disposition is **REJECT promotion; correct and review again**.
The old R2 negative result and all historical SV1C/invalid-SV1D records remain
unchanged. The next safe work is a minimal evidence/provenance correction and
production-path regression suite. No capacity or scientific run is allowed
from `144d151`.

## Append-only checkpoint: binary successor evidence contract `7e8d9fa` — 2026-09-11

The current scientific HEAD is clean, pushed, and exactly `7e8d9fa`. This
checkpoint preserves the accepted R2 calendar/risk/economic semantics and only
hardens successor evidence registration and verification.

All seven registered R2 configs now explicitly set binary
`evidence_contract_version` to 2, and the config checker requires it. This
closes the prior ambiguity where `evstream_v3` with an omitted version would
default to the legacy epoch-1 envelope. The binary evidence completion path
now commits market-data evidence, schedules, receipts, and decisions to both
the manifest and run status; strict checkpoint records are replayed against
the exact binary prefixes; fixed-file symlinks are rejected; and strict
microstructure/latency completion cannot be empty.

The parity contract has an independent production-renderer pass for full-log
dev-607 and dev-607-g8. It checks renderer build identity, source frame/event
counts and hashes, rendered attestation, deterministic rendered venue files,
and records renderer identity in the parity attestation. The archive fixture
was upgraded to a contiguous epoch-4/global-sequence stream so the production
reader is exercised in the contract test.

Verification on the exact tree: focused Go suites, shell syntax and diff
hygiene, the R2 config contract, and clean bounded
`GOMAXPROCS=2 GOMEMLIMIT=4GiB make test` all pass. The last full gate included
all package tests plus integrated-long-run, R2, generic archive, and R2
renderer-parity archive suites. A fresh fetch of
`origin/autoresearch/v2-performance-research` still has no commit after
reviewed `b1847ac`; no performance branch implementation was merged.

No capacity, pinned build, CDF probe, development cell, freeze, or holdout
action occurred. Holdouts `619/631/641` remain untouched. This does not grant
promotion: targeted race/fresh-process checks and one fresh exact-tree
independent Sol-xhigh review remain before capacity or scientific execution.

## Append-only checkpoint: exact current mechanical gate `0bfb809` — 2026-09-11

The scientific tree is clean and pushed at `0bfb809`. The targeted race suite
passed at `GOMAXPROCS=2 GOMEMLIMIT=4GiB`; the fresh-process execution,
binary-evidence, and logging-neutrality checks passed as well. `go vet ./...`
and `git diff --check` pass. The prior clean `make test` at the code-bearing
ancestor `7e8d9fa` passed after the renderer/parity changes; this latest commit
contains only this append-only state record.

The retained temporary-workspace logs are named
`exsim-race-0bfb809.log`, `exsim-fresh-0bfb809.log`, and
`exsim-binary-neutral-0bfb809.log`. No source, config, historical evidence, or
economic result was altered by these checks. The performance refetch found no
commit after reviewed `b1847ac`.

This closes the current mechanical gate but is not independent scientific
acceptance. Exactly one fresh Sol-xhigh review of the complete tree remains.
Only an accepted review permits actual binary capacity measurement, pinned
Go 1.27 builds, and the development-only SV1D activation probe. No capacity,
development, freeze, or holdout action occurred; holdouts `619/631/641` remain
untouched.

## Append-only operational update: SV1D provenance/runner closure (`301a131`) — 2026-09-11

The exact scientific HEAD is `301a131`, clean and pushed. This checkpoint
preserves the R2 calendar/lifecycle, risk, actor, and historical negative
semantics. It adds only successor evidence/provenance hardening: a production
`sv1dprobe` CLI, immutable simulator/analyzer/renderer identities in the
tri-arm plan/results, renderer self-attestation bound to the rendered binary
evidence, and a development-only runner with fresh namespaces, explicit
review/probe authorization, clean same-revision pinned-tool checks, and a
measured binary-capacity prerequisite. The runner has not been authorized or
executed.

Mechanical verification on this exact tree is complete: clean bounded
`GOMAXPROCS=2 GOMEMLIMIT=4GiB make test`, `go vet ./...`, targeted race tests
for `analysis`, `cmd/mvanalyze`, `cmd/prunegate`, and `tests`, and fresh-process
determinism/binary-evidence/log-mode neutrality all pass. A temporary
non-campaign 3-second run through clean Go 1.27 binaries and the actual
`evsrender` executable passed with 942 event frames and 15 routes; the renderer
attestation reports `vcs.modified=false`, linux/amd64/v1, trimpath, and
CGO-disabled provenance and hashes the rendered attestation.

The first make-test attempt on the pre-commit dirty tree was correctly stopped
by the parity contract; the clean committed rerun passed. The performance
branch was refreshed from `origin/autoresearch/v2-performance-research`
through last-reviewed commit `b1847ac`, with no newer commits and no imported
performance code. No capacity floor, pinned campaign build, SV1D arm, dev-607,
freeze, or holdout `619/631/641` was consumed. Promotion remains pending one
fresh exact-tree independent Sol-xhigh review of `301a131`; acceptance is the
only next authorization for capacity measurement and the seed-659 activation
probe.

## Append-only operational update: observation coverage correction (`e4d069c`) — 2026-09-11

The current scientific HEAD is clean, pushed, and exactly `e4d069c`. The fresh
Sol-xhigh review of the prior exact tree `b1d2a3a` was correctly rejected. Its
reachable initial-interval and persistence findings are now corrected without
altering R2 economics or the SV1D participant contract. Strict activation and
control paths validate the explicit one-second public CDF snapshot grid from
the registered start through terminal coverage, reject missing or shifted
intervals, and admit only a contract-bound empty opening state when no earlier
book transition exists. Adjacent non-two-sided modes remain one persistence
interval rather than resetting at mode changes.

The strict production fixture now emits the complete registered cadence, keeps
the first fill before the delayed second receipt, and delays the synthetic
second quote until after public two-sided restoration. This is fixture repair
for the audited producer timeline, not an economic retuning. Focused strict
renderer audit, full analysis tests, and clean bounded `make test` passed.

Promotion is still blocked. The next implementation gates are exact registered
tri-arm plan names/identities and plan digest, nonzero incomplete-arm handling,
content-bound independent-review attestation, immutable per-arm corpus hashes
and final manifest, corrected runner metadata/content-addressed binaries, and
a measured binary-capacity preflight. No capacity, Go 1.27 campaign build,
SV1D execution, development cell, freeze, or holdout `619/631/641` was run.
The performance feed remains at `b1847ac`, with no imported code.

## Append-only operational update: SV1D runner and capacity binding (`1c870f7`) — 2026-09-11

The exact scientific tree is clean and pushed at `1c870f7`. This checkpoint
adds no economic, roster, calendar, or historical-result change. The measured
capacity contract now verifies that each retained resource trace carries the
same filesystem identity as the attestation. The development-only activation
runner retains a plan-bound typed incomplete result for each failed arm,
continues to the remaining arms, preserves partial evidence, and publishes a
non-success tri-arm score rather than treating a missing result as a passing
control.

The exact tree passes bounded `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test`,
`go vet ./...`, targeted race coverage for the changed analysis/CLI packages,
fresh-process determinism and binary evidence/log-mode neutrality, shell
syntax, and diff hygiene. The earlier dirty-tree full-test rejection is not
used as a green gate; the clean committed rerun is the retained mechanical
evidence.

Promotion is still pending one fresh independent exact-tree Sol-xhigh review.
No measured capacity attestation, clean pinned Go 1.27 campaign binary,
SV1D activation probe, development cell, freeze authorization, or successor
holdout `619/631/641` has been consumed. The performance feed remains at
reviewed `b1847ac`; no performance implementation was imported. Acceptance of
the exact review is required before capacity measurement and the seed-659
probe.

## Append-only operational update: strict SV1D launch provenance (`d96fa1c`) — 2026-09-11

The code-bearing scientific tree is clean, pushed, and pinned at `d96fa1c`;
this entry is documentation-only. No R2
economic semantics or retained result was changed. Strict SV1D launch
validation now independently rechecks the canonical plan and all external
review, capacity, tool, runner, activation-metadata, filesystem, evidence
epoch, and runtime identities at arm-audit and tri-arm-score time. The
development-only shell runner monitors the finite cgroup and host envelope,
rejects swap/OOM-counter changes and filesystem drift, and retains typed
incomplete results for failed arms.

The clean bounded `make test`, `go vet ./...`, targeted race suites, explicit
fresh-process determinism/evidence-neutrality tests, shell syntax, and diff
hygiene all pass. The performance feed was fetched through reviewed `b1847ac`
and had no new commits; no performance implementation was imported. No
capacity measurement, pinned campaign build, SV1D arm, development cell,
freeze authorization, or holdout `619/631/641` was consumed. Promotion remains
blocked only on one fresh exact-tree independent Sol-xhigh review, after which
the measured binary-capacity preflight and development-only activation probe
may proceed.

## Append-only operational update: retained capacity revalidation (`7d92e73`) — 2026-09-11

The fresh independent Sol-xhigh review of exact tree `ccc29c2` rejected one
valid promotion blocker: strict arm-audit and final-score checks validated the
copied capacity attestation but did not reopen the retained capacity manifest,
resource traces, capacity tools/configs, arm evidence, or rendered evidence
under `capacity_root`. The R2 calendar/lifecycle and current correctness
semantics were accepted; no capacity or scientific run was authorized.

The code-bearing scientific tree is now clean, pushed, and pinned at `7d92e73`;
this entry is documentation-only. Strict retention now binds both
`capacity_root` and `capacity_records_root` from activation metadata to the
attestation and invokes the full `VerifySV1DCapacityAttestation` contract at
each strict arm audit and tri-arm score. A regression deletes the measurement
manifest and confirms the retained bundle is rejected. No R2 economics or
historical evidence changed.

Clean bounded `make test`, `go vet ./...`, targeted race tests, explicit
fresh-process determinism/evidence neutrality, shell syntax, and diff hygiene
all pass at this code-bearing tree. No capacity attestation, pinned Go 1.27
campaign build, SV1D activation, development cell, freeze, or holdout
`619/631/641` was consumed. The performance feed remains at reviewed `b1847ac`
with no newer commit and no imported code. Promotion awaits another fresh
exact-tree independent Sol-xhigh review; acceptance alone permits capacity
preflight and the seed-659 activation probe.

## Append-only pause checkpoint: SV1D scoring provenance correction in progress — 2026-09-11

After the second fresh exact-tree Sol-xhigh rejection, scientific HEAD remains
`96c34065` with an uncommitted correction. Strict scoring now binds complete
arm results to a fresh re-audit of retained run/rendered evidence, while the
runner retains content-addressed arm result records below the metadata-bound
`provenance/arm-results` root and uses descriptor-bound no-symlink reads.
`TREATMENT_NOT_ACTIVATED` and `ANTI_CHEATING_REJECTED` scores now retain all
validated venue and aggregate diagnostics before returning. This is provenance
and scoring hardening only; R2 economics, retained evidence, and experiment
authorization are unchanged.

Focused analysis/CLI and `evstream`/`types`/`exchange`/`simulations/multivenue`
tests passed, as did shell syntax and diff hygiene. A new full `make test`
reached the package suites through `simulations/latencylab` and was stopped
with SIGINT when the quota boundary was reached; its final clean/archive gate
was not observed. The patch is therefore intentionally uncommitted and awaits
next-session full test, vet/race/fresh-process validation, commit/push, and a
fresh exact-tree independent review. No capacity preflight, pinned campaign
build, SV1D probe, development cell, freeze, or holdout `619/631/641` was run.

## Append-only checkpoint: SV1D scoring provenance correction mechanically green — 2026-09-12

The saved correction was resumed and fully checked. All package,
integrated-long-run, R2, multivenue, and test suites completed successfully;
the only nonzero `make test` result was the expected dirty-tree archive/parity
guard. `go vet ./...`, targeted race tests, fresh-process
determinism/evidence-neutrality, shell syntax, and diff hygiene passed. The
performance feed was fetched through reviewed `b1847ac` and remains unchanged.

The correction is ready to commit and push: complete strict arm results are
re-audited from retained evidence, content-addressed arm records are rooted in
the activation metadata, no-symlink reads are descriptor-bound, and negative
scientific statuses retain all validated diagnostics. No economic semantics,
historical evidence, capacity artifact, or experiment changed. A clean
post-commit full gate and one fresh exact-tree independent review remain before
capacity or SV1D activation; development cells, freeze, and holdouts
`619/631/641` remain untouched.

## Append-only current checkpoint: weekly pause after resource-evidence hardening (`aaf751e`) — 2026-09-12

The exact scientific HEAD is `aaf751e` on
`autoresearch/ffa-ecology-gen0`; it is clean and pushed. The change is limited
to the SV1D activation runner's resource evidence. It monitors every simulator,
renderer, audit, and score stage under the existing bounded envelope and writes
one immutable stage record per stage. The successful result contains exactly
ten records (three arms times three stages plus scoring), with cgroup peak,
host/disk minima, swap and OOM deltas. The final resource manifest binds those
records to activation metadata, score, score-corpus manifest, and resource
policy hashes. Incomplete or out-of-envelope runs cannot produce a successful
resource manifest.

The clean post-commit `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test` passed, including
all package, integrated-long-run, R2, archive, and parity contracts. The
focused `go test ./cmd/sv1dprobe -count=1`, `bash -n
scripts/run-v2-r2-sv1d-activation.sh`, and `git diff --check` passed. The
pre-commit full run's sole failure was the expected dirty-tree archive/parity
guard; it is not treated as a green gate. Prior exact-tree vet, targeted race,
fresh-process determinism, and evidence-neutrality checks belong to the
preceding checkpoint and remain to be rerun or explicitly revalidated against
`aaf751e` before fresh review.

Adjudication: no R2 economic semantics, SV1D participant behavior,
configuration, historical evidence, or experiment result changed. The
performance red-team feed remains reviewed through `b1847ac`, with no newer
commit and no imported performance optimization. The host snapshot is about
33 GiB free disk, 27 GiB available RAM, and no swap; no binary capacity
attestation has been issued.

Promotion state remains closed: no fresh exact-tree Sol-xhigh review has yet
accepted `aaf751e`; no pinned Go 1.27 campaign build, capacity run, SV1D
activation, `dev-607`, `dev-613`, `dev-617`, parity control, freeze, or holdout
`619/631/641` has run. The R2 predecessor remains archived as
`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE` and is not being rescued.

Weekly quota pause / next controlled sequence:

1. Recheck exact Git state and fetch only performance commits newer than
   `b1847ac`.
2. Rerun final-tree vet, targeted race, fresh-process determinism, and binary
   evidence-contract checks.
3. Obtain one independent Sol-xhigh review of the complete exact tree.
4. If accepted, measure binary capacity with the registered resource contract,
   then build pinned Go 1.27 binaries.
5. Run only the development seed-659 SV1D activation probe; inspect its
   immutable tri-arm/resource manifests before any `dev-607` run.

No holdout may be read or consumed before explicit freeze authorization.
Historical evidence may be archived only after its own measurement contract
passes; no evidence deletion is required for this pause.

## Append-only mechanical-gate update: final-tree checks passed (`24f3ee1`) — 2026-09-12

The code-bearing source remains `aaf751e`; `24f3ee1` adds only this
append-only state record. On the exact final tree, bounded `go vet ./...`
passed, as did targeted `go test -race ./analysis ./cmd/sv1dprobe
./cmd/sv1dresource ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`.
The focused fresh-process checks for the baseline execution stream, binary
evidence determinism/log-mode neutrality, and perp-exposure evidence
determinism/log-mode neutrality also passed. The clean bounded `make test`
at `aaf751e` passed all package, integrated-long-run, R2, archive, and parity
contracts; shell syntax and `git diff --check` passed.

These checks add no economic or historical claim. The performance feed remains
at reviewed `b1847ac` with no newer commit. Resource state remains about 33
GiB free disk, 27 GiB available RAM, and zero swap. No capacity attestation,
campaign binary, SV1D activation, development cell, freeze, or holdout
execution has occurred.

The remaining promotion gate is one fresh independent Sol-xhigh review of the
complete exact tree, including R2 calendar semantics, correctness hardening,
binary evidence, strict scoring/provenance, and resource manifests. Review
acceptance is required before one measured binary-capacity run and the pinned
Go 1.27 build. The next scientific execution is the development-only seed-659
activation probe, not `dev-607`; its complete tri-arm and resource evidence
must be accepted before the registered development sequence. Holdouts
`619/631/641` remain untouched and unauthorized.

## Append-only quota-stop update: SV1D namespace lock hardening (`fe33dd8`) — 2026-09-12

Scientific HEAD is clean and pushed at `fe33dd8`. The checkpoint is operational
provenance hardening only: a separately testable Linux lock adapter now binds
the SV1D namespace lock to an opened regular-file descriptor, rejects final or
parent symlinks, preserves existing lock contents, and prevents concurrent
capacity/activation namespaces. Both runners retain the helper binary and
digest. The activation resource manifest additionally requires the exact
ordered arm/stage path set and corresponding stage labels. No economic or
historical claim was changed.

The bounded full test finished all Go package and ordinary contract suites. Its
exit was nonzero only because the source tree was intentionally dirty while
the parity/archive checks require a clean gate worktree; this does not replace
the deferred clean post-commit gate. Focused tests, shell syntax, and diff
hygiene passed. The weekly quota stop occurred before clean `make test`, vet,
race/fresh-process revalidation, independent review, capacity, activation, or
any development/holdout execution. Disk was approximately 33 GiB free, RAM
approximately 27 GiB available, and swap was absent at pause.

Promotion remains closed. The next controlled sequence is: clean final-tree
mechanical checks; performance-feed delta check after `b1847ac`; one fresh
independent Sol-xhigh review of R2 calendar semantics, correctness hardening,
binary evidence, strict scoring, resource manifests, and locking; then, only
on acceptance, one binary-capacity measurement, pinned Go 1.27 build, and the
development-only seed-659 SV1D activation probe. `dev-607`, `dev-613`,
`dev-617`, parity controls, freeze, and holdouts `619/631/641` remain
unauthorized.

## Append-only exact-tree audit update: binary evidence and lock findings closed (`fdbf077`) — 2026-09-12

The exact scientific HEAD is `fdbf077`, clean and pushed. The fresh mechanical
gate is complete after the prior Sol-xhigh review findings were independently
implemented and regression-tested. Required dictionary references now reject
zero, undefined, and empty values; optional values use explicit presence
bits with reserved reference zero. New schema revisions preserve legitimate
optional blanks in CDF decisions and global balance-change records. Index
loading rejects descriptor counts and body sizes before allocation. Renderer
ingestion is constant-time per record, while deferred validation remains
linear and safe for forged maximum sequence values. The private capacity
internal-arm path now requires the expected inherited descriptor and a
successful nonblocking `flock -n 3`, not merely an environment marker.

The exact-tree evidence consists of focused package tests, a bounded clean
`make test`, `go vet ./...`, the prescribed targeted race matrix plus changed
evidence-package race coverage, fresh-process baseline/binary/log-mode/perp
exposure determinism checks, shell syntax, and diff hygiene. All passed under
`GOMAXPROCS=2 GOMEMLIMIT=4GiB`; no OOM or swap event occurred. The performance
branch remains reviewed through `b1847ac` with no newer commit and no imported
performance code.

Adjudication remains operational, not scientific: R2 calendar/lifecycle
semantics, SV1D economics, configurations, historical results, and retained
evidence are unchanged. No capacity measurement, pinned Go 1.27 campaign
binary, activation probe, development cell, parity control, freeze, or holdout
was run. The next and only promotion gate is one fresh exact-tree independent
Sol-xhigh review. Acceptance may authorize the measured binary-capacity
preflight and pinned build, followed by the development-only seed-659 probe;
holdouts `619/631/641` remain unauthorized.

## Append-only exact-tree audit update: public-wrapper authorization finding closed (`86854e3`) — 2026-09-12

The fresh Sol-xhigh audit of `0c19cdf` found a real but not historically
activated defect in the binary-capacity control plane. The former pipe token
was forgeable by a caller of the public `sv1dresource` adapter when combined
with the environment marker and a caller-opened descriptor, allowing internal
arm setup before a later sentinel failure. This did not affect R2 or any
successor trajectory because the capacity and scientific execution gates had
not run.

The scientific branch corrected this without changing R2 calendar semantics,
SV1D economics, configs, evidence encoding, or historical artifacts. The
private arm now receives the held namespace lock as FD3 and independently
verifies the signed exact-tree review, canonical plan, review documents,
target config identity, pinned simulator/analyzer/renderer/runner identities,
and the actual resource-parent binary before it can create output. The
follow-up contract invokes an actual public wrapper with a valid lock and
current source/tree identities; it reaches the missing review identity check
and creates no arm directory. All eleven CDF v4 optional numeric fields have
both wire-level contradiction directions covered.

Exact remediation commits: `49dbcd4` (implementation and schema coverage),
`b33f089` (public-wrapper regression), and `86854e3` (review-gate reachability
inputs). Focused Go suites including the full `simulations/multivenue` package,
shell syntax, `git diff --check`, and the clean integrated-long-run contract
passed. The performance branch was fetched through `b1847ac` with no new
commits; no performance code was imported. A latent `RangeSelected` sequence
continuity issue remains deferred with indexed analytics and does not block
the current evstream path.

Promotion remains closed pending one fresh exact-tree Sol-xhigh review of the
current clean tree (including code checkpoint `86854e3`). Until acceptance, do not run binary capacity, pinned Go 1.27
binaries, seed-659 activation, `dev-607`, any development cell, freeze, or
holdout `619/631/641`.

## Append-only exact-tree review closure: `11da855` — 2026-09-12

The independent Sol-xhigh review of exact tree `81a6665` was a `REJECT` for
two reproducible promotion blockers. First, the capacity private arm could be
reached directly using `SV1D_LOCK_HELD=1` and a caller-opened expected lock
descriptor; the direct reproduction reached arm setup before a later fixture
failure. Second, CDF v4 optional numeric slots did not enforce canonical
presence/value agreement. `0d9c5e1` closes the latter with both wire-level
regressions. `11da855` closes the former with an opt-in one-shot pipe handoff
from `sv1dresource`, parent-PID/parent-executable validation, a production
handoff probe, and a control test rejecting forged direct entry before arm
creation.

The exact `11da855` tree passed the focused evidence/risk packages, clean
bounded `make test`, `go vet ./...`, targeted race matrices, fresh-process
determinism/evidence-neutrality checks, shell contracts, and diff hygiene.
The performance feed remains reviewed through `b1847ac` with no new commit.
No economic semantics, registered configuration, historical result, capacity
attestation, campaign binary, activation, development cell, freeze, or
holdout was consumed.

Promotion is intentionally paused at the next independent-review boundary.
The next required action is one fresh exact-tree Sol-xhigh review of R2
calendar/lifecycle, correctness hardening, binary evidence/rendering,
provenance/scoring, resource manifests, and both lock entrypoints. Acceptance
is required before capacity, pinned binaries, seed-659 activation, or `dev-607`;
holdouts `619/631/641` remain unauthorized.

## Append-only current-tree pointer after authorization remediation — 2026-09-12

The historical review-closure records above remain unchanged. The current
clean branch contains the authorization remediation and its public-wrapper
regressions (`49dbcd4`, `b33f089`, `86854e3`) plus later documentation-only
records. Resolve the exact current `HEAD` and tree at the next review launch;
the required review scope is the complete current tree, not a stale historical
hash. Promotion remains closed pending that review. No capacity, activation,
development cell, freeze, or holdout `619/631/641` has run.

## Append-only exact-tree sampler correction — 2026-09-12

The independent Sol-xhigh review of exact scientific tree `f258af9` returned
`REJECT` for reachable blocker `SV1D-RSRC-001`. A 250-ms ticker did not itself
bound timestamps recorded after filesystem/process/cgroup collection. The
reviewer and the scientific branch independently reproduced a three-second
non-campaign run with 15 samples and `maximum_sample_gap_nano=250894382`,
exceeding the strict `250000000` ns contract. This cannot affect historical
results: no binary-capacity arm, activation, development, freeze, or holdout
run had started.

The minimal correction is pushed as `5cd7b4d`. `SampleInterval` remains the
registered maximum observation gap and the validator still rejects any
violation. Actual timestamps remain post-collection; the sampler now schedules
fixed half-interval deadlines and rebases after an overrun, providing
measurement headroom without weakening the contract or changing economics.
The new real multi-tick strict-validation regression passed five repetitions;
the repaired three-second reproduction produced 26 samples with a maximum gap
of `125773682` ns.

Exact-tree mechanical evidence at `5cd7b4d`: clean bounded `make test`,
`go vet ./...`, targeted race tests, fresh-process determinism and binary
evidence/log-mode/perp-exposure neutrality checks, focused binary evidence
contract tests, and `git diff --check` all pass. The performance feed was
refetched through `b1847ac` with no newer commit; no performance implementation
was imported. No capacity, pinned Go 1.27 build, activation, development cell,
freeze, or holdout `619/631/641` has run.

The next boundary is one fresh independent exact-tree Sol-xhigh review of the
complete candidate. Until acceptance, capacity and seed-659 activation remain
unauthorized, as do `dev-607`, `dev-613`, `dev-617`, freeze, and all holdouts.
The detailed remediation record is
`research/v2-r5-sv1d-sampler-cadence-fix-2026-09-12.md`.
