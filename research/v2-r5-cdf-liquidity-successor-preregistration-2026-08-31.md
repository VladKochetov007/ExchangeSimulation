# V2-R2-SV1 CDF-liquidity successor preregistration

Date: 2026-08-31
Candidate: `V2-R2-SV1`
Predecessor: R2 at scientific HEAD `230e78fd202c78ea34ac3f0857089a2344a7cebd`
Boundary: the predecessor is closed as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**.
This is a successor semantic amendment, not a rescue, rescore, or retuning of
R2.  The predecessor's code, configuration, evidence, and negative conclusion
remain historical controls.  The R2 calendar/lifecycle semantics and existing
correctness hardening are carried forward unchanged.

## Mechanism hypothesis

A separately configurable finite class of CDF/USD elastic liquidity suppliers
can reduce persistent one-sided CDF collapse often enough for the full ecology
to remain strictly valuatable.  The mechanism is an endogenous inventory and
risk response to delayed local market observations, not a price, spread,
volume, survival, or valuation target.

The eight existing ABC/USD elastic suppliers are unchanged.  The successor
adds a separate roster of CDF/USD suppliers, empty by default.  Historical R2
configurations therefore retain their exact participant population and
behavior.

Each new supplier will have, in the registered successor configuration:

- finite CDF and USD balances, no recapitalization path, and a finite absolute
  inventory limit of 1,000 CDF units;
- a maximum quote quantity of 0.5 CDF per decision and a two-second decision
  interval;
- a private reference initialized only as a declared starting belief at the
  bootstrap price and adapted with a four-hour half-life from eligible local
  observations; it is not a live external price feed;
- an inventory target bounded by the finite position limit and determined by
  the observed local price relative to that private belief;
- at most one resting limit order, on the inventory-reducing side, at an
  observed local touch; no spread objective and no obligation to quote either
  side;
- cancellation/repricing when the local observation is stale or unusable, the
  inventory target changes, the risk/position limit is reached, or the desired
  side is unavailable; no automatic replacement is required when a side
  disappears.

The actor receives only delayed local book snapshots through the normal market
interface.  It has no simulator-state access, global instantaneous oracle,
forced two-sided quote rule, forced fill, or forced replenishment rule.  Normal
exchange fills, fees, balances, inventory, and marks expose it to explicit
loss/PnL and risk.  The implementation must not add a hidden price anchor or a
survival callback.

The four-supplier roster per venue is a registered starting design, not a
parameter selected after observing the successor result.  It is deliberately
finite and small enough that concentration remains measurable.  The roster is
represented by a generic configurable policy/specification rather than a
hard-coded CDF branch.

## Activation criteria

The short activation probe must show, in the treatment world, all of the
following in retained evidence:

1. a supplier receives eligible delayed local observations and makes a
   decision;
2. at least one supplier order is accepted and at least one CDF fill changes
   its inventory;
3. the supplier bears observable trading/fee/PnL exposure, reconstructible
   from fills, balances, positions, and marked account evidence;
4. inventory changes alter the desired side or quantity in at least one
   decision;
5. at least one quote is cancelled/withdrawn or repriced because of a local
   observation, inventory/risk condition, or unavailable side;
6. the supplier is not forced to maintain a two-sided quote and does not
   receive capital or inventory replenishment.

The paired control uses the predecessor participant population and the same
development seed/horizon.  It is a diagnostic control for local activation,
not a new R2 claim.  The probe is development-only and precedes any full 24h
successor campaign.

## Anti-cheating and kill criteria

Reject the successor as a scientific mechanism if any of these are observed:

- CDF survival is primarily caused by a mechanically guaranteed quote, forced
  two-sided liquidity, forced replenishment, or an external/hidden price
  anchor;
- a supplier exceeds its declared finite inventory/capital/borrowing bounds,
  is silently recapitalized, or requires unbounded replenishment;
- the CDF book remains persistently one-sided, strict terminal valuation still
  fails, or the full ecology cannot remain valuatable;
- the new roster prescribes the market: its aggregate CDF volume share exceeds
  75%, or it supplies more than 75% of displayed CDF depth for more than half
  of measured active intervals in a venue;
- supplier activity is materially driven by simulator state rather than the
  delayed local interface.

The 75% concentration limits are preregistered diagnostics, not optimization
targets.  A pass also requires qualitative review of the raw decision and
order evidence; a numerical pass cannot override an anti-cheating failure.

## Required diagnostics

The successor extractor/report must preserve per-supplier and aggregate CDF
metrics for each venue:

- traded CDF volume share and resting displayed-depth share;
- inventory path, maximum absolute inventory, capital/borrow usage, and
  realized/unrealized PnL exposure;
- quote lifetime, quote count, cancellation/withdrawal and reprice frequency;
- time each CDF side is absent, both with and without the roster in the paired
  development control where the comparison is identifiable;
- local observation age, action reason, target inventory, side, price, and
  quantity for every supplier decision.

These are measurements, not objectives.  The report must distinguish observed
counterfactual evidence from inference and must fail closed on missing or
malformed supplier-decision records.

## Scope and decision boundary

Only development seeds are permitted during this successor activation stage;
holdouts 619/631/641 remain untouched.  No full 24h campaign is authorized by
this preregistration alone.  Before it, the candidate requires focused and
full tests, evidence-contract/determinism checks, and one fresh independent
Sol-xhigh review of the complete successor tree.

The successor may be promoted to a 24h development campaign only if the probe
passes the activation and anti-cheating criteria and the independent review
accepts the exact implementation.  A failed probe closes `V2-R2-SV1` without
rewriting the R2 negative control.
