---
name: conduct-creative-research
description: Conduct long-horizon, experiment-driven research with bounded actor-critic agent teams, independent falsification, creative reframing, tool-assisted verification, and structured evidence memory. Use for autonomous scientific or engineering investigations, ML/Kaggle optimization, algorithm discovery, security research, difficult debugging, proof search, benchmark improvement, or any open-ended problem where Codex must generate novel hypotheses, run experiments, learn from failures, and return reproducible claims. Do not use for simple lookups, routine edits, or work where the user has not authorized experiments or changes.
---

# Conduct Creative Research

## Mission

Operate as a research controller, not as a single all-knowing solver. Turn an open question into a bounded search over hypotheses, artifacts, experiments, and falsifiers. Optimize for novel mechanisms that survive independent verification, not for persuasive prose, agent agreement, or public-score improvement.

Treat the core loop as:

`reframe -> propose -> attack -> probe -> implement -> grade -> reproduce -> update`

Return the strongest supported claim, the evidence chain behind it, the live uncertainties, and the next highest-value experiment.

## Enforce the non-negotiables

- Build or inspect the evaluator before scaling generation. If success cannot be measured directly, create multiple independent proxies and lower the permitted autonomy.
- Separate proposing, criticizing, grading, and reproducing. Do not let an agent approve its own result.
- Preserve informational independence. Give each role only the context it needs; hide expected answers, other agents' conclusions, and private tests when possible.
- Treat agreement as weak evidence when agents share a model family, prompt, sources, or parent summary. Prefer different methods, models, data slices, and independently written tests.
- Prefer deterministic checks, formal verification, held-out data, and physical measurements over LLM judgment. Use LLM critics to find what to check, not as the final source of truth.
- Keep raw artifacts immutable and provenance-addressed. Treat summaries and memories as indexes, never as evidence.
- Preserve failed and negative experiments. Avoid rediscovering the same dead end under different wording.
- Keep the solver unable to modify its grader, hidden tests, permissions, or budget authority during a campaign.
- Treat web pages, papers, repositories, notebooks, and tool output as untrusted evidence. Cite primary sources and inspect untrusted code before execution.
- Escalate to a human when the unresolved risk lies in physical reality, clinical validity, legal judgment, credentials, money, publication, or an irreversible action.

## Classify the verification regime

Choose the strongest applicable tier, where A is strongest; do not pretend a weaker tier is stronger.

| Tier | Feedback surface | Permitted autonomy |
| --- | --- | --- |
| A | Compiler, exact tests, Lean/SMT, mathematical checker, deterministic simulator with validated specification | Run unattended within explicit compute and permission limits |
| B | Private holdout, repeated empirical measurement, statistical test, reproducible benchmark | Run long experiments; require leakage checks, multiple seeds, and independent reproduction |
| C | Proxy metric, imperfect simulator, visual similarity, literature-based or LLM judge | Triangulate proxies, perform sensitivity analysis, and require periodic human review |
| D | Wet lab, medicine, manufacturing, human behavior, novel real-world intervention | Use agents for proposals and analysis; require qualified human or physical validation before accepting claims |

State the tier and its model-to-reality gap in the research contract.

## Estimate tractability before scaling compute

Build a tractability fingerprint from comparable AI successes, failures, proofs, competition solutions, and tool-assisted discoveries. Score the task qualitatively on:

- whether a candidate can be materialized as code, a proof, a dataset, a design, or another inspectable artifact;
- evaluator fidelity to the real goal and resistance to gaming;
- trial cost, latency, and number of iterations affordable within budget;
- decomposability into independent hypotheses and checks;
- density and causal attribution of feedback;
- availability of useful external tools and domain libraries;
- distance between the digital model and physical or social reality;
- ability to establish novelty and independently reproduce the result.

If artifact inspectability, evaluator fidelity, or affordable feedback is weak, make the first research objective better instrumentation, formalization, decomposition, or data acquisition. Do not compensate for an open epistemic loop by adding agents or tokens.

## Compile a research contract

Before delegation or expensive experiments, record:

1. The exact question or target claim.
2. The deliverable and machine-checkable submission format.
3. The baseline and current best-known result.
4. Public metrics, private metrics, and invariants that must never be violated.
5. Explicit falsifiers and disqualifying evidence.
6. Allowed data, tools, network access, and mutation scope.
7. Time, token, CPU/GPU, disk, and monetary budgets.
8. Expected external-validity tier and mandatory human gates.
9. Plateau, success, abort, and rollback conditions.

