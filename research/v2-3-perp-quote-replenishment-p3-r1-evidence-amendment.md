# V2-3 P3 R1 — evidence-contract amendment before replacement screen

Status: **preregistered before R1 configuration rendering or simulation.**

This document preserves the original economic preregistration in
[`v2-3-perp-quote-replenishment-p3-preregistration.md`](v2-3-perp-quote-replenishment-p3-preregistration.md).
It changes only the evidence identity needed to decide whether that claim can
be measured. Attempt 0 is retained and invalidated in
[`v2-3-perp-quote-replenishment-p3-attempt-0.md`](v2-3-perp-quote-replenishment-p3-attempt-0.md).

## Unchanged economic screen

| arm | `perp_maker_replenish_below_bps` |
| --- | ---: |
| A | 0 |
| B | 5,000 |

The replacement screen again uses only seeds 101 and 103, a 5-minute horizon,
full evidence, and the same parent population, cadence/phase, prices, quote
sizes, inventory policy, funding, router, latency, fees, matching rules, and
post-only policy. The rendered-config differ must permit only R1 labels,
provenance, seed, and this already-declared threshold. No P3 market result from
attempt 0 may alter this screen.

## R1 lifecycle identity contract

Every `PARTIAL_FILL` and `FULL_FILL`
`perp_quote_replenishment_lifecycle` row must persist the exact venue
`trade_id`; acknowledgements, rejections, and cancellations persist numeric
`trade_id: 0`. A zero fill trade ID is valid because venue trade sequences begin
at zero. Availability derives from the transition and exact independent venue
event, never from a numeric sentinel.

Independent replay must join a fill by venue, participant, order ID, trade ID,
symbol, side, quantity, exchange timestamp, and actor observation time. It
must reject a missing/wrong trade ID, a duplicate exact venue fill, a side or
residual mismatch, a future actor receipt, and an unexpected policy action. It
may not select a candidate by timestamp/quantity alone.

## R1 validity gate

Before any activation score, every cell requires:

1. final `greeks.json` and `latency.json` sentinels;
2. immutable copied config and run metadata;
3. persisted-evidence artifact hash with `.result.events > 0` and a 64-hex
   digest;
4. valid P3 replay with zero structural, threshold, outcome, duplicate, or
   lifecycle mismatch counters;
5. valid V2 receipt/frontier artifact and ordinary lifecycle/viability output;
   and
6. retained raw logs and `analysis-metadata.json` naming every artifact.

Only then does the original policy activation gate apply. These runs remain
retain-only: no V2 P3 raw evidence may be pruned by the ae13f9a prune gate or
by the R1 workflow.
