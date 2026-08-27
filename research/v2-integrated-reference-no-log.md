# Integrated-reference no-log parity contract

`research/configs/v2-integrated/reference-dev-601-none.json` is a derived
execution-neutral parity input for the preregistered five-minute integrated
smoke. It keeps every economic, actor, feed, router, clock, latency, seed, and
identity field of `reference-dev-601.json` unchanged. Only `log_mode`, the
seven evidence-recorder flags below, and the description differ:

- `record_maker_quote_size_decisions`
- `record_maker_inventory_rebalance_decisions`
- `record_liability_hedger_decisions`
- `record_noise_flow_phase_decisions`
- `record_option_liability_user_decisions`
- `record_market_data_receipts`
- `record_decision_frontier_vectors`

Those recorders are evidence-only and are not read by actors, scheduling, RNG,
matching, or accounting. The simulator validator rejects the actor-specific
rows, receipts, and frontier vectors when `log_mode=none`; the current validator
intentionally couples those sidecars to full-mode raw decision evidence (for
example, CDF rebalance and liability-user evidence), which cannot be persisted
with `log_mode=none`.
The full-evidence run is therefore the sole source for the complete scientific
evidence contract. This companion is used only for execution-hash neutrality;
the actor does not read any recorder or receipt output.

The derived input is not a new economic treatment and does not alter the
pre-registered integrated reference values.
