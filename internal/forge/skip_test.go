package forge

import "testing"

func TestSkipReasonNamesWhyAPullRequestIsNotWorked(t *testing.T) {
	for _, tc := range []struct {
		draft, exists, same bool
		target, want        string
	}{
		{draft: true, exists: true, same: true, target: "main", want: "it is a draft"},
		{exists: true, same: true, target: "release", want: "it targets release, not main"},
		{exists: false, target: "main", want: "the repository it came from was deleted"},
		{exists: true, same: false, target: "main", want: "it comes from a fork"},
		{exists: true, same: true, target: "main", want: ""},
	} {
		if got := skipReason(tc.draft, tc.target, "main", tc.exists, tc.same); got != tc.want {
			t.Errorf("skipReason(%+v) = %q, want %q", tc, got, tc.want)
		}
	}
}
