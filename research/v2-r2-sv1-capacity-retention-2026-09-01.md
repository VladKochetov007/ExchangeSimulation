# Superseded SV1 capacity-probe retention amendment

Date: 2026-09-01  
Successor: `V2-R2-SV1-CDF-LIQUIDITY`  
Protocol: `v2-r2-sv1-capacity-archive-retention-v1`

The successful capacity probe at source revision
`2f93ace63fa1577c5d2122dfe207c4e0f369b9ee` measured a peak retained output of
`32929468561` bytes and a required free-space floor of `37224435857` bytes.
Its attestation and evidence manifest passed their original contract. The
probe root is:

```text
/home/vlad/external-scratch/v2-r2-sv1-24h-capacity-2f93ace63fa1577c5d2122dfe207c4e0f369b9ee
```

The current source revision is now `c67fa69`, so that attestation cannot
authorize a current-revision binary. Keeping the old 33 GB probe unpacked would
prevent a fresh full-output capacity measurement on this host while retaining
the current development evidence namespace.

## Explicit retention rule

`scripts/archive-v2-r2-sv1-capacity-probe.sh` is a narrow, fail-closed
superseded-probe compactor. It may compact a probe only when all of the
following hold:

- the scientific worktree is clean;
- the attestation is valid against the supplied validating binary, source
  revision, configuration hash, process width, and reserve;
- the attestation source revision differs from the current source revision;
- the source revision is a real commit and an ancestor of the current HEAD;
- the probe run metadata, manifest build identity, validating binary VCS
  identity, and registered treatment configuration independently agree with
  the attestation;
- the probe root is present under the dedicated external SV1 capacity
  namespace and its evidence manifest verifies;
- every probe path is a non-symlink on the probe root's single filesystem;
- the archive target is external to the repository and source probe root;
- archive and sidecar publication uses exclusive temporary files and
  no-replacement hard-link publication; archive creation, zstd integrity,
  member enumeration, and full `tar --compare` all pass with no comparison
  output;
- the retention receipt, archive checksum, and sidecars are written before the
  exact old probe root and old attestation are removed.

The tool refuses a current-revision probe, non-ancestor or cross-bound
provenance, symlinked targets, nested mounts, pre-existing archives/sidecars,
dirty source, and malformed or incomplete attestation data. A failure after
archive publication leaves the source probe intact; deletion is reached only
after final archive, sidecar, receipt, and filesystem checks pass.
This amendment applies only to superseded capacity measurements; the active
capacity probe for the candidate under test remains unpacked and manifest-
verifiable for the duration of its gate.

No capacity probe has been compacted under this new protocol yet. An
independent review of the exact implementation is required before invoking it.
The amendment changes storage retention only; it does not alter simulator
economics, evidence bytes, event ordering, or the registered development
sequence.
