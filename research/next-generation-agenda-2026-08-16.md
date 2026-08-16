# Next-generation simulation agenda

Purpose: stop running variants of the simplified ecology and make the internal
mechanics faithful to how exchanges describe their own systems. Grounding is in
`research/exchange-mechanics-reference-2026-08-16.md`.

Findings this agenda must stress-test rather than assume:

- **P (presence)**: the never-repricing ladder's extreme high-volatility edge was
  a monopoly on being the only resting depth, not information. It earned
  +23,212,665 per member against cancel-first makers and **-5,626,776** against
  continuously quoting ones. Its low-volatility edge (+191,447, rising to
  +240,824 against continuous makers) is genuine.
- **C (carrying capacity)**: at twenty times baseline fundamental volatility every
  maker class is insolvent, and worse when continuously present: perp maker
  -30.7M per member cancel-first against -57.3M continuous.
- **K (conservation)**: population sum = insurance fund payout - fees. The fund
  paid 5.9M in the high-volatility arm. "Sum equals fees" is only the
  no-bankruptcy special case.
- **Q (quoting discipline)**: submit-before-cancel repairs execution for every
  class but doubles maker-versus-maker crossings, 9.5% to 19.8% of trades.

## Ranked realism upgrades

Ranking is by expected impact on P and C, not by implementation cost.

### 1. Per-participant latency, replacing registration-order execution

Today intra-step ordering is `for _, a := range r.actors` in registration order
(simulation/runner.go:257), so execution quality depends on constructor position
in the scenario builder. Exchanges order by arrival time.

`simulation/` already provides `DelayedGateway` with separate request, response
and market-data providers, six providers including `LogNormalLatency` and
`HawkesLatency`, and an `EventScheduler` delivering at exact simulation
timestamps with per-channel FIFO. The multivenue scenario simply does not use it.
This is wiring plus a per-class latency configuration block, not new machinery.

Expected effect: **P is at risk of reversing.** The ladder never reprices, so it
is indifferent to request latency, while makers must round-trip observe, cancel
and replace. Adding realistic latency hands the ladder back part of the presence
advantage that continuous quoting removed. **C should strengthen**: makers who
learn about a fundamental move one latency later are picked off harder, so
insolvency should arrive at lower volatility.

### 2. Amend-with-priority as a first-class order operation

We have no amend. Cancel-then-submit loses priority (correct per documentation),
but submit-then-cancel gives the maker two live orders, which no exchange allows,
and is why crossings doubled. Documented semantics: amend keeps time priority on
a size decrease, loses it on a size increase or any price change.

Expected effect: **Q changes materially and P weakens.** A maker that can amend
in place is continuously present *without* the overlap window, which is the
regime our submit-before-cancel arm was standing in for. This is the single
upgrade that most directly tests whether the presence monopoly is an artifact of
our two crude quoting modes.

### 3. Maintenance-margin tiers, partial liquidation, and ADL

Liquidation is currently an all-or-nothing force close (exchange/liquidation.go:17)
and the insurance fund may go arbitrarily negative; ours reached -5.9M. Documented
venues use tiered maintenance margin, partial liquidation, and auto-deleveraging
that reduces opposing positions ranked by leveraged profit once the fund is
exhausted.

Expected effect: **C shifts and becomes better defined.** The boundary is
currently "makers go bankrupt and an unbounded fund absorbs it". With ADL the
boundary becomes "makers go bankrupt and the winners are forcibly deleveraged",
which caps the winning classes' payoffs — precisely the classes that showed
+42.9M per member. Expect the high-volatility payoff table to compress sharply on
both tails.

### 4. Price banding and limit up/down

Nothing currently stops a book printing arbitrarily far from the index. Documented
engines reject orders outside a dynamic band around the last price.

Expected effect: **C strengthens and becomes measurable in a new way.** Bands
convert unbounded adverse selection into rejected orders and halted trading, which
is how real venues survive the volatility our arms use. Likely to raise the
volatility at which makers become insolvent.

### 5. Funding from an impact-price premium index

Ours is `SimpleFundingCalc{BaseRate, Damping, MaxRate}`. Documented funding uses
impact bid/ask at a configured impact margin notional, time-weighted across the
interval, plus an interest term and a clamp.

Expected effect: **leaves P and C largely unchanged**, but makes the perpetual
maker's loss attributable, since funding is currently a function of our own skew
parameter rather than of book depth.

### 6. Configurable self-trade prevention

`cancelOwnCrossingQuotes` hardcodes `EXPIRE_MAKER`. Documented modes include
`EXPIRE_TAKER`, `EXPIRE_BOTH`, `NONE`, `DECREMENT`, `TRANSFER`, scoped by account
group.

