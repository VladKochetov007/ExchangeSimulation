# Skill validation — authoring task

This record describes synthetic skill evaluation, not market experiments.
Baseline: `32d340f188460712d94b43f76f1de1928b013bf0`.
Initial review candidate: `e307bca132aa1b3caabe0055f6d1d6f64d1cba87`.
Follow-up typed causal-verdict field: `8b08780aca9c3241bcd4e6eea0e6f8d953fad96a`.
Mechanical-only consistency fix: `f2cca325fa552766e23f83c3204bfa8f83f9e241`.
Final skill directory tree: `d1ff061a7d98b9e003d7ac4166d1f7434f54eb17`.
Later review-record/formatting commits do not change that tested skill tree.

## Completed local checks

Using installed Go `go1.27.0`, each command completed with exit status 0:

```bash
go run ./.agents/skills/market-ecology-research/scripts/validate.go --root .
go test ./.agents/skills/market-ecology-research/scripts/validate.go ./.agents/skills/market-ecology-research/scripts/validate_test.go -count=1 -v
go vet ./.agents/skills/market-ecology-research/scripts/validate.go ./.agents/skills/market-ecology-research/scripts/validate_test.go
go test ./tests -run '^TestTrackedFilesAvoidSystemTempPaths$' -count=1
git diff 32d340f --check
```

The helper uses only the Go standard library and reads metadata, linked-source
existence and local Markdown references. It rejects duplicate study IDs,
dependency cycles, dangling policies/files/evidence, invalid enums, omitted or
unknown fields, unexpected null/trailing input, review acceptance after service
failure, and economic/causal verdicts without the required evidence state.
A separate otherwise-valid mechanical fixture accepts the two non-substantive
causal states and rejects all four substantive causal verdicts.
A synthetic completed valid no-trade record is accepted, with no automatic
economic verdict. External evidence locations are opaque metadata, never read.

This is a typed version-1 record check and a check of the deliberately limited
YAML shape emitted by this skill. It is not a general JSON Schema/YAML engine,
an authorization checker against owner conversations, an evidence verifier, or
a scientific grader. Human/reviewer assessment still establishes claim scope.
The creator's Python validator was not used because repository instructions
restrict Python to visualization; equivalent applicable metadata checks run in Go.

The merged readiness packet, pilot protocol and next-study brief compare
byte-for-byte equal to baseline `32d340f`. No simulator, actor, economic default,
historical result, protected evidence or retained manifest changed.

## Mock inputs and independent evaluation

[mock-cases.md](mock-cases.md) records all ten synthetic prompts. Two fresh
Luna-xhigh reviewers received disjoint A/B assignments and the skill references,
without supplied expected answers. Their actual responses are preserved in
[mechanics.txt](reviews/mechanics.txt) and [inference.txt](reviews/inference.txt).

| Review | Execution | Verdict and resolution |
|---|---|---|
| A: mechanics, evidence and timing | COMPLETED | e307bca: ACCEPT_WITH_REQUIRED_CHANGES; same reviewer accepted 8b08780 after causal-field correction |
| B: authorization, history and inference | COMPLETED | 8b08780: ACCEPT_WITH_REQUIRED_CHANGES; same reviewer accepted the bounded f2cca32 consistency delta, retaining the original overall verdict |

Neither reviewer implemented the candidate. A's statement that it used no
independent reviewer means it did not spawn another reviewer within its own task.
Both were independent of the author, not independent model families.
These are authoring-scope reviews, not acceptance of a pilot or market result.

| Case | Observed mock response |
|---|---|
| A1 pilot scope | execution quantity, not maker profitability/capital capacity; no run |
| A2 static arbitrage | $2.80 arithmetic gain in the supplied fixture, not emergent alpha |
| A3 delayed fill | exchange execution precedes cancellation despite later receipt; reconcile identities and 5 = 2 + 3 |
| A4 latency | unchanged realized callback path does not establish general irrelevance |
| A5 signed statistic | real log return undefined, not imputed zero |
| B1 no trades | valid evidence and rational abstention remain distinct from activation |
| B2 negative history | preserve restoration non-activation; no capital rescue |
| B3 missing authority | INTAKE/PLAN only, no autonomous backlog execution |
| B4 service failure | UNAVAILABLE/NOT_ISSUED; bounded fallback then external package |
| B5 synthesis | separate endpoints/populations and unrun proposal; no invented effect values |

The two required record changes are now covered by local regression tests.
The reviewers performed static inspection, not validator execution or runtime
skill-selector testing. Ten qualitative mocks are a smoke evaluation, not a
statistical estimate of future agent reliability or a whole-platform audit.

## Discovery limits

The 196-line core, supported name/description frontmatter, reference paths and
explicit-only metadata pass structural validation. This session's initial
skill catalogue predates the new folder; no interactive `/skills` selector was
exercised. Reading the skill by explicit path during mock evaluation is not
evidence of selector discovery. The [program README](README.md) supplies a
fresh-session lookup and exact version check from the feature checkout.

No study implementation, world, seed reservation or holdout inspection was
performed. Broad market suites were intentionally not run for this authoring task.
