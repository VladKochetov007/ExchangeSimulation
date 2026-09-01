# V2-R2-SV1 review-remediation checkpoint — f1a2749

Date: 2026-09-01  
Candidate: `V2-R2-SV1-CDF-LIQUIDITY`  
Scientific branch: `feature/r2-cdf-survival-successor`  
Predecessor: R2, retained unchanged as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**

This is an append-only successor record. It does not rewrite the predecessor
result or reinterpret retained historical experiments.

## Exact inputs and boundary

The exact scientific tree tested at this checkpoint is:

```
f1a2749c6cb7ab632252df913be9967a8947f44c
```

The asynchronous performance branch was fetched at the checkpoint. It remains
at `c4434ad`; there are no newer commits. No performance-branch code was
merged. Historical JSON evidence and hashes remain untouched.

No registered development cell and no holdout seed were run, inspected, or
partially consumed. The next permitted scientific action is still a
development-only activation probe after fresh independent review.

## Review-remediation changes

This checkpoint closes the concrete blockers reported in the preceding
independent Sol-xhigh review:

* the simulator can materialize its normalized effective configuration before
  launch; activation provenance hashes that exact artifact, and the registered
  long-run runner rejects a config that is not already normalized;
* CDF activation now requires a finite target-gap quote, a fill that changes
  inventory, and a later actionable decision that changes side or quantity;
  the actor remains limited by signed position, gross inventory, quote cash,
  and quote quantity caps;
* the receipt decoder retains and validates side, order type, time-in-force,
  and post-only fields for both gateway action and decision records; submit
  evidence must reconcile those fields, the order identity, and the delayed
  local frontier in both directions;
* receipt terminal time must equal the provenance simulation end; the fixed
  evidence manifest includes the market-data manifest and all four receipt
  ledgers when that contract is active;
* paired treatment/control provenance includes the exact simulator binary hash,
  source revision, and registered `linux/amd64/v1` target; the analyzer source
  revision must match both paired runs;
* comparison canonicalization removes only the CDF roster entry and preserves
  unrelated market-data receipt roles;
* the shared V2-0 receipt auditor treats the newly defined post-only flag as a
  validated field while still rejecting invalid flag values and nonzero
  reserved bytes.

## Verification

Focused packages passed after the remediation:

```
go test ./analysis ./evstream ./types ./exchange ./simulation -count=1
go test ./simulations/multivenue -count=1
```

The multivenue package completed in 192.511 seconds. Shell syntax and
`git diff --check` passed before commit.

The clean repository gate at `f1a2749` passed:

```
GOMAXPROCS=4 make test
GOMAXPROCS=4 go vet ./...
```

`make test` passed every Go package, both integrated long-run contract suites,
and both archive suites. The targeted race matrix also passed across the
changed analysis, receipt, simulation, and multivenue evidence paths. Fresh
process binary-evidence determinism and evidence-neutrality checks are part of
the passing multivenue suite.

The post-only regression first exposed a shared-auditor incompatibility: the
CDF-specific reader accepted the field while the general receipt auditor still
counted it as reserved. The common auditor was corrected and now has explicit
acceptance and invalid-encoding tests. This preserves compatibility for old
zero-valued records and makes the new field fail closed when malformed.

## Status and next gate

Commit `f1a2749` is pushed. The exact tree is awaiting one fresh independent
Sol-xhigh review of the complete successor candidate. That review must decide
whether the normalized-config, finite-inventory activation state machine,
gateway identity, terminal/retention, and paired binary-provenance contracts
are sufficient for a narrow development-only probe.

Review acceptance would authorize only clean Go 1.27 `linux/amd64/v1` binary
construction and the smallest development activation probe. It would not
authorize `dev-607`, any other registered development cell, freeze, or any
holdout. The probe must remain external to the repository and its audit must be
accepted before a larger development campaign is considered.

## Independent Sol-xhigh rejection — f1a2749

Ampere performed the fresh independent review of exact `f1a2749` and rejected
the narrow activation gate. The review accepted the finite economics,
delayed-local-data boundary, gateway order identity, effective configuration,
binary pairing, terminal anchoring, historical R2 isolation, and mechanical
test gates. It identified three concrete fail-closed blockers:

* inventory response was counted whenever a post-fill quote differed from the
  previous quote, so a market-only target change could be misclassified as
  inventory responsiveness; the registered elasticity/reference policy was
  not independently reconstructed;
* schedule/receipt/decision event ordinals were checked for uniqueness and
  continuity but not replayed in causal order, allowing a receipt to appear
  before its schedule if the ordinal set remained contiguous;
