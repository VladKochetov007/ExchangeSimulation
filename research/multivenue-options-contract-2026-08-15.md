# Multi-Venue Options Research Contract - 2026-08-15

## Question and verification tier

Can a deterministic three-venue simulator distinguish genuine option-dealer
feedback, cross-venue fragmentation, and inventory/liquidity effects from its
own scheduling, accounting, and modelling assumptions?

This is **tier C** research until the simulated microstructure has been
calibrated against external data. Deterministic tests establish that a result
is a property of this model; they do not establish that the result holds in a
live market. No strategy is called profitable or an equilibrium unless it
survives repeated seeded runs, full accounting, and an explicit model ablation.

## Evidence consulted

- [CryptoMMStoikov.pdf](/home/vlad/Documents/Trading/CryptoMMStoikov.pdf)
  uses dynamic spread/reference-price rules and finds its optimised start and
  step spreads rise with realised volatility. It is useful calibration evidence
  for adaptive quoting, not a derivation of Avellaneda-Stoikov.
- [FX Options and Smile Risk.pdf](/home/vlad/Documents/Trading/FX_Options_and_Smile_Risk.pdf)
  distinguishes spot from maturity-matched forwards, delta conventions, and
  delta/vega hedging. It rules out silently labelling a spot-proxy Black-76
  delta as the true forward or portfolio spot delta.
- [CEX-DEX-arb.pdf](/home/vlad/Documents/Trading/CEX-DEX-arb.pdf) models an
  uncertain second leg and resulting residual inventory. It supports treating
  cross-venue arbitrage as a state machine with pre-funded venue-local
  inventory, acknowledgements, partial fills, and explicit failure cost.
- [Execution.pdf](/home/vlad/Documents/Trading/Execution.pdf) links impact,
  liquidity, order flow, and resilience. It motivates measuring liquidity and
  markouts alongside PnL rather than optimising trade count alone.
