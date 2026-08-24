# V2-3 P2 attempt 0 — invalidated evidence campaign

Status: **INVALIDATED BEFORE CAUSAL SCORE.** This is a provenance record, not
a P2 result. The raw worlds are retained at
`research/artifacts/historical/v2-3-p2-attempt0-buy-side-omission/` and must
not be pruned or treated as the final P2 cells.

## What ran

Four five-minute, full-evidence P2 cells ran from source revision
`6c3cedda3737c98409f085a250c2a9679b27a73f` (binary SHA-256
`36aedc9dac256df29299f280bddde0ad578f6e567f9653db5329859632e4cd14`).
Their persisted-evidence multiset attestations are:

| arm | seed | events | digest |
| --- | ---: | ---: | --- |
| A | 101 | 367,465 | `924b744660e8c855c0a98b57880fe4305eb54847dc0e0e917b62a63d6310ebc9` |
| A | 103 | 330,147 | `ff099a21be474f7cb89050413bf164bf2c8150431e74c39cc458e612bd9bd71f` |
| B | 101 | 368,074 | `104150f39ab4ed32c39c6d3dce3be25a1b9aedb6c380e4a6c8df09d81777d478` |
| B | 103 | 331,057 | `63360392b06737e5bc7649f5ad41a32df16ea16322ea8a6e64d89c87f08ebd36` |

The A control independently validates: 180 `POLICY_DISABLED` decisions and
zero submissions for each seed. The B treatment was active (46 submissions,
44 accepted orders, 88 fills, 5,665,000,000 filled units for seed 101; 50,
48, 96, and 9,150,000,000 respectively for seed 103). These are diagnostic
activation observations only.

## Defect and scope

`MakerInventoryRebalanceDecision.Side` used the `exchange.Side` enum, whose
valid BUY value is numeric zero. Its JSON field used `omitempty`, so every
submitted BUY decision persisted with no `side` field. The execution-side
`OrderAccepted` record still says BUY, but the independent P2 audit correctly
requires the decision record itself to name the planned side. It consequently
reported one `decision_policy_mismatch` per B submission and one
`request_fields_mismatch` per accepted B order.

This is an evidence-instrumentation defect, not evidence that the P2 policy
did not run and not a simulator-economic defect. It cannot be repaired by
inferring the missing field from the subsequent request or fill: that would
destroy the intended independently falsifiable decision frontier. Because a
paired screen needs one complete evidence contract for both arms, both A and B
attempt-0 cells are invalidated together.

## Correction and rerun criterion

Commit `ce096d2` keeps the internal enum for execution but serializes an
explicit `SideEvidence` string once a BUY or SELL action is selected. Deferred
actions still have no side. The correction has focused normal/race tests and a
fresh-process execution-hash evidence-on/off neutrality regression; it adds no
scheduler event, RNG draw, actor-visible state, request ordering, or economic
decision change. Commit `b9c271c` adds a negative-inventory BUY replay fixture
covering the omitted value.

Before a new final P2 attempt, rebuild `multivenue` and `mvanalyze` at the
corrected revision; rerun focused normal and race tests plus vet; rerender and
byte-diff the immutable A/B configs; then run all four cells from scratch.
Each must carry final `greeks.json` and `latency.json` sentinels, complete the
full required extraction including the persisted-evidence artifact digest, and
pass `makerrebalance` without decision or request-field mismatches. Only then
may P2 receive an activation or causal interpretation.
