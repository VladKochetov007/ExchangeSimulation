# Repository consolidation — development baseline

Date: 2026-09-20 (UTC)  
Integration branch: `integration/repository-consolidation-20260920`  
Scientific source selected for integration: `autoresearch/ffa-ecology-gen0`  
Remote: `origin` → `https://github.com/VladKochetov007/ExchangeSimulation.git`

This record reconciles accessible worktrees and relevant refs into a coherent
development baseline. It is an engineering publication record, not a new
market study, scientific freeze, or realism claim.

## Initial topology and recoverability

The initial local and fetched state was inspected before integration:

| ref | commit | tree | disposition at start |
|---|---|---|---|
| `main` / `origin/main` | `ffe1434cfc60b5f79b5289b610d3d5137286f514` | `fa23b65580a64a1743f91cee686583ce99585816` | published baseline |
| `autoresearch/ffa-ecology-gen0` / origin | `f507c7ee2b11fd05e1adb9f17fa0a66faf889ea1` | `86c1ef04e06ce9e48494d5462bd1adea262e9498` | selected scientific closeout baseline |
| `feature/r2-cdf-survival-successor` | `1fda96039e684d51e176e0cef2bcff145f2a6c8a` | `d9a2f71bc2b2c17336454ab514ec2471a9011d26` | preserve historical SV1C line |
| `feature/r2-cdf-survival-successor-sv1d` | `039f008f1aa9f3a748bf3fac33d0bbb7a2e1892e` | `3755b4899512ab78d1533d4eb9986c20b3a3ee8b` | preserve historical SV1D line |
| `origin/autoresearch/latency-race` | `d48e9eeb8fd7d2dbba035571ec499272106bfce3` | `7fd539d58cbbbb9ae0419b3e23ef05f8c1b19a01` | defer separate latency study |
| `origin/autoresearch/v2-performance-research` | `b1847ac40e8b7483e6e8a3f94b3705b4058884b0` | `4061dca610df0bd7e1de335ffccc9613d915841f` | defer performance feed |
| `origin/perf/ffa-gen0-port` | `899a6113ce0ee68bc28afe903ab985eba8bb047b` | `e6e83280ba6d3ea5ca3849748d92b21f94c8739b` | preserve port history |
| `origin/perf/r2-cdf-survival-port` | `39768dfed4ba4a5134f0c5ccf53351a79a0b1d64` | `5514aea7d2086642e3ec7a3b7548ec037dbcaddc` | defer port promotion package |
| `origin/redteam/economic-audit` | `e85e16c5e920382e5df9aa050ac5ff9b22b51661` | `9db2afc4f37daa2adb5165b941a62913b1b6517e` | preserve audit feed; no wholesale merge |

`main` is an ancestor of `f507c7e`; the integration branch was therefore
created from `main` and advanced with `git merge --ff-only
autoresearch/ffa-ecology-gen0`. No history was rewritten and no branch was
force-updated.

The Git common directory, worktree administrative records, submodule state,
LFS state, in-progress operation markers, and process state were inspected.
There are no submodules or LFS-managed files. No merge, cherry-pick, revert,
rebase, build, simulation, or analysis process was active during inventory.
`git fetch origin --no-prune` completed before the integration.

Recoverability artifacts are private and intentionally not pushed:

- named refs under `refs/archive/repository-consolidation-20260920/` retain
  every distinct accessible or missing-worktree commit and relevant remote
  tip; the exact mapping is in
  `/home/vlad/external-scratch/repository-consolidation-20260920/archive-refs.tsv`;
- verified Git bundle:
  `/home/vlad/external-scratch/repository-consolidation-20260920/exchange-simulation-history-20260920.bundle`,
  SHA-256
  `b784628e4c732c8649e198473b5ad46fdd80a16d7d752003f6d3a046ca0b15f0`,
  31 MiB, verified as a complete history bundle;
- tracked dirty patch from `cdf-order-debug-971267d`:
  `cdf-order-debug-971267d-working-tree.patch`, SHA-256
  `3d72b9d46e7a92d6f3a30cf7a393e50a265b009e369ab2e76d4698b07f12cff0`;
