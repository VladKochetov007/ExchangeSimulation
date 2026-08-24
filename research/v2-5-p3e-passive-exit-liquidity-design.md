# V2-5 P3e — passive finite-term exit under sub-minimum aggressive depth

Status: **implementation, independent-replay, and P0 preflight gates passed;
one 98-hour P0 activation cell is preregistered but not yet completed or
scored.** It follows the valid P3c lifecycle falsification and the invalid
P3d attempt. It does not alter their retained evidence, conclusions, or the
current V2 economic configuration.

## Implementation gate — passed, not an economic result

The implementation is an opt-in P4 policy (`v2_5_p3e_passive_exit_v1`). With
no `passive_exit` configuration, legacy P3 behavior remains IOC-only. When the
policy is declared, it can submit a child only after the ordinary local IOC
size rule emits `EXECUTABLE_SIZE_UNAVAILABLE`; it chooses the delivered
same-side touch and sends an ordinary `LIMIT` / `GTC` / `post_only` request.
The child cannot undercut the effective exchange/actor unwind floor.

The actor records the configured slice and deadline on every P4 decision; each
P4 submission records explicit order type, time-in-force, and a pointer-valued
`post_only` flag so `false` remains distinguishable from a legacy omitted
field. At deadline it records one cancellation request identity tied to the
accepted order. A later unfilled residual emits `PASSIVE_EXIT_DEADLINE_EXPIRED`
and remains open; it does not fabricate a close or stop funding.

Independent replay reconstructs the failed IOC precondition, passive side,
price, legal size, GTC/post-only wire contract, canonical venue acceptance or
rejection, and a successful cancellation chain. It rejects a forged passive
price/side/size, a post-only bit stripped from the canonical venue order, a
passive submission while an IOC was actually legal, and a forged
cancel/order/actor-outcome identity. The fresh-process `GOMAXPROCS=1/4` matrix
also showed evidence OFF/ON equality of the ordered execution hash and stable
receipt/decision sidecar digests while the P4 policy was active but entry was
deliberately mandate-censored. These are mechanism/evidence gates only; they
do not establish that a real P3c-like term will activate P3e.

## New static/evidence finding — C-002

P3c did not merely cache a shallow book while deeper public liquidity was
available. The public `ABC-PERP` snapshots at the recorded term-end interval
contained exactly one visible level on each side:

| venue | executable ask to buy back the P3 short | visible bid | venue minimum |
| --- | ---: | ---: | ---: |
| central | 16,286 | 41,567 | 100,000 |
| south | 16,348 | 8,009 | 100,000 |

The P3c replay found these same quantities at every one of the 3,600
post-term decisions per active venue. The source path explains why this was a
real executable-depth condition rather than a missing-price artifact:

```text
public BookSnapshot (up to 20 displayed levels)
  -> TermCarryAllocator.observeBook
  -> locally delivered best bid / ask and visible quantity
  -> orderFromGap
  -> min(remaining gap, lot cap, matching-touch quantity)
  -> venue minimum-size admission
```

At the observed time there was no second displayed level to aggregate. A
multi-level sweep would therefore not make a legal 100,000-unit aggressive
child. The raw evidence also contains unrelated sub-minimum market-order
rejections on these dust touches. This is evidence of an ecology-level
executable-depth discontinuity, not proof that the allocator chose the wrong
touch or that a numeric zero represented missing price.

P3d's purported `unwind_min_order_size=0` treatment was invalid because it
asked the actor to submit those sub-minimum quantities. Its code correction
now correctly preserves the venue minimum. A rerun with the same premise is
prohibited.

## Question

Can a finite-term treasury reduce a legally sized residual through an ordinary
*passive* exit when the locally delivered aggressive touch is present but below
the venue's minimum order quantity?

This is not a question about funding anchoring, term-carry profitability,
market stabilization, or whether every finite term must close. It isolates a
different local execution policy from P3c/P3d:

```text
aggressive exit unavailable at a legal minimum
  -> post a non-marketable legal minimum at a locally observed passive touch
  -> allow independent future taker flow to fill it, partly or wholly
  -> retain funding, price, and orphan risk until an actual fill closes it
```

## Local economic hypothesis

A participant with an existing matched term can distinguish two choices at
its contractual exit:

1. take the currently displayed opposing touch when it admits a legal,
   participation-bounded IOC child; or
2. when that touch is smaller than the venue minimum, provide a legal minimum
   of closing liquidity at its own-side public touch and wait for counterparties.

The second choice is not a synthetic close. It pays the ordinary maker fee or
rebate, exposes the owner to adverse selection, continues funding and market
risk, risks no fill before its deadline, and never accesses hidden liquidity,
a global book, a future snapshot, a last-price fallback, a forced fill, or a
sub-minimum exception. It is economically different from changing the venue
minimum or silently sweeping an unavailable depth ladder.

