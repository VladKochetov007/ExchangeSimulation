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

## Fresh setup rejection — 6afe3e5

Kant independently reviewed exact `6afe3e57b8575f2d95a5549f1a7774f2e5405841`
after the preceding remediation. The source/config/roster/binary/process
provenance bindings and canonical external-root logic were accepted, but the
narrow capacity gate was **REJECTED** for two new fail-closed defects. The
probe expanded an unset `probe_root` under `set -u` before assigning it, so a
normal invocation terminated before its safety checks. In addition, the 4 GiB
minimum-free reserve was not enforced before launch, on failed/non-numeric
poll samples, or after the simulator exited; a nominal attestation could
therefore be emitted without proving the reserve remained available.

No registered cell or holdout was run. The exact `6afe3e5` tree remains a
review input, not a launch candidate.

## Capacity reserve remediation — 7763129

Commit `7763129ad2f7c37e969cf39192c8be1a8f3addce` removes the premature
`probe_root` expansion and moves canonical root validation ahead of output
creation. It identifies an existing filesystem ancestor and requires a
numeric free-space reading above the 4 GiB reserve before launch. Every active
poll now treats a missing or non-numeric `df` result as a hard abort and
terminates the simulator; crossing the reserve also aborts. A final numeric
measurement is mandatory and must remain above the reserve before a successful
attestation can be written. The probe records the initial and final readings,
reserve, source revision, binary/config hashes, evidence format, and
`GOMAXPROCS=4`; the SV1 runner binds all of those control values through the
shared verifier.

The shared contract test now exercises direct and symlinked repository-root
rejection, verifies the previous unbound-variable path is gone, and covers the
reserve binding and mismatch rejection. `bash -n`, SV1 config validation,
`git diff --check`, the clean `GOMAXPROCS=4 make test`, and
`GOMAXPROCS=4 go vet ./...` pass on `7763129`. The commit is pushed. No pinned
successor binary has been built and no capacity probe, registered cell,
freeze, or holdout is authorized until a fresh independent Sol-xhigh review
accepts this exact tree.

## Fresh setup rejection — 52d2f38

Fermat independently reviewed exact `52d2f38d4944b269e7bb834dd40bb50dc3fdec27`
and rejected the narrow rebuild/capacity gate. The prior root, provenance, and
minimum-free fixes were accepted. Four additional fail-closed issues remained:
the capacity measurement stopped before the retained evidence manifest and
attestation were written; the shared verifier did not bind the retained probe
root, evidence-manifest digest, or reserve samples; an error or wrapper signal
could leave the simulator running, and reserve-abort termination had no
escalation; and the capacity probe did not acquire the SV1 namespace lock, so
two probes could race toward the shared attestation. The reviewer also noted
that the existing fixture did not exercise those control-plane paths.

No rebuild, capacity probe, registered cell, freeze action, or holdout access
was authorized. No cell or holdout was run.

## Retained-capacity and supervision remediation — 6d53a85

Commit `6d53a85941c74ec5c08963431155069876dcd1af` closes those four setup
issues. The probe acquires the SV1 namespace lock before work, installs EXIT
and signal cleanup traps, supervises the simulator with TERM followed by
bounded escalation to KILL, and guards both output-size and free-space
measurements. It measures retained output and free space again after all
required evidence files and the evidence manifest are written, before building
the capacity claim. The temporary attestation is validated before publication
and the published attestation is validated again.

The shared verifier now binds SV1 attestations to a canonical retained probe
root, the `treatment-607` retained evidence set, its evidence-manifest SHA-256,
numeric initial/final free-space samples, and the 4 GiB reserve. Contract tests
cover retained-manifest deletion, direct/symlinked repository-root rejection,
lock contention, the former unbound-variable path, and required supervision /
postprocessing guards. The clean `GOMAXPROCS=4 make test`,
`GOMAXPROCS=4 go vet ./...`, shell syntax, SV1 config provenance, and diff
checks pass on `6d53a85`; the commit is pushed.

This remains a setup-only remediation. A fresh independent Sol-xhigh review of
the exact `6d53a85` tree is required before the pinned Go 1.27 build or any
capacity probe. No registered cell, freeze authorization, or holdout access is
authorized.

## Capacity-floor remediation — e421e1c

Commit `e421e1c` closes the remaining capacity-floor mismatch. The probe now
fails before publishing an attestation if retained-output free space is below
the measured `peak_output_bytes + safety_margin_bytes` floor, in addition to
the fixed 4 GiB reserve. The shared SV1 attestation verifier enforces the same
relationship and therefore cannot accept a claim whose final sample is merely
above the reserve while below the measured output-plus-margin requirement. The
contract fixture covers that distinction explicitly.

The exact-tree shell syntax, integrated R2 contract tests, SV1 configuration
provenance check, and `git diff --check` passed before the commit. A clean
`GOMAXPROCS=4 make test` followed by `GOMAXPROCS=4 go vet ./...` passed after
the commit (`GATE_STATUS=0`). The commit is pushed. The performance feed was
refreshed at `c4434ad32b2344e80a086d7e6ff9a8efcdaa1d7e`; there were no newer
commits and no performance changes were imported.

