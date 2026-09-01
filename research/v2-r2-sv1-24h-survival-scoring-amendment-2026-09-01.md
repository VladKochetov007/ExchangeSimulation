# V2-R2-SV1 24-hour survival scoring amendment

Date: 2026-09-01
Candidate: `V2-R2-SV1-24H-CDF-LIQUIDITY`
Scorer contract: `v2-r2-sv1-24h-development-scorer-v1`
Survival contract: `v2-r2-sv1-24h-survival-side-availability-v1`

This amendment registers the numerical survival endpoint before any registered
24-hour SV1 development cell is run. It does not change the simulator,
participant economics, calendar, configs, historical R2 result, or holdout
boundary. It makes the existing preregistered requirements of persistent
one-sided failure and strict valuation auditable by fixing their measurement
parameters.

## Registered measurement

The scorer invokes the existing `mvanalyze` viability metric with:

- simulation start: `1735689600` seconds;
- survival start: `1735693200` seconds, exactly one simulated hour after start;
- viability window: `3600` simulated seconds;
- required symbols: `CDF/USD` on venues `central`, `north`, and `south`;
- required post-warm-up observations: at least one snapshot for every required
  venue book;
- maximum empty-side share: `0.02` per venue book over all post-warm-up
  snapshots;
- maximum empty-side share: `0.02` in every reported post-warm-up CDF/USD
  viability window.

An empty-side snapshot is any viability observation in which either the bid or
ask side of the selected CDF/USD book is absent. The aggregate predicate is
therefore two-sided availability of at least 98% for every required venue, and
the window predicate rejects a persistent one-sided interval even if the
aggregate happens to pass. The scorer requires both predicates; it does not
replace them with a mean across venues or a terminal-only observation.

## Strict terminal valuation

Each of the six primary full cells must also retain a nonempty terminal account
set in `greeks.json`. Every terminal account record must have:

- phase `terminal_post_mark`;
- timestamp `1735776000000000000` nanoseconds;
- positive CDF and USD marks; and
- mark source
  `two_sided_ABC_USD_and_CDF_USD_mid`.

This is a valuation-contract predicate, not a claim that all accounts have
identical wealth or that the market price is realistic.

## Outcome classification

The precommitted scorer emits exactly one of:

- `VIABLE_DEVELOPMENT_CANDIDATE`: all mechanical/evidence, activation,
  terminal, survival, CDF-audit, and anti-cheating predicates pass;
- `NON-VIABLE_AT_24H_MARKET_SURVIVAL_GATE`: evidence, activation, CDF audit,
  and anti-cheating predicates pass, but strict terminal valuation or the
  survival endpoint fails;
- `INVALID_DEVELOPMENT_EVIDENCE`: a required evidence, provenance, extraction,
  activation, or audit contract fails. This is not interpreted as an economic
  negative result.

The scorer writes each derived measurement atomically and refuses to overwrite
an existing result. Failed derived measurements are retained with an
`.invalid` suffix for diagnosis. No raw source evidence is removed by this
contract.

The reserved holdout seeds `619`, `631`, and `641` remain absent and unread by
the development scorer. A passing development score is not freeze
authorization and cannot open the holdout boundary.
