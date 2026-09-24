# ME-000 — mechanical readiness closeout

Status: **COMPLETE WITH NARROW INDEPENDENT ACCEPTANCE**. No ME-001 economic
world or reserved holdout was run for this result.

## Question and conclusion

Can the fixed ABC/USD immediate-execution pilot bind the effective runtime
configuration and independently reconstruct the focal actor's delivered
opportunity, order, venue execution, ledger and terminal result? At exact code
commit `b25223a4593a9e5129bab27cdbd5f2f8be146bcf` (tree
`2dd21b21b4b78669ebff6b3ca289a8b59b427704`), the bounded fixture and
adversarial tests support **mechanical readiness**, not an economic finding.

## Contract and evidence

F1: `experiment/executionpilot` constructs `executionlab.Immediate` from the
proposed 2/10, 4/8, 6/6 maker/taker and 0.5/2/5 ABC grid. Its locked plan
contains the actual constructed world contract: actor IDs, fees, endowments,
clocks, delays, realized maker cadences, horizon, and target. Verification
reconstructs that contract and binds source commit/tree, Go 1.27 toolchain,
simulator and analyzer binary digests, canonical typed-plan digest, raw plan
digest and the versioned evidence schema. Output directories must be new.

F2: the `execution-pilot-opaque-v3` stream records global event order,
venue public snapshots, focal per-publication transport outcomes, actor
receipt/decision/send events, venue order/trade/cancel/ledger events and
terminal book/balances. The analyzer separately checks all 4,000 phased
decision ticks; published, enqueued, dropped and delivered snapshots; local
decision eligibility; send/admission/response timing; exact request, order,
trade and cancellation identities; quantity and fixed-point fee arithmetic;
cash/asset deltas; and terminal valuation. A genuine publisher drop is not
fabricated as an actor observation. An unexplained missing receipt for an
in-horizon enqueued snapshot invalidates evidence. The actor's self-report
is compared only after independent event reconstruction.

The accepted tests cover complete and partial fills, rejection, accepted
zero-fill after the facing side disappears, unavailable terminal marks,
queued earlier fills received after cancellation, omitted/duplicated receipts,
changed same-price depth, wrong/missing cancellation request IDs, malformed
or rehashed evidence, and evidence-observer neutrality. The selected policy
must send at the first eligible tick; a missing send is an implementation
failure, whereas a complete clock with no eligible delivered state is a
valid `NO_OBSERVED_OPPORTUNITY` fixture. No economic frequency is inferred
from these fixtures.

## Reproduction and identities

From a clean checkout of `b25223a` with the registered Go 1.27 toolchain:

```bash
GOMAXPROCS=7 GOFLAGS=-p=7 go test ./marketdata ./simulations/executionlab ./experiment/executionpilot -count=1
GOMAXPROCS=7 GOFLAGS=-p=7 go vet ./...
GOMAXPROCS=7 GOFLAGS=-p=7 make test
GOMAXPROCS=7 GOFLAGS=-p=7 go test -race ./marketdata ./simulations/executionlab ./experiment/executionpilot -count=1
```

Observed: each command exited 0. The full `make test` gate was rerun from
the clean committed tree; its earlier dirty-tree archive parity refusal was
not a source-test failure. The race gate finished after the review and passed.
The reviewer did not execute these commands independently.

The implementation identity is `b25223a`; no economic run or analyzer output
identity exists yet. The governing skill last changed at
`f2cca325fa552766e23f83c3204bfa8f83f9e241`, directory tree
`d1ff061a7d98b9e003d7ac4166d1f7434f54eb17`. The [review ledger](../../reviews/me000-readiness-20260923.md)
preserves three substantive rejections and the final scoped acceptance.

## Boundaries and next action

This closes only the two readiness fixes. It does not say a 27-world batch
has run, that the 10 bp mandate is met, that composition has a causal effect,
or that the simulator is empirically realistic. Focal enqueued snapshots due
after shutdown are horizon-censored. The fresh-process digest controls and
actual resource use belong to ME-001. R2/SV1D remains closed at its prior
non-activation boundary.

Next: prospectively lock and independently review the existing ME-001 draft,
then exercise only the owner-authorized development matrix if its own gate
passes. See [machine result](result.json).
