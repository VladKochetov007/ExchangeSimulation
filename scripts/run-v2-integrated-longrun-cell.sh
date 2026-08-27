#!/usr/bin/env bash
# Run one pre-registered integrated V2 long-run candidate cell. Completion is
# fail-closed: only final greeks.json and latency.json sidecars are sentinels.
# No holdout may run before the immutable freeze policy authorizes it.
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
	echo "usage: $0 dev-607|dev-613|dev-617|dev-607-none [multivenue-binary]" >&2
	exit 2
fi
cell=$1
case "$cell" in
	dev-607|dev-613|dev-617|dev-607-none) ;;
	 holdout-619|holdout-631|holdout-641)
		[[ "${V2_LONGRUN_ALLOW_HOLDOUT:-0}" == 1 ]] || {
			echo "refusing integrated long-run holdout before immutable freeze" >&2
			exit 1
		}
		;;
	*) echo "unregistered integrated long-run cell: $cell" >&2; exit 2 ;;
esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-integrated-longrun/$cell.json"
output_root=${V2_LONGRUN_OUTPUT_ROOT:-"$root_dir/research/artifacts/v2-integrated-longrun/candidate"}
output="$output_root/$cell"
binary=${2:-"$root_dir/bin/multivenue"}
sim_revision=${V2_LONGRUN_SIMULATOR_REVISION:-$(git -C "$root_dir" rev-parse HEAD)}
horizon=${V2_LONGRUN_HORIZON:-24h}

[[ -s "$config" && -x "$binary" ]] || {
	echo "missing integrated long-run config or executable: $config / $binary" >&2
	exit 1
}
[[ "$sim_revision" =~ ^[0-9a-f]{40}$ ]] || {
	echo "invalid V2_LONGRUN_SIMULATOR_REVISION: $sim_revision" >&2
	exit 1
}
"$root_dir/scripts/check-v2-integrated-longrun-configs.sh" >/dev/null
[[ ! -e "$output" ]] || {
	echo "refusing to overwrite integrated long-run evidence directory: $output" >&2
	exit 1
}

log_mode=$(jq -er '.log_mode' "$config")
seed=$(jq -er '.seed' "$config")
holdout=false
[[ "$cell" == holdout-* ]] && holdout=true
mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg cell "$cell" \
	--argjson seed "$seed" \
	--arg horizon "$horizon" \
	--arg log_mode "$log_mode" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$sim_revision" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	--argjson holdout "$holdout" \
	'{
	  experiment_id: ("v2-integrated-longrun-" + $cell),
	  hypothesis_id: "V2-INTEGRATED-LONG-CANDIDATE",
	  cell: $cell, seed: $seed, holdout: $holdout,
	  simulated_horizon: $horizon, log_mode: $log_mode,
	  config_sha256: $config_sha256, binary_sha256: $binary_sha256,
	  git_revision: $git_revision, go_version: $go_version,
	  gomaxprocs: $gomaxprocs, output_dir: $output,
	  command: ["multivenue", "-config", "run-config.json", "-duration", $horizon, "-log-mode", $log_mode],
	  completion_sentinels: ["greeks.json", "latency.json"],
	  raw_log_policy: "retained until every registered integrated long-run evidence contract passes"
	}' >"$output/run-metadata.json"

set +e
"$binary" -config "$config" -duration "$horizon" -logdir "$output" -log-mode "$log_mode" \
	>"$output/simulator.stdout.log" 2>"$output/simulator.stderr.log"
status=$?
set -e
if [[ "$status" -ne 0 ]]; then
	echo "integrated long-run simulator failed with status $status: $output" >&2
	exit "$status"
fi
if [[ ! -s "$output/greeks.json" || ! -s "$output/latency.json" ]]; then
	echo "simulator exited without both completion sentinels: $output" >&2
	exit 1
fi
echo "completed integrated long-run cell: $output"
