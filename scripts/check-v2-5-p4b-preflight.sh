#!/usr/bin/env bash
# Run the bounded P4b mechanics/evidence preflight.  This is not an economic
# outcome cell and has no authority to score or prune a P4b world.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
binary=${MV_BIN:-"$root_dir/bin/multivenue"}
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
config_dir="$root_dir/research/configs/v2-5-p4b"
summary="$root_dir/research/artifacts/v2-5-p4b/preflight-summary.json"

test -x "$binary"
test -x "$analyzer"
"$root_dir/scripts/check-v2-5-p4b-configs.sh" >/dev/null

# Keep the temporary mechanics output under the declared artifact root.  This
# avoids depending on a host-wide temp directory and lets repository hygiene
# tests prove that the preflight cannot accidentally embed an environment
# path in tracked code.  The trap removes it on every exit path.
preflight_root=$(mktemp -d "$root_dir/research/artifacts/v2-5-p4b/.preflight.XXXXXX")
trap 'rm -rf "$preflight_root"' EXIT

for arm in A B; do
	cell="$preflight_root/$arm-401"
	mkdir -p "$cell"
	"$binary" -config "$config_dir/$arm-401.json" -duration 5m -logdir "$cell" -log-mode full >"$cell/simulator.stdout" 2>"$cell/simulator.stderr"
	test -s "$cell/greeks.json"
	test -s "$cell/latency.json"
	for metric in observationreceipts perpexposurehedger termcarry termcarryp4chain streamhash evidenceartifacthash; do
		"$analyzer" -metric "$metric" -json "$cell" >"$cell/$metric.json"
	done

	jq -e '
		.result.valid == true and .result.receipt_audit_valid == true and
		.result.decisions > 0 and .result.submitted > 0 and .result.fills > 0 and
		.result.non_reducing_fills == 0 and .result.receipt_evidence_errors == 0
	' "$cell/perpexposurehedger.json" >/dev/null
	jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
	jq -e '.result.valid == true' "$cell/termcarry.json" >/dev/null
	jq -e '.result.valid == true and .result.exact_cost_decisions_evaluated > 0' "$cell/termcarryp4chain.json" >/dev/null
	jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null
	jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null

	runtime_events=$(jq -r '.events' "$cell/evidence-artifact-hash.json")
	runtime_digest=$(jq -r '.digest' "$cell/evidence-artifact-hash.json")
	offline_events=$(jq -r '.result.events' "$cell/evidenceartifacthash.json")
	offline_digest=$(jq -r '.result.digest' "$cell/evidenceartifacthash.json")
	if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
		echo "P4b preflight runtime/offline evidence digest mismatch: $arm-401" >&2
		exit 1
	fi
done

mkdir -p "$(dirname -- "$summary")"
jq -n \
	--arg source_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg duration "5m" \
	--slurpfile a "$preflight_root/A-401/evidenceartifacthash.json" \
	--slurpfile b "$preflight_root/B-401/evidenceartifacthash.json" \
	--slurpfile ap "$preflight_root/A-401/perpexposurehedger.json" \
	--slurpfile bp "$preflight_root/B-401/perpexposurehedger.json" \
	'{
	  contract: "v2-5-p4b-preflight-v1",
	  source_revision: $source_revision,
	  multivenue_sha256: $binary_sha256,
	  mvanalyze_sha256: $analyzer_sha256,
	  duration: $duration,
	  cells: {
	    "A-401": {
	      evidence_events: $a[0].result.events,
	      evidence_digest: $a[0].result.digest,
	      exposure_valid: $ap[0].result.valid,
	      exposure_decisions: $ap[0].result.decisions,
	      exposure_fills: $ap[0].result.fills
	    },
	    "B-401": {
	      evidence_events: $b[0].result.events,
	      evidence_digest: $b[0].result.digest,
	      exposure_valid: $bp[0].result.valid,
	      exposure_decisions: $bp[0].result.decisions,
	      exposure_fills: $bp[0].result.fills
	    }
	  },
	  checks: {
	    config_family: true,
	    final_completion_sentinels: true,
	    term_carry_chain: true,
	    physical_exposure_chain: true,
	    receipt_frontiers: true,
	    runtime_offline_evidence_digest: true,
	    raw_evidence_policy: "temporary mechanics diagnostics; no scoring or prune authority"
	  }
}' >"$summary"

printf 'P4b mechanics/evidence preflight passed: %s\n' "$summary"
