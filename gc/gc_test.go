package gc

import (
	"reflect"
	"testing"
)

func TestTricolorBasic(t *testing.T) {
	roots := []string{"A", "C"}
	all := []string{"A", "B", "C", "D", "E", "F", "G"}
	refs := map[string][]string{
		"A": {"B"},
		"B": {"A"},
		"C": {"D", "F", "B"},
		"E": {"F", "G"},
	}

	unreachable := Tricolor(roots, all, lookup(refs))
	expected := []string{"E", "G"}

	if !reflect.DeepEqual(unreachable, expected) {
		t.Fatalf("incorrect unreachable set: %v != %v", unreachable, expected)
	}
}

func lookup(refs map[string][]string) func(id string) []string {
	return func(ref string) []string {
		return refs[ref]
	}
}
