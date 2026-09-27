# ME-016 — displayed-imbalance leaning in repeated maker competition

Status: **REVIEWED DEVELOPMENT RESULT; POLICY SHIFT OBSERVED; NO CONSISTENT POSITIVE PUBLIC-AVAILABILITY RESPONSE; MAKER GAIN NOT IDENTIFIED.** This result uses the [locked protocol](protocol.md) and exactly its 12 assigned economic worlds plus one preregistered technical repeat. It is not a continuation of ME-015's simulator trajectory, a confirmation study, or empirical validation.

## Question and registered contrast

Two of four finite ABC/USD spot makers use the same AS-style inventory-aware quote rule. Both gain-zero and gain-two versions receive the same delayed public best-level feed. At each eligible decision, the gain-two policy shifts its bid and ask together in the direction of displayed bid–ask depth imbalance; gain zero computes the same observations but ignores that shift. The other two makers retain their pure fixed-distance policy. The rule preserves its own half-spread, but matching against competitors can change public quotes, executions and inventory. Displayed depth includes the focal maker's own orders. The signal is therefore a potentially self-referential policy input, **not** an externally verified predictor.

The registered primary endpoint is `Q = public_two_sided_ns / 2,700,000,000,000` in the exclusive `[600,3300)`-second window. Assignment starts at world start. M1 slots are `[pure, signal, pure, signal]`; the paired gain-two minus gain-zero Q is compared within each of three development seed labels. M2 mirrors the slots and is a sensitivity check. These are six distinct paired comparisons but **three seed labels, not six independent replications**. The finite 55-minute world includes a 10-minute warm-up without state reset. Maker budgets, 0.1-ABC quote cap, ordinary inventory and admission rules, demand roster, FIFO venue, 1-bp USD taker fee, 1-second directed transport and 5-second maker decision clock are held fixed. A seed-once book initializer does not replenish depth. There is no supplied post-initialization price path, forced market-survival backstop or capital reset.

## Provenance and validity

Simulator/config/protocol candidate C is `eff182e98ba1745c82904686a40de50426814219`, tree `496c9dac879d783ae3ff911ce3009cbb1c3603e8`, Go 1.27.0, schema `repeated-spot-opaque-v5`. The locked protocol SHA-256 is `bfd46f5bfd55f64b164516df58471b34e944ffd3fd286edae78ff98136b48808`. Pinned `mespotrun` SHA-256 is `0f39151a188b9805d5ec98525b7172f4dc1e6d0bb5af65df2b15c60fcc1b58e4`; pinned `mespotanalyze` is `f97921461b954d00b18bd6bd564d223f240b9d48d21996d108514974eb2d3691`. Plans bind each effective world, raw/typed plan, binaries and source; manifests bind canonical evidence and four information sidecars. The later Go-only surface code is `a0f457873389381e33677f7f826fe6abe197373a`, binary SHA-256 `c140ce7bb585b0ffcfc383dfc5468f616b93eda89696621b99921fc62f177051`. It reads retained results and hashes; it did **not** produce the trajectories or replace strict replay. The skill at lock is commit `608e815bbc3e7545b996f648b596bceed6d7b809`, tree `60b91e73f1000a285e003bed122648636a737b2d`.

All 12 assigned simulator processes and strict analyses completed with valid v5 replay, source-time owner attribution, exact-window order-derived/public best-depth and spread agreement, receipt ordering and venue/account checks. An independent [complete-result review](../../reviews/me016-result-20260927.md) rechecked the retained hashes and primary numerators; a separate reviewer accepted only the bounded economic interpretation. The first analyzer attempt ended with exit 1 and no result; the console identified a missing fresh output directory. Its failure resource record remains at the external root, followed by a successful analysis of the **same** raw cell, without rerunning the simulator. The independent reviewer noted that the retained resource record alone does not prove the missing directory was the sole failure cause. This is an operational attempt, not an economic observation.

All 13 simulator processes, including the one technical repeat, had completed resource records under a finite 8-GiB memory cgroup. Across the 12 **economic** cells the maximum run allocation was 61,689,856 bytes, maximum sampled run cgroup memory 107,474,944 bytes, and maximum sampled analysis cgroup memory 135,872,512 bytes; the technical repeat's run cgroup peak was 116,207,616 bytes, the maximum across all 13 simulator processes. The minimum recorded available disk exceeded 62 GB. No OOM event or swap use was observed. Launch commands specified `MemorySwapMax=0` and `CPUQuota=400%`, but the resource JSON does not independently attest those two hard settings. The technical repeat is byte-identical to the first run in evidence, sidecars, manifest and analyzed result; only its `GOMAXPROCS=1` value is explicit in the retained resource command. The first process's worker count is not independently attested by that record. Controls are reproducibility checks, not additional seed worlds.

