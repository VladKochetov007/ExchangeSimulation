# ME-014 Research Continuation Status — 2026-10-15

Owner-directed stop point: this note closes the current work session. No new analysis, code, review, or experiment is authorized by this status record.

## Current branch and identity

- Published development baseline: `main` at `d654d323e777505c7d64cac065d29fa288cc926e` when this research branch was created. `main` was not changed.
- Current branch: `research/me014-public-composition-20261015`.
- Current branch tip before this status note: `57fe699d6883af56f72b5894e34c98b78525e7ef`.
- Research branch changes are test/documentation-only and concern external composition of existing public exchange APIs. They do not change production economics, defaults, evidence format, or historical results.

## Work completed

The branch first added a public-consumer risk-composition fixture and closed a narrow API question: with deterministic exclusive phase ownership, callers can construct coherent marks across venues, preflight risk inputs before sweeping, and prevent a later-time phase error from pumping more venue phases at that timestamp. This is plumbing evidence only; the fixture does not establish the production mark-selection policy or a complete runner frontier. Its accepted scope and earlier reviewer boundaries are recorded in the [ME-014 public-composition probe](../program/ideas/ME-014/public-composition-probe-20261015.md).

The latest increment added an external-package test for public funding snapshot/preview composition across repeated synthetic terms. With fixed positions `+1, +1, −2`, 1 bp per term, mark 100, and an external finite reserve of 3 quote atoms, client cash/reserve were `100/100/100 + 3` initially, `100/100/101 + 2` after 50 terms, and `99/99/102 + 3` after 100. At a mark of 10,000 with zero payer cash, the preview identified both long payers and rejected before the test adapter mutated account or caller-owned state.

That result is strictly a deterministic arithmetic/composition probe. It does not run the runner's verified pre-ingress phase, serialize against concurrent exchange mutations, post an evidence-journaled payment, or bind client balances, venue reserve, accruals and settlement schedule in one production transaction. The test's reserve is caller-owned, outside `ExchangeBalance`; its `MutatePerpBalance` operations are only serialized fixture writes. It does not replace or validate the legacy `SettleFunding` path, and it establishes no market or carry outcome. Full limits and reproduction are in the [funding composition probe report](funding_public_composition_probe_20261015.md).

## Independent checks and validation

- Sol-6 medium reviewer Lovelace: scoped review of the risk-composition and later-timestamp phase-stop fixture; the identified phase-count evidence limitation is stated in that probe record.
- Sol-6 medium reviewer Volta: scoped review of external API composability and mark/phase assumptions; no production mark policy or complete runner frontier was accepted.
- Sol-6 medium reviewer Bernoulli: **ACCEPT** of the funding probe's accounting arithmetic and claim boundaries; no must-fix finding. The reviewer did not run tests or simulations.
- On the funding candidate, `GOMAXPROCS=2 go test -race ./research/me014composition -count=1`, `GOMAXPROCS=2 go vet ./research/me014composition`, `GOMAXPROCS=2 go test ./...`, and `git diff --check` passed.
- `GOMAXPROCS=2 GOFLAGS=-p=2 make test` passed with exit 0 on clean commit `c3d1fee806f5361a77dede5c4ad865d659e041fb`. All Go packages and the integrated-long-run, R2, and archive contract scripts passed. The `sv1dresource` incomplete-measurement exit-7 line is an expected negative fixture inside a passing contract script.
- No economic world, capacity run, holdout, new seed, or historical rescore was launched. No holdout outcome was inspected.

## Separate E2 worktree observed at stop

Read-only repository state inspection found `/home/vlad/ExchangeSimulation-e2-atomic-payment-20260927` clean at `feature/e2-atomic-payment-20260927`, commit `b43e8eb86d0f87c0f8b2b82ad1986f16e2a8b70f`, tracking its matching origin branch. Its visible history includes `f53abf00` (`feat: add joint funding settlement coordinator`). Its tmux test pane showed `GO_TEST_EXIT=0` for `GOMAXPROCS=4 GOFLAGS=-p=2 go test ./...`; no Go test process was active at the time checked.

That branch was **not** inspected at source-diff level, independently reviewed, merged, or treated as accepted by this work. Its existence means the funding commit gap documented by this probe must be read as a limitation of the tested `main`-based probe, not as a claim that no other local candidate exists. The other worktree and its evidence remain untouched.

## If work resumes later

1. Independently inspect and review the exact E2 atomic-payment candidate and its runner, evidence, lock-order, and failure semantics; do not merge it wholesale based on its branch name or test pane.
2. Reconcile any accepted implementation with the authoritative ME-014 readiness ledger and its unresolved gates, including actual `NewSim` wiring, same-time two-venue ordering/atomicity, sealed canonical payment reconstruction, post-payment risk order, and finite carry lifecycle.
3. Only after those readiness and protocol gates are satisfied may a separately authorized, reviewed development protocol consider an E2 world. This note authorizes no such work.

The R2/SV1D line remains closed at its recorded non-activation boundary. Long-horizon market-ecology claims, ME-014 readiness, a completed E2 baseline, freeze, and holdout validation remain unestablished.