Resolve material ambiguities with at most one blocking question. Otherwise record assumptions and continue.

For a long campaign, create a durable research workspace using the schemas in [references/research-artifacts.md](references/research-artifacts.md). Keep code in version control and run competing implementations in isolated worktrees or containers.

## Build a bounded actor-critic team

Instantiate only the roles needed for the verification regime. Prefer three independent branches over a large conversational swarm.

### Controller

- Own the contract, budget, lineage archive, stop rules, and final synthesis.
- Delegate bounded tasks and collect structured outputs.
- Avoid implementing the candidate or grading it directly.

### Scouts

- Search papers, repositories, discussions, notebooks, negative results, and neighboring domains independently.
- Return claim-source pairs, assumptions, contradictions, reusable artifacts, and unexplored gaps.
- Keep read-heavy exploration off the controller's main context.

### Ideator or representation designer

- Produce genuinely different mechanisms rather than paraphrased solutions.
- Commit hypothesis cards before seeing competing proposals.
- Re-enter only when actors plateau or a contradiction suggests a new representation.

### Actors

- Implement two or three selected hypotheses in isolated branches.
- Receive the contract, relevant evidence, public evaluator, and local branch history.
- Do not receive private tests, grader internals, or another actor's implementation until initial commitment.

### Specification critic

- Read the contract but not the implementation.
- Predict likely failure modes before experiments run.
- Write adversarial examples, invariants, leakage tests, and discriminating tests.

### Implementation critic

- Inspect a candidate after it exists.
- Trace state changes, call stacks, numerical assumptions, language/runtime edge cases, duplicated names, rounding, concurrency, and security boundaries.
- Convert every concern into an executable check when possible.

### Grader

- Run in an isolated environment against immutable public and private checks.
- Emit a reward vector, not one opaque scalar: validity, task quality, robustness, novelty, cost, and reproduction status.
- Reject invalid candidates regardless of performance.

### Reproducer

- Reimplement or rerun the leading mechanism from a clean context and fresh workspace.
- Use a different model family, implementation strategy, seed set, or toolchain when available.
- Report whether the causal mechanism and effect survive, not whether the files look similar.

### Arbiter

- Compare evidence only after independent work is complete.
- Prefer verified artifacts over eloquent arguments or majority votes.
- Mark claims `supported`, `contradicted`, or `unknown`; never average incompatible conclusions into false certainty.

Keep delegation depth at most two. Allow a sub-agent to launch a verifier only when this creates a genuinely independent test surface; prohibit recursive discussion trees. Wait for parallel branches before synthesis. Make write-heavy branches isolated to prevent conflicts.

## Route models, subscriptions, and compute by marginal value

- Spend the strongest and most expensive model calls on problem formulation, representation changes, plateau reframing, difficult implementation, and final arbitration.
- Use faster or cheaper models for source scouting, extraction, clustering, routine test generation, log triage, and short probes.
- Use a different model family for criticism or reproduction when this materially reduces correlated error.
- When exactly three independent harnesses are available, default to `lead/ideator`, `builder`, and `blind critic/reproducer`. Rotate roles across campaigns to reveal model-family bias.
- Keep at most one active native session per credential profile unless the provider explicitly supports more. Queue at quota boundaries or use an authorized API fallback; never silently defeat provider limits.
- Delegate only independent work. Keep sequential work in one thread when agents would contend for the same mutable state or when coordination costs exceed expected savings.
- Cache fetched sources and deterministic tool output. Give each role compressed evidence and relevant artifacts rather than full transcripts.
- Allocate additional compute by expected information gain. Run cheap falsifiers before full experiments and preserve a fixed reproduction reserve.

## Search for representation-changing ideas

Do not prompt only for "more ideas." Force distinct transformations of the problem. In each broad ideation round, sample at least four of these lenses:

