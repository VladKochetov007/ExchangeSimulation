#!/usr/bin/env bash
# Run one pre-registered SV1 24-hour development cell. Completion is
# fail-closed: final sidecars, manifest identity, and an atomic run-status are
# all required. This namespace has no holdout selector.
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
	echo "usage: $0 treatment-607|treatment-613|treatment-617|control-607|control-613|control-617|treatment-607-g8|control-607-none [multivenue-binary] [prunegate-binary]" >&2
	exit 2
fi
cell=$1
case "$cell" in
	treatment-607|treatment-613|treatment-617|control-607|control-613|control-617)
		config_name="$cell.json"
		expected_gomaxprocs=4
		;;
	control-607-none)
		config_name="control-607-none.json"
		expected_gomaxprocs=4
		;;
	treatment-607-g8)
		config_name="treatment-607.json"
		expected_gomaxprocs=8
		;;
	holdout-619|holdout-631|holdout-641)
		echo "refusing reserved holdout in development runner; use a separately pinned post-freeze protocol" >&2
		exit 1
		;;
	*) echo "unregistered SV1 long-run cell: $cell" >&2; exit 2 ;;
esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
[[ -z "${EXSIM_BINARY_EVIDENCE:-}" ]] || {
	echo "refusing registered launch with prototype EXSIM_BINARY_EVIDENCE override" >&2
	exit 1
}
v2_r2_acquire_namespace_lock || {
	echo "could not acquire the R2 evidence namespace lock" >&2
	exit 1
}
config="$root_dir/research/configs/v2-r2-sv1-24h/$config_name"
config_provenance_manifest="$root_dir/research/v2-r2-sv1-24h-config-provenance.json"
output_root="$v2_r2_output_root"
output="$output_root/$cell"
binary=${2:-"$root_dir/bin/multivenue"}
prunegate_binary=${3:-"$root_dir/bin/prunegate"}
sim_revision=${V2_R2_SV1_SIMULATOR_REVISION:-$(git -C "$root_dir" rev-parse HEAD)}
horizon=24h
simulation_start_nano=1735689600000000000
simulation_end_nano=1735776000000000000
head_revision=$(git -C "$root_dir" rev-parse HEAD)

[[ -s "$config" && -s "$config_provenance_manifest" && -x "$binary" && -x "$prunegate_binary" ]] || {
	echo "missing SV1 long-run config or executable: $config / $binary / $prunegate_binary" >&2
	exit 1
}
[[ "$sim_revision" =~ ^[0-9a-f]{40}$ ]] || {
	echo "invalid V2_R2_SV1_SIMULATOR_REVISION: $sim_revision" >&2
	exit 1
}
[[ "$sim_revision" == "$head_revision" ]] || {
	echo "simulator revision $sim_revision is not current repository HEAD $head_revision" >&2
	exit 1
}
v2_r2_require_output_root "$output_root" || {
	echo "refusing non-canonical or symlinked SV1 evidence root: $output_root" >&2
	exit 1
}
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "refusing SV1 launch from a dirty source worktree; build and run from a clean worktree" >&2
	exit 1
}
[[ "${GOMAXPROCS:-}" == "$expected_gomaxprocs" ]] || {
	echo "registered cell $cell requires GOMAXPROCS=$expected_gomaxprocs (got ${GOMAXPROCS:-unset})" >&2
	exit 1
}
"$root_dir/scripts/check-v2-r2-sv1-24h-configs.sh" >/dev/null
[[ ! -e "$output" ]] || {
	echo "refusing to overwrite SV1 evidence directory: $output" >&2
	exit 1
}

