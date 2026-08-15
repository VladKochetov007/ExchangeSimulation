# Deterministic Execution-Lab Pilot, 2026-08-15

## Question

Does slicing a parent order lower all-in implementation shortfall relative to
one immediate market order in the current replenishing-book model?

This is an execution-cost question, not a trading-PnL question. Each arm runs
in a separate fixed-seed world. Exogenous noise decisions share a seed; any
subsequent book-path divergence is the policy treatment.

## Model And Measurement

- One `ABC/USD` spot venue with four adaptive market makers and eight delayed
  random takers.
- Parent starts after one simulated second, receives one millisecond request,
  response, and market-data latency, and trades a buy parent.
- Immediate policy sends one market child. TWAP sends five market children 200
  ms apart. Both use a 5 bp quote taker fee.
- All runs use the deterministic phase runtime, including the ordered delayed
  courier. `GOMAXPROCS=1` and `14` produce identical parent reports.
- Every child records request, order ID, requested/filled quantity, quote
  notional, quote fee, reject/cancel status, and venue match timestamps.
- For filled quantity `Q`, decision mid `M0`, quote notional `C`, quote fees
  `F`, and side sign `s` (`+1` buy, `-1` sell), reported filled shortfall is
  `s * (C - Q*M0) + F`. When a terminal two-sided book mid exists, target
  shortfall additionally marks an unfilled remainder at that mid. That is a
  transparent mark-to-complete obligation, not an invented final execution.
  Reports with an unpriced foreign fee or no terminal two-sided mark are
  invalid for all-in target shortfall.

The implementation is [execution.go](/home/vlad/development/exchange_simulation/simulations/executionlab/execution.go),
[sim.go](/home/vlad/development/exchange_simulation/simulations/executionlab/sim.go),
and [cmd/executionlab/main.go](/home/vlad/development/exchange_simulation/cmd/executionlab/main.go).

## Evidence

Twenty paired seeds `42..61`, three parent sizes, raw JSONL committed under
[`research/artifacts`](/home/vlad/development/exchange_simulation/research/artifacts).

| Parent size / cadence | Immediate mean target IS | TWAP mean target IS | Paired TWAP - immediate | Completion observation |
| --- | ---: | ---: | ---: | --- |
| 0.2 ABC | 7.001 bp | 7.001 bp | 0.000 bp | both 100% |
| 2.0 ABC / 1 Hz | 9.272 bp | 7.001 bp | -2.271 bp; TWAP lower 20/20 | both 100% |
| 2.0 ABC / 5 Hz | 9.272 bp | 7.001 bp | -2.271 bp; TWAP lower 20/20 | both 100% |
| 5.0 ABC / 5 Hz | 14.911 bp | 7.283 bp | -7.627 bp; TWAP lower 20/20 | immediate mean fill 4.9325 ABC; TWAP 100% |

The low-size null is expected: each parent remains within displayed touch
liquidity, so slicing changes neither price nor fee. The mid-size result
supports the conditional hypothesis that allowing quote replenishment between
children reduces mechanical impact. The 1 Hz and 5 Hz results are nearly
identical because this ecology replenishes the relevant depth within 200 ms;
they do not establish a general cadence ranking.

The high-size result is completion-adjusted rather than a filled-only
comparison. Immediate execution leaves 0.06749 ABC unfilled on average and
its 15.116 bp filled-only number is not comparable to a complete order.
Its 14.911 bp target shortfall marks that exact residual at the terminal
two-sided book mid. TWAP completes every target at 7.283 bp. This supports
the stated control only under the configured terminal-mark convention.

## Claims That Survived

- A deterministic, nonzero-latency execution experiment is now possible.
- In this particular replenishing ecology, TWAP improves target all-in cost at
  the tested 2 and 5 ABC sizes and is neutral at low participation.
- The result is not an assertion that TWAP always wins: it is conditional on
  five fixed-interval market children, this maker refresh policy, constant
  latency, and no predictive alpha.

## Falsified Or Still Open

- “TWAP is universally better” remains false as a research claim. Different
  urgency, adverse selection, replenishment, or terminal-mark assumptions can
  reverse the result.
- The current lab does not model a bounded random latency tail, venue-arrival
  acknowledgement timestamp, VWAP participation cap, hidden liquidity, or
  alpha decay. Those are next experiment dimensions, not silently assumed.
- Low-latency strategy-profit claims remain untested. The ordered courier
  removes host scheduling as a confounder, but a tiered strategy still needs a
  two-leg execution ledger, fee accounting, and latency-label permutation
  checks.

## Reproduction

```bash
go test ./simulations/executionlab -count=1
go run ./cmd/executionlab -seeds 20 -seed 42 -duration 4s -target-qty 200000000
```

Use separate worlds for policy comparisons. Do not place immediate and TWAP
parents into the same book: the first parent would change the second's
liquidity and invalidate the comparison.
