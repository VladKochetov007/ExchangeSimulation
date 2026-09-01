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
provenance, symlinked targets, nested mountpoints (including same-device bind
mounts found through the mount table), pre-existing archives/sidecars, dirty
source, and malformed or incomplete attestation data. A failure before the
cleanup phase leaves the source probe intact. Cleanup first renames the exact
probe root into a process-specific quarantine and then removes only that
quarantine; an interruption during deletion may leave the quarantine in place,
while the complete verified archive remains authoritative.
This amendment applies only to superseded capacity measurements; the active
capacity probe for the candidate under test remains unpacked and manifest-
verifiable for the duration of its gate.

No capacity probe has been compacted under this new protocol yet. An
independent review of the exact implementation is required before invoking it.
The amendment changes storage retention only; it does not alter simulator
economics, evidence bytes, event ordering, or the registered development
sequence.

## Verified retention execution — 2026-09-01

The fresh independent Sol-xhigh review of exact tree
`41de87bbdb5f26c10c27c884b4cd8688baf756c5` returned **ACCEPT WITH NARROWER
CLAIM** for the setup gate. It authorized one controlled invocation of this
protocol against the already validated superseded `2f93ace` capacity probe,
provided no concurrent mutation occurred. No scientific cell, parity control,
freeze action, or holdout was authorized by that review.

That invocation completed successfully:

```text
source revision: 2f93ace63fa1577c5d2122dfe207c4e0f369b9ee
protocol revision: 41de87bbdb5f26c10c27c884b4cd8688baf756c5
probe bytes: 32929468561
archive: /home/vlad/v2-r2-sv1-capacity-archive-2f93ace-retained.tar.zst
archive bytes: 4020725425
archive sha256: f80cdcc72ae1b18735b4cf0f4cda88118da4985ce5d6917ce22d30a1249c68e8
comparison: tar_compare_clean
```

The archive, checksum, member list, empty comparison log, and retention
receipt are regular files. The receipt records 37 retained source files; the
tar member listing contains 46 entries including directory entries. `zstd -t`,
member enumeration, archive checksum binding, and the full content comparison
all passed. Only after those checks did the protocol remove the exact
quarantined probe root and its attestation. Both are now absent; the archive
is the recoverable retained record. No current or historical scientific
evidence was deleted. The host recovered approximately 33 GiB, leaving about
72 GiB available for a fresh revision-bound measurement.

This compaction is a storage-retention operation only. It does not authorize a
binary rebuild, capacity probe, development cell, freeze, or holdout. The next
valid step is a clean Go 1.27 rebuild from the final documented HEAD followed
by a new revision-bound capacity measurement.
