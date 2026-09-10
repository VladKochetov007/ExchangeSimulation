# V2 R2 SV1D market-logic triage — 2026-09-09

Status: append-only successor-gate record. The SV1C/R2 predecessor remains
closed as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**. SV1D has not run a
world, activation probe, development cell, freeze, or holdout. Holdouts `619`,
`631`, and `641` remain untouched.

## Exact identities and asynchronous feed

The scientific tree reviewed for this checkpoint is:

```text
branch: feature/r2-cdf-survival-successor-sv1d
HEAD:   ad9a6e96f6f4b39013c5564fb8b2a12cc46950ac
```

The performance/red-team remotes were fetched with `git fetch origin --prune`
without switching worktrees. The newest reviewed refs are:

| ref | revision | disposition |
| --- | --- | --- |
| `origin/autoresearch/v2-performance-research` | `b1847ac40e8b7483e6e8a3f94b3705b4058884b` | no newer commit; binary-evidence and performance work deferred |
| `origin/perf/r2-cdf-survival-port` | `39768dfed4ba4a5134f0c5ccf53351a79a0b1d64` | independent CDF/performance findings; no wholesale merge |
| `origin/redteam/economic-audit` | `e85e16c5e920382e5df9aa050ac5ff9b22b51661` | audit feed; findings independently adjudicated |

No performance implementation or evidence-format migration was imported into
SV1D. The current scientific evidence contract remains `evstream_v3`.

## Independent review input

Sol-xhigh reviewer Curie inspected the exact pre-fix SV1D risk/mark tree at
`9c2ab23` and rejected promotion pending focused corrections. The review was
read-only and produced no simulator evidence. Its findings and dispositions
are:

| finding | invariant | disposition at `ad9a6e9` |
| --- | --- | --- |
| lifecycle risk used the epoch returned before expiry processing | risk must consume the latest complete mark set after lifecycle callbacks | **fixed**: deterministic and threaded expiry paths now call the public current-epoch option-risk sweep after `CheckExpiries` |
| strict snapshot path called live mutable maintenance first | a nonzero committed epoch must use only the five-argument snapshot method | **fixed**: live maintenance is selected only for the legacy `markEpoch == 0` path; strict profiles never call it |
| terminal maintenance validation could overlap a mark producer | terminal validation must be excluded from an in-flight mark pass | **fixed**: `ValidateMaintenanceAtCurrentMarks` takes `markPassMu` before `e.mu.RLock` |
| caller-owned `CommitMarkEpoch` cannot lock arbitrary external instrument state | external integrations must finish mark/configuration writes before commit and avoid concurrent mutation during the commit/risk boundary | **known API condition**: the contract is documented by the existing caller-owned API; no current scientific path uses unsynchronized external producers |
| providers/calculators/hooks must not re-enter exchange mark APIs while the pass lock is held | a callback must be pure/non-reentrant under the declared automation contract | **known API condition**: current providers/calculators/hooks are read-only and non-reentrant; a dedicated contract review remains required before external integrations are promoted |
| exported option mark/configuration fields are mutable | option risk configuration must be frozen or synchronized while a snapshot is being evaluated | **not activated in SV1D**: no external concurrent option mutation is used by the registered world; retain as an integration limitation rather than silently changing economics |

The three high-risk findings were repaired in the single semantic commit
`ad9a6e9`. The regression
`TestStrictRiskUsesOnlyCommittedPositionMaintenanceSnapshot` proves that the
strict path uses the immutable snapshot method and never calls mutable live
maintenance. Existing mark-producer serialization coverage remains in
`TestMarkProducersSerializeRiskEpochCommit`.

## Historical and scientific impact

These findings were discovered before any SV1D trajectory existed. They could
have affected a hypothetical SV1D liquidation/terminal decision, so the
candidate was not allowed to proceed from `9c2ab23`. No retained historical
R2, SV1C, P3e, P4/P5, P6, P7d, or mixed-timing result is rewritten or reopened.
The repairs change risk synchronization and strict evaluation safety; they do
not change the R2 calendar, supplier roster, actor economics, or historical
evidence hashes.

