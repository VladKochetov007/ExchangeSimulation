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

## Facts already known to be compromised before measurement

Recorded here so that they are not presented later as findings.

| fact | why it cannot be read at face value |
|---|---|
| Anything involving CDF/USD or ABC/CDF | V-002: the second spot book rises forty-fold, driven by its own maker's inventory-skew loop |
| Cross-venue agreement of prices | V-004: enforced by a shared zero-latency index, with no cross-venue arbitrageur in the population |
| Option smile and skew | the value takers hold SABR views by construction, so a smile is partly an assumption travelling through the book. Section 10 of the plan exists to separate this and has not been run |
| Liquidation and leveraged tail behaviour | V-005: nothing is ever liquidated, so there is nothing to measure |
| Dated-ladder liquidity | flow was added to that ladder during the liveness campaign precisely because it was starved (L-006), so its activity is not evidence about the market |
