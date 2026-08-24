# V2-5 P3b — v2 evidence encoding preflight

Status: **PASSED (instrumentation preflight only).** This retained five-minute
world validates the P3 v2 plan/first-exposure evidence contract before the
registered nine-hour realization cell. It is not a P3b outcome, funding,
carry, basis, or market-quality result.

| item | value |
| --- | --- |
| config / SHA-256 | `configs/v2-5-p3b/term-realization-107.json` / `b58a21a1f9c2e77f2516a5e45736f3990790cf10ff0377cde45ac3166203cab3` |
| seed / horizon / process setting | 107 / 5 simulated min / `GOMAXPROCS=3` |
| committed source | `9bdb0b5132a14e1610ec72ab3f38d98098071f41` |
| simulator / analyzer SHA-256 | `648d2c977a6ae1360485b793f911a866c2e99ce9f1681e4298113e1451b0f881` / `e161bef701ef52f1e62da33b3b0ca302340070ea26ae2e545891868da496cfeb` |
| execution observations / ordered hash | 56,189 / `f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2` |
| persisted evidence / multiset digest | 56,648 / `231d8d54c7c9c9ea4105758853e50d0b0db6cc08702d6d8ba8850709413b9e3e` |
| evidence artifact | 56,648 records / `8ab665903e4db10503f97b07f817139770ff2ca7142a15e99fec6bc8697a907f` |

Both final completion sentinels are nonempty and the retained raw evidence,
sidecars, checkpoints, and preregistered preflight metrics all parse. The
term-carry replay is valid: 450 decisions, four submitted/accepted requests,
five fills, two active/open terms, and zero source, frontier, arithmetic,
lifecycle, position, terminal-spot, terminal-perp, or first-exposure
mismatches. Receipt replay is valid with 2,679 schedules, 2,670 deliveries,
and four decision joins. Conservation (3,149 delta rows) has zero mismatches
or broken chains; positions, generic order lifecycle, and zero-settlement
derivative checks are clean.

The retained v2 JSON includes two submitted entry plans with
`plan_created_at` equal to the entry decision time and zero
`first_exposure_at`; their later `TERM_ACTIVE` decisions preserve that plan
time and cite the first canonical fill exactly one simulated second later.
The new P3 v2 fields do not change the world: the ordered execution hash is
identical to the historical P3a B world under the same seed/config economics,
although the persisted evidence artifact correctly differs because its
schema is versioned.

The preflight therefore permits the immutable nine-hour P3b realization cell.
Its raw evidence remains retained and is not a substitute for that cell.