- [Avellaneda and Stoikov (2008)](https://people.orie.cornell.edu/sfs33/LimitOrderBook.pdf)
  provides the baseline reservation-price and fill-intensity assumptions.

## Current model status

The direct deterministic-phase path can operate several independent exchanges
on one simulated clock. A venue already supports spot, local spot-margin
borrowing, perpetuals, dated futures, European cash-settled options, and
listing/expiry. These are reusable building blocks, not yet a complete
three-venue economy.

The following observability work is now in the branch:

1. Dynamically listed futures/options use a symbol-tagged fallback stream
   (`derivatives.jsonl`) rather than disappearing because no static logger was
   registered before listing.
2. Black-76 exposes validated delta, gamma, vega, and theta sensitivities.
3. `cmd/derivsim` writes a full `greeks.json` profile and compact summary.
   It tags the current forward as `spot_mid_proxy` and its sample phase as
   `post_quote_pre_hedge_fill`.
4. The three-venue direct scenario now creates independent local spot/perp,
   dated-future, and hour/day option boards with local auto-borrow spot margin,
   deterministic A-S linear makers, option dealers, and seeded flow. Its logs
   wrap every record with `venue_id`.
5. Per-position Greek rows retain listing timestamp and expiry. The
   `cmd/greekreport` utility reports both rolling generation and remaining
   maturity buckets, avoiding a false short/long distinction when one ageing
   option contract spans both regimes.

This does **not** make the current option setup a volatility-surface study:
IV is flat/static, the forward is a zero-carry spot proxy, and a periodic actor
sample is not an exchange-owned pre-expiry snapshot.

## Hypotheses and status

| ID | Hypothesis | Status | Cheapest discriminant | Falsifier |
| --- | --- | --- | --- | --- |
| H-009 | Dynamic listings hide the derivative board, so any existing option/futures flow conclusion is measurement artefact. | Supported and fixed for symbol-tagged event capture. | List a future and call/put chain; assert their stream has symbols and snapshots. | A dynamically listed contract emits no symbol-tagged book event. |
| H-010 | With `OptionPBuy=0.8`, the baseline dealer becomes short gamma/vega and delta hedging reduces, but does not eliminate, reported delta. | Preliminary, model-conditioned. | Fixed-seed active 12-second run with profile output. | Sign or delta pattern fails under the same config/digest. |
| H-011 | A genuine Avellaneda-Stoikov maker improves inventory-tail control relative to the current linear-skew baseline once both see the same order flow. | Pending. | Pure quote formula + one-book common-random-number run. | Lower inventory tail is obtained only by lower quote survival/fill without cost-adjusted benefit. |
| H-012 | Three venue fragmentation changes option/linear risk only when routing has execution risk and venue-local collateral. | Pending. | Three direct venues, then an ordered-latency extension; record leg fills and residual delta. | Effect appears with no routed orders or disappears after correcting accounting. |
| H-013 | Short tenors concentrate gamma/theta while longer tenors concentrate vega when ATM notional/flow are matched. | Supported, model-conditioned. | Deterministic 48h remaining-maturity Greek report. | Opposite pattern after precision and forward normalization. |

## Reproduced smoke evidence

Command:

```bash
mkdir -p logs/research
GOMAXPROCS=14 go run ./cmd/derivsim \
  -config=research/derivsim-active.json -duration=12s \
  -logdir=logs/research/derivsim-greeks-smoke
```

The `greeks.json` SHA-256 was
`96600d4e498075efdb041ea64a8bc9f3b487b252c73404b9a633d90de337237b`.
Its 12 one-second profiles ended with seven non-zero option positions,
option delta `+0.1997106562`, hedge delta `-0.09971015`, net delta
`+0.1000005062`, gamma `-1.9801954906e-08`, and vega
`-3616795.4166` quote units per `1.00` volatility move.

Interpretation: the flow configuration created a short-gamma/short-vega dealer
book. The remaining delta is consistent with a finite hedge band and reporting
before the newly submitted hedge fills. This is not realised vega PnL: flat IV
makes realised model-volatility PnL zero by construction. It is also not an
expiry result because the 300-second listed tenor did not expire in 12 seconds.

## Implementation sequence and gates

1. **Risk data contract.** Immutable per-position rows now retain symbol,
   listing timestamp, expiry, strike, signed position and model inputs. Add
   venue/actor/client identity plus exchange-owned pre-expiry/post-settlement
   rows before using exact terminal short-tenor conclusions.
2. **Forward and surface boundary.** Add a cached phase-updated forward source
   keyed by underlying/expiry. Do not call a competing exchange under the
   venue lock. Add static, mean-reverting, jump, sticky-strike, and
   sticky-delta surface arms; only then attribute realised vega PnL.
3. **True A-S baseline.** For a linear instrument quote
   `r = F - q * risk_aversion * variance * horizon` and a spread based on
   risk aversion plus a calibrated fill-decay parameter. Estimate variance
   from a documented EWMA of normalised returns and fit/check fill distance
   rather than assuming Poisson intensity. Preserve the current linear skew as
   the control arm.
4. **Three independent venues.** Implemented with a shared deterministic clock
   but distinct exchanges, mounts, IDs, logs, and prefunded accounts. Each has
   spot, perp, rolling dated futures, hour/day option tenors, and local
   auto-borrow spot margin with explicit collateral factors and limits.
5. **Participants.** Add spot/linear A-S makers, option dealers, noise flow,
   and value flow per venue. Cross-venue routing is a later actor that keys all
   requests/orders by `(venue, ID)` and maintains a per-leg ledger; it never
   assumes atomic execution.
6. **Evaluation.** Use common random numbers and at least three deterministic
   seeds. Report terminal marked PnL, fees, borrow/interest, inventory tails,
   quote survival, fill distance, post-fill markouts, liquidation count,
   two-sided depth coverage, residual delta/gamma/vega, and full risk logs.

## Disqualifiers

- Any fixed-seed digest differs across `GOMAXPROCS=1` and `14`.
- A report omits venue, symbol, position, forward source, IV source, or a
  terminal risk state.
- A cross-venue strategy has a submitted-leg count but lacks a completed-fill
  ledger and terminal residual exposure.
- A claim of realised vega PnL is made while IV is static.
- An A-S comparison changes order-flow randomness, starting inventory,
  latency, or fee schedule between arms.
