# Avellaneda-Stoikov Control Audit, 2026-08-15

## Source Model

The linear maker is based on Avellaneda and Stoikov's 2008 limit-order-book
market-making model, *High-frequency trading in a limit order book*,
Quantitative Finance 8(3), 217-224, DOI
[10.1080/14697680701381228](https://doi.org/10.1080/14697680701381228).
The paper's useful control ingredients are a reservation price that penalizes
inventory and a spread that widens with risk/volatility and fill-distance
decay. A common public implementation reproduces those ingredients with an
exogenous diffusion/arrival simulator; see
[fedecaccia/avellaneda-stoikov](https://github.com/fedecaccia/avellaneda-stoikov).

## Implemented Control

[stoikov.go](/home/vlad/development/exchange_simulation/simulations/multivenue/stoikov.go)
quotes one bid and one ask from

```text
r = F - q * gamma * sigma^2 * tau
spread = gamma * sigma^2 * tau + 2/gamma * log(1 + gamma/kappa)
bid = r - spread/2
ask = r + spread/2
```

where `F` is the local reference-book midpoint, `q` is actual filled inventory,
`sigma^2` is an EWMA of realised reference-mid changes, `tau` is a configured
rolling inventory horizon, `gamma` is risk aversion, and `kappa` is the fill
decay control. Prices are rounded outward to the instrument tick and sent as
real exchange limit orders. The exchange's matching, queue priority, and
other agents determine fills; no synthetic Poisson fill is injected.

## What It Is And Is Not

- It is an Avellaneda-Stoikov-inspired *quote control* in an actual simulated
  order book. Inventory affects reservation price and variance widens spread.
- It is not a calibrated reproduction of the paper. The intensity scale and
  exponential fill curve are not estimated from generated or historical
  fill-distance observations. `kappa` is a scenario parameter.
- It uses a rolling horizon for perpetual quoting, not a finite terminal HJB
  horizon. It does not include adverse-selection alpha, maker rebates, queue
  position prediction, impact, or a dynamic volatility/arrival regime.
- The option dealer is not an Avellaneda-Stoikov option maker. It is a
  fixed-spread inventory-skew baseline with a delta hedge; calling the options
  experiment a Stoikov options result would be incorrect.

## Research Consequence

The current implementation is suitable for testing whether an inventory-aware
linear controller changes quote survival, fill distance, inventory tails, and
marked wealth relative to the project's prior linear-skew maker. That ablation
has not yet been accepted. It is not suitable for asserting optimality,
calibrated fill probabilities, equilibrium, or live-market profitability.

The next valid calibration experiment should estimate arrival/fill intensity by
distance from the observed midpoint, fit a documented curve, then report
out-of-sample quote survival, fill, inventory, markout, and marked-equity
metrics under common-random-number worlds. A lower inventory tail achieved by
withdrawing liquidity is a falsification, not a win.
