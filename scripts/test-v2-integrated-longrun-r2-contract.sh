#!/usr/bin/env bash
# Cheap control-plane tests for the R2 successor. These tests never launch a
# simulation and never inspect a reserved holdout artifact.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf -- "$tmp_root"' EXIT

fail() {
	printf 'integrated long-run R2 contract test failure: %s\n' "$*" >&2
	exit 1
}

expect_failure() {
	if "$@" >/dev/null 2>&1; then
		fail "command unexpectedly succeeded: $*"
	fi
}

"$root_dir/scripts/check-v2-integrated-longrun-r2-configs.sh" >/dev/null
source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

left_manifest_dir="$tmp_root/ordered-left"
right_manifest_dir="$tmp_root/ordered-right"
mkdir -p "$left_manifest_dir" "$right_manifest_dir"
printf '%s\n' '{"raw_files":[{"path":"venues/a.jsonl","bytes":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"path":"venues/b.jsonl","bytes":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}' >"$left_manifest_dir/evidence-manifest.json"
cp "$left_manifest_dir/evidence-manifest.json" "$right_manifest_dir/evidence-manifest.json"
v2_r2_compare_ordered_raw_manifests "$left_manifest_dir" "$right_manifest_dir" || fail "identical ordered manifests did not compare equal"
jq '.raw_files |= reverse' "$right_manifest_dir/evidence-manifest.json" >"$right_manifest_dir/permuted.json"
mv "$right_manifest_dir/permuted.json" "$right_manifest_dir/evidence-manifest.json"
expect_failure v2_r2_compare_ordered_raw_manifests "$left_manifest_dir" "$right_manifest_dir"

expect_failure env GOMAXPROCS=4 "$root_dir/scripts/run-v2-integrated-longrun-r2-cell.sh" holdout-619 /bin/true
expect_failure "$root_dir/scripts/extract-v2-integrated-longrun-r2-cell.sh" \
	"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/verify-v2-integrated-longrun-r2-cell.sh" \
	"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/check-v2-integrated-longrun-r2-parity.sh" "$tmp_root/parity"
expect_failure "$root_dir/scripts/score-v2-integrated-longrun-r2-development.sh" "$tmp_root/score"

printf 'integrated long-run R2 contract tests: pass\n'
