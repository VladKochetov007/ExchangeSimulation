# V2-3 P3 design — confirmed passive-quote replenishment

Status: **design only; no implementation, rendered configuration, or P3 market
screen exists yet.** This is a narrow V2-3 microstructure mechanism prompted by
the retained V2-5 P3c term-completion evidence. It is not an exit-policy fix,
funding/carry result, price-stability treatment, or a replacement for a
price-elastic counterparty population.

## Observed failure, bounded claim

P3c's two finite carry terms reached their declared end with the required
perpetual cover price locally available, but only 16,286 / 16,348 raw units
were displayed at the central / south asks. Both values were below the
instrument's 100,000-unit venue minimum, so the allocator made no admissible
unwind request. The later P3d zero-floor contrast is invalid because it
mistook that venue minimum for an actor-only policy.

The raw P3c record rules out a cheap alternative: a deeper local ask sweep
could not form a 100,000-unit child at the first post-term publication. Each
book had one displayed level only. Earlier active-term snapshots did
occasionally show at least 100,000 units, so the retained result does not
justify forecasting a fixed future exit capacity from a terminal snapshot.

The contributing implementation behaviour is observable locally: a
`StoikovMarketMaker` stores its requested bid/ask quantities as `bidQty` and
`askQty`, but on a partial own fill it updates inventory only. At later quote
ticks an unchanged price and unchanged *target* quantity returns early while
the actual resting residual can be much smaller. A full fill or cancellation
does clear the side and retains the existing replacement path. `maker_state`
is deliberately not evidence of residual book quantity: its bid/ask fields
are requested targets, not exchange order state.

This identifies a missing declared policy, not a proof of a matching-engine
bug and not an entitlement to depth. P3 adds a maker-local response to its own
confirmed partial fills. Any resulting execution still uses ordinary delayed
cancel/order requests and can be rejected, cancelled, or remain unfilled.

## Local mechanism and exact scope

Add an opt-in `perp_maker_replenish_below_bps` V2 configuration field. It
applies only to the existing `ABC-PERP` Stoikov maker on each venue.

* `0` disables the policy and preserves the legacy P3 actor path.
* `1..10,000` declares the fraction of the current side target below which a
  **confirmed resting residual** is eligible for normal refresh.
* The first screen fixes the enabled value at **5,000 bps** before any screen
  world is run: a side is eligible only after more than half of that target has
  been consumed. This deliberately avoids automatically resetting queue
  priority after small partial fills.

At a regular existing quote tick, after normal local-reference and quote-plan
construction, an unchanged target pair can therefore enter the existing
cancel/replace path when either acknowledged, active side is below its declared
threshold. The policy must use only the maker's own order acknowledgements,
fill notifications, and cancellation notifications; it may not inspect an
exchange order book or use a new market-data source. It adds no timer,
scheduler event, RNG draw, background goroutine, matching rule, fee rule, or
fill guarantee.

For a target quantity `Q > 0`, known resting quantity `R >= 0`, and threshold
`T`, eligibility is the exact strict inequality:

```text
R * 10,000 < Q * T
```

using overflow-safe integer comparison. At `T=5,000`, an exactly half-filled
side is not eligible; a residual one unit below half is eligible. A side with
no acknowledged live order is handled by the existing normal refresh path,
not treated as a replenishment action. Pending requests still block a refresh.

The existing perps use cancel-before-submit because `SubmitBeforeCancel` is
false. P3 deliberately retains this legacy ordering rather than folding in the
spot-only P0 passive-refresh policy. If one side triggers, the actor uses its
existing pair-refresh path, so both quotes can be cancelled/replaced together;
this cost and queue-priority effect is part of the declared P3 policy rather
than hidden as a one-side book mutation.

## Actor state and evidence contract

The maker retains, separately from target quantities, `bidRestingQty` and
`askRestingQty`:

* acknowledgement of a current quote establishes the corresponding target as
  its known resting amount;
* every confirmed partial own fill decrements only that side;
* full fill, cancellation, rejection, or replacement clears it; and
* an acknowledgement/fill that does not belong to the current quote cannot
  manufacture a resting quantity.

