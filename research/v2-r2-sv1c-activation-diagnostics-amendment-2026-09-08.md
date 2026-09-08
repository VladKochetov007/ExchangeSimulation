# SV1C activation diagnostics amendment

Date: 2026-09-08
Candidate: V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK

The activation probe must independently bind treatment and control configs,
binary identities, evidence manifests, exact event ordering, and terminal
outcomes to the SV1C namespace. In addition to the inherited finite-supplier
activation predicates, the diagnostic must show explicit strict-risk
configuration: spot and perpetual auto-borrow disabled, cross-asset collateral
marks disabled, no supplier debt or replenishment, and complete coherent
account-scoped liquidation evidence.

An activation probe is a mechanism check only. It cannot authorize a 24-hour
campaign, freeze, holdout, or reinterpretation of SV1B/R2 evidence.
