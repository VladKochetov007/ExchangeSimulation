#!/usr/bin/env bash
# Build the compact V2-2b screening table from retained, independently
# extracted artifacts. It never opens, removes, or rewrites raw evidence.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-2b"
rows=$(mktemp "${TMPDIR:-/tmp}/v2-2b-rows-XXXXXX")
trap 'rm -f "$rows"' EXIT

for arm in I0R0 I1R0 I0R1 I1R1; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		for required in run-metadata.json evidenceartifacthash.json observationreceipts.json frontiervectors.json crossvenue.json arbitrage.json activation.json; do
			if [[ ! -s "$cell/$required" ]]; then
				echo "missing V2-2b metric: $cell/$required" >&2
				exit 1
			fi
		done
		jq -n \
			--arg arm "$arm" \
			--argjson seed "$seed" \
			--slurpfile provenance "$cell/run-metadata.json" \
			--slurpfile evidence_hash "$cell/evidenceartifacthash.json" \
			--slurpfile receipts "$cell/observationreceipts.json" \
			--slurpfile vectors "$cell/frontiervectors.json" \
			--slurpfile dispersion "$cell/crossvenue.json" \
			--slurpfile arbitrage "$cell/arbitrage.json" \
			--slurpfile activation "$cell/activation.json" \
			'{
			  arm: $arm,
			  seed: $seed,
			  provenance: $provenance[0],
			  evidence_artifact_hash: $evidence_hash[0].result,
			  receipt_audit: $receipts[0].result,
			  frontier_vector_audit: $vectors[0].result,
			  crossvenue_dispersion: $dispersion[0].result,
			  crossvenue_after_fee_edges: [
			    $arbitrage[0].result.cycles[] | select(.cycle | startswith("cross_venue"))
			  ],
			  activation: $activation[0],
			  local_execution_activation: [
			    $activation[0].metaorders[] as $parent |
			    ([
			      $dispersion[0].result.positive_observation_times_nanos[]? |
			      select(. >= $parent.start_timestamp and . <= $parent.end_timestamp)
			    ]) as $positive |
			    {
			      venue_id: $parent.venue_id,
			      trader_id: $parent.trader_id,
			      parent_qty: $parent.parent_qty,
			      filled_qty: $parent.filled_qty,
			      start_timestamp: $parent.start_timestamp,
			      end_timestamp: $parent.end_timestamp,
			      positive_sample_count: ($positive | length),
			      positive_sample_times_nanos: $positive
			    }
			  ]
			}' >>"$rows"
	done
done

jq -s --arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" '
  . as $cells |
  def cell($arm; $seed): first($cells[] | select(.arm == $arm and .seed == $seed));
  def dispersion($arm; $seed): cell($arm; $seed).crossvenue_dispersion.midpoint_range_bps.Mean;
  def edge_observations($arm; $seed):
    [cell($arm; $seed).crossvenue_after_fee_edges[].profitable] | add // 0;
  def edge_longest_run($arm; $seed):
    [cell($arm; $seed).crossvenue_after_fee_edges[].longest_run_nanos] | max // 0;
  def router($arm; $seed): cell($arm; $seed).activation.routers[0] // {};
  {
    schema_version: 1,
    experiment_id: "V2-2b",
    analysis_revision: $analysis_revision,
    metric_contract: {
      midpoint_dispersion: "offline omniscient fresh two-sided ABC-USD midpoint max-min range with positive sampled times; 2s staleness; three venues",
      executable_edges: "offline omniscient after-fee 5bps bid/ask diagnostic; not router information or PnL",
      router: "direct non-atomic report independently reconciled from persisted group legs",
      information: "V2-0/V3 independent sidecar replay and per-link activity"
    },
    cells: $cells,
    paired_contrasts: [101, 103] | map(. as $seed | {
      seed: $seed,
      informed_effect_router_off: {
        dispersion_mean_bps_delta: dispersion("I1R0"; $seed) - dispersion("I0R0"; $seed),
        after_fee_profitable_observations_delta: edge_observations("I1R0"; $seed) - edge_observations("I0R0"; $seed),
        after_fee_longest_run_nanos_delta: edge_longest_run("I1R0"; $seed) - edge_longest_run("I0R0"; $seed)
      },
      router_effect_informed_off: {
        dispersion_mean_bps_delta: dispersion("I0R1"; $seed) - dispersion("I0R0"; $seed),
        after_fee_profitable_observations_delta: edge_observations("I0R1"; $seed) - edge_observations("I0R0"; $seed),
        after_fee_longest_run_nanos_delta: edge_longest_run("I0R1"; $seed) - edge_longest_run("I0R0"; $seed),
        submitted_groups: router("I0R1"; $seed).submitted_groups // 0
      },
      informed_effect_router_on: {
        dispersion_mean_bps_delta: dispersion("I1R1"; $seed) - dispersion("I0R1"; $seed),
        after_fee_profitable_observations_delta: edge_observations("I1R1"; $seed) - edge_observations("I0R1"; $seed),
        after_fee_longest_run_nanos_delta: edge_longest_run("I1R1"; $seed) - edge_longest_run("I0R1"; $seed),
        submitted_groups: router("I1R1"; $seed).submitted_groups // 0
      },
      router_effect_informed_on: {
        dispersion_mean_bps_delta: dispersion("I1R1"; $seed) - dispersion("I1R0"; $seed),
        after_fee_profitable_observations_delta: edge_observations("I1R1"; $seed) - edge_observations("I1R0"; $seed),
        after_fee_longest_run_nanos_delta: edge_longest_run("I1R1"; $seed) - edge_longest_run("I1R0"; $seed),
        submitted_groups: router("I1R1"; $seed).submitted_groups // 0
      }
    })
  }' "$rows" >"$artifact_dir/summary.json"
