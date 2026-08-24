# V2-4 L1-P3 — untouched-seed replication result

The registered holdout replication is **MIXED**. Seeds 107 and 109 reproduce
the L1-P2 relative-phase direction and positive interaction; seed 113 reverses
both. Under the preregistered all-three-seed rule, this is not a successful
replication and does not promote the L1-P2 timing result beyond screening.

## Evidence gate

All 12 full-evidence cells (A/B/C/D × 107/109/113) have final
`greeks.json`/`latency.json`, valid receipt and persisted-evidence artifact
audits, valid liability and noise phase replays, 540 liability state updates,
zero nonreducing delivery fills, and passing CDF/USD non-collapse floors. Raw
evidence is retained at `research/artifacts/v2-4-l1p3/{A,B,C,D}/seed-{107,109,113}`
and is not prunable.

All runs use multivenue SHA-256
`c4bdcc1a283b3d9ed82ee41c2775720f3795ff22f6b1af9d6e5ca4b87238dee2`.
The complete per-cell configuration hashes and evidence digests are in
`research/artifacts/v2-4-l1p3/l1p3-summary.json`.

## Exact registered endpoint

`M = absolute_gap_sum / gap_samples` at liability decision times. The table
retains exact raw ratios; decimals are descriptive only.

| seed | A: L0/N0 | B: L1/N0 | C: L0/N1 | D: L1/N1 |
| --- | ---: | ---: | ---: | ---: |
| 107 | 493,658,026,704 / 2,700 = 182,836,306.187 | 267,337,666,261 / 2,697 = 99,124,088.343 | 244,327,202,111 / 2,700 = 90,491,556.337 | 229,629,622,856 / 2,697 = 85,142,611.367 |
| 109 | 403,016,017,145 / 2,700 = 149,265,191.535 | 250,718,208,422 / 2,697 = 92,961,886.697 | 245,599,225,952 / 2,700 = 90,962,676.279 | 456,397,810,956 / 2,697 = 169,224,253.228 |
| 113 | 194,400,597,342 / 2,700 = 72,000,221.238 | 247,913,804,824 / 2,697 = 91,922,063.339 | 244,205,172,921 / 2,700 = 90,446,360.341 | 199,971,233,311 / 2,697 = 74,145,803.971 |

| seed | aligned mean | de-aligned mean | aligned − de-aligned | interaction |
| --- | ---: | ---: | ---: |
| 107 | 133,989,458.777 | 94,807,822.340 | +39,181,636.436 | +78,363,272.873 |
| 109 | 159,244,722.382 | 91,962,281.488 | +67,282,440.894 | +134,564,881.788 |
| 113 | 73,073,012.605 | 91,184,211.840 | −18,111,199.235 | −36,222,398.470 |

The common-denominator integer calculation is recorded in the JSON artifact.
Because the third untouched seed is opposite, the registered verdict is
**MIXED**—not falsified (the three seeds are not all opposite), but not
supported as a holdout replication.

## Interpretation

The L1-P2 result is now a seed-sensitive clock finding rather than a
timing-robust causal attribution. No population, price, spread, latency,
frequency, or inventory parameter may be tuned to restore its average effect.
The retained evidence permits a separate noncausal diagnostic of why seed 113
reverses; that diagnostic cannot rescue the preregistered claim. L2 roster
demotion and any market-level timing claim remain blocked.
