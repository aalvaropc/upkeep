package forge

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/pull"
)

var bumpTitle = regexp.MustCompile(`(?i)^(?:[a-z]+(?:\([^)]*\))?!?:\s*)?bump\s+(\S+)\s+from\s+v?(\S+)\s+to\s+v?(\S+?)\.?\s*$`)

var updateTitle = regexp.MustCompile(`(?i)^(?:[a-z]+(?:\([^)]*\))?!?:\s*)?update\s+(?:module\s+|dependency\s+)?(\S+)\s+to\s+v?(\S+?)\.?(?:\s+(?:\([^)]*\)|\[[^\]]*\]))*\s*$`)

var versionChange = regexp.MustCompile("`v?([0-9][^`\\s]*)`\\s*(?:->|→)\\s*`v?([0-9][^`\\s]*)`")

func BumpFromPull(title, branch, body string) *pull.Bump {
	switch {
	case strings.HasPrefix(branch, "dependabot/go_modules/"):
		return BumpFromTitle(title, branch)
	case strings.HasPrefix(branch, "renovate/"):
		return renovateBump(title, body)
	}
	return nil
}

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

func renovateBump(title, body string) *pull.Bump {
	m := updateTitle.FindStringSubmatch(strings.TrimSpace(title))
	if m == nil || !goModulePath(m[1]) {
		return nil
	}
	module, target := m[1], "v"+m[2]
	from, to, ok := changeIn(body, module)
	if !ok || !strings.HasPrefix(to+".", target+".") {
		return nil
	}
	level, err := pull.LevelOf(from, to)
	if err != nil {
		return nil
	}
	if level == pull.Major {
		module = withMajor(module, to)
	}
	return &pull.Bump{Module: module, From: from, To: to, Level: level}
}

func goModulePath(path string) bool {
	host, rest, ok := strings.Cut(path, "/")
	return ok && rest != "" && strings.Contains(host, ".") && !strings.ContainsAny(path, "@`[]()")
}

func changeIn(body, module string) (from, to string, ok bool) {
	var found [][]string
	for line := range strings.Lines(body) {
		m := versionChange.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if strings.Contains(line, module) {
			return "v" + m[1], "v" + m[2], true
		}
		found = append(found, m)
	}
	if len(found) != 1 {
		return "", "", false
	}
	return "v" + found[0][1], "v" + found[0][2], true
}

func withMajor(module, to string) string {
	suffix := github.MajorSuffix(module)
	v, err := pull.ParseVersion(to)
	if suffix == "" || err != nil || v.Major < 2 {
		return module
	}
	return strings.TrimSuffix(module, suffix) + fmt.Sprintf("v%d", v.Major)
}
