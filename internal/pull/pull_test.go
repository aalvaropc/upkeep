package pull

import "testing"

func TestLevelOf(t *testing.T) {
	tests := []struct {
		from, to string
		want     Level
	}{
		{"v4.5.2", "v5.3.1", Major},
		{"v0.4.0", "v0.5.0", Minor},
		{"v1.2.3", "v1.3.0", Minor},
		{"v1.2.3", "v1.2.4", Patch},
		{"v1.2.3", "v1.2.4-rc.1", Patch},
		{"v1.2.3+incompatible", "v1.2.9", Patch},
	}
	for _, tt := range tests {
		got, err := LevelOf(tt.from, tt.to)
		if err != nil {
			t.Fatalf("LevelOf(%q, %q) error: %v", tt.from, tt.to, err)
		}
		if got != tt.want {
			t.Errorf("LevelOf(%q, %q) = %q, want %q", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestLevelOfErrors(t *testing.T) {
	for _, pair := range [][2]string{
		{"1.2.3", "v1.2.4"},
		{"v1.2", "v1.3.0"},
		{"v1.2.3", "vX.2.3"},
		{"v1.2.3", "v1.2.-1"},
	} {
		if _, err := LevelOf(pair[0], pair[1]); err == nil {
			t.Errorf("LevelOf(%q, %q) returned no error", pair[0], pair[1])
		}
	}
}

func TestHasLabel(t *testing.T) {
	r := Request{Labels: []string{"Upkeep", "bug"}}
	if !r.HasLabel("upkeep") {
		t.Error("HasLabel is not case-insensitive")
	}
	if r.HasLabel("docs") {
		t.Error("HasLabel found a label that is not there")
	}
}
