# V2 evidence-schema audit — zero-valued enums

Status: **passed targeted preflight** at source revision `522421e`. This is a
narrow evidence-schema audit prompted by the invalid P2 BUY-side observation;
it is not a general serialization rewrite and changes no simulator economics,
scheduler behavior, RNG consumption, or actor-visible state.

## Question and scope

The P2 defect occurred because the valid internal `exchange.Buy` value is zero
and a JSON `omitempty` field silently removed it. The audit searched every Go
JSON field tagged `omitempty`, then traced the zero-valued enum members through
the scientific JSONL evidence, V2-0 binary receipt sidecars, and their
independent decoders. It classified the following representation boundaries:

| zero-valued member | persisted evidence boundary | availability/serialization contract | regression |
| --- | --- | --- | --- |
| `Side(Buy)` | accepted orders, fills, JSON evidence; V2 decision sidecar | required non-optional JSON field or fixed byte slot | `TestPersistentEvidenceRetainsZeroValuedEnums`; `TestMarketDataReceiptAttestsInboxArrival` |
| `OrderType(Market)` | accepted orders; V2 decision sidecar | required non-optional JSON field or fixed byte slot | same |
| `TimeInForce(GTC)` | accepted orders; V2 decision sidecar | required non-optional JSON field or fixed byte slot | same |
| `Visibility(Normal)` | accepted orders | required non-optional JSON field | JSON wire fixture |
| `OrderStatus(Open)` | accepted orders | required non-optional JSON field | JSON wire fixture |
| `PositionSide(Both)` | accepted orders and fill notifications | required non-optional JSON field | JSON wire fixture |
| `MDType(MDSnapshot)` | V2 schedule/receipt sidecar | required fixed byte slot; auditor accepts zero | receipt byte fixture |

The test unmarshals JSON into an independent generic envelope and asserts the
wire field exists and carries the expected zero-valued member. The V2-0 test
asserts the explicit byte offsets for snapshot, BUY, MARKET, and GTC; a fixed
record has no omission mechanism.

## `omitempty` classification

No persisted field whose Go type is one of the numeric enum types above uses
`json:",omitempty"` after the P2 correction.

| field class | classification | decision |
| --- | --- | --- |
| P2 `SideEvidence string` | conditional action evidence, not the numeric enum | omitted only when no BUY/SELL action was selected; every `SUBMIT_IOC` now serializes `BUY` or `SELL` explicitly |
| optional slices/maps, trace IDs, and diagnostic check lists | truly absent/empty collections or optional provenance | retain `omitempty` |
| retry-limit, human-readable reason, action/defer strings | string protocol fields, not numeric enums | retain existing compatibility semantics; required action fields are not omitted |
| actor `EventType`, gateway `QueryType`, and runtime state enums | runtime-only; not directly persisted scientific JSON | out of scope for this evidence-wire audit |

`InstrumentAnnouncement.SettlementPrice int64` still has `omitempty`. It is a
numeric price, not an enum. Because a zero settlement price may be valid under
the planned signed-price contract, it is recorded as a separate price-evidence
API follow-up; this audit does not conflate it with an enum omission or change
lifecycle semantics.

## Related P2 analyzer finding

During the preflight replay, the independent P2 analyzer initially rejected
otherwise exact spot `OrderAccepted` outcomes because those payloads omit a
redundant `symbol`; the immutable book file is named `CDF-USD.jsonl` while the
economic symbol is `CDF/USD`. Commit `3eed5da` recovers only this declared P2
book identity from the file path and adds a regression. It does not alter the
generic scanner, simulator output, or evidence digest.

The four cells produced immediately before this audit replay cleanly under the
corrected analyzer, but are retained only as a **pre-audit diagnostic attempt**.
They must not receive a P2 score. The final campaign must be rebuilt and run
from this audited revision, with A/B × seeds 101/103 extracted from scratch.

## Preflight conclusion

The P2 BUY omission was a real schema bug and is now covered at the only
conditional P2 decision boundary. No second numeric-enum `omitempty` hazard
was found in current persisted scientific/provenance structs. The separate
zero-price lifecycle field remains explicitly open for the signed-price work;
it is not silently considered safe by this result.
