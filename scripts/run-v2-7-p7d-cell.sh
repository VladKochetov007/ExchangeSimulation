#!/usr/bin/env bash
# Run one immutable V2-7 P7d cell. Completion requires both final report
# sentinels; a process exit or partial directory is never evidence.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "usage: $0 C|L|S 431|433|439|443|449 [multivenue-binary]" >&2
  exit 2
fi
cell=$1
seed=$2
case "$cell" in C|L|S) ;; *) echo "unknown P7d cell: $cell" >&2; exit 2 ;; esac
case "$seed" in
  431|433) holdout=false ;;
  439|443|449)
    holdout=true
    [[ "${P7D_ALLOW_HOLDOUT:-0}" == 1 ]] || {
      echo "refusing P7d holdout before development promotion: set P7D_ALLOW_HOLDOUT=1" >&2
      exit 1
    }
    ;;
  *) echo "unregistered P7d seed: $seed" >&2; exit 2 ;;
esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-7-p7d/$cell-$seed.json"
artifact_root=${P7D_OUTPUT_ROOT:-"$root_dir/research/artifacts/v2-7-p7d/full"}
output="$artifact_root/$cell-$seed"
binary=${3:-"$root_dir/bin/multivenue"}

# Analyzer/evidence commits may advance HEAD without changing the simulator
# executable.  For a paired campaign, pin the simulator provenance explicitly
# when reusing that immutable binary; otherwise default to the current HEAD.
git_revision=${P7D_SIMULATOR_REVISION:-$(git -C "$root_dir" rev-parse HEAD)}
[[ "$git_revision" =~ ^[0-9a-f]{40}$ ]] || {
  echo "invalid P7D_SIMULATOR_REVISION: $git_revision" >&2
  exit 1
}

if [[ ! -s "$config" || ! -x "$binary" ]]; then
  echo "missing P7d config or executable: $config / $binary" >&2
  exit 1
fi
if [[ -e "$output" ]]; then
  echo "refusing to overwrite P7d evidence directory: $output" >&2
  exit 1
fi
if [[ "$holdout" == true ]]; then
  "$root_dir/scripts/check-v2-7-p7d-holdout-configs.sh" >/dev/null
else
  "$root_dir/scripts/check-v2-7-p7d-configs.sh" >/dev/null
fi

mkdir -p "$output"
cp "$config" "$output/run-config.json"
horizon=4h
preflight=false
if [[ "${P7D_PREFLIGHT:-0}" == 1 ]]; then
  horizon=15m
  preflight=true
fi
jq -n \
  --arg cell "$cell" \
  --argjson seed "$seed" \
  --arg horizon "$horizon" \
  --argjson preflight "$preflight" \
  --arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
  --arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
  --arg git_revision "$git_revision" \
  --arg go_version "$(go version)" \
  --arg gomaxprocs "${GOMAXPROCS:-default}" \
  --argjson holdout "$holdout" \
  --arg output_dir "$output" \
  '{
    experiment_id: ("v2-7-p7d-directional-distress-" + $cell + "-seed-" + ($seed|tostring)),
    hypothesis_id: "V2-7-P7D-DIRECTIONAL-DISTRESS",
    cell: $cell, seed: $seed, holdout: $holdout, simulated_horizon: $horizon, preflight: $preflight,
    config_sha256: $config_sha256, binary_sha256: $binary_sha256,
    git_revision: $git_revision, go_version: $go_version,
    gomaxprocs: $gomaxprocs, output_dir: $output_dir,
    command: ["multivenue", "-config", "run-config.json", "-duration", $horizon, "-log-mode", "full"],
    completion_sentinels: ["greeks.json", "latency.json"],
    raw_log_policy: "retain until every registered P7d evidence contract passes"
  }' >"$output/run-metadata.json"

"$binary" -config "$config" -duration "$horizon" -logdir "$output" -log-mode full

if [[ ! -s "$output/greeks.json" || ! -s "$output/latency.json" ]]; then
  echo "P7d process exited without both completion sentinels: $output" >&2
  exit 1
fi
echo "completed P7d cell: $output"
