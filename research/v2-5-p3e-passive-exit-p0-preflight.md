# V2-5 P3e P0 — passive-exit preflight

Status: **passed mechanical preflight; not P0 evidence.** This five-minute
full-evidence run verifies only the new P4 policy serialization, analyzer
contract, and retained-artifact workflow. Its 96-hour term end cannot occur
inside the horizon, so it cannot activate, falsify, or support the passive
exit mechanism.

The immutable protocol is
[`v2-5-p3e-passive-exit-p0-preregistration.md`](v2-5-p3e-passive-exit-p0-preregistration.md).

## Provenance

| item | value |
| --- | --- |
| source revision | `e1905d73e1ff83556d8adb6bcdc4304e13a5aa37` |
| config / SHA-256 | `configs/v2-5-p3e/p0-B-107.json` / `206fc24ee0bc7f16aacabcbebc794b1fedd611922ef58e4578d5df142b323835` |
| seed / horizon / process setting | 107 / 5 simulated minutes / `GOMAXPROCS=4` |
| simulator / analyzer / prune-gate SHA-256 | `916671bb084bcc3cb00458071170646301e71755905e059688d1948ac9363c4c` / `d385b7c6b4e4434f24a2b851d051b1b3f9929a40ef9b1aaa0029949cee8d765d` / `e398ad816ef70a385b94e1d897d902a4f564ad8b27068762c49ef90c6dfc27e4` |
| final execution observations / ordered hash | 56,189 / `f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2` |
| persisted-evidence records / multiset digest | 56,648 / `962dd193b300dfa724fe08449a83e92f462107dbc7f8c598745aec7c9d6a343a` |
| exact persisted JSON-record artifact digest | 56,648 / `16c850a020faaa0e156869b82eee296eddb6a3efa56151b3a209c97248d9a97c` |

The manifest marks the build worktree modified because four pre-existing,
user-owned ae13f9a scoreboard artifacts were already modified. The built
simulator, analyzer, and prune-gate sources were the committed revision above;
those artifacts were neither read as inputs nor changed by this preflight.

Both final-only completion sentinels (`greeks.json`, `latency.json`) are
nonempty. Raw venue JSONL, compact market-data sidecars, manifest,
checkpoints, and every extracted artifact remain at
`artifacts/v2-5-p3e/preflight-B-107/`.

## Independent checks

| contract | result |
| --- | --- |
| P4 term-carry replay | valid: 450 decisions, 4 submitted/accepted requests, 5 fills; all source, arithmetic, gateway, venue, actor, continuity, and passive-cancellation counters zero |
| P4 policy serialization | explicit passive configuration parsed: slice `100,000`, deadline `1736042405000000000`; the P4 policy is present without producing an exit action before term end |
| receipt/frontier replay | valid: 2,679 schedules, 2,670 deliveries, 4 audited decisions; zero early, future, reordered, or missing-due observations |
| actual allocator latency | all three market-data links: 40 ms mean delivery; request/response links: 20 ms mean delivery |
| generic order lifecycle | 5,669 accepted, 1,826 fills, 4,614 cancellations; all registered generic error counters zero |
| balance and position replay | 3,149 balance deltas: zero mismatches, broken chains, or decode failures; position disagreement and unrepresentable values both zero |
| derivative replay | zero funding/exercise direction, duplication, and payoff-arithmetic failures in this pre-funding horizon |
| evidence identity | offline `evidenceartifacthash` exactly equals the runtime persisted JSON artifact count and digest |

The term policy entered two ordinary matched terms and left them active, which
is expected in five minutes. The action stream contains no P3e passive-exit
action and therefore supplies no test of the primary activation condition.

## Retention and prune gate

`scripts/extract-v2-5-p3e-metrics.sh` writes the complete P3e mechanical
contract atomically and compares the runtime and offline persisted-evidence
artifact identities. It explicitly does not score activation or prune logs.

The hardened global `prunegate` test passes. A read-only P3e preflight check
returns `MEASUREMENT_INCOMPLETE` with `arm=unknown` because the historical
ae13f9a measurement manifest intentionally has no V2-P3e arm contract. That
is fail-closed behavior, not a waiver: P3e evidence remains retained and no
manual pruning path is used.

## Next gate

The registered 98-hour P0 B cell may run from the exact immutable config only.
Its final result must be re-extracted with the same script, then independently
classified as activated, falsified at activation, not exercised, or invalid.
It cannot be used for an A/B market or funding claim.
