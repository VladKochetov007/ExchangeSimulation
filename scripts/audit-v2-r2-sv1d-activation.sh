#!/usr/bin/env bash
# Audit one registered SV1D treatment arm through the production renderer and
# the strict CDF activation library. This adapter never changes a trajectory;
# it fails closed when the immutable evidence contract is not reconstructible.
set -euo pipefail

if [[ $# -ne 5 ]]; then
	echo "usage: $0 <run-dir> <audit-output.json> <source-revision> <simulator-binary> <mvanalyze-binary>" >&2
	exit 2
fi

run_dir=$1
audit_output=$2
source_revision=$3
simulator_binary=$4
analyzer=$5
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
renderer=${EVSRENDER_BIN:-"$root_dir/bin/evsrender"}

[[ -d "$run_dir" && -s "$run_dir/events.evs" ]] || {
	echo "SV1D audit: missing binary evidence run directory or events.evs" >&2
	exit 1
}
[[ -x "$renderer" && -x "$analyzer" && -f "$simulator_binary" ]] || {
	echo "SV1D audit: missing renderer, analyzer, or simulator binary" >&2
	exit 1
}

config_sha256=$(sha256sum -- "$run_dir/run-config.json" | awk '{print $1}')
binary_sha256=$(sha256sum -- "$simulator_binary" | awk '{print $1}')
mkdir -p -- "$(dirname -- "$audit_output")"
rendered_dir=$(mktemp -d)
temporary_output=$(mktemp "${audit_output}.tmp-XXXXXX")
cleanup() {
	rm -rf -- "$rendered_dir"
	rm -f -- "$temporary_output"
}
trap cleanup EXIT

"$renderer" -dir "$run_dir" -out "$rendered_dir" >/dev/null
if ! "$analyzer" -metric cdfactivation -json \
	-cdf-rendered-evidence-dir "$rendered_dir" \
	-cdf-config-sha256 "$config_sha256" \
	-cdf-source-revision "$source_revision" \
	-cdf-binary-sha256 "$binary_sha256" \
	"$run_dir" >"$temporary_output"; then
	mv -- "$temporary_output" "$audit_output"
	exit 1
fi

jq -e 'type == "object" and (.result | type == "object") and (.result.evidence_valid == true)' \
	"$temporary_output" >/dev/null || {
	mv -- "$temporary_output" "$audit_output"
	echo "SV1D audit: strict evidence contract failed" >&2
	exit 1
}
mv -- "$temporary_output" "$audit_output"
