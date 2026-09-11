#!/usr/bin/env bash
# Audit one registered SV1D treatment arm through the production renderer and
# the strict CDF activation library. This adapter never changes a trajectory;
# it fails closed when the immutable evidence contract is not reconstructible.
set -euo pipefail

if [[ $# -ne 6 ]]; then
	echo "usage: $0 <run-dir> <audit-output.json> <expected-provenance.json> <simulator-binary> <mvanalyze-binary> <evsrender-binary>" >&2
	exit 2
fi

run_dir=$1
audit_output=$2
expected_provenance=$3
simulator_binary=$4
analyzer=$5
renderer=$6

[[ -d "$run_dir" && -s "$run_dir/events.evs" ]] || {
	echo "SV1D audit: missing binary evidence run directory or events.evs" >&2
	exit 1
}
[[ -f "$expected_provenance" && ! -L "$expected_provenance" && -x "$renderer" && ! -L "$renderer" &&
	-x "$analyzer" && ! -L "$analyzer" && -f "$simulator_binary" && ! -L "$simulator_binary" ]] || {
	echo "SV1D audit: missing provenance, renderer, analyzer, or simulator binary" >&2
	exit 1
}

[[ -s "$run_dir/run-config.json" && ! -L "$run_dir/run-config.json" ]] || {
	echo "SV1D audit: missing run-config.json" >&2
	exit 1
}
jq -e -s '
	length == 1 and (.[0] |
	.schema_version == 1 and
	.contract == "v2-r2-sv1d-audit-provenance-v1" and
	(.probe_id | type == "string" and length > 0) and
	(.source_revision | test("^[0-9a-f]{40}$")) and
	(.config_sha256 | test("^[0-9a-f]{64}$")) and
	(.simulator_binary_sha256 | test("^[0-9a-f]{64}$")) and
	(.analyzer_sha256 | test("^[0-9a-f]{64}$")) and
	(.renderer_sha256 | test("^[0-9a-f]{64}$")) and
	(.simulator_vcs_revision | test("^[0-9a-f]{40}$")) and
	(.analyzer_vcs_revision | test("^[0-9a-f]{40}$")) and
	(.renderer_vcs_revision | test("^[0-9a-f]{40}$")) and
	.simulator_vcs_revision == .source_revision and
	.analyzer_vcs_revision == .source_revision and
	.renderer_vcs_revision == .source_revision and
	.simulator_vcs_modified == false and .analyzer_vcs_modified == false and .renderer_vcs_modified == false and
	.simulator_trimpath == true and .analyzer_trimpath == true and .renderer_trimpath == true and
	.simulator_cgo_enabled == "0" and .analyzer_cgo_enabled == "0" and .renderer_cgo_enabled == "0" and
	(.simulator_go_version | startswith("go1.27")) and
	(.analyzer_go_version | startswith("go1.27")) and
	(.renderer_go_version | startswith("go1.27")) and
	.binary_goos == "linux" and .binary_goarch == "amd64" and .binary_goamd64 == "v1" and
	.simulator_goos == "linux" and .simulator_goarch == "amd64" and .simulator_goamd64 == "v1" and
	.analyzer_goos == "linux" and .analyzer_goarch == "amd64" and .analyzer_goamd64 == "v1" and
	.renderer_goos == "linux" and .renderer_goarch == "amd64" and .renderer_goamd64 == "v1")' \
	"$expected_provenance" >/dev/null || {
	echo "SV1D audit: expected provenance document is structurally invalid" >&2
	exit 1
}

expected_probe_id=$(jq -er '.probe_id' "$expected_provenance")
expected_source_revision=$(jq -er '.source_revision' "$expected_provenance")
expected_config_sha256=$(jq -er '.config_sha256' "$expected_provenance")
expected_simulator_sha256=$(jq -er '.simulator_binary_sha256' "$expected_provenance")
expected_analyzer_sha256=$(jq -er '.analyzer_sha256' "$expected_provenance")
expected_renderer_sha256=$(jq -er '.renderer_sha256' "$expected_provenance")
expected_binary_goos=$(jq -er '.binary_goos' "$expected_provenance")
expected_binary_goarch=$(jq -er '.binary_goarch' "$expected_provenance")
expected_binary_goamd64=$(jq -er '.binary_goamd64' "$expected_provenance")
actual_config_sha256=$(sha256sum -- "$run_dir/run-config.json" | awk '{print $1}')
actual_simulator_sha256=$(sha256sum -- "$simulator_binary" | awk '{print $1}')
actual_analyzer_sha256=$(sha256sum -- "$analyzer" | awk '{print $1}')
actual_renderer_sha256=$(sha256sum -- "$renderer" | awk '{print $1}')
[[ "$actual_config_sha256" == "$expected_config_sha256" &&
	"$actual_simulator_sha256" == "$expected_simulator_sha256" &&
	"$actual_analyzer_sha256" == "$expected_analyzer_sha256" &&
	"$actual_renderer_sha256" == "$expected_renderer_sha256" ]] || {
	echo "SV1D audit: supplied artifacts do not match external provenance" >&2
	exit 1
}

build_field() {
	local binary=$1 field=$2
	go version -m "$binary" | awk -v field="$field" '$1 == "build" && index($2, field) == 1 {sub(field, "", $2); print $2; exit}'
}
simulator_vcs_revision=$(build_field "$simulator_binary" 'vcs.revision=')
simulator_vcs_modified=$(build_field "$simulator_binary" 'vcs.modified=')
simulator_trimpath=$(build_field "$simulator_binary" '-trimpath=')
simulator_cgo_enabled=$(build_field "$simulator_binary" 'CGO_ENABLED=')
simulator_go_version=$(go version -m "$simulator_binary" | awk '$1 == "go" {print $2; exit}')
simulator_goos=$(build_field "$simulator_binary" 'GOOS=')
simulator_goarch=$(build_field "$simulator_binary" 'GOARCH=')
simulator_goamd64=$(build_field "$simulator_binary" 'GOAMD64=')
analyzer_vcs_revision=$(build_field "$analyzer" 'vcs.revision=')
analyzer_vcs_modified=$(build_field "$analyzer" 'vcs.modified=')
analyzer_trimpath=$(build_field "$analyzer" '-trimpath=')
analyzer_cgo_enabled=$(build_field "$analyzer" 'CGO_ENABLED=')
analyzer_go_version=$(go version -m "$analyzer" | awk '$1 == "go" {print $2; exit}')
analyzer_goos=$(build_field "$analyzer" 'GOOS=')
analyzer_goarch=$(build_field "$analyzer" 'GOARCH=')
analyzer_goamd64=$(build_field "$analyzer" 'GOAMD64=')
renderer_vcs_revision=$(build_field "$renderer" 'vcs.revision=')
renderer_vcs_modified=$(build_field "$renderer" 'vcs.modified=')
renderer_trimpath=$(build_field "$renderer" '-trimpath=')
renderer_cgo_enabled=$(build_field "$renderer" 'CGO_ENABLED=')
renderer_go_version=$(go version -m "$renderer" | awk '$1 == "go" {print $2; exit}')
renderer_goos=$(build_field "$renderer" 'GOOS=')
renderer_goarch=$(build_field "$renderer" 'GOARCH=')
renderer_goamd64=$(build_field "$renderer" 'GOAMD64=')
jq -e --arg source_revision "$expected_source_revision" \
	--arg simulator_revision "$simulator_vcs_revision" --arg simulator_modified "$simulator_vcs_modified" \
	--arg simulator_trimpath "$simulator_trimpath" --arg simulator_cgo "$simulator_cgo_enabled" --arg simulator_go "$simulator_go_version" \
	--arg simulator_goos "$simulator_goos" --arg simulator_goarch "$simulator_goarch" --arg simulator_goamd64 "$simulator_goamd64" \
	--arg analyzer_revision "$analyzer_vcs_revision" --arg analyzer_modified "$analyzer_vcs_modified" \
	--arg analyzer_trimpath "$analyzer_trimpath" --arg analyzer_cgo "$analyzer_cgo_enabled" --arg analyzer_go "$analyzer_go_version" \
	--arg analyzer_goos "$analyzer_goos" --arg analyzer_goarch "$analyzer_goarch" --arg analyzer_goamd64 "$analyzer_goamd64" \
	--arg renderer_revision "$renderer_vcs_revision" --arg renderer_modified "$renderer_vcs_modified" \
	--arg renderer_trimpath "$renderer_trimpath" --arg renderer_cgo "$renderer_cgo_enabled" --arg renderer_go "$renderer_go_version" \
	--arg renderer_goos "$renderer_goos" --arg renderer_goarch "$renderer_goarch" --arg renderer_goamd64 "$renderer_goamd64" \
	'.source_revision == $source_revision and
	 .simulator_vcs_revision == $simulator_revision and .simulator_vcs_modified == ($simulator_modified == "false") and
	 .simulator_trimpath == ($simulator_trimpath == "true") and .simulator_cgo_enabled == $simulator_cgo and .simulator_go_version == $simulator_go and
	 .simulator_goos == $simulator_goos and .simulator_goarch == $simulator_goarch and .simulator_goamd64 == $simulator_goamd64 and
	 .analyzer_vcs_revision == $analyzer_revision and .analyzer_vcs_modified == ($analyzer_modified == "false") and
	 .analyzer_trimpath == ($analyzer_trimpath == "true") and .analyzer_cgo_enabled == $analyzer_cgo and .analyzer_go_version == $analyzer_go and
	 .analyzer_goos == $analyzer_goos and .analyzer_goarch == $analyzer_goarch and .analyzer_goamd64 == $analyzer_goamd64 and
	 .renderer_vcs_revision == $renderer_revision and .renderer_vcs_modified == ($renderer_modified == "false") and
	 .renderer_trimpath == ($renderer_trimpath == "true") and .renderer_cgo_enabled == $renderer_cgo and .renderer_go_version == $renderer_go and
	 .renderer_goos == $renderer_goos and .renderer_goarch == $renderer_goarch and .renderer_goamd64 == $renderer_goamd64' \
	"$expected_provenance" >/dev/null || {
	echo "SV1D audit: supplied binaries do not match external build provenance" >&2
	exit 1
}