This is still a setup-only remediation. It does not authorize the pinned
binary build, the external capacity probe, a registered cell, freeze
authorization, or holdout access. The next gate is a fresh independent
Sol-xhigh review of exact `e421e1c`.

## SV1 extraction, parity, and scoring checkpoint — 2630fbb

Commit `2630fbb1dc49eff03b33317085fa2cb1f89e34ff` adds the SV1 adapters around
the shared fail-closed extractor and verifier, the registered seed-607 parity
contract, and the precommitted six-cell development scorer. The parity schema
now distinguishes exact treatment G4/G8 equality from normalized execution
equality for the full/no-log control pair. The scorer records the fixed
post-warm-up side-availability endpoint in
`v2-r2-sv1-24h-survival-scoring-amendment-2026-09-01.md`; this is a measurement
contract only and does not alter economics or instrument populations.

No simulator cell, capacity probe, freeze authorization, or holdout seed was
run or read. The development scorer refuses existing holdout directories and
uses the reserved status `RESERVED_AND_NOT_READ_BY_DEVELOPMENT_SCORER`.

Verification on the exact pushed tree:

- `env GOMAXPROCS=4 make test`: PASS; all Go packages, integrated R2/SV1
  contract tests, and archive tests passed;
- `env GOMAXPROCS=4 go vet ./...`: PASS;
- focused `go test ./evstream ./types ./exchange ./simulations/multivenue
  -count=1`: PASS in 194.776 seconds, including fresh-process evidence
  determinism and neutrality;
- narrow `go test -race ./simulations/multivenue -run
  "Test(V20Evidence|BinaryEvidence)" -count=1`: PASS in 140.481 seconds;
- the broader targeted race matrix passed all non-multivenue packages but
  timed out after 600 seconds in the pre-existing
  `TestV23P2RebalanceEvidenceIsFreshProcessDeterministicAndNeutral` test. It
  produced no race report or OOM. This is recorded as a bounded test-suite
  limitation, not as a race pass.

The asynchronous performance branch was fetched again from the recorded
`c4434ad32b2344e80a086d7e6ff9a8efcdaa1d7e` point; no newer commits were
present and no performance code was imported. A fresh independent Sol-xhigh
review of exact `2630fbb` remains required before clean pinned binary
construction or any activation probe. The capacity probe and all registered
development/holdout cells remain unauthorized.

## Exact-tree review and pinned-build checkpoint — ca5f6e0

The fresh independent Sol-xhigh review of exact `ca5f6e0c8d014145f06f2ca209b35b7ddffc401a`
returned **ACCEPT WITH NARROWER CLAIM**. It found no source-level blocker to
the pinned build or capacity probe and confirmed that the tree is suitable only
for the preregistered development setup sequence. The review does not claim
24-hour survival, economic viability, holdout readiness, or a repository-wide
race-clean result. It specifically retains the broad-race timeout as a bounded
limitation: the unchanged V23 P2 fresh-process test timed out after 600 seconds
without a race diagnostic, while the focused evidence race passed.

The reviewer confirmed that the R2 calendar/lifecycle and risk hardening are
preserved, the finite CDF supplier uses delayed local observations without an
oracle or forced two-sided replenishment, and the `evstream_v3` plus
extractor/verifier/scorer/parity contracts remain fail-closed and holdout
isolated. It noted the narrower treatment logging-neutrality scope and the
remaining common-mode risk between supplier policy and its reconstruction
audit; neither was judged a blocker to the setup gate.

From that exact clean tree, the five required binaries were built in tmux with
Go 1.27.0, `GOAMD64=v1`, `CGO_ENABLED=0`, `GOMAXPROCS=4`,
`-trimpath`, and VCS metadata enabled. Each binary reported
`vcs.revision=ca5f6e0c8d014145f06f2ca209b35b7ddffc401a`,
`vcs.modified=false`, and the required Linux/amd64/v1 target. The resulting
SHA-256 values were:

| binary | SHA-256 |
|---|---|
| `multivenue` | `44bbe7eed125c9e4e5d07ef0dd0f361a0e0256e9322e4e0f5e51efa53997941d` |
| `cdf-liquidity-audit` | `196e225c5a0f1e90d53baf3a88cce5a21286211f287bdd03359431b9dc4dc802` |
| `mvanalyze` | `7837e5443dd4ab7742008c1753c167af8a79a0541c567d406301563d526507e9` |
| `evsrender` | `14b162bdda3c823b27ce003059ebfcdad47b2bcad212746d73e9ca5d957b8b98` |
| `prunegate` | `4b02aab0db85c0429c4de9237ecaa075bcd8d33c8b9bb059bece827f1a15d891` |

Because this record is a documentation-only commit after the reviewed tree,
these binaries are a recorded successful build checkpoint, not the final
launch artifacts. The source HEAD must be rebuilt after this record is
committed so the capacity attestation and every later runner provenance field
bind to the final documented revision. No capacity probe, development cell,
freeze action, or holdout access has occurred.

## Capacity probe result — dbbac47 / retained failure

