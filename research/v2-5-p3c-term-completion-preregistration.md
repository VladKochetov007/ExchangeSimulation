# V2-5 P3c — finite term-completion preregistration

Status: **preregistered before a P3c preflight, P3c world, or P3c raw-evidence
inspection.** P3a established only a locally informed matched entry and P3b
established only that a reconstructed active term reaches an ordinary funding
ledger transfer. P3c is the next lifecycle gate: can the named finite policy
unwind each of its actual terms exactly once, leave no P3 inventory, and
preserve accounting? It is not a funding-anchor, basis, profit, price-quality,
or realism experiment.

## Fixed cell and declared horizon policy

| config | seed | horizon | output root |
| --- | ---: | --- | --- |
| `configs/v2-5-p3c/term-completion-107.json` | 107 | 98 simulated hours | `artifacts/v2-5-p3c/term-completion-107/` |

The config retains every P3b market and financial parameter: two-second
decisions; 12 eight-hour funding intervals; a 100m raw-ABC risk ceiling; 10m
IOC-leg cap; five-bps taker fee; 500 annual-bps spot financing/borrow; one-bps
balance-sheet, margin-risk, leg-risk, and minimum-net-carry charges; 40-ms
delivered public market data; and 20-ms request/response delay. A normalized
config comparison proves that the only non-provenance policy delta is
`term_carry_allocator.mandate_end_at_nano`.

P3b showed the first available term ends at
`1736035201000000000`. P3c declares a **participant-known treasury mandate**
of `1736035205000000000`: that initial term end plus P3's explicit two-tick
(four-second) close budget. It is a persisted economic allocation horizon, not
the simulator termination timestamp. The actor uses it only to refuse a *new*
term whose declared term end plus close budget exceeds the mandate; it does
not expose a simulation end time and does not forcibly liquidate an existing
term. The 98-hour simulation ends roughly two hours after that mandate, giving
ordinary two-second IOC retries a fixed, actor-unknown observation window.

Thus P3c tests a finite one-term treasury mandate rather than the unbounded
P3b allocator. The configuration must not be modified after this registration;
in particular no rate, fee, spread, book, population, clock, latency, capital,
or demand parameter may change in response to its preflight or outcome.

## Cheap evidence preflight

Before the 98-hour full-evidence cell, run exactly one five-minute full-log
preflight with this exact config and seed at
`artifacts/v2-5-p3c/preflight-107/`. It may establish only that the new mandate
does not suppress an otherwise eligible initial plan, that v2 plan/first-fill
records parse, and that required evidence artifacts can be generated. It is
not a P3c lifecycle result and cannot justify a config change.

Build from the committed source head before each launch:

```text
go build -o bin/multivenue ./cmd/multivenue
go build -o bin/mvanalyze ./cmd/mvanalyze
go build -o bin/prunegate ./cmd/prunegate
```

Use full logs and `GOMAXPROCS=4` for the registered full cell. Completion is
only nonempty final `greeks.json` **and** `latency.json`; a process name,
checkpoint, partial sidecar, raw log, or terminal line is not a completion
sentinel. Retain raw JSONL, manifest, checkpoints, receipt/decision sidecars,
and the following extracted artifacts before interpretation or any prune
decision:

```text
termcarry, observationreceipts, derivatives, conservation, positions,
orderlifecycle, lifecycle, streamhash, evidenceartifacthash
```

## Primary gates and predictions

All source/frontier/gateway/venue/actor/arithmetic/lifecycle,
first-exposure, position-continuity, terminal-spot, and terminal-perpetual
P3 counters must be zero. Receipt replay, balance-delta chain replay, generic
position, order-lifecycle, derivative-funding, and lifecycle analyzers must
parse and pass their registered mechanical failure counters. The latency
sidecar must retain the 40-ms market-data and 20-ms request contract. The
persisted-evidence multiset and exact JSON-record artifact digest must be
saved.

The P3c lifecycle activation criterion is all of:

1. at least one P3 v2 `SUBMIT_ENTRY_SPOT_IOC` plan reaches a canonical first
   fill and a separately reconstructed `TERM_ACTIVE` matched pair;
2. every P3 term that becomes `TERM_ACTIVE` produces exactly one later
   `TERM_CLOSED`, after its recorded `term_end`, with no duplicate close;
3. `open_terms == 0`, `closed_terms == active_terms`, and all P3 terminal spot
   and perpetual inventory reconciles to zero from independent balance and
   position evidence;
4. all P3 funding settlements, if a P3 balance transfer occurs before close,
   match exactly one active term and have a valid generic directed/sign audit;
   a valid zero-rate or no-transfer instant is not silently treated as missing;
   and
5. after the close, attempted re-entry is explicitly refused by the declared
   `TERM_HORIZON_CENSORED` policy, rather than by a hidden simulation stop or a
   numeric-price sentinel.

The expected count of active terms and funding payments is deliberately not
fixed from P3b. P3c is a fresh world and may form zero, one, or more eligible
venue-local terms before its finite mandate. It must close every term it does
form. A terminally open term, duplicate close, mismatched terminal position,
or accounting/evidence failure invalidates the completion claim. No initial
matched term is **NOT EXERCISED**, not a reason to alter the cell.

## Adversarial checks and interpretation fence

The existing independent replay mutations remain required: missing or
duplicate close, dropped unwind fill, funding outside an active term, delayed
or reordered receipt, forged first-exposure timestamp, and a one-unit terminal
spot-balance mismatch. P3c's result must also inspect the persisted mandate
deadline and subsequent `TERM_HORIZON_CENSORED` decisions, so a world-end
dependent stop cannot masquerade as a valid finite policy.

A pass supports only:

```text
declared finite treasury mandate + delayed local observations
  -> ordinary non-atomic entry and matched inventory
  -> ordinary funding exposure while active
  -> term-end IOC unwind retries
  -> exactly-once close and flat reconstructed ownership
```

It does not show that the funding expectation is empirically correct, that
terminal net carry is positive after all exit cost, that funding causes basis
mean reversion, or that the allocator is a realistic population. Only after
P3c passes may a paired market-level comparison be preregistered; its
activation must separately show a funding/carry inventory and order-flow
change before any basis or price interpretation.
