# V2-5 P3b — first realized funding-term preregistration

Status: **preregistered before a P3b world or P3b raw-evidence inspection.**
P3a established only that the enabled allocator can form a locally informed,
ordinary matched pair. P3b asks the narrower next question: does a
versioned-realized first exposure become a matched active term at an ordinary
funding settlement? It is neither a causal A/B market experiment nor a test of
funding anchoring, realized carry profitability, basis convergence, or realism.

## Fixed cell and contract

| config | seed | horizon | output root |
| --- | ---: | --- | --- |
| `configs/v2-5-p3b/term-realization-107.json` | 107 | 9 simulated hours | `artifacts/v2-5-p3b/term-realization-107/` |

The P3 v2 allocator is enabled at each venue with exactly P3a's declared
participant policy: two-second decisions, 12 eight-hour settlement intervals,
100m raw-ABC risk ceiling, 10m IOC leg cap, 5-bps taker fee, 500 annual-bps
long-cash/short-asset financing, and one bps each balance-sheet, margin-risk,
leg-risk, and minimum-net-carry charges. Its delayed public feed is exactly
40 ms, with 20-ms order request latency. It has no mandate deadline and no
access to simulator termination, a global mark, index, PnL, or another venue.

The change from P3a is only the registered horizon and P3 v2 evidence schema:
`plan_created_at` is pre-ingress intent, while `first_exposure_at` is set once
from the first canonical fill. The finite actual initial commitment is one
matched 10m lot when available; 100m is only a risk ceiling. No fee, rate,
spread, clock, population, capital, latency, or demand parameter may change in
response to P3b.

Before the nine-hour cell, one five-minute full-evidence **encoding preflight**
uses this exact immutable configuration, source build, and seed in
`artifacts/v2-5-p3b/preflight-107/`. It may establish only that P3 v2 records
parse, retain nonzero plan/first-exposure fields when entry fills, and satisfy
the ordinary evidence contracts. It is not an additional P3b outcome or an
activation score, and it cannot justify changing the registered nine-hour
configuration.

## Primary activation and evidence gates

Completion means nonempty final `greeks.json` **and** `latency.json`, never a
host process name. Before interpretation retain raw JSONL, manifest,
checkpoints, V2 receipt/decision sidecars, and extract:

```text
termcarry, observationreceipts, derivatives, conservation, positions,
orderlifecycle, lifecycle, streamhash, evidenceartifacthash
```

All term-carry source/frontier/gateway/venue/actor/arithmetic/lifecycle,
position-continuity, terminal-spot, terminal-perpetual, and first-exposure
counters must be zero. The receipt audit must pass. The latency rows must show
the declared 40-ms delivered market-data and 20-ms request delay. Conservation,
position replay, order lifecycle, and the generic derivative funding audit
must parse and show their registered mechanical failures at zero.

The primary activation criterion is all of:

1. at least one P3 v2 `SUBMIT_ENTRY_SPOT_IOC` plan whose subsequent canonical
   `ORDER_FILL` establishes `first_exposure_at`;
2. at least one separately reconstructed `TERM_ACTIVE` with equal-and-opposite
   nonzero spot/perpetual inventory before the settlement instant;
3. `active_term_funding_settlements >= 1`, with no settlement attributed to an
   inactive or overlapping term; and
4. the corresponding venue/symbol funding record has a reconstructed directed,
   sign-consistent payment and no duplicate funding payment.

The funding payment is a ledger transfer, not a model-implied gain. Its amount
and sign will be reported from preserved `balance_change` evidence alongside
the independent contract-wide derivative sign/conservation checks. A zero
payment at a valid present zero funding rate remains a measured economic zero,
not unavailable; this particular cell does not manipulate the rate to produce
one.

## Stop rules and interpretation fence

If no active-term settlement occurs, P3b is **NOT EXERCISED**. If evidence,
receipt, lifecycle, position, funding-direction, or accounting gates fail, the
cell is invalid and no market interpretation is made. A settlement with an
unmatched/orphan position is not an activation pass. Open terms at the
nine-hour terminal are expected because the declared term is 96 hours; they
are explicit censored ownership, not a close claim.

A pass supports only the local chain:

```text
local public observations → named term policy → ordinary non-atomic fills
→ first canonical exposure → matched active inventory → ordinary funding transfer
```

It does not validate the actor's funding-persistence prior, prove positive
realized net carry after all eventual exit costs, or establish a marginal
funding effect on the market. P3c remains required for one eventual unwind and
terminal flatness before a paired market comparison can be preregistered.
