package main

import (
	"io"
	"testing"
)

func TestOwnerRunsLearnFixesUnlessToldNotTo(t *testing.T) {
	f, _, _, err := parseRunArgs([]string{"-repo", "o/r"}, io.Discard)
	if err != nil || !f.learn {
		t.Fatalf("default flags = %+v, %v", f, err)
	}
	f, _, _, err = parseRunArgs([]string{"-repo", "o/r", "-learn=false"}, io.Discard)
	if err != nil || f.learn {
		t.Fatalf("-learn=false = %+v, %v", f, err)
	}
}
