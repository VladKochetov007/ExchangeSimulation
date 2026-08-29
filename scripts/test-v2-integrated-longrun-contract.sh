#!/usr/bin/env bash
# Cheap, non-simulation tests for the integrated long-run control plane.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf "$tmp_root"' EXIT

fail() {
	echo "integrated long-run contract test failure: $*" >&2
	exit 1
}

expect_failure() {
	if "$@" >/dev/null 2>&1; then
		fail "command unexpectedly succeeded: $*"
	fi
}

"$root_dir/scripts/check-v2-integrated-longrun-configs.sh" >/dev/null

left_manifest_dir="$tmp_root/ordered-left"
right_manifest_dir="$tmp_root/ordered-right"
mkdir -p "$left_manifest_dir" "$right_manifest_dir"
printf '%s\n' '{"raw_files":[{"path":"venues/a.jsonl","bytes":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"path":"venues/b.jsonl","bytes":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}' >"$left_manifest_dir/evidence-manifest.json"
cp "$left_manifest_dir/evidence-manifest.json" "$right_manifest_dir/evidence-manifest.json"
source "$root_dir/scripts/v2-integrated-longrun-r5-contract.sh"
v2_r5_compare_ordered_raw_manifests "$left_manifest_dir" "$right_manifest_dir" || fail "identical ordered manifests did not compare equal"
jq '.raw_files |= reverse' "$right_manifest_dir/evidence-manifest.json" >"$right_manifest_dir/permuted.json"
mv "$right_manifest_dir/permuted.json" "$right_manifest_dir/evidence-manifest.json"
if v2_r5_compare_ordered_raw_manifests "$left_manifest_dir" "$right_manifest_dir"; then
	fail "permuted raw manifest was accepted as equal"
fi

expect_failure env GOMAXPROCS=4 V2_LONGRUN_OUTPUT_ROOT="$tmp_root/holdout" \
	"$root_dir/scripts/run-v2-integrated-longrun-cell.sh" holdout-619 /bin/true
expect_failure env GOMAXPROCS=4 \
	"$root_dir/scripts/run-v2-integrated-longrun-cell.sh" holdout-619 /bin/true
expect_failure "$root_dir/scripts/extract-v2-integrated-longrun-cell.sh" \
		"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/extract-v2-integrated-longrun-cell.sh" \
	"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4/../../../../../../home/vlad/v2-integrated-longrun-candidate-20260828-v5/holdout-619"
expect_failure "$root_dir/scripts/verify-v2-integrated-longrun-cell.sh" \
	"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/check-v2-integrated-longrun-parity.sh" "$tmp_root/parity"
expect_failure "$root_dir/scripts/score-v2-integrated-longrun-development.sh" "$tmp_root/score"

echo "integrated long-run contract tests: pass"
