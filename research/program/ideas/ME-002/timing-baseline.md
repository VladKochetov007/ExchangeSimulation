# ME-002 prospective timing basis — retained ME-001 evidence only

This is an **exploratory, pre-ME-002-outcome** characterization of the
completed [ME-001 development screen](../ME-001/report.md). It is used only
to choose synthetic ME-002 delays and describe the observation limit. It is
not a new market world, confirmation result or empirical network measurement.

Source: immutable ME-001 C0/S2 runs at seeds `1009`, `1013`, `1019`, each
from simulator commit `13c02d533892e20bfa3153ed789b8a51ef893ddb` and
its original per-run manifest. The Go-only profiling implementation is
`12410c6b1f4c5ea18aea5ee65ca8fbd51d631f00` (after the profiler commit
`e733fea`); clean Go 1.27.0 `-trimpath` binary SHA-256
`1c4af2079a84ae72124cd5a8453aa295fb6113d4a10b67cef7e5f716bfae4210`,
`vcs.modified=false`. The full profile is retained at
`/home/vlad/ExchangeSimulation-me002-development-12410c6/analysis/me001-c0-predecision-timing.json`,
SHA-256 `c4f54789f3aa3359014c3a9e7cebc434e9eacaa2c92c19d18d25eaee30146112`.
This local external path is not a backup claim. The command verifies the
per-run evidence file SHA-256, binary stream execution hash and frame count.
The accepted ME-001 analyzer separately reconstructed the original worlds;
this profiler does not replace it.

| Quantity, pre-decision `[0, 1s)` | Seed 1009 | Seed 1013 | Seed 1019 |
|---|---:|---:|---:|
| Focal public publications / actor receipts | 22 / 22 | 22 / 22 | 22 / 22 |
| Focal policy ticks | 999 | 999 | 999 |
| Publication-to-receipt lag, all observed | 1 ms | 1 ms | 1 ms |
| Publication gap median / p90 | 9 / 100 ms | 9 / 100 ms | 9 / 100 ms |
| Complete sampled best-touch runs; median | 13; 100 ms | 13; 100 ms | 13; 100 ms |
| Complete displayed-depth ≥5 ABC episodes; range | 3; 73–200 ms | 3; 73–100 ms | 2; 73–100 ms |
| ≥5 ABC episode right-censored at 1s | 0 | 1 (99 ms observed) | 0 |
| ≥0.5 ABC complete episodes / right-censored | 0 / 1 (988 ms observed) | 0 / 1 (988 ms observed) | 0 / 1 (988 ms observed) |

The source runner and focal policy tick at 1 ms; the exchange defaults to
100 ms periodic snapshots, but observed publications include shorter and
same-timestamp gaps. A sampled touch run ends when the *next received
snapshot* differs. It does not prove the true exchange touch persisted
unchanged between snapshots. A displayed-depth episode is defined by a
received positive two-sided snapshot whose full visible ask quantity is at
least the stated target, ending at the next received snapshot that fails
that condition. It does not establish that the later-arriving order could
actually fill at those prices. Intervals crossing the one-second cutoff are
right-censored, not counted as complete lifetimes. This is why ME-002 must
report realized receipt/processing/order times and actual fills rather than
asserting an economic effect from nominal milliseconds alone.

Reproduction from the clean profiling commit:

```bash
go build -trimpath -o <new-binary-path> ./cmd/melatencyprofile
<new-binary-path> -symbol ABC/USD -cutoff-ns 1000000000 -client 13 \
  -targets 50000000,500000000 -out <new-output-path> \
  /home/vlad/ExchangeSimulation-me001-development-13c02d5/runs/C0-S2-1009 \
  /home/vlad/ExchangeSimulation-me001-development-13c02d5/runs/C0-S2-1013 \
  /home/vlad/ExchangeSimulation-me001-development-13c02d5/runs/C0-S2-1019
```

The output path must be new; the command refuses to overwrite retained
artifacts. The chosen 1/90 ms network and 0/120 ms processing levels are
prospective synthetic interventions informed by these coarse observed
timescales, not fitted to an ME-002 outcome.
