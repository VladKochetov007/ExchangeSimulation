#!/usr/bin/env bash

# Input validation shared by binary-evidence extractors and their contract tests.
# The evidence stream is binary; only its JSON sidecars are parsed as JSON.

v2_r2_require_nonempty_regular_file() {
	local path=$1
	[[ -f "$path" && ! -L "$path" && -s "$path" ]]
}

v2_r2_require_json_object_file() {
	local path=$1
	v2_r2_require_nonempty_regular_file "$path" || return 1
	jq -e 'type == "object"' "$path" >/dev/null
}

v2_r2_require_evidence_input_file() {
	local input_kind=$1
	local path=$2
	case "$input_kind" in
		binary)
			v2_r2_require_nonempty_regular_file "$path"
			;;
		json)
			v2_r2_require_json_object_file "$path"
			;;
		*)
			return 2
			;;
	esac
}