* the integrated evidence manifest added receipt files only when the receipt
  manifest already existed, so a configured receipt run with all five receipt
  artifacts missing could pass the retention layer.

No cells or holdouts were run by the reviewer. The predecessor R2 result stays
archived as the negative control. The remediation is to add a same-observation
no-fill counterfactual and target-policy reconstruction, replay the shared
event ordinal stream, and make configured receipt retention unconditional.

## Remediation checkpoint — 13ad8fd

The exact semantic remediation is now pushed as
`13ad8fd38e6355969953b497b407f6ad18d78e81`:

* the independent CDF extractor reads the registered reference price, base
  holding, and elasticity; it recomputes the target-position equation for each
  actionable quote;
* after a fill, the activation counterfactual uses the prior actionable
  position and the current target under the same observation. It counts an
  inventory response only when the actual side or quantity differs from that
  no-fill quote, so a market-only target change is not sufficient;
* the receipt auditor replays the shared schedule/receipt/decision event stream
  in global ordinal order and requires schedule before receipt and receipt
  before the decision frontier that cites it;
* when `evstream_v3` and `record_market_data_receipts` are enabled, evidence
  manifest construction and verification require all five market-data receipt
  artifacts by configuration, regardless of whether the manifest was already
  present. Empty binary ledgers remain representable as zero-record artifacts;
* temporary archive-contract fixtures include the configured receipt artifacts
  and explicitly verify that deleting one causes verification failure.

Focused analysis, receipt, and simulation suites pass. The clean
`GOMAXPROCS=4 make test` gate passes all packages, integrated contracts, R2
contracts, and archive tests; `go vet ./...` and the targeted race matrix pass
as well. The causal replay, market-only counterfactual, and missing-ledger
tests pass. This checkpoint remains below activation authorization pending one
fresh independent Sol-xhigh review of the exact tree.

## Independent Sol-xhigh rejection — f20a0fe

Volta performed the next fresh review of exact
`f20a0fe4442d3e27c3727df7ee02431349b4d846` and rejected the narrow activation
probe on two fail-closed scientific-audit blockers. No development cell or
holdout was run.

First, the CDF extractor checked the target-position equation using the
decision-emitted reference price, but the CDF roster did not register the
reference half-life and the extractor did not reconstruct the supplier's
reference update process. A coordinated mutation of the reference, target,
and quote could therefore evade the intended policy check.

Second, the no-fill counterfactual was evaluated after accepting any positive
quote quantity up to the policy maximum. An underquoted actual response could
therefore be counted as inventory-responsive even though it was not a valid
supplier action under the registered finite-inventory policy.

The review accepted the prior effective-configuration, delayed-local-data,
gateway identity, terminal/retention, paired provenance, and historical-R2
isolation controls. The remediation below addresses only the two reported
audit gaps and does not alter supplier economics.

## Remediation checkpoint — reference and exact-quote reconstruction

The extractor now records and requires the CDF supplier's registered reference
price, reference half-life, base holding, and elasticity. For every decision it
reconstructs the reference state from the initial reference and the actor's
positive two-sided local midpoint using the same exponential half-life update;
the emitted reference must equal that independently reconstructed state. A
positive emitted mark must also be the usable local midpoint and may occur only
on an actor action path that can legally consume such an observation.

Actionable quote quantities are now exact rather than merely bounded above by
the policy result. The reconstruction applies the finite gross-inventory and
maximum-quote caps, then applies the same quote-cash affordability rule and
maker-fee arithmetic used by the actor. Invalid or underquoted actions are not
eligible to satisfy the post-fill inventory-response criterion.

New regressions cover coordinated reference/target/quote mutation, missing
reference-update configuration, underquoted inventory response, and a valid
market-only change whose capped quote remains identical to the no-fill
counterfactual. The focused analysis/receipt/simulation gate will be rerun on
the resulting semantic commit, followed by the clean full gate and one fresh
independent Sol-xhigh review. No cell or holdout is authorized by this
checkpoint.

## Fresh Sol-xhigh acceptance — e35d891

Volta completed a fresh independent review of exact
`e35d89103107165dc5700876dda49fa0e1462d1b`. The verdict is **ACCEPT for the
smallest development-only activation probe**. The reviewer found no reachable
fail-closed blocker in the finite CDF economics, effective configuration and
paired provenance, delayed receipt/gateway identity, independently replayed
reference EMA, exact inventory/quote-cash sizing, no-fill counterfactual,
terminal retention, or historical-R2 isolation.

