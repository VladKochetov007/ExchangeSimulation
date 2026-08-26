#!/usr/bin/env bash
# P7c uses the shared fail-closed distress evidence contract with a new
# hypothesis ID, horizon and development seed set.  Keep extraction logic
# shared with the already audited P7a/P7b paths.
set -euo pipefail
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec env P7_DISTRESS_PROTOCOL=p7c "$root_dir/scripts/extract-v2-7-p7a-cell.sh" "$@"