Expected effect: **small for P and C, but it is a confound in the crossing
measurement.** Our maker-versus-maker crossing rate is measured under one
hardcoded STP rule.

### 7. Matching-algorithm composition (TOP, LMM, FIFO/pro-rata split)

We have whole-algorithm selection only. CME composes steps with configured
percentages.

Expected effect: **leaves P and C unchanged**; matters for queue-position
research, which we are not yet doing.

## Prerequisite before Config A: a slippage policy on every crossing actor

`RoundTripTrader.cross` submits an IOC limit at exactly the cached touch
(roundtrip.go:148-149) with no allowance. It fills 4.2% of the time *in a fully
repaired book with no latency at all*. The same pattern appears in
valuetrader.go:122/128/149/158, supplier.go:136 and metaorder.go:306. Only the
carry desks and the maker hedge cross with a bound.

Adding latency to actors that price against a touch they can no longer reach will
drive those classes to zero fills while makers appear to thrive — which reads as a
maker-favourable ecology result and is not one. This is the same shape as FFA-28
(1,218 silent off-tick rejects) and FFA-57 (accepted, never filled). Tracked as
FFA-70 with its own falsifier.

## Proposed configurations

Each is a 12h run at seed 91 with replicate seeds to follow, and each must satisfy
the telemetry contract below.

### Config A — "latency-real": does the presence monopoly survive arrival-time ordering?

Upgrades 1 and 2. Per-class latency: makers on a tight log-normal, takers wider,
the passive ladder on the same distribution as makers so it gets no free speed.
Makers quote by amend-with-priority rather than either crude mode.

Arms: {cancel-first, submit-before-cancel, amend-keep-priority} x {no latency,
realistic latency}, at baseline volatility and at the volatility where C bit.

Pre-registered predictions, to be recorded before running:
- If P is robust, the ladder stays unprofitable against amend-quoting makers at
  high volatility even with latency.
- If P was an artifact of our two crude quoting modes, the ladder's high-volatility
  result returns toward positive under amend quoting, and the "presence monopoly"
  conclusion narrows to "cancel-first quoting specifically".
- Falsifier for the latency direction: if maker payoffs are unchanged by latency,
  then registration-order execution was not materially distorting the comparison.

### Config B — "risk-real": where is the carrying-capacity boundary with real risk machinery?

Upgrade 3, plus 4. A fundamental-volatility ladder of at least five points
bracketing the observed boundary, with maintenance-margin tiers, partial
liquidation, ADL, and price banding enabled.

Measures the boundary as three separate quantities, which the current runs
conflate: the volatility at which makers stop being *profitable*, at which they
become *insolvent*, and at which the insurance fund is *exhausted* and ADL fires.

Pre-registered prediction: banding and partial liquidation should raise the
insolvency volatility, while ADL should cap the winning classes near the fund
size rather than at +42.9M per member.

### Config C — "who is needed": which classes are necessary once the book is never empty

With execution repaired, several classes were only viable because the book was
broken, and others may now be redundant. Ablate one class at a time from the
repaired ecology and measure the effect on spread, depth, fill rates and the
payoff table, rather than on payoff alone.

This is the direct answer to "what new agent classes become necessary once the
book is no longer artificially empty", and it should be run *after* A, so the
ablations sit on realistic execution.

## Telemetry and conservation contract for all new runs

A run is invalid for analysis unless it emits all of the following.

1. **Venue ledgers** — already added: per-venue fee revenue and insurance fund.
   Required because population sum = fund payout - fees, not -fees.
2. **Conservation residual printed with every payoff table**, with the run rejected
   if the residual exceeds a stated fraction of fees. Current known residual is
   8.4% of fees with the cross-asset graph enabled and 0.12% without, tracked as
   FFA-69 and localised to the equity valuation layer, not the balance ledger.
3. **Asset-unit conservation check** — population delta per asset must equal fees
   collected in that asset. This held exactly in E-141 and is the strongest
   available guard against a value leak.
4. **Fill rate by class and by order style** (tools/fill_rates.py), because a class
   that cannot execute is indistinguishable from a class that chooses not to.
5. **Counterparty mix by trade count and by volume** (tools/counterparty_mix.py),
   because volume-weighted tape statistics were about 90% two makers crossing.
6. **Empty-book step fraction** (tools/empty_book_steps.py) as a regression guard:
   any run above a few percent has a quoting defect, not an ecology result.
7. **New: ADL and liquidation events** — count, notional, and which classes were
   deleveraged, once upgrade 3 lands.
8. **New: rejected-order counts by reason**, including price-band rejections. The
   1,218 silently rejected hedge orders of FFA-28 and the accepted-but-never-filled
   orders of FFA-57 were both invisible in payoff tables.
