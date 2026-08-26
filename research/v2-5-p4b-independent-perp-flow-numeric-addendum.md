# V2-5 P4b — immutable numeric addendum

Status: **immutable before P4b config rendering, preflight, or outcome
inspection**.  The rendered JSON files under `research/configs/v2-5-p4b/`
are the executable authority.  This addendum records why each number is
present; no value may be changed in response to a P4b outcome.

## Treatment and source revision

| choice | value | class | ex-ante rationale |
|---|---:|:---:|---|
| funding control cap | 1 bps/interval | A | exact weak-funding control from completed P4 |
| funding treatment cap | 75 bps/interval | A | exact inherited-funding P4 treatment |
| A/B serialization | only `funding_max_rate_bps` differs after normalization | A | preserve the validated P4 sole intervention |
| term-carry policy | P4 v4 passive-exit policy unchanged | A | P4 six-link actor already passed activation/execution integrity |
| source simulator/evidence contract | current V2 head, full evidence | A | no simulator semantic change is introduced by this screen |
| development seeds | 401, 409 | C | first unused odd prime seeds after P7c development; reserved before outcome |
| untouched holdouts | 419, 421, 431 | C | next unused odd primes, reserved before outcome and forbidden for debugging |

## Independent perpetual-flow condition

The exposure participant is copied exactly from the validated P2a bounded
physical-exposure contract.  It is present and enabled in both A and B; this
is a fixed conditioning source, not the A/B intervention.

| choice | value | class | ex-ante rationale |
|---|---:|:---:|---|
| participant policy | one `PerpExposureHedger` per venue, reflected random-walk mode | A | P2a's validated local physical-exposure representation |
| symbol | `ABC-PERP` | A | P2a contract and P4 perpetual leg |
| decision interval | 2 s | A | P2a validated decision clock |
| exposure update interval | 10 s | A | P2a validated state clock, distinct from decisions |
| exposure step | 10,000,000 raw ABC (0.1 ABC) | A | P2a declared finite exposure increment |
| absolute exposure bound | 100,000,000 raw ABC (1 ABC) | A | P2a declared finite motive balance sheet |
| request cap | 10,000,000 raw ABC (0.1 ABC) | A | P2a one-step ordinary IOC cap |
| tick size | 1,000,000 raw quote units | A | P2a executable positive-price grid |
| initial quote balance | 20,000,000,000,000 raw USD | A | P2a prefunded account; no implicit reserve |
| initial perp margin | 10,000,000,000,000 raw USD | A | P2a ordinary margin account |
| exposure feed delay | 40 ms delivered market data | A | P2a public-feed contract (`delay=20ms`, `market_data_scale=2`) |
| exposure request delay | 20 ms | A | P2a execution-latency leg |
| exposure fee | configured 5 bps taker fee | A | exchange fee inherited unchanged |

No exposure magnitude is selected from a P4 basis response.  The source actor
is the previously registered and independently audited P2a contract.

## Market, lifecycle, and population inputs

All fields not listed below are copied byte-for-byte from P4 `A-107.json`
before the seed/metadata substitutions and from P4 `B-107.json` for the
treatment funding cap.  The P4 environment is intentionally retained rather
than made more active after observing its null.

| choice | value | class | ex-ante rationale |
|---|---:|:---:|---|
| run horizon | 98 h | A | P4 twelve eight-hour commitment intervals and its registered cutoff |
| step / snapshot / automation / quote | 1 s / 1 s / 1 s / 1 s | A | P4 clocks |
| noise interval and phase | 2 s / 0 | A | P4 broad-flow cadence and phase |
| funding interval | 28,800 s | A | P4 central funding horizon and carry financials |
| commitment intervals | 12 | A | P4 finite term |
| mandate end | 1,736,035,205,000,000,000 ns | A | P4 declared absolute contract time |
| passive-exit deadline | 1,736,038,805,000,000,000 ns | A | P4 declared one-hour post-settlement boundary |
| term target / lot | 100,000,000 / 10,000,000 raw ABC | A | P4 exact-cost target and leg lot |
| minimum order | 100,000 raw ABC | A | venue minimum inherited from P4 |
| taker fee | 5 bps | A | exchange and term-carry fee equality |
| spot/perp makers | 2 spot, 1 perpetual | A | P4 roster |
| maker quote quantity | 20,000,000 raw ABC | A | P4 displayed depth |
| maker anchor | `own_mid` | A | P4 environment; no price-discovery redesign is bundled |
| option/future/noise flow | P4 values | A | hold all unrelated populations fixed |
| latency profiles | P4 values plus `perp_exposure_hedger` | A | only declared new participant link is added |

The exposure actor's receipt and decision recorders are enabled in both arms.
The P4 term-carry recorder remains enabled.  Receipt roles are exactly:

```json
["term_carry_allocator", "perp_exposure_hedger"]
```

`strict_population_accounting` remains true, and both final `greeks.json` and
`latency.json` are mandatory completion sentinels.

## Analysis and censoring

The P4 analysis cutoff remains
`1736038805000000000` ns.  For each first independently verified treatment
target crossing, use the existing exact 30-second pre / 300-second post
oriented-premium event study, requiring the unchanged sample coverage gates.
No qualifying crossing, invalid chain, or insufficient coverage is converted
to numeric zero; it is `NOT IDENTIFIED` or censored under the registered rules.

The exposure actor's repeated state updates and local hedge orders are
supporting activation observables.  They never substitute for the P4 basis
endpoint.  P4b has no profitability, wealth, stability, or realism endpoint.
