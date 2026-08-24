# V2-5 P0 activation smoke plan

Status: **preregistered before the first market-backed V2-5 P0 smoke.**

This plan operationalizes the evidence gate in
[`v2-5-funding-carry-p0-preregistration.md`](v2-5-funding-carry-p0-preregistration.md).
It is not a funding-on/off comparison and cannot support a conclusion about
perpetual basis anchoring, profitability, or market realism.

## Immutable smoke cell

| field | value |
|---|---|
| config | `configs/v2-5-p0/activation-101.json` |
| seed | 101 |
| horizon | 5 simulated minutes |
| evidence | full raw logs; V2-0 receipts; decision/outcome evidence; execution checkpoints |
| actor feed/request delay | fixed 10 ms per declared funding-carry link |
| funding horizon | one received funding interval |
| named cost components | all explicitly present and zero for this activation-only cell |

The zero cost values are not a calibrated economic assumption. They avoid
turning a short infrastructure smoke into a post-hoc threshold search when the
legacy population's endogenous funding rate is only a few integer bps. A later
P1 comparison must preregister nonzero costs and its own population/control
before it runs.

## Required evidence checks

The cell passes only if `mvanalyze -metric fundingcarry` reports a valid audit
and all of these are nonzero where expected:

1. `funding_carry_decision` evaluations;
2. fresh delayed `MDFunding` receipt matches for funding-sensitive evaluations;
3. independently recomputed funding income and net carry;
4. at least one submitted leg **only if** a local premium and signed net carry
   satisfy the fixed policy; and
5. exact gateway and venue/actor outcome linkage for each submitted leg.

No submission is not a failed run by itself: it is an explicit activation
failure (`NOT IDENTIFIED`) unless the preserved decisions demonstrate that the
fixed prerequisites never occurred. The response is diagnosis, not changing
this config after seeing it.

The raw evidence remains retained until the analyzer, receipt audit, and
evidence-artifact digest all pass. Any later market P1 cell receives new,
separately committed configurations.
