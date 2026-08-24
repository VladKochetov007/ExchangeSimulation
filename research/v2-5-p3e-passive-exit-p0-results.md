# V2-5 P3e P0 — passive finite-term exit activation result

Status: **SUPPORTED (screening) — narrow P4 activation/integrity only.** This
is a single pre-registered B/107 cell. It establishes neither a causal
comparison nor an economic market outcome.

The immutable protocol is
[`v2-5-p3e-passive-exit-p0-preregistration.md`](v2-5-p3e-passive-exit-p0-preregistration.md).
Raw evidence, compact sidecars, completed extracted metrics, and the
machine-readable verdict remain retained at
`research/artifacts/v2-5-p3e/p0-B-107/`.

## Provenance and completed evidence contract

| field | value |
| --- | --- |
| fixed config / SHA-256 | `configs/v2-5-p3e/p0-B-107.json` / `206fc24ee0bc7f16aacabcbebc794b1fedd611922ef58e4578d5df142b323835` |
| simulator revision at run | `1714c26b6c9253f51cc3da61df52c30956cb4ea8` |
| seed / horizon / evidence / GOMAXPROCS | 107 / 98 simulated hours / full / 4 |
| simulator binary SHA-256 at launch | `a4326f11a53a4b8c2170797ef045761ac3c601668f5250f2c8d69627aa446583` |
| terminal execution observations / ordered hash | 50,636,477 / `e385d5b62666f3ca035d5da45ba01023649eb0e4fd63884326638a52473be7b2` |
| exact persisted JSON records / artifact digest | 51,165,698 / `2e36cc856c71df92624057e48e7aa9e193d6a96b24f0ec9a0c6fa41b26203bf0` |
| persisted-evidence multiset digest | 51,165,698 / `3e6c1625db379a858403a80c19a6a72f2826f8aa0169c1b8594c5dcf4713f2e1` |
| extractor revision / analyzer SHA-256 | `45f3eab4acb5d7abe85af7af92395e92e67325d8` / `64df29c5d4f673e6f3ef56588433db18364f7ed7b85bc00df07d21252e9a0523` |

The simulator completed before analysis: nonempty final `greeks.json` and
`latency.json` are both present. The fail-closed extractor then exited zero,
wrote all nine registered metrics and `analysis-metadata.json`, and compared
the runtime exact artifact identity to the independently replayed one. Both
agree exactly on the count and `2e36…3bf0` digest above. Raw P3e evidence is
retained; no manual prune authority is claimed.

## Registered P0 activation predicate

| preregistered condition | independently reconstructed evidence | result |
| --- | --- | --- |
| owned finite term reaches active state | 2 active terms; 345,539 `TERM_ACTIVE` decision rows; all entry/fill/frontier/lifecycle counters clean | pass |
| ordinary IOC is ineligible because opposite displayed depth is below legal minimum | at each P4 decision, the buy-to-cover `ABC-PERP` ask was 16,286 (central) or 16,348 (south), below the explicit 100,000 minimum; the independent passive-child validator verifies that failed IOC precondition from the delivered local book | pass |
| legal local post-only child is emitted | 2 `SUBMIT_UNWIND_PERP_POST_ONLY` actions: BUY, 100,000 units, bid 4,999,000,000, `LIMIT/GTC`, `post_only=true` | pass |
| gateway and canonical venue evidence agree | both children were accepted; the term-carry replay reports zero gateway, venue, actor-outcome, field, and duplicate-outcome mismatches | pass |
| complete independent mechanics/evidence contract | term-carry and receipt replays valid; zero information-frontier, generic order-lifecycle, position, balance-chain, funding/exercise, and artifact-identity failures | pass |

The actual passive orders are ordinary venue orders, not synthetic closures. Both
were accepted and later filled for exactly 100,000 units. Their fills are an
observed liquidity outcome, not an additional score condition. The policy also
recorded 11 `PASSIVE_EXIT_RESTING` decisions and two explicit
`PASSIVE_EXIT_REFERENCE_UNAVAILABLE` defers; those defer reasons remain
visible rather than becoming hidden zero-price fallbacks.

## Mechanical cross-checks

- The V2-0 local-information replay is valid: 3,175,215 schedules, 3,175,206
  receipts, 10 audited decisions, and zero early/future/reordered/missing-due
  or bad-frontier cases. The three term-carry links delivered at the configured
  40 ms market-data and 20 ms request/response means.
- The independent P4 replay is valid: 529,200 decisions, 10 submitted and
  accepted requests, 11 fills, zero source/arithmetic/gateway/canonical/actor
  evidence mismatches, zero position-continuity errors, and zero external
  term-funding settlements. It separately observes 23 active-term and one
  proven residual-exit funding settlement; that residual is visible, not
  silently erased.
- Generic order replay records 3,338,654 accepted orders, 1,787,716 fill
  records, and 2,441,559 cancellations, with every registered error counter
  zero. Position reconstruction has zero disagreement and no unrepresentable
  open values. Conservation delta chains have zero mismatches, breaks, or
  decode failures; the USD aggregate residual is the recorded bounded integer
  truncation of −23 quote units.
- Derivative replay has zero funding direction, duplicate-payment, exercise,
  or payoff-arithmetic failures. No cancellation is scored: the P4 deadline is
  five seconds after the P0 horizon, as preregistered.

## Interpretation and strict non-claims

**P0 activation is supported.** In this fixed cell, the bounded policy
recognized the registered local sub-minimum executable-depth condition and
constructed/admitted two exact legal passive children through normal delayed
gateway and venue paths. This is screening-level evidence for the narrow
implementation/evidence contract only.

P0 does **not** support a claim that P3e fixes the P3c lifecycle failure, that
a finite term closes reliably, that passive liquidity is available generally,
that deadline cancellation works, or that residual risk is profitable. Although
the raw trace contains two later `TERM_CLOSED` rows, P0 was not preregistered
as a closure screen and its deadline lies outside the observed horizon; those
rows are not promoted to a closure result. It also makes no claim about market
prices, basis, funding anchoring, carry profitability, ecology, realism, or
robustness.

The next permitted economic test, if the signed-price hard gate is satisfied,
is a fresh separately preregistered same-build lifecycle A/B experiment with
the passive deadline within the horizon. It must score activation, resting and
partial fills, deadline cancellation, residual state, continuous positions,
post-close funding attribution, accounting, and actual flat closure separately.
