# Deterministic Latency Race, 2026-08-15

## Hypothesis

When two otherwise identical agents observe the same public signal and compete
for a scarce, already profitable two-leg conversion, the lower end-to-end
market-data-plus-request latency agent captures the opportunity. This is an
arrival-mechanism claim, not a claim that low latency is always profitable.

## Controlled Market

- A phase-deterministic exchange publishes one `SIG/USD` trade at simulated
  20 ms.
- `ABC-A/USD` has one $99 ask and `ABC-B/USD` has one $101 bid.
- Alpha and beta each submit a one-unit `Market + FOK` buy in A and sell in B
  after observing the signal. Both accounts start with enough USD and ABC, so
  a completed pair must be base-neutral.
- Physical client IDs are deliberately adverse to the result: alpha is `20`,
  beta is `10`. Equal-latency runs are rejected, so a client-ID tie is never
  described as a latency outcome.
- Default channel latencies are 1 ms for alpha and 5 ms for beta. The ordered
  delayed courier preserves client FIFO and a documented fixed phase order.

## Evidence

Artifacts are
[`alpha fast`](/home/vlad/development/exchange_simulation/research/artifacts/latencylab-alpha-fast-1ms-beta-slow-5ms-2026-08-15.json),
[`label swap`](/home/vlad/development/exchange_simulation/research/artifacts/latencylab-alpha-slow-5ms-beta-fast-1ms-2026-08-15.json),
and
[`reversed actor registration`](/home/vlad/development/exchange_simulation/research/artifacts/latencylab-alpha-fast-reversed-actors-2026-08-15.json).

| Assignment | Signal observed | Result | Actual USD delta | ABC delta |
| --- | --- | --- | ---: | ---: |
| Alpha 1 ms, beta 5 ms | alpha 21 ms, beta 25 ms | alpha fills both legs at $99/$101; beta receives two FOK rejects | alpha +$2, beta $0 | both 0 |
| Alpha 5 ms, beta 1 ms | alpha 25 ms, beta 21 ms | beta fills both legs; alpha receives two FOK rejects | beta +$2, alpha $0 | both 0 |

Reversing actor registration preserves the alpha-fast output byte-for-byte.
The high-ID alpha therefore wins because it arrives earlier in simulated time,
not because of client-ID or actor-registration priority. The lab has matching
reports under `GOMAXPROCS=1` and `14`.

## What Survived

- Lower latency wins this deliberately scarce conversion when the latency gap
  exceeds one simulation step.
- The winner's observed cashflow equals locked cashflow and its exchange
  ledger delta. There is no unhedged ABC residue or skipped fee asset.

## Boundaries

- FOK makes each individual leg atomic only within its own book. The example
  is constructed so the fast agent fills both legs and the slow agent fills
  neither; it does not solve general cross-book leg risk.
- The books contain an exogenous positive gross edge and zero fee. It proves
  response-to-execution priority, not net strategy profitability after fees,
  inventory, adverse selection, or signal decay.
- Equal-latency races, random latency tails, and a multi-opportunity ecology
  remain separate experiments. The next admissible extension must retain this
  per-leg fill and residual ledger.

## Reproduction

```bash
go test ./simulations/latencylab -count=1
go run ./cmd/latencylab -alpha-latency=1ms -beta-latency=5ms
go run ./cmd/latencylab -alpha-latency=5ms -beta-latency=1ms
```
