# V2-5 P3c — finite term-completion result

Status: **FALSIFIED (development lifecycle screen).** The registered finite
P3 policy entered two legitimate matched spot/perpetual terms and reached its
declared term end, but neither could submit even the first perpetual unwind
order. Both remained open through the full two-hour post-term observation
window; south then received a funding transfer after its term window. The
current fixed-minimum-size exit policy is therefore not a viable finite
carry-lifecycle mechanism in this ecology.

This is a valid negative result, not a corrupted world: completion sentinels,
raw evidence, receipts, generic accounting, positions, order lifecycle,
derivative funding, lifecycle, and both evidence digests are retained and
parse. The **term-carry completion claim**, rather than the evidence artifact,
failed its preregistered gates.

The immutable preregistration is
[`v2-5-p3c-term-completion-preregistration.md`](v2-5-p3c-term-completion-preregistration.md).
The machine-readable verdict is
[`term-completion-107-verdict.json`](artifacts/v2-5-p3c/term-completion-107-verdict.json).
The full raw record remains at
`research/artifacts/v2-5-p3c/term-completion-107/` and is not safe to prune.

## Provenance

| item | value |
| --- | --- |
| config / SHA-256 | `configs/v2-5-p3c/term-completion-107.json` / `b4dfa018d075f12ede15dcc082d862322cadf6dcf0f06e0c4a8fd4c70a6b222b` |
| seed / horizon / process setting | 107 / 98 simulated hours / `GOMAXPROCS=4` |
| simulator revision recorded by manifest | `0ee828ffff098538fb9741b690d424d759b14b18` (only pre-existing non-simulator artifact edits made the manifest worktree flag true) |
| simulator / analyzer / prune-gate SHA-256 | `cb4c8c5ea1ec8b4872894d5f760b8a4b348ab015c08f782fa2680f4db87aba74` / `8d9ea7edb3cafc7a221c981da81c10e2bb32308413c991b714d0a2a634a9d8bb` / `f33bf066769d791f6afd8c83b3a1b3176186d9f03b3791ac97fa60590f60e437` |
| final execution observations / ordered hash | 50,614,472 / `429482c755a7a9e3635728f6f06bdffc646b65f209d5688e9cac7736f028e67b` |
| persisted evidence records / multiset digest | 51,143,681 / `82c2f8453361b24f7dc3645ae1bf85ec49569c93bd01c9691b209aafaf269130` |
| exact persisted JSON-record artifact digest | 51,143,681 / `785182a0949d3cf7042ad8a7efe5e2c2e03b1836fb46e0690eb69f31176faf07` |

Both final-only completion sentinels are nonempty. P3c's only financial-policy
delta from P3b is the explicit, participant-known
`mandate_end_at_nano=1736035205000000000`; it is not the 98-hour simulator end
time. The 98-hour run ended at `1736042400000000000`, leaving nearly two
hours after the declared close budget for ordinary retries.

## Independent evidence gates

| gate | result |
| --- | --- |
| receipt/frontier replay | valid: 3,175,215 schedules, 3,175,206 deliveries, 4 audited decisions; no early/future/reordered/missing-due observation |
| delivered latency | all allocator feeds: 40 ms market data; all request/response deliveries: 20 ms |
| balance replay | 3,078,491 delta rows checked; 0 mismatches, 0 broken chains, 0 decode failures |
| generic positions | 0 disagreement, 0 non-zero net contracts, 0 unrepresentable values |
| generic order lifecycle | 3,336,746 accepted, 1,784,100 fills, 2,441,509 cancellations; every registered error counter 0 |
| generic funding audit | 36 venue settlement records; 0 broken, duplicate, sign-wrong, misdirected, or undirected transfers |
| accounting identity | ABC residual 0; USD residual -23 raw units (`2.27e-14` relative), the existing bounded truncation residual |
| P3 source/frontier/gateway/actor/arithmetic replay | all registered counters 0 |

The information and mechanical contracts therefore do not explain the failed
close. The P3-specific lifecycle replay correctly marks the result invalid for
its own claim: two active/open terms, zero closed terms, 23 funding settlements
within an active term, and one settlement outside the declared active term.

## Falsified close mechanism

Two P3 v2 plans (central and south) were filled into matched
`spot=+10,000,000`, `perp=-10,000,000` positions. At the shared recorded term
end `1736035201000000000`, each state transitioned to `UNWIND_PERP`. Covering
the short perpetual requires buying its executable ask. Prices and asks were
present; this was neither a no-price sentinel nor a missing-book condition.
The actual local top-of-book quantities were instead too small for the
allocator's registered `min_order_size=100,000` policy:

| venue | post-term 2 s decisions | delivered perp ask quantity | submitted unwind orders | close |
| --- | ---: | ---: | ---: | --- |
| central | 3,600 | 16,286 on every observed post-term decision | 0 | no |
| south | 3,600 | 16,348 on every observed post-term decision | 0 | no |

For all 7,200 post-term decisions the actor emitted the structured,
persisted `EXECUTABLE_SIZE_UNAVAILABLE` reason. It did not manufacture a
price, submit an undersized order, change its risk limit, or use a hidden
reference. Consequently there are zero `SUBMIT_UNWIND_PERP_IOC`, zero
`SUBMIT_UNWIND_SPOT_IOC`, and zero `TERM_CLOSED` actions in the complete
evidence stream.

The terminal account snapshot independently retains the two unclosed P3
positions: central and south each have +10,000,000 ABC spot inventory and a
-10,000,000 `ABC-PERP` position. North was flat. The P3 replay's terminal
spot/perpetual *mismatch* counters remain zero because it correctly agrees
with this retained, non-flat exposure; zero mismatch is not a claim of
terminal flatness.

The missed exit had a material lifecycle consequence. Central's twelfth
funding transfer occurred exactly at `term_end` and is included by the
preregistered inclusive active-term rule. South's twelfth transfer arrived at
`1736035205000000000`, four seconds after its recorded term end, while the
unclosed short remained outstanding: +150,014 USD to allocator client 8. The
independent term replay detects this as the one
`funding_settlement_outside_active_term` failure. This is not a funding-sign
defect; the generic funding audit independently confirms direction and
conservation.

The explicit mandate policy itself was exercised by the flat north allocator:
after a prospective next term could no longer fit its declared horizon it
emitted `TERM_HORIZON_CENSORED` while remaining `IDLE`. It did not cause the
central/south terms to close, and it cannot be credited as a lifecycle fix.

## Interpretation and next gate

P3a and P3b remain valid narrow evidence of local entry and of ordinary
funding transfers while matched inventory exists. P3c falsifies the stronger
claim that their current fixed-minimum-size execution policy can complete a
finite term in this market. No paired funding/basis/price experiment may use
this participant yet.

The next work is not a parameter retest of P3c. It requires a separate,
preregistered P3 exit-liquidity design: state an economically justified
child-order/participation policy, its exposure and deadline treatment, the
observed-depth activation metric, and adversarial tests for residual/partial
execution. That design must distinguish genuine market illiquidity from an
actor-imposed minimum-size constraint before it can be integrated or compared
against this failed historical cell.
