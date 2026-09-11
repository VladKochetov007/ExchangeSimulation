# V2-R2 SV1D measured binary-capacity preflight

Status: preregistered, not yet executed

This document defines the resource-only preflight required before the
development-only SV1D activation probe. It is a capacity measurement, not a
scientific arm and cannot be used as a market-survival result. The historical
R2 JSON evidence tree remains untouched and is not reinterpreted under this
contract.

## Identity and boundary

- Contract: `v2-r2-sv1d-binary-capacity-preflight-v1`.
- Purpose: `five_minute_sv1d_binary_evidence_capacity_preflight`.
- Capacity seed: `977`, distinct from the registered activation seed `659`.
- Horizon: exactly five simulated minutes, from
  `1735689600000000000` through `1735689900000000000` nanoseconds.
- Arms: `treatment`, `mode-off`, and `no-roster`, executed sequentially and
  retained in one fresh capacity namespace.
- Scientific eligibility: false. No activation score, realism metric, or
  holdout claim may be derived from this run.
- Evidence: `evstream_v3`, evidence schema epoch `4`, `full` log mode.
- Runtime policy: `GOMAXPROCS=2`, `GOMEMLIMIT=4GiB`, and a finite cgroup
  memory limit supplied by the execution scope. An unbounded `memory.max` is a
  hard failure; the host’s available RAM is never treated as a substitute for
  a cgroup limit.

The preflight is bound to the exact SV1D review-bound plan, source/tree
revision, signed review/report hashes, target config hashes, capacity-only
config hashes, simulator/analyzer/renderer hashes, the capacity runner hash,
the Go resource-measurer hash, and this policy file’s hash.

## Capacity-only configuration delta

Each capacity config is generated from the corresponding registered seed-659
config. The only permitted typed-field changes are:

- `seed`: `659` -> `977`;
- `experiment_id`: the capacity-only arm identity;
- `hypothesis_id`: `V2-R2-SV1D-CAPACITY-ONLY`;
- `status`: `capacity-preflight-only`;
- `description`: capacity-only wording.

All economic, lifecycle, roster, evidence, timing, and actor fields must be
identical after typed JSON normalization. The launcher records a canonical
delta document for all three arms and binds its SHA-256 digest into the
capacity attestation. A failed delta comparison prevents execution.

## Measurement contract

The Go resource measurer samples every `250ms` until each arm’s simulator and
renderer command has exited. It retains the complete sample vector and its
SHA-256 digest. Each sample includes:

- free bytes on the actual output-parent filesystem;
- apparent and 512-byte allocated bytes below the measured output tree;
- the measured process-tree RSS;
- cgroup current bytes, finite cgroup limit, and OOM counters;
- host `MemAvailable` and swap used.

The measurement also records the mount device, numeric filesystem ID,
filesystem type, mount ID, and filesystem UUID. The output parent and measured
tree must resolve to the same filesystem. Symlinks in the measured tree are a
hard failure.

The aggregate values are recomputed from retained samples:

```text
peak_filesystem_consumption = initial_available - minimum_available
measured_peak = max(peak_apparent, peak_allocated,
                    peak_filesystem_consumption)
required_free = max(2 * measured_peak,
                    measured_peak + 2 GiB)
required_memory = max(ceil(1.5 * peak_process_tree_RSS),
                      peak_process_tree_RSS + 1 GiB)
```

The verifier rejects counter resets, sample gaps above `250ms`, missing
terminal samples, nonzero swap, any OOM counter delta, an incomplete arm, a
nonzero arm exit, missing binary completion trailer, missing renderer
attestation, or a formula mismatch. The final live filesystem check is made
against the actual parent of the future scientific output root, not against
the directory containing an attestation copied from elsewhere.

## Retention and promotion rule

The three capacity evidence trees, resource sample records, delta document,
tool identities, and final attestation are retained until the successor
scientific campaign has passed its evidence-contract and independent-review
requirements. No file is deleted to meet the floor. The preflight authorizes
only a measured capacity floor; it does not authorize the activation probe.

The next sequence remains:

1. finish the fail-closed capacity runner and tests;
2. run the full mechanical gates and targeted race/fresh-process checks;
3. obtain one fresh independent Sol-xhigh review of the exact complete tree;
4. execute this preflight in a finite cgroup and verify its attestation;
5. build clean Go 1.27 provenance-pinned binaries;
6. run only the registered five-minute seed-659 activation probe;
7. review that probe before any 24-hour development cell.

No holdout `619`, `631`, or `641` is permitted before a separate explicit
freeze authorization.
