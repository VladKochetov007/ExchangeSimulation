# V2-5 P5 — dated-future carry causal identification protocol

Status: **preregistered before P5 implementation, numeric choices, immutable
configs, preflight, or outcome inspection.** P4 is complete and **FALSIFIED**
at its registered basis endpoint; P5 neither rescues nor reinterprets it.

## Scope and legacy boundary

Question:

> Does permission for an exact-cost, participant-local cash-and-carry policy to
> submit ordinary spot/future legs cause pre-settlement dated basis to compress,
> relative to the identical policy running as a non-trading shadow observer?

The legacy `derivsim.CashCarryArb` is prohibited in P5. It treats zero prices as
unavailable, computes midpoints locally, applies a fixed edge optionally scaled
directly by `sqrt(time_to_expiry/tenor)`, gates on submitted intent rather than
reconstructed exposure, and does not preserve the required independent cost,
receipt, order, and lifecycle evidence. Its direct shrinking-edge rule cannot
validate economic maturity convergence.

P5 requires a new opt-in policy. It may reuse proven generic gateway,
participant-local receipt/frontier, exact arithmetic, canonical order,
position, accounting, and bounded passive-exit infrastructure, but it may not
silently inherit a perpetual funding term or the legacy desk's target rule.

## Economic identity without a target-price oracle

For a cash-settled future, a matched rich-future trade is:

```text
buy spot at executable ask
sell future at executable bid
hold both until the declared settlement event
sell the residual spot through ordinary execution after settlement
```

Ignoring costs, the future and spot terminal settlement reference cancels from
the matched pair. The locked gross spread is `future_bid - spot_ask`. The cheap
mirror locks `spot_bid - future_ask` before costs. This identity does not require
the participant to write, know, or impose the future settlement price.

Time to expiry enters only through named local economics: financing or asset
borrow over the exact remaining duration, balance-sheet occupation, declared
settlement/TWAP mismatch risk, margin/liquidation risk, latency/non-atomic leg
risk, fees, and minimum required return. A lower break-even absolute spread may
therefore arise as time-dependent costs expire. No explicit square-root,
linear, or other desired convergence envelope may be written into the observed
market price or admission threshold.

## Sole causal intervention

Both arms instantiate the same policy, receive the same declared public feeds,
compute the same candidates, and emit the same compact decision evidence.

- **A — shadow:** exact candidates and defer/eligibility reasons are evidenced,
  but the policy has no authority to change target inventory or submit orders.
- **B — active:** the otherwise identical policy may change its target and send
  ordinary non-atomic spot/future orders.

The sole economic serialization must be an explicit trade-permission field.
No population, capital, price belief, cost, size, liquidity, spread, maker,
demand, latency, clock, listing, expiry, settlement, or evidence field may
differ. Shadow evidence must be proven telemetry-neutral.

## Ordered identification chain

### 1. Delivered contract and market information

Every candidate joins to participant-local delivered spot and dated-future
books plus the exact public instrument announcement. Evidence must retain venue,
symbol, contract identity, listing/publication time, expiry, settlement policy,
book publication/sequence identities, scheduled/actual delivery, per-link
ordinal/digest, and decision frontier. `delivered_at <= decision_time` is
mandatory for every source. Missing, stale, duplicated, reordered, future, or
wrong-contract input fails this link.

The participant may use executable touch prices only. A true midpoint may be
reported as a diagnostic when both sides exist, but cannot replace the
executable locked spread. Numeric zero and negative prices remain present values
where the future contract permits them; availability is explicit.

### 2. Exact remaining term and settlement contract

The analyzer independently recomputes nanoseconds to expiry from the delivered
announcement and decision time. It verifies positive remaining term, the
declared cash-settlement source/window, instrument multiplier, price domain,
and a registered minimum time needed to admit and deliver both legs. An expired,
settled, wrong-multiplier, or horizon-censored contract is not a candidate.

### 3. Exact fully costed locked carry

For both rich and cheap directions the independent analyzer retains:

- executable gross locked spread;
- four expected taker fees unless a different execution role is explicitly
  preregistered;
- long-spot quote financing or short-spot asset borrow over exact remaining
  nanoseconds;
- balance-sheet capital charge;
- margin/liquidation-risk charge;
- latency/non-atomic-leg charge;
- settlement/TWAP mismatch and post-settlement spot-exit charge; and
- minimum required return.

Every term is exact rational/fixed-point arithmetic with checked/wide
intermediates. Actor-stored values are comparison targets only. No `abs(price)`
shortcut, hidden free borrow, actor-internal settlement estimate, omitted exit
fee, or direct time-dependent price edge is allowed.

### 4. Target and ordinary execution

