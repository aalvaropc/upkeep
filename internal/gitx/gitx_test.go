package gitx

import "testing"

func TestIsTest(t *testing.T) {
	tests := map[string]bool{
		"auth/token.go":               false,
		"auth/token_test.go":          true,
		"testdata/golden.json":        true,
		"api/testdata/request.json":   true,
		"internal/latest/contest.go":  false,
		"docs/testing.md":             false,
		"middleware/auth_test_fix.go": false,
	}
	for path, want := range tests {
		if got := (FileChange{Path: path}).IsTest(); got != want {
			t.Errorf("IsTest(%q) = %v, want %v", path, got, want)
		}
	}
}
