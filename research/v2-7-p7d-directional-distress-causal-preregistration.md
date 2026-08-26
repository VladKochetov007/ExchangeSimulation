# V2-7 P7d — finite-capital leveraged directional exposure

Status: **preregistered design; no P7d outcome or preflight world has been
inspected**.

P7a--P7c established that a fixed physical-liability hedge can enter and hold
an ordinary perpetual position, but its participant-specific margin path is
not exercised even at the registered two-day horizon.  That actor is
economically hedged outside the venue, so its exchange position is not a clean
test of unhedged directional distress.  P7d introduces a separate local
directional mandate with finite own capital and an explicit margin-borrowing
policy.  It does not alter P7a--P7c or reinterpret their outcomes.

This is a mechanism-identification screen, not a crisis-realism, price-path,
funding, basis, profitability, or market-stability experiment.

## Local economic mechanism

The treatment participant represents a leveraged directional desk with a
declared one-sided thesis.  It receives only its delayed local `ABC-PERP`
public-book feed, submits ordinary IOC orders at the locally observed
executable touch, and works a finite target position.  It has no physical
offset, global index, mark oracle, opponent state, or forced fill.  The target
is held after entry; no synthetic close or balance reset is allowed.

The desk is permitted to auto-borrow quote collateral only when its finite
perpetual wallet cannot reserve the exchange's ordinary initial-margin amount.
Borrowing is recorded as an ordinary margin loan with the existing collateral,
interest, cap, and repayment rules.  The declared own collateral is chosen
before outcomes; the borrowed amount is an endogenous consequence of the
target and the exchange margin formula.

The control carries the same policy/configuration and borrowing permission but
the participant is disabled.  Long and short are separate registered
orientations; neither may be selected or discarded after observing outcomes.

## Registered cells

| cell | participant | target perpetual position | purpose |
|---|---|---:|---|
| C | disabled directional mandate | 0 | matched roster/evidence control |
| L | enabled directional mandate | +2,000,000,000 raw ABC (long 20 ABC) | long-risk treatment |
| S | enabled directional mandate | -2,000,000,000 raw ABC (short 20 ABC) | short-risk treatment |

Every cell uses the same three venues, population, feeds, venue rules,
latencies, clocks, P3e exit settings, and full evidence contract.  Only the
declared directional enablement/target differs between C, L, and S.  Both
orientations are scored separately; a favorable orientation is not a reason to
drop the other one.

Development seeds are **431 and 433**, reserved before any P7d run.  Untouched
holdouts are **439, 443, and 449**, and are not consumed unless the registered
development promotion rule is met.

## Evidence and activation contract

The existing participant-local receipt contract remains mandatory.  Before
scoring, the fail-closed extractor must verify:

1. final `greeks.json` and `latency.json` completion sentinels and successful
   run exit;
2. complete persisted evidence and runtime/offline evidence-digest equality;
3. every directional decision has a valid local receipt frontier and no future
   observation;
4. ordinary IOC request, acceptance/rejection, fill, cancellation, and
   position transitions join one-to-one;
5. target entry is reached only by admitted/fill-linked venue orders;
6. all quote-margin borrow records identify this participant and the declared
   `auto_perp` reason;
7. independent mark/collateral/position replay has no arithmetic or stale-mark
   errors;
8. participant-specific margin checks, calls, forced closes, deficits,
   insurance movements, and bankruptcy are reconstructed from contemporaneous
   evidence; generic liquidations for other accounts never substitute for this
   path;
9. conservation and terminal position/account state pass without synthetic
   resets.

Activation is separate from risk.  Each enabled orientation must produce
decisions, ordinary admitted/fill-linked IOC orders, and the declared target
position (or a documented ordinary-liquidity residual).  A zero-fill or
incomplete target is an execution failure, not evidence of a risk null.

## Numeric addendum (frozen before outcomes)

