# V2-7 P7a — finite-collateral leveraged exposure

Status: **preregistered design; no outcome worlds have been inspected**.

This protocol is the first V2-7 mechanism-identification slice.  It does not
claim that a crisis is realistic, that liquidation is profitable, or that a
particular price path is desirable.  Its question is narrower:

> Can a participant with a persistent, economically declared physical
> liability, ordinary venue execution, and finite perpetual collateral become
> an independently observable risk event without a scripted price shock or a
> threshold-only code exercise?

The verification regime is Tier B/C: simulator invariants and independent
event reconstruction are exact, while external economic validity remains a
model-to-market inference.  A zero-trigger cell is **NOT EXERCISED**, never a
successful liquidation validation.

## Mechanism and information contract

The participant reuses the audited V2-5 local-feed execution boundary, but
uses a new policy version, `v2_7_fixed_liability_v1`.  It represents a
producer/consumer with a fixed off-exchange physical exposure and hedges that
exposure with a signed perpetual position.  A negative physical exposure is
an economically declared short physical liability, so the target perpetual
position is positive.  The physical ledger is a motive, not an exchange cash
transfer and not a price oracle.

At each two-second local decision it computes, with checked integer arithmetic:

```text
target_perp = -fixed_physical_exposure
hedge_gap   = target_perp - actor_filled_position
quantity    = min(abs(hedge_gap), max_request_qty)
```

It submits an ordinary IOC at the last locally delivered executable touch.
There is no shared mark, direct funding read, forced fill, atomic opposite leg,
fee subsidy, or synthetic liquidation.  Once the declared target is filled it
holds the exposure; it does not reopen after an exchange-side liquidation.

## Development design

The initial screen is a leverage ladder, not a post-outcome shock search.  The
market, target exposure, fee, clocks, feeds, and request cap are identical
across active levels.  Only the participant's finite initial perpetual margin
differs.  The disabled control has the same account, feed, timers, and evidence
contract but cannot submit.

| cell | participant | initial perp margin | effective gross leverage at the opening $50 reference | purpose |
|---|---|---:|---:|---|
| C | installed, disabled | 12,000,000 raw USD | 4.17x if active | roster/evidence control |
| L | enabled, fixed liability | 12,000,000 raw USD | 4.17x | lower-risk active level |
| H | enabled, fixed liability | 6,000,000 raw USD | 8.33x | higher-risk active level |

The fixed physical exposure is `-1,000,000,000` raw ABC (10 ABC), the target
is therefore `+1,000,000,000` raw ABC, and each request is capped at
`250,000,000` raw ABC.  At the inherited $50 opening reference the position's
gross notional is approximately $500 and the exchange's 10% initial-margin
rule requires approximately $50.  The 12m/6m raw-USD deposits are an
ex-ante capital ladder (not chosen from a realized mark path); the high level
leaves a smaller buffer for an adverse move while remaining admissible.  The
existing five-basis-point taker fee is unchanged.

All cells inherit the V2 environment's three venues, matching rules, maker and
taker roster, borrowing configuration, spread/depth, latency profiles,
funding, listing/expiry, and deterministic one-second runner step.  The
cross-asset graph is disabled for this first slice so CDF/USD mark viability
cannot censor a perp-only distress question.  No existing V2 population is
retuned.

Development seeds are **307 and 311**.  They are new screening seeds, not
holdouts from P4/P5/P6.  Untouched holdouts are **313, 317, and 331** and are
not consumed by preflights or debugging.  The registered development horizon
is 4 simulated hours, with 30 minutes of post-entry observation; the initial
mechanics preflight may use at most 15 simulated minutes and cannot score
distress.  If the active levels pass activation and evidence gates, the same
immutable configurations are run on the untouched holdout policy.  No level
is selected retrospectively.

## Required evidence and scoring

Every cell must retain full raw evidence, market-data receipt/frontier
sidecars, final `greeks.json` and `latency.json`, and the exact persisted
evidence artifact digest.  Independent extraction must reconstruct:

1. participant decisions, local receipt frontier, target/gap, ordinary IOC
   request and venue outcome;
2. canonical accepted/fill/cancel events and actor position transitions;
3. pre-evaluation mark, collateral, borrowed balance, notional, maintenance
   and warning margin;
4. margin calls, liquidation checks, forced-close fills, partial residuals,
   deficits, insurance-fund transfers, and bankruptcy/terminal balances;
5. position closure, one-event-per-close, conservation residuals, and absence
   of duplicated settlement or post-liquidation reopening.

The primary paired development outputs are separate indicators/counts:

* actor activation: decisions, admitted orders, canonical fills, and target-gap
  reduction;
* risk activation: margin checks and calls;
* forced-close activation: liquidation count and independently reconstructed
  position reduction;
* deficit activation: positive remaining debt, insurance-fund debit, and
  bankruptcy, all requiring exact balance/insurance agreement;
* terminal residual position and collateral state.

The permitted classifications are `SUPPORTED (screening)`, `MIXED`,
`FALSIFIED AT ACTIVATION`, `FALSIFIED AT EXECUTION`, `NOT IDENTIFIED`, and
`NOT EXERCISED`.  A market-level price or volatility change is not an endpoint
of this protocol.  A liquidation event without independent pre-mark,
position, balance, and conservation reconstruction is not a success.

## Falsifiers and mutations

The branch is invalid if the participant reads a global mark/funding value,
if its receipt frontier permits future data, if an IOC is not an ordinary
venue order, if a fill increases the target gap, or if the enabled/disabled
delta changes any non-declared execution input.  The following mutations must
be rejected by the analyzer before a cell can be scored:

* reverse the fixed-liability target sign;
* drop or duplicate a decision/fill or an exposure state;
* inject a future, delayed, duplicated, or reordered book receipt;
* alter the admitted price away from the local executable touch or exceed the
  request cap;
* drop a forced-close fill/cancellation or duplicate a liquidation event;
* replace contemporaneous collateral/mark with a stale snapshot;
* force a synthetic balance reset after liquidation.

Evidence ON/OFF fresh-process runs must have identical execution-domain hashes;
the persisted evidence digest is compared separately.  Trigger count zero is
reported as **NOT TESTED/NOT EXERCISED** for that mutation or endpoint.

## Promotion and stop rules

P7a is promoted only if both active levels have valid activation and the
independent risk path is observable in at least one development cell.  A lack
of liquidation is a meaningful inactive result, not permission to lower the
threshold.  A liquidation without a participant-built position, or one caused
only by a changed threshold, fails the economic mechanism.  A complete
development failure stops holdout promotion and leads to a redesign ledger
entry rather than parameter tuning.  A later P7b may vary shock source (flow
versus an independently motivated liability event), but it is a new
preregistration and cannot be folded into this result.

