package analysis

// evidenceOrder identifies a persisted record well enough to establish
// causality. Reconstructed successor evidence carries an optional logical
// global sequence, which is authoritative whenever both records have it.
// Historical JSON logs retain the timestamp/file/ordinal fallback because they
// have no global order field.
type evidenceOrder struct {
	timestamp      int64
	file           string
	ordinal        int64
	globalSequence uint64
}

func eventEvidenceOrder(event Event) evidenceOrder {
	return evidenceOrder{
		timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
		globalSequence: event.GlobalSequence,
	}
}

func evidenceAfter(use, prerequisite evidenceOrder) bool {
	if use.globalSequence != 0 && prerequisite.globalSequence != 0 {
		return prerequisite.globalSequence < use.globalSequence
	}
	if prerequisite.timestamp > use.timestamp {
		return false
	}
	if use.file == prerequisite.file {
		return prerequisite.ordinal < use.ordinal
	}
	return use.timestamp > prerequisite.timestamp
}

func evidenceBefore(left, right evidenceOrder) bool {
	if left.globalSequence != 0 && right.globalSequence != 0 {
		return left.globalSequence < right.globalSequence
	}
	if left.timestamp != right.timestamp {
		return left.timestamp < right.timestamp
	}
	if left.file != right.file {
		return left.file < right.file
	}
	return left.ordinal < right.ordinal
}

func latestCausalPrerequisite(prerequisites []evidenceOrder, use evidenceOrder) (evidenceOrder, bool) {
	var latest evidenceOrder
	found := false
	for _, prerequisite := range prerequisites {
		if !evidenceAfter(use, prerequisite) {
			continue
		}
		if !found || evidenceBefore(latest, prerequisite) {
			latest = prerequisite
			found = true
		}
	}
	return latest, found
}
