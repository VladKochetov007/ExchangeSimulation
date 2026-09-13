# SV1D capacity producer/private-entry correction

Candidate `67bba6699a39167e1c2f6cf7e432cfcda16aeda4` received a substantive
independent **REJECT**, execution **COMPLETED**, from fresh native context
`01a09ae9-3a99-7c91-b12f-54f4c5e9f956`. The full report and raw reproductions are
retained outside its unchanged checkout at
`/home/vlad/external-scratch/sv1d-review-repair-20260913/source-review.md`.
The report SHA-256 is
`dfbed09bb4cc3be4568fb1b5531cc3247655196a55c3bd1ea96cbda87609182d`.
No production build, signed acceptance, capacity or activation run resulted.

Two concrete source findings, independently reproduced before correction:

1. `SV1D-C67-001`: the runner passed temporary staging tool paths into the
   measured command and metadata, while the verifier required retained paths.
   The successor passes the retained hash-addressed simulator/analyzer/renderer
   paths. Bytes, Go metadata, source and review bindings remain mandatory.
2. `SV1D-C67-002`: private entry discarded the capacity-only config fields
   before comparing economic identity, without first validating their values.
   The successor first requires seed 977, the exact capacity arm identity,
   hypothesis, status and description. The original unchanged-economic-fields
   comparison still applies. Post-run seed validation is not a substitute for
   this pre-start boundary.

`analysis/sv1d_capacity_runner_test.go` incorporates the independent production
argument/predicate reproductions and adds field/type mutations. It tests actual
shell construction against the Go command and metadata verifiers, separate raw
and typed-plan digests, and pre-start rejection of unregistered seed/config
values. Reserved seeds appear only in JSON rejection fixtures; no world or
holdout evidence is opened. Fail-before and pass-after logs are retained with
the rejected candidate's external evidence package.

This is an explicit corrected source successor, not an extension of the
rejected commit's review or the old ef839a2 bundle. Full clean tests and the
same independent review context must accept the successor before pinned build,
fresh verified bundle and registered seed-977 capacity. No economic semantics,
numeric criteria, resource floors, experiment ordering, or historical claims
are amended by these corrections.
