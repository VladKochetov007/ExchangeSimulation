# Current Research Finalization - 2026-08-15

## Scope and acceptance rule

The project is being developed as a market-microstructure laboratory, not as a
backtest calibrated to a presumed equilibrium. A market effect is admissible
only when the same fixed seed and configuration reproduce a canonical outcome
under normal host parallelism. A passing unit test is necessary for engine
correctness, but it is not evidence that a strategy effect is real.

This pass used source-level call-stack tracing, adversarial regressions,
fixed-seed replicas, race tests, `go vet`, and long random-walk diagnostics.
No tests were removed. Every confirmed defect listed below has retained
coverage or a reproducible command in `research/experiments.jsonl`.

## Hypotheses and outcomes

| ID | Hypothesis | Outcome | Why it held or failed |
| --- | --- | --- | --- |
| H-001 | Composed public operations can violate coupled ledger/order-book invariants. | Supported. | Individual reservation, fee, position, and book operations were locally plausible but did not constrain their composition. Confirmed examples included negative isolated collateral, overflowed level quantities/position boundaries, foreign-fee overcommit, and per-fill fixed-fee fragmentation. |
| H-003/H-006 | Fixed seed plus race-free Go code is enough for reproducible simulation. | Falsified for the legacy runner; supported for the deterministic-phase runtime. | Quiescence drains work but does not order equal-time actor ticks, responses, market data, venue ingress, or automation. A phase executor defines that order explicitly. Direct and scheduler-backed delayed mounts reproduce when they opt into the ordered courier. |
| H-004 | Aggregate debt can be right while spot/perp debt attribution is wrong. | Supported and fixed. | Repaying more than the perp-attributed part of a loan left `BorrowedSpot` larger than total debt, hiding a later perp borrow from risk. Repayment now retires attribution consistently and balance telemetry matches the client ledger. |
| H-005 | Stop/reconnect and delayed delivery can outlive their logical session. | Supported and fixed in the covered direct paths. | Closed channels and old-session callbacks had no ownership boundary. Lifecycle guards, reconnect closure, early terminal-event replay, and session-scoped market-data ownership now cover the retained scenarios. |
| H-007 | Rewriting the matching core in C++ or Rust is the best current performance improvement. | Rejected for this stage. | Profiling put most wall time in quiescence, ingress draining, map work, synchronization, and logging; price-time matching was about 0.7% of cumulative CPU. Replacing the matcher would not repair causality and would make verification harder. |
| H-008 | Long-run cross-book liquidity loss was a renderer or nondeterminism artifact. | Partially falsified. | An initial all-side withdrawal was an implementation defect and was fixed. After deterministic phases, later GHI/ABC ask depletion is reproducible and caused by an unhedged market maker exhausting finite GHI inventory below its fixed quote lot. It is a model policy limitation, not a chart bug. |
| H-009/H-010 | Dynamic derivatives are observable and option-buy flow creates a short-convexity dealer baseline. | Supported, model-conditioned. | Symbol-tagged derivative fallback logs and validated per-position Black-76 Greeks replace inference from submitted orders. Flat IV and the spot-mid forward proxy prevent realised-volatility claims. |
| H-013 | Live short-dated exposure has higher gamma while day-scale exposure has higher vega. | Supported for one replicated static-IV arm. | A deterministic 48-hour, three-venue baseline has larger peak gamma for TTE <= 6h and larger peak vega for TTE >= 24h on every venue. It is a local-sensitivity observation, not PnL or equilibrium evidence. |

## Confirmed corrections

### Deterministic simulation boundary

The direct, zero-latency simulation path now has a fixed point ordered as
venue-owned jobs, deterministic venue ingress, deterministic egress, then
actors in registration order. Scheduler-backed ticker work is acknowledged
when consumed, including when a periodic job is retired. This was necessary
because the old model could advance simulation time while a goroutine had
removed a due tick but had not yet processed it.

Evidence:

- E-019 falsified legacy quiescence: identical 10-second randomwalk runs
  differed even at `GOMAXPROCS=1`.
- E-021 established a matching 30-minute randomwalk digest across one and
  fourteen OS threads after phase ordering.
- E-026 established that the later spot fee-plan fix did not disturb a
  five-minute randomwalk digest at fourteen threads.
- E-027 establishes the same condition for the derivative ecology: three
  20-second `cmd/reprocheck` replicas at fourteen threads produced
  `8f7693f7fbf07c339dacff7e0c2206ef9114c532b210843c8d775b1406bb30d5`.

The deterministic phase runtime now also owns scheduler-backed delayed gateway
delivery. It has a fixed request, response, and market-data courier order and
is covered by the execution and fee-simulation digest tests. This establishes
repeatability for configured latency experiments; it does not turn a
midpoint-signalled or incompletely accounted strategy into profitability
evidence.

### Spot accounting and matching admission

`df59e03` adds an exact detached-match preflight for the exchange's spot
settlement path. It freezes each execution fee, simulates absolute balances
and aggregate reservations, and validates the residual GTC order before the
live matcher mutates a book. Unfunded resting makers are virtually removed
while planning. Their cancellations are committed only after the incoming
order has a solvent final plan, so a rejected market/FOK order cannot cancel
someone else's quote.

This closes the following confirmed paths:

- a received-asset fixed fee greater than proceeds;
- one order split across several counterparties or iceberg refreshes, each
  charging `FixedFee` once;
- a partial fill that makes the maker's residual reservation unsupported;
- a credit overflowing an asset whose entire previous balance is reserved;
- FOK depth disappearing after virtual fee pruning;
- auto-borrow left behind after a later admission failure.

The design intentionally cancels a maker before its first fill if the complete
candidate batch is not solvent. It favors ledger conservation and deterministic
execution over a solvent-prefix policy. That choice affects displayed liquidity
and should be exposed as a venue policy if made configurable.