Each eligible/considered unchanged-target quote tick in a P3 evidence run
persists one `perp_quote_replenishment_decision` record in the evidence-only
domain. Required fields are:

```text
venue_id, maker, client_id, symbol, decision_time,
enabled, threshold_bps,
bid_order_id, ask_order_id,
bid_target_qty, ask_target_qty,
bid_known_resting_qty, ask_known_resting_qty,
bid_replenishment_due, ask_replenishment_due,
refresh_due, reason,
bid_price, ask_price, bid_request_id, ask_request_id,
outcome_expectation, censor_reason
```

`reason` distinguishes `POLICY_DISABLED`, `ABOVE_THRESHOLD`,
`BID_BELOW_THRESHOLD`, `ASK_BELOW_THRESHOLD`, `BOTH_BELOW_THRESHOLD`, and
`NORMAL_PRICE_OR_SIZE_CHANGE`; a missing acknowledgement/pending request is
reported rather than represented by a zero price or inferred fill. The record
is emitted before gateway requests. It is persisted through `LogEvidenceOnly`,
outside the ordered execution hash; ordinary order, fill, and cancellation
events remain the independent venue source of truth.

The analyzer must reconstruct active quote state independently from exact
request/acceptance, fill, and cancellation chains. It must recompute the
strict threshold using arbitrary-precision arithmetic, join the decision's
request IDs to venue outcomes, and report missing, duplicate, wrong-order,
wrong-side, field-mismatched, or unexpected refresh relations separately. It
must not trust `maker_state`, an actor pointer, a book snapshot, or a terminal
inventory to infer the residual.

## Fixed first mechanism-identification screen

After unit, mutation, receipt-boundary, and analyzer gates pass, render a
short full-evidence control/treatment screen from one fixed parent ecology.
The screen has intentionally narrow primary claims:

| arm | `perp_maker_replenish_below_bps` | meaning |
| --- | ---: | --- |
| P3-A | 0 | legacy target-only quote refresh policy |
| P3-B | 5,000 | replenish only confirmed residuals below half target |

All price/skew, inventory limits, quote sizes, maker cadence and phase,
funding, term-carry, router, population, latency, fees, post-only settings,
and book parameters are held equal. The rendered-input differ must allow only
provenance labels, seed, recorder settings, and this field. It must reject any
unrelated change.

The screen is valid only when at least one P3-B side has a confirmed partial
fill and becomes threshold-eligible before a normal price/size change. Its
primary result is mechanical activation: P3-B must submit the ordinary next
pair refresh exactly when the independently reconstructed condition is true;
P3-A must emit no policy-triggered refresh. The required separate viability
rows are quote presence, two-sided-perp-book share, maker request activity,
accepted/rejected orders, fill volume, and evidence/hash validity.

No change in price, basis, carry close, average spread, or terminal inventory
rescues a failed activation. A screen with no qualifying partial fill is **NOT
EXERCISED**, not a reason to alter the 5,000-bps threshold or upstream
population. A successful screen only establishes the declared local
replenishment policy and its short-horizon viability consequences.

## Adversarial gates and interpretation boundary

Required unit/mutation cases are:

1. partial fill below threshold refreshes at the next ordinary tick despite
   unchanged price and target;
2. partial fill at/above threshold preserves the existing quote and priority;
3. full fill/cancellation/rejection retain legacy clearing/replacement
   semantics;
4. a missing residual decrement, side swap, incorrect strict inequality, or
   excess/fabricated refresh is rejected by the independent replay;
5. dropped, duplicate, delayed, and reordered own fill/cancellation evidence
   causes a structural replay failure rather than a guessed residual;
6. recorder on/off fresh-process and relevant `GOMAXPROCS` runs retain exactly
   the same execution hash; and
7. a normal positive-domain world without P3 enabled retains its ordered
   execution hash and fills.

This mechanism may make local passive liquidity more durable. It does not
prove sufficient exit capacity for future term carry, realistic long-run depth,
funding anchoring, stable prices, or that more displayed quantity is socially
desirable. Only after P3 itself passes its fixed activation and viability gate
may a separately preregistered finite-term lifecycle screen ask whether it
changes the P3c-type exit failure.