| choice | value | class | ex-ante rationale |
|---|---:|---|---|
| venue roster and non-P7 fields | inherited `v005-stress-perp` projection | A | preserve the audited V2 environment |
| cross-asset graph | false | A | isolate quote-margin/perpetual distress and avoid unrelated CDF marks |
| target magnitude | 2,000,000,000 raw ABC (20 ABC) | C | twice the P7c 10-ABC hedge, large enough that 5.5B raw own margin is only a finite 5.5% notional buffer while retaining ordinary venue scale |
| target orientation | +2e9 and -2e9 | C | register both signs; no post-outcome orientation selection |
| maximum IOC child | 500,000,000 raw ABC (5 ABC) | C | four bounded ordinary children; preserves partial-fill/leg risk |
| own perp margin | 5,500,000,000 raw USD ($55,000) | A | inherit P7c near-initial-margin own capital; not reselected from P7 outcomes |
| spot operating wallet | 50,000,000 raw USD | A | inherit fixed-liability participant wallet; not a hidden perp reserve |
| auto-perp borrow | enabled for this declared policy | C | explicit finite-margin lending mechanism; no free credit |
| maximum quote borrow | 5,500,000,000 raw USD | C | caps borrowed amount at own collateral; sufficient for the declared 10B raw initial requirement and prevents unlimited leverage |
| tick, fees, feed delay, decision clock | inherited P7c values | A | preserve venue and information contracts |
| horizon | 4 simulated hours | C | enough for entry plus repeated mark/risk sweeps while limiting development evidence volume |
| preflight | at most 15 simulated minutes | C | mechanics-only; cannot score participant distress |
| development seeds | 431, 433 | C | fresh odd seeds not used by prior P4--P7 screens |
| holdouts | 439, 443, 449 | C | fresh seeds reserved before outcomes |

At the inherited approximately $50,000 ABC reference, 20 ABC has about
$1,000,000 notional and requires about $100,000 (10,000,000,000 raw) initial
margin.  The participant owns $55,000 of perp collateral and may borrow only
the endogenous shortfall, giving an own-equity buffer of roughly $55,000
against the 5% maintenance requirement before fees and price movement.  The
calculation motivates a reachable-risk screen; it does not guarantee a
liquidation and is not fitted to a realized path.

## Registered endpoints and classification

Report long and short independently:

* decision/receipt activation and target progress;
* admitted, filled, cancelled and externally matched IOC quantities;
* own perp balance, borrow principal/interest and reserved margin;
* mark/equity/notional/maintenance/warning trajectories;
* expected margin breaches and independently linked margin-call events;
* participant-specific forced-close quantity and residual position;
* deficit, insurance-fund transfer, and bankruptcy, each separately;
* funding before and during residual exposure;
* conservation and exactly-one position closure where a forced close occurs.

The primary development risk predicate is an independently reconstructed
participant-specific maintenance breach with a corresponding venue risk event
by the four-hour endpoint, conditional on valid target activation.  A clean
forced close is supporting evidence, not a requirement for recognizing a
margin breach.  The active orientation is `NOT EXERCISED` when both seeds have
valid activation but zero participant risk events; it is `FALSIFIED AT
ACTIVATION` or `FALSIFIED AT EXECUTION` when the participant/evidence contract
fails.  Opposite valid signs are `MIXED`.  Deficit, insurance, and bankruptcy
remain separate zero/nonzero endpoints.

No P7d result may claim that liquidation is realistic for the full ecology,
that funding anchors basis, that the market price is realistic, or that the
directional desk is profitable.  Holdouts are authorized only if at least one
orientation has valid activation in both development seeds and at least one
clean participant-specific risk event; otherwise retain development only and
design the next exposure source/horizon rather than tuning this one.

## Mutations and stop rules

The analyzer must independently catch: disabled control submission; target
sign reversal; dropped/duplicated directional decision or fill; future/delayed/
duplicated/reordered feed receipt; wrong local touch; missing or excess borrow;
stale collateral/mark; dropped/duplicated risk event; synthetic balance reset;
and post-liquidation reopening.  A zero trigger is `NOT TESTED/NOT EXERCISED`,
never a pass.  Any evidence or deterministic-hash failure stops scoring and
preserves the raw world.
