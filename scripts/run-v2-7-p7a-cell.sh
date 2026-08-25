#!/usr/bin/env bash
# Run one immutable V2-7 P7a development cell. Only final greeks.json and
# latency.json make the world complete; a process exit or partial directory is
# never evidence.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 C|L|H 307|311 [multivenue-binary]" >&2
	exit 2
fi
cell=$1
seed=$2
case "$cell" in C|L|H) ;; *) echo "unknown P7 cell: $cell" >&2; exit 2 ;; esac
case "$seed" in 307|311) ;; *) echo "unregistered P7 seed: $seed" >&2; exit 2 ;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-7-p7/$cell-$seed.json"
artifact_root=${P7_OUTPUT_ROOT:-"$root_dir/research/artifacts/v2-7-p7a"}
output="$artifact_root/$cell-$seed"
binary=${3:-"$root_dir/bin/multivenue"}

if [[ ! -s "$config" || ! -x "$binary" ]]; then
	echo "missing P7 config or executable: $config / $binary" >&2
	exit 1
fi
if [[ -e "$output" ]]; then
	echo "refusing to overwrite P7 evidence directory: $output" >&2
	exit 1
fi
"$root_dir/scripts/check-v2-7-p7-configs.sh" >/dev/null

horizon=4h
if [[ "${P7_PREFLIGHT:-0}" == 1 ]]; then
	horizon=15m
fi

mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg cell "$cell" \
	--argjson seed "$seed" \
	--arg horizon "$horizon" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	--arg mode "${P7_PREFLIGHT:-0}" \
	'{
	  experiment_id: ("v2-7-p7a-distress-" + $cell + "-seed-" + ($seed|tostring)),
	  hypothesis_id: "V2-7-P7A-DISTRESS",
	  cell: $cell, seed: $seed, simulated_horizon: $horizon,
	  preflight: ($mode == "1"),
	  config_sha256: $config_sha256, binary_sha256: $binary_sha256,
	  git_revision: $git_revision, go_version: $go_version,
	  gomaxprocs: $gomaxprocs, output_dir: $output_dir,
	  command: ["multivenue", "-config", "run-config.json", "-duration", $horizon, "-log-mode", "full"],
	  completion_sentinels: ["greeks.json", "latency.json"],
	  raw_log_policy: "retain until every registered P7a evidence contract passes"
	}' >"$output/run-metadata.json"

"$binary" -config "$config" -duration "$horizon" -logdir "$output" -log-mode full

if [[ ! -s "$output/greeks.json" || ! -s "$output/latency.json" ]]; then
	echo "P7 process exited without both completion sentinels: $output" >&2
	exit 1
fi
echo "completed P7 cell: $output"
