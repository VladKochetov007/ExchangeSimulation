# ME-015 — finite local-reference quoting: development result

Status: **REVIEWED DEVELOPMENT RESULT; PUBLIC TWO-SIDED-TIME RESPONSE OBSERVED; MAKER-GAIN CONTRAST NOT IDENTIFIED.** This is a successor to the valid negative [ME-013 r1](../ME-013/report.md), not a rescore or confirmation of it. The 24 economic worlds and two technical duplicate processes exhaust the [prospectively locked ME-015 matrix](protocol.md). Neither historical holdouts nor a new-seed confirmation partition was used.

## Economic question and estimand

Four finite, post-only makers share one ABC/USD spot book with recurring finite demand. Their normal quote policy requires a two-sided delayed local book. ME-015 asks whether allowing each maker to use its own *previously delivered* two-sided midpoint for less than 15 simulated seconds when its latest local book is one-sided or empty changes subsequent public two-sided-book availability. The reference is a policy belief, not a venue mark or a promise to quote. The same treatment applies to pure and AS-style makers; ordinary cash, base, working-inventory, cancellation and admission limits remain in force. The 15-second lifetime was chosen prospectively from three 5-second maker decision intervals and the 1-second delivery lag, after observing r1 but before ME-015 outcomes.

The primary estimand is `Q_ON − Q_OFF` for the all-pure-maker roster P, where `Q` is the fraction of the exclusive `[10,55 min)` window with **both public bid and ask resting**. The assignment begins at world start. Thus this is an effect of *whole-world assignment* on a later window, not an isolated in-window rescue effect or a comparison of unchanged background trajectories. All-A (A) and mirrored mixed (M1/M2) rosters are secondary; they do not enlarge the number of primary seed replications. The registered fee-net passive-endowment maker-gain comparison requires a strict current public two-sided terminal midpoint; no cached reference may substitute.

## Identity, methods and validity

The immutable simulator/configuration candidate is `4cc34e402d2ad3fd359a3bb8fb928f36fe779b1d`, tree `845e5417fab0f67a566dae241acc10609a81028e`, Go 1.27.0, schema `repeated-spot-opaque-v4`. The locked [protocol](protocol.md) has SHA-256 `08f38eb6f1642bab459c83244187b7eb24946ea7ea1619f39d0ae85ea2ad60ad`; per-cell raw and typed effective-plan digests are bound by each run manifest. The source-pinned `mespotrun` SHA-256 is `c6cd4d8ef0c4b7e69bf0f62f02583cebd796608e0150b443c4cc59aec0534bf9`; strict `mespotanalyze` is `d20c5011af4d274fc1bc5f72ed4e2a57a1b385f82e8c59e4b7deaf7908774c51`; independent window `mespotdiagnose` is `c3727d5fce66fbfb400a8dbd70980d0dfc41b091eb00f0d5c8da6b05e2d984f2`. The skill version is commit `608e815bbc3e7545b996f648b596bceed6d7b809`, tree `60b91e73f1000a285e003bed122648636a737b2d`.

Each assignment ran for 55 simulated minutes with a fixed 10-minute warm-up and no state reset; quote size was 0.1 ABC. There were 2 reference modes × 4 maker rosters × development seeds 18101/18111/18117 = **24 distinct cells**. The first ON/P/18101 world was also the counted finite-resource preflight. Two fresh-process executions reproduced that cell as technical controls; they are not extra economic observations. Strict evidence replay, raw manifest/sidecar binding, independent resting-depth reconstruction, exact window counts and resource checks passed for every assigned cell. All 52 run/analysis resource records completed with exit zero under an 8-GiB no-swap cgroup and 400% CPU limit. The largest retained run allocation was 61,251,584 bytes; maximum run and analysis cgroup memory were 102,600,704 and 107,876,352 bytes. The retained external root, `/home/vlad/e0-me015-candidate-4cc34e40-f5DrAN`, is **not** an off-machine backup.

Both controls have byte-identical analyzed results, manifests, canonical streams and market-data sidecars to the first cell. Their labels say Go workers 1 and 4, but the retained resource command record does **not** independently record `GOMAXPROCS`; worker-count attribution is weaker than byte-equality attribution. No technical duplicate is treated as another seed.

