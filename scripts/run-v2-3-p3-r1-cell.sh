#!/usr/bin/env bash
# Run a P3 R1 cell without allowing attempt-0 evidence paths to be reused.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec env P3_VARIANT=v2-3-p3-r1 "$root_dir/scripts/run-v2-3-p3-cell.sh" "$@"
