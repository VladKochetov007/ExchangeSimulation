package evstream

// Dictionary interns repeated strings — venue ids, symbols, event names, asset
// codes — so an event references four bytes instead of carrying the string.
//
// In the integrated workload a symbol like "ABC-FUT-1735696801" appears in
// millions of records. JSON pays for it every time; here it is written once.
//
// Ids are assigned strictly in first-use order. Because the event order is
// deterministic, the assignment is too, and the same run always produces the
// same ids. That is what allows dictionary entries to be part of the hashed
// stream rather than metadata alongside it.
type Dictionary struct {
	ids    map[string]uint32
	values []string
}

// NewDictionary returns an empty dictionary. Id 0 is reserved to mean "no
// reference", so assignment starts at 1 and a zero VenueRef is unambiguous.
func NewDictionary() *Dictionary {
	return &Dictionary{ids: make(map[string]uint32), values: []string{""}}
}

// Lookup returns the id for s if it has been assigned.
func (d *Dictionary) Lookup(s string) (uint32, bool) {
	id, ok := d.ids[s]
	return id, ok
}

// Assign gives s the next id. The caller is responsible for emitting the
// corresponding dictionary frame, so that a reader learns the mapping from the
// stream itself.
func (d *Dictionary) Assign(s string) uint32 {
	id := uint32(len(d.values))
	d.ids[s] = id
	d.values = append(d.values, s)
	return id
}

// Define records a mapping learned while reading. Out-of-order or duplicate
// ids are rejected: a stream that redefines an id is corrupt, and silently
// accepting it would let two events that reference the same id mean different
// things.
func (d *Dictionary) Define(id uint32, value string) error {
	if id != uint32(len(d.values)) {
		return ErrCorrupt
	}
	d.ids[value] = id
	d.values = append(d.values, value)
	return nil
}

// Value resolves an id. The second result is false for an id the stream never
// defined, which a reader should treat as corruption rather than as an empty
// string.
func (d *Dictionary) Value(id uint32) (string, bool) {
	if id >= uint32(len(d.values)) {
		return "", false
	}
	return d.values[id], true
}

// Len returns the number of assigned ids, excluding the reserved zero.
func (d *Dictionary) Len() int { return len(d.values) - 1 }
