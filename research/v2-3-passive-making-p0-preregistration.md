# V2-3 P0 — passive spot quoting

Status: **preregistered before implementation and simulation.** This is the
first, deliberately narrow V2-3 slice. It is not a claim that the CDF/USD
runaway has been cured.

## Observation

The ae13f9a CDF/USD runaway is compatible with inventory-skew feedback. The
current spot population has three refreshable passive-maker families on that
book: Stoikov, fixed-distance, and imbalance makers. Each submits ordinary
limit replacements. A replacement that crosses the book at *venue arrival*
can therefore become an unlabelled aggressive inventory trade. The old
submit-before-cancel path can additionally hide the event through
cancel-maker self-trade prevention.

## Local hypothesis

If passive quote refreshes are enforced post-only at the exchange admission
boundary, they cannot themselves consume opposite liquidity. Any later
aggressive inventory action must be represented by a separate policy, not by
a quote-refresh limit order.

## P0 implementation

1. Add an explicit `post_only` bit to a limit GTC request and accepted-order
   evidence.
2. Reject a post-only request before order-ID allocation, reservation,
   auto-borrow, matching, fills, or book mutation when it would take the
   *arrival-time* opposite touch. Market, IOC, and FOK requests cannot claim
   post-only semantics and are explicitly rejected.
3. Add a named actor submission helper and apply it only to the CDF-relevant
   refreshable **spot** maker families: Stoikov, fixed-distance, and
   imbalance makers. The V2 config switch is opt-in, preserving legacy
   scenarios until an experiment selects it.
4. When that switch is enabled, a Stoikov maker requests cancellation before
   sending replacements. Request latency still determines venue arrival; P0
   does not introduce atomic replace, new scheduler events, or a priority
   privilege.

The following remain intentionally outside P0: futures and option dealers,
bootstrap ladders, latent-liquidity intentions, explicit hedges/rebalances,
and non-multivenue demonstration strategies. They are separately named
passive-looking paths, not evidence that the P0 CDF mechanism is complete.

## Activation and safety measurements

- every selected maker quote decision/accepted order carries `post_only=true`;
- an arrival-time cross is rejected as `POST_ONLY_WOULD_TAKE`, with zero
  fills, no ID allocation, no reserves/borrow, and no book change;
- a non-crossing post-only quote rests normally and retains the bit in the
  accepted-order log;
- all selected makers cancel before replacing when post-only is selected;
- marketable hedges retain their explicit IOC/taker policy and never acquire
  the post-only bit.

## First causal screen (after implementation checks)

Hold the current price-elastic population and all clocks fixed. Compare a
short CDF/USD control with ordinary replacement quotes against the same
configuration with `spot_passive_maker_post_only=true`, paired seeds 101 and
103. Record: post-only rejections, maker-originated fills, quote presence,
two-sided-book share, fill volume, maker inventory autocorrelation, and
terminal/opening price ratio.

**Prediction:** the treatment has zero maker-refresh fills caused by a
post-only crossing. It may have lower activity or worse book presence; that is
a failed viability gate, not stabilization. P0 does not preregister a price
ratio improvement because it removes only one candidate amplifier while
leaving price-only skew, constant inventory size, flow blindness, and the
activity-generator population intact.

## Kill criteria and adversarial mutations

- If a post-only order fills on admission, reserves funds, consumes an ID, or
  changes depth when it should reject, P0 fails mechanically.
- If a test mutation strips the bit from a crossing quote and the tests do not
  distinguish the resulting fill from a post-only rejection, the detector is
  inadequate.
- If treatment quote/fill/book-presence gates collapse, report the mechanism
  as economically non-viable at this configuration; do not tune its spread or
  add a price pin.
- A quieter path without demonstrated post-only activation is not evidence.

## Interpretation boundary

P0 establishes the passive/aggressive execution boundary only. Asymmetric
inventory sizing, adverse-selection/flow response, and explicit capped
rebalance are later V2-3 slices with separate predictions and controls.
