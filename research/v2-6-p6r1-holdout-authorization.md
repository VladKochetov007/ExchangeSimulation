# V2-6 P6-R1 untouched-holdout authorization

Status: **authorized, not yet run**.

The paired development screen for the preregistered cross-asset collateral
mark repair is complete and recorded in
[`v2-6-p6r1-viability-results.md`](v2-6-p6r1-viability-results.md).  All ten
O0--O4 × {211,213} cells passed the full evidence contract, exercised the CDF
collateral path, and passed their stage-specific activation gates.  The
registered rule therefore permits the untouched holdout policy.

Holdouts are the predeclared seeds **223, 227, and 229** for every stage
O0--O4.  No seed was selected from a response, and no funding, liquidity,
spread, clock, population, option, or collateral value was changed after
development outcomes.  The holdout configs are copied from the hash-pinned
development family with only seed/experiment-description provenance changes;
their exact hashes are checked by
[`scripts/check-v2-6-p6r1-configs.sh`](../scripts/check-v2-6-p6r1-configs.sh).

Each holdout must use the same eight-hour full-evidence contract and the only
completion sentinels remain final `greeks.json` and `latency.json`.  The
extractor must pass all registered evidence, lifecycle, settlement, option
surface, and cross-asset viability predicates before any holdout is scored or
pruned.  O3/O4 surface results remain inherited-prior/descriptive unless a
complete paired stage comparison is independently scored; no holdout result
alone licenses an emergent-smile claim.

This authorization does not authorize economic retuning or replacement of the
original P6 preregistration.  If a holdout stage is inactive or fails its
evidence contract, it is reported as NOT EXERCISED or invalid under the fixed
rules rather than rescued by changing the configuration.

