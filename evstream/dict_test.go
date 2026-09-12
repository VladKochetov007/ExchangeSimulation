package evstream

import (
	"errors"
	"testing"
)

func TestDictionaryReservesZeroAndRejectsEmptyValues(t *testing.T) {
	dictionary := NewDictionary()
	if _, ok := dictionary.Value(0); ok {
		t.Fatal("reserved dictionary id zero resolved as a value")
	}
	if _, ok := dictionary.Lookup(""); ok {
		t.Fatal("empty string unexpectedly has a dictionary id")
	}
	if _, err := dictionary.Assign(""); !errors.Is(err, ErrEmptyDictionaryValue) {
		t.Fatalf("empty assignment error = %v, want ErrEmptyDictionaryValue", err)
	}
	if err := dictionary.Define(1, ""); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("empty definition error = %v, want ErrCorrupt", err)
	}
	if err := dictionary.Define(0, "defined"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("zero definition error = %v, want ErrCorrupt", err)
	}

	id, err := dictionary.Assign("defined")
	if err != nil || id != 1 {
		t.Fatalf("first non-empty assignment = (%d, %v), want (1, nil)", id, err)
	}
	if value, ok := dictionary.Value(id); !ok || value != "defined" {
		t.Fatalf("assigned value = (%q, %t), want (defined, true)", value, ok)
	}
}

type permissiveResolver struct{}

func (permissiveResolver) Lookup(uint32) (string, bool) { return "", true }

func TestResolveRequiredRejectsZeroAndEmptyResolverValues(t *testing.T) {
	resolver := permissiveResolver{}
	if _, err := ResolveRequired(resolver, 0); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("zero required reference error = %v, want ErrCorrupt", err)
	}
	if _, err := ResolveRequired(resolver, 1); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("empty required reference error = %v, want ErrCorrupt", err)
	}
}
