# Frozen autopsy: ae13f9a

## Scope and gate decision

This is the adversarial autopsy of frozen simulator commit
`ae13f9aa6e5fd23539637a8c4a3d2d4f4c3ad107`, not a calibration report. The
internal verification regime is Tier A (deterministic program plus executable
audits); claims about resemblance to real markets remain Tier C because the
ecology and its information boundary are model choices.

**Gate decision: COMPLETE WITH EXPLICIT LIMITS.** The three 24-hour baselines,
all eighteen paired 24-hour causal treatments, and both registered 24-hour
liquidation-stress worlds have complete retained measurement contracts. The
hardened prune gate classifies all 21 `f2_*` baseline/treatment worlds as
`SAFE_TO_PRUNE`, but no campaign evidence has been deleted. Remaining gaps are
recorded below as `NOT TESTED` or coverage-qualified—not silently promoted to
passes. This permits v2 design; it does not permit an ae13f9a realism claim.

The old [`validation-summary.json`](artifacts/validation-summary.json) names a
pre-ae13f9a freeze and is historical only. It is not a source for the findings
below.

## Exact provenance

The frozen configuration is
[`frozen-baseline-2026-08-22.json`](configs/frozen-baseline-2026-08-22.json)
(SHA-256 `ca933bf2244eec8e104d4313456bed386809703bb7a1179125b8d9255f1b1036`).
Every baseline used full persisted logs, a one-second step, and a 24-hour
horizon.

| seed | ordered execution observations | ordered execution hash | persisted evidence records | unordered evidence digest |
|---:|---:|---|---:|---|
| 101 | 108,220,950 | `5e9c827e…581cf56` | 105,650,553 | `74cf5e47…b57f16c8` |
| 102 | 113,618,719 | `79b7d3f6…9388edbe` | 111,048,322 | `d128250e…6a676a3` |
| 103 | 107,762,992 | `6b8b1571…05283334e` | 105,192,595 | `ab530d43…0d6178cb` |

`execution_stream_hash` is an ordered simulation-observation digest.
`evidence_artifact_hash` is an unordered digest over exactly the persisted
JSON records, because the physical log files do not preserve one global causal
ordering. They are intentionally different domains. V-012/V-017 established
the terminology, removed 2,570,397 duplicate telemetry observations from the
execution sink, and require the runtime and offline evidence-artifact digest to
agree. For seed 101, fresh 24-hour runs agree across three GOMAXPROCS=1 and
two GOMAXPROCS=4 processes; full logging preserved the same execution hash.
That resolves V-008 for the stated execution contract.

All 21 current baseline/treatment worlds pass `bin/prunegate -json` against
[`measurement-manifest.json`](measurement-manifest.json) and the ae13f9a
verdict artifact. This is a measurement-completeness certificate only; raw
evidence remains under `logs/f2_*`.

## What is mechanically supported

| claim | status and scope | independent evidence |
|---|---|---|
| Deterministic execution | **SUPPORTED** for the declared freeze/config/seed contract | V-008 fresh-process checkpoint reproductions; ordered execution hashes above. |
| Persisted-artifact identity | **SUPPORTED** | Runtime/offline evidence-artifact digests agree; V-012 separates this unordered domain from ordered execution. |
| Linear fill-to-position paths | **SUPPORTED** for perpetual and dated-future fills | Baseline seeds replay 1,139,566 / 1,153,404 / 1,134,326 matched paths with no missing, unexpected, or chain-failed transitions; V-023/V-028 mutations are caught. |
| Accounting and venue ledgers | **SUPPORTED, bounded by integer truncation** | Closed-system and venue-take reconstruction pass; accounting mutations (extra fee/settlement effects) are caught. Do not read this as an exact real-valued conservation proof. |
| Funding and exercise semantics | **SUPPORTED for exercised paths** | Direction, duplicate-funding, payoff, strike, multiplier, and call/put mutations are caught. |
| Contract expiry and delisting | **SUPPORTED for observable future/option books** | All baselines: 363 expired-and-settled contracts each, zero late fills, zero post-expiry snapshots, and zero nonempty post-expiry quotes. V-025/V-031 delayed-expiry mutant is caught. |
| Liquidation trigger arithmetic | **SUPPORTED only for the no-debt ABC-PERP subset** | V-026 replays 8,942/8,942 and 7,533/7,533 breach checks in stress seeds 101/103 using independent `math/big` arithmetic; stale-mark mutation is caught. |
| Courier information boundary | **SUPPORTED for scheduler-backed gateway delivery** | Negative-delay mutation is caught; zero-delay mutation emits exactly zero in retained telemetry instead of silently disappearing. |
| Matching price/time semantics | **SUPPORTED at matcher fixture level** | LIFO and price-skip mutations are caught. A run-level queue-order audit is not available. |

