#!/usr/bin/env bash
# SV1-specific namespace wrapper around the accepted R2 evidence primitives.
# The economic and calendar checks remain shared; only the output namespace and
# registered cell identities differ from historical R2.
set -euo pipefail

source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1-24h-development-20260901-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1-24h-development-20260901-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1-24h-development.lock"

v2_r2_capacity_attestation_path() {
	printf '%s\n' '/home/vlad/v2-r2-sv1-24h-binary-capacity-20260901-v1.json'
}

v2_r2_require_attestation_path() {
	local cell=$1
	case "$cell" in
		treatment-607|treatment-613|treatment-617|control-607|control-613|control-617|treatment-607-g8|control-607-none) ;;
		*) return 1 ;;
	esac
	[[ ! -L "$v2_r2_attestation_root" ]] || return 1
	[[ ! -L "$v2_r2_attestation_root/$cell.json" ]] || return 1
	[[ "$(realpath -m -- "$v2_r2_attestation_root/$cell.json")" == "$v2_r2_attestation_root/$cell.json" ]]
}