Scope boundary: dated futures, perpetuals, and options have different
position-margin settlement and still lack this exact per-execution fee plan.
`FixedFee` remains unsupported for economic derivative experiments until their
ledger-aware counterpart exists.

### Event and market-data ownership

The rejected-order auto-borrow rollback now emits both the compensating balance
change and an explicit `repay` event with reason `order_admission_rollback`.
An event-sourced consumer no longer sees a borrow that disappeared only from
the in-memory ledger.

Market-data publication now gives every subscriber an independently owned
copy, including deep copies of snapshot depth. This prevents an actor from
mutating another subscriber's message or `OrderBook.LastTrade`. Subscriptions
are now scoped to `(symbol, client ID, gateway session)`: reconnect/disconnect
clears old subscriptions, and an old queued unsubscribe cannot remove a new
session's subscription. These are ownership fixes, not a lossless-feed or
sequence-recovery design; bounded gateway channels still need an explicit
backpressure/recovery policy for high-volume experiments.

### Derivative observability and exact rolling tenors

Dynamically listed futures and options now flow to a symbol-tagged derivative
fallback logger. Greek telemetry has a validated Black-76 sensitivity API and
retains immutable signed dealer positions with listing timestamp, expiry,
strike, call/put, model forward, IV, delta, gamma, and vega. `cmd/greekreport`
separates original rolling generations from live remaining-maturity buckets.

Long-running option research exposed two listing defects before any result was
accepted. Epoch-grid floor rounding shortened advertised maturities; an upward
grid repair stretched them. Dated futures and options now retain a per-tenor
rolling expiry and issue successor contracts at exactly listing time plus the
configured tenor. E-029 verifies a full 48-hour three-venue output byte-for-
byte across one and fourteen OS threads. The detailed outcome and caveats are
in [multivenue-48h-greeks-2026-08-15.md](multivenue-48h-greeks-2026-08-15.md).

### Book, position, and lifecycle hardening

Recent retained fixes also reject unrepresentable resting level quantity,
preserve iceberg display bounds, reject the unsupported signed minimum position,
and preflight exact FOK matching. The critical principle is placement-time
validation: matching mutates order state before settlement, so discovering an
invalid fill after `Match` is too late to make an ordinary rejection truthful.

## Falsified research interpretations

1. A coarse simulation step is not a harmless performance/timelapse knob.
   Moving from 1 ms to 6 ms changed timer coalescing and actor reaction
   opportunities, materially changing fills and final mids. Keep the model at
   1 ms; accelerate only rendered playback.
2. The original randomwalk GIF did not prove a price-path renderer bug alone.
   Matplotlib's `+5e4` offset was misleading and was fixed, but blank GHI
   intervals also reflected a real one-sided book in the pre-policy model.
3. The first 200-minute GHI/ABC depletion was not evidence of a stable market
   equilibrium. It is finite unhedged inventory being transferred through
   triangular flow. An inventory-hedged CrossPairMM must include hedge fills,
   latency, cost, partial-fill risk, and inventory reporting before comparison.
4. Midpoint-based cash-and-carry and option-parity submissions are not PnL
   evidence. Their current implementations use non-executable signals and do
   not maintain complete per-leg fill/exposure ledgers. Their reported trade
   counts are intent counts, not completed conversions.

## Open blockers before strategy or volatility conclusions

- Replace the fee-aware basis race's midpoint signal with executable all-in
  depth, an order/leg fill ledger, residual policy, and strict terminal marked
  PnL before interpreting ecological latency advantage as profitability.
- Add exchange-owned `post_mark`, `pre_expiry`, and `post_settlement` risk
  rows. Current actor-owned rows preserve observed positions but are not exact
  terminal settlement state. Add maturity-matched forwards and a dynamic IV
  surface before attributing realised delta/gamma/vega/theta PnL.
- Repair analyzer Epps alignment: independently dropped windows are currently
  paired by slice index rather than timestamp. Also add baseline and terminal
  balance checkpoints before conservation/PnL analysis.
- Replace cash-carry/parity midpoint intent logic with executable all-in edge,
  leg-level fills, residual hedge policy, and terminal marked PnL.
- Implement a ledger-aware per-execution fee plan for margined instruments or
  explicitly reject `FixedFee` on those instruments.
- Choose and test a CrossPairMM inventory policy. The existing finite,
  unhedged policy is valid only as a liquidity-withdrawal scenario, not a
  general equilibrium baseline.

## Reproducible commands

```bash
GOMAXPROCS=14 go test ./... -count=1
GOMAXPROCS=14 go test -race ./actor ./exchange ./simulation ./simulations/randomwalk ./simulations/derivsim -count=1
go vet ./...

# Direct randomwalk repeatability: compare all JSONL hashes from two runs.
GOMAXPROCS=14 go run ./cmd/randomwalk -duration=5m -snapshot-only -logdir="$(mktemp -d)"

# Derivative repeatability: preserves all replicas and writes a manifest.
GOMAXPROCS=14 go run ./cmd/reprocheck \
  -config=research/derivsim-active.json -duration=20s -runs=3 \
  -out="$(mktemp -u /tmp/derivsim-repro-XXXXXX)" -gomaxprocs=14

# Three direct venues with exact rolling 6h/48h option tenors.
GOMAXPROCS=14 go run ./cmd/multivenue \
  -config=research/multivenue-expiry-48h.json -duration=48h \
  -logdir="$(mktemp -d)"
```

For a final 200-minute visualization, use the 1 ms engine clock and compress
time only in the renderer. Generate it only after explicitly choosing whether
finite CrossPairMM inventory loss is the scenario being demonstrated.
