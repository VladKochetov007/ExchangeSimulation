# V2-5 P4b — funding conditional on independent perpetual flow

Status: **design frozen before P4b config rendering, preflight, or outcome
inspection**.

## Question and scope

The completed P4 screen identified the full delivered-funding → exact-carry →
target → ordinary spot/perpetual execution chain, but its paired perpetual
basis endpoint was exactly unchanged.  That result is local to the P4
own-mid ecology and its small carry order.  P2a independently established a
different local mechanism: a bounded physical-exposure participant can create
ordinary perpetual orders from a delayed local book.

P4b asks the narrower conditional question:

> When that already validated independent perpetual-flow source is present,
> does the registered funding intervention change the measured spot/perpetual
> basis over the same event-study window?

P4b is **not** a rescue of P4 and does not claim an interaction between the
presence of the exposure participant and funding.  There is no exposure-off
cell in this screen.  The historical P4 pair is contextual evidence only; the
causal comparison is the same-build funding A/B pair with the exposure source
held fixed.

## Causal design

For each development seed:

```text
A: P4 weak-funding control, funding_max_rate_bps = 1
B: P4 inherited-funding treatment, funding_max_rate_bps = 75
```

Both arms add exactly one `PerpExposureHedger` per venue in its already
validated bounded reflected-random-walk mode.  The hedger has no funding or
index input and observes only its delayed local `ABC-PERP` book.  It is held
identical across A and B; its presence is a conditioning feature, not the
registered A/B intervention.  The term-carry allocator remains the P4 actor
and retains its exact-cost, non-atomic, finite-term policy.

The sole A/B economic delta is therefore the existing P4 funding cap.  No
capital, fee, spread, maker, demand, clock, latency, price, or lifecycle input
may be changed after this registration.

## Development and holdout policy

Development pairs use seeds **401 and 409**, the first unused odd prime seeds
after the completed P7c development cells.  Untouched holdouts are reserved as
**419, 421, and 431** before any P4b outcome; they cannot be used for debugging
or tuning.  Two development pairs are screening evidence only.  Holdouts are
licensed only if both development pairs pass every activation and evidence
contract and the registered conditional basis predicate is not falsified.

Each cell runs **98 simulated hours**, preserving P4's twelve eight-hour
commitment intervals and its registered finite-term exit boundary.  The
analysis cutoff and event-study windows are inherited unchanged from P4:
30 seconds before and 300 seconds after the first independently verified
treatment target crossing, with the existing coverage gates and censoring
rule.  Full persisted evidence is required; no compact-only market shortcut is
introduced.

## Six-link and exposure activation contract

The P4 six-link chain remains mandatory and is scored before any basis value:

1. delivered funding identity and age;
2. independently recomputed expected funding;
3. exact fully costed net carry;
4. changed carry target;
5. canonical ordinary, non-atomic spot/perpetual orders and fills;
6. measurable paired basis response.

In addition, every enabled exposure hedger must pass the existing independent
P2 replay: delayed local receipt frontier, reflected exposure-state updates,
target/gap arithmetic, ordinary gateway/order/fill linkage, exact fee, and
per-fill gap reduction.  A missing or invalid exposure path makes the P4b
conditional question **NOT IDENTIFIED**; it is not evidence for or against
funding.

The exposure actor is expected to produce repeated state changes and at least
one accepted, fill-qualified hedge in every venue across each paired seed.  A
zero-fill venue, a future observation, an off-touch request, a forged fill, or
an exposure update that increases the independently replayed gap fails that
cell's activation contract.  The actor's no-funding design is part of the
information-boundary test.

## Registered endpoints and verdicts

Primary endpoint: the exact P4 paired oriented-premium change, computed only
after all six links and the P2 exposure replay pass.  Report every qualifying
venue/seed raw pre mean, post mean, exact delta, and paired A-minus-B delta.
The registered sign rule is unchanged: a positive paired convergence value in
both development seeds is the only **SUPPORTED (screening)** outcome.  Exact
zero or a negative value in either complete pair is **FALSIFIED** at the
conditional basis endpoint.  Missing coverage, missing target crossing, or an
invalid exposure/P4 chain is **NOT IDENTIFIED**; a target change without a
matched ordinary fill is **FALSIFIED AT EXECUTION**.

Supporting endpoints are not substitutes for the primary endpoint:

- funding rate and expected-funding change;
- exact carry components and threshold crossing;
- carry target, admitted/fill-qualified orders, and residual legs;
- exposure decisions, target gaps, accepted/fill-qualified hedges, and
  participant perpetual inventory;
- basis width, persistence, and the existing funding event-study diagnostics.

No profitability, wealth, broad liquidity, price-stability, or realism claim
is licensed by P4b.  A funding-linked participant response without a basis
response remains a complete **FALSIFIED** result at the registered market
endpoint, not a rescued success.

## Evidence and adversarial preflight

Every cell must retain final `greeks.json` and `latency.json`, exact runtime
and offline evidence-artifact digest agreement, complete V2 receipts, P4 term
carry artifacts, and P2 exposure artifacts before scoring.  The independent
analyzer must not import either actor implementation.

Before a long cell, the cheap preflight must reject these mutations:

1. funding rate sign or publication identity changed;
2. exact financing/fee/balance-sheet/margin/leg-risk arithmetic altered;
3. carry target changed without a valid funding crossing;
4. a spot or perpetual leg is dropped, duplicated, made atomic, or forged;
5. exposure target sign, state step, cap, local touch, or filled position is
   altered;
6. a future, delayed, duplicated, or reordered feed receipt is used;
7. an exposure fill attestation is dropped or a zero-price absence is forged;
8. evidence recording changes the execution hash across fresh processes and
   GOMAXPROCS 1/4.

The P4b runner has no prune authority.  Raw evidence is retained until the
P4b-specific measurement contract and result record are complete.

## Interpretation fence

If P4b again has links 1–5 but an exact-zero basis response, the result says
that this larger, independently flowing perpetual ecology still does not make
funding a marginal basis anchor at the registered scale.  It does not justify
raising funding, increasing exposure, changing spreads, or selecting a better
seed after the fact.  If the conditional basis endpoint is positive, promotion
still requires a separately registered exposure-off/on factorial to identify
the interaction and untouched holdout replication.
