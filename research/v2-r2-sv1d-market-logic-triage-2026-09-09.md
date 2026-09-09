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