The mechanical verdict is deliberately narrower than “the whole exchange is
validated.” Option fills lack a persisted per-fill position transition;
cross-margin/option/FX/borrowed-collateral liquidation is excluded from the
independent replay; and GTC cancellation requests are not retained in the raw
evidence.

## Causal screen: what the frozen mechanisms actually did

Every row is a paired 24-hour comparison, seed 101 against 101 and 103 against
103. Two seeds are **screening evidence only**. Raw values, activations, and
per-component reasoning are in [`causal-ablations.md`](causal-ablations.md)
and [`ablation-verdicts.json`](artifacts/ablation-verdicts.json).

| arm | activation | result |
|---|---|---|
| own-mid-anchor | Makers changed from consensus to local reference | **SUPPORTED (screening):** maximum executable cross-venue edge widens 125.75x / 57.92x, passing the preregistered >10x rule. Quote-mediated common reference is the dominant frozen convergence channel. |
| basis-off | Carry and dated-carry actors absent | **MIXED:** perp width rises 250.53→291.96 / 256.95→295.22 bps and half-life slows; dated convergence was already anti-convergent and moves toward zero when removed, falsifying that component. |
| parity-off | Parity actor absent | **SUPPORTED (screening):** maximum raw parity residual rises 219.84→607.67 / 132.91→485.28 bps and residual runs lengthen. |
| triangle-off | Triangle actor absent | **FALSIFIED:** the preregistered null prediction fails; mean triangular dislocation rises 24,160→71,853 / 18,625→65,840 bps. |
| funding-off | 36 funding instants per control become zero | **FALSIFIED** as the marginal perp anchor here: every measured pooled basis field is bit-identical between treatment and control. Funding mechanics working does not imply funding identifies the frozen price. |
| delta-hedge-off | Hedge fills fall 171,097/175,929 to zero while option trading survives | **MIXED:** mean absolute dealer delta rises strongly, but option-to-underlying correlation and maximum delta disagree by seed. |
| vanna-volga-off | VV actor absent while option dealers still trade | **FALSIFIED:** dealer vega, vanna, and volga all shrink rather than rise; IV curvature weakens but survives. |
| option-value-takers-off | SABR-view actor absent while 1.00m/0.98m option trades survive | **SUPPORTED (screening):** market-price IV curvature falls 1,493.9→295.7 / 1,594.8→208.1 and dispersion falls, while the surface remains non-flat. The frozen surface is materially inherited from programmed beliefs, not wholly emergent. |
| latency-x10 | Weighted delivered delay rises 37.995→485.294 and 37.762→454.846 ms | **MIXED:** dispersion and adverse selection rise in both seeds; executable-opportunity lifetime moves in opposite directions. |

The supported own-mid result does not say observing other venues is illegitimate.
It says this implementation gives heterogeneous venues a shared,
zero-latency/common-reference channel whose strength dominates the frozen
world. The information boundary records a separate direct Vanna-Volga
dealer-inventory function call, so no claim of market-mediated VV risk transfer
is warranted.

## Registered liquidation stress (V-005)

The registered stress is **EXERCISED for forced closes**: seed 101 has 7,054
liquidations affecting 15 accounts; seed 103 has 7,177 affecting 16. Each
observed close passes the position-path and contract-conservation replay. No
deficit, bankruptcy, or insurance-fund absorption occurs in either seed.

Therefore:

- Forced-close mechanics and the V-026 trigger subset are supported.
- Deficit, insurance, and bankruptcy mechanisms are **NOT EXERCISED** under
  the registered population/stress and are not semantically validated.
- It would be post-hoc strengthening to change stress merely to make those
  paths fire. V2 must explain economic reachability before adding another
  stress regime.

Full provenance is in [`v005-stress-ae13f9a.json`](artifacts/v005-stress-ae13f9a.json).

## Frozen stylized-fact scoreboard

The scores below summarize the immutable three-seed baseline measurement in
[`stylized-facts-baseline.md`](stylized-facts-baseline.md). `PASS` means the
descriptive observable exists, not that it is causally endogenous or robust.