The acceptance conditions are deliberately narrow: rebuild clean Go 1.27
`linux/amd64/v1` simulator and analyzer binaries from this exact revision, run
only the registered five-minute seed-607 treatment/control activation probe,
retain complete artifacts, and obtain post-probe independent review. This does
not authorize `dev-607`, a 24-hour campaign, freeze, or any holdout. No cells
or holdouts were run during the review.

## Activation-probe analyzer contract finding — c542768

The accepted five-minute seed-607 treatment/control activation probe was run
from clean Go 1.27 `linux/amd64/v1` binaries built from `c542768`. Both arms
completed normally and all raw, binary-evidence, receipt, manifest, checkpoint,
and provenance inputs were retained under:

```
/home/vlad/external-scratch/v2-r2-sv1-activation-c5427680a1095c8e0e0bcff6a1d0f6228438cf15
```

The wrapper deliberately did not publish a comparison or activation-provenance
verdict because the extractor rejected the treatment on a legal actor state:
`wait` with reason `inventory_at_target` and no outstanding order/request. The
control audit was valid. The treatment otherwise showed 12 registered
suppliers, all 12 trading, all 12 with PnL change, all 12 inventory-responsive,
368 cancellations, and 36 checks. This is an analyzer contract bug activated
only by this new probe, not evidence that the CDF market failed or survived.

The failed comparison remains preserved at:

```
/home/vlad/external-scratch/v2-r2-sv1-activation-c5427680a1095c8e0e0bcff6a1d0f6228438cf15/cdf-liquidity-comparison.json.tmp-1818357
```

The remediation accepts the four registered no-action reasons
(`inventory_at_target`, `one_sided_or_locked_book`,
`limit_or_touch_unavailable`, and `quote_cash_limit`) only when order,
submission, and cancellation identities are empty. Positive-mark target-policy
validation also now applies to every legal action path, including no-action
wait/withdraw states, rather than only submission/rest actions. A focused
regression covers both valid no-action waits and rejection of stale order state.

Classification: **ANALYZER BUG, ACTIVATED IN THE NEW PROBE, NO HISTORICAL IMPACT**.
No registered development cell, 24-hour campaign, or holdout was consumed; no
historical result requires rescore or rerun. The corrected analyzer requires a
fresh exact-tree review and a new activation-probe run before any broader
development authorization.

## Additional independent rejection — e35d891 cash and boundary audit

Hume independently reviewed the immutable `e35d891` tree after the Volta
acceptance. The review did not inspect the later wait-state correction or
authorize any run. It rejected activation on two additional fail-closed
blockers.

First, BUY quote cash was still self-attested: the analyzer used emitted
`quote_cash_available` both as the affordability input and as the value being
checked. A coordinated inflated headroom value could therefore make an
otherwise unaffordable quote appear policy-exact. This is a real analyzer
contract weakness, not a simulator economic finding, and it has no historical
impact because no registered cell or holdout had run under SV1.

Second, the activation runner accepted an arbitrary absolute output root,
including the repository and descendants reached through symlinked parents,
despite the documented external-output boundary. This is a provenance and
retention-boundary defect; it does not alter the predecessor result or any
historical trajectory.

## Remediation checkpoint — independent quote cash and output boundary

The successor analyzer now reconstructs quote cash from the registered initial
quote balance and the ordered supplier evidence. A BUY submission creates a
fixed-point reservation; acceptance binds it to an order; rejection or
cancellation releases it; BUY fills consume reserved cash; SELL fills add net
quote proceeds; full fills release any remainder. Every emitted
`quote_cash_available` value must equal the independently reconstructed
available balance, and missing venue sequence or malformed cash evidence fails
closed. The cash ledger is separate from the decision-emitted affordability
claim, and a regression rejects an inflated post-fill headroom mutation.

The activation runner now resolves both the scientific repository and the
requested output root with `realpath`, rejecting the repository itself and all
resolved descendants, including paths through symlinked parents. A standalone
contract test covers the direct and symlink cases before any binary or output
creation is attempted.

This remediation changes analyzer/provenance validation only; it does not alter
CDF actor economics, R2 configuration, historical JSON evidence, or any
simulation trajectory. The exact successor tree requires a fresh independent
Sol-xhigh review before the activation probe is retried.

## Fresh exact-tree Sol-xhigh acceptance — 0e42809

Euler completed a fresh independent read-only review of exact
`0e42809af509aed9733bffcc629c340c71415c44`. The verdict is **ACCEPT for the
narrowly authorized five-minute seed-607 paired activation probe**. The review
found no blocker in the finite CDF economics, delayed local reference/target,
inventory/PnL/withdrawal behavior, independent quote-cash reconstruction,
venue-sequence ordering, fail-closed evidence handling, output-root boundary,
provenance, or historical-R2 isolation.

