# Independent technical review request — E0 PR1

Review target: code commit `012f672` and the bounded implementation/documentation tree `21b2b4552fc66b47a35219781ff1a1115f105f95` on `feature/e0-repeated-spot-pr1-20260926`, descended from planning revision `88717ca28ed21405debd5ec1771e114c12745be0`. Review is read-only. No economic world, protected holdout or old R2/SV1D result should be opened or rerun.

Start with the [E0 design draft](config-drafts/e0-repeated-spot.md), [PR1 checkpoint](pr1-implementation-20260927.md), and the exact source/tests in [`simulations/repeatedspot/world.go`](../../../simulations/repeatedspot/world.go), [`seed_once.go`](../../../simulations/repeatedspot/seed_once.go), [`policies.go`](../../../simulations/repeatedspot/policies.go), [`e0_draft.go`](../../../simulations/repeatedspot/e0_draft.go), and [`world_test.go`](../../../simulations/repeatedspot/world_test.go). `git diff 88717ca..21b2b45 -- simulations/repeatedspot research/program/longrun-baselines` shows the complete PR1 delta. The prior planning reviewer saw neither this source nor these tests.

Bounded review questions:

1. Does the assembler build exactly one declared FIFO spot book, with simulated decision/transport clocks and no derivative automation or hidden background account?
2. Does the seed use genuinely finite balances, post each side only once, and avoid refill after fills, rejections or cancellations? Are the t=0 interleaving and delayed-demand assumptions stated precisely?
3. Do policy/fee parameter snapshots and the exported contract accurately describe the built world? Identify hidden constructor defaults or callback captures that make the digest insufficient; do not mistake it for a source/binary attestation.
4. Do failed builder, closed/incomplete run and ordinary fill/cancel paths fail safely? Are tests independent enough to catch a regression rather than merely observe a lucky fixture path?
5. Does any document imply economic validity, fair AS-versus-pure comparison, price survival, hard inventory caps, or empirical realism before E0-3/E0-4 are complete?

Verification already run on clean commit `21b2b45`: `GOMAXPROCS=4 GOFLAGS=-p=2 make test` exit 0, `go vet ./...` exit 0, `go test -race ./simulations/repeatedspot -count=3` exit 0, `go test ./simulations/repeatedspot -count=20` exit 0, skill validator and diff checks pass. The initial full-gate attempt on the dirty worktree is recorded as a clean-worktree fixture refusal, not as a pass.

Requested output: file/line findings classified `BLOCKER`, `FIX_BEFORE_MERGE`, `DEFERRED`, or `NO_ISSUE`; one scoped verdict for technical PR1 integration only. A review of PR1 cannot authorize E0 economic worlds or substitute for prospective protocol/evidence review. The live review service returned `UNAVAILABLE / NOT_ISSUED` once (thread limit); do not represent this package as a completed review.
