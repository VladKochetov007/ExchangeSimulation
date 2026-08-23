# V2-2a — executable-router information contract

## Question

Can the existing venue-qualified spot router be promoted from a mechanical
non-atomic-leg test into an information-bounded V2 participant, without
changing its economic selection rule or treating an unobserved quote as a
local observation?

## Existing mechanism and gap

`CrossVenueArb` already has three separately funded venue accounts, evaluates
an executable best bid against an executable best ask, includes configured
taker fees, requires displayed touch quantity for its FOK lot, and leaves a
one-sided fill as venue-local residual inventory. Its legs have modeled public
data, request, and response latency.

Before this slice its custom mounts were intentionally outside V2-0 receipt
coverage. Consequently, a router report could establish that orders were
non-atomic but could not establish what public information preceded a routed
decision. The V2-0 coverage test correctly rejected an attempt to label those
links as audited.

## Local change

For an instrumented V2 router run only:

1. each of its three venue-local accounts receives its own V2-0 delayed-feed
   ledger under the declared `cross_venue_router_tier` role;
2. a route decision is withheld until each declared venue feed has a nonempty
   delivered frontier;
3. each submitted buy or sell FOK request has a V3 decision vector containing
   the three exact receipt prefixes used by the router's comparison;
4. every scalar V2-0 order decision on a router leg is required to have one
   such vector.

The all-three-frontier requirement is deliberately stricter than the old
two-venue opportunity calculation. It makes the information set explicit for
the fixed three-venue router rather than silently allowing an absent third
venue to mean either "unknown" or "not considered". It is a V2 router-policy
change, not a modification of `ae13f9a`.

Evidence-off worlds retain the old construction solely for fresh-process
instrumentation-neutrality controls; they are not scientific evidence runs.

## Fixed initial smoke cell

The construction control has:

- three venues and one router tier;
- independently prefunded spot accounts, one per venue;
- a positive deterministic data/request/response latency on every router leg;
- `record_market_data_receipts` and
  `record_decision_frontier_vectors` enabled;
- `cross_venue_router_tier` as the only router evidence role;
- no asset transfer, shared wallet, direct price repair, or router-specific
  external price source.

It is a bounded smoke, not the later 2x2 price-discovery factorial.

## Predictions and kill criteria

| check | prediction | failure interpretation |
| --- | --- | --- |
| coverage | all three router accounts have individually registered receipt links | V2-0 role coverage is incomplete; stop |
| decision eligibility | no router FOK order precedes a delivered frontier from every declared venue | local information set is ambiguous; stop |
| evidence join | every router scalar order has exactly one three-component vector and every component is delivered no later than its decision | decision provenance is not independently reconstructible; stop |
| activation | the smoke produces at least one qualified router group, or a deliberately seeded targeted unit fixture does | mechanism inactive; no price-discovery inference |
| non-atomicity | a leg rejection after its counterpart fills remains a residual in the per-group report | router is implicitly atomic or report is weak; stop |
| neutrality | evidence OFF/ON and GOMAXPROCS 1/4 retain one execution hash; ON artifacts match across processes | instrumentation altered the world; reject |

## Explicit exclusions

- This spot-only slice has no perp funding or derivative margin cost. Those
  enter a later, separately preregistered carry/router extension; they are not
  hidden as zero cost here.
- The router uses exact FOK execution for one displayed lot. It does not claim
  multi-level optimal sweep sizing or a transfer network.
- Passing this contract does not establish that router activity improves
  synchronization or price discovery. Those are outcomes for the V2-1/V2-2
  informed-makers × routers factorial after activation is proven.

## Adversarial mutations

The independent frontier-vector decoder must reject a future-injected router
component, a dropped router decision vector, and a router request vector that
omits one declared venue frontier. The existing residual-leg unit mutation
continues to establish that one accepted/fill-qualified leg cannot be hidden
by an aggregate completed-group count.

## Decision rule

Promote only the narrow claim that router decisions have an independently
auditable delayed public information set. If activation fails, retain the
mechanical router as an unactivated participant and do not run the 2x2.

## Result — construction gate passed

The generic short baseline probe produced valid scalar receipt evidence but no
router decision. That is a useful negative activation result: it does not
justify a synchronization or price-discovery claim. The required construction
control therefore used legal, non-crossed pre-run resting liquidity with a
known executable north-ask/central-bid edge. It is deliberately a targeted
activation fixture, not a calibrated population world or a scripted price
repair mechanism.

Under that fixture, the router submitted a qualified group and emitted exactly
two scalar FOK requests. The independent V2-0 audit was valid and the V3 audit
joined each scalar decision to exactly three delivered feed frontiers. The
router refuses to send before all three declared feed frontiers exist.

Three adversarial V3 mutations rewrite file hashes and record counts before
audit: a component delivered after its decision, a missing third-venue
component, and deleted decision vectors. The independent decoder rejects all
three. The established residual-leg unit tests remain the non-atomicity
detector; this all-liquidity fixture does not pretend to manufacture a
partial-fill result.

Fresh two-minute processes with GOMAXPROCS 1 and 4 have the same ordered
execution hash with evidence OFF and ON. Evidence-on scalar and vector
sidecars have equal nonzero counts and digests across processor settings; V3
has exactly three components per decision. Evidence-off is a technical
neutrality control only.

During this gate the audit found and corrected two schema/identity defects
before evidence promotion: `Market` request `Price == 0` is a structural wire
value, not an unavailable market price, and router account IDs are venue-local
so the V3 join key must include both `(client_id, link_id)`. No economic result
was generated before those corrections.

The promoted claim is limited to auditable router information provenance. The
2x2 informed-maker × router price-discovery factorial remains blocked on an
economically motivated dislocation source and event-level channel attribution.
