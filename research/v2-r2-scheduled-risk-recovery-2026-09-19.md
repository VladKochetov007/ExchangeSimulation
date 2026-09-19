# R2 scheduled-risk recovery amendment — 2026-09-19

Status: successor candidate implemented; fresh independent review and a new
development-only activation probe are still required. No development cell,
freeze, or holdout was run from this amendment.

## Boundary and predecessor

The predecessor is C5, commit `5015fd00a3720c3bd9312d93ae5635fdbf8647a4`,
with tree `9409efe743d02b0dae2d4aa7f65de61c1ad1ca26`. Its registered seed-659
activation probe is retained as `INCOMPLETE_ARM` for all treatment, mode-off,
and no-roster arms. The common error was strict option risk valuation at a
deterministic post-derivative-mark phase where the ABC/USD book was temporarily
empty. The exchange cleared the option's stale mark as required. The later
same-timestamp actor quote did not retroactively repair that earlier valuation
boundary.

That result remains a producer/risk-telemetry gate, not a CDF-liquidity result.
It has no directional score and does not invalidate historical experiments.

## Independent adjudication

The fresh Luna independent review inspected the exact C5 source and retained
seed-659 diagnostic evidence. Its qualified verdict was:

* the underlying strict option-mark failure is correct and must remain
  fail-closed;
* the arm-level permanent `INCOMPLETE_ARM` is caused by coupling a transient
  scheduled telemetry capture failure to the venue-wide fatal `riskErr`;
* the admissible correction is to defer only a scheduled capture caused by a
  currently unavailable price, retry at the next ordinary scheduled boundary,
  and retain the deferral/retry/outcome audit trail.

The review explicitly rejected stale option-mark reuse, cached option
valuation, phase reordering, fixture/config changes, and any weakening of the
strict pre-expiry or terminal valuation gates. The previous unavailable-review
attempts and the accepted C5 review remain historical records; this amendment
does not convert service failures into verdicts.

## Implemented contract

Commit `88ea00d` implements the minimal correction:

1. Scheduled post-derivative-mark capture tracks the last attempt separately
   from the last successful snapshot. A transient `ErrNoPrice` is deferred and
   retried at the next ordinary Greek boundary. Near-expiry cadence retains
   the existing higher-frequency attempt behavior.
2. Any other capture error, including an out-of-domain mark, still sets the
   fatal venue error immediately. `pre_expiry` and `terminal_post_mark` use
   the unchanged strict `captureVenueRisk` path and therefore still fail closed.
3. The exchange's stale option mark is still cleared when its declared
   underlying is unavailable. No mark is reused, synthesized, or read from an
   actor cache, and deterministic phase ordering is unchanged.
4. Every scheduled attempt records a `risk_capture_diagnostic` with venue,
   phase, simulated timestamp, attempt number, `initial`/`retry` kind,
   `captured`/`deferred`/`failed` outcome, retry boundary, and error text when
   present. It is retained in `greeks.json` schema 7 and, for evidence-contract
   v2, in the canonical ordered binary evidence stream. Legacy binary contract
   v1 does not emit the evidence-only event and does not consume a venue
   sequence number.

The diagnostic is telemetry only. It does not alter orders, marks, positions,
balances, matching, actor observations, exchange phase order, or registered
seed/config/roster/warm-up behavior.

## Regression and gate evidence

The focused tests cover:

* unavailable option mark -> no fatal scheduled error, no fabricated timeline
  row, explicit deferral, no early retry, then a successful audited retry;
* present but out-of-domain option mark -> immediate fatal failure and an
  explicit failed diagnostic;
* binary evidence retention of both diagnostic events with zero unencodable
  payloads;
* production binary rendering without a per-venue sequence gap when the
  legacy binary contract excludes evidence-only rows.

After the routing edge was corrected, the full focused gate passed:

    go test ./simulations/multivenue ./analysis -count=1
    simulations/multivenue: 228.333s
    analysis: 3.873s

The first full attempt exposed the legacy-contract sequence edge; it was fixed
before `88ea00d` was committed and the filtered renderer/recovery tests passed.

## Scientific impact and next gate

The correction was not present in the C5 seed-659 run, so that retained run is
not repaired or rescored offline. It remains an incomplete activation probe.
No prior retained experiment could have activated this new producer-side
diagnostic behavior, and no holdout was consumed. A fresh activation run is
required because the simulator semantics and risk-evidence output contract
changed.

Before that run, designate one immutable candidate including this amendment
record, obtain one fresh independent Luna/Terra/Sol review of the exact tree,
then rebuild and verify a clean pinned binary. Only if that review and the
existing capacity/provenance gates pass may the registered seed-659 activation
probe be repeated. A successful probe still does not authorize development
cells or holdouts; the existing SV1D protocol controls those boundaries.