The older F1/F2/F3/F6/F8 adjudications and historical-impact matrix remain in
`research/v2-r5-market-logic-triage-2026-08-30.md`. The performance branch's
binary evidence prototype is not an oracle for those claims and is deferred
until its separate promotion contract is complete.

## Mechanical evidence at this boundary

After the semantic patch, the exchange package and the focused regressions
passed with Go 1.27 under `GOMAXPROCS=2` and `GOMEMLIMIT=4GiB`. The earlier
bounded focused package gate also completed successfully for `evstream`,
`types`, `exchange`, and `simulations/multivenue` in 1,014.794 seconds, but it
predated this patch and is not treated as the final exact-tree gate. A clean
full `make test`, `go vet ./...`, targeted race matrix, and evidence-contract
matrix remain required.

No activation artifacts, binaries, capacity measurement, review attestation,
development cell, or holdout were created from this checkpoint. No retained
evidence was deleted.

## Gate decision and next action

Classification: **SEMANTIC PATCH COMMITTED; NOT PROMOTED**.

The candidate may proceed only through the following bounded sequence:

1. run the clean full test/vet/race/evidence-contract gate on `ad9a6e9`;
2. complete the activation-contract review, including the known callback and
   external-mark ownership conditions;
3. build the registered Go 1.27 provenance-pinned binaries and generate the
   seed-659 treatment, mode-off, and no-roster manifests;
4. obtain one fresh independent Sol-xhigh review of the exact complete tree;
5. if accepted and capacity is safe, run only the five-minute seed-659
   activation probe; and
6. independently grade that probe before considering any full development
   cell.

No holdout is authorized by this note.

## Post-review contract repair checkpoint — 2026-09-10

An independent Sol-xhigh review by Schrodinger inspected the immutable
pre-repair candidate at `2d31abb` (the review was read-only and did not run a
world). It rejected promotion on three contract findings:

| finding | intended invariant | repair |
| --- | --- | --- |
| terminal-negative comparison classification was unreachable because the scorer first required `valid == true` | a valid, reconstructible terminal valuation failure is valid negative evidence even though it makes no activation claim | `v2_r2_sv1d_classify_comparison` recognizes `UNAVAILABLE_TERMINAL_FAILURE` only when evidence/provenance are valid; the scorer consumes that single classifier and binds its published status to the embedded comparison |
| capacity attestation closed only the cell while the measured namespace was the probe root | every measured direct root artifact must be enumerated and integrity-bound | the capacity runner and validator require the exact root shape (one cell plus `config.stderr.log`) and hash the root-level normalization stderr |
| scorer provenance claims were not all bound to the embedded comparison | status, activation, paths, hashes, exit status, validity, and terminal-negative claims must agree with the comparison actually scored | scorer-side coherence validation now compares all published top-level and nested comparison claims against the reconstructed comparison and actual files |

The rejected state was not given a review attestation and did not authorize
capacity, activation, development, freeze, or holdout execution. The minimal
semantic repair is committed in `789e2bf`; the deliberately retired and
regenerated SV1D artifacts are separate mechanical commits. It preserves the
R2 calendar, finite supplier economics, historical JSON evidence, and the
SV1C predecessor's negative result.

At the repaired candidate, before the final documentation-bound regeneration:

- the SV1D config checker and expanded activation-contract fixture passed;
- `go vet ./...` passed;
- the uncached focused suite passed: `evstream` 0.178 s, `types` 0.012 s,
  `exchange` 2.662 s, and `simulations/multivenue` 1002.515 s;
- race gates passed for `exchange` (16.457 s), `tests` (42.743 s), and the
  selected evidence/calendar/risk/CDF multivenue matrix (5.311 s);
- the asynchronous performance, CDF-port, and economic-audit refs were
  fetched at their recorded tips and had no newer commits;
- no capacity probe, seed-659 activation probe, development cell, 24-hour
  world, freeze, or holdout was run.