## Candidate P3e policy contract

P3e would be a separately versioned, opt-in addition to the term-carry actor.
The existing aggressive path remains unchanged whenever the locally observed
opposing-touch quantity is at least the effective venue minimum.

When that path emits `EXECUTABLE_SIZE_UNAVAILABLE` at term end:

1. Select the **same-side passive touch** from the actor's delivered local
   snapshot: bid for a buy-to-cover; ask for a sell-to-close. Absence is an
   explicit `PASSIVE_EXIT_REFERENCE_UNAVAILABLE` defer, not a numeric price.
2. Submit at most one ordinary `post_only` limit child, using the existing
   request latency and the venue's normal price-time admission. Its quantity is
   `min(remaining position magnitude, declared passive-exit slice)` and must
   be at least the instrument minimum. The first implementation screen fixes
   the slice to exactly one venue minimum rather than optimizing it.
3. Keep the accepted passive child live until an ordinary fill/cancel/reject
   outcome. Partial fills reduce the independently reconstructed exposure;
   they do not mark the term closed. A fully filled child only advances the
   existing unwind state machine if the corresponding actual position is flat.
4. On an ordinary cancel or reject, the next existing actor tick may make one
   new locally observed choice. Repricing requires cancel-before-replace and
   is separately attested. P3e adds no timer, RNG draw, scheduler event,
   hidden order-book read, or special matching path.
5. At a declared passive-exit deadline, an unfilled residual enters an explicit
   `PASSIVE_EXIT_DEADLINE_EXPIRED` state. It remains economically open and
   observable; there is no terminal flatness claim, collateral release, or
   price/funding fallback.

The passive child must be rejected as an *invalid P3e policy event* if it is
marketable at arrival, reserves or borrows more than the normal order path,
uses an order quantity below the venue rule, has no local receipt frontier, or
shares a special client/exchange privilege.

## Causal comparison, only after implementation gates

The eventual immutable comparison would keep P3's existing finite-term policy
and market configuration fixed:

| arm | sole exit-policy difference |
| --- | --- |
| A | legacy: defer when legal aggressive child is unavailable |
| B | P3e: one receipt-bounded, legal post-only slice at the local passive touch |

The first screen is an integrity/activation screen, not a market-outcome
screen. B must have at least one actual P3c-like sub-minimum aggressive-touch
condition and then emit a legal non-marketable passive child through the
ordinary gateway. A must emit no P3e child. Both require valid information,
order-lifecycle, position, balance, funding, and exact persisted-evidence
artifacts.

Only after that passes may a separately preregistered lifecycle screen ask
whether a passive child gets independent counterparty fills, lowers the
remaining term exposure, and completes exactly one close. A lack of fill is a
valid liquidity result, not permission to widen spreads, alter maker sizes,
change clocks, lower the venue minimum, or call the term closed.

## Required independent evidence and falsifiers

The actor-side decision record must retain the selected local passive side,
price, price availability, visible passive quantity, aggressive-touch
availability/quantity, declared slice, effective venue minimum, post-only
intent, source snapshot identity, receipt frontier, outstanding passive order,
and deadline. Independent replay must derive side, limit, size, and
eligibility solely from persisted local snapshots, policy, venue rule, gateway
request, and canonical outcomes.

Required adversarial fixtures include:

- present sub-minimum aggressive touch activates passive eligibility, while an
  absent book does not;
- a present zero/negative price remains present and is rejected only by this
  crypto instrument's explicit positive price domain;
- a forged passive price, side, slice, stale/future source, or below-minimum
  request fails replay;
- a post-only child made marketable at arrival is rejected and cannot fill;
- dropped/duplicated passive fill, cancel, or acceptance is caught;
- partial fill preserves the residual, duplicate close is rejected, and an
  expired deadline cannot fabricate flat positions or stop funding; and
- evidence on/off and fresh process/GOMAXPROCS matrices preserve the ordered
  execution hash.

## Kill criteria and interpretation boundary

P3e is rejected before a full world if a legal passive child cannot be
constructed from public local information, if it changes matching/admission
semantics, or if the auditor cannot distinguish an open passive order from a
closed term. It is falsified at activation if B sees the registered condition
but cannot issue a legal ordinary post-only order. It remains **NOT EXERCISED**
if the condition never occurs.

Even a later P3e lifecycle pass would establish only an auditable finite-term
exit policy under this development population. It would not validate the
12-interval funding-persistence prior, show positive carry after exit, prove
funding causes basis convergence, repair broad liquidity, or license a V2
realism claim.
