# V2-R2-SV1B CDF liquidity successor preregistration

Date: 2026-09-02  
Candidate: `V2-R2-SV1B-24H-CDF-LIQUIDITY`  
Predecessor: `V2-R2-SV1` and the closed R2 candidate  
Stage: development-only registration; no freeze or holdout authorization

## Scientific boundary

The R2 candidate is closed as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**.
This document registers a separately named successor. It does not rescore,
repair, or reinterpret the predecessor. The old SV1 treatment-607 trajectory
is a consumed diagnostic pilot under an earlier contract and is not evidence
for SV1B.

SV1B carries the accepted R2 calendar/lifecycle, risk, matching, actor
information, and `evstream_v3` evidence semantics without economic retuning of
those mechanisms. The eight existing `elastic_supplier` participants remain
on their registered ABC/USD path unchanged. SV1B adds only a separately
configurable CDF/USD roster; an empty roster remains the historical default.

The immediate question is narrow:

> Can a finite, delayed-local, inventory-sensitive CDF/USD participant class
> trade and bear risk in a way that reduces persistent one-sided CDF collapse
> often enough for the complete ecology to remain strictly valuatable?

This is a deterministic simulator study (verification tier B for repeated
machine-checkable development evidence, with a material tier-C model-to-reality
gap). It is not a claim about real-market survival or broad realism.

## Mechanism hypothesis

Finite CDF/USD suppliers with bounded capital and inventory will respond to
their own delayed local CDF/USD observations and private inventory targets.
When the local book moves above or below the supplier's private reference, the
supplier may trade only on the inventory-reducing side. Ordinary fills change
its inventory, cash, fees, and PnL; the participant may then reprice or
withdraw. A surviving CDF book, if observed, must therefore be a consequence
of ordinary market interaction and finite risk-bearing capacity rather than a
survival callback or prescribed two-sided quote.

The private reference starts at the declared bootstrap belief of 300,000,000
raw quote units and adapts only toward eligible local observed midpoints with a
registered role-specific half-life. It is an initial participant belief, not a
live external price feed. Decisions use the delayed local snapshot delivered
by the normal actor gateway. No global market state, simulator object, hidden
index, or instantaneous cross-venue mark is available to the supplier.

## Registered CDF roster

The roster has four roles per venue. Values below are raw fixed-point units;
`base_precision = 100,000,000` and `quote_precision = 100,000`. Thus the
initial base balances are 400--700 CDF, and the initial quote balances are
$1.8--$2.7 million. The quote balances are deliberately close to, but above, each
role's maximum reference-price position notional (including the registered
maker fee); they are finite and can bind if the observed price rises.

| role | phase offset | initial CDF | initial USD raw | max position | max inventory | max quote qty | max loss budget | elasticity / % | reference half-life |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `cdf_elastic_supplier_1` | 0 ns | 40,000,000,000 | 180,000,000,000 | 40,000,000,000 | 80,000,000,000 | 40,000,000 | 30,000,000,000 | 12,000,000,000 | 3.0 h |
| `cdf_elastic_supplier_2` | 500,000,000 ns | 50,000,000,000 | 210,000,000,000 | 50,000,000,000 | 100,000,000,000 | 50,000,000 | 36,000,000,000 | 15,000,000,000 | 4.0 h |
| `cdf_elastic_supplier_3` | 1,000,000,000 ns | 60,000,000,000 | 240,000,000,000 | 60,000,000,000 | 120,000,000,000 | 60,000,000 | 42,000,000,000 | 18,000,000,000 | 5.0 h |
| `cdf_elastic_supplier_4` | 1,500,000,000 ns | 70,000,000,000 | 270,000,000,000 | 70,000,000,000 | 140,000,000,000 | 70,000,000 | 48,000,000,000 | 21,000,000,000 | 6.0 h |

All roles use:

- `symbol = CDF/USD`, `base_asset = CDF`, `quote_asset = USD`;
- `base_holding = 0`, `reference_price = 300,000,000`;
- `base_precision = 100,000,000`, `quote_precision = 100,000`;
- `interval = 2,000,000,000 ns`, `max_observation_age = 60,000,000,000 ns`;
- `maker_fee_bps = 5`.

Each role also registers a positive `max_loss_quote` budget equal to 10% of
its initial endowment marked at the declared 300,000,000 reference price:
`initial_quote_balance + initial_base_balance * reference_price /
base_precision`. The actor marks that endowment only with its delayed local
CDF/USD midpoint, carries quote cash reserved by live orders into equity, and
withdraws permanently for the run when either loss from initial equity or
peak-to-current drawdown reaches the budget. Arithmetic failure is a separate
fail-closed `equity_unavailable` state. This budget is a participant risk
constraint, not a target for CDF price, spread, volume, or survival.

