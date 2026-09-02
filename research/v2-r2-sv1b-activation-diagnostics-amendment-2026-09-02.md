# V2-R2-SV1B activation and diagnostic amendment

Status: preregistered successor-candidate contract; no development or holdout
cell is authorized by this document.

## Scientific boundary

The calendar-based R2 candidate remains archived as the negative predecessor:
`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`. SV1B is a separately named
successor candidate. It does not revise the predecessor result and it does not
turn an activation probe into a 24-hour market-survival claim.

SV1B retains the accepted R2 calendar/lifecycle semantics and the eight
historical ABC/USD elastic suppliers unchanged. Its only new economic class is
the separately configurable, finite CDF/USD elastic-liquidity roster. The
roster is absent from all historical R2 configurations and absent from paired
SV1B controls.

## Mechanism hypothesis

Finite, economically motivated CDF/USD supply and demand can prevent persistent
one-sided CDF collapse often enough for the full ecology to remain valuatable.
The proposed mechanism is bounded inventory-sensitive quoting by independently
funded participants. It does not prescribe a price, spread, volume, valuation
availability, or survival outcome.

## Activation contract

The development activation pair is `activation-643` versus
`activation-643-control`. The existing typed CDF audit must establish, for the
configured treatment roster:

* every configured supplier has local decision evidence and at least one fill;
* each supplier has nonzero PnL change and an inventory-responsive decision;
* the roster has finite initial balances, finite inventory/quote limits, zero
  borrow activity, and valid marked-risk state where a loss budget is configured;
* at least one cancellation or withdrawal is evidenced, including the
  registered withdrawal-without-replacement gate;
* delayed local observation receipts and action links reconstruct every
  decision; and
* the aggregate and per-venue supplier-removal diagnostic is valid.

The activation audit is an evidence boundary, not a requirement that the
supplier rescue the market. A typed terminal economic failure is a valid
negative diagnostic endpoint only when its exact stage, timestamp, venue,
symbol, cause, evidence seal, and partial-report status satisfy the terminal
contract. It supplies no terminal valuation or survival observation.

## Anti-cheating criteria

Reject the successor if CDF activity relies on any of the following:

* infinite or replenished capital, unbounded inventory, or unregistered
  borrowing;
* a hidden instantaneous global price or simulator-state oracle;
* a guaranteed two-sided quote obligation or mechanically forced replenishment;
* direct spread, volume, survival, or valuation-availability targeting; or
* an actor decision that cannot be reconstructed from its delayed local market
  observation and bounded state.

The live actor enforces a positive minimum executable quantity. When a target
gap is below that threshold it withdraws or waits and does not submit a
sub-minimum order. Its reference, target, inventory, cash, marked equity,
loss-limit, observation age, and quote lifecycle are independently checked by
the typed audit.

## Concentration and removal diagnostics

The CDF audit records supplier volume share, maximum and time-weighted resting
depth share, per-supplier inventory/PnL, quote lifetime, withdrawal frequency,
and raw and minimum-qualified side absence. For every public CDF/USD book
snapshot it also computes a structural removal projection:

1. identify the currently resting CDF orders reconstructed from accepted,
   cancelled, and filled order evidence;
2. sum those orders separately on the bid and ask sides;
3. subtract each side from the corresponding aggregate public displayed depth;
4. classify the residual book as side-available, side-absent, or below the
   configured minimum executable depth.

This is a retained-snapshot structural counterfactual. It does not replay
matching, alter the historical path, or claim that prices would be unchanged
after removing the supplier. Any supplier quantity exceeding the corresponding
aggregate side fails the diagnostic closed.

The paired treatment/control comparison is a total-population intervention:
the control removes the CDF roster, rather than isolating one supplier while
holding other CDF suppliers fixed. Results will be described with that
estimand and will not be presented as a single-agent marginal effect.

## Kill criteria

Kill the SV1B successor and retain the evidence as a negative or invalid
candidate if:

* the CDF book remains persistently one-sided or strict valuation still fails;
* a supplier dominates volume or resting depth so that prices/liquidity are
  effectively prescribed;
* the supplier requires unbounded replenishment or borrowing;
* survival occurs only because the participant is structurally forced to quote
  both sides; or
* any typed evidence, removal projection, conservation, or provenance check
  fails.

No kill criterion is a demand that broad realism metrics become green.

## Capacity boundary

The binary-evidence capacity probe is a dedicated 24-hour calibration run on
the development-only `activation-643` configuration. It is not a registered
treatment trajectory and its outcome is not scored. The attestation records
both the measured calibration configuration hash and the hash of the primary
SV1B treatment configuration that it authorizes for launch. The launch runner
will reject an attestation that is not explicitly marked calibration-only and
does not bind both identities.

## Promotion sequence

After this amendment and its provenance hashes are committed:

1. run the activation pair only on development seed 643;
2. inspect typed terminal status, activation, removal, concentration, and
   conservation evidence;
3. obtain independent review of the exact candidate tree and activation result;
4. only after acceptance run the dedicated binary capacity calibration;
5. only after capacity and a clean pinned build pass consider registered SV1B
   development cells; and
6. do not consume holdout seeds.

The registered SV1B cells remain development-only until a separate explicit
freeze authorization.
