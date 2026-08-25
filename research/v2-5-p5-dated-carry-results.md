# V2-5 P5 dated-carry development result

Status: **NOT EXERCISED**, exactly as registered. This is a development
classification, not a funding, basis, profitability, or realism result.

The immutable P5 configs and numeric addendum were fixed before these worlds.
The four cells used the same simulator binary (`12abec970d33544ad5e59c2286c7e774fde00bdc641191c0af8536ef56a1d589`),
simulator build revision `9a9c5903fd9d38cad3076b58823ddc9b18aa0c46`, 26-hour
horizon, full evidence, and the registered paired seeds 117 and 119. The
config hashes are recorded in each cell's `run-metadata.json` and the exact
runtime/offline evidence artifact agreement is recorded in
`analysis-metadata.json`.

| seed | arm | decisions | exact-cost candidates | eligible | target changes | submitted | fills | evidence records | evidence digest |
|---:|:---:|---:|---:|---:|---:|---:|---:|---:|:---|
| 117 | A shadow | 140400 | 126888 | 0 | 0 | 0 | 0 | 14266784 | `b8117ea2487548910c7e797dc7f4e046882c26108a3017e727189659b71467ae` |
| 117 | B active | 140400 | 126888 | 0 | 0 | 0 | 0 | 14266784 | `a8a623088e59ac8e5caf41ff18ca6b30f239a7f7465ee91d10227adff0f81766` |
| 119 | A shadow | 140400 | 126888 | 0 | 0 | 0 | 0 | 14177078 | `0238a99362d99250f9af206a0d23e58f4542aa24813e12a0cadfaeaff97b8fc6` |
| 119 | B active | 140400 | 126888 | 0 | 0 | 0 | 0 | 14177078 | `c46f468910bea08ae2de5b6bd873dfc66657187c1ff8f40a97f1481e339219bf` |

All four cell contracts passed independently: participant receipt/frontier
validation, exact cost replay, policy serialization, gateway admission,
canonical venue outcomes, positions/fills, conservation, lifecycle,
settlement, expiry, and artifact-digest equality. The mandate sidecar was
also valid in all cells (`936` decisions, `240` admitted/fill-linked child
orders, `2,400,000,000` filled quantity per cell); this is an activity
diagnostic only and is not a carry activation.

The pair artifacts (`pair-117.json`, `pair-119.json`) show valid control and
treatment evidence contracts and the sole intended `trade_enabled` config
delta, but both have `no_treatment_eligible_terms`. Therefore the registered
chain stops before target activation. No basis window was measured, no
development statistic exists, and the untouched seeds 139/149/151 were not
run. No P5 number is changed in response to this outcome.

The machine-readable verdict is
`research/artifacts/v2-5-p5/development-verdict.json`; the reproducible
scoring command is `scripts/score-v2-5-p5-development.sh`.

## Interpretation and next gate

The result says only that the frozen P5 net-carry hurdle was not reached by
the observed executable books under either paired seed. It does not show that
dated carry is ineffective in general, nor that the implementation is wrong.
The preregistered response is to preserve the result, not tune the hurdle,
capital, spreads, clocks, or demand. P5 holdout promotion is not licensed.
The next research gate is the separately registered V2-6 staged options
ecology; P5 remains an inactive mechanism record in the final V2 ledger.
