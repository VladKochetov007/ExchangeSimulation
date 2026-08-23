# Blinded stylized-fact scoreboard

The plan requires the list to be written down before the measurements are
taken, and the frozen population not to be changed in response to any of them.
This file is the list. Measurements are appended underneath as they are made,
and the frozen configuration is not touched either way.

**Frozen baseline.** Commit `01c9ceb11aa1`, config
`research/configs/frozen-baseline-2026-08-21.json`, 24 simulated hours, seeds
101, 102 and 103, every run pinned to one core (V-008).

**Rule.** If a fact comes out badly, it is recorded badly. Adjusting the
population to improve it converts validation into calibration, and any such
change belongs to a separate, separately named version.

## The facts to be measured

### Returns
Distribution shape, tail index, skewness, excess kurtosis, autocorrelation of
raw returns, autocorrelation of absolute and squared returns, volatility
clustering, and how each behaves as the sampling horizon is aggregated.

### Order flow
Sign autocorrelation and its decay, order-size distribution, interarrival
times, cancellation rate, and clustering of trades and of volume.

### Limit order book
Spread distribution and its relation to volatility, depth by distance from the
mid, book imbalance, queue depletion, fill probability and time to fill,
resilience after an aggressive trade, and touch replenishment.

### Market impact
Mean price response by traded size and by horizon; concavity; the split between
temporary and permanent; reversion; the mass of trades with no impact at all;
impact conditional on liquidity regime. The mechanical-versus-revision
decomposition already has a history of censoring and horizon artefacts in this
project and is treated as suspect until re-derived.

### Cross-venue
Price dispersion, return correlation, lead-lag, arbitrage duration, price
discovery share, and shock transmission. To be read against V-004: the venues
share a zero-latency index, so any co-movement result is partly mechanical.

### Perpetual
Premium and basis distribution, basis autocorrelation and half-life, funding
against premium, the response to a funding payment, open interest, liquidation
count, and spot-versus-perpetual price discovery.

### Dated futures
Basis against time to expiry, convergence, roll behaviour, migration of volume
and open interest between contracts, calendar structure, and dislocation at
expiry. There is no explicit convergence force in the model; if convergence
appears, the trading mechanism producing it must be named.

### Options
Implied volatility computed from quoted and traded prices only, never from any
participant's internal model: IV against strike, skew and smile, term
structure, parity residuals, liquidity against moneyness and maturity, expiry
effects, the underlying's response to hedging flow, and the relation between
dealer Greek exposure and underlying volatility.

## Measurements

Not yet taken. The measurement pass is blocked on the deterministic re-runs of
the frozen baseline, which are queued.

### ae13f9a frozen measurement — first complete baseline scoreboard

The preceding “not yet taken” status is historical. The untouched frozen
simulator is `ae13f9aa6e5fd23539637a8c4a3d2d4f4c3ad107`, config
`research/configs/frozen-baseline-2026-08-22.json`, 24 simulated hours, seeds
101/102/103. Raw logs remain retained. The exact persisted-evidence artifacts
and all raw metric JSON are under `research/artifacts/scoreboard/f2_baseline_*`
and `research/artifacts/stylized-ae13f9a/`; this table is a score, not a
calibration target.

| family | result across seeds | verdict | interpretation |
|---|---|---|---|
| Raw 1-second returns | ACF(1) −0.584 to −0.598 on every venue | FAIL | Strong bid–ask/quote-clock reversal; not the near-zero raw-return ACF required of a credible liquid market. |
| Volatility dependence | 1-second absolute-return ACF(1) 0.296–0.374, but ACF(10) only 0.024–0.067 | INCONCLUSIVE | Short-lived dependence exists, but a 24h panel does not establish realistic long-memory clustering. |
| Heavy tails | Excess kurtosis −0.47 to 0.51; Hill tail index 33.5–52.8 | FAIL | The sampled return distribution is close to bounded/light-tailed, not convincingly heavy-tailed. |
| Trade-sign memory | ACF(1) 0.339–0.512; central/south ACF(50) 0.055–0.084 | PASS (descriptive) | Persistent signed flow is present; causal origin remains the programmed participant mix. |
| LOB continuity/depth | 86,400 snapshots per venue/day; median 5 levels/side, 34–36 tick spread, touch depth about 0.39–0.42bn base units | INCONCLUSIVE | Books are normally two-sided and multi-level, but fixed quote schedules and activity generators prevent a claim of emergent liquidity ecology. |
| Impact concavity | Pooled exponents vary from −0.77 to 0.70 and R² from 0.002 to 0.98; role-conditioned curves frequently reverse sign | FAIL | No stable concave impact law survives role conditioning; pooled apparent concavity is composition-sensitive. |
| Triangular/cross-venue dislocation | Raw executable triangular edge is profitable at ~99.7–99.8% of observations, mean 18,590–48,123 bps | FAIL | This is persistent severe incoherence, not transient arbitrage; shared maker reference does not repair the cross-instrument loop. |
| Perpetual basis | Mean absolute perp basis 250–282 bps, half-life thousands of seconds | FAIL | The perp remains materially displaced and highly persistent; the funding-off causal null is not evidence of healthy anchoring. |
| Dated futures convergence | Basis/convergence data exist, but frozen slope is not consistently economically convergent | FAIL | No basis-to-expiry convergence claim is supported. |
| Market-implied option surface | 36 fitted expiries and ~0.82–0.84m independently priced observations/seed; curvature 1,494–1,595 and dispersion 0.52–0.56 | INCONCLUSIVE | A surface exists in transaction prices, but the option-value-taker ablation shows its curvature materially inherits SABR beliefs. |
| Dealer hedge mechanics | Persisted Greek reconstruction finds option and hedge deltas nearly offset in baseline | PASS (mechanical) | This supports the implemented hedge path, not a claim that hedging feedback is realistic. |
| Ecology | 258 accounts persist, but terminal wealth/HHI and role returns vary sharply with the runaway regime | FAIL | Continuous activity is not a stable, economically credible ecology. |

The next audit steps are deliberately not parameter changes: clock perturbation,
remaining mutation tests, and a complete frozen-autopsy synthesis. In
particular, the cross-venue and option rows must be read with the predeclared
information and SABR-prior caveats rather than promoted to emergence.

## Facts already known to be compromised before measurement

Recorded here so that they are not presented later as findings.

| fact | why it cannot be read at face value |
|---|---|
| Anything involving CDF/USD or ABC/CDF | V-002: the second spot book rises forty-fold, driven by its own maker's inventory-skew loop |
| Cross-venue agreement of prices | V-004: enforced by a shared zero-latency index, with no cross-venue arbitrageur in the population |
| Option smile and skew | the value takers hold SABR views by construction, so a smile is partly an assumption travelling through the book. Section 10 of the plan exists to separate this and has not been run |
| Liquidation and leveraged tail behaviour | V-005: nothing is ever liquidated, so there is nothing to measure |
| Dated-ladder liquidity | flow was added to that ladder during the liveness campaign precisely because it was starved (L-006), so its activity is not evidence about the market |
