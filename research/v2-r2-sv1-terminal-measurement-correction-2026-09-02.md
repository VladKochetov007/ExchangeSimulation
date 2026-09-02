# SV1 terminal measurement correction

Date: 2026-09-02  
Candidate: `V2-R2-SV1-24H-CDF-LIQUIDITY`  
Scope: development scoring contract; no simulator economics changed

The previous SV1 scorer conflated a typed nonpositive endpoint mark with an
invalid terminal measurement. That would turn an observed strict-valuation
failure into an evidence-shape failure and would hide the distinction needed
by the registered kill criteria.

The corrected contract has two predicates:

* `terminal_measurement_valid` requires a complete terminal-post-mark record at
  the exact simulation end with numeric CDF and USD marks, even when a mark is
  zero or negative;
* `terminal_mark_valid` additionally requires the registered two-sided source
  and strictly positive CDF/USD values.

Thus a typed zero/nonpositive mark is valid evidence and a strict valuation
failure. A missing, malformed, wrong-phase, or wrong-time record remains
invalid evidence. The correction is implemented as the standalone jq contract
`scripts/v2-r2-sv1-terminal-measurement.jq`, tested by
`scripts/test-v2-r2-sv1-terminal-contract.sh`, and the scorer version is
`v2-r2-sv1-24h-development-scorer-v4`.

This is a measurement-contract correction only. It does not rescue the
predecessor R2 result, authorize a cell, or establish a treatment effect.