The zero phase is the explicit default and is omitted from normalized JSON
because the field is optional; nonzero phase offsets are serialized and
verified. This preserves a stable canonical representation without weakening
the registered phase contract.

The role differences are preregistered participant heterogeneity, not
post-run calibration: finite balance-sheet capacity, inventory elasticity,
private-reference memory, and scheduler phase are varied monotonically by
role. The supplier still posts at most one passive order, has no obligation to
quote either side, and can withdraw without replacement when its observation,
desired side, or risk budget is unavailable.

## Controls and fresh seeds

The paired control removes the new CDF roster while preserving the rest of the
registered base configuration. Because this changes participant population
and scheduler topology, the comparison estimates a **total population
intervention effect**, not a supplier-policy-only effect. A no-roster control
cannot prove what would happen under an unchanged actor-ID topology.

The full development pairs use the first three unused odd primes greater than
the reserved holdout maximum 641, selected by an outcome-blind rule:

`643`, `647`, and `653`.

Seed 643 also has the registered G4/G8 and no-log parity controls. The separate
five-minute activation probe uses seed 643 and is a prerequisite to the full
24-hour campaign; it is not itself a survival result. Holdouts `619`, `631`,
and `641` remain closed and must not be read or run.

## Activation criteria

The activation probe must retain valid paired evidence showing all of the
following in treatment:

1. every required supplier receives eligible delayed local observations and
   emits decisions;
2. at least one supplier order is accepted and at least one CDF fill changes
   its inventory;
3. account balances, fills, fees, positions, and terminal marks expose a
   reconstructible nonzero PnL/risk transition;
4. inventory changes alter a supplier's target side or quantity;
5. at least one quote is cancelled, withdrawn, or repriced for a local
   observation, inventory, risk, or unavailable-side reason;
6. at least one successor decision exposes the marked-equity state and its
   risk-limit flag remains false until a valid budget breach, if any;
7. no supplier borrows, receives capital/inventory replenishment, or is forced
   to maintain two-sided quotes.

The typed audit must pass its finite-capital, inventory, delayed-observation,
fill, PnL, withdrawal, and concentration checks. A valid false survival
measurement is an economic result; missing or malformed measurement is invalid
evidence and cannot be classified as a negative mechanism result.

## Anti-cheating and kill criteria

SV1B is rejected if any of the following occurs:

- survival is primarily produced by a mechanically guaranteed quote, forced
  two-sided obligation, forced replenishment, infinite or effectively
  non-binding capital, or a hidden external price anchor;
- a supplier exceeds its registered balances, inventory/position limits, or
  borrowing prohibition;
- the CDF book remains persistently one-sided or strict terminal valuation
  still fails;
- the supplier class has aggregate CDF volume share above 75%, or supplies
  more than 75% of displayed CDF depth for more than half of the measured
  active intervals in any venue;
- supplier behavior is driven by simulator state rather than its delayed local
  interface;
- the supplier is structurally forced to quote both sides or to replace a
  withdrawn quote.
- the successor’s positive loss budget is absent, ignored by the policy, or
  can be reset after a loss without a newly declared participant state.

The numerical audit is necessary but is not a qualitative anti-cheating
verdict. Before any freeze decision, an independent reviewer must inspect the
retained decision/order evidence separately for hidden anchoring, common-policy
concentration, capital non-bindingness, and mechanical survival. The scorer
must label this qualitative review as pending until that review occurs.

## Measurements and falsifiers

For every supplier and venue, retain traded volume share, displayed-depth
share, inventory path, maximum absolute inventory, cash/borrow usage, fees and
PnL, quote lifetime, quote/reprice/withdrawal counts, local observation age,
target position, action reason, side, price, and quantity. For treatment and
paired control, retain exact CDF/USD empty-side windows and mark availability.

The hypothesis is falsified for this candidate if the activation probe fails,
the 24-hour treatment remains unvaluatable, no measurable treatment effect is
identified across the preregistered fresh pairs, or any anti-cheating/kill
criterion fires. A negative or unknown result closes SV1B without changing the
R2 predecessor.

## Promotion sequence

The sequence is fixed before measurement:

1. register and hash this document, the fresh roster, configs, and seed rule;
2. run focused tests, full tests, vet, targeted race, determinism, and
   evidence-neutrality checks;
3. obtain one fresh independent Sol-xhigh review of the exact successor tree;
4. build pinned clean Go 1.27.0 binaries and measure actual successor storage
   capacity;
5. run only the five-minute seed-643 activation probe and independently review
   it;
6. only if activation and qualitative gates pass, run the registered fresh
   24-hour development pairs and seed-643 parity controls;
7. extract and independently review all development evidence before any
   freeze authorization;
8. keep holdouts untouched until a separate explicit freeze authorization.

No step in this preregistration authorizes a holdout, a full campaign before
activation, economic retuning after an observed result, or deletion of raw
evidence before its retention contract passes.
