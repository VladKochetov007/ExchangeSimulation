# SV1 treatment-607 extraction-contract incident

Date: 2026-09-01  
Successor: `V2-R2-SV1-CDF-LIQUIDITY`  
Predecessor: R2, retained unchanged as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**

This is an append-only incident record. It does not rewrite the predecessor,
the SV1 preregistration, or any economic result.

## Attempt identity

The first registered SV1 treatment attempt used:

- cell: `treatment-607`;
- source revision: `2f93ace63fa1577c5d2122dfe207c4e0f369b9ee`;
- binary evidence format: `evstream_v3`;
- runner status: `0` and `completion_verified: true`;
- simulation horizon: the registered 24 hours;
- raw cell: `/home/vlad/v2-r2-sv1-24h-development-20260901-v1/treatment-607`.

The runner produced the complete raw cell and its external attestation. The
first extraction attempt returned status `1` before creating any derived
artifact. Its exact failure was:

```text
jq: parse error: Invalid numeric literal at line 1, column 191
integrated long-run extraction failure: malformed JSON object: .../events.evs
```

`events.evs` is the canonical binary event stream, not a JSON object. The
extractor had applied the JSON-sidecar validator to every required input before
reaching its existing binary renderer. This is a control-plane/analyzer
contract defect, not an economic outcome and not evidence that CDF liquidity
survived or failed.

The raw attempt remains preserved in:

```text
/home/vlad/v2-r2-sv1-treatment-607-invalid-extraction-2f93ace.tar.zst
```

Archive SHA-256:

```text
8f261d1a8cc8384133d5a6cd48da4c2c658e62b330738ba3a48d5a2676fcdda1
```

`zstd -t` and the full `tar --compare` pass completed before removal. The
comparison log contains only the expected UID/GID/mtime notices caused by the
archive's normalized metadata; it contains no size or content mismatch. The
archived external attestation was also compared byte-for-byte with its source.
The four Go 1.27 binaries used for the attempt are retained beside the
archive with the `2f93ace` suffix, and the runner stdout/stderr sidecars are
retained under the same invalid-attempt name.

The archive includes the complete cell and the external treatment attestation.
Its member listing is retained beside it as
`v2-r2-sv1-treatment-607-invalid-extraction-2f93ace.members`; the extraction
failure log is retained outside the repository at
`/home/vlad/v2-r2-sv1-treatment-607-invalid-extraction-2f93ace-extract.log`.

## Correction and regression

The extractor now classifies required inputs explicitly:

- `events.evs` must be a nonempty, non-symlinked regular binary file;
- all JSON sidecars must be nonempty, non-symlinked regular JSON objects;
- missing files and symlink substitutions fail closed.

The shared input contract has a cheap fixture regression in
`scripts/test-v2-integrated-longrun-r2-contract.sh`. It proves that a binary
stream is not sent to `jq`, JSON sidecars remain checked, and missing or
symlinked binary streams are rejected. No simulator or economic code changed.

The corrected source requires a fresh clean build, revision-bound capacity
measurement, independent review, and a fresh `treatment-607` run. The archived
attempt must not be extracted or scored as though it were produced by the
corrected revision.

## Scientific disposition

Classification: **ANALYZER/CONTROL-PLANE BUG; NO SCIENTIFIC ACTIVATION RESULT**.

No CDF activation, survival, kill criterion, or historical claim is inferred
from this attempt. No holdout was inspected or consumed. The successor remains
at the pre-activation development gate until the corrected treatment is
extracted, independently reconstructed, and reviewed.
