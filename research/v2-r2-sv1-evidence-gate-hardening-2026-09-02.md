# SV1 evidence and provenance gate hardening

Date: 2026-09-02  
Candidate: `V2-R2-SV1-24H-CDF-LIQUIDITY`  
Scope: fail-closed evidence/provenance adapters; no economic change

This checkpoint closes three independent promotion-gate defects identified by
the fresh exact-tree red-team review:

1. The SV1 scorer now rejects a nonzero `cdf-liquidity-audit` exit status even
   when the process emitted a JSON object. The object is moved to an explicit
   `.invalid` path for diagnosis; it is never treated as a valid economic
   negative or accepted score.
2. The SV1 deterministic verifier now compares `priceunavailable.json` as well
   as `cdfliquidity.json`, so the typed unavailable-price contract is covered by
   the same fresh-recomputation equality check.
3. SV1 binary checks require the exact installed toolchain identity `go1.27.0`.
   The historical shared R2 predicate remains unchanged for archived callers.

These changes do not authorize a development cell, alter the CDF participant,
change calendar economics, or open a holdout. The invalid-output retention rule
preserves forensic evidence when a gate fails.
