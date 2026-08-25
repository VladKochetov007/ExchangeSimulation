# V2-6 P6 — immutable numeric addendum

Status: **frozen ex ante** with the P6 preregistration. No P6 outcome was
examined while selecting these values.

## Provenance classes

* **A — inherited:** copied from the completed V2-4 rendered environment so
  stage differences are not confounded by a new base ecology.
* **B — economic/exogenous:** a participant objective or contract term chosen
  from an external liability interpretation.
* **C — design:** an experimental boundary or finite sample choice fixed before
  outcomes.

## Fixed environment and horizon

* source environment: `research/configs/v2-4-l1p3/A-107.json`, with only the
  seed, experiment metadata, and the registered option-stage fields changed
  (A);
* venues/rules, makers, non-option actors, feeds, latency profiles, funding,
  passive-maker policy, clocks, phases, and evidence mode: byte-identical
  within a seed across O0–O4 (A);
* development seeds: 211 and 213 (C, unused paired primes after P5);
* untouched holdouts: 223, 227, 229 (C, not used for debugging);
* simulated horizon: 8 h (C, four inherited 2 h short-option listing windows
  are available while retaining a terminal buffer);
* full persisted evidence, receipt/frontier sidecars, final `greeks.json` and
  `latency.json` sentinels (A/evidence contract);
* option-IV market analysis: only persisted market trades/quotes and the
  declared positive-domain Black–76 inversion (A/model contract).

## Stage values

| field | O0 | O1 | O2 | O3 | O4 | class/rationale |
|---|---:|---:|---:|---:|---:|---|
| `option_dealer_count` | 1 | 3 | 3 | 3 | 3 | A in O1+; O0 is C to expose the simple reference |
| dealer vol model | flat | realized `[300,1800,7200]` s, premiums `[1,1.2,1.5]`, floor .2, ceiling 3 | same | same | same | C: strike-flat heterogeneity without a smile prior |
| `dealer_hedge_mode` | off | off | on | on | on | C: O2 is first hedge-feedback stage |
| hedge policies | `none` | `none` | `banded,static,timed` | same | same | A/C: inherited policy roster, enabled only at O2 |
| `option_liability_user` | absent | absent | one | one | one | B: finite downside-liability objective |
| `option_value_taker_count` | 0 | 0 | 0 | 1 | 1 | C: minority explicit SABR prior |
| value-taker model | absent | absent | absent | SABR alpha `[.6]`, beta 1, rho −.25, nu .7 | same | C: inherited prior parameters, activated only O3 |
| `vanna_volga_desk_count` | 0 | 0 | 0 | 0 | 1 | C: one explicit risk-transfer desk |
| random option flow | inherited count/probability | identical | identical | identical | identical | A: activity-control population, no vol belief |

The liability user has an explicit target of `1.0` ABC option units per venue,
executes `0.1`-unit IOC children, targets the listed put nearest 95% of the
latest delivered ABC/USD midpoint, and refuses an ask above
`10,000 USD` per underlying unit (B). Its decision cadence is 5 s at phase 0,
with a 20 ms request / 40 ms market-data link (A for the link; C for the
finite objective and phase). It submits no order when the local underlying,
put ask, target quantity, budget, or terminal round trip is unavailable; each
defer reason is persisted. It has finite local balances and no borrowing
privilege beyond ordinary exchange admission (B). The endowment is 1,000 ABC
units and 100,000,000 USD in the spot wallet, plus 10,000,000 USD in the perp
wallet. The latter is an explicit inherited derivative-trader capital
allocation because option premium reservation is charged from the perp wallet;
it is not a price or execution guarantee.

The O2 liability target and premium budget are deliberately not selected to
guarantee a fill. A zero-fill outcome is a valid liquidity result and makes
the O2 activation gate fail rather than being rescued by changing demand.

## Analysis cutoffs

Option surface observations begin after the first 30 minutes and exclude the
final 30 minutes (C). A listed generation counts only if its listing, at least
one market-price observation, and settlement/expiry lifecycle are independently
reconstructed. No stage is promoted to holdout on a convenient subset of
strikes or venues.
