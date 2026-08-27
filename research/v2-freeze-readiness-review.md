# V2 freeze-readiness review

Review cut: `0f7aedf` plus the preceding P7d and V2-6 disposition commits on
`autoresearch/ffa-ecology-gen0`.  This is a freeze-readiness decision, not a
realism score and not a new simulation outcome.

## Decision

**NO — not yet ready to declare the immutable V2 freeze.**

The economic design is sufficiently settled to stop adding mechanisms, and
negative/inactive findings can be frozen as explicit limitations.  Three
pre-freeze obligations remain: (1) decide and integrate the evidence-transport
fail-closed correction, (2) create a clean-source freeze candidate with fresh
determinism and required mechanical/mutation gates, and (3) generate the final
calibration/untouched-holdout validation package only from that declared
freeze.  None requires economic tuning.

## Category scorecard

| category | status | basis and permitted claim |
|---|---|---|
| matching/mechanics | READY WITH EXPLICIT LIMITATION | Unit and integration fixtures cover price/time priority, post-only, IOC/FOK, signed prices and self-trade paths; run-level priority and option per-fill position evidence remain limited. |
| accounting/conservation | READY WITH EXPLICIT LIMITATION | Spot/derivative/funding/fee/settlement identities and mutation detectors are strong; a clean freeze rerun is still required. |
| price semantics | READY | Signed representation, availability, domain policies, zero settlement wire and positive-world equivalence are complete at `320262e`/`5afdd45`. |
| lifecycle | READY WITH EXPLICIT LIMITATION | Expiry/settlement and P3e passive exit are auditable; replenishment is `NOT EXERCISED`, not a lifecycle success. |
| information boundaries | READY WITH EXPLICIT LIMITATION | V2-0/1/2 receipt/frontier contracts and mutations pass for declared decisions; no-op/internal direct reads are not universal evidence. |
| cross-venue information | READY WITH EXPLICIT LIMITATION | Delayed local feeds and quote-mediated dispersion reduction are supported screening; no broad price-discovery claim. |
| cross-venue arbitrage | READY WITH EXPLICIT LIMITATION | Executable non-atomic router activation is supported; trade-mediated convergence was not identified in the registered smoke. |
| passive/inventory mechanics | READY WITH EXPLICIT LIMITATION | P0 admission, P1 size and P2 costly rebalance are screening-supported; actor-ordering is mixed and P3-R1 replenishment inactive. |
| economic demand/ecology | READY WITH EXPLICIT LIMITATION | Liability/directional motives activate and are auditable; broad activity-generator replacement remains untested and legacy noise flow remains. |
| perp funding | READY WITH EXPLICIT LIMITATION | P4 six-link participant response activates but the registered basis endpoint is falsified; this is not a general funding-null claim. |
| dated futures | READY WITH EXPLICIT LIMITATION | Dated instruments/lifecycle work mechanically; P5 exact-cost carry is `NOT EXERCISED`, so convergence is unresolved. |
| options | READY WITH EXPLICIT LIMITATION | P6-R1 O0–O4 viability/activation replicates; option emergence and hedge-direction causality are explicitly `NOT IDENTIFIED`, and SABR/VV structure is inherited. |
| distress | READY WITH EXPLICIT LIMITATION | P7d per-seed participant-risk replay is valid (439 mixed, 443/449 supported screening); no aggregate rule, participant-scoped deficit/insurance or bankruptcy contract. |
| timing robustness | READY WITH EXPLICIT LIMITATION | Phase controls exist; L1-P3 is mixed across 107/109/113 and is not promoted. |
| mutation coverage | READY WITH EXPLICIT LIMITATION | Many high-value accounting, funding, exercise, expiry, latency and fill-path mutations are caught; GTC cancellation, broad cross-margin, option per-fill and no-op evidence classes remain not tested. |
| determinism | NOT READY | Historical fresh-process/GOMAXPROCS gates pass, but a clean current freeze rebuild and fresh hash attestation are outstanding. |
| performance | READY WITH EXPLICIT LIMITATION | Measured hotspots and an analyzer prefilter optimization are recorded; the simulator/analyzer must be reprofiled from the exact freeze candidate. |
| holdout integrity | READY WITH EXPLICIT LIMITATION | P6-R1 and P7d packages retain complete evidence; P7d aggregate is `NOT IDENTIFIED`, and final realism holdout seeds/horizon must be pinned before consumption. |
| evidence durability | NOT READY | Logger write/flush/close failures are currently ignored on the main branch. The isolated fail-closed correction is under review and should enter the freeze candidate before final validation. |