The compact [machine surface](surface.json) (SHA-256 `5dc22f7016cf37b1f162d3dad6cb5ffb8377a8d06115462c31f096228def5919`) contains every cell's plan/result/manifest/evidence/resource hashes, exact integer window measures, each signal-maker funnel, source-snapshot ownership counts and four maker account/risk summaries. Raw streams and resources remain under `/home/vlad/e1-me016-candidate-eff182e9-8v0HQc`; this local external location is **not** an off-machine backup. No historical ME-015 or holdout artifact was overwritten.

## Development observations

The primary M1 contrast is mixed. Times are **seconds out of the same 2,700-second window**; differences are gain two minus gain zero, not confidence intervals.

| Seed | M1 gain 0 | M1 gain 2 | M1 difference | M2 difference (slot sensitivity) |
|---|---:|---:|---:|---:|
| 18101 | 1,095 | 1,089 | −6 | −9 |
| 18111 | 1,062 | 1,054 | −8 | −8 |
| 18117 | 1,131 | 1,136 | +5 | +5 |

M1's registered median is **−6 seconds** (−0.2222 percentage points), range **−8 to +5 seconds** (−0.2963 to +0.1852 points). There is no consistent positive availability improvement in these three development seed pairs; the positive third pair and tiny scale preclude a claim of zero effect or equivalence. M2's differences are −9, −8 and +5 seconds. The similar signs are a slot check, not independent confirmation. The [paired plot](figures/occupancy.png) is rendered from the machine surface by a [versioned visualization script](figures/plot_occupancy.py); its seed labels are categorical, not a time or parameter trend.

The signal was not dormant. Each world has 1,080 scheduled signal-maker decisions across the two AS-family makers. In M1, the common research-only gain-two shadow identifies 282–322 tick-resolved quote differences per gain-zero world and 268–324 per gain-two world; these are arm-specific post-assignment information states, **not** a matched denominator for causal conditioning. Actual shifted-target decisions are zero by design at gain zero and 268, 318 and 324 in the three gain-two M1 worlds. In those gain-two worlds, 184, 227 and 225 shadow-qualified decisions had a risk-feasible side, an actual place request and at least one accepted place; the stage counts are decision-level branches, so one decision can also contain a rejected request. The full per-maker, per-cell scheduled→fresh-depth→shadow→risk→send→admission→source-snapshot resting/best→fill funnel is in the surface. Counts at later stages do not turn a programmed policy shift into predictive alpha or prove that a particular order caused the whole-world Q difference.

The focal maker is part of its own displayed signal. For example, in the M1 gain-zero/18101 world, the signal maker at client 3 appeared alone at the public best bid in 184 and best ask in 239 of its 3,307 delivered source snapshots; client 5 did so in 374 and 348 of 3,305. These are counts of delivered snapshots, not fractions of all simulation time or independent observations. In every measured window the public best-depth integral is maker-contributed; the finite seed initializer contributes no measured best-level depth. Self-renewal of local references and own displayed imbalance remain plausible alternatives to information about external demand.

Book quality does not follow from two-sided time alone. The registered time-weighted best-depth and conditional-spread measures accompany Q. Mean best bid/ask depth below divides the exact base-atom×nanosecond integral by the **full 2,700-second window**; conditional spread divides its price-unit×nanosecond integral by **actual two-sided time**, so missing-side intervals are not silently discarded from Q. Values are rounded for reading; exact numerators and denominators are in the surface.

| Roster / seed | Best bid ABC, gain 0→2 | Best ask ABC, gain 0→2 | Conditional spread USD/ABC, gain 0→2 |
|---|---:|---:|---:|
| M1 / 18101 | 0.0514 → 0.0513 | 0.0826 → 0.0821 | 11.82 → 11.94 |
| M1 / 18111 | 0.0489 → 0.0493 | 0.0782 → 0.0786 | 11.75 → 12.02 |
| M1 / 18117 | 0.0506 → 0.0512 | 0.0770 → 0.0780 | 11.18 → 11.61 |
| M2 / 18101 | 0.0509 → 0.0507 | 0.0826 → 0.0821 | 11.84 → 11.95 |
| M2 / 18111 | 0.0490 → 0.0494 | 0.0785 → 0.0777 | 11.79 → 12.02 |
| M2 / 18117 | 0.0504 → 0.0510 | 0.0766 → 0.0771 | 11.15 → 11.61 |

