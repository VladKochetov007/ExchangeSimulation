.result as $result |
($min_side_depth // 0) as $depth_floor |
($result.windows // [] | map(select(.symbol == "CDF/USD"))) as $windows |
($result.book_summaries // [] | map(select(.symbol == "CDF/USD")) | sort_by(.venue_id)) as $books |
([range(0; $expected_windows)] | map($start_nano + . * $window_nano)) as $expected_starts |
($windows | group_by(.venue_id) | map({
	venue_id: .[0].venue_id,
	spans: (map({start, end}) | sort_by(.start))
})) as $window_groups |
($books | map(.venue_id) | sort) as $book_venues |
($window_groups | map(.venue_id) | sort) as $window_venues |
{
	schema_version: 1,
	contract: $contract,
	cell: $cell,
	seed: $seed,
	metric: "viability",
	start_nano: $start_nano,
	window_nano: $window_nano,
	expected_windows_per_venue: $expected_windows,
	max_empty_side_share: $max_empty,
	minimum_executable_side_depth: $depth_floor,
	observed_window_venues: $window_venues,
	venues: ($books | map({
		venue_id,
		windows,
		viable,
		snapshots,
		empty_side_snapshots,
			qualified_empty_side_snapshots: (.qualified_empty_side_snapshots // .empty_side_snapshots),
			observed_empty_side_share: (if .snapshots > 0 then (.empty_side_snapshots / .snapshots) else null end),
			observed_qualified_empty_side_share: (if .snapshots > 0 then ((.qualified_empty_side_snapshots // .empty_side_snapshots) / .snapshots) else null end)
	})),
	window_metrics: ($windows | sort_by(.venue_id, .start) | map({
		venue_id, start, end, snapshots, empty_side_snapshots,
		qualified_empty_side_snapshots: (.qualified_empty_side_snapshots // .empty_side_snapshots),
		empty_side_share: (if .snapshots > 0 then (.empty_side_snapshots / .snapshots) else null end),
		qualified_empty_side_share: (if .snapshots > 0 then ((.qualified_empty_side_snapshots // .empty_side_snapshots) / .snapshots) else null end)
	})),
	observed_windows: ($windows | length),
	predicates: {
		cdf_books_present: (($books | length) == ($required_venues | length) and $book_venues == $required_venues),
		post_warmup_snapshots_present: (all($books[]; .snapshots > 0)),
			exact_post_warmup_window_coverage: (($windows | length) == ($expected_windows * ($required_venues | length)) and
				($window_groups | length) == ($required_venues | length) and
				$window_venues == $required_venues and
			all($window_groups[]; (.spans | map(.start)) == $expected_starts and
				all(.spans[]; .end == (.start + $window_nano))) and
			all($books[]; .windows == $expected_windows)),
		aggregate_two_sided_98pct: (all($books[]; (.snapshots > 0 and ((.qualified_empty_side_snapshots // .empty_side_snapshots) / .snapshots) <= $max_empty))),
		no_persistent_one_sided_window: (all($windows[]; (.snapshots > 0 and ((.qualified_empty_side_snapshots // .empty_side_snapshots) / .snapshots) <= $max_empty)))
	},
	viability_result_shape: {books: ($result.books // 0), windows: ($result.windows | length)}
}
