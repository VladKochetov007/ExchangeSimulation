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
