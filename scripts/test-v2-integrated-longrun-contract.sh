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
expect_failure env GOMAXPROCS=4 V2_LONGRUN_OUTPUT_ROOT="$tmp_root/holdout" \
	"$root_dir/scripts/run-v2-integrated-longrun-cell.sh" holdout-619 /bin/true
expect_failure env GOMAXPROCS=4 \
	"$root_dir/scripts/run-v2-integrated-longrun-cell.sh" holdout-619 /bin/true
expect_failure "$root_dir/scripts/extract-v2-integrated-longrun-cell.sh" \
		"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/extract-v2-integrated-longrun-cell.sh" \
		"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4/../../../../../../home/vlad/v2-integrated-longrun-candidate-20260828-v4/holdout-619"
expect_failure "$root_dir/scripts/check-v2-integrated-longrun-parity.sh" "$tmp_root/parity"
expect_failure "$root_dir/scripts/score-v2-integrated-longrun-development.sh" "$tmp_root/score"

echo "integrated long-run contract tests: pass"
