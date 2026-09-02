def nonempty_string:
	type == "string" and length > 0;

type == "object" and
.schema_version == 1 and
.phase == "terminal_post_mark" and
.simulation_start_nano == $start and
.simulation_end_nano == $end and
(.strict_population_accounting | type) == "boolean" and
.evidence_format == "evstream_v3" and
(.evidence_sealed | type) == "boolean" and
(.terminal_risk_captured | type) == "boolean" and
(.terminal_population_captured | type) == "boolean" and
(.status == "completed" or .status == "terminal_failure") and
(
	if .status == "completed" then
		.code == "COMPLETED" and
		.evidence_sealed == true and
		.terminal_risk_captured == true and
		((.strict_population_accounting == false) or .terminal_population_captured == true) and
		(has("error") | not) and
		(has("evidence_seal_error") | not)
	else
		(.code == "PRICE_UNAVAILABLE" or .code == "PRICE_DOMAIN_ERROR") and
		.evidence_sealed == true and
		(.error | nonempty_string) and
		((.evidence_seal_error // "") == "")
	end
)
