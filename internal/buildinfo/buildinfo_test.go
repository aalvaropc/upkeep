package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	s := String()
	if !strings.HasPrefix(s, "upkeep ") {
		t.Fatalf("String() = %q, want prefix %q", s, "upkeep ")
	}
	if !strings.Contains(s, runtime.Version()) {
		t.Fatalf("String() = %q, want the Go version %q", s, runtime.Version())
	}
}

func TestVersionOverride(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = "v1.2.3"
	if got := Version(); got != "v1.2.3" {
		t.Fatalf("Version() = %q, want v1.2.3", got)
	}
}
