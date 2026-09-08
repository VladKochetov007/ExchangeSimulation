# V2-R2-SV1C strict-risk successor preregistration

Date: 2026-09-08  
Candidate: `V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK`  
Predecessors: closed R2 and the rejected/blocked `V2-R2-SV1B` candidate  
Stage: development-only; no freeze or holdout authorization
Source baseline before this amendment: `179b298`

## Scientific boundary

The original R2 candidate remains closed as **NON-VIABLE AT THE 24H
MARKET-SURVIVAL GATE**. `SV1B` remains an archived successor checkpoint, not
accepted evidence: its finite CDF roster and R2 calendar are preserved, while
the independent review findings on cross-margin action, borrowing admission,
static CDF collateral, and borrowed-spot enforcement block promotion.

`SV1C` is a separately named semantic successor. It preserves the R2 calendar,
the eight historical ABC/USD suppliers, the finite delayed-local CDF supplier
contract, and the existing evidence representation. It does not rewrite,
rescore, or rescue any predecessor trajectory. Historical R2 configurations
retain their prior borrowing and collateral defaults; this amendment applies
only when the new strict-risk contract is explicitly registered.

## Mechanism hypothesis

Finite, economically motivated CDF/USD supply and demand can reduce persistent
one-sided collapse often enough for the full ecology to remain valuatable. A
qualifying supplier must trade with finite capital and inventory, bear explicit
mark-to-market PnL and loss risk, alter its quote with local inventory and
delayed observations, and be able to reduce or withdraw liquidity. The roster
is not a survival callback and does not encode a target price, spread, volume,
valuation availability, or market-survival outcome.

## Strict risk amendment

The strict successor registers a fail-closed debt boundary until a separate,
reviewed coherent collateral/liquidation policy is injected:

* spot and perpetual auto-borrow are explicitly disabled;
* cross-asset collateral marks are explicitly disabled, so a CDF listing or
  bootstrap reference cannot authorize CDF-backed USD credit;
* CDF supplier accounts start with zero debt and have no replenishment path;
* any future borrowing-enabled successor must provide an injected policy that
  values only declared unencumbered collateral, applies collateral-asset
  haircuts separately from liability limits, uses freshness-bounded
  provenance-bearing marks, includes existing debt and derivative maintenance,
  excludes proposed-loan proceeds, and has recurring spot-debt enforcement;
* missing marks, missing policy entries, unknown assets, arithmetic overflow,
  and incomplete enforcement are risk-unknown failures, never zero exposure or
  implicit capacity.

This is an explicit change in the registered debt/lending mechanism, not a
mechanical configuration cleanup. It is deliberately conservative: SV1C tests
the CDF-liquidity hypothesis without attributing a survival effect to an
unreviewed credit model. A later borrowing-enabled candidate requires its own
preregistration, regressions, and independent review.

The liquidation contract also requires one coherent mark epoch for a
cross-margin account. The complete same-quote portfolio is planned and
finalized at account scope; partial fills retain residual exposure and do not
recognize a terminal deficit while the account still has an open position.
Public or injected callers that cannot provide a coherent mark set must fail
closed before cancellation, matching, repayment, or insurance mutation.

## Activation criteria

The development activation probe is valid only if treatment evidence shows all
of the following for every configured supplier:

1. delayed local observations are delivered and independently bound to the
   decision frontier;
2. at least one ordinary order is accepted and one fill changes inventory;
3. finite balances, positions, fees, and marked PnL change consistently;
4. inventory changes alter target side or quantity;
5. at least one quote is cancelled, repriced, or withdrawn for a local
   observation, inventory, risk, or unavailable-side reason;
6. no borrowing, capital/inventory replenishment, hidden external oracle, or
   forced two-sided quote obligation occurs.

The strict-risk audit must also show zero supplier debt, no static CDF credit
path, complete mark-epoch/account-scope risk evidence, and valid terminal
population accounting. A short probe may establish mechanism activation only;
it cannot establish 24-hour market survival or a causal treatment effect.

## Anti-cheating and concentration criteria

Reject SV1C if survival is produced primarily by mechanically guaranteed
liquidity, effectively unlimited capital, forced replenishment, an external
instantaneous price anchor, a guaranteed two-sided quote, or a hidden direct
simulator-state read. Reject if supplier limits do not bind as declared, debt
appears, the supplier loss budget can be reset without a new participant state,
or strict valuation still fails.

Retain per supplier and venue: CDF volume share, resting-depth share, inventory
and cash paths, PnL, loss-budget state, quote lifetime, submit/reprice/cancel/
withdraw counts, observation age, side availability, and periods in which
either book side would disappear without the supplier. The supplier class must
not dominate the measured CDF market under the existing concentration limits;
the numerical result remains subject to a separate qualitative independent
review.

## Kill criteria

The candidate closes as falsified, invalid, or unsupported if:

* the activation evidence is incomplete or malformed;
* treatment remains persistently one-sided or strictly unvaluatable;
* no preregistered treatment effect is identified across fresh development
  pairs;
* a supplier dominates volume/depth or effectively prescribes the price;
* any supplier requires unbounded capital or replenishment;
* market survival depends on a structural two-sided quoting obligation;
* strict-risk evidence is absent, stale, non-coherent, or fail-open.

A negative result is retained as a result. It does not authorize retuning the
roster or reopening the R2 predecessor.

## Development-only sequence

1. Commit this preregistration and the exact strict-risk/configuration
   contract; hash all registered inputs.
2. Run focused tests, full tests, vet, targeted race, determinism, and binary
   evidence contract tests.
3. Obtain one fresh independent Sol-xhigh review of the exact successor tree.
4. Build clean provenance-pinned Go 1.27 binaries and perform only the
   registered small activation probe on development seed 643.
5. Independently extract/review the probe before considering the registered
   24-hour development pairs 643/647/653 and seed-643 parity controls.
6. Evaluate mechanical correctness, supplier activation, strict valuation,
   lifecycle, concentration, and paired causal predicates before any freeze
   request.
7. Obtain an explicit freeze authorization before touching holdouts `619`,
   `631`, or `641`.

No step in this document authorizes a holdout run, a predecessor rerun, an
offline trajectory repair, or deletion of retained evidence.