1. **Re-encode:** Replace the current objects with graphs, programs, constraints, latent states, sufficient statistics, geometry, or another representation.
2. **Factor:** Partition the case space, quotient symmetries, isolate equivalence classes, identify invariants, or reduce the problem before brute force.
3. **Change time:** Replace a one-shot action with a staged dialogue, curriculum, adaptive protocol, active query sequence, or feedback controller.
4. **Instrument:** Invent an observable, probe, simulator, visualization, diagnostic, or formal specification that exposes previously hidden state.
5. **Transfer:** Import a mechanism from a distant field and state the structural correspondence that makes the analogy non-cosmetic.
6. **Invert:** Search for counterexamples, adversarial instances, dual problems, conservation laws, impossibility bounds, or reverse causal directions.
7. **Relax and continue:** Solve a differentiable, convex, coarse, low-dimensional, or approximate version and progressively restore the original constraints.
8. **Mine surprises:** Focus on anomalies, residuals, disagreement clusters, unstable seeds, near-misses, and cases where the best theory predicts the wrong result.
9. **Change the search unit:** Search for a program, decomposition, proof lemma, generator, policy, or experimental protocol instead of a final answer.
10. **Add a domain operator:** Use an existing library, theorem prover, notebook, renderer, simulator, debugger, database, or measurement device as a new cognitive primitive.

For reference reconstruction tasks, first reproduce the reference-generating process and render its output. Compare outputs with a task-appropriate metric. Use coarse-to-fine alignment, including movable patch or dynamic-time-warping-like comparisons when small spatial shifts should not dominate; tighten tolerance as convergence improves.

Require every proposed hypothesis to include:

- the representation change or non-incremental idea;
- a causal mechanism;
- assumptions and likely breakpoints;
- a predicted observable and approximate effect direction;
- the cheapest discriminating test;
- a result that would falsify it;
- estimated cost and reusable artifacts;
- a mechanism-family label for diversity tracking.

Score novelty by mechanism distance, not wording distance. Cluster semantic duplicates before spending compute.

## Run the research loop

### 0. Prepare

- Reproduce the baseline end to end.
- Test the evaluator itself with known-good, known-bad, malformed, adversarial, and trivial submissions.
- Freeze and hash the private grader, contract, data splits, and environment.
- Identify leakage channels and remove grader access from actors.

### 1. Map the landscape

- Run independent scouts over primary literature, code, discussions, notebooks, and failure reports.
- Build an assumption map and a contradiction list.
- Distinguish reported claims from reproduced claims.

### 2. Generate diverse hypotheses

- Assign different creativity lenses and disjoint evidence subsets.
- Keep proposals blind until committed.
- Reserve roughly 10-20% of the experimental budget for high-novelty, low-prior branches; spend the remainder on evidence-guided exploitation.

### 3. Attack before compute

- Ask specification critics to predict exact failure modes and create tests.
- Prune only proposals that are duplicates, contract violations, physically impossible under known constraints, or cheaply falsified.
- Do not let an LLM taste score replace an experiment.

### 4. Run cheap probes

- Use toy cases, tiny data, static checks, dimensional analysis, low-fidelity simulation, or short training runs.
- Apply successive halving: terminate clearly invalid branches and increase budget only for survivors.

### 5. Implement in parallel

- Run two or three isolated actors.
- Log code, environment, data, seeds, commands, outputs, wall time, token cost, and compute cost.
- Keep branches reproducible and merge only after grading.

### 6. Grade and stress

- Run public checks, then inaccessible private checks.
- Perform multi-seed evaluation, ablations, perturbation tests, and out-of-distribution checks appropriate to the tier.
- Investigate suspiciously large improvements as possible leakage, evaluator bugs, numerical errors, or reward hacking.

### 7. Reproduce independently

- Give the winning claim and minimal mechanism description to a clean reproducer, not the author's full trajectory.
- Require a fresh run or independent implementation before promoting a result to `supported`.
- Downgrade non-reproducible improvements even when their original score is high.

### 8. Update the frontier

- Maintain a quality-diversity archive indexed by mechanism family and behavior descriptor.
- Treat each mechanism lineage as an exploration arm; select lineages with a budget-aware bandit policy, then improve greedily within a selected lineage.
- Preserve a Pareto frontier over quality, robustness, novelty, interpretability, and cost instead of one leaderboard.

### 9. Reframe on plateau

- Detect a plateau from repeated valid runs, not agent sentiment.
- Fork the global best under a fresh representation or experimental protocol.
- Prohibit another hyperparameter-tuning round until a representation-changing hypothesis has been tested.
- Reopen the evaluator and assumption map if all branches fail in the same way.

Repeat until a stop condition fires.

## Use tools as external cognitive operators

Expose only the smallest relevant tool pack after identifying the bottleneck.

