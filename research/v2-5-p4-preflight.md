# V2-5 P4 mechanical preflight

Status: **passed; not an economic outcome.** The immutable numeric contract and
cells remain those in
[`v2-5-p4-funding-carry-numeric-addendum.md`](v2-5-p4-funding-carry-numeric-addendum.md).
No preflight observation was used to change funding, costs, capital, target,
liquidity, spread, latency, clock, threshold, horizon, or population.

## Revisions and binaries

The preflight simulator was built at `819d569` after the P4 configs, analyzer,
and fail-closed extraction scripts had been committed. Its SHA-256 was
`b98949bad47fd578da85ba6f8d5456a3568ff85cb7157b6e26b97a7ba47fef2d`.
Commits `70edae9` and `5bfcc21` subsequently corrected and hardened only the
offline basis analyzer. They do not import into `cmd/multivenue` and cannot
alter scheduling, RNG, matching, participant state, or the simulated world.
The development campaign must nevertheless rebuild at its final source
revision and record that binary separately.

The exact economic config was immutable B-107,
`271825ccd0441c73d18a7f0d60e2dfe5c356a82494765acf6a6ac4e6b187f20b`.
All worlds ran for five simulated minutes. This horizon was selected only to
exercise mechanics and is not a P4 treatment cell.

## Determinism and evidence neutrality

Fresh full-evidence processes at `GOMAXPROCS=1` and `GOMAXPROCS=4` and an
evidence-off process at `GOMAXPROCS=4` all ended with exactly:

```text
execution observations  56,189
execution_stream_hash    f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2
```

The evidence-off config was generated mechanically from B-107 by changing only
`log_mode`, `record_market_data_receipts`, `market_data_receipt_roles`, and
`record_term_carry_decisions`. Removing those four instrumentation keys from
both JSON documents produced byte-identical normalized economic configs. The
evidence-off config hash is
`a21756bc383114f1def9960905dfc3be3937cf4e1b78624c8a207f7e1c926d62`.

An initial attempt to combine the full-evidence config with `-log-mode none`
failed closed before simulation with `term-carry decisions require full
persisted evidence`. It created no world directory and remains recorded in the
preflight stderr; it is not evidence.

Both full-evidence processes independently produced the same artifact:

```text
persisted JSON records   56,648
evidence multiset digest 6906866f4af89e05e9b23a68a1733f6b5ea6cc11c93942d90c5af678a1a65e71
```

Offline artifact replay exactly matched each runtime count and digest. Receipt
replay, the general term-carry audit, and the P4 chain passed in both processes,
and the compact receipt/decision sidecars were byte-identical across process
and `GOMAXPROCS` settings.

## Adversarial analyzer findings

The first paired replay correctly refused to score because the P4 basis
analyzer recovered a spot symbol with the generic filename form `ABC-USD`,
while the registered instrument is `ABC/USD`. Actual spot snapshots omit the
symbol from their payload and therefore depend on the already established
spot-file parser. Commit `70edae9` fixed this analysis-only defect and added an
exact actual-wire fixture. No raw evidence changed.

The second review found that a floating-point event-study delta could allow a
roundoff-scale sign to decide the registered positive/zero boundary. Commit
`5bfcc21` now computes every premium, window mean, arm delta, paired difference,
and seed mean as an exact rational. Float values remain presentation fields;
the separately persisted exact value and sign control the verdict. The
constant short preflight correctly reconstructs exact paired statistic `0`,
not a tiny positive value.

The mutation/unit gate catches forged actor arithmetic, reversed funding sign,
changed receipt identity, absent exact carry crossing, target omission,
dropped canonical perpetual fill, one-leg exposure, stale/missing basis,
cutoff leakage, and the real spot-snapshot wire shape. Focused race tests,
`go vet`, and `go test ./...` passed.

## Mechanical activation only

The short same-build A/B replay exercised the complete analyzer path: A had
294 exact-cost deferrals and no submission; B had 56 exact-cost decisions and
two independently linked, matched one-lot executions. This is useful only as a
mechanical preflight. Its exact five-minute basis statistic was zero and is not
the registered 98-hour development result. It cannot establish funding
anchoring, convergence, profitability, robustness, or realism.

Raw preflight evidence is retained under
`research/artifacts/v2-5-p4/preflight-*`. No prune gate has authority over it.