Bid/ask best-depth changes are mixed; conditional spreads are wider with gain two in these cells. The maker's *programmed* half-spread is unchanged, so the book-level difference comes through endogenous ranking/execution/inventory, not a direct encoded wider half-spread. This is descriptive context with no registered spread-success threshold and no evidence that quotes at the best were economically valuable. All 48 maker accounts have fills and no recorded working-cap time or resource-denied quote decisions in the fixed window, but finite capital/inventory still apply; nonbinding constraints here are not proof of unlimited resources or strategy capacity.

**All 12 terminal public books are one-sided or empty.** Strict terminal midpoints and every maker's registered fee-net passive-endowment benchmark gain are unavailable. We therefore do not score profit, maker superiority, risk-adjusted return or capital capacity from cash alone, a stale local reference, or the initial seed price. The result is a valid availability experiment with a separate valuation limitation, not an invalid stream and not a completed market-survival test.

## Interpretation and boundaries

The supported assignment statement is exact but narrow: **in this fixed simulated M1 ecology, assigning 2-bp displayed-imbalance gain instead of zero from world start changed later public two-sided time by −6, −8 and +5 seconds across three paired development seeds.** The result does not support a robust positive-availability claim. A seed label couples some exogenous role streams, but endogenous paths and random-draw consumption can diverge after the policy changes; the contrast is a whole-world assignment, not a controlled single-decision effect. Neither thousands of fills nor M2's mirrored slots increase the seed replication count. No p-value, equivalence bound or real-market corridor was registered.

Policy implementation correctness, nonzero delivered signal opportunities, actual shifted targets, accepted/resting orders, a mixed market-level contrast, strict terminal-price missingness and external empirical validity are different facts. The v5 evidence supports the first five at their stated scope; it supplies no real-data comparison. The market may have no independent absolute price anchor after its finite initialization orders. The 55-minute run and intermittent two-sided quotes establish neither long-run stability nor a general high-frequency strategy result. This study does not reopen ME-015's bounded public-availability response or its missing maker gains, and it does not activate a new E0 economic rescue or holdout.

## Reproduction without new worlds

Use the clean C checkout `/home/vlad/ExchangeSimulation-e1-C-eff182e9`, the retained external root above, Go 1.27.0, and the pinned binary hashes. Each `plans/ME016-...-plan.json` names its exact registered cell; matching `cells/...-run` contains the manifest, canonical stream and four information sidecars, while `analysis/.../result.json` is the strict replay. The first cell's successful analysis resource is `resources/ME016-M1-g0-s18101-analysis-resource-retry.json`; the earlier failed attempt remains `...-analysis-resource.json`. To regenerate the compact surface from retained results, use the clean `a0f45787` `mesignalsurface` binary (SHA-256 above) and a **new** output path:

```bash
mesignalsurface -root /home/vlad/e1-me016-candidate-eff182e9-8v0HQc \
  -first-analysis-resource /home/vlad/e1-me016-candidate-eff182e9-8v0HQc/resources/ME016-M1-g0-s18101-analysis-resource-retry.json \
  -out NEW-ME016-surface.json
```

That command verifies all registered file digests, effective-plan/manifest/result linkage, finite resource records, run inventory and the technical repeat; it does not re-execute the simulator or independently replay each event. Re-running strict analysis requires the pinned C simulator/analyzer binaries, an unchanged clean C checkout and a fresh exclusive result path. The report's later documentation commit is not the simulator or analyzer identity. The figure is regenerated with the repository `.venv` Python, solely for visualization. No holdout, confirmation world or new economic study is part of this reproduction.

## Next question, not an automatic run

The first E1 screen shows a real quote-policy action but no directional public-availability improvement in three development seeds, and terminal maker gains remain unidentified. The next useful *mechanical* increment is the already planned [ME-014 two-venue spot/perpetual funding baseline](../ME-014/idea.md): independently verify finite accounts, funding clocks/payments, delayed information and multi-period ledger replay before any economic E2 cell. That work tests a distinct carry/funding mechanism; it does not rescue this signal result. A later signal-profitability study would require a prospectively defensible terminal executable valuation or actual closeout, not retrospective midpoint substitution. Confirmation and empirical comparison remain separately gated.
