package exsim

import "encoding/json"

// jsonMarshalString defers to encoding/json for strings the verbatim path
// cannot reproduce. Rendering back to canonical JSON is only useful if it is
// exact, so the awkward cases go to the reference implementation rather than to
// a reimplementation of its escaping rules.
func jsonMarshalString(s string) ([]byte, error) { return json.Marshal(s) }
