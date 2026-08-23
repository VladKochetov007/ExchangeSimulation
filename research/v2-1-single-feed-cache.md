# V2-1a — single participant-local delayed public-feed cache

## Status

Implemented and smoke-validated. This is one mechanism-isolation slice, not
the heterogeneous informed-maker population and not a V2 economic result.
`ae13f9a` remains historical and untouched. Enabling this path starts a V2
world; it is rejected by the legacy shared-index configuration.

## Failure under investigation

The ae13f9a control synchronized venues through a shared instantaneous
consensus index. The index itself was endogenous, but it was available to
makers before any participant-specific public feed, so a price-discovery claim
could not distinguish local observation from simulator-global state.

## Local hypothesis

Before a maker can form a cross-venue composite, it must be able to maintain a
private cache which advances only when a message has reached that maker's own
delayed public-feed inbox. A copied top of book—not an exchange pointer, book
pointer, or consensus object—is sufficient for the first single-source test.

## Implementation

`LocalBookCache` stores one declared venue/symbol's copied best bid, ask,
sizes, source sequence, and publication timestamp. It accepts only a two-sided
snapshot of its symbol, rejects a backwards source sequence, and has no clock
or exchange reference. `StoikovMarketMaker` may opt into it with
`use_local_reference_cache`; `Config.SpotMakerLocalReferenceCache` enables the
path only for ABC/USD spot makers.

The config hard-rejects:

- `maker_anchor=consensus`, which would preserve the shared global input; and
- a missing or zero `spot_maker` latency profile, which would make the claimed
  public-feed delay absent.

The cache receives the same actor-facing snapshots that the maker otherwise
uses. This deliberately makes its first activation behaviorally close to the
own-mid control. Its purpose is information-boundary proof, not a chart change.

## Preregistered smoke checks

| check | prediction | result |
|---|---|---|
| cache activation | every ABC/USD spot maker has a nonempty cache whose declared source is its venue | PASS |
| stale/reordered source mutation | an older sequence cannot overwrite the later local observation | CAUGHT |
| invalid/one-sided snapshot | cannot create usable cache state | CAUGHT |
| V2-0 evidence join | emitted maker orders cite a delivered non-future local frontier | PASS; independent audit valid with nonzero receipts and decisions |
| evidence neutrality | ON/OFF execution hash equal across fresh GOMAXPROCS 1 and 4 | PASS |
| persisted sidecar determinism | schedule/receipt/decision counts and digests equal across those fresh ON worlds | PASS |

The fresh-process test uses a 2-minute deterministic smoke world with a
10-millisecond constant `spot_maker` link. It compares OFF/ON evidence and
GOMAXPROCS 1/4; the cache world has one exact execution hash across all four
cells. It does not reuse the ae13f9a control hash because this is a new V2
configuration.

## Interpretation / limit

This proves a narrow but necessary claim: an activated maker cache can be tied
to delayed actor-visible evidence before each audited quote/order. It does
**not** prove cross-venue information, a local composite, heterogeneous price
discovery, or economic realism.

One trade gateway plus one source has one V2-0 frontier. A remote-feed maker
will necessarily have at least a local trading frontier and a remote feed
frontier. The scalar limitation is now addressed by the separately documented
[V2-1b frontier-vector contract](v2-1-frontier-vectors.md). Do not attach a
remote feed until that vector is activated in a live maker scenario; otherwise
the apparent cross-venue cache would still be unprovable.

## Next experiment

Add one remote public-feed cache to one maker, prove the live vector binds it
to its trading link before every quote, and prove the maker cannot read the
shared consensus index. Only then add heterogeneous sources/weights/latencies.
