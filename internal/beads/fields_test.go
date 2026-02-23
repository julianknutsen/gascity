package beads

import "testing"

func TestGetAttachedMol(t *testing.T) {
	desc := "Pancakes: dry=flour,sugar,salt.\nattached_molecule: gc-7\nSome other line."
	got := GetAttachedMol(desc)
	if got != "gc-7" {
		t.Errorf("GetAttachedMol = %q, want %q", got, "gc-7")
	}
}

func TestGetAttachedMolEmpty(t *testing.T) {
	desc := "Pancakes: dry=flour,sugar,salt.\nSome other line."
	got := GetAttachedMol(desc)
	if got != "" {
		t.Errorf("GetAttachedMol = %q, want empty", got)
	}
}

func TestGetAttachedMolEmptyString(t *testing.T) {
	got := GetAttachedMol("")
	if got != "" {
		t.Errorf("GetAttachedMol(\"\") = %q, want empty", got)
	}
}

func TestSetAttachedMol(t *testing.T) {
	desc := "Pancakes: dry=flour,sugar,salt."
	got := SetAttachedMol(desc, "gc-7")
	want := "Pancakes: dry=flour,sugar,salt.\nattached_molecule: gc-7"
	if got != want {
		t.Errorf("SetAttachedMol =\n%q\nwant\n%q", got, want)
	}
}

func TestSetAttachedMolReplace(t *testing.T) {
	desc := "Pancakes: dry=flour.\nattached_molecule: gc-5\nMore text."
	got := SetAttachedMol(desc, "gc-9")
	want := "Pancakes: dry=flour.\nattached_molecule: gc-9\nMore text."
	if got != want {
		t.Errorf("SetAttachedMol =\n%q\nwant\n%q", got, want)
	}
}
