package pull

import (
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

func ParseVersion(v string) (Version, error) {
	nums, err := parseVersion(v)
	if err != nil {
		return Version{}, err
	}
	out := Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}
	s := strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		out.Pre = s[i+1:]
	}
	return out, nil
}

func (v Version) Prerelease() bool { return v.Pre != "" }

func CompareVersions(a, b Version) int {
	for _, pair := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	return comparePrerelease(a.Pre, b.Pre)
}

func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil && an != bn:
			if an < bn {
				return -1
			}
			return 1
		case aerr == nil && berr != nil:
			return -1
		case aerr != nil && berr == nil:
			return 1
		case aerr != nil && berr != nil && as[i] != bs[i]:
			return strings.Compare(as[i], bs[i])
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}
