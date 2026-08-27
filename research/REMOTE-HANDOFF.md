# V2 research handoff

Generated for transfer to a second machine. This is a provenance and
continuation record, not a new scientific result. The persistent objective is
the V2 artificial-market program in the accompanying `goal-objective.md`
inside the private state archive (and in the Codex attachment on the original
machine).

## Exact repository state at handoff

- Repository: `/home/vlad/development/exchange_simulation`
- Branch: `autoresearch/ffa-ecology-gen0`
- HEAD: `7f563665872a739961eed644f8633b87c6e44538`
- HEAD subject: `docs(research): checkpoint integrated gate`
- Public remote: `github`
  (`https://github.com/VladKochetov007/ExchangeSimulation.git`)
- Current branch already equals `github/autoresearch/ffa-ecology-gen0`.
- Local `main`: `ffe1434`; it is ten commits ahead of
  `github/main` (`99961760ee17ad7831810bf40f094f6df1b7d1d0`).
- No simulator, analyzer, extractor, or long-run process is active at the
  time of this handoff.
- Four tracked files have pre-existing user-owned working-tree edits and are
  intentionally not staged by the research branch:
  `research/artifacts/scoreboard/f2_baseline_101/derivatives.json`,
  `exposure.json`, `reaction.json`, and `streamhash.json`.
  Their exact diff is in the private archive as `working-tree.patch`.
- All untracked retained evidence is preserved. Do not run a broad `git add .`
  or delete untracked artifacts on the remote machine.

The two commits immediately preceding this handoff are:

1. `30176e0` — hardened integrated long-run provenance, registered GOMAXPROCS
   and 24-hour identity, copied-config execution, and atomic completion
   status.
2. `5af4573` — validated clean binary provenance for long-run runs.

## Scientific position

The ae13f9a frozen autopsy is historical and must not be reopened or retuned.
Signed-price migration is complete (`320262e`, hardened at `5afdd45`, final
closure `7644b2`). P3e P0 and the fresh P3e lifecycle screen are complete;
P3e lifecycle is screening-level support for its finite-term execution
contract only. P4 funding/carry, P5 dated carry, P6 option emergence, and
P7d distress results remain bounded by their recorded preregistrations; do not
rewrite their negative, inactive, or not-identified conclusions.

The accepted integrated reference smoke is compatibility evidence only. It
uses source revision `b312336`, has six execution checkpoints and 352,099
execution observations, and execution-hash prefix
`9d40fd652c0c291079332dbe70cfd33bc744f991cdb5da4ca6bdc730803a4e01`.
Its persisted evidence digest is
`3dc2ac37e4e0d594eb17fada96cdb29ebc2bc2ad82a2caf714019a55fb5d66d2`.
It does not license a market-level realism or funding claim.

The next licensed gate is the registered integrated V2 24-hour candidate
screen, but it has **not** started. The protocol and configs are committed in
`75c10df` and `4167a95`; the extractor is in `fe4d88e`; the runner hardening is
in `30176e0`. Before spending long-run compute, the independent Sol-xhigh
review required further pre-run hardening:

- add/verify the explicit `dev-607-g8` parity identity and G4/G8 enforcement;
- parse every required JSON artifact and reconcile metadata, config, seed,
  cell, hypothesis, and analyzer VCS identity;
- require a clean analyzer build whose embedded revision equals the declared
  analysis revision;
- add bounded-integer conservation tolerances and late-path activation;
- require order-lifecycle, position, settlement, expiry, derivative, margin,
  and liquidation integrity predicates;
- require CDF collateral borrowing and zero `PRICE_UNAVAILABLE` rejection
  activation in the candidate composition;
- classify disabled P4/P5/P3 recorders as `OUT_OF_SCOPE` /
  `RECORDER_NOT_ENABLED`, not as inactive mechanism exercise;
- precommit the three-development-cell aggregate scorer plus exact seed-607
  full-G4/no-log-G4/full-G8 parity checks.

No outcome may be interpreted until that contract is committed and the
independent reviewer accepts it. Do not consume reserved holdouts
`619/631/641` before an immutable freeze authorizes them. Candidate development
seeds are `607/613/617`; `601` is historical smoke only.

