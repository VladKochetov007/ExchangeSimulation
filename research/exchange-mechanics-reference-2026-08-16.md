# Publicly documented exchange mechanics, and what this simulator does instead

Sources are the exchanges' own documentation and public regulatory or educational
material. No proprietary statistics or order-flow data are used.

## 1. Order lifecycle, cancel/replace, and queue priority

**Documented behaviour.** Cancel-and-resend and in-place amendment are different
operations with different priority consequences. Cancelling an order and sending
a new one loses time priority: the new order executes behind existing orders at
the same price. An "amend keep priority" request modifies in place and retains
time priority. Priority is retained only for a *decrease* in size; an increase in
size or any price change loses priority.
(Binance `order_amend_keep_priority`; Crypto.com limit order amendment.)

**What we do.** `StoikovMarketMaker.requote` either cancels then submits
(`SubmitBeforeCancel=false`) or submits then cancels (`true`). There is no
amend primitive at all, so "keep priority" is unrepresentable, and our
submit-before-cancel arm gives the maker *two* live orders during the overlap —
which is not what an exchange amend does, and is the reason maker-versus-maker
crossings doubled from 9.5% to 19.8% of trades when we enabled it.

## 2. Self-trade prevention

**Documented behaviour.** STP is a per-order mode, not a global rule. Documented
modes: `EXPIRE_TAKER` (cancel the incoming remainder), `EXPIRE_MAKER` (cancel the
resting order, the common default), `EXPIRE_BOTH`, `NONE`, `DECREMENT` (reduce
both by the prevented quantity), and `TRANSFER`. Scope is the account or a shared
`tradeGroupId`, so sub-accounts of one entity are covered.
(Binance spot and USDS-margined futures STP FAQ.)

**What we do.** `cancelOwnCrossingQuotes` (exchange/order_handling.go:1455)
unconditionally cancels the *resting* side when a client would cross itself —
i.e. `EXPIRE_MAKER` hardcoded, not configurable, with no account-group concept.

## 3. Matching priority

**Documented behaviour.** CME Globex composes an algorithm from steps: LMM
allocation of a configured percentage, TOP order priority for the order that
bettered the market (one buy and one sell only, matched first regardless of
size), then a Split between FIFO and pro-rata whose percentages sum to 100.
(CME Globex matching algorithm documentation.)

**What we do.** `DefaultMatcher` (price-time) and `ProRataMatcher`, selected per
venue, with no TOP, no LMM allocation, and no FIFO/pro-rata split.

## 4. Latency and fairness

**Documented behaviour.** Exchanges deliberately shape latency. IEX applies a
symmetric 350 microsecond delay to all orders. TSX Alpha applied a randomised
1-3 millisecond delay to marketable orders only. ParFX and EBS introduced
randomised delays in FX in 2013. Asymmetric designs delay liquidity-removing
messages while letting cancels through, explicitly so makers can pull quotes
before being picked off.
(IEX/SEC filings, TSX Alpha, Coalition Greenwich, academic surveys.)

**What we do.** Venue-local participants have *no* latency at all: the manifest
records "direct deterministic mounts; no latency experiment is enabled". A full
latency stack already exists in `simulation/` — `DelayedGateway` with separate
request, response and market-data providers, and `ConstantLatency`,
`UniformRandomLatency`, `NormalLatency`, `LogNormalLatency`, `LoadScaledLatency`
and `HawkesLatency` — plus an `EventScheduler` that delivers at exact simulation
timestamps with per-channel FIFO. It is simply not wired into the multivenue
scenario. Cross-venue routers are the only consumers.

**The deeper issue.** Because there is no latency, intra-step ordering is decided
by *registration order*: `Runner.drainDeterministicPhases` pumps
`for _, a := range r.actors` (simulation/runner.go:257) in the order actors were
appended (`AddActor`, runner.go:99). In `simulations/multivenue/sim.go` makers are
registered first (line 874) and takers last (lines 893-911, 1399, 1450). A
participant's execution quality therefore depends on where its constructor sits
in the scenario builder. No exchange behaves this way; arrival time does this job.

## 5. Liquidation, insurance fund, auto-deleveraging

**Documented behaviour.** A tiered maintenance-margin schedule drives partial
liquidation; the insurance fund absorbs negative-equity shortfalls; when the fund
is exhausted, auto-deleveraging forcibly reduces the positions of opposing
traders ranked by leveraged profit (PnL ratio and effective leverage).
(Bybit, OKX, BitMEX, BingX ADL documentation.)

**What we do.** Bankruptcy zeroes the negative perp balance and charges the debt
to `InsuranceFund` (exchange/exchange.go:1965-1975). There is no maintenance-margin
tier schedule, no partial liquidation, and no ADL: the fund is allowed to go
arbitrarily negative. Our high-volatility runs drove it to -5.9 million, which on
a real venue would have triggered ADL long before.

## 6. Funding rate

**Documented behaviour.** Funding = average premium index + clamp(interest rate -
premium index, +/-0.05%), where the premium index uses *impact* bid/ask prices
computed from an impact margin notional, time-weighted over the interval (5,760
samples per 8 hours). The result is capped by a maintenance-margin-derived bound,
and when the cap binds, settlement can switch to hourly.
(Binance futures funding documentation.)

**What we do.** `SimpleFundingCalc{BaseRate, Damping, MaxRate}` with a configurable
interval and cap. No impact-notional depth sampling, no time-weighted average over
the interval, no interest-rate term, no cap-triggered interval change.

## 7. Mark price, index construction, price protection

**Documented behaviour.** The index is a weighted average across multiple
constituent venues; the mark price is a protected derivation of it, and matching
engines apply price banding that rejects orders outside a dynamic band around the
last price, with limit up/down halting trading outside the band.
(Binance mark/index documentation; CME price limits and banding.)

**What we do.** `spotIndexProvider` publishes either own mid, a consensus of
venue mids, or the exogenous fundamental — the last of which is a zero-lag,
noise-free channel to true value. `degraded_index` now adds lag and observation
noise. There is no price banding and no limit up/down, so nothing stops a book
printing arbitrarily far from the index.

## 8. Fees

**Documented behaviour.** Tiered maker/taker schedules by 30-day volume, with
maker rebates at the top tiers on many venues.

**What we do.** `PercentageFee{MakerBps, TakerBps}` flat per participant class,
no tiers and no rebates. Our makers pay 0 and takers 5 bps throughout.
