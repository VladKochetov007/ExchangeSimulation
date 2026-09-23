# ME-010 — Financing and derivative child-study readiness

## Question and scope

Which one financing/lifecycle/payoff contract is ready for a separately named extension?

Strongest intended claim: **MECHANICAL** (proposed, not established).
Perp carry, dated carry/roll, parity and hedging require distinct child protocols.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S18](../../policy-catalogue.md#s18), [S19](../../policy-catalogue.md#s19), [S20](../../policy-catalogue.md#s20), [S21](../../policy-catalogue.md#s21), [S22](../../policy-catalogue.md#s22), [S23](../../policy-catalogue.md#s23), [S24](../../policy-catalogue.md#s24), [S25](../../policy-catalogue.md#s25).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/multivenue/funding_carry.go](../../../../simulations/multivenue/funding_carry.go)
- [simulations/multivenue/dated_term_carry.go](../../../../simulations/multivenue/dated_term_carry.go)
- [simulations/derivsim/optionmm.go](../../../../simulations/derivsim/optionmm.go)

Required measurement/opportunity contract: Financing accrual, fees, positions, marks, expiry/payment/settlement-pending and terminal risk.

Dependencies: [ME-000](../ME-000/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/v2-5-p4-funding-carry-results.md](../../../../research/v2-5-p4-funding-carry-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/v2-5-p5-dated-carry-results.md](../../../../research/v2-5-p5-dated-carry-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/v2-6-p6-options-results.md](../../../../research/v2-6-p6-options-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/v2-7-p7d-results.md](../../../../research/v2-7-p7d-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

P4 basis falsification, P5 non-exercise and P6 partial/unidentified outcomes stay historical; no automatic rerun.

## Tentative design and next action

Select a bounded child such as ME-010-PERP only after owner direction; do not run the umbrella.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