- untracked analyzer files from the system-temporary checkout
  `<system-temp-root>/cdf-analysis-side.JACU0e/source` were
  copied without modification, with hashes recorded in the machine summary.

The bundle covers Git history only. It is not a backup of working-tree files,
indexes, ignored evidence, credentials, or external raw artifacts.

## Worktree inventory

All 46 entries returned by `git worktree list --porcelain` were accounted for.
The complete machine-readable path, existence, branch/detached state, full
HEAD, and clean/dirty/prunable status is in
[`artifacts/repository-consolidation.json`](artifacts/repository-consolidation.json).

The important non-clean or inaccessible entries were:

| path | state | preserved contribution |
|---|---|---|
| `/home/vlad/external-scratch/cdf-order-debug-971267d` | exists, detached at `971267d`, tracked `analysis/cdf_activation.go` modified | patch archived; proposed `SimTS` ordering change remains unresolved and unapplied |
| `<system-temp-root>/cdf-analysis-side.JACU0e/source` | exists, detached at `fb965a0`, two relevant files untracked | both files archived by hash; older analyzer variant remains historical/unresolved |
| `<system-temp-root>/exchange-sim-baseline-09d9f18` | missing/prunable, HEAD `09d9f18` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/TIemh2AcJU/src` | missing/prunable, HEAD `39c4554` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-648d408.7anPfo/src` | missing/prunable, HEAD `648d408` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-678ca4d.1C0WNR/src` | missing/prunable, HEAD `678ca4d` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-95e5083.eVYYTK/src` | missing/prunable, HEAD `95e5083` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-bounded.hP4MWL/src` | missing/prunable, HEAD `45cd4c9` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-final.rSiTof/src` | missing/prunable, HEAD `2fa7fbe` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-retry.YlW3dN/src` | missing/prunable, HEAD `7b7bedc` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build-streaming.EgLzVf/src` | missing/prunable, HEAD `79afc70` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-clean-build.CBOwdx/src` | missing/prunable, HEAD `bfd03ea` | commit retained by archival ref; no working files or evidence claimed |
| `<system-temp-root>/v2-r4-build-16a5e91` | missing/prunable, HEAD `16a5e91` | commit retained by archival ref; no working files or evidence claimed |

No existing worktree was reset, cleaned, removed, or pruned. A laptop or
machine not visible to this Git common directory remains outside the inventory
coverage.

## Contribution and provenance map

| origin | disposition | rationale |
|---|---|---|
| `f507c7e` scientific branch | `INTEGRATE` / adopted baseline | current R2/SV1D closeout, calendar, risk hardening, evidence/analyzer contracts, tests, and navigation pointers are the newest coherent development line |
| `ffe1434` `main` | `ALREADY_PRESENT` | retained as the ancestor and publication base |
| SV1C tip `1fda960` | `PRESERVE_AS_HISTORICAL`, `SUPERSEDED_BY_NAMED_CHANGE` | closed negative activation line with divergent intermediate provenance; current closeout records the accepted scope without merging its obsolete state |
| SV1D tip `039f008` | `PRESERVE_AS_HISTORICAL`, `SUPERSEDED_BY_NAMED_CHANGE` | generated gate/config history and prior successor attempts; not a replacement for the accepted closeout baseline |
| detached review/build checkouts (`8de607d`, `5015fd0`, `6cab934`, `85c927d`, `971267d`, and repeats) | `PRESERVE_AS_HISTORICAL` | exact source/review/bundle identities remain reachable; repeated trees do not add current source changes |
| `origin/autoresearch/latency-race` | `PRESERVE_AS_HISTORICAL` / deferred | separate event-driven latency research, not required by the current development baseline |
| `origin/autoresearch/v2-performance-research` | `PRESERVE_AS_HISTORICAL` / deferred | performance and binary-evidence feed; no wholesale adoption under this consolidation |
| `origin/perf/ffa-gen0-port`, `origin/perf/r2-cdf-survival-port` | `PRESERVE_AS_HISTORICAL` / deferred | port experiments and promotion package are not current scientific code |
| `origin/redteam/economic-audit` | `PRESERVE_AS_HISTORICAL` | independent audit findings are retained for future adjudication, not silently promoted into `main` |
| dirty `SimTS` patch at `971267d` | `UNRESOLVED_CONFLICT` / deferred | changes event ordering semantics and lacks a current registered invariant; archived verbatim and not applied |
| untracked analyzer files at `fb965a0` | `PRESERVE_AS_HISTORICAL` / unresolved | older stale-observation experiment; current analyzer already contains later evidence/provenance contracts |

No contribution was selected merely because it was newer. No old
implementation was imported over the current risk, signed-price, calendar,
evidence, or analyzer semantics. Experimental rosters remain controlled by
their existing configurations and are not enabled by the consolidation.

## Scientific state preserved

The current repository continues to state:

- R2/SV1D is closed at a valid non-activation boundary;
- the predecessor is non-viable at the registered 24-hour survival gate;
- the retained seed-659 trajectory belongs to source `971267d`, not `8de607d`;
- `8de607d` is the accepted queued-fill analyzer-correction/rescore candidate,
  with review scope limited to that correction and retained evidence;
- the supplier traded but did not satisfy the registered restoration predicate;
- no formal tri-arm promotion, completed 24-hour successor campaign, freeze,
  or holdout validation follows from the rescore; and
- the proposed counterparty/capacity study is not authorized.

The consolidation does not edit signed historical manifests, old scores,
retained raw evidence, preregistrations, or numerical outcomes. It only adds a
navigation layer and a provenance map around the selected development tree.

## Integration validation

The first documentation checkpoint was `d51dec9`. Its full test exposed a
documentation-only portability defect: literal system-temporary paths in
tracked research files violated `TestTrackedFilesAvoidSystemTempPaths`. The
minimal repair is `24ddb38`; it keeps hashes and private archive references,
but uses configurable/repository-relative placeholders in tracked text. The
exact repaired candidate then passed the following bounded checks:

- `git diff --check`;
- `make test` after inspecting its targets (Go packages, in-memory multivenue
  determinism fixtures, repository path policy, and all four contract/archive
  fixture scripts);
- `go vet ./...`;
- targeted race tests for analysis, command adapters, and `tests`;
- JSON/link/schema consistency checks for the new records; and
- final clean-tree and remote-tip checks.

The machine summary records exit status and SHA-256 for the retained
validation logs. `make test`, vet, race, JSON parsing, and path-policy checks
are all passing at the repaired candidate.

## Independent integration review

An independent read-only Luna xhigh review of candidate
`27afde1da9a438e9b52ac58ba2bae2ef8c422ee7` completed with **ACCEPT**. Its
bounded scope checked ancestry, deferred-branch separation, the 46-worktree
inventory and dirty preservation, closeout boundaries, validation identity,
and publication instructions. It found no lost change, dangerous conflict
resolution, or inflated scientific claim. The review did not approve a market
study, a freeze, or empirical realism; it accepted only this repository
consolidation scope. The final machine summary records the verdict without
inventing a report digest because the reviewer returned a session result
rather than a persisted artifact.

These checks validate source integration only. They do not rerun a market
world, inspect holdout outputs, authorize a campaign, or transfer an old
review verdict to a changed scientific claim.

## Publication and cleanup boundary

Before publication, `origin/main` must still equal the recorded initial
`ffe1434c...`; `main` will then be advanced normally to the validated
consolidation descendant and pushed without force. The integration branch and
all source/research branches remain available; other agents must rebase or
merge deliberately rather than reset to this pointer.

Optional cleanup candidates, **not removed** by this task, are missing/prunable
administrative entries under the system-temporary root, regenerable build/cache outputs, tracked
Python bytecode under `tools/__pycache__`, and old detached worktrees after an
owner review confirms their archival refs and external artifacts. Protected
evidence, review bundles, branches, worktrees, and private material were not
deleted or moved.
