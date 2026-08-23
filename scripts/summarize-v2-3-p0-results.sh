#!/usr/bin/env bash
# Build the compact, machine-readable P0-r1 score input from only required
# retained artifacts. This script never reads a terminal price statistic.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-3-p0-r1"
output="$artifact_dir/p0-summary.json"
scratch_dir=$(mktemp -d -t v2-3-p0-summary-XXXXXX)

required=(
  observationreceipts.json evidenceartifacthash.json
  postonly-cdf.json postonly-spot-policy.json
  postonly-cdf-cdf_spot_maker.json
  postonly-cdf-fixed_distance_maker.json
  postonly-cdf-imbalance_maker.json
  postonly-derivative-scope.json viability.json cdf-viability.json
  analysis-metadata.json
)

for arm in A B C; do
  for seed in 101 103; do
    cell="$artifact_dir/$arm/seed-$seed"
    for name in "${required[@]}"; do
      if [[ ! -s "$cell/$name" ]]; then
        echo "missing required P0-r1 artifact: $cell/$name" >&2
        exit 1
      fi
    done

    jq -n \
      --arg arm "$arm" --argjson seed "$seed" \
      --slurpfile metadata "$cell/run-metadata.json" \
      --slurpfile receipt "$cell/observationreceipts.json" \
      --slurpfile artifact "$cell/evidenceartifacthash.json" \
      --slurpfile pooled "$cell/postonly-cdf.json" \
      --slurpfile policy "$cell/postonly-spot-policy.json" \
      --slurpfile scope "$cell/postonly-derivative-scope.json" \
      --slurpfile cdf "$cell/cdf-viability.json" \
      --slurpfile stoikov "$cell/postonly-cdf-cdf_spot_maker.json" \
      --slurpfile fixed "$cell/postonly-cdf-fixed_distance_maker.json" \
      --slurpfile imbalance "$cell/postonly-cdf-imbalance_maker.json" \
      '
      def activity($x): $x[0].result | {
        events, accepted, accepted_post_only, accepted_regular,
        rejected_would_take, rejected_invalid,
        post_only_fills, post_only_filled_qty, unmatched_fill_orders
      };
      def receipt_roles:
        [ $receipt[0].result.link_activity[] |
          select((.role == "spot_maker") or (.role == "fixed_distance_maker") or (.role == "imbalance_maker"))
        ] | group_by(.role) |
        map({role: .[0].role, schedules:(map(.schedules)|add), receipts:(map(.receipts)|add), decisions:(map(.decisions)|add)});
      def cdf_windows: $cdf[0].windows;
      {
        arm: $arm,
        seed: $seed,
        provenance: {
          config_sha256: $metadata[0].config_sha256,
          binary_sha256: $metadata[0].binary_sha256,
          git_revision: $metadata[0].git_revision,
          gomaxprocs: $metadata[0].gomaxprocs,
          evidence: {events: $artifact[0].result.events, digest: $artifact[0].result.digest}
        },
        information_boundary: {valid: $receipt[0].result.valid, receipt_roles: receipt_roles},
        cdf_pooled: activity($pooled),
        spot_policy: activity($policy),
        cdf_maker_activity: {
          cdf_spot_maker: activity($stoikov),
          fixed_distance_maker: activity($fixed),
          imbalance_maker: activity($imbalance)
        },
        derivative_scope: activity($scope),
        cdf_viability: {
          venues: (cdf_windows | map(.venue_id) | unique),
          snapshots: (cdf_windows | map(.snapshots) | add),
          empty_side_snapshots: (cdf_windows | map(.empty_side_snapshots) | add),
          two_sided_share: (1 - ((cdf_windows | map(.empty_side_snapshots) | add) / (cdf_windows | map(.snapshots) | add))),
          trades: (cdf_windows | map(.trades) | add),
          volume: (cdf_windows | map(.volume) | add)
        }
      } |
      .gates = {
        information_boundary_valid: .information_boundary.valid,
        receipt_roles_active: ([.information_boundary.receipt_roles[] | (.schedules > 0 and .receipts > 0)] | all),
        cdf_makers_active: ([.cdf_maker_activity[] | ((.accepted + .rejected_would_take) > 0)] | all),
        derivative_scope_clean: (.derivative_scope.accepted_post_only == 0),
        cdf_all_venues_present: (.cdf_viability.venues == ["central", "north", "south"] and .cdf_viability.snapshots > 0),
        cdf_two_sided_available: (.cdf_viability.two_sided_share > 0),
        cdf_trading_active: (.cdf_viability.trades > 0 and .cdf_viability.volume > 0)
      }' >"$scratch_dir/$arm-$seed.json"
  done
done

jq -s '
  def row($arm; $seed): .[] | select(.arm == $arm and .seed == $seed);
  def deltas($to; $from): {
    cdf_accepted: ($to.cdf_pooled.accepted - $from.cdf_pooled.accepted),
    cdf_post_only_rejections: ($to.cdf_pooled.rejected_would_take - $from.cdf_pooled.rejected_would_take),
    policy_post_only_rejections: ($to.spot_policy.rejected_would_take - $from.spot_policy.rejected_would_take),
    cdf_two_sided_share: ($to.cdf_viability.two_sided_share - $from.cdf_viability.two_sided_share),
    cdf_trades: ($to.cdf_viability.trades - $from.cdf_viability.trades),
    cdf_volume: ($to.cdf_viability.volume - $from.cdf_viability.volume)
  };
  . as $rows |
  {
    schema_version: 1,
    experiment: "V2-3-P0-R1",
    comparison_contract: {
      b_minus_a: "exchange-level arrival-time post-only admission",
      c_minus_b: "actor requested cancel-before-replace order",
      scope: "passive spot population only; CDF/USD viability primary"
    },
    rows: $rows,
    paired_deltas: [101, 103] | map(. as $seed |
      ($rows | row("A"; $seed)) as $a |
      ($rows | row("B"; $seed)) as $b |
      ($rows | row("C"; $seed)) as $c |
      {seed: $seed, b_minus_a: deltas($b; $a), c_minus_b: deltas($c; $b)}
    )
  }' "$scratch_dir"/*.json >"$output.tmp"

if ! jq -e '[.rows[].gates | to_entries[] | select(.value != true)] | length == 0' "$output.tmp" >/dev/null; then
  echo "P0-r1 required activation or viability gate failed; summary retained for diagnosis" >&2
  mv "$output.tmp" "$output"
  exit 1
fi
mv "$output.tmp" "$output"
printf 'wrote %s\n' "$output"
