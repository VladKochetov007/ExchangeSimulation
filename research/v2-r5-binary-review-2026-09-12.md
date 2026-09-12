# V2 successor binary-evidence review closure — 2026-09-12

This note is append-only scientific process history. It does not rewrite the
predecessor R2 result or promote the SV1D successor.

## Reviewed candidate

- Scientific branch: `autoresearch/ffa-ecology-gen0`
- Exact reviewed tree: `81a66653108afc5458a9a0a3a8b61bce807dc898`
- Independent reviewer: Sol-xhigh, `gpt-5.6-sol`, review agent
  `01a095e9-46e1-7c42-b364-a46f0d7908ab`
- Verdict: `REJECT`
- Performance feed: `origin/autoresearch/v2-performance-research`, reviewed
  through `b1847ac`; a subsequent fetch found no newer commit.

The review found two promotion blockers:

1. The private capacity `--internal-arm` accepted a forged environment marker
   plus a caller-opened descriptor for the expected lock path. The direct
   reproduction reached `run_capacity_arm` and created its arm metadata before
   failing at a later sentinel (`exit 95`). The existing descriptor and flock
   checks did not authenticate the handoff from the resource wrapper.
2. CDF decision schema v4 accepted contradictory numeric presence: an absent
   optional value could carry nonzero wire bytes, and a present optional value
   could carry zero. Both forms were silently normalized by decoding.

## Closure

- `0d9c5e1 fix: reject noncanonical CDF numeric presence` validates every v4
  optional numeric field before normalization and adds both contradiction
  regressions.
- `11da855 fix: require trusted SV1D arm handoff` adds an opt-in
  resource-wrapper child handoff on a pipe-backed FD3, binds the token to the
  actual `sv1dresource` parent PID, rejects direct forged lock-marker entry,
  and updates the capacity runner to request the handoff. The outer namespace
  flock remains held by the waiting capacity runner.

The end-to-end adapter probe passed through the handoff and reached only the
controlled missing-sentinel failure with harmless fixture binaries. The
control-plane regression rejects direct forged entry before creating the arm
directory.

## Exact-tree verification after closure

At `11da855`, all of the following passed with `GOMAXPROCS=2
GOMEMLIMIT=4GiB`: focused `evstream`, `types`, `exchange`, `analysis`,
`cmd/sv1dresource`, and `simulations/multivenue` suites; clean `make test`
(including integrated-long-run, R2, archive, and parity contracts);
`go vet ./...`; the prescribed targeted race matrix; changed evidence-package
race coverage; fresh-process baseline/binary/log-mode/perp-exposure
determinism checks; shell syntax; the R2 control contract; and `git diff
--check`.

No economic code, R2 calendar, SV1D roster/configuration, historical artifact,
capacity attestation, campaign binary, development cell, freeze, or holdout
changed. Holdouts `619/631/641` remain untouched and unauthorized.

## Next boundary

The next action is one fresh independent Sol-xhigh review of exact tree
`11da855`, covering R2 calendar/lifecycle semantics, current correctness
hardening, binary evidence and rendering, strict scoring/provenance, resource
manifests, and both lock entrypoints. Until that review accepts, do not run
binary capacity, build campaign binaries, run the seed-659 activation probe,
or launch `dev-607`.
