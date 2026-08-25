#!/usr/bin/env bash
set -euo pipefail
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base="$root_dir/research/artifacts/v2-5-p5"
output="$base/development-verdict.json"
if [[ -e "$output" ]]; then echo "refusing to overwrite $output" >&2; exit 1; fi
for seed in 117 119; do
  for arm in A B; do
    cell="$base/$arm-$seed"
    [[ -s "$cell/analysis-metadata.json" && -s "$cell/datedcarryp5.json" && -s "$cell/datedmandatep5.json" ]] || { echo "incomplete cell $cell" >&2; exit 1; }
  done
  [[ -s "$base/pair-$seed.json" ]] || { echo "missing pair $seed" >&2; exit 1; }
done
pairs_valid=$(jq -s 'all(.[]; .result.structural_config_valid == true and .result.same_source_revision == true and .result.control_audit.valid == true and .result.treatment_audit.valid == true and .result.control_mandate_audit.valid == true and .result.treatment_mandate_audit.valid == true)' "$base/pair-117.json" "$base/pair-119.json")
no_eligible=$(jq -s 'all(.[]; .result.treatment_audit.eligible_decisions == 0 and .result.treatment_audit.target_changes == 0 and .result.treatment_audit.submitted == 0)' "$base/pair-117.json" "$base/pair-119.json")
activation=$(jq -s 'all(.[]; .result.activation_valid == true)' "$base/pair-117.json" "$base/pair-119.json")
execution=$(jq -s 'all(.[]; .result.execution_valid == true)' "$base/pair-117.json" "$base/pair-119.json")
basis=$(jq -s 'all(.[]; .result.basis_measurable == true)' "$base/pair-117.json" "$base/pair-119.json")
if [[ "$pairs_valid" != true ]]; then classification="INVALID"
elif [[ "$no_eligible" == true ]]; then classification="NOT EXERCISED"
elif [[ "$activation" != true ]]; then classification="FALSIFIED AT ACTIVATION"
elif [[ "$execution" != true ]]; then classification="FALSIFIED AT EXECUTION"
elif [[ "$basis" != true ]]; then classification="NOT IDENTIFIED"
else classification="PENDING CROSS-SEED SIGN"; fi

cell_summary=$(jq -n \
  --slurpfile a117 "$base/A-117/datedcarryp5.json" --slurpfile b117 "$base/B-117/datedcarryp5.json" \
  --slurpfile a119 "$base/A-119/datedcarryp5.json" --slurpfile b119 "$base/B-119/datedcarryp5.json" \
  --slurpfile p117 "$base/pair-117.json" --slurpfile p119 "$base/pair-119.json" \
  '[{seed:117,control:$a117[0].result,treatment:$b117[0].result,pair:$p117[0].result},{seed:119,control:$a119[0].result,treatment:$b119[0].result,pair:$p119[0].result}]')

jq -n --arg classification "$classification" --argjson pairs_valid "$pairs_valid" --argjson no_eligible "$no_eligible" --argjson activation "$activation" --argjson execution "$execution" --argjson basis "$basis" --argjson cells "$cell_summary" '{protocol:"v2-5-p5-dated-carry-v1",classification:$classification,development_seeds:[117,119],untouched_holdout_seeds:[139,149,151],holdouts_consumed:false,pair_contracts_valid:$pairs_valid,no_exact_cost_eligible_treatment:$no_eligible,activation_valid:$activation,execution_valid:$execution,basis_measurable:$basis,cells:$cells,interpretation:"No exact-cost eligible treatment candidate appeared in either paired development seed; no target, order, fill, or basis endpoint is licensed.",next_action:"Do not tune P5 numbers or consume holdouts."}' > "$output"
echo "scored P5 development: $classification"
