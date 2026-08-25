#!/usr/bin/env bash
# Run one immutable V2-5 P5 cell. Final greeks.json and latency.json are the
# only completion sentinels; process exit and partial evidence are not enough.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 A|B 117|119|139|149|151 [multivenue-binary]" >&2
	exit 2
fi
arm=$1
seed=$2
case "$arm" in A|B) ;; *) echo "unknown P5 arm: $arm" >&2; exit 2 ;; esac
case "$seed" in 117|119|139|149|151) ;; *) echo "unregistered P5 seed: $seed" >&2; exit 2 ;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-5-p5/$arm-$seed.json"
output="$root_dir/research/artifacts/v2-5-p5/$arm-$seed"
binary=${3:-"$root_dir/bin/multivenue"}

if [[ ! -s "$config" || ! -x "$binary" ]]; then
	echo "missing P5 config or executable" >&2
	exit 1
fi
if [[ -e "$output" ]]; then
	echo "refusing to overwrite P5 evidence directory: $output" >&2
	exit 1
fi
"$root_dir/scripts/check-v2-5-p5-configs.sh" >/dev/null

mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg arm "$arm" \
	--argjson seed "$seed" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	'{
	  experiment_id: ("v2-5-p5-dated-carry-" + $arm + "-seed-" + ($seed|tostring)),
	  hypothesis_id: "V2-5-P5-DATED-CARRY",
	  arm: $arm, seed: $seed, simulated_horizon: "26h0m0s",
	  config_sha256: $config_sha256, binary_sha256: $binary_sha256,
	  git_revision: $git_revision, go_version: $go_version,
	  gomaxprocs: $gomaxprocs, output_dir: $output_dir,
	  command: ["multivenue", "-config", "run-config.json", "-duration", "26h", "-log-mode", "full"],
	  completion_sentinels: ["greeks.json", "latency.json"],
	  raw_log_policy: "retain until every P5 cell and paired evidence contract passes"
	}' >"$output/run-metadata.json"

"$binary" -config "$config" -duration 26h -logdir "$output" -log-mode full

if [[ ! -s "$output/greeks.json" || ! -s "$output/latency.json" ]]; then
	echo "P5 process exited without both completion sentinels: $output" >&2
	exit 1
fi
echo "completed P5 cell: $output"
