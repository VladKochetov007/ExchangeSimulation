# ME-002-B timing readiness — pre-outcome source trace

This note describes the existing one-child `Immediate` policy and the separately
versioned cadence replay. It is not an observed ME-002-B result. The ME-002
processing comparison remains attached to its own source and protocol.

| Stage | Production source and modeled time | Boundary |
|---|---|---|
| Venue book and publication | `executionlab` emits an exchange snapshot with a source sequence and simulated publication timestamp. | A publication is not a guaranteed executable quote at a later arrival. |
| Feed delivery | The focal directed mount schedules delivery after the configured market-data delay; the actor logs `book_snapshot_receipt`. | Delivery is distinct from publication; dropped/not-enqueued publications remain visible. |
| Processing | This study fixes `ProcessingDelay=0`: a positive two-sided received snapshot immediately replaces the actor's cached quote. One-sided messages leave the last usable two-sided quote cached. | The completed ME-002 study used 0/120 ms processing; this is not a third factor here. No host CPU time or wall sleep enters the model. |
| First-action gate | `DecisionAfter=1 s` is a threshold, not an independently scheduled order event. The immediate policy's recurring `PollInterval` ticker checks the threshold and cached quote; it sends one child and never revises it. | The proposed 1-ms poll first permits action at 1.000 s; the 80-ms poll at 1.040 s, if a usable quote is already cached. Gate/poll phase is part of the fixed treatment. |
| Same-time ordering | The deterministic runner processes scheduled venue/courier work to a fixed point. The actor drains response, market-data and ticker phases in that priority. At the ticker, due processing completions precede the `decision_tick` and order send. | A same-time delivered quote may be selected at that tick; the replay verifies source, delivery and tick identities instead of inferring order from file traversal. |
| Request and exchange | `sendNext` sends one market-GTC BUY child for the 5-ABC target. The directed request path schedules venue arrival; admission, matching, fills and any rejection occur at exchange time. | Selected local depth need not equal pre-match arrival depth. |
| Response and horizon | Acceptance/rejection, fills and cancellation return on the directed response path. The four-second terminal hook records balances and a two-sided book mark when available. | Exchange execution time and actor receipt time are separately joined. Missing response or incomplete terminal evidence is invalid, not an economic zero. |

The first usable-cache time is the first receipt that leaves a positive
two-sided quote available. For an actual first decision, define
`eligible_from = max(first_usable_cache_time, DecisionAfter)`,
`gate_wait = max(0, DecisionAfter - first_usable_cache_time)` and
`gate_adjusted_poll_wait = decision_at - eligible_from`. Selected quote age is
`decision_at - selected_publication_at`. These are simulation nanoseconds;
no-action worlds leave decision-dependent intervals unavailable. A later first
action changes the encountered market state by design; it does not isolate
pure network or computation speed.

The new cadence cell is locked separately from `LatencyCell`; it holds the
same C0 world with 1/90 ms directed feed/request/response, zero processing,
1/80 ms poll and one fixed 5-ABC target. The existing opaque latency evidence
stream is unchanged. A new typed-plan contract and replay bind the selected
poll interval and reject missing, duplicated, misphased or reordered ticks,
future-state receipts, source/response mismatches, fee/ledger errors and
incomplete terminal evidence. Synthetic neighboring-gate fixtures exercise
phase without adding economic cells. The 1-ms path is compared against the
unchanged ME-002 reconstruction on the same fixture.

The venue pre-match depth and continuous quote lifetime are not fully
established by this snapshot evidence. Report observable publication and
selected-snapshot states, order arrival and realized fills separately. If
no exact pre-match state is reconstructed, the answer to that subquestion is
`NOT_RECONSTRUCTIBLE`, not an inferred equal book. The retained ME-001
sampled-depth episodes motivate the numerical poll level but are not
continuous executable opportunity durations.
