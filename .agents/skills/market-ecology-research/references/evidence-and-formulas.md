# Mechanisms, units and independent evidence

## Economic contract

For each idea name the objective, who benefits/pays, liabilities and exit rule.
State initial capital/inventory, financing and borrowing limits, and any sources
or sinks. List the observations actually delivered before each decision,
including abstention when the estimand requires it. Separate local predicted
behavior from aggregate effects and identify a discriminating contrast.

Trace information -> feasible action -> request -> admission/rejection ->
ordered fills/cancelled residual -> portfolio/obligation -> measured objective.
A received snapshot need not equal the venue book when a delayed request arrives.
For accepted requests reconcile admitted = filled + cancelled residual at
completion. Rejected/unadmitted quantity is a different path; do not invent an
order or cancellation to make the equality hold.

## Equation register

| Class | Required specification | Permitted interpretation |
|---|---|---|
| ACCOUNTING_IDENTITY | units, fixed-point precision, signed domain, independent arithmetic, exact rounding/overflow policy | implementation consistency |
| PROGRAMMED_POLICY | inputs, parameters, objective, constraints, no-action behavior | implementation/activation, not discovery of the formula |
| THEORETICAL_APPROXIMATION | assumptions, numerical error, valid domain and counterexamples | conditional approximation |
| EMPIRICAL_REGULARITY | estimator, alternative forms, calibration split and unseen comparison | evidence within a compatible data regime |

A measurement convention such as terminal marking must state its benchmark and
counterfactual meaning. Marking a residual obligation does not execute it.
Do not annualize a short run into a confident return estimate.
Account for fees/rebates in the assets actually charged, financing, transfers,
injections/withdrawals, realized and unrealized components and exit costs.

Cash/asset/debt reconciliation is not constant total marked wealth. Keep three
graphs distinct: actual transfers; benchmark-relative PnL attribution; causal
effects of one population on another's objective. Purchases are not automatic
losses to the buyer. Markouts are not cash transfers. Negative trading PnL can
be consistent with a hedger's liability-risk objective.

## Domains and missing values

Check signed-price domains for log returns, ratios, annualization, basis units,
discounting and option inversion. Record unavailable values with explicit reasons;
never filter them silently or use zero as a missing value.
Do not weaken strict valuation or terminal-risk requirements.
Use the declared expiry/payment/settlement-pending contract, not an assumed one.
Inspect the producer of historical count fields: zero checks may not mean zero
violations, zero events, or zero tests.

## Executable arbitrage

Reconstruct both directional bid/ask routes with actual depth, lot/tick sizes,
charged-asset fees, balances and admission rules. Include inventory segregation,
borrowing/transfer delay, leg timing, non-atomic fills, residual exposure and
costed closure. A midpoint cycle is not executable profit.
Separate static known-state positive/negative edge fixtures from natural
opportunity incidence and intervention effects in endogenous worlds.
Do not freeze endogenous counterpart responses into an old tape and claim the
new comparison remains fully endogenous.

## Independent reconstruction

Pin raw manifest, stream/schema, ordering, source/config and analyzer identities.
Join economic identities and causal links, not directory traversal.
Keep exchange match time and participant delivery time.
An exchange fill before cancellation can arrive afterward legitimately; reject
unanchored, mismatched, late-executed or quantity-overrun fills under the actual contract.
Declare mutation tests for dropped/duplicated/misordered events, wrong identities,
fees/units, incomplete output and unsupported domains.
Evidence instrumentation must not alter RNG, scheduling or actor-visible state.
Keep old invalid scores and accepted corrections as different versioned artifacts.
Use existing evidence contracts and manifests; no format migration is implied.
