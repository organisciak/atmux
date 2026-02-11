package cmd

import "testing"

func TestHidePath(t *testing.T) {
	got := hidePath("/Users/demo/projects/atmux")
	want := "<path hidden>/atmux"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDisplayPathForList(t *testing.T) {
	path := "/Users/demo/projects/atmux"

	if got := displayPathForList(path, true, false); got != "<path hidden>/atmux" {
		t.Fatalf("hide path mismatch: %q", got)
	}

	if got := displayPathForList(path, false, false); got != path {
		t.Fatalf("plain path mismatch: %q", got)
	}
}
