# Repeated-Parent Execution Study, 2026-08-15

## Hypothesis

In a replenishing limit-order book, slicing repeated low-to-mid-frequency
parent orders should reduce all-in target implementation shortfall relative to
one immediate market child. The earlier one-parent study established this
mechanism once; this study tests whether it persists across repeated clients
sharing one venue and alternating directional demand.

## Design

- Twenty paired seeds, `42..61`; one immediate and one five-slice TWAP world
  per seed. The worlds share every configured seed and differ only in parent
  policy.
- One `ABC/USD` direct deterministic venue: four adaptive market makers,
  eight independently seeded delayed random takers, and twenty independent
  parent-order clients. There are 32 participants in every world.
- Parent decisions are staggered one simulated second apart. Sides alternate
  buy/sell to avoid defining a one-way inventory stress as an execution-policy
  effect. Every target is 2 ABC; the TWAP arm sends five 0.4-ABC market
  children 200 ms apart. Each arm therefore supplies a 1 Hz parent sequence
  and a 5 Hz child schedule.
- Duration is 25 simulated seconds. The last parent has a finite constant
  latency drain horizon before shutdown.
- Every parent records decision bid/ask midpoint, requested and filled size,
  exact child fills, quote fees, rejects/cancels, and a strict two-sided
  terminal midpoint. The analysis rejects a parent with unpriced fees, an
  invalid terminal mark, a schedule mismatch, or a quantity inconsistency.

Raw reports: [executionlab-20parents-20seeds-2026-08-15.jsonl](artifacts/executionlab-20parents-20seeds-2026-08-15.jsonl).
Paired aggregate: [executionlab-20parents-20seeds-summary-2026-08-15.json](artifacts/executionlab-20parents-20seeds-summary-2026-08-15.json).

## Result

All 800 parents completed and all had valid two-sided terminal marks. Mean
target shortfall was 9.356 bp for immediate execution and 7.000 bp for TWAP.
The paired TWAP-minus-immediate difference was -2.356 bp, with deterministic
paired-bootstrap 95% interval [-2.377, -2.335] bp. TWAP was lower in all
20 seed worlds; the exact two-sided sign-test p-value is `1.91e-6`.

The seed-42 full JSONL output is byte-identical under `GOMAXPROCS=1` and
`GOMAXPROCS=14` (`sha256=0a52a6f956ebb0865a7672b117e202ebbdaecc5a6dcdece32cba87aa5f253494`).

## Interpretation

This supports a limited market-mechanics claim: under this symmetric,
constant-latency, fast-replenishing ecology, distributing a 2-ABC parent over
five 200 ms children lowers measured all-in execution cost across a sustained
sequence of parent clients. It is stronger than the prior one-parent result
but still not a general TWAP theorem.

It does not test alpha decay, stochastic/correlated latency, hidden liquidity,
participation caps, an urgency utility, informed flow, or a multi-venue route.
The alternating schedule deliberately removes persistent signed meta-order
pressure, so it should not be used to infer trend impact or an equilibrium
strategy allocation.

## Reproduction

```bash
go test ./simulations/executionlab -count=1
mkdir -p logs/research
GOMAXPROCS=14 go run ./cmd/executionlab \
  -seeds=20 -seed=42 -duration=25s -parent-count=20 \
  -parent-interval=1s -slices=5 -slice-interval=200ms \
  > logs/research/executionlab-20parents.jsonl
python3 tools/analyze_executionlab.py logs/research/executionlab-20parents.jsonl
```