| domain | frozen result | score |
|---|---|---|
| Raw 1s returns | ACF(1) −0.584 to −0.598 across venues | **FAIL** |
| Volatility dependence | absolute-return ACF(1) 0.296–0.374 but ACF(10) only 0.024–0.067 | **INCONCLUSIVE** |
| Heavy tails | excess kurtosis −0.47 to 0.51; Hill index 33.5–52.8 | **FAIL** |
| Trade-sign memory | ACF(1) 0.339–0.512 | **PASS (descriptive)** |
| LOB continuity/depth | two-sided, multi-level books; fixed schedules/activity support remain | **INCONCLUSIVE** |
| Impact concavity | unstable sign/exponent after role conditioning | **FAIL** |
| Triangular/cross-instrument coherence | profitable ~99.7–99.8% of observations; mean edge 18,590–48,123 bps | **FAIL** |
| Perpetual basis | mean absolute basis 250–282 bps; half-life thousands of seconds | **FAIL** |
| Dated futures convergence | not consistently convergent into expiry | **FAIL** |
| Market-price IV surface | fitted price surface exists, but SABR taker ablation removes most curvature | **INCONCLUSIVE** |
| Dealer hedge mechanics | baseline exchange-owned delta reconstruction nearly offsets option delta | **PASS (mechanical)** |
| Ecology | persistent accounts but regime-sensitive wealth and activity-generator dependence | **FAIL** |

No clock variant repairs the strongly negative raw-return ACF or the enormous
triangular dislocations. Thus the failures are not merely a presentation choice
or one coarse resolution setting.

## Clock-artifact result

The initial five-hour two-seed screen and preregistered follow-up establish
**CLOCK-SENSITIVE (screening)**, not an individual clock mechanism. Holding a
100 ms step fixed, each broad cadence package crosses the declared basis
midpoint in both seeds. The narrow tests refute simple attribution: quote-only
passes only seed 101 (90.99 / 185.00 bps) while dated-carry-only passes only
seed 103 (116.27 / 76.28 bps). Changing intervals changes both frequency and
relative phase; the frozen configuration cannot apply a pure phase-offset
control without a new timing feature and freeze.

Consequently, no frozen economic effect depending on basis, opportunity rate,
or return timing is timing-robust. The defensible result is a coupled
cadence-lattice interaction with unresolved components, documented in
[`clock-artifacts-ae13f9a.md`](clock-artifacts-ae13f9a.md) and
[`clock-factor-ae13f9a.json`](artifacts/clock-factor-ae13f9a.json).

## Mutation coverage and open evidence limits

The mutation suite now catches: extra spot fees; unrecorded venue movements;
wrong funding sign and duplicate funding; call/put, strike and multiplier
settlement errors; LIFO and best-price matching faults; missing immediate
cancellation records; duplicate/omitted linear settlement; omitted persisted
linear fills; future information delivery; zero courier latency; delayed
contract expiry; and stale perpetual liquidation marks. Several controls pass
while the mutant fails, which is evidence about the validator rather than a
self-reported simulator event.

Still **NOT TESTED / not reconstructible from frozen evidence**:

- per-fill option position paths;
- individual GTC cancel request/state transitions;
- run-level queue priority wiring (as opposed to matcher fixtures);
- cross-margin portfolios with option marks, FX collateral, isolated margin,
  or borrowed balances;
- deficit, bankruptcy, and insurance-fund paths;
- every actor's historical inbox/decision information set.

These are limits on claims, not evidence that the code is wrong. See
[`mutation-suite.md`](mutation-suite.md),
[`information-boundary-audit.md`](information-boundary-audit.md), and
[`validation-audit.md`](validation-audit.md).

## Economic interpretation

What is economically coherent in the frozen ecology is local and qualified:
order books clear, price/time priority fixtures work, carry/parity actors can
move their measured residuals, dealer hedge accounting offsets marked delta,
and cross-venue information in maker quotes powerfully synchronizes prices.

What is not economically coherent as a market-level claim:

- the triangle is persistently and severely incoherent even though its desk
  restrains the error;
- funding is not identifiable as a marginal anchor;
- dated future convergence fails;
- return dynamics are dominated by strong negative one-second reversal;
- price-surface curvature is substantially injected by SABR-view participants;
- several actor classes exist principally to keep books active rather than to
  express a liability, value, or execution objective;
- the common maker index and VV inventory handoff violate the desired
  participant-specific information architecture.

The autopsy therefore rejects “the market stays alive” and “the chart has a
surface” as validation standards. The frozen simulator is mechanically useful
and causally informative about its own coded mechanisms; it is not a
scientifically defensible model of realistic fragmented-market ecology.

## Next phase

V2 design may now begin, but only as a new mechanism program rather than a
tuning loop. The required design ledger is [`v2-design.md`](v2-design.md),
with each change written as observed failure → local mechanism → predicted
observable → causal ablation → kill criterion. The initial priorities are
participant-specific delayed cross-venue feeds, non-privileged maker
references, explicit executable arbitrage, post-only passive requotes,
separate inventory rebalance, economically motivated demand, identifiable
funding/carry, and staged option beliefs.

## Machine-readable provenance

[`frozen-autopsy-ae13f9a.json`](artifacts/frozen-autopsy-ae13f9a.json) is the
machine-readable index. It links the freeze, baseline hashes, causal verdicts,
V-005, scoreboard, clock result, mutation limits, and source artifacts.
