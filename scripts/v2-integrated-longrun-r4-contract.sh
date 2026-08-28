#!/usr/bin/env bash

# Shared fail-closed checks for the immutable r4 evidence namespace. This file
# is sourced by the runner, extractor, parity checker, and scorer.
v2_r4_output_root="/home/vlad/v2-integrated-longrun-candidate-20260828-v4"

v2_r4_require_output_root() {
	local output_root=$1
	[[ "$output_root" == "$v2_r4_output_root" ]] || return 1
	[[ ! -L "$output_root" ]] || return 1
	[[ "$(realpath -m -- "$output_root")" == "$v2_r4_output_root" ]] || return 1
	if [[ -e "$output_root" ]]; then
		[[ -d "$output_root" ]] || return 1
		[[ "$(realpath -e -- "$output_root")" == "$v2_r4_output_root" ]] || return 1
	fi
}

v2_r4_require_cell_path() {
	local cell_path=$1
	[[ "$cell_path" == /* ]] || return 1
	local current=/ component
	local path_without_root=${cell_path#/}
	IFS=/ read -r -a path_components <<< "$path_without_root"
	for component in "${path_components[@]}"; do
		[[ -n "$component" ]] || continue
		current="${current%/}/$component"
		[[ ! -L "$current" ]] || return 1
	done
	[[ -d "$cell_path" && ! -L "$cell_path" ]] || return 1
	local canonical_cell
	canonical_cell=$(realpath -e -- "$cell_path") || return 1
	[[ "$(realpath -e -- "$v2_r4_output_root")" == "$v2_r4_output_root" ]] || return 1
	[[ "${canonical_cell%/*}" == "$v2_r4_output_root" ]] || return 1
	[[ "$(realpath -m -- "$cell_path")" == "$canonical_cell" ]] || return 1
}

v2_r4_binary_go_version() {
	go version -m "$1" | sed -n '1s/.*: //p'
}

v2_r4_is_go_127() {
	[[ "$1" == go1.27* ]]
}
