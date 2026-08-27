# V2 clean-candidate gate: independent review

Date: 2026-08-27

## Reviewed material

- candidate source commit: `29d28835469a44afe1eeaf2ed63032d56f23b936`;
- simulator semantic correction: `2068d9d`;
- clean clone built with Go `go1.26.6-X:nodwarf5`, `-buildvcs=true -trimpath`,
  and `vcs.modified=false`;
- four fresh 10-second seed-101 smoke runs at GOMAXPROCS 1/4, full/logs-off;
- runtime and offline evidence-artifact digests;
- full `go test ./...`, `go vet ./...`, targeted race tests, and the
  cross-process determinism test;
- machine package `research/artifacts/v2-freeze-candidate/clean-candidate-gate.json`.

## Verdict

**RECOMPUTE.** The package is valid and useful as a short-horizon
execution/logging-neutrality smoke, but it is not sufficient to declare an
immutable V2 freeze candidate.

## Claims supported by the smoke

All four fresh processes produced 7,073 ordered execution observations with
execution digest
`fff1801f4f97795674421f3508854fbcd473d94ac356208e1387d16e15106d43`.
The three full-log runs each produced 7,073 persisted records with evidence
digest
`1d2d0b425053ff9158081a2775630d703db5b27fdd9227c0616ffe97a16e6148`, and
offline `mvanalyze -metric evidenceartifacthash` matched each runtime artifact.
The logs-off run retained the same execution digest. The run manifests in the
VCS-stamped rerun identify revision `29d2883...` and `vcs.modified=false`.

This supports only exact short-horizon process/parallelism/logging neutrality
and evidence-artifact recomputation.

## Reasons for recomputation before freeze

1. Ten seconds cannot exercise funding, expiry/settlement, distress,
   liquidation, or late scheduler behavior. The existing freeze contract
   requires fresh long-horizon verification after V2 changes.
2. The smoke config does not record participant receipt/frontier sidecars, so
   the information-boundary evidence contract is not exercised.
3. The smoke does not retain a complete campaign run-config/run-metadata,
   analyzer metadata, exit-status record, and registered evidence/mutation
   package bound together. The durable machine package records the command and
   hashes, but the smoke is not a substitute for the full provenance bundle.
4. Candidate-bound accounting, lifecycle, settlement, funding, risk,
   positive-world equivalence, prune-gate, and high-value mutation artifacts
   have not been regenerated from this exact source candidate.
5. The full-suite race invocation previously exceeded its ten-minute budget in
   the pre-existing expensive P3 replenishment test without a race report;
   targeted changed-boundary race tests passed. This remains a coverage
   limitation, not evidence of a race.

## Required next gate

Rebuild all binaries from a normal clean clone with embedded VCS metadata, run
the registered long-horizon mechanical/control package in fresh directories,
retain exact configs and completion metadata, and rerun the candidate-bound
determinism, accounting/lifecycle/information/risk/mutation checks. Only after
that package is independently reviewed may the source be declared immutable.
No economic outcome or holdout claim is licensed by this smoke.

