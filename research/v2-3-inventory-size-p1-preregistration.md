# V2-3 P1 — inventory-asymmetric quoted size

Status: **preregistered before implementation, rendered configuration, or
simulation.** This is a narrow mechanism-identification screen following the
completed P0 passive-refresh contract. It is not a price-stability, calibration,
or robustness claim.

## Question and fixed boundary

P0 established an explicit distinction between a passive quote refresh and an
aggressive action:

- post-only exchange admission; and
- cancel-before-replace actor submission order (P0 arm C).

Those two P0-C settings are held fixed here. The price-elastic population,
population counts, spreads, price inventory skew, reference policy, latency,
clocks, request ordering, demand, and volatility-size elasticity are also held
fixed. P1 does **not** implement an inventory rebalance and it does not alter
quote prices.

The observed remaining limitation is that a long Stoikov maker can move its
reservation price yet continue to display the same bid and ask quantity. That
does not make its marginal willingness to acquire versus dispose of inventory
explicit in displayed depth.

## Local mechanism and scope

P1 adds `spot_stoikov_inventory_size_skew_bps` to the V2 configuration. It
applies only to `StoikovMarketMaker` instances quoting a `SpotInstrument`:
`ABC/USD`, `CDF/USD`, and `ABC/CDF` in the cross-asset roster. It deliberately
does not change the perpetual Stoikov maker, derivative makers, fixed-distance
makers, imbalance makers, bootstrap/latent books, hedges, or explicit router
orders. Those remain distinct policy families and later experiments.

The control sets the new field to zero. The treatment sets it to **5,000 bps**.
The configuration validator admits only `[0, 5,000]`, so each side remains at
least half of its volatility-adjusted base quote size at the configured full
risk limit. This prevents a size treatment from silently becoming a
withdraw-the-book treatment.

For each quote decision, let:

- `Q` be the existing volatility-adjusted symmetric quote size;
- `r` be the risk position already used by the current maker inventory
  fraction: `NetDelta` when the existing hedge policy is configured, otherwise
  raw spot inventory;
- `L` be the existing positive inventory limit; and
- `m = min(abs(r), L)`.

The implementation uses fixed-point integer arithmetic, with truncation
toward zero at each named division:

```text
full_adjustment = floor(Q * size_skew_bps / 10,000)
adjustment      = floor(full_adjustment * m / L)
```

If `r > 0` (long), `(bid_qty, ask_qty) = (Q - adjustment, Q + adjustment)`.
If `r < 0` (short), the pair is reversed. At `r = 0`, both sizes are `Q`.
The policy is odd under inventory-sign reversal and preserves the pair's total
quoted size `2Q`; it redistributes displayed marginal appetite rather than
injecting or removing total depth. All products use the existing checked
`TryMulDiv` primitive. An unrepresentable policy calculation is an explicit
instrumentation/error condition in a fixture, never a fallback to a zero
quantity.

P1 changes neither the price pair nor the exchange's post-only decision. It
continues to send the two independently delayed C-order requests; a size is a
request quantity, not a reservation, fill, or execution privilege.

## Decision evidence contract

Each scoped Stoikov quote decision, in both control and treatment, must emit
one compact persisted `maker_quote_size_decision` record. Its required fields
are:

```text
venue_id, symbol, maker, decision_time,
client_id, bid_request_id, ask_request_id,
base_volatility_size, risk_position, inventory_limit,
size_skew_bps, full_adjustment, adjustment,
bid_qty, ask_qty, post_only, cancel_before_replace
```

The event is emitted at the actor decision point before either request is sent;
it is not reconstructed from a later book snapshot. It is instrumentation
only: no scheduler event, RNG draw, actor-visible state, callback into the
matching engine, or request ordering may be introduced. The existing raw order
acceptance/rejection evidence remains the independent record of what actually
reached the venue.

`maker_quote_size_decision` belongs to the persisted-evidence artifact domain,
not the ordered execution-stream hash domain. Its writer must therefore bypass
the checkpoint sink while retaining ordinary JSONL persistence. A paired
logging-on/off fixture compares the simulated execution hash with the recorder
enabled/disabled; an evidence record that changes that hash fails the P1
instrumentation contract.

The P1 analyzer must join each decision side to independent accepted/rejected
order evidence by the exact `(venue_id, client_id, request_id)` tuple. Accepted
order evidence must therefore persist the originating `request_id` alongside
its existing flat order fields. It must separately report a missing,
duplicate, or wrong-side relation rather than inferring an outcome from a
timestamp, or treating an absent order as a zero-sized quote. Its policy
activation check uses the decision event itself; it must not use post-hoc
inventory or fill outcomes to define the treatment.

