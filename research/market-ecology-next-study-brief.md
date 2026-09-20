# Next-study design brief — counterparty composition and focal-strategy capacity

**Status: NOT AUTHORIZED TO RUN.** This is a bounded design option left by the
R2/SV1D development-line closeout. It is not an SV1D rescue, a new
preregistration, a configuration, a seed reservation, or a request to launch
simulation work.

## Proposed question

> How do counterparty composition and capital allocation determine the
> capacity, profitability, risk, and impact of one existing strategy?

This is closer to the original market-ecology objective than another attempt to
make the integrated ecology survive. It asks how an existing participant
function performs conditionally on the ecology around it, rather than treating
survival or a realism score as the mechanism target.

## Smallest plausible study

Use one asset, one or two existing venues, one focal strategy already present
in the platform, and a small set of already implemented counterparty classes
with explicit objectives and finite resources. Keep the current R2/SV1D line
closed. Do not add a larger supplier, add capital to rescue the ecology, or
reuse the CDF activation predicate.

Possible development cells would compare a baseline counterparty mix with a
small number of predeclared composition interventions, such as removal or
replacement of one existing class. An own-capital intervention would change
the focal strategy's assigned capital within a declared allocation scheme.
Adding total capital, changing agent count, and redistributing a fixed pool
are different interventions and must not be conflated.

The following should remain fixed across a matched comparison:

- total agent count and, where the question is composition rather than scale,
  total capital;
- liabilities and endowments outside the declared allocation intervention;
- participant information, observation delay, latency, and decision cadence;
- venue rules, matching, fees, risk limits, funding, settlement, and calendar;
- randomization policy and the focal strategy's code/configuration; and
- the measurement and terminal-validity contract.

If fixed-total-capital redistribution is not the question, an additive-capital
arm must be named as such rather than described as redistribution.

## Endpoints

Proposed primary endpoint: a preregistered **focal strategy capacity frontier**
— the largest tested focal flow/notional or capital condition that completes
the declared horizon with valid evidence, strict valuation, and the focal
strategy's predeclared economic acceptability and risk limits. The frontier
must be measured over a finite grid chosen before outcomes are seen; it is not
an optimization against a green score.

Supporting endpoints should include only a small fixed set:

- objective-appropriate focal outcome (net PnL is insufficient when the focal
  participant is a hedger with liability-reduction utility);
- drawdown, liquidation/deficit, inventory and financing exposure;
- executed quantity, fill quality, spread paid/earned, and market impact;
- counterparty volume/depth share, inventory, PnL, and withdrawal behavior; and
- ledger conservation and marked-wealth diagnostics kept conceptually
  separate.

## Causal interpretation

Useful trading activity is not itself a causal economic function. A focal
strategy should count as active only when its decisions, receipts, fills,
inventory and objective-linked state are reconstructible. A composition effect
requires a matched intervention and a predeclared contrast in the focal
frontier or objective, not merely a difference in aggregate volume.

Removal, restoration, and replacement answer different questions. Necessity is
conditional on the tested ecology. Pairwise strategy rankings do not identify
outcomes in an arbitrary multi-strategy population, and non-transitive pairwise
relations do not by themselves prove persistent capital cycles. A restricted
best-response search is not a global equilibrium result.

## Development/confirmation boundary and finite budget

If authorized, development should be capped before launch at one short horizon,
at most three development seeds, one baseline plus a small predeclared set of
composition/allocation cells, and one independently reviewed extraction pass
per cell. No holdout seed should be inspected while choosing the interventions.
The confirmation partition, if any, should be separately preregistered with a
small number of untouched seeds and a frozen analyzer. A failed activation or
invalid evidence path stops the cell; it does not license parameter rescue.

The exact strategy, composition classes, capital grid, acceptability function,
seed partition, horizon, and multiplicity rule still require prospective
preregistration and owner authorization. Those unresolved numeric choices are
why this document is a design brief rather than a locked protocol.

## Guardrails for future interpretation

- Technical capacity means RAM, disk, and runtime. Strategy capacity means an
  economically acceptable capital/flow region; they are not the same.
- Ledger cash transfers are not automatically trading PnL or causal influence
  on another strategy's profitability.
- A hedger may rationally pay trading costs to reduce liability risk.
- Marked portfolio wealth need not be constant merely because ledgers reconcile.
- Calendar periodicity and imposed adaptation are not endogenous market
  dynamics.
- New random seeds test replication in the simulated setting. Empirical
  realism requires a compatible real-data or literature comparison.

The current line remains closed until the owner makes a separate decision.
