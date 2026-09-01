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
