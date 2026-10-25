package stub

import (
	"errors"
	"testing"
)

func TestErr(t *testing.T) {
	err := Err("record")
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Err() = %v, want it to wrap errors.ErrUnsupported", err)
	}
	if got, want := err.Error(), "record: not implemented: unsupported operation"; got != want {
		t.Fatalf("Err().Error() = %q, want %q", got, want)
	}
}