The first full SV1 capacity measurement was launched only after the exact-tree
review and clean Go 1.27 rebuild. It used `treatment-607`, `evstream_v3`,
`GOMAXPROCS=4`, and the canonical external root
`/home/vlad/external-scratch/v2-r2-sv1-24h-capacity-dbbac47c0b8f3496425fdbcac9598d3dae7992e5`.
The simulator was terminated by the probe at 19:09 wall time with status 143
when final free space was `4,261,502,976` bytes, below the required
`4,294,967,296`-byte reserve. The retained partial output measured
`21,169,232,941` bytes; no capacity attestation was published and the run did
not produce a completed 24-hour cell.

The partial output remains untouched for audit and contains the probe metadata,
the partial `events.evs` stream, market-data streams, checkpoints, and
provenance files. It is a failed capacity measurement, not a scientific
outcome, and cannot authorize any SV1 cell. No simulator cell, freeze action,
or holdout was run or read. The failure is attributable to host capacity under
the current `evstream_v3` contract; it is not evidence against the CDF
mechanism.

After termination, only the regenerable Go build/test cache was cleaned,
recovering approximately 5.6 GiB. The incomplete derivative-proxy tree, all
historical R2/R5 evidence, and the failed capacity output were preserved.
The next capacity attempt must use a fresh root and a binary rebuilt from the
final documented source revision; the reserve and measured-output floor must
not be lowered.

## Retention-preserving capacity cleanup — c5d7f56

The host capacity stop was addressed without discarding historical evidence.
The explicitly stale R5 directory
`/home/vlad/v2-r5-preserved-dev607-adc4e03-stale-revision` was first archived
to
`/home/vlad/v2-r5-preserved-dev607-adc4e03-stale-revision.tar.zst`. The archive
passed `zstd -t`, normalized 51-member equality, and a full tar content
comparison against the still-present source. Its SHA-256 is
`b78524023b68dcab8f36b3d175d5fdb9ad46c86ff034d62a02892744a861e729`; the
checksum, member list, and comparison log are retained beside the archive.
Only then was the unpacked duplicate removed, recovering approximately 33 GiB.
The historical result remains recoverable from the verified archive. The
incomplete derivative-proxy tree, the failed SV1 capacity output, and all other
retained evidence were not touched.

The source tree remains clean at `c5d7f56` after this documentation update.
The next capacity attempt must use a fresh output root and a clean binary built
from that final documented revision; no capacity attestation has yet been
published and no development or holdout cell is authorized.

## Additional verified historical compactions — 43df7ce

To make the measured capacity floor attainable while retaining prior results,
three non-current unpacked trees were compacted only after archive checksum,
member-list, and full content-comparison checks:

| retained record | archive | SHA-256 | unpacked space recovered |
|---|---|---|---:|
| incomplete R5 derivative proxies | `/home/vlad/v2-r5-deriv-proxies-20260830.tar.zst` | `0f7bd00f4f98842a7f65c85908d60e3f7f3e9be35ffc0cec9944a06d861e5ebd` | ~14 GiB |
| interrupted R5 V5 world | `/home/vlad/v2-integrated-longrun-candidate-20260828-v5-interrupted-20260829.tar.zst` | `fe6a19afd5d83158d61163b0a0cb04b9091e57ce819265b436ff85c6557c1b32` | ~9.9 GiB |
| failed SV1 capacity probe | `/home/vlad/v2-r2-sv1-capacity-dbbac47-retained-failure.tar.zst` | `2504095a4e8bc82bbefca92337df27ed0d72e4855c34e8dcadb0b48403e56ee5` | ~21.2 GiB |

The source trees were removed only after their archives passed exact content
comparison; the archives, checksums, member manifests, and comparison logs are
retained. The SV1 capacity archive preserves the partial stream, provenance,
and explicit reserve-triggered failure, and remains a failed measurement—not
an attestation or scientific cell. No current evidence, calendar config,
source code, or holdout was changed. Free disk after compaction is
approximately 80 GiB, with the failed-capacity archive still retained.

## Reviewed retention execution — 41de87b

The fresh independent Sol-xhigh review of the exact clean setup tree
`41de87bbdb5f26c10c27c884b4cd8688baf756c5` returned **ACCEPT WITH NARROWER
CLAIM**. It found no blocker to the retention operation, while explicitly
excluding survival, viability, 24-hour outcome, repository-wide race-clean,
and holdout claims. It authorized one invocation against only the validated
superseded `2f93ace` capacity root.

The authorized invocation passed all retention checks. The 32,929,468,561-byte
probe was retained in
`/home/vlad/v2-r2-sv1-capacity-archive-2f93ace-retained.tar.zst`, a
4,020,725,425-byte zstd archive with SHA-256
`f80cdcc72ae1b18735b4cf0f4cda88118da4985ce5d6917ce22d30a1249c68e8`.
The archive comparison was empty, and the receipt binds the archive to the
old revision, old binary, registered configuration, and attestation. The
source root and old attestation were removed only after validation and are
recoverable from the verified archive. This does not alter any scientific
result and does not authorize a cell; no holdout was consumed.

The next candidate must be rebuilt from the final documented HEAD and receive
a fresh revision-bound capacity attestation before treatment-607.
