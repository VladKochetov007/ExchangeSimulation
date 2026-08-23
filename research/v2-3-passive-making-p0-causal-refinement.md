# V2-3 P0 causal refinement — passive refresh contract

Status: **preregistered before a P0 market run.** This addendum does not
modify the original P0 preregistration or report an outcome.

## Why the original two-arm screen was insufficient

`spot_passive_maker_post_only=true` originally changed two things for a
Stoikov refresh: venue admission became post-only and the actor sent cancels
before its replacements. A treatment/control difference could therefore not
be attributed specifically to the exchange admission contract.

The simulator now exposes
`spot_passive_maker_cancel_before_replace`, which is valid only with
`spot_passive_maker_post_only=true`. It is propagated to Stoikov,
fixed-distance, and imbalance spot makers. It introduces no scheduler event,
RNG draw, matching priority, or delivery guarantee.

"Cancel before replace" means **actor submission order**. Requests retain
their independently modeled request latency, so the exchange still assesses a
post-only order against the book that exists at that order's actual arrival.
It is not an atomic cancel/replace primitive and does not guarantee that a
cancel arrives first.

## Fixed A/B/C arms

All arms retain the same seed, duration, population, fees, latency profiles,
clocks, inventory control law, reference policy, and log/evidence contract.
The passive-refresh switch applies to the configured refreshable spot-maker
population; CDF/USD is the primary screen book, not an assertion that other
spot books are unaffected.

| Arm | `spot_passive_maker_post_only` | `spot_passive_maker_cancel_before_replace` | Meaning |
| --- | --- | --- | --- |
| A | false | false | Legacy submit-before-cancel refresh, ordinary limit orders. |
| B | true | false | Legacy actor submission order, but each refresh must rest or reject at venue arrival. |
| C | true | true | Actor sends cancels before replacements; each replacement remains post-only at arrival. |

The configuration validator rejects C without post-only admission. There is
no undefined `cancel_before_replace=true, post_only=false` arm.

## Identified comparisons and measurements

### B minus A — exchange passive-admission mechanism

This is the only comparison that identifies the post-only admission contract.
For every `POST_ONLY_WOULD_TAKE` rejection, the mechanical invariant is:

- no order ID allocation;
- no reserve or auto-borrow mutation;
- no match, fill, trade notification, or book mutation.

The exchange test seeds both crossing directions and the adversarial mutation
removes the bit from the same request; only the mutated request can fill.
The P0 evidence metric separately records accepted post-only quotes,
would-take rejections, and fills of accepted post-only resting orders.

### C minus B — actor refresh-order mechanism

This comparison is not a post-only attribution. It tests whether explicit
submission ordering changes quote gaps, post-only rejection frequency,
inventory behavior, or viability. It must report actual delivered request
order/arrival where available rather than infer it from the actor setting.
An absent effect is meaningful: latency may erase the sender-order difference.

### Fixed viability gates

For every arm and paired seed, report independently:

1. selected-maker quote decisions and accepted/rejected quote activity;
2. CDF/USD quote presence and two-sided-book share;
3. CDF/USD fill volume and maker activity;
4. post-only would-take rejection rate (B and C);
5. maker inventory path and refresh-induced fill diagnostic.

Price stability remains descriptive only. Neither a calmer terminal ratio nor
a lower activity rate rescues a failed viability gate. No spreads, clocks,
inventory skew, or demand population may be tuned after this screen.

## Implementation-level regression contract

Focused tests prove all three source contracts:

- arm B emits post-only replacements before its legacy cancel requests;
- arm C emits cancellation requests before post-only replacements for Stoikov
  and fixed-distance/imbalance's shared replacement core;
- the global config reaches all selected maker families;
- an invalid C-without-B configuration fails before simulation construction.

The original exchange admission, actor helper, persisted acceptance/rejection
metric, and stripped-bit mutation tests remain the mechanical evidence for
B minus A. This addendum only separates the previously bundled actor ordering
policy.

## Run gate

No P0 market artifact is evidence until the V2 information-boundary contract
is complete enough to audit its selected maker decisions and the configured
receipt/decision evidence passes its independent validator. This refinement
does not waive that gate.