## Performance track

Performance work belongs on separate worktrees/branches and cannot alter a
running scientific cell. The analyzer fused-decoder prototype was rejected in
`fccde0e`: it was about 22% faster on valid replay, but differed on malformed
input and duplicate-key semantics. Keep `encoding/json` as the semantic
reference. The evidence-integrity branch is `6f61ad3`; the scan branch is
`c40bb31`; neither is merged into the scientific branch. Re-profile only after
the pre-run contract is stable. Never implement an unbounded RAM log buffer;
measure byte rate, backpressure, durability, and shutdown semantics first.

## Resource and evidence state

At handoff, free disk was approximately 237 GB on a 1.3 TB Btrfs volume. The
working tree contains about 1.3 TB under `research/artifacts`, 963 GB under
`logs`, and 54 GB under `scratch`; these are retained private evidence and are
not suitable for GitHub. Rebuildable `.cache`, `.venv`, `bin/`, test binaries,
and old compressed cache files are omitted from the private transfer archive.
The archive creation is bounded to the named evidence paths and does not
remove the originals.

Current checked-in simulator/analyzer binaries under `bin/` are not a valid
provenance source; rebuild from a clean clone after the final pre-run commits.
The runner requires `vcs.modified=false`, an embedded revision equal to the
current repository HEAD, copied `run-config.json`, a fixed 24-hour horizon,
registered GOMAXPROCS, both final sidecars (`greeks.json`, `latency.json`), and
an atomic `run-status.json` written only after successful completion.

## Safe continuation procedure

1. Extract the private archive at the repository root (or use its recorded
   `private-state/` directory) and read this file plus `goal-objective.md`.
2. Verify the Git branch and HEAD above. Preserve the four dirty scoreboard
   edits; they are historical user work, not a reason to reset the tree.
3. Inspect and commit protocol hardening, extractor hardening, and the
   precommitted candidate scorer as separate logical commits. Run `bash -n`,
   `git diff --check`, focused tests, and `go test ./...`.
4. Ask the independent Sol-xhigh reviewer to review the preregistration,
   exact config diff, runner, extractor, scorer, and evidence contract. Do not
   launch a 24-hour cell until the review is ACCEPT or ACCEPT WITH NARROWER
   CLAIM.
5. Rebuild `multivenue`, `mvanalyze`, and `prunegate` in a clean clone with
   embedded VCS metadata. Record hashes, Go version, config hashes, and
   analysis revision.
6. Run only registered development cells sequentially, with disk/RAM checks
   between cells. Extract complete evidence before any pruning. Score only the
   precommitted candidate predicate; this screen is not a realism claim.
7. After candidate review, reassess freeze readiness. Keep P4/P5/options/
   timing/distress limitations explicit rather than tuning them to pass.

## Remote Codex continuation prompt

Give the remote Codex this command after extracting the private archive:

```text
Read /path/to/exchange_simulation/research/REMOTE-HANDOFF.md and
/path/to/exchange_simulation/private-state/goal-objective.md completely.
Treat the handoff and the persistent objective as the active V2 research goal.
Verify Git HEAD, branch, dirty files, private evidence archive, disk and RAM
before acting. Continue from the saved integrated-long-run gate: harden and
commit the protocol, fail-closed extractor, and precommitted scorer; obtain an
independent Sol-xhigh adversarial review; rebuild clean provenance-pinned
binaries; then run only the registered development cells 607/613/617 plus the
607 G4/G8/no-log parity controls. Do not consume holdouts 619/631/641 before
freeze authorization. Preserve all historical results and do not reopen
ae13f9a, P3e, signed-price, P4/P5, P6, P7d, or the mixed timing line without a
new concrete regression. Use at most 70% CPU and 20 GB RAM, monitor disk, and
never delete evidence before its measurement contract passes. Keep performance
work in a separate worktree. Continue autonomously through the V2 freeze and
holdout-validation program described in the objective, with independent review
at every meaningful scientific gate.
```

The archive manifest and SHA-256 are recorded separately when archive creation
finishes. The archive is intentionally outside the Git repository so it is
not accidentally pushed to the public remote.
