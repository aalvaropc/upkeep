package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var version string

func Version() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func Revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if rev != "" && dirty {
		rev += "+dirty"
	}
	return rev
}

func String() string {
	s := "upkeep " + Version()
	if rev := Revision(); rev != "" {
		s += " (" + rev + ")"
	}
	return fmt.Sprintf("%s %s %s/%s", s, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
