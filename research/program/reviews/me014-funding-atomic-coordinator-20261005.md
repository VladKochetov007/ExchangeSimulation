# ME-014 joint funding settlement coordinator — bounded mechanics review

## Review execution and verdict

Execution: **COMPLETED**. Verdict: **ACCEPT, scoped to the source-level joint settlement mechanics checkpoint**.

Independent reviewer: Sol-6 medium, fresh read-only reviewer context `01a10ce6-6bc4-7941-b3b2-b0eee6affa4c`. The reviewer did not implement or edit the candidate and did not run tests, simulations, or holdouts.

Reviewed source tree: `16dc862adf3a92701d5ec55533a65d270499a07d`, based on source commit `d654d323e777505c7d64cac065d29fa288cc926e`. This tree contains the candidate code/tests and the two-line prospective settlement-contract clarification. The final review was returned against this exact worktree content; no source changes followed it. This review record and readiness update were added afterward and are not themselves part of the reviewed source tree.

## Review history retained

Two earlier review passes rejected intermediate source states. The first identified a nil exchange-ledger dereference and acceptance of arbitrary nonempty `RATE_UNAVAILABLE` reason text. The next identified short-circuiting after a one-sided spot book, which could conceal malformed visible quotes on the other book, and replay acceptance of unknown unavailable-reason strings. The candidate now checks both venue books and all remaining displayed quotes before applying the registered economic unavailability classification, uses the v2 reason enum in source/replay validation, and rejects missing/malformed reserve state before append. Regression tests cover those cases.

The review discussion also raised classifying crossed books as structural. The preregistered contract explicitly treats a crossed/non-positive displayed pair as `RATE_UNAVAILABLE`; that classification is retained. Structural tick, time, venue, source, and contradictory-record errors take precedence and fail closed. No post-outcome economic choice was made.

The final read-only pass accepted the corrections around remaining-quote tick/time validation, unavailable-reason replay validation, reserve guards, and global append-receipt sequence validation. This is a mechanics acceptance, not an economic or production-pipeline approval.

## What the candidate implements

`exchange.SettleFundingEpoch` accepts a frozen registration list and the due attempt set for a common instant. It sorts venue/perpetual identities, locks registered exchanges in stable order, verifies due-calendar/request equality, validates the prior source window and current boundary, captures venue-local account/remainder/reserve state, and prepares whole-venue previews. It submits all due venue records to one injected canonical appender call. It checks receipt count and monotonic event-sequence ranges beyond the observed source frontier before applying any posted cash, fractional remainder, or reserve change. Request permutation cannot change the canonical venue order.

An unavailable source window is recorded as an unavailable attempt with no derived payment terms and advances the calendar cycle without catch-up. An insufficient payer balance is an economic venue-local terminal outcome, not a structural source error; it does not suppress a solvent coincident venue's payment. Structural source, arithmetic, reserve, append, receipt, or clock failures prevent cash mutation and leave settlement state failed in memory.

The prospective contract and selected rate/mark/calendar rules remain unchanged except for explicitly documenting pair-wide structural-error precedence and the v2 unavailable-reason codes.

## Validation record

On the reviewed source tree:

- Toolchain: `go1.27.0 linux/amd64`.
- `GOMAXPROCS=4 GOFLAGS=-p=2 go test ./...`: **PASS**, exit 0.
- `GOMAXPROCS=2 go vet ./...`: **PASS**, exit 0.
- `GOMAXPROCS=2 go test ./instrument ./exchange ./simulations/multivenue -run 'Funding' -count=1`: **PASS**.
- `GOMAXPROCS=2 go test -race ./instrument ./exchange ./simulations/multivenue -run 'Funding' -count=1`: **PASS**, exit 0.
- `go run ./.agents/skills/market-ecology-research/scripts/validate.go --root .`: **PASS**.
- `git diff --cached --check`: **PASS**.
- Initial `GOMAXPROCS=4 GOFLAGS=-p=2 make test` from the dirty source worktree: package tests and the first three integration fixtures passed; the final R2 archive/parity fixture correctly refused the dirty gate worktree, so this attempt exited 2 and is **not** counted as a full `make test` pass.
- Clean committed-tree `GOMAXPROCS=4 GOFLAGS=-p=2 make test`: **PASS**, exit 0, on clean source commit `f53abf0010d68c44c46a5ef0f2365c41b85c5913`. The four integration contract/archive fixtures all passed; the printed `sv1dresource` incomplete-command message is an expected negative case within the R2 contract fixture.

No economic world or payment-run test was launched by these commands.

## Explicit limits of acceptance

This review does not accept any of the following:

- proof that the caller's registration list is the immutable, complete runner deployment roster;
- proof that the runner exclusively owns time, drains all events through `t^-`, or generates the captured windows at the correct event frontier;
- a production `FundingSettlementEpochAppender`, its durable all-or-invalid stream behavior, or canonical binary event coverage;
- independent reconstruction of every account cash delta, fractional remainder, reserve movement, source window, and settlement status from sealed production evidence;
- post-payment risk preflight, terminal risk/world-stop propagation, or publication to actors;
- E2 `NewSim` integration, an end-to-end multi-payment lifecycle, natural funding activation, or any economic result.

No economic world, development seed, confirmation seed, or holdout was run or consumed. The next admissible work is a separately reviewed runner/evidence integration gate; this acceptance does not authorize an E2 experiment.
