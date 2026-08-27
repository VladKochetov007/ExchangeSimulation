# Independent review: integrated V2 reference smoke

Date: 2026-08-27

Reviewer: independent Sol-family xhigh review pass (`integrated_smoke_reviewer`)

## Inputs reviewed

- Protocol: `research/v2-integrated-reference-preregistration.md`
- Run result: `research/v2-integrated-reference-smoke-results.md`
- Machine attestation: `research/v2-integrated-reference-smoke-attestation.json`
- Analysis manifest: `research/v2-integrated-reference-analysis-manifest.json`
- Immutable run: `research/artifacts/v2-integrated/reference-dev-601-full-clean`
- Replay implementation and tests at simulator/analyzer revision
  `bacb7d86cc45c616b0825235919850df6fbb04cc`

## Verdict

**ACCEPT** for the narrow, preregistered integrated-reference compatibility and
evidence-boundary smoke claim.

## Review findings

- Full versus no-log execution parity and GOMAXPROCS parity agree on all six
  checkpoints, 352,099 execution observations, and the final execution hash.
- Runtime and offline persisted-evidence digests agree for all 359,325 records.
  All 15 JSONL streams parse and match their manifest line counts and hashes.
- Receipt/frontier checks, role activation, population accounting, and
  conservation checks remain internally consistent. The bounded residual is
  reported in fixed-point units rather than mislabelled currency units.
- The maker-refresh replay is based on physical per-book event order rather than
  the actor's ordering declaration. It checks request fields, post-only
  admission, prices, quantities, cumulative fills, cancellations, duplicate and
  unknown identities, ordinary non-passive IOC orders, and dropped acceptance
  mutations. It proves 6,102 decision sides partition as 6,017 accepted, 69
  rejected, and 16 terminal-censored; 5,970 replacement sides partition as 5,493
  cancellation-terminated and 477 full-fill-terminated.
- Horizon censoring is fail-closed: every terminal account must have the same
  nonzero timestamp, and the censored decision must occur at that timestamp.
  Focused mutations for absent, zero, and mixed terminal timestamps fail.
- The analyzer archive was rebuilt from a clean Git clone at the full revision
  above with `vcs.modified=false`; the recorded binary SHA-256 is
  `e00314ce97de6763edd3b45770090a3bb04b243a1841eca9113135a128ce5870`.

## Claim boundary

This review does not license realism, funding anchoring, basis convergence,
trade-mediated price discovery, or emergence. Settlement, cross-asset CDF
borrowing, and executable router legs are `NOT EXERCISED` in the five-minute
smoke. SABR/Vanna--Volga activity is inherited from O4 and is not emergent
evidence. The full-evidence run is the scientific source; the no-log companion
exists only for execution-neutrality testing.

No further repair or simulation rerun was required after the final censor and
clean-build provenance corrections.