## What may remain explicit limitations

The following are scientifically valid negative/inactive outcomes rather than
reasons to retune before freezing: P4's falsified basis endpoint; P4b's
execution failure; inactive P5 dated carry; unlicensed trade-mediated router
convergence; broad noise-flow replacement; mixed L1-P3 timing; P3-R1
replenishment; option emergence/hedge direction; and participant-scoped
bankruptcy.  The final autopsy must state them and must not relabel them as
successes.

## Pre-freeze blockers

1. **Evidence transport contract.**  Review the isolated
   `autoresearch/perf-evidence-integrity` patch.  If it passes differential and
   race checks, integrate it as a correctness/provenance commit; it changes no
   successful-run economics but prevents a failed flush from emitting a false
   evidence hash or completion sentinel.  If rejected, document an equivalent
   fail-closed correction before freeze.
2. **Clean candidate and gates.**  Build `multivenue`, `mvanalyze` and
   `prunegate` from a clean tree; record toolchain, flags, binary/config hashes,
   source/analyzer revisions and evidence schema.  Re-run fresh-process exact
   determinism across the declared GOMAXPROCS values and the mechanical,
   accounting, lifecycle, price, receipt/frontier and high-value mutation
   gates.  Any semantic divergence creates a new candidate rather than being
   papered over.
3. **Final validation partition.**  Freeze first, then pin untouched seeds and
   horizons for the full stylized-fact/ecology scoreboard.  Preserve raw
   evidence until each measurement contract and prune gate passes.  Do not
   call calibration/development observations holdout support.

## Review method and claim boundary

The decision was checked against `research/V2-CURRENT-STATE-AUDIT.md`,
`research/v2-design.md`, all P4–P7 result/preregistration files, the signed
price audit, mutation ledger, performance profiles, and the current P7d/P6
machine artifacts.  Configured Sol-xhigh review agents were unavailable due to
the model-usage limit; this is a fresh primary-agent red-team review and is not
an independent review.  No simulator or holdout world was launched for this
document.

The next licensed action is evidence-durability disposition and clean freeze
preparation—not another funding, timing, options, distress, or ecology tuning
experiment.

## Amendment after evidence-transport correction (2026-08-27)

The original decision above predates the evidence commits.  The fail-closed
transport correction is now present at `6f988a3`, `eee70d4`, `ba96597`, and
`2068d9d`, with review `research/reviews/v2-8-evidence-durability-review.md`.
Logger/checkpoint/receipt/frontier failures are latched; compact sidecars are
atomically published; `Sim.Close` retains its first result; and
`NewSim` rejects reused non-empty evidence directories (while allowing only
pre-run `run-config.json`/`run-metadata.json`).

Evidence durability is therefore **READY WITH EXPLICIT LIMITATION** for the
fresh-directory, zero-exit, newly validated sidecar contract.  The contract
does not claim crash-durable streaming JSONL, directory `fsync`, or recovery
of a reused directory.  The clean candidate gate must use fresh directories
and validate `greeks.json`, `latency.json`, runtime/offline evidence digests,
and analyzer metadata before scoring.

The remaining blockers are consequently (1) a clean-source candidate with
fresh-process exact determinism and mechanical/mutation gates, and (2) final
validation generated only after that candidate is declared immutable.  No
economic redesign is licensed by this amendment.