When exact net carry crosses the registered hurdle, B may set matched signed
spot/future targets; A must report `SHADOW_ELIGIBLE` and remain at zero target.
B submits actual gateway orders, one leg at a time, with request latency,
ordinary admission, depth, fees, partial fills, rejection, and orphan exposure.
An accepted request is not a fill, a one-sided fill is not matched carry, and a
target is not execution.

The policy must not force the second leg, alter a book, reserve hidden
liquidity, bypass margin, synthesize a position, or erase an orphan. Every fill
must join one-to-one to canonical venue evidence and independently replayed
positions.

### 5. Settlement and terminal closure

Future expiry permanently disables trading in the contract. The canonical
settlement event closes the future exactly once. The remaining spot hedge is
real exposure until an ordinary admitted/fill-proven exit closes it. Missing
liquidity, delayed settlement, partial exit, cancellation, or deadline expiry
remain explicit residual states.

No future funding may occur after expiry; collateral may not release twice;
settlement price zero or negative remains a present value; conservation and
terminal positions must pass. Profitability remains secondary and is measured
only after proven closure.

### 6. Pre-settlement basis response

Only links 1–5 license causal basis scoring. The analyzer reconstructs spot and
future executable basis from canonical public books, aligned by the treatment's
first independently verified target/order intervention. The same contract,
venue, and timestamps are applied to A. B cannot choose a favorable contract or
clock after outcomes.

The numeric addendum must preregister a single primary statistic before runs.
It must exclude the mechanical terminal settlement print itself and distinguish:

- cross-sectional basis by exact time to expiry;
- within-contract pre-settlement basis change;
- quote-mediated versus carry-trade-mediated changes; and
- missing/one-sided/stale/undefined ratio observations.

A zero price is valid numeric evidence but makes conventional relative/bps
ratios undefined. Signed price differences remain reportable. No shorter window,
alternate bucket, selected venue, or selected maturity may replace a failed
primary endpoint.

## Required adversarial validation

Before an outcome run, mutations must catch at least:

- future/stale/duplicate/reordered book or announcement receipts;
- wrong contract, expiry, settlement source, multiplier, or time to expiry;
- zero/negative settlement changed to unavailable;
- reversed rich/cheap direction;
- omitted/doubled fee, borrow, financing, margin, leg, or settlement-risk term;
- forged actor exact arithmetic or unregistered rounding;
- explicit legacy `sqrt(TTE)` edge reintroduced;
- shadow target/order activity;
- active target without canonical request/admission/fill;
- accepted-but-unfilled or one-leg/orphan exposure counted as matched;
- fill after expiry, double settlement, or double collateral release;
- synthetic spot reset after settlement;
- basis derived from actor state, settlement print, or a future snapshot;
- denominator-zero bps recorded as numeric zero; and
- favorable basis movement upgrading an incomplete causal chain.

Instrumentation neutrality must hold across fresh processes and relevant
`GOMAXPROCS` settings. Compact evidence must not add scheduler events, consume
RNG, mutate participant-visible state, or change execution ordering.

## Classification

- **INVALID:** evidence, information, arithmetic, canonical chain, settlement,
  accounting, conservation, digest, or forbidden-oracle failure.
- **NOT EXERCISED:** no registered exact-cost candidate appears in either arm.
- **NOT IDENTIFIED:** any ordered link or measurable primary basis endpoint is
  missing in a required pair.
- **FALSIFIED AT ACTIVATION:** exact eligibility exists but B does not change
  target while A remains shadow-flat.
- **FALSIFIED AT EXECUTION:** target changes but registered matched ordinary
  spot/future exposure is not established.
- **SUPPORTED (screening):** every link passes and every required paired primary
  basis statistic has the preregistered convergence sign.
- **MIXED:** complete execution occurs but required paired basis directions
  disagree or only a subset is measurable.
- **FALSIFIED:** complete matched intervention occurs in all required pairs but
  no primary paired basis statistic has the convergence sign.

Secondary activity, residual, settlement, funding, profitability, and terminal
PnL fields cannot upgrade the primary verdict.

## Numeric gate

Before implementation or preflight, a separate immutable addendum and exact
configs must freeze: development/holdout seeds; horizon and listing cycle;
eligible maturity set; minimum remaining term; decision interval and phase;
market-data/request latency; capital, position cap, lot, and order minimum;
all cost and risk terms; shadow/active serialization; leg order; settlement
and residual-exit policy; analysis cutoff/censoring; event-study/basis statistic;
coverage thresholds; all populations/books/clocks; binary/config hashes; and
raw-evidence retention.

Numeric choices may not be selected from P5 outcome worlds. If no canonical
single cost/risk value exists, preregister an economically scaled ladder and
report every level rather than selecting a passing treatment afterward.
