# V2-5 P3a — term-carry allocator integrity result

Status: **SUPPORTED (development screening), narrowly.** The enabled finite
term-carry allocator reached a locally informed, ordinary, independently
replayable matched spot/perpetual entry path in this one five-minute
development world. This is **not** evidence that funding anchors price, that
the declared 12-interval persistence belief is realistic, that carry was
realized, or that basis converged.

The immutable preregistration is
[`v2-5-p3a-term-carry-preregistration.md`](v2-5-p3a-term-carry-preregistration.md).
The machine-readable verdict is
[`p3a-verdict.json`](artifacts/v2-5-p3a/p3a-verdict.json). Raw evidence and
all required extracted artifacts remain retained under
`research/artifacts/v2-5-p3a/`; this screen is not safe to prune.

## Provenance

| item | A: disabled | B: enabled |
| --- | --- | --- |
| config / SHA-256 | `configs/v2-5-p3a/A-107.json` / `b381be43daed1c7cfd7d05bb632b81e8870282d9a1a34c26977ba6c7fb1de935` | `configs/v2-5-p3a/B-107.json` / `cab3740164d8176f47d079d3c9a4086f5b069d7caec2ff7119660502220c1cf1` |
| seed / horizon / process setting | 107 / 5 simulated min / `GOMAXPROCS=3` | same |
| simulator source / SHA-256 | `fe5152ce57affd96cb9472a27026bcc3e56ecc38` / `7d32a373c76994ee590010e1e8a6fc56a3585491bf85a9047ebdacebae829822` | same |
| final execution observations / ordered hash | 56,352 / `98bf11a0e8cad9f5da5d3d00cfc99908bd6005964f9ba159c42e770aaed1d19a` | 56,189 / `f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2` |
| persisted evidence records / multiset digest | 56,802 / `062e880fe30913314c8ccdb1d7e7bb660eb098a9fe984be80fd774bca1eed417` | 56,648 / `4955ab18bf51a723a3c7050822a994e838b642c7b94f01a361b6e150dd105dc6` |
| persisted evidence-artifact digest | `229422399354ff81bc82071b652a34264d08234dc55f8d87468100597c9bf512` | `15eff06bd198c851fbad2335acc751100176a918e8d51a7003cbc0ea6ffc377c` |

The final reanalysis was rebuilt after the retained-evidence V-039/V-040
repairs and V-041's versioned first-exposure replay at `55cd06a`; its SHA-256
is `942871e5e01be82090238a01b55281cbf937c30318a02b9706480a285bf0ecc4`.
Those repairs are analyzer-only and did not regenerate either world. The
simulator's immutable A/B delta, after removing provenance strings, is only
`term_carry_allocator.enabled: false → true`.

## Required evidence contract

Both completion sentinels (`greeks.json`, `latency.json`) are nonempty. Both
cells retain raw venue JSONL, manifests, checkpoints, V2 receipts, V2 decision
sidecars, and all seven preregistered extracted metrics.

| check | A | B |
| --- | ---: | ---: |
| term-carry replay valid | yes | yes |
| receipt replay (schedules / delivered receipts / scalar decisions) | 2,679 / 2,670 / 0 | 2,679 / 2,670 / 4 |
| source/frontier/gateway/arithmetic/lifecycle/position errors | 0 / 0 / 0 / 0 / 0 / 0 | 0 / 0 / 0 / 0 / 0 / 0 |
| terminal perp / terminal spot balance mismatches | 0 / 0 | 0 / 0 |
| terminal delta-chain mismatches / broken chains | 0 / 0 | 0 / 0 |
| generic order-lifecycle errors | 0 | 0 |

All three allocator links delivered their market-data observations at exactly
40 ms and request observations at exactly 20 ms. Each had three terminally
undelivered market-data schedules, which were scheduled after the requested
world horizon; receipt replay classifies them as terminal rather than missing
due evidence. The B USD conservation residual is -5 raw quote units
(relative magnitude `4.95e-15`), the existing bounded integer-truncation
residual; ABC is exact and all A asset residuals are exact.

## Registered A/B activation result

| arm | policy decisions | submitted / accepted / rejected / fills / cancelled | active / open / closed terms | funding settlements while active |
| --- | ---: | ---: | ---: | ---: |
| A, disabled | 450 | 0 / 0 / 0 / 0 / 0 | 0 / 0 / 0 | 0 |
| B, enabled | 450 | 4 / 4 / 0 / 5 / 0 | 2 / 2 / 0 | 0 |

The control evaluated its local policy but emitted only three
`NOT_SUBSCRIBED` and 447 `POLICY_DISABLED` records. Thus a live feed or
recorder did not itself create orders. The enabled arm shows two submitted
spot IOC legs, two submitted perpetual IOC legs, and ordinary venue
acceptance. Its exact lifecycle action counts also contain 239 `TERM_ACTIVE`,
54 economically explained `NET_CARRY_BELOW_MINIMUM` deferrals, 145
`ZERO_PREMIUM` deferrals, three initial `FUNDING_UNAVAILABLE`, and two
`LOCAL_REFERENCE_UNAVAILABLE` decisions. The latter are explicit observed
deferrals, not hidden zero-price substitutions.

The independently replayed B terms are held at central and south with
terminal spot-balance deltas equal to their reconstructed spot inventory and
terminal perpetual positions equal to the opposite inventory. North did not
activate a term. Five minutes is far shorter than the eight-hour funding
interval and declared 96-hour commitment, so two open terminal-censored terms
and zero funding settlements are the registered outcome, not a lifecycle
failure.

## Interpretation and next gate

`B - A` identifies only the installed allocator's enabled ordinary-entry path
under its declared local-source and 12-interval belief contract. It does not
identify any market-level funding channel. P3b may now be preregistered as a
separate nine-hour realization cell whose primary activation condition is at
least one funding settlement while a reconstructed matched term remains
active. P3b must use the v2 plan/first-exposure fields, and its finite 10m
initial commitment must be described exactly rather than conflated with the
100m risk ceiling. No basis, profitability, price, spread, clock, latency, or
population result from P3a will be used to tune P3b.
