# FFA Ecology Findings Board

## Accepted baseline

- The three-venue `ABC` derivative setup is deterministic under the phase
  runtime and has accepted strict USD option-risk telemetry. It is a valid
  controlled substrate, not an FFA ecology.
- The cross-venue router correctly retains non-atomic FOK failures and local
  residuals, but it routes only `ABC/USD` and does not model transfers or
  terminal wealth.

## FFA-00: option-flow zero semantics

- **Prediction:** a nullable probability field distinguishes omitted baseline
  from explicit all-sell flow.
- **Result:** supported by multivenue and derivsim decode/normalization
  regressions. Omitted values normalize to `0.65` and `0.5` respectively;
  JSON zero survives as zero.
- **Evidence:** `go test ./simulations/multivenue ./simulations/derivsim -count=1`,
  `go test ./... -count=1`, and `go vet ./...` on branch
  `autoresearch/ffa-ecology-gen0`.
- **Consequence:** directional option-flow arms are now expressible. This does
  not validate their Greeks, PnL, or ecology interpretation.

## Blocking contradictions

- `multivenue` cannot compose cross-currency spots with derivatives and
  multiple venue rules; its universe is hard-coded to `ABC/USD`.
- The current market-data sequence is global across symbols/types and has no
  router resynchronization. A high-rate FFA router must not infer a per-book
  gap from that number or trade on unverified local depth.
- Current actor order is deterministic but registration ordered. Ties require
  order-permutation controls or an explicit fairness policy.
- Terminal risk telemetry covers only option dealers; FFA needs population-wide
  accounts in a declared numeraire before a payoff matrix exists.

## Next discriminating decision

Build the declarative scenario/venue profile and population-accounting
interfaces together. Adding another strategy to the hard-coded `ABC` world
before those contracts exist would create activity without an evaluable game.