The conditions remain strict: rebuild clean Go 1.27 `linux/amd64/v1` binaries
from this exact revision; use a fresh external-scratch output root; run only
the registered five-minute seed-607 treatment/control activation probe; retain
all artifacts; and stop on any analyzer, provenance, evidence, sequence, or
boundary failure. This does not authorize `dev-607`, a 24-hour campaign,
freeze, or any holdout. No cell or holdout was run during the review.

The reviewer noted one nonblocking limitation for later gates: a conservative
rejection is possible when exchange cancellation and actor-observed fill
evidence cross timing domains. The retained identical seed-607 trajectory had
no such crossing, but this limitation must be corrected before broader or
stress testing if it activates.

## Independent post-probe acceptance — d85bfb1

Arendt independently reviewed the completed paired activation artifacts under
`/home/vlad/external-scratch/v2-r2-sv1-activation-d85bfb1bcac20a8c22b3d2629ecb5da83c17abd3`
and returned **ACCEPT for the activation probe only**. The review verified the
exact Go 1.27 `linux/amd64/v1` binary hashes, treatment/control configuration
identity, all 29 manifest entries per arm, evidence-only sidecar digests,
terminal evstream anchors, receipt completeness, and the absence of unlisted
arm files. No files were changed or deleted and no simulator or other cell was
run during the review.

The treatment contained 12 suppliers, 1,800 decisions, 111 supplier fills,
416 accepted quotes, 368 cancellations, 12/12 trading suppliers, 12/12 PnL-
changing suppliers, and 12 independently reconstructed inventory responses.
Quote-cash, balance, and PnL reconciliation residuals were zero; borrow usage
was zero. Bid/ask absence was `6/900` per side in both arms and was identified
as initial warm-up, not a treatment effect. The seven cancellation-pending
quotes were terminal censoring. The possible delayed fill/cancel timing
crossing did not activate.

This is an activation finding only. It does not establish survival improvement,
causal reduction in side absence, 24-hour viability, or stressed withdrawal
behavior. Any broader stress/development run must retain the fail-closed
boundary and correct the noted timing limitation if it activates. No holdout or
registered 24-hour development cell was authorized or consumed by this review.

## Independent 24-hour setup rejection — 48279d4 capacity/provenance boundary

Laplace independently reviewed the exact successor setup at
`48279d4237d8c32bb1b400ef22690fe9acb5e573` before any 24-hour capacity probe or
registered development cell. The economic and activation contracts were not
rejected. The review found four fail-closed setup defects: a lexical output-root
check could be bypassed through a symlinked ancestor; the capacity attestation
did not bind the registered config hash; the capacity measurement did not bind
`GOMAXPROCS=4`; and the config checker did not verify the recorded source and
registered-config hashes from its provenance manifest.

No cell or holdout was run. The exact `48279d4` tree remains a historical
review input and is not a launch candidate.

## Capacity/provenance remediation — 8784b46

Commit `8784b46d6d48ba78ceefd9a670f409c363ebe4a2` fixes only the four setup
boundary defects. The capacity probe now canonicalizes the requested external
root before creation and rejects the scientific repository and all resolved
descendants, including symlinked ancestors. It requires `GOMAXPROCS=4` and
records that value in both probe metadata and the retained capacity attestation.
The registered cell runner passes the normalized config hash and process width
to the fail-closed attestation verifier; the verifier rejects either mismatch.
The committed config provenance manifest and checker now bind every immutable
source config, activation roster, and registered successor config by SHA-256.
The shared contract test covers successful binding and both config/process
mismatch failures while retaining compatibility for historical callers that
use the unbound attestation form.

The clean exact-tree gates passed at `8784b46`: shell syntax, SV1 config
provenance validation, `git diff --check`, the integrated R2 contract suite,
`GOMAXPROCS=4 make test`, and `GOMAXPROCS=4 go vet ./...`. The commit is pushed
to the scientific successor branch. The performance feed was fetched at
`c4434ad32b2344e80a086d7e6ff9a8efcdaa1d7e`; it has no newer commits and no
performance code was imported.

This remediation does not authorize a capacity probe, a registered 24-hour
cell, freeze, or a holdout. The next gate is one fresh independent Sol-xhigh
review of the exact `8784b46` tree. Only an acceptance may authorize the clean
Go 1.27 build and one retained external capacity probe.