mkdir -p -- "$(dirname -- "$audit_output")"
rendered_dir=$(mktemp -d)
temporary_output=$(mktemp "${audit_output}.tmp-XXXXXX")
validated_output=$(mktemp "${audit_output}.validated-XXXXXX")
cleanup() {
	rm -rf -- "$rendered_dir"
	rm -f -- "$temporary_output" "$validated_output"
}
trap cleanup EXIT

"$renderer" -dir "$run_dir" -out "$rendered_dir" >/dev/null
if ! "$analyzer" -metric cdfactivation -json \
	-cdf-rendered-evidence-dir "$rendered_dir" \
	-cdf-config-sha256 "$expected_config_sha256" \
	-cdf-source-revision "$expected_source_revision" \
	-cdf-binary-sha256 "$expected_simulator_sha256" \
	-cdf-binary-goos "$expected_binary_goos" \
	-cdf-binary-goarch "$expected_binary_goarch" \
	-cdf-binary-goamd64 "$expected_binary_goamd64" \
	"$run_dir" >"$temporary_output"; then
	mv -- "$temporary_output" "$audit_output"
	exit 1
fi

jq -e -s --arg config_sha256 "$expected_config_sha256" --arg source_revision "$expected_source_revision" \
	--arg binary_sha256 "$expected_simulator_sha256" \
	'length == 1 and (.[0] | type == "object" and (.result | type == "object") and
	 .result.valid == true and .result.evidence_valid == true and
	 .result.provenance.config_sha256 == $config_sha256 and
	 .result.provenance.source_revision == $source_revision and
	 .result.provenance.binary_sha256 == $binary_sha256)' \
	"$temporary_output" >/dev/null || {
	mv -- "$temporary_output" "$audit_output"
	echo "SV1D audit: strict evidence contract failed" >&2
	exit 1
}
jq --slurpfile provenance "$expected_provenance" --arg probe_id "$expected_probe_id" \
	'. + {audit_provenance: $provenance[0], probe_id: $probe_id}' \
	"$temporary_output" >"$validated_output" || {
	mv -- "$temporary_output" "$audit_output"
	echo "SV1D audit: could not bind external probe provenance" >&2
	exit 1
}
mv -- "$validated_output" "$audit_output"
