# V2-0: participant information-boundary evidence

## Status

Completed V2 instrumentation foundation. It is telemetry only and is disabled
by default. It does not alter the ae13f9a frozen configuration or its
simulator semantics. The independently tested V2-1 cache, remote-feed, and
heterogeneous-roster construction controls build on this contract; their
separate economics gates remain open.

## Claim and scope

For an explicitly audited, delayed participant link, V2-0 establishes:

```text
courier admission
  -> modeled scheduled arrival
  -> successful actor-inbox delivery
  -> emitted order/quote decision
```

The proof is about actor-visible public-feed messages on configured delayed
links. It does not claim that every exchange broadcast was delivered to every
subscriber: a source-side buffer drop is a separate transport event, and no
participant may treat an absent message as observed. It also does not yet
record no-op strategy ticks; an **audited decision** is an emitted order
(including a maker quote) at the actor-facing gateway boundary. V2-1's first
single-feed cache may use this contract unchanged. A later multi-source
composite must extend the decision frontier from one link prefix to an explicit
vector before it can support a heterogeneous-maker claim.

## Sidecar contract, schema 2

`market-data-evidence-v2.json` names three fixed-width files and their
individual SHA-256 digests.

| ledger | bytes/row | row time | purpose |
|---|---:|---|---|
| `market-data-schedules-v2.bin` | 88 | source publication / courier scheduled time | one row when a source message enters the delayed courier |
| `market-data-receipts-v2.bin` | 88 | actual actor-inbox insertion | one row only after the inbox send succeeds |
| `market-data-decisions-v2.bin` | 96 | actor request emission time | order/quote request and exact local frontier before request latency |

Schedule and receipt rows contain client ID; immutable source venue / unique
participant link / role catalog IDs; symbol; MD type; source sequence; 128-bit
fingerprint of all actor-visible message fields; publication, scheduled, and
delivery timestamps; monotone per-link ordinal; and globally monotone evidence
event ordinal. A link is `venue/role/client/<id>`, not a shared role label.
The fingerprint remains nonzero for directed lifecycle messages that retain a
legacy zero sequence number.

A decision row contains client/link/symbol, order details, decision time, and
the linked feed's frontier: receipt ordinal, delivered time, and a 128-bit
rolling receipt-prefix digest. Its evidence event ordinal permits independent
cross-file ordering reconstruction at equal simulated timestamps.

For order requests, the persisted `price` field follows the exchange request
protocol rather than the market-reference contract: a `Market` order must
carry `price=0` because it has no limit price, while a `LimitOrder` must carry
a positive price. This structural request zero is not a missing observation,
mark, or reference price; the independent vector decoder validates it from the
order type.

The sidecars are only invoked after courier admission or successful inbox
delivery. They create no scheduler events, use no RNG, expose no state to
actors, and start no telemetry goroutine. The writer finalizes after the final
simulation fixed point; an encoding or finalization error fails `Sim.Run`.

## Independent audit

`mvanalyze -metric observationreceipts -json <run>` uses `analysis/receipts.go`,
which independently decodes the binary layout. It validates:

- each file hash and record count against the manifest;
- catalog identity, field domains, reserved bytes, publication <= schedule <=
  delivery, and per-link FIFO ordinals;
- source-message uniqueness from `(client, link, sequence, fingerprint)`;
- schedule-to-receipt bijection and missing delivery when scheduled on or
  before terminal simulated time;
- globally contiguous evidence-event ordering;
- each decision's exact reconstructed receipt-prefix digest and
  `frontier_delivered_at <= decision_time`.

The scenario controller rejects a declared audited role with no participant,
or with any participant whose custom mount bypassed the recorder. Link catalogs
also preserve role identity for that completion check.

## Adversarial tests

The independent fixture recomputes hashes after each mutation. It catches:

- an observation scheduled before publication (future injection);
- a dropped due receipt;
- delayed receipt cited by an earlier decision;
- duplicated source identity;
- reordered per-link schedule ordinal.

Fresh subprocess test executes the frozen baseline configuration for GOMAXPROCS
1 and 4, with evidence OFF and ON. It requires one exact
`execution_stream_hash` across all four worlds, a valid nonempty sidecar for
ON worlds, and byte-identical schedule/receipt/decision counts and digests
across GOMAXPROCS.

## Short performance evidence

Profile corpus: frozen baseline configuration, seed 101, 30 simulated minutes,
GOMAXPROCS=1, logging disabled, checkpoint interval 60 seconds, binary built
from this uncommitted V2-0 candidate.

| mode | wall | max RSS | ordered execution hash |
|---|---:|---:|---|
| evidence OFF | 31.55 s | 1.04 GiB | `537853d5e03b7a5d…` |
| evidence ON, `spot_maker` | 36.05 s | 1.08 GiB | `537853d5e03b7a5d…` |

The ON artifact audited validly: 293,480 schedules, 293,408 receipts, and
6,218 decisions. The 72 not-yet-received schedules were beyond the run's
terminal simulated time and are therefore explicitly classified rather than
silently treated as drops. CPU/allocation profiles show overall allocations
remain dominated by matching preview-book clones (~76% alloc-space); V2-0 adds
about 4.9% of sampled allocation space, including 0.09 GiB in message
fingerprinting. The full-log control was 38.27 s / 1.08 GiB RSS; full-log
V2-0 was 40.27 s / 1.04 GiB RSS, with the same execution hash. The 30-minute
ON sidecars total 49.9 MiB, implying roughly 2.4 GiB for this one audited role
at 24 hours if activity is stationary. This is acceptable only because the
measurement manifest will retain it as a compact required evidence artifact.

Profiling was performed with command-level CPU/alloc controls that wrap
`Sim.Run` only; they do not enter config, the scheduler, or actor state. Full
logging spends roughly one quarter of sampled CPU in `encoding/json.Marshal`.
The next committed optimization replaces only the four-field outer
`map[string]any` envelope with a byte-identical struct: five-run microbench
means were about 1.36 µs / 944 B / 19 allocs versus 0.77 µs / 368 B / 9
allocs. A 30-minute full-log replay retained the identical execution hash and
the exact persisted-artifact digest while reducing wall time from 38.27 to
36.34 seconds. This is a local serializer-envelope change, not a `go-json`
adoption. `goccy/go-json` remains deferred pending a dedicated byte-corpus,
analyzer, and mutation comparison.

## Gate

V2-0 passes only when all tests, race tests, fresh-process test, full-log
profiling, command builds, and source review remain green. `goccy/go-json` is
not an approved dependency from this measurement: JSON is material, but a
serializer swap can change exact evidence bytes. It requires separate corpus
byte-equivalence, fresh-process digest equality, analyzer, and mutation proof.
