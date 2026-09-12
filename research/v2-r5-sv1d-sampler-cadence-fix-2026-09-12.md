# SV1D resource-sampler cadence correction — 2026-09-12

This is an append-only promotion-gate record. It does not alter the R2
economic model, the SV1D participant model, registered experiment configs, or
historical evidence.

## Rejected exact-tree review

The independent Sol-xhigh review of exact scientific commit `f258af9`
(`14670c42a06ff4fc88d62d11b85979bb2fdc59ef`) returned `REJECT`. Reviewer
`McClintock` (`01a0964e-ef84-7821-b711-7a4f3c7feb89`, `gpt-5.6-sol`) found
`SV1D-RSRC-001`: the resource sampler used a 250 ms ticker, but recorded each
sample timestamp after filesystem, process-tree, host-memory, and cgroup
collection. Ticker delivery and collection latency could therefore produce a
strict-contract violation.

The finding was independently reproduced on the scientific tree with a
non-campaign command only:

```text
command: sv1dresource -sample-interval 250ms ... -- /bin/sleep 3
sample_interval_nano: 250000000
maximum_sample_gap_nano: 250894382
sample_count: 15
exceeds_contract: true
```

This was a reachable operational measurement defect, not a simulator or
historical-result defect. No capacity arm, activation probe, development cell,
freeze, or holdout had run, so the historical-impact classification is
`CONDITION IMPOSSIBLE` for all retained scientific runs.

## Intended invariant and correction

`SampleInterval` is the registered maximum permitted gap between the actual
observations. Actual timestamps remain taken after collection so the resource
record cannot claim data that were not observed. The sampler now uses a fixed
deadline schedule at half that interval, leaving headroom for collection and
scheduler latency. If collection overruns a deadline, the next deadline is
rebased instead of allowing an unbounded backlog; the strict validator still
rejects any observed gap above the registered maximum.

The correction is `5cd7b4d` (`fix: schedule SV1D resource samples with deadline
headroom`). It changes only `analysis/sv1d_resource.go` and its regression test;
the capacity contract remains `250000000` ns and no economic behavior changes.

## Verification after correction

- focused `go test ./analysis ./cmd/sv1dresource -count=1`: pass;
- the new real multi-tick strict-validation regression repeated five times:
  pass;
- independent three-second CLI reproduction after the fix: 26 samples,
  maximum gap `125773682` ns, strict-contract comparison `false`;
- clean bounded `GOMAXPROCS=2 GOMEMLIMIT=4GiB make test`: pass; the expensive
  multivenue package completed in 205.194 s;
- `GOMAXPROCS=2 GOMEMLIMIT=4GiB go vet ./...`: pass;
- targeted race matrix for analysis, SV1D CLIs, gate tools, and tests: pass;
- fresh-process execution determinism, binary-evidence determinism/neutrality,
  and perp-exposure determinism/neutrality: pass;
- focused binary evidence tests in `evstream/exsim`, `exchange`, `types`, and
  `simulations/multivenue`: pass;
- `git diff --check`: pass.

The performance feed was fetched from `origin` through last-reviewed commit
`b1847ac`; no newer commit was present and no performance implementation was
imported. No capacity measurement, pinned campaign build, activation probe,
development cell, freeze, or holdout `619/631/641` was consumed.

## Remaining boundary

The current exact tree is clean and pushed at `5cd7b4d`. The promotion gate
remains closed until one fresh independent Sol-xhigh review accepts the
complete exact tree, including R2 calendar/lifecycle semantics, correctness
hardening, binary evidence, strict scoring/provenance, resource manifests,
locking, and the corrected sampler. Only acceptance can authorize the binary
capacity preflight and subsequent seed-659 activation probe. `dev-607`,
`dev-613`, `dev-617`, freeze, and holdouts remain unauthorized.

## Append-only review-availability stop — 2026-09-12

The follow-up review was attempted twice against the documentation-complete
exact tree `82f452028740d535253ce2d822be750250466ff2` (tree
`ebd2e711618eb421ec8cb66c1828ed779f6388cd`) and both Sol-xhigh launches failed
before inference with backend HTTP 403. No verdict is recorded. The candidate
remains unpromoted; no capacity, activation, development, freeze, or holdout
run occurred. Retry the same complete-tree review when the independent review
service/quota is available.
