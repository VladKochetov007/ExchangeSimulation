# V2 performance methodology and preview-book allocation result

## Scope and contract

Performance work is allowed only when it preserves the simulated world. Its
acceptance surface is therefore two-dimensional:

1. a bounded profile must identify a material cost; and
2. an independently reproduced execution and persisted-evidence digest must
   remain exact before and after the change.

Performance profiles are host observations, not scientific simulation evidence.
They use temporary directories and are never passed to the measurement manifest
or prune gate. A profile wraps `Sim.Run` only; it may not modify simulator
configuration, scheduler events, RNG streams, actor-visible state, or logger
ordering.

For each candidate, retain: source revision, command, duration, seed,
`GOMAXPROCS`, wall time, maximum RSS, CPU/allocation top tables, execution
hash, and (when raw logging is enabled) exact persisted-evidence artifact hash.
Promote only changes with an explicit semantic argument and a cheap falsifier.
Do not turn a profile result into an economic claim.

## Protocol

1. Profile one bounded representative cell before changing code. Separate
   logging, compact evidence, scheduler, matching, actor, and option work.
2. Form a local hypothesis naming allocation or CPU source, not an assumed
   optimization.
3. Change one semantic-neutral unit only. Add a regression test for its stated
   invariant.
4. Run focused tests, race-sensitive tests where state is shared, and an exact
   before/after checkpoint comparison from fresh binaries.
5. When the change can touch persisted bytes, also compare runtime/offline
   evidence artifact digests. No serializer is accepted by merely parsing its
   own output.
6. Reprofile same cell. Treat one wall-clock sample as screening evidence;
   compare allocation-space and repeated medians before making a throughput
   promise.
7. Stop when remaining hotspot requires an economic or evidence-contract
   redesign. Record it rather than disguising it as a micro-optimization.

## P-001 — preview-book capacity allocation

### Hypothesis

Spot admission and FOK preflight clone both book sides to invoke the exact
matching engine. `book.NewBook` reserved 1,024 order-map and 256 level-map
slots for every short-lived clone. The clone already knows source index
cardinality, so capacity hints should remove allocation/GC work while leaving
all queue and matching semantics unchanged.

### Bounded profile

Configuration: `research/configs/frozen-baseline-2026-08-22.json`; seed 101;
10 simulated minutes; `log_mode=none`; host default `GOMAXPROCS`; command-level
CPU and sampled allocation profiles. The before binary was `6152c9b`; the
candidate contains only `NewBookWithCapacity` and use by the preview clone.

| measurement | before | candidate | screening change |
|---|---:|---:|---:|
| wall time | 5.19 s | 4.39 s | -15% |
| maximum RSS | 576,144 KiB | 589,564 KiB | noisy single-run result |
| sampled allocation space | 9,857.42 MiB | 2,148.56 MiB | -78% |
| `book.NewBook` allocation | 7,344.53 MiB (74.5%) | no longer material | removed root cause |

Before, CPU samples were dominated by GC scanning (`runtime.scanObject` 22.9%
flat) and type-pointer traversal (17.7%). After, `book.NewBook` no longer
appears in the allocation top table. GC remains meaningful, so P-001 is not a
claim that all throughput limits are solved.

### Semantic argument and falsifiers

`NewBook` retains its historical long-lived capacities. Only detached preview
books call `NewBookWithCapacity(source.Side, len(source.Orders),
len(source.Limits))`. Book map capacity is not part of public book state;
matching walks linked price/queue order, and this path does not range over
these maps. The source sizes are conservative because a source index can retain
more rows than active preview queues.

The change would be rejected if it changed queue priority, matching results,
order admission, execution checkpoints, raw evidence records, or host-race
behavior. `TestNewBookWithCapacityPreservesBookSemantics` exercises order,
best-price, and cancel behavior from zero capacity hints.

Fresh GOMAXPROCS=1 binaries, same 10-second baseline cell and checkpoint interval:

| binary | execution observations | ordered execution hash |
|---|---:|---|
| `6152c9b` | 6,982 | `c7fd560fc61663dc80379a40adce087e20923b09620976b57a25ac7bcc03ada6` |
| P-001 candidate | 6,982 | `c7fd560fc61663dc80379a40adce087e20923b09620976b57a25ac7bcc03ada6` |

With `log_mode=full`, both binaries additionally produced 6,982 persisted
records and the exact evidence-artifact digest
`f6d2b7064955b6d4aed99d625e8bea6007e5e511b12afebe52ccd70f0803f225`.
The existing fresh-process logging/GOMAX determinism regression also passes on
the candidate.

### Decision

P-001 is a supported semantics-preserving allocation optimization. It is not a
new simulator freeze and does not alter V2 economic interpretation. Its profile
cell is too short and single-sampled to promise a 24-hour wall-clock reduction;
run repeated larger cells only when campaign throughput requires it.

## Serializer decision: `goccy/go-json` deferred

Full raw logging previously made `encoding/json.Marshal` a material CPU cost,
but the four-field envelope allocation was already removed while preserving
literal legacy bytes. Current bounded no-log profiles show preview cloning, not
JSON, as the dominant avoidable allocation. Replacing JSON libraries now would
mix a telemetry byte-contract experiment with a throughput claim.

Before any serializer trial, create a corpus covering every persisted event
type, nested map payload, escaping edge case, integer boundary, null, and
floating-point field. Require byte-for-byte JSON equality (not merely decoded
object equality), runtime and offline artifact-digest equality, malformed-input
parser tests, and fresh-process logging-on/off execution neutrality. If one
event differs, either retain `encoding/json` or declare a versioned evidence
format migration with regenerated controls; do not silently change an evidence
digest.

## Next measurements

1. Repeat full-log profiles after V2-2 router path exists; quantify JSON versus
   matching after P-001 rather than extrapolating an obsolete percentage.
2. Attribute allocation between `prepareSpotExecutionPlan`, event courier,
   book snapshots, and logging with same duration/configuration and at least
   three samples per cell.
3. Only then consider bounded pools or compact telemetry. Every pool needs a
   reset/aliasing mutation and race test; allocator savings alone are not an
   acceptance criterion.
4. Do not start C++ work. P-001 confirms a Go-local allocation issue was
   sufficient to remove the dominant cost without changing market semantics.
