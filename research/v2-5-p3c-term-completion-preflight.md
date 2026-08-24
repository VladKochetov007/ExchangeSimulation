# V2-5 P3c — mandate encoding preflight

Status: **PASSED (instrumentation/configuration preflight only).** This retained
five-minute world verifies that the declared P3c treasury mandate preserves an
eligible initial P3 v2 plan, first-fill evidence, and all required mechanical
sidecars before the registered 98-hour term-completion cell. It is not a P3c
close result, funding/basis/profit result, or market-quality result.

| item | value |
| --- | --- |
| config / SHA-256 | `configs/v2-5-p3c/term-completion-107.json` / `b4dfa018d075f12ede15dcc082d862322cadf6dcf0f06e0c4a8fd4c70a6b222b` |
| seed / horizon / process setting | 107 / 5 simulated min / `GOMAXPROCS=4` |
| simulator revision recorded by manifest | `1b4d25ecd7748179b2ed7f762c07dc8f0de9c2e2` (the preregistration document was uncommitted at build time; no simulator source was modified) |
| simulator / analyzer / prune-gate SHA-256 | `ef32c40f15a26e6968cd9ac6906c5db537034eca1710077ac9d78c2a96585e47` / `7384a8f493cf62804c86fe1a6f319021bd23367a564887c9167e0221ccb5bde2` / `378297e59df669c6f9baf69010b4a65160f89793109e61731eef7d1d16275b42` |
| execution observations / ordered hash | 56,189 / `f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2` |
| persisted evidence / multiset digest | 56,648 / `9d9f608f899b75f0d60ec0e23fb1b72fa6a7dae5274fd9202ca89477ddd38665` |
| evidence artifact | 56,648 records / `6ebd26dc7b7f5f54502a7837384fe06716d3a73410c9857678db1447eb63f241` |

Both completion sentinels are nonempty. The term-carry replay is valid: 450
decisions, four submitted/accepted requests, five fills, two active/open terms,
and zero source, frontier, gateway, arithmetic, lifecycle, position,
terminal-spot, terminal-perpetual, or first-exposure mismatches. Receipt replay
is valid with 2,679 schedules, 2,670 deliveries, and four decision joins;
terminally undelivered observations occur after the short horizon. Conservation
checks 3,149 balance deltas with no mismatch or broken chain; generic position,
order-lifecycle, and derivative funding counters are clean.

Every allocator link recorded a 40-ms market-data delivery mean; request and
response delivery means are 20 ms. The P3c preflight execution hash exactly
equals the P3b preflight/P3a enabled positive-world hash, confirming that the
future mandate does not perturb the pre-mandate world. The persisted evidence
digests differ because the persisted configuration intentionally identifies the
P3c policy and mandate.

The sole conclusion is that the immutable 98-hour P3c cell may run. Its raw
evidence remains retained and does not substitute for a complete close.