## Preregistered short screen

Render four full-evidence, five-minute worlds from the same fixed parent
configuration used by P0-R1 C:

| Arm | seed | `spot_stoikov_inventory_size_skew_bps` | Meaning |
| --- | ---: | ---: | --- |
| P1-A | 101, 103 | 0 | P0-C passive refresh and existing price skew, symmetric size control. |
| P1-B | 101, 103 | 5,000 | P0-C plus spot-Stoikov inventory-asymmetric size. |

The rendered-input differ permits only provenance labels, seed, complete raw
evidence / V2 receipt settings inherited from P0-R1, and this one new field.
It must reject an unintended change to any price, spread, reference, hedge,
clock, latency, population, post-only, or cancel-order setting. Each cell is
complete only after final `greeks.json`, `latency.json`, the exact persisted
evidence artifact digest, and P1-specific extracted artifacts exist.

### Activation predictions

For P1-B, in every seed with at least one nonzero-risk decision:

1. every emitted record has `size_skew_bps=5,000` and non-negative,
   representable quantities;
2. zero-risk decisions remain exactly symmetric;
3. long risk has `ask_qty >= bid_qty`, short risk has `bid_qty >= ask_qty`,
   and a nonzero adjustment whenever integer precision permits one;
4. the decision's price, post-only bit, and actor-order flag match P0-C;
5. accepted/rejected request quantities agree with the associated decision;
   a rejected post-only request is a rejection, not evidence of a zero quote.

P1-A must emit the same evidence schema with `size_skew_bps=0`, zero
adjustment, and exactly symmetric sizes. It is the direct control for P1-B.

### Primary outcomes

The primary causal outcome is **activation**: the signed association between
risk position and `(ask_qty - bid_qty)` at the decision point. P1-B predicts
the declared deterministic sign relation above; P1-A predicts zero difference.
This is scored before any market-level statistic.

Secondary, descriptive outcomes are reported separately by seed:

- mean absolute maker net delta and its lag-one autocorrelation;
- CDF/USD and all scoped spot-book quote presence, two-sided share, touch
  depth, trade count, traded volume, and scoped-maker accepted/rejected
  activity;
- distribution of post-only rejections by maker class; and
- terminal/opening price ratio, labelled descriptive only.

No price-stability result rescues failed activation, and no quieter book is
called a success without the fixed viability measurements.

### Viability gates

For every cell and seed:

1. every scoped spot Stoikov maker has a decision record and either accepted
   quote activity or an explicit post-only rejection;
2. CDF/USD has nonzero snapshot count, nonzero two-sided share, trades, and
   traded volume;
3. each scoped spot book has at least one two-sided observation or is reported
   as a failed viability gate, never imputed as a zero spread/depth result;
4. P1 decision records and accepted/rejected orders are structurally
   parsable, with every unmatched relation reported; and
5. V2-0 receipt/frontier evidence and the persisted-evidence artifact digest
   validate for the full cell.

## Kill criteria, falsification, and mutations

- If the policy changes a quote price, post-only admission bit, actor ordering,
  refresh cadence, or any non-spot Stoikov maker, the implementation has
  exceeded P1 scope and the screen is invalid.
- If the treatment has no nonzero-risk decisions, it is **NOT EXERCISED**, not
  a successful inventory result.
- If nonzero-risk decisions occur but the deterministic side-size relation
  does not change, P1 is **FALSIFIED / inactive**; do not tune its coefficient.
- If price/size decision evidence cannot be joined to venue request evidence,
  P1 is **NOT IDENTIFIED** rather than scored from book snapshots.
- If activity or two-sided-book gates collapse, record a non-viable policy;
  do not change spreads, clocks, demand, or the size coefficient after seeing
  the screen.

The focused adversarial suite must include a side-swap mutation (long inventory
increases bid instead of ask) and a coefficient-strip mutation. The activation
detector must reject both. Unit fixtures must cover zero, long, short, clamped
over-limit, volatility-scaled base size, rounding-to-zero adjustment, and an
overflow refusal. A logging-on/off determinism fixture must prove the new
decision evidence does not alter the simulated trajectory.

## Interpretation limit

P1 can establish that the declared inventory-asymmetric displayed-size policy
is activated and can identify its short-horizon viability consequences. It
cannot establish that price stability has emerged, that a particular coefficient
is calibrated, or that the policy is realistic across all maker families. A
separate P2 is required for explicit costly/restricted inventory rebalance;
flow/adverse-selection response remains another separate mechanism.
