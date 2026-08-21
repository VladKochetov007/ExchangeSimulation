# Test integrity ledger

A validation suite is only evidence if its own tests fail for the reasons they
claim. This file records tests that were found to be measuring something other
than what they asserted, and what was done about them.

## TI-1 — the concurrency fuzzer was asserting host throughput

**Observed failure.** `TestFuzzConcurrentGateways` failed during a routine
`make test` while sixteen simulation processes were saturating the machine:

    concurrent_fuzz_test.go:192: fuzzer depth failure: only 36 trades executed

It then failed 8 times in 12 consecutive runs under that load, and passed 3 of
3 on an idle machine.

**Initial hypothesis, and it was wrong.** `Gateway.Send` is non-blocking by
design and drops onto a full request buffer, so the obvious explanation was
that a loaded host let the buffer fill and silently discarded orders. That
explanation was written down, instrumented, and disproved.

**Instrumentation.** A probe drove 6 clients × 3,000 requests through the same
lossy `Send` path and counted every stage: generated, enqueued, still queued,
and acknowledged (an acknowledgement being the terminal answer to one request,
identified by its own request id).

| measurement | after 3s, as the old test measured | after draining |
|---|---|---|
| generated | 18,000 | 18,000 |
| acknowledged | 4,874 – 6,796 | **18,000** |
| still queued | 11,198 – 13,120 | 0 |
| dropped by `Send` | 6 | **0** |

The six apparent drops were an artefact of reading the two counters at
different instants; draining first accounts for every request.

**Root cause.** Nothing was dropped and nothing was lost. Requests are consumed
by a single exchange goroutine at a few thousand a second, and on a busy host
draining 18,000 of them takes **17–22 seconds**. The old test waited 50ms after
its producers finished, then cancelled automation and asserted on the result.
It was measuring a partially processed queue and reading the shortfall as a
trade count.

**Old test weakness.** It mixed two kinds of assertion in one test. The safety
checks — conservation, position netting, no negative reserves, no crossed book
— are properties of the final state and hold at any speed. The depth check
(`trades < 100`) is a progress assertion, and it was bound to wall-clock time
through a queue whose drain rate depends on the host. Worse, the safety checks
themselves were running against a state that had never quiesced, so they were
weaker than they appeared: most of the workload had not been processed when
they ran.

**What was changed.** Two things, neither of which lowers a threshold or
lengthens a sleep:

1. `TestFuzzConcurrentGateways` now waits for every gateway's request buffer to
   empty before stopping the exchange, so its existing checks run against a
   genuinely quiesced state. The wait has a 30s watchdog that fails loudly on a
   real stall.
2. A new `TestConcurrentGatewayWorkloadAccountsForEveryRequest` carries the
   correctness half on a deterministic workload: a fixed 6 × 400 operations,
   producers released together from a barrier, every request enqueued directly
   rather than through the lossy path, and full stage accounting.

**Redesigned invariants.** None of these depend on machine speed:

- every generated request is enqueued, and every enqueued request receives
  exactly one terminal answer — a request answered zero times is lost and a
  request answered twice is duplicated;
- every trade produces exactly two fill notifications, one per side, and no
  fill identity `(trade, client, order)` is delivered more than once;
- positions net to zero, USD and ABC conserve within the truncation dust bound,
  no reserve or spot balance is negative, no book is left crossed;
- `Gateway.Send` racing `Close` neither panics nor enqueues, checked separately
  and deliberately without asserting how many sends got through.

The watchdog exists to catch a hang. It does not define how much trading should
have happened.

**Stress and repetition.** All of the following were run with the campaign's
sixteen simulation processes still occupying the machine, load average ~22:

| condition | repetitions | result |
|---|---|---|
| `GOMAXPROCS=1` | 8 | pass |
| `GOMAXPROCS=2` | 8 | pass |
| `GOMAXPROCS=4` | 8 | pass |
| `GOMAXPROCS=16` | 8 | pass |
| `-race`, all three concurrency tests | 3 each | pass |
| idle machine | 2 | pass |

Typical accounting from one run: 2,400 generated, 2,400 enqueued, 2,400
acknowledged, 0 rejected, 936 trades, 1,872 fill notifications, exactly 2.00 per
trade, 0 duplicates.

**What this does not establish.** The engine consumes requests from one
goroutine, so throughput is bounded by that goroutine and degrades with host
load. That is a property worth knowing and is not asserted anywhere: there is
no performance test, and this ledger entry is not a claim that the engine is
fast. It is a claim that nothing is lost.

**Economic model untouched.** No simulator behaviour was changed. The edits are
confined to `tests/`.
