package strategy

import "testing"

const corpus = "Pull request: Bump example.com/lib\n-func Total() int { return Add(1, 2) }\n+func Total() int { return Sum(1, 2) }\nclock/clock.go\n*clockwork.FakeClock\n"

func TestGrounded(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"plain prose from the diff", "Renames the call from Add to Sum so the package builds again.", true},
		{"a path", "Adjusts clock/clock.go for the new type.", true},
		{"a qualified identifier", "The field now holds a pointer to clockwork.FakeClock.", true},
		{"backticked and present", "Calls `Sum` instead of `Add`.", true},
		{"backticked and invented", "Calls `Total` through `Registry`.", false},
		{"a capitalized word that is not in the change", "Switches to Bounded Caches for the Sum call.", false},
		{"an invented camel case name", "Wraps the call in retryWithBackoff.", false},
		{"sentence starts are free", "Updated the call. Tests pass again.", true},
		{"lowercase words are free", "removes the stale helper and keeps behaviour the same", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Grounded(tt.text, corpus); got != tt.want {
				t.Fatalf("Grounded(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
