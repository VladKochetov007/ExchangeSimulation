# ME-007 — Three-asset executable conversion

## Question and scope

Can both triangular routes be reconstructed with legal quantities, real fee assets and residual costs?

Strongest intended claim: **MECHANICAL** (proposed, not established).
Three assets, one venue first; finite balances; actual sequential legs.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S17](../../policy-catalogue.md#s17).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/feesim/triarb.go](../../../../simulations/feesim/triarb.go)
- [simulations/multivenue/naive.go](../../../../simulations/multivenue/naive.go)

Required measurement/opportunity contract: Asset-specific cashflows, fee rounding, partial legs and residual inventory.

Dependencies: [ME-000](../ME-000/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/no-arbitrage-audit.md](../../../../research/no-arbitrage-audit.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Start with mechanics. A later endogenous opportunity hypothesis needs its own reviewed protocol.

## Tentative design and next action

The existing [FeeAwareTriArb](../../../../simulations/feesim/triarb.go) is a
three-leg **sequential market-order** state machine using delivered book
touches and a three-fee-bp approximation. It does not establish executable
full-depth edge, actual charged-fee assets, robust partial-leg accounting,
residual inventory closeout or net profit. Its timeout resets local state
without proving that earlier filled exposure was unwound. Historical
[cross-book drift](../../../../research/no-arbitrage-audit.md) is versioned
frozen-baseline evidence, not an opportunity or payoff measured on this main.

First primary question is **MECHANICAL**: can both ABC/USD → Q/ABC → Q/USD
and reverse routes be independently reconstructed in exact asset units,
including each charged fee and any stranded inventory, under finite balances?
Falsifier: one legal positive-edge route, one no-edge route or one partial-leg
case cannot reconcile the actor's decisions, exchange fills and three asset
ledgers under declared rounding. A programmed positive edge in a fixture
does not demonstrate an endogenous market opportunity. Confounds for a later
causal study include delayed quotes, depth consumption, leg sequencing,
feed phase, finite capital and exogenous price anchors.

Before any economic world, design at least six synthetic fixtures: two
directions × positive/negative edge and two one-leg-failure/rounding cases.
Use actual bid/ask depth and legal lots, actual fee assets, response-versus-
exchange execution times, per-leg balances and a declared costed residual
closeout rule. Compare an independent reconstruction with the actor report;
fail closed on mismatched or missing evidence. Do not change the historical
frozen audit or assume the fee approximation is exact. This mechanical stage
has **zero market worlds** and a proposed ≤2-minute fixture-test budget.

Only if that stage passes could a separate prospective causal screen compare
router off/on × two new development seeds (four worlds plus two controls)
for eligible episode duration and costed completed-group payoff, under a
measured resource cap of no more than 15 minutes, 4 GiB RSS and 1 GiB
evidence. Its ecology, horizon, seeds and numeric thresholds are not chosen
here. No implementation, economic execution or historical holdout use is
authorized by this card.