The complete [machine surface](surface.json) (SHA-256 `3f8bf07e7dc119f373ca9e626a556dc523e7448d09c438179b0184835cd69024`) includes all 24 cells, exact integer-nanosecond numerators, result/diagnostic/manifest/evidence/resource digests, missing terminal marks and both controls. The [figure](figures/occupancy.png) is a visualization of that surface, not another analyzer.

## Results

Every OFF cell had **zero** public two-sided time in the 2,700-second observation window. ON times below are seconds out of 2,700; divide by 27 for percent. Each entry is one world, not an event-level replication.

| Roster, reference ON | Seed 18101 | Seed 18111 | Seed 18117 | Reference OFF, each seed |
|---|---:|---:|---:|---:|
| P — four pure makers, primary | 1,102 | 1,069 | 1,134 | 0 |
| A — four AS-style makers | 117 | 1,065 | 184 | 0 |
| M1 — P–A–P–A slots | 1,095 | 1,062 | 1,131 | 0 |
| M2 — A–P–A–P slots | 1,097 | 1,062 | 1,132 | 0 |

For P, paired `Q_ON − Q_OFF` values are **40.8148, 39.5926 and 42.0000 percentage points**. The registered median is **40.8148 points**, with observed range **39.5926–42.0000**. This is a three-seed development range, not a confidence interval or an empirical-market estimate. The A response is heterogeneous (4.33%, 39.44%, 6.81% ON); mixed rosters have similar occupancy to P in these three worlds, but are distinct compositions, not additional primary replicates. We do not infer P-versus-A economic superiority from occupancy.

The ON/P worlds contain 1,424/1,492/1,404 cached-reference decisions and 1,052/1,077/1,034 shadow-eligible placement evaluations, respectively; the longest selected cache age recorded in the full surface is 13 seconds, below the strict 15-second cutoff. These decisions show that the treatment was used under finite constraints. However, **all OFF cells have zero shadow-eligible decisions inside the window**: collapse had already occurred and their caches had expired. The observed ON–OFF contrast therefore does not identify the marginal effect of a particular *in-window* eligible fallback decision, nor a count of individual order-level restorations. The whole-world assignment can change the early trajectory and sustain later self-renewal. The diagnostic's eligible/waiting/evaluation counts do not alone reconstruct every send→admission→resting join, so they are not relabeled a complete intervention funnel.

Across the ON cells, simultaneous maker-only two-sided resting time equals public two-sided time, and maker-only bid/ask depth integrals equal public bid/ask depth integrals in the registered window. All measured window depth is maker-contributed. Their quotes can refresh their own delayed two-sided reference, a self-referential persistence mechanism. This is neither a hidden guaranteed-liquidity backstop nor independent evidence of absolute price discovery. First-to-last two-sided midpoint drift in P ON is zero in all three seeds, but that sparse, self-referential statistic cannot establish stationarity; mixed rosters drift downward in the same descriptive field. ON/P worlds had 5,879–5,998 full-world trades versus 16–39 in OFF/P, showing activity, not profit or independent statistical sample size.

**All 24 terminal books are one-sided or empty.** Strict terminal midpoints and the registered fee-net maker-gain numerator are unavailable in every cell. Both the treatment and control P-versus-A gain contrasts are `NOT_IDENTIFIED`, not zero or unfavorable. Cash balances, fees, fills and exposures can be reconstructed, but a cash-only or stale-reference replacement would answer a different, unregistered economic question. This second E0 design iteration improves *intermittent public book availability*; it still does not establish a priceable terminal comparison, long-run survival, a maker return–risk ranking, a capital-capacity frontier or a general market ecology.

## Interpretation, review and limits

The supported causal statement is narrow: in this fixed simulated ecology and three development seed pairs, enabling the finite maker-local reference from world start increased P's later public two-sided time by the measured 39.59–42.00 percentage points. This is a policy-package effect under matched starting conditions; treatment-induced endogenous paths may diverge. It is not a claim that reference use alone after minute 10 caused a specific book recovery, that another participant supplied independent depth, or that the resulting quotes were economically informative. There is no confirmation or empirical comparison.

The [scoped independent review record](../../reviews/me015-result-20260927.md) preserves separate pre-outcome and complete-result checks. The economic reviewer accepted only the bounded whole-world assignment interpretation and required explicit terminal missingness, OFF zero eligibility and self-renewal. The technical reviewer independently checked the complete machine surface against retained artifacts and accepted the integer endpoint and missing-gain treatment, with the worker-label qualification above. Neither review certifies external realism or creates a terminal price.

