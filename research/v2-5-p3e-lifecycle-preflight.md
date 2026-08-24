# V2-5 P3e lifecycle A/B — non-scoring preflight

Status: **PASS (mechanical preflight only).** These five-minute A/107 and B/107
cells cannot reach the registered 96-hour term end or the lifecycle cutoff and
are excluded from every activation, fill, residual, funding, and closure score.

## Frozen build

| item | value |
| --- | --- |
| experiment source | `4ef83610266e68981f5ea0e4df334e714c2e62f7` |
| `bin/multivenue` SHA-256 | `70303065a0714224bc28322a1972e5c4c6c3f43f783579a06eb4172adb697805` |
| `bin/mvanalyze` SHA-256 | `041112576e84e530e9f852b594d9749ef65e7cc2a3071b97e1447d1b800c1a35` |
| `bin/prunegate` SHA-256 | `3ca77f0f34826ab4aa56d064747a02a6a28b20c9744c491782034e355cdbac13` |
| runtime | seed 107, five simulated minutes, full evidence, `GOMAXPROCS=4` |

The same binaries are frozen for the four full cells. No source change is
permitted between this preflight and full execution.

## Config and serialization checks

`scripts/check-v2-5-p3e-lifecycle-configs.sh` passed. The preflight manifest
for A has no `term_carry_allocator.passive_exit` member. B serializes exactly
`slice_qty=100000` and `deadline_at_nano=1736038805000000000`. Their manifest
SHA-256 values are:

| arm | config SHA-256 | manifest SHA-256 |
| --- | --- | --- |
| A/107 | `bb11a68bec082305d6c5245d1ef47b4f1103f36ea8d001e2d7b6ab0b2dff715f` | `d7aa036daaa594ad56feaa1f60c9171210ce54d943487dbf738bbd3dee6892ae` |
| B/107 | `cfcf9ede724cd9a6b3f2c5f36c1c0e6c0ddc6272d73287295808ae69ad6f21e1` | `3db8cd33dc96166ae47f1bc473d19613cce99fcc20f7dfd83eeda0433916dd78` |

Both cells produced nonempty final `greeks.json` and `latency.json`. The
ordered execution checkpoint is identical at 56,189 observations with hash
`f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2`.
This proves that declaring the dormant P4 policy did not alter the short
preflight execution trajectory.

## Evidence and extractor behavior

All ten registered metric files were produced. Receipt, term-carry,
derivative, conservation, position, order-lifecycle, stream-hash, and exact
artifact checks passed for both cells. Runtime and offline exact persisted
artifact identity matched:

| arm | exact records | exact artifact digest |
| --- | ---: | --- |
| A/107 | 56,648 | `6720b21c194cecd74f293043d5d922024618c70d61ba1cb951ef8c19a2d66ee2` |
| B/107 | 56,648 | `6906866f4af89e05e9b23a68a1733f6b5ea6cc11c93942d90c5af678a1a65e71` |

The persisted digests differ because A and B deliberately serialize different
policy evidence; they are not execution hashes.

The lifecycle wrapper exited nonzero for both short cells, as required. Each
`termcarrylifecycle.json` has schema version 1, zero eligible terms, and exactly
one integrity row: `analysis_deadline_not_observed`. Its observation end is
`1735689900000000000`, before the registered
`1736038805000000000` cutoff. No other integrity failure is present. This is
the preregistered fail-closed response to a censored preflight, not an
experiment verdict.

Raw preflight evidence remains uncommitted and retained under
`research/artifacts/v2-5-p3e/preflight-lifecycle-{A,B}-107/`. It has no prune
authority.
