# V2-5 P1a — fee-aware feasibility run plan

Status: **preregistered before execution.** This operational plan instantiates
the contract in
[`v2-5-funding-carry-p1a-feasibility-preregistration.md`](v2-5-funding-carry-p1a-feasibility-preregistration.md).

## Immutable run

| field | value |
| --- | --- |
| config | `configs/v2-5-p1a/fee-aware-107.json` |
| seed / horizon | 107 / 30 simulated minutes |
| simulator binary | `scratch/multivenue-v2-5-p1a` rebuilt from the committed source head |
| analyzer binary | `scratch/mvanalyze-v2-5-p1a` rebuilt from the committed source head |
| prune gate | `scratch/prunegate-v2-5-p1a` rebuilt from the committed source head |
| process setting | `GOMAXPROCS=12` |
| output | `artifacts/v2-5-p1a/fee-aware-107/` — it must not pre-exist |

The normalizer preflight completed against the committed config with a
two-second non-evidence diagnostic only. Its disposable output is outside the
research artifact tree and is not P1a evidence. The full cell is completed
only by non-empty final `greeks.json` and `latency.json`; host process names,
partial sidecars, or terminal text do not establish completion.

## Before execution

1. Record source commit, binary SHA-256, config SHA-256, disk headroom, and
   command line in the final verdict.
2. Confirm output absence and that the config uses exactly the P1a economics
   committed in the preregistration.
3. Rebuild `multivenue`, `mvanalyze`, and `prunegate` from source. The only
   code used is already present in the P0 evidence implementation; P1a adds no
   simulator or analyzer behavior.

## Required extraction before any prune consideration

All raw logs and V2 sidecars remain retained. Extract the following into the
cell artifact directory:

1. `observationreceipts` and `fundingcarry` independent audits;
2. `evidenceartifacthash`, `streamhash`, terminal `conservation`, and generic
   `derivatives`/funding semantics;
3. a compact P1a decision histogram grouped by venue, rate, net carry, and
   `action_or_defer_reason`; and
4. exact submitted/accepted/filled/cancelled/orphan outcomes by venue.

The generic prune gate is advisory only for this new V2 contract. It must not
delete a raw P1a artifact. A valid no-action result is still scientific
evidence and remains retained.

## Interpretation fence

If the declared 24-bps fixed hurdle prevents all first-leg submissions, P1a
ends as **NOT EXERCISED**. Do not change fees, cost terms, funding rate,
population, clocks, or horizon while treating the output as the same test.
The result instead becomes a design constraint for a later, separately
registered V2-5 representation or participant-economics slice.
