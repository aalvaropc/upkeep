package gitx

import (
	"strings"
	"testing"
)

const simpleConflict = `package notes

const (
	MaxTagLength = 32

<<<<<<< HEAD
	MaxNotesPerOwner = 1000
=======
	MaxAttachments = 5
>>>>>>> main
)
`

func TestParseConflicts(t *testing.T) {
	hunks, err := ParseConflicts("notes/limits.go", simpleConflict)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 1 {
		t.Fatalf("hunks = %+v", hunks)
	}
	h := hunks[0]
	if h.File != "notes/limits.go" || h.Start != 6 || h.Lines != 2 ||
		h.Ours != "\tMaxNotesPerOwner = 1000" || h.Theirs != "\tMaxAttachments = 5" {
		t.Fatalf("hunk = %+v", h)
	}
}

func TestParseConflictsWithBaseSection(t *testing.T) {
	text := "a\n<<<<<<< HEAD\nours 1\nours 2\n||||||| merged common ancestors\nold\n=======\ntheirs\n>>>>>>> main\nz\n"
	hunks, err := ParseConflicts("f", text)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 1 || hunks[0].Ours != "ours 1\nours 2" || hunks[0].Theirs != "theirs" || hunks[0].Lines != 3 {
		t.Fatalf("hunks = %+v", hunks)
	}
}

func TestParseSeveralConflicts(t *testing.T) {
	text := "<<<<<<< a\none\n=======\nuno\n>>>>>>> b\nmiddle\n<<<<<<< a\ntwo\n=======\ndos\ntres\n>>>>>>> b\n"
	hunks, err := ParseConflicts("f", text)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 2 || hunks[0].Start != 1 || hunks[1].Start != 7 || hunks[1].Lines != 3 {
		t.Fatalf("hunks = %+v", hunks)
	}
}

func TestParseConflictsNone(t *testing.T) {
	hunks, err := ParseConflicts("f", "package a\n\nfunc f() {}\n")
	if err != nil || len(hunks) != 0 {
		t.Fatalf("hunks %v error %v", hunks, err)
	}
}

func TestParseConflictsUnclosed(t *testing.T) {
	_, err := ParseConflicts("f.go", "<<<<<<< HEAD\nx\n=======\ny\n")
	if err == nil || !strings.Contains(err.Error(), "f.go:1") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolve(t *testing.T) {
	got, err := Resolve(simpleConflict, []string{"\tMaxNotesPerOwner = 1000\n\tMaxAttachments = 5\n"})
	if err != nil {
		t.Fatal(err)
	}
	want := "package notes\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxNotesPerOwner = 1000\n\tMaxAttachments = 5\n)\n"
	if got != want {
		t.Fatalf("resolved =\n%s\nwant\n%s", got, want)
	}
	if HasMarkers(got) {
		t.Fatal("markers left after resolving")
	}
}

func TestResolveSeveralAndEmpty(t *testing.T) {
	text := "<<<<<<< a\none\n=======\nuno\n>>>>>>> b\nmiddle\n<<<<<<< a\ntwo\n=======\ndos\n>>>>>>> b\nend"
	got, err := Resolve(text, []string{"first", ""})
	if err != nil {
		t.Fatal(err)
	}
	if got != "first\nmiddle\nend" {
		t.Fatalf("resolved = %q", got)
	}
}

func TestResolveCountMismatch(t *testing.T) {
	if _, err := Resolve(simpleConflict, nil); err == nil || !strings.Contains(err.Error(), "more conflicts") {
		t.Errorf("too few resolutions: %v", err)
	}
	if _, err := Resolve(simpleConflict, []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "2 resolutions were given for 1") {
		t.Errorf("too many resolutions: %v", err)
	}
	if _, err := Resolve("<<<<<<< x\nnever closed", []string{"a"}); err == nil {
		t.Error("an unclosed conflict was accepted")
	}
	same, err := Resolve("plain\ntext", nil)
	if err != nil || same != "plain\ntext" {
		t.Errorf("text without conflicts: %q %v", same, err)
	}
}

func TestHasMarkers(t *testing.T) {
	if !HasMarkers(simpleConflict) || !HasMarkers("x\n=======\ny") || !HasMarkers("x\n>>>>>>> main") {
		t.Error("markers were not detected")
	}
	if HasMarkers("package a\n\nconst line = \"=======\"\n") {
		t.Error("a string that contains equals signs is not a marker")
	}
}