The default ten-minute `make test` behavior remains a separately measured
runtime concern because the existing fresh-process multivenue test exceeds
that limit; the final candidate gate must record the explicit longer-timeout
full make invocation and its result. The final exact-tree Sol-xhigh review is
still required before any capacity measurement or activation probe.

## Final uncached mechanical gate before exact-tree review — 2026-09-10

The exact scientific candidate tested here is clean HEAD
`da57f8e28f89b6c51615ecec513e763fff352439` on
`feature/r2-cdf-survival-successor-sv1d`. The asynchronous performance,
CDF-port, and economic-audit refs were fetched again; none advanced beyond
`b1847ac`, `39768df`, and `e85e16c`, respectively. No code from those feeds
was merged.

The uncached command
`PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local GOMAXPROCS=2
GOMEMLIMIT=8GiB GOFLAGS="-count=1 -timeout=30m" make test` passed with exit
status zero. The long multivenue package took `1000.377s`. The apparent SV1C
failure lines are expected negative-fixture diagnostics; the complete shell
contract matrix and integrated-long-run archive checks passed.

Before this gate, the independent-review repair history exposed two further
producer/consumer contract issues. Commit `69a6a60` added the missing
`supplier_volume_share` field to the accepted comparison fixture. Commit
`4b9c08a` fixed the cardinality predicate to apply `length` to the validated
arrays, not to their enclosing objects. The generated artifacts were retired
and regenerated in `655c89`/`c45aa67` and `1fb69ac`/`da57f8e`. The final
config checker and SV1D contract tests now pass, and the repairs do not alter
economic mechanics or historical identities.

This is a mechanical gate only, not an independent scientific acceptance.
The exact-tree Sol-xhigh review remains outstanding. Until it accepts the
complete tree, no capacity probe, seed-659 activation probe, development
cell, freeze, or holdout is authorized. Holdouts `619`, `631`, and `641`
remain untouched.

## Exact-tree review packet — 2026-09-10

The generated-artifact candidate `e592a1d23fa2d330a97927c673e23b3e3c3e4da6`
passed the SV1D checker, expanded activation-contract fixture matrix, SV1C
contract fixtures, shell syntax checks, `git diff --check`, and `go vet ./...`.
Its clean Go 1.27.0 binaries were CGO-disabled, provenance-pinned, and
reported `vcs.modified=false`; their SHA-256 identities were recorded in the
implementation checkpoint. The targeted exact-tree race matrix passed for
`exchange` (`16.411s`), `tests` (`42.867s`), and the selected multivenue
evidence/calendar/risk/CDF tests (`5.283s`). The exact fresh-process
determinism/evidence-neutrality pair passed in `128.837s`. No activation or
holdout world ran.

The independent review chain is intentionally not hidden: Tesla rejected the
earlier exact tree at `bb436a2` / `46377d44619f687fd1234679067540d9d896f3a0`
because the runner/scorer producer-consumer boundary omitted
`evidence_valid` and `terminal_negative`, with no producer-shaped fixture.
The scientific branch accepted and repaired those findings in `988ba59`,
completed the comparison fixture in `69a6a60`, and corrected a real jq
cardinality bug in `4b9c08a`. The full uncached repository test then passed at
the preceding exact source candidate, with the long multivenue package taking
`1000.377s`. These changes are contract/provenance corrections only; they do
not alter R2 economics or historical evidence.

The packet is still **NOT PROMOTED**. One fresh independent Sol-xhigh review
of the complete final tree is required before the binary-capacity probe. No
capacity measurement, seed-659 activation probe, development cell, freeze, or
holdout is authorized; holdouts `619`, `631`, and `641` remain untouched.

## Final exact-tree uncached repository gate — 2026-09-10

The complete generated candidate at `b1f81cba9d9b2c7ca3e7139b0ba90d96a43a8fdb`
passed the uncached full repository gate. The `simulations/multivenue`
package took `1003.940s`, the complete shell contract/archive matrix passed,
and `FULL_MAKE_FINAL_B1F81CB_EXIT=0` was recorded. This closes the local
mechanical gate only; it is not a review acceptance and no world or evidence
artifact was created.
