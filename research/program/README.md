# Market-ecology program

This is the finite idea registry for the counterfactual market-ecology laboratory.
The authoritative index is [registry.json](registry.json); each ID links to its
idea card and any existing protocol. The [policy catalogue](policy-catalogue.md)
maps candidate families to inspected source and explicit gaps.

Authoring baseline: `32d340f188460712d94b43f76f1de1928b013bf0`,
the merge of `a878dca` and planning tip `febf66b`.
Current research progress is recorded by each registry entry, not inferred from
that authoring baseline. [ME-000](ideas/ME-000/report.md) completed its bounded
mechanical readiness gate at `b25223a`; this is not an ME-001 economic result.
The owner's 2026-09-23 overnight goal conditionally authorizes ME-001 development
only after a prospective protocol lock. No ME-002+ study is authorized to run.

## Starting queue

| Priority | IDs | Next boundary |
|---|---|---|
| 1 | [ME-000](ideas/ME-000/report.md) | CLOSED: F1/F2 mechanical readiness accepted at `b25223a` |
| 2 | [ME-001](ideas/ME-001/idea.md) | lock/review the [merged pilot draft](../market-ecology-capacity-pilot-protocol-DRAFT.md) before any development world |
| 3 | ME-002–ME-007 | separately chosen timing, instruction, allocation or arbitrage question |
| 4 | ME-008–ME-010 | payoff readiness, selected ensemble or named derivative child |
| 5 | ME-011–ME-012 | restricted games and explicit capital dynamics after reliable payoffs |

This is dependency order, not a run queue. ME-010 is a planning umbrella and needs
a named child protocol; an umbrella cannot authorize a combined derivative campaign.
The ME-001 plan remains one ABC/USD venue, immediate execution, three quantities
and three compositions, three proposed seeds, 27 worlds plus two controls.
Its economic object is execution quantity capacity. The source-grounded
[readiness packet](../NEXT-STUDY-READINESS-PACKET.md) owns the two prerequisites.

## Historical map (HISTORY, never new-main results)

| Records | Relevant IDs | Preserved interpretation |
|---|---|---|
| [executionlab studies](../executionlab-2026-08-15.md) | ME-000/001/003 | historical immediate/TWAP software and cost evidence; new composition matrix unrun |
| [V2-2b informed maker/router](../v2-2b-price-discovery-smoke-results.md) | ME-005/006 | quote-mediated screen supported; router residual-edge endpoint falsified; decomposition mixed/incomplete |
| [no-arbitrage audit](../no-arbitrage-audit.md) | ME-007 | historical omniscient quote diagnostic with corrected scanner; not executable participant profit |
| [P1 size response](../v2-3-inventory-size-p1-results.md), [P2 rebalance](../v2-3-inventory-rebalance-p2-results.md) | ME-008 | activation/integrity screens; no stability or profitability inference |
| [L0 liability](../v2-4-liability-hedger-l0-results.md) | ME-009 | finite local motive integrity; no broad demand-elasticity claim |
| [clock artifacts](../clock-artifacts-ae13f9a.md), [L1-P2 phase](../v2-4-l1p2-noise-phase-results.md) | ME-002 | old mixed timing limitations preserved; later relative-phase screen is population-specific |
| [P4 funding carry](../v2-5-p4-funding-carry-results.md) | ME-010 | registered market-basis endpoint FALSIFIED despite participant activation |
| [P5 dated carry](../v2-5-p5-dated-carry-results.md) | ME-010 | NOT EXERCISED under its exact eligibility contract |
| [P6 options](../v2-6-p6-options-results.md) | ME-010 | O0–O2 activation/integrity; directional transmission NOT IDENTIFIED; O3 paired/O4 NOT EXERCISED |
| [P7d directional distress](../v2-7-p7d-results.md) | ME-010 | bounded development participant-risk screen; not a new-main stress certificate |
| [R2/SV1D closure](../v2-r2-sv1d-iteration-closeout.md) | ME-009/012 context | line closed at valid non-activation; no new survival result |

Exact identities remain in those original reports and manifests. In particular,
the five-minute seed-659 trajectory used `971267d`; `8de607d` identifies
the accepted queued-fill analyzer correction/rescore. R2 remains non-viable at its
24-hour survival gate. SV1D supplied trading activity but failed the restoration
predicate. No formal tri-arm promotion or completed 24-hour successor follows.
Historical reserved holdouts remain untouched.

Use the [consolidation record](../REPOSITORY-CONSOLIDATION.md) before inspecting
an old ref. Do not repeat worktree archaeology or read raw evidence for catalogue
completion. New studies require new hypotheses/conditions, not rescue attempts.

## Skill discovery and version

The repository skill is
[market-ecology-research](../../.agents/skills/market-ecology-research/SKILL.md).
It requires explicit invocation through `/skills` or `$market-ecology-research`.
Modes are natural prompt conventions, not CLI flags.
[Official documentation](https://learn.chatgpt.com/docs/build-skills) describes
repository discovery and `allow_implicit_invocation: false`.

Launch Codex from a checkout containing this feature, use `/skills`, and select
the exact repository path. If the entry is absent, restart and recheck.
Do not claim discovery solely from file existence or a frontmatter validator.

From that checkout, record the loaded version with:

```bash
git status --short
git log -1 --format=%H -- .agents/skills/market-ecology-research
git rev-parse HEAD:.agents/skills/market-ecology-research
```

The first command reveals local changes; the second identifies the last skill
commit and the third its directory tree at HEAD. At protocol lock also record
the governing full repository candidate and protocol identity.
No global skill installation, plugin service or credentials change is required.
The optional anti-ai-alop and tmux skills named in AGENTS.md were not found in
the inspected local skill locations; this task uses plain documentation and
short checks and does not claim either skill was loaded.

## Invocation examples

```text
$market-ecology-research
Mode: PLAN. Study: ME-001. Use the merged draft. No simulations.
```

```text
$market-ecology-research
Mode: PREPARE. I authorize only the two recorded ME-000 readiness fixes and
their regression tests. No economic worlds or holdouts.
```

```text
$market-ecology-research
Mode: RUN. Execute only <owner-approved protocol revision> for <study ID>,
within its pinned seeds, horizon, world count and resource limits.
```

```text
$market-ecology-research
Mode: SYNTHESIZE. Compare completed studies <IDs>; no new runs and no
upgrading historical scope.
```

These are examples, not present authorizations. A real RUN instruction must
resolve the placeholders to a pinned protocol and finite owner-approved budget.

## Validation and records

```bash
go run ./.agents/skills/market-ecology-research/scripts/validate.go --root .
go test ./.agents/skills/market-ecology-research/scripts/validate.go ./.agents/skills/market-ecology-research/scripts/validate_test.go
git diff --check
```

The helper reads metadata, templates and Markdown links; it never opens raw
evidence, launches a market, grants authority or decides scientific truth.
Its typed record definitions and
[record contract](../../.agents/skills/market-ecology-research/references/statistics-and-reporting.md)
define schema version 1. Null later-stage paths mean no report/result exists.
Only completed reports may supply empirical or synthesis conclusions.
Mock evaluations are identified separately in [skill-validation.md](skill-validation.md).
