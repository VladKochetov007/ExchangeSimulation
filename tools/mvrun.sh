#!/usr/bin/env bash
# Runs the multivenue binary, rebuilding first if any Go source is newer.
#
# Four experiments in this campaign were run against a binary compiled before
# the fix under test, and each time the result looked like a real null. The
# manifest build stamp and run_outcome.py catch it after the fact, but only if
# somebody remembers to look. Rebuilding here removes the choice.
set -euo pipefail
cd "$(dirname "$0")/.."
BIN=.cache/multivenue
if [ ! -x "$BIN" ] || [ -n "$(find . -name '*.go' -newer "$BIN" -not -path './.cache/*' -print -quit)" ]; then
    echo "mvrun: rebuilding $BIN" >&2
    go build -o "$BIN" ./cmd/multivenue
fi
exec "$BIN" "$@"
