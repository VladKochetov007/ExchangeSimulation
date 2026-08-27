# Research artifact templates

Use these schemas for long-running or resumable campaigns. Keep every JSONL append-only. Store large outputs separately and reference them by content hash and path.

## Contents

1. Workspace layout
2. Research contract
3. Hypothesis card
4. Experiment record
5. Evidence and claim records
6. Active frontier
7. Grader output
8. Campaign dashboard

## Workspace layout

```text
research/
  contract.yaml
  state.json
  frontier.md
  hypotheses.jsonl
  experiments.jsonl
  evidence.jsonl
  claims.jsonl
  decisions.jsonl
  artifacts/
  graders/
  reports/
```

Keep hidden graders outside the actor-visible workspace. Put candidate implementations in isolated worktrees or containers.

## Research contract

```yaml
question: ""
target_claim: ""
deliverable:
  type: ""
  schema: ""
baseline:
  artifact: ""
  metrics: {}
verification:
  tier: "A|B|C|D"
  public_metrics: []
  private_metrics: []
  invariants: []
  falsifiers: []
scope:
  allowed_data: []
  allowed_tools: []
  mutable_paths: []
  forbidden_actions: []
budgets:
  wall_time_s: 0
  tokens: 0
  cpu_hours: 0
  gpu_hours: 0
  money: 0
stop:
  success: []
  plateau: ""
  abort: []
human_gates: []
assumptions: []
```

## Hypothesis card

```json
{
  "id": "H-0001",
  "parent_ids": [],
  "mechanism_family": "representation/factorization",
  "statement": "",
  "representation_change": "",
  "causal_mechanism": "",
  "assumptions": [],
  "predicted_observable": "",
  "predicted_direction_or_delta": "",
  "cheap_test": "",
  "falsifier": "",
  "known_failure_modes": [],
  "estimated_cost": {},
  "proposed_by": "",
  "committed_at": "",
  "status": "proposed|pruned|probing|implemented|supported|contradicted|unknown"
}
```

Require `representation_change`, `causal_mechanism`, `cheap_test`, and `falsifier`; reject an empty hypothesis card.

## Experiment record

```json
{
  "id": "E-0001",
  "hypothesis_ids": ["H-0001"],
  "branch": "",
  "actor": "",
  "code_commit": "",
  "environment_hash": "",
  "data_hashes": [],
  "grader_hash": "",
  "command": "",
  "seeds": [],
  "started_at": "",
  "finished_at": "",
  "wall_time_s": 0,
  "token_cost": 0,
  "cpu_hours": 0,
  "gpu_hours": 0,
  "money": 0,
  "public_metrics": {},
  "private_metrics": {},
  "invariants_passed": false,
  "artifact_hashes": [],
  "notes": ""
}
```

Record the predicted outcome before filling measured metrics.

## Evidence and claim records

```json
{
  "evidence_id": "EV-0001",
  "kind": "primary-source|experiment|formal-check|physical-measurement|review",
  "locator": "URL or artifact path",
  "content_hash": "",
  "supports": [],
  "contradicts": [],
  "limitations": [],
  "verified_by": [],
  "created_at": ""
}
```

```json
{
  "claim_id": "C-0001",
  "text": "",
  "status": "supported|contradicted|unknown",
  "supporting_evidence": [],
  "contradicting_evidence": [],
  "assumptions": [],
  "external_validity": "",
  "reproduction_ids": [],
  "updated_at": ""
}
```

Never promote a claim to `supported` solely from an LLM review.

## Active frontier

Keep `frontier.md` short enough to fit in one agent message:

```markdown
# Active frontier

## Target and baseline

## Supported claims

## Live mechanism families

| Hypothesis | Mechanism | Evidence | Risk | Next test | Cost |
| --- | --- | --- | --- | --- | --- |

## Contradictions and evaluator risks

## Recently closed dead ends

## Budget remaining

## Next three decisions
```

Regenerate this view from the ledger. Do not overwrite the raw records it summarizes.

## Grader output

```json
{
  "candidate_id": "",
  "valid": false,
  "violations": [],
  "reward": {
    "task_quality": 0,
    "robustness": 0,
    "novelty": 0,
    "reproduction": 0,
    "cost_efficiency": 0
  },
  "public_private_gap": null,
  "suspicious_improvement": false,
  "required_followups": [],
  "grader_hash": "",
  "artifacts": []
}
```

Use validity as a hard gate. Keep the reward vector visible; derive any scalar search score in the controller so it remains auditable.

## Campaign dashboard

Track at least:

- best valid private score and delta from baseline;
- independently reproduced score;
- improvement per dollar, token, and GPU-hour;
- public-private gap;
- reward-hacking or leakage incidents;
- fraction of proposed mechanisms tested;
- mechanism diversity among surviving branches;
- duplicate and rediscovered dead ends;
- human minutes per accepted claim;
- critic precision, recall, and calibration when critic predictions are logged.
