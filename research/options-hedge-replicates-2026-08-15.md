# Three-Venue Option Hedge Replicates, 2026-08-15

## Question

Does a stateful spot delta hedge make a short-option dealer's risk profile more
stable than leaving its option delta unhedged in the same deterministic,
fragmented market ecology?

The treatment is `dealer_hedge_mode=on|off`. The seed is the independent
replication unit; each world contains three separately funded venues and they
must not be counted as 60 independent samples.

## Accepted Design

- 20 paired seeds, `42..61`; deterministic direct phase runtime; three local
  venues, no cross-venue routing, asset transfer, or shared collateral.
- Each venue has spot, perpetual, dated futures, five-strike European option
  chains, local auto-borrow spot margin, two Avellaneda-Stoikov-inspired spot
  makers, one such perpetual maker, a futures maker, a fixed-spread option
  dealer, noise flow, and option-buy-biased flow (`PBuy=0.65`).
- Six simulated hours, a five-hour short option tenor which settles within the
  run, a one-day long tenor which remains live, and five-minute exchange-owned
  Greek snapshots. IV is fixed at 80%.
- The dealer starts with no ABC. A spot hedge is accepted only through actual
  fills and may borrow ABC against USD collateral. The profile is sampled after
  a derivative mark update, before same-timestamp hedge fills; it is therefore
  a pre-hedge-fill risk observation, not a claim of instantaneous zero delta.
- Terminal marked equity uses the strict exchange mark hierarchy: wallet debt
  exactly once, futures entry-to-mark PnL, signed option market value, and no
  fallback to a stale last trade. Every accepted world has three terminal
  snapshots and at least 4,021 positive-time-to-expiry option-position rows.

Configuration:
[multivenue-hedge-expiry-6h-replicates-2026-08-15.json](/home/vlad/development/exchange_simulation/research/multivenue-hedge-expiry-6h-replicates-2026-08-15.json).
World summaries:
[JSONL](/home/vlad/development/exchange_simulation/research/artifacts/multivenue-hedge-expiry6h-20seeds-2026-08-15.jsonl).
Paired statistics:
[JSON](/home/vlad/development/exchange_simulation/research/artifacts/multivenue-hedge-expiry6h-20seeds-summary-2026-08-15.json).

## Results

All currency figures below are USD after the `100,000` quote precision is
applied. A negative difference means hedge-on is lower.

| Seed-paired world metric | Hedge on | Hedge off | On - off | 95% paired bootstrap CI | Direction |
| --- | ---: | ---: | ---: | ---: | --- |
| Mean absolute net delta, ABC | 0.1593 | 0.3245 | -0.1652 | [-0.2047, -0.1275] | on lower in 20/20; sign p=1.91e-6 |
| Mean peak absolute net delta, ABC | 1.0624 | 1.3435 | -0.2811 | [-0.3834, -0.1854] | on lower in 18/20; sign p=4.02e-4 |
| Mean absolute gamma | 3.818e-8 | 3.182e-8 | +6.355e-9 | [+3.592e-9, +9.532e-9] | on higher in 17/20 |
| Mean absolute vega | 6.25895e8 | 6.25825e8 | +6.985e4 | [+3.227e4, +1.164e5] | relative change about 0.011% |
| Mean marked-equity change | +$27,041.35 | +$27,271.06 | -$229.71 | [-$262.78, -$194.63] | on lower in 20/20 |
| Mean terminal maintenance | $85,838.34 | $85,827.72 | +$10.62 | [$5.19, $15.98] | on higher in 16/20 |

## Interpretation

The supported claim is narrow: in this ecology, filled spot hedges reduce both
average and peak *linear* ABC exposure. The delta reduction is large relative
to the unhedged level and survives every paired seed.

The hedge does not make the option book gamma- or vega-neutral. A spot trade
has no direct gamma or vega, but it changes future book states, fills, and the
dealer's option inventory. The observed gamma difference is therefore
endogenous treatment feedback, not evidence that spot hedging creates gamma.
Vega is effectively unchanged in economic magnitude because fixed IV removes a
realized-volatility channel; it remains a local Black-76 sensitivity.

The hedge-on arm gives up about $230 of marked equity change on a $650 million
initial dealer account. That is consistent with hedge transaction cost and
path-dependent fills, but it is not an estimate of a real-world risk-adjusted
Sharpe ratio. No preference can be inferred without an explicit utility,
capital charge, dynamic surface, and adverse-selection model.

The option-buy-biased flow leaves the dealer generally short convexity. In a
representative north-venue pre-expiry row, the dealer had net delta `+0.2411`,
gamma `-3.9655e-8`, and vega `-7.8594e8`; the reported hedge component was
`+0.0093` before the next hedge fill. The five-hour contracts settle during the
run, while day-scale contracts retain nonzero vega afterward. This is the
expected maturity separation under a static-IV Black-76 model, but not
realized vega PnL.

## Rejected Stress Result

An initial eight-hour, six-hour-tenor stress run rejected 2 of 20 hedge-on
worlds because `ABC/USD` became one-sided and strict terminal USD valuation had
no two-sided conversion mark. The simulator failed closed; those worlds were
not silently dropped into the accepted sample. This exposes a genuine finite
inventory/liquidity-policy limitation for a longer horizon. The accepted
six-hour configuration crosses a short expiry before that boundary and must
not be generalized to an indefinitely liquid market.

## Assumptions That Matter

- Static IV means vega is a local derivative of the configured model, not a
  realized exposure to an IV surface move.
- The option's stored underlying mark is a spot-mid forward proxy. There is no
  maturity-specific forward curve, rate, dividend, or cross-currency FX graph.
- The option dealer is a fixed-spread inventory-skew policy. Only the linear
  spot/perpetual makers use the Avellaneda-Stoikov-inspired controller.
- The two treatments share a starting seed but diverge after the first hedge
  because their orders interact with the ecology. The paired comparison is a
  causal treatment comparison, not a frozen-book counterfactual.