# The simulator intentionally accepts only the two pre-run provenance files in
# a fresh output directory. Keep process stdout/stderr beside (not inside) the
# cell until NewSim has opened its evidence sink; creating log files inside the
# cell before launch would make the directory look reused and fail closed.
mkdir -p "$output_root"
binary_revision=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
binary_trimpath=$(go version -m "$binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
binary_cgo_enabled=$(go version -m "$binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
binary_goos=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOOS=") == 1 {sub("GOOS=", "", $2); print $2; exit}')
binary_goarch=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOARCH=") == 1 {sub("GOARCH=", "", $2); print $2; exit}')
binary_goamd64=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOAMD64=") == 1 {sub("GOAMD64=", "", $2); print $2; exit}')
binary_go_version=$(v2_r2_binary_go_version "$binary")
prunegate_revision=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
prunegate_modified=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
prunegate_trimpath=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
prunegate_cgo_enabled=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
prunegate_go_version=$(v2_r2_binary_go_version "$prunegate_binary")
[[ "$binary_revision" == "$sim_revision" ]] || {
	echo "binary VCS revision $binary_revision does not match requested $sim_revision" >&2
	exit 1
}
[[ "$binary_modified" == "false" ]] || {
	echo "binary is not a clean VCS build (vcs.modified=$binary_modified)" >&2
	exit 1
}
[[ "$binary_trimpath" == "true" && "$binary_cgo_enabled" == "0" && "$binary_goos" == "linux" && "$binary_goarch" == "amd64" && "$binary_goamd64" == "v1" ]] || {
	echo "binary reproducibility settings are not enforced (-trimpath=$binary_trimpath CGO_ENABLED=$binary_cgo_enabled target=$binary_goos/$binary_goarch/$binary_goamd64)" >&2
	exit 1
}
v2_r2_is_go_127 "$binary_go_version" || {
	echo "binary is not built with the pinned Go 1.27 toolchain (got $binary_go_version)" >&2
	exit 1
}
[[ "$prunegate_revision" == "$sim_revision" && "$prunegate_modified" == "false" &&
	"$prunegate_trimpath" == "true" && "$prunegate_cgo_enabled" == "0" ]] || {
	echo "prunegate is not a clean reproducible build of current HEAD" >&2
	exit 1
}
v2_r2_is_go_127 "$prunegate_go_version" || {
	echo "prunegate is not built with the pinned Go 1.27 toolchain (got $prunegate_go_version)" >&2
	exit 1
}

log_mode=$(jq -er '.log_mode' "$config")
evidence_format=$(jq -er '.evidence_format' "$config")
config_sha256=$(sha256sum "$config" | awk '{print $1}')
config_provenance_sha256=$(sha256sum "$config_provenance_manifest" | awk '{print $1}')
[[ "$evidence_format" == "evstream_v3" ]] || {
	echo "registered successor cell requires evstream_v3 evidence (got $evidence_format)" >&2
	exit 1
}
v2_r2_require_binary_capacity_attestation "$binary" "$sim_revision" "" "$config_sha256" 4 || {
	echo "refusing long-run launch without a matching measured binary-evidence capacity attestation" >&2
	exit 1
}
seed=$(jq -er '.seed' "$config")
config_hypothesis=$(jq -er '.hypothesis_id' "$config")
config_experiment=$(jq -er '.experiment_id' "$config")
holdout=false
[[ "$cell" == holdout-* ]] && holdout=true
mkdir -p "$output"
"$binary" -config "$config" -logdir "$output" -log-mode "$log_mode" -evidence-format "$evidence_format" -write-effective-config "$output/run-config.json"
cmp -s "$config" "$output/run-config.json" || {
	echo "registered config is not already the simulator's normalized effective config: $cell" >&2
	exit 1
}
binary_sha256=$(sha256sum "$binary" | awk '{print $1}')
jq -n \
	--arg cell "$cell" \
	--argjson seed "$seed" \
	--arg horizon "$horizon" \
	--argjson simulation_start_nano "$simulation_start_nano" \
	--argjson simulation_end_nano "$simulation_end_nano" \
	--arg log_mode "$log_mode" \
	--arg evidence_format "$evidence_format" \
	--arg config_sha256 "$config_sha256" \
	--arg config_provenance_manifest "$config_provenance_manifest" \
	--arg config_provenance_sha256 "$config_provenance_sha256" \
	--arg binary_sha256 "$binary_sha256" \
	--arg git_revision "$sim_revision" \
	--arg go_version "$(go version)" \
	--arg binary_go_version "$binary_go_version" \
	--arg prunegate_path "$prunegate_binary" \
	--arg prunegate_sha256 "$(sha256sum "$prunegate_binary" | awk '{print $1}')" \
	--arg prunegate_vcs_revision "$prunegate_revision" \
	--arg prunegate_vcs_modified "$prunegate_modified" \
	--arg prunegate_trimpath "$prunegate_trimpath" \
	--arg prunegate_cgo_enabled "$prunegate_cgo_enabled" \
	--arg prunegate_go_version "$prunegate_go_version" \
	--argjson gomaxprocs "$GOMAXPROCS" \
	--arg output_dir "$output" \
	--arg evidence_manifest_path "$output/evidence-manifest.json" \
	--arg external_attestation_path "$v2_r2_attestation_root/$cell.json" \
	--argjson holdout "$holdout" \
	--arg binary_path "$binary" \
	--arg config_path "$config" \
	--arg binary_vcs_revision "$binary_revision" \
	--arg binary_vcs_modified "$binary_modified" \
	--arg binary_trimpath "$binary_trimpath" \
	--arg binary_cgo_enabled "$binary_cgo_enabled" \
	--arg binary_goos "$binary_goos" --arg binary_goarch "$binary_goarch" --arg binary_goamd64 "$binary_goamd64" \
	--arg config_hypothesis "$config_hypothesis" \
	--arg config_experiment "$config_experiment" \
	'{
		  schema_version: 6, runner_contract: "v2-r2-sv1-24h-runner-v1",
		  experiment_id: ("v2-r2-sv1-24h-" + $cell),
		  config_experiment_id: $config_experiment,
		  hypothesis_id: $config_hypothesis,
		  cell: $cell, seed: $seed, holdout: $holdout,
		  simulated_horizon: $horizon, log_mode: $log_mode, evidence_format: $evidence_format,
		  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  config_sha256: $config_sha256, binary_sha256: $binary_sha256,
		  config_provenance_manifest: $config_provenance_manifest,
		  config_provenance_sha256: $config_provenance_sha256,
		  config_path: $config_path, binary_path: $binary_path,
		  prunegate_path: $prunegate_path, prunegate_sha256: $prunegate_sha256,
		  prunegate_vcs_revision: $prunegate_vcs_revision,
		  prunegate_vcs_modified: ($prunegate_vcs_modified == "true"),
		  prunegate_trimpath: ($prunegate_trimpath == "true"),
		  prunegate_cgo_enabled: $prunegate_cgo_enabled, prunegate_go_version: $prunegate_go_version,
		  binary_vcs_revision: $binary_vcs_revision, binary_vcs_modified: ($binary_vcs_modified == "true"),
		  binary_trimpath: ($binary_trimpath == "true"), binary_cgo_enabled: $binary_cgo_enabled,
		  git_revision: $git_revision, go_version: $go_version, binary_go_version: $binary_go_version,
		  binary_goos: $binary_goos, binary_goarch: $binary_goarch, binary_goamd64: $binary_goamd64,
		  gomaxprocs: $gomaxprocs, output_dir: $output_dir,
		  evidence_manifest_path: $evidence_manifest_path,
		  external_attestation_path: $external_attestation_path,
		  command: ["multivenue", "-config", "run-config.json", "-duration", $horizon, "-logdir", $output_dir, "-log-mode", $log_mode, "-evidence-format", $evidence_format],
		  completion_sentinels: ["greeks.json", "latency.json"],
		  raw_log_policy: "retained until every registered SV1 evidence contract passes"
		}' >"$output/run-metadata.json"
run_metadata_sha256_before=$(sha256sum "$output/run-metadata.json" | awk '{print $1}')

stdout_log="$output_root/$cell.simulator.stdout.log"
stderr_log="$output_root/$cell.simulator.stderr.log"
set +e
"$binary" -config "$output/run-config.json" -duration "$horizon" -logdir "$output" -log-mode "$log_mode" -evidence-format "$evidence_format" \
	>"$stdout_log" 2>"$stderr_log"
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
jq -e 'type == "object" and (.build.revision | type) == "string" and
	.build.revision == $revision and .build.modified == false and
	.config.seed == $seed and .config.log_mode == $log_mode and .config.evidence_format == $evidence_format and
	.config.experiment_id == $experiment' \
	--arg revision "$sim_revision" --argjson seed "$seed" --arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
	--arg experiment "$config_experiment" "$output/manifest.json" >/dev/null || {
	echo "manifest provenance/config identity mismatch: $output" >&2
	exit 1
}
jq -e 'type == "object"' "$output/greeks.json" >/dev/null || {
	echo "malformed greeks completion sentinel: $output" >&2
	exit 1
}
jq -e 'type == "object"' "$output/latency.json" >/dev/null || {
	echo "malformed latency completion sentinel: $output" >&2
	exit 1
}
jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
	'(.initial_accounts | type == "array" and length > 0 and
	 all(.[]; .account.timestamp == $simulation_start_nano)) and
	(.terminal_accounts | type == "array" and length > 0 and
	 all(.[]; .account.timestamp == $simulation_end_nano))' \
	"$output/greeks.json" >/dev/null || {
	echo "greeks report does not attest the registered 24-hour simulated horizon: $output" >&2
	exit 1
}
v2_r2_require_checkpoint_stream "$output/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano" || {
	echo "checkpoint stream does not attest the registered 24-hour horizon: $output" >&2
	exit 1
}
run_metadata_sha256_after=$(sha256sum "$output/run-metadata.json" | awk '{print $1}')
[[ "$run_metadata_sha256_before" == "$run_metadata_sha256_after" ]] || {
	echo "run metadata changed during simulation: $output" >&2
	exit 1
}
v2_r2_write_evidence_manifest "$output" || {
	echo "failed to write complete evidence manifest: $output" >&2
	exit 1
}
status_tmp="$output/run-status.json.tmp-$$"
jq -n \
	--argjson exit_status "$status" \
	--arg cell "$cell" \
	--arg horizon "$horizon" \
	--argjson simulation_start_nano "$simulation_start_nano" \
	--argjson simulation_end_nano "$simulation_end_nano" \
	--arg run_metadata_sha256 "$run_metadata_sha256_after" \
	--arg manifest_sha256 "$(sha256sum "$output/manifest.json" | awk '{print $1}')" \
	--arg greeks_sha256 "$(sha256sum "$output/greeks.json" | awk '{print $1}')" \
	--arg latency_sha256 "$(sha256sum "$output/latency.json" | awk '{print $1}')" \
	--arg checkpoints_sha256 "$(sha256sum "$output/checkpoints.jsonl" | awk '{print $1}')" \
	--arg evidence_manifest_sha256 "$(sha256sum "$output/evidence-manifest.json" | awk '{print $1}')" \
	--argjson sentinels '["greeks.json", "latency.json"]' \
	'{schema_version: 1, cell: $cell, exit_status: $exit_status,
	  completion_verified: true, simulated_horizon: $horizon,
	  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
	  completion_sentinels: $sentinels,
	  run_metadata_sha256: $run_metadata_sha256,
	  manifest_sha256: $manifest_sha256, greeks_sha256: $greeks_sha256,
	  latency_sha256: $latency_sha256, checkpoints_sha256: $checkpoints_sha256,
	  evidence_manifest_sha256: $evidence_manifest_sha256}' >"$status_tmp"
mv "$status_tmp" "$output/run-status.json"
v2_r2_write_attestation "$output" || {
	echo "failed to write external evidence attestation: $output" >&2
	exit 1
}
echo "completed SV1 24-hour cell: $output"