ME-013's book extinction and primary non-identification remain historical facts for its own source and seeds. ME-015 does not rewrite them. E0 has now conservatively used **53 of 120** authorized process attempts: 27 predecessor attempts, 24 ME-015 economic worlds and two ME-015 technical controls. This protocol adds no calibration arm and does not automatically authorize a third E0 economic redesign. E1/E2 can reuse tested mechanics; their integrated payoff claims cannot inherit priceability from this result. The next justified work is a *separate* prospective account of how a non-self-referential valuation or finite objective-driven counterparty could support a stable payoff comparison, while independently continuing unblocked E1/E2 mechanical tasks. No economic parameter is to be changed retrospectively to force a terminal mark.

## Reproduction without new worlds

The clean candidate worktree is `/home/vlad/ExchangeSimulation-e0-reference-C-4cc34e40`. The external root retains each `R2-{ON,OFF}-{P,A,M1,M2}-q10000000-sSEED-plan.json`, matching `*-run/run-manifest.json`, `evidence.evs`, four market-data sidecars, `analysis/*-result.json`, `analysis/*-liquidity-diagnostic-v4.json` and run/analysis resource records. The `R2-` cell-ID prefix identifies the local-reference experiment adapter; it does **not** change the closed historical R2/SV1D line. The first cell's raw evidence SHA-256 is `f3da0540f05691024a748c55cb339f41ec45efe569e21b995b5b080efc06a008`, with manifest SHA-256 `a2bed4c67c4c788c31db763628e7d743f2007dcfec2ff2fd66b0ba2f930a3256`. Full per-cell identities are in the machine surface.

From clean C, build `mespotanalyze`, `mespotdiagnose` and `mespotsurface` with Go 1.27.0 `-trimpath`; verify the pinned binary digests above. To re-analyze a retained world, use `mespotanalyze -repo <C checkout> -simulator <pinned mespotrun> -plan <retained cell plan> -run <retained cell run dir> -out <new exclusive result path>`. For the independent window diagnostic, use `mespotdiagnose -manifest <retained manifest> -result <accepted strict result> -evidence <retained evidence.evs> -output <new exclusive diagnostic path> -measurement-start-ns 600000000000 -measurement-end-ns 3300000000000 -tick-size 100000 -shadow-reference-max-age-ns 15000000000`. It checks the raw file digest and exact window reconciliation. To rebuild the aggregate from retained accepted results/diagnostics in a **new** path:

```bash
/home/vlad/e0-me015-candidate-4cc34e40-f5DrAN/tools/mespotsurface \
  -study ME-015 -root /home/vlad/e0-me015-candidate-4cc34e40-f5DrAN \
  -diagnostic-binary /home/vlad/e0-me015-candidate-4cc34e40-f5DrAN/tools/mespotdiagnose \
  -diagnostic-commit 4cc34e402d2ad3fd359a3bb8fb928f36fe779b1d \
  -out NEW-ME-015-surface.json
```

The surface tool validates identities and resources but does not itself reread every raw event; the strict analyzer and independent diagnostic perform raw reconstruction. Do not run `mespotrun` merely to reproduce this report. The post-run documentation commit and later integration into `main` are not the simulator identity.

## Claim ledger

| Claim | Type | Evidence | Limit |
|---|---|---|---|
| 24 assigned cells and two controls completed under C | Mechanical/evidence | Machine surface, retained manifests/resources, scoped technical review | Does not imply economic or empirical validity |
| P ON has 39.59–42.00-point more two-sided window time than OFF | Causal development assignment contrast | Three paired integer-nanosecond primary differences in surface | Whole-world assignment; only three development seeds |
| The finite local reference was selected in ON worlds | Policy activation | Decision and delivered-snapshot replay, shadow diagnostics | Not individual-order restoration or independent price discovery |
| Maker-contributed depth entirely accounts for ON public window depth | Descriptive mechanism | Independently reconstructed maker-only/public depth-time and depth integrals | Self-renewal possible; not an absolute anchor |
| P/A fee-net maker gain is unavailable in both modes | Registered economic estimand | All 24 strict terminal-mark statuses and preregistered missingness rule | No profitability, equivalence, capital capacity or confirmation claim |

Applicable modules: repeated spot execution and local information are exercised. Funding, derivatives, capital adaptation, restricted game payoff, empirical comparison and long-run stability are **N/A** here because this design neither varies nor identifies them.
