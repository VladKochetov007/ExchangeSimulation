# V2-4 L1-P — delivery-liability decision-phase screen

Status: **preregistered before phase-control implementation, configuration
rendering, smoke execution, or L1-P simulation.** This is a narrow timing
identification screen following the completed L1 local-motive result. It does
not retune an economic parameter and does not replace, demote, or add any
participant population.

## Question

All current simulation-time tickers begin after one whole interval. Therefore
the L1 CDF/USD liability hedgers' two-second decisions are aligned with each
other and with other periodic work. L1 demonstrated a local motive under that
one timing relationship, but cannot show that the result is phase-insensitive.

L1-P asks:

> Holding the L1-B delivery-liability policy and its entire economic contract
> fixed, does changing only the deterministic phase of its two-second decision
> ticker preserve independently reconstructed local-gap reduction and the
> prespecified CDF/USD non-collapse floor?

This is not an assertion that one phase is economically better. Its purpose is
to detect an accidental cadence-lattice dependency before a later deployment
or roster-allocation claim.

## Phase representation and required implementation contract

Introduce a reusable deterministic periodic-timer offset capability. For an
interval `I` and phase offset `p`, the first callback occurs at
`simulation_start + I + p`, then at that instant plus integer multiples of
`I`. The accepted domain is `0 <= p < I`.

For `p=0`, the implementation must use the existing ticker path and preserve
the legacy schedule, event ordering, RNG consumption, and positive-world
trajectory exactly. A nonzero phase is a declared V2 timing-semantic change,
not telemetry. It must create no goroutine in deterministic simulation mode
and no RNG draw.

Only `cdf_liability_hedger.decision_phase_offset` is exposed in this slice.
The underlying periodic capability may later serve other actors, but L1-P
must not phase-shift maker refresh, noise flow, supplier, router, market-data,
request-latency, funding, listing, settlement, hedge, or liquidation clocks.

Every L1-P decision row must persist the configured
`decision_phase_offset_nanos` without `omitempty`. The independent L1 replay
must derive its expected decision times from the immutable config and fixed
multivenue start epoch, reject a missing/mismatched phase field for an
explicitly phase-configured run, and reject an off-phase decision. Historic
L0/L1 evidence with no phase setting remains a separately labelled legacy
zero-phase schema and is not rewritten.

## Fixed parent and arms

Both arms start from the final L1-B parent. They retain:

- `delivery_liability` side policy and the exact obligation RNG stream;
- 2-second decision interval and 10-second obligation interval;
- bounded `±200,000,000` obligation steps, `±2,000,000,000` bound, and
  `100,000,000` request cap;
- deposits, CDF/USD symbol, IOC executable-touch form, 5-bps fee schedule,
  local 40-ms feed / 20-ms request links, receipt recording, terminal censor,
  full logging, seeds, all legacy six broad `noise_flow` actors, all makers,
  suppliers, other flows, and every parent clock frequency.

The sole economic timing delta is:

| arm | `decision_phase_offset` | first CDF liability decision after start |
| --- | ---: | --- |
| P0 | 0 s | 2 s |
| P1 | 1 s | 3 s |

`experiment_id` and output/provenance paths may differ. The config renderer
must reject any difference other than these fields.

## Evidence, activation, and mutations

Run full-evidence 30-minute P0/P1 cells at paired seeds 101 and 103. A cell is
complete only after final `greeks.json` and `latency.json`; raw evidence is
retained until every artifact below passes.

Each cell must retain and pass:

1. V2 observation receipt/frontier audit;
2. exact persisted-evidence artifact hash;
3. L1 policy/state/fill replay extended with independent phase validation;
4. CDF/USD full and 10-second-warmup viability reports;
5. the L1 non-collapse floor and per-slot activation counts; and
6. immutable analyzer metadata and configuration hash.

For an exercised cell, each slot must have at least 120 state updates and at
least one accepted request. The CDF/USD non-collapse floor remains at least
150 trades, two distinct taker roles, one maker role, and 95% two-sided
published snapshots after warmup in each venue. These are activity floors,
not ecology claims.

Required tests/mutations before cells are rendered:

- a zero phase follows the legacy timer path and is execution-hash equivalent
  to the absent phase configuration under fresh processes and GOMAXPROCS 1/4;
- a one-second phase fires its first liability tick at start+3s and then every
  two seconds without changing its obligation or policy RNG path;
- invalid negative or interval-or-larger phase is rejected before simulation;
- a mutated phase field, off-phase decision time, or removed explicit
  zero-phase field is caught by offline replay;
- V2 receipts remain at or before every actual submitted decision; and
- evidence recorder on/off remains execution-hash neutral for the nonzero
  phase case.

## Scoring and kill criteria

Score in this order: evidence integrity; phase activation; ordinary local
policy replay; non-collapse floor; then descriptive paired gaps and fills.

The phase-robust local-motive criterion is **SUPPORTED (screening)** only if,
in both P0 and P1 cells and both seeds, all exercised delivery-liability fills
reduce their own independently reconstructed absolute gap, every cell clears
the activation and non-collapse gates, and no phase evidence mismatch occurs.

If phase controls fail to change observed decision timestamps, the comparison
is **NOT IDENTIFIED**. If any valid P1 fill violates local-gap reduction, the
local policy implementation is invalid. If P1 fails the non-collapse floor,
the deployment candidate is phase-sensitive even if its individual policy
remains valid. There is no preregistered directional threshold for the
aggregate price, spread, volume, or mean gap difference between P0 and P1;
those values are descriptive diagnostics only.

Passing L1-P does not demonstrate phase robustness beyond this one half-period
offset, price stability, realistic demand, legacy-actor replacement, or a
causal effect on the wider market. Failure is a result: it blocks L2 roster
allocation until the relevant timing interaction is decomposed.

## Registered outcome (append-only)

The predeclared local-motive and non-collapse criterion passed in P0/P1 ×
seed-{101,103}; the result is **SUPPORTED (screening)** at this one
half-period offset. Every evidence contract and local fill-direction check
passed. The outcome and provenance are recorded in
[`v2-4-l1p-phase-results.md`](v2-4-l1p-phase-results.md) and
`artifacts/v2-4-l1p/l1p-summary.json`.

The descriptive mean-gap and fill changes are large in both seeds, but had no
preregistered directional threshold. They remain a clock-interaction discovery
candidate and do not convert the registered result into an ecology-wide phase
robustness claim.
