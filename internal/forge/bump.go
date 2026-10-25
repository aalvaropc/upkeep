package forge

import (
	"regexp"
	"strings"

	"github.com/aalvaropc/upkeep/internal/pull"
)

var bumpTitle = regexp.MustCompile(`(?i)^(?:[a-z]+(?:\([^)]*\))?!?:\s*)?bump\s+(\S+)\s+from\s+v?(\S+)\s+to\s+v?(\S+?)\.?\s*$`)

func BumpFromTitle(title, branch string) *pull.Bump {
	if !strings.HasPrefix(branch, "dependabot/go_modules/") {
		return nil
	}
	m := bumpTitle.FindStringSubmatch(strings.TrimSpace(title))
	if m == nil {
		return nil
	}
	from, to := "v"+m[2], "v"+m[3]
	level, err := pull.LevelOf(from, to)
	if err != nil {
		return nil
	}
	return &pull.Bump{Module: m[1], From: from, To: to, Level: level}
}
