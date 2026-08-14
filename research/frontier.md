# Active frontier

## Target and baseline

Main `29f45be`: `go test ./...`, `go test -race ./...`, and `go vet ./...` pass.
Verification tier A. External-validity gap: venue simplifications listed in `docs/realism-gaps.md` remain out of scope unless they violate stated library behavior.

## Supported claims

- Foreign fee assets are reserved per open order rather than merely checked at placement.
- Liquidation repayment preserves spot/perp loan attribution after repaying more than the perp-attributed share.
- Terminal venue and actor lifecycles are idempotent; scheduled delayed outputs are dropped after stop.
- Same-timestamp expiries settle in symbol order, before the option risk sweep.
- `make audit FUZZ_COUNT=3` passes vet, static correctness analysis, complete/race test suites, and repeated invariant fuzzing.

## Live mechanism families

| Hypothesis | Mechanism | Evidence | Risk | Next test | Cost |
| --- | --- | --- | --- | --- | --- |
| H-001 | State-machine invariant gap | Two financial accounting defects confirmed and fixed | High | Cross-asset partial-fill/fixed-fee bounds | CPU |
| H-002 | Extension-point contract mismatch | No new defect in this pass | Medium | Interface implementation sweep | CPU |
| H-003 | Concurrent publication/time ordering | Four lifecycle/order-state defects confirmed and fixed | High | Repeated deterministic scenario comparison | CPU |

## Contradictions and evaluator risks

- Existing regressions are strong but do not exhaust custom instrument, fee model, and lifecycle composition.
- `derivsim` fixed-seed runs still diverge after ordered strategy traversals; do not treat single-run PnL or impact figures as reproducible evidence until timer/actor event ordering is made deterministic.

## Budget remaining

- Wall time: approximately 10 hours.
- CPU: approximately 9 hours. GPU reserved but not useful for Go tests.

## Next three decisions

1. Run bounded fuzz/race stress and inspect failures.
2. Trace public interface paths and map mutation sites to invariants.
3. Fix only reproduced or code-proven defects; rerun full gates.
