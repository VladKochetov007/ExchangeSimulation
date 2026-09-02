(.terminal_accounts | type) == "array" and
(.terminal_accounts | length) > 0 and
all(.terminal_accounts[];
	.phase == "terminal_post_mark" and
	(.account | type) == "object" and
	.account.timestamp == $end and
	(.mark_source | type) == "string" and
	(.marks | type) == "object" and
	(.marks.CDF | type) == "number" and
	(.marks.USD | type) == "number")
