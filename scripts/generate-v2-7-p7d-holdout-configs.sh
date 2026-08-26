#!/usr/bin/env bash
# Materialise only the preregistered untouched P7d holdout configs. Values
# are copied from the hash-pinned development configs; only seed and explicit
# holdout provenance text vary.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7d"
score="$root_dir/research/artifacts/v2-7-p7d/p7d-development-score.json"
holdout_seeds=(439 443 449)

"$root_dir/scripts/check-v2-7-p7d-configs.sh" >/dev/null
[[ -s "$score" ]] || { echo "missing P7d development score: $score" >&2; exit 1; }
jq -e '.classification == "SUPPORTED (screening)" and
  .holdouts_consumed == false and
  .predicates.activation_valid == true and
  .predicates.participant_risk_replay_valid == true and
  .predicates.long_risk_exercised == true and
  .predicates.short_risk_exercised == true' "$score" >/dev/null || {
  echo "P7d development promotion rule is not satisfied" >&2
  exit 1
}

for seed in "${holdout_seeds[@]}"; do
  for cell in C L S; do
    source="$config_dir/$cell-431.json"
    target="$config_dir/$cell-$seed.json"
    [[ -s "$source" ]] || { echo "missing development source config: $source" >&2; exit 1; }
    if [[ -e "$target" ]]; then
      echo "refusing to overwrite immutable holdout config: $target" >&2
      exit 1
    fi
    jq --arg cell "$cell" --argjson seed "$seed" '
      .seed = $seed
      | .experiment_id = ("v2-7-p7d-directional-distress-" + $cell + "-seed-" + ($seed|tostring))
      | .description = ("P7d finite-capital fixed-directional distress screen " + $cell + "; untouched holdout; no environment or policy delta.")
    ' "$source" >"$target"
  done
done

echo "generated P7d holdout configs in $config_dir"
