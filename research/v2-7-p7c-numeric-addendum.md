# V2-7 P7c numeric addendum

Status: **immutable before P7c outcomes**. This addendum freezes the numbers
and the rendered control/treatment config family. No P7c outcome or preflight
world is used to choose a value.

| choice | value | class | ex-ante rationale |
|---|---|---|---|
| venue roster | north, central, south | A | inherited V2/P7b environment |
| cross-asset graph | false | A | inherited perp-only distress scope |
| fixed physical exposure | -1,000,000,000 raw ABC | A | inherited 10-ABC physical liability |
| target perp exposure | +1,000,000,000 raw ABC per venue | A | exact hedge of the declared liability |
| request cap | 250,000,000 raw ABC | A | inherited bounded ordinary IOC execution |
| quote wallet | 50,000,000 raw USD | A | inherited participant operating wallet |
| tick | 10,000 raw quote | A | inherited ABC-PERP grid |
| public-feed delay | 40 ms | A | inherited local-feed information contract |
| decision interval | 2 s | A | inherited actor clock |
| initial perp margin | 5,500,000,000 raw USD ($55,000) | A | inherited P7b near-initial-margin level; not reselected |
| full-run horizon | 48 h | C | natural two-day physical-liability risk window; tests horizon reachability |
| preflight horizon | 15 min | C | cheap mechanics-only gate; cannot score distress |
| development seeds | 367, 371 | C | fresh odd seeds reserved before P7c outcomes |
| holdout seeds | 373, 379, 383 | C | fresh seeds reserved before outcomes; forbidden until promotion |
| evidence | full + receipts/frontiers + strict risk | A | inherited scientific evidence contract |

The inherited venue funding intervals are 28,800 s (north), 3,600 s
(central), and 7,200 s (south). A 48-hour run therefore contains six, 48 and
24 scheduled funding instants respectively. The horizon is a risk-window
choice, not an observed-price or trigger-time selection. No spread, demand,
liquidity, capital, margin rate, fee, borrow, latency or clock value is changed
from the P7b treatment settings.

The exact config hashes are recorded below after rendering and before any P7c
world is run. Holdout configs are not rendered unless the development
promotion rule is met.

| cell | SHA-256 |
|---|---|
| C-367 | `TO_BE_RENDERED` |
| C-371 | `TO_BE_RENDERED` |
| T-367 | `TO_BE_RENDERED` |
| T-371 | `TO_BE_RENDERED` |
