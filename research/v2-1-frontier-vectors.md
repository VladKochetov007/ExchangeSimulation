# V2-1b — decision frontier vectors

## Status

Instrumentation substrate implemented and independently mutated. It extends
V2-0 without changing its schema-2 sidecars or claiming a remote maker exists.
No scheduler event, RNG draw, actor-visible value, or telemetry goroutine was
added. The normal actor path has a nil hook and is unchanged.

## Problem

The V2-0 decision row contains one feed frontier. That is enough for V2-1a's
single local cache, but false for a maker that uses both a local trading feed
and a remote public feed. Recording only the trading-link frontier would leave
the remote cache unobservable and make an apparent cross-venue result
unfalsifiable.

## Contract

The optional V2-1b artifact has two fixed-width ledgers:

| ledger | bytes/row | meaning |
|---|---:|---|
| `market-data-decision-vectors-v1.bin` | 72 | one actor order decision, its trading client/link, request fields, time, and declared number of feed components |
| `market-data-frontier-components-v1.bin` | 56 | one `(feed client, link, receipt-prefix)` component of that decision |

`market-data-frontier-vectors-v1.json` binds their hashes to the SHA-256 of
the already-finalized `market-data-evidence-v2.json`. A component names its
feed client, V2-0 link ID, receipt ordinal, delivery time, and rolling prefix
digest. Components are written in sorted `(client, link)` order. Empty
frontiers are rejected: a V2 remote-reference maker must wait for an actual
delivered source observation before it may quote from that source.

For a live remote-maker configuration, the artifact also declares its required
scalar trading client/link. The auditor requires every V2-0 order decision on
that link to have exactly one matching vector. This inverse coverage rule is
essential: checking only that each vector joins a scalar decision would let a
dropped vector row masquerade as a decision with no evidence.

The analyzer independently:

1. audits the V2-0 base receipt evidence;
2. verifies the base-manifest and vector-file digests;
3. reconstructs every receipt prefix from raw V2-0 receipt bytes;
4. checks each component's exact prefix, delivery time, and `delivery <= decision`;
5. checks the declared component count, uniqueness, and order; and
6. joins the vector to the ordinary gateway-emitted scalar order decision; and
7. for each declared required trading link, rejects missing or duplicate
   vectors for scalar gateway decisions.

The last check is deliberate: a vector log is not evidence of a trade request
unless the actor-facing trading gateway independently emitted the same request.

## Tests / mutations

| test | independent prediction | result |
|---|---|---|
| real integration | BaseActor pre-send hook, delayed gateway, V2-0 scalar decision, and vector row join exactly | PASS |
| future injection | changing a component receipt time after its decision invalidates audit even after checksum rewrite | CAUGHT |
| dropped component | deleting all component rows and changing manifest count/digest yields a missing-component failure | CAUGHT |
| dropped decision vector | deleting a vector and its components after rewriting hashes/counts yields a missing-vector failure on the required trading link | CAUGHT |
| empty frontier | recorder refuses a decision that claims a feed before its first receipt | CAUGHT |
| base invariant | V2-0 sidecars remain independently valid and unmodified by the vector artifact | PASS |

## Scope / next gate

This is a provenance prerequisite, not an economics change. It is now attached
to exactly one documented remote-feed smoke world; see
[V2-1c](v2-1-remote-feed-smoke.md). Heterogeneous source membership, weights,
and maker families remain blocked until that one-maker case has its complete
fresh-process and mutation gate.