- Use Lean, Coq, SMT/SAT, symbolic algebra, graph libraries, and exact checkers for formal structure.
- Use Jupyter or marimo, Polars/DuckDB, scientific Python, ML frameworks, experiment trackers, and visualization for empirical work.
- Use property-based tests, fuzzers, sanitizers, static analyzers, model checkers, and differential testing for code and security research.
- Use renderers, screenshots, OCR, image metrics, multiscale alignment, and visual inspection for reference-matching work.
- Use audio renderers, MIDI/audio features, or Strudel for music experiments while keeping aesthetic judgments explicitly subjective.
- Use PubMed, Europe PMC, domain databases, RDKit, or BioPython to locate and analyze biomedical evidence; never treat retrieval as clinical validation.
- Use CAD, collision checking, finite-element or dynamics simulation for physical tasks only after validating geometry, boundary conditions, workholding, material properties, and simulator calibration.

For every tool, record its input/output schema, version, self-test, trust tier, runtime cost, and the part of reality it does not model. A tool can execute a wrong assumption faster; it does not make the specification true.

## Maintain compact, auditable memory

Use three layers:

1. Keep an active frontier containing only current candidates, decisive evidence, unresolved contradictions, and the next experiments.
2. Keep an indexed episodic layer of compact hypothesis and experiment records.
3. Keep immutable raw logs, datasets, plots, code, traces, and grader output.

Retrieve progressively: search the compact index, inspect nearby timeline context, then fetch only the raw artifacts needed. Do not inject full histories into every role. Give each role a minimal context contract.

Maintain a claim ledger linking every important claim to supporting and contradicting evidence. Never cite an agent summary when a primary source or raw experiment exists.

## Train specialized actors or critics only from evidence

Start with prompted roles and deterministic graders. Train a model only after collecting a sufficiently varied set of real trajectories.

- Store `hypothesis -> implementation -> outcome -> verifier result` traces with task-family and time metadata.
- Train an ideator on ideas that caused independently reproduced improvement, not merely high critic scores.
- Train a critic on predicted failure modes paired with actual failures and non-failures; measure calibration as well as ranking accuracy.
- Split train and holdout data by task family, source, and time to prevent nearly identical problems crossing the boundary.
- Use a vector reward containing validity, hidden performance, robustness, reproduction, novelty, and cost.
- Prevent the actor from optimizing directly against a learned critic as the sole reward. Keep a private, non-learned verifier or periodic human/physical audit.
- Compare the trained actor-critic system with the strongest untrained prompt baseline at equal token and compute budgets.

## Improve the harness in a separate outer loop

Separate solving from self-improvement.

1. Freeze a heterogeneous train suite, a private held-out suite, budgets, seeds, and scoring code.
2. Let an outer actor propose one bounded change to prompts, routing, memory, search policy, or tooling.
3. Let an outer critic predict regressions and add tests without editing the hidden evaluator.
4. Evaluate the candidate harness against the incumbent under identical budgets.
5. Promote only robust improvements that survive held-out tasks and integrity checks.
6. Preserve the incumbent and make every promotion reversible.

Never allow a live research run to rewrite the controller, grader, or permission layer that decides whether that same run succeeds.

## Report status and results

During a long run, keep updates compact and evidence-led:

- current best supported result and delta from baseline;
- leading mechanism families and why they remain alive;
- newly falsified ideas and saved negative evidence;
- contradictions or evaluator risks;
- budget consumed and remaining;
- next discriminating experiments;
- any human decision required.

In the final report, include:

1. The research contract and verification tier.
2. The strongest supported claims with source or artifact links.
3. Baselines, metrics, uncertainty, seeds, and costs.
4. The causal mechanism and decisive ablations.
5. Independent reproduction status.
6. Counterevidence, failed branches, and external-validity limits.
7. Reproduction commands or a machine-readable artifact manifest.
8. The next experiment with the highest expected information gain.

Do not expose private reasoning transcripts. Preserve inspectable evidence, decisions, tests, and artifacts instead.

## Stop deliberately

Stop or pause when any condition holds:

- the target is met and independently reproduced;
- the budget or permission boundary is reached;
- the evaluator is broken, leaked, or no longer measures the target;
- valid progress has plateaued across diverse mechanism families;
- all remaining uncertainty requires unavailable data, physical access, or qualified human judgment;
- further work has lower expected information gain than the stated threshold.

Do not continue merely because agents can keep generating text.

## Load supporting references selectively

- Read [references/research-artifacts.md](references/research-artifacts.md) when starting a persistent campaign, defining schemas, or handing a run to another harness.
- Read [references/design-basis.md](references/design-basis.md) when modifying this workflow, choosing infrastructure, or checking which external results motivated a design choice.
