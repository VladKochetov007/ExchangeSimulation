# Design basis and source map

Use this file to audit or revise the workflow, not as mandatory context for every campaign. Prefer the linked primary paper, official documentation, or repository over this summary. Treat recent preprints and project claims as provisional until independently reproduced.

## Contents

1. Environment and search loops
2. Actor-critic and multi-agent independence
3. Creativity plus automated verification
4. Limits of autonomous research
5. Infrastructure and memory
6. Skill and evaluation mechanics

## Environment and search loops

- [EurekAgent](https://arxiv.org/abs/2606.13662): motivate permissions, artifacts, budgets, isolated evaluation, human oversight, and a simple prepare/propose/parallel-implement loop instead of over-prescribed agent chatter.
- [AIDE](https://arxiv.org/abs/2502.13138): treat ML engineering as tree search over executable code, reusing and improving promising solution nodes.
- [AIDE² report](https://www.weco.ai/blog/first-evidence-of-recursive-self-improvement): report an outer loop that improves an inner research harness; useful mechanisms include lineage-as-bandit-arm selection, fork-on-stall, minimal role-specific context, private evaluation, and anti-reward-hacking pressure. The report also documents rejected ideas and a broken statistical defense, so do not treat it as conclusive RSI proof.
- [OpenRSI](https://github.com/AlexWortega/OpenRsi): implement propose, pre-compute critique, evaluate, adversarially verify, and keep; require causal mechanisms, predicted deltas, and falsification conditions. Treat repository benchmarks as project-reported until independently replicated.
- [AREX](https://arxiv.org/abs/2607.21461): use discovery-verification asymmetry, constraint-wise audits, targeted follow-up research, and compact verified state for long-horizon deep research.
- [Ouroboros](https://arxiv.org/abs/2608.08311): separate reviewed core evolution from frozen benchmark campaigns and emphasize authoritative guardrails and reversible changes.

## Actor-critic and multi-agent independence

- [MLE-Ideator](https://arxiv.org/abs/2601.17596): separate strategic ideation from implementation and train a smaller ideator from execution rewards.
- [Asymmetric Actor-Critic for Multi-turn LLM Agents](https://arxiv.org/abs/2604.00304): explore a strong fixed actor supervised by a smaller critic, supporting the generation-verification asymmetry while not replacing objective graders.
- [DynaDebate](https://arxiv.org/abs/2601.05746): target homogeneous reasoning paths through explicit path diversification, process-level critique, and tool-triggered verification.
- [Multiagent Debate](https://arxiv.org/abs/2305.14325): provide evidence that debate can improve some reasoning tasks, but do not use debate alone as proof because shared models and contexts produce correlated errors.
- [OpenAI subagent documentation](https://learn.chatgpt.com/docs/agent-configuration/subagents): recommend parallel agents for bounded independent work and warn about token use, context pollution, and write-heavy coordination conflicts.

## Creativity plus automated verification

- [FunSearch](https://www.nature.com/articles/s41586-023-06924-6): pair LLM-generated programs with a systematic evaluator and search for interpretable programs rather than raw answers.
- [AlphaEvolve](https://deepmind.google/blog/alphaevolve-a-gemini-powered-coding-agent-for-designing-advanced-algorithms/): combine broad cheap generation, deeper models, evolutionary selection, and automated evaluators; fit best when work can be expressed as verifiable algorithms.
- [Google AI co-scientist](https://research.google/blog/accelerating-scientific-breakthroughs-with-an-ai-co-scientist/): supply useful roles—generation, reflection, ranking, evolution, proximity, and meta-review—but use pairwise ranking for prioritization rather than independent ground truth.
- [ROGII wellbore geology 7th-place write-up](https://www.kaggle.com/competitions/rogii-wellbore-geology-prediction/writeups/7th-place-solution-hmm-unet-agent-is-all-you): illustrate agent-assisted iteration in a metric-rich competition and the continuing importance of validation and leakage analysis.

## Limits of autonomous research

- [ResearchClawBench](https://arxiv.org/abs/2606.07591): evaluate end-to-end rediscovery over 40 tasks and report large remaining gaps in experimental protocol, evidence alignment, and identifying the scientific core.
- [Manufacturing physical-reasoning evaluation](https://adamkarvonen.github.io/machine_learning/2025/04/13/llm-manufacturing-eval.html): illustrate visual errors, impossible workholding, rigidity/chatter failures, and the gap between benchmarkable knowledge and tacit physical reality.
- Use formal tools and simulators to verify their specifications; separately validate that the specification models the real task.

## Infrastructure and memory

- [Claudexor](https://github.com/razzant/claudexor): provide isolated multi-harness runs, best-of-N, cross-family reviewers, bounded delegation, typed evidence, and budget/quota accounting. Use it as an execution layer, not as the scientific controller.
- [Buzz](https://github.com/block/buzz): provide a self-hosted event log, agent/human workspace, signed audit trail, workflows, and multi-agent communication. Add it when several machines or people need a shared control plane; local Git and an evidence ledger are simpler for one host.
- [claude-mem](https://github.com/thedotmack/claude-mem): demonstrate progressive retrieval from search to timeline to full observations. Use this as episodic retrieval, not as the authoritative claim ledger.

## Skill and evaluation mechanics

- [OpenAI skill documentation](https://learn.chatgpt.com/docs/build-skills): define progressive disclosure, `SKILL.md`, optional references/scripts, and explicit or implicit invocation.
- [OpenAI agent evaluation documentation](https://developers.openai.com/api/docs/guides/agent-evals): recommend traces during debugging and repeatable datasets/eval runs once good behavior is defined.
- [OpenAI trace grading documentation](https://developers.openai.com/api/docs/guides/trace-grading): support structured scoring of end-to-end model calls, tools, handoffs, and guardrails for regression analysis.
