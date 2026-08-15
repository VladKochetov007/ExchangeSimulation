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

## FFA-01: venue-local allocation selection

- **Prediction:** matching allocation can be a declared venue characteristic
  without reintroducing map-iteration or host-scheduling causality.
- **Result:** supported for the two implemented mechanisms. `VenueRules`
  selects existing `price_time` or exact `pro_rata` matcher by venue ID; the
  simulator traverses the canonical `VenueIDs` list rather than ranging the
  rule map. Unknown venue IDs and rules reject at normalization.
- **Evidence:** the mixed north-FIFO/central-pro-rata/south-pro-rata world has
  the same venue-log digest at `GOMAXPROCS=1` and `14`; the multivenue package
  and its race suite pass, as do the full project test and vet gates.
- **Limit:** this is a mechanism-selection boundary, not a result about
  liquidity quality. It does not implement real venue hybrids, allocation
  minimums, top allocation, auctions, or per-venue fees.

## FFA-04: population-accounting gate

- **Prediction:** population fitness can be measured only when every
  independently funded account has an explicit initial and terminal marked
  value in the same numeraire.
- **Result:** `strict_population_accounting` records venue ID, client ID, role,
  initial bootstrap-account value, and terminal two-sided-mark account value
  for every connected participant. A missing terminal `ABC/USD` two-sided
  mark fails a strict run instead of silently selecting from partial wealth.
- **Limit:** the current strict builder is USD/ABC-specific. It is a gate for
  the existing control world, not yet a cross-currency valuation graph.

## E-038: strict FFA control harness

- The retained control manifest uses three venue-local policies
  (north FIFO, central/south pro-rata), two spot-flow and two option-flow
  agents per venue, and strict population accounting. Its purpose is to prove
  the manifest-to-account-report pipeline before adding `CDF` or selection.
- This is explicitly not a non-transitive result: it retains one underlying,
  static policies, static IV, and no population update rule.
- Output directories are excluded from the scenario manifest before hashing;
  changing an artifact sink cannot change scenario identity.
- **Accepted control:** five simulated minutes at `GOMAXPROCS=1` and `14`
  produced the same manifest hash
  `a1f81dfffdca0c249591cc23aa62941e546342006e746a37ed1a611a0951adc4`
  and `greeks.json` hash
  `bda88d8bdf9bc6f21ef545e36a7d75f757eec4855603278573471acb6bd58c27`.
  Both reports contain 27 initial and 27 terminal account rows. Raw artifacts
  are retained under `logs/research/ffa-ecology-control-e038-canonical-*`.

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
