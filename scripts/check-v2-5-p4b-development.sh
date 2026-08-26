#!/usr/bin/env bash
# Check the immutable P4b development score and its retained cell evidence.
# This is a provenance/integrity check; it does not rerun the analyzer.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base="$root_dir/research/artifacts/v2-5-p4b"
full="$base/full"
score="$base/p4b-development-score.json"

test -s "$score"
jq -e '
  .contract == "v2-5-p4b-independent-perp-flow-v1" and
  .verdict == "FALSIFIED AT EXECUTION" and
  .development_seeds == [401,409] and
  .untouched_holdouts == [419,421,431] and
  .activation_status == "PASSED" and
  .execution_status == "FAILED" and
  .basis_status == "NOT MEASURABLE" and
  (.pair_verdicts == [
    {seed:401, verdict:"FALSIFIED AT EXECUTION"},
    {seed:409, verdict:"FALSIFIED"}
  ])
' "$score" >/dev/null

jq -e '
  (.paired_results | length) == 2 and
  (.paired_results[] | .seed as $seed |
    if $seed == 401 then
      .complete == false and .verdict == "FALSIFIED AT EXECUTION" and
      .result.control_valid == true and .result.treatment_valid == true and
      .result.activation_valid == true and .result.execution_valid == false and
      .result.basis_measurable == false and .result.valid == false
    elif $seed == 409 then
      .complete == true and .verdict == "FALSIFIED" and
      .result.control_valid == true and .result.treatment_valid == true and
      .result.activation_valid == true and .result.execution_valid == true and
      .result.basis_measurable == true and .result.valid == true and
      .result.seed_statistic_exact == "0" and .result.seed_statistic_sign == 0 and
      (.result.venues | length) == 1 and
      (.result.venues[0].paired_convergence_exact == "0") and
      (.result.venues[0].paired_convergence_sign == 0
       )
    else false end
  )
' "$score" >/dev/null

for seed in 401 409; do
	for arm in A B; do
		cell="$full/$arm-$seed"
		jq -e --arg arm "$arm" --argjson seed "$seed" '
			.analysis_contract == "v2-5-p4b-independent-perp-flow-v1" and
			.arm == $arm and .seed == $seed and
			.completion_sentinels == ["greeks.json", "latency.json"] and
			.runtime_evidence_artifact.events > 0 and
			(.runtime_evidence_artifact.digest | test("^[0-9a-f]{64}$"))
		' "$cell/analysis-metadata.json" >/dev/null
		runtime_events=$(jq -r '.events' "$cell/evidence-artifact-hash.json")
		runtime_digest=$(jq -r '.digest' "$cell/evidence-artifact-hash.json")
		offline_events=$(jq -r '.result.events' "$cell/evidenceartifacthash.json")
		offline_digest=$(jq -r '.result.digest' "$cell/evidenceartifacthash.json")
		[[ "$runtime_events" == "$offline_events" ]]
		[[ "$runtime_digest" == "$offline_digest" ]]
		jq -e '
			.result.valid == true and .result.receipt_audit_valid == true and
			.result.decisions > 0 and .result.submitted > 0 and .result.fills > 0 and
			(.result.hedgers | length) == 3 and
			(.result.hedgers | all(.fills == .reducing_fills and .terminal_absolute_gap == "0"))
		' "$cell/perpexposurehedger.json" >/dev/null
	done
done

for seed in 419 421 431; do
	if [[ -e "$full/A-$seed" || -e "$full/B-$seed" || -e "$base/pair-$seed.json" ]]; then
		echo "P4b holdout seed was consumed: $seed" >&2
		exit 1
	fi
done

echo "V2-5 P4b development score and evidence contract verified"
