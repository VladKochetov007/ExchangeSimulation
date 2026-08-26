#!/usr/bin/env bash
# P7b uses the same fail-closed distress evidence contract as P7a, with its
# own hypothesis ID, horizon and development seeds.  Keep this wrapper thin so
# any contract hardening is shared by both protocols.
set -euo pipefail
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec env P7_DISTRESS_PROTOCOL=p7b "$root_dir/scripts/extract-v2-7-p7a-cell.sh" "$@"
