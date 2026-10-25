package gitx

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

const (
	markerOurs   = "<<<<<<<"
	markerBase   = "|||||||"
	markerSplit  = "======="
	markerTheirs = ">>>>>>>"
)

func ParseConflicts(file, text string) ([]Hunk, error) {
	lines := strings.Split(text, "\n")
	var hunks []Hunk
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], markerOurs) {
			continue
		}
		var ours, theirs, base []string
		current := &ours
		end := -1
		for j := i + 1; j < len(lines); j++ {
			line := lines[j]
			if strings.HasPrefix(line, markerTheirs) {
				end = j
				break
			}
			switch {
			case strings.HasPrefix(line, markerBase):
				current = &base
			case line == markerSplit:
				current = &theirs
			default:
				*current = append(*current, line)
			}
		}
		if end < 0 {
			return nil, fmt.Errorf("%s:%d: the conflict opened here is never closed", file, i+1)
		}
		hunks = append(hunks, Hunk{
			File:   file,
			Start:  i + 1,
			Lines:  len(ours) + len(theirs),
			Ours:   strings.Join(ours, "\n"),
			Theirs: strings.Join(theirs, "\n"),
		})
		i = end
	}
	return hunks, nil
}

func Resolve(text string, resolutions []string) (string, error) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	used := 0
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], markerOurs) {
			out = append(out, lines[i])
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], markerTheirs) {
				end = j
				break
			}
		}
		if end < 0 {
			return "", fmt.Errorf("line %d: the conflict opened here is never closed", i+1)
		}
		if used >= len(resolutions) {
			return "", fmt.Errorf("the file has more conflicts than the %d resolutions given", len(resolutions))
		}
		if r := strings.TrimSuffix(resolutions[used], "\n"); r != "" {
			out = append(out, trimEcho(out, untilNextConflict(lines[end+1:]), r))
		}
		used++
		i = end
	}
	if used != len(resolutions) {
		return "", fmt.Errorf("%d resolutions were given for %d conflicts", len(resolutions), used)
	}
	return strings.Join(out, "\n"), nil
}

func HasMarkers(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, markerOurs) || strings.HasPrefix(line, markerTheirs) || line == markerSplit {
			return true
		}
	}
	return false
}

const minEcho = 2

func untilNextConflict(lines []string) []string {
	for i, l := range lines {
		if strings.HasPrefix(l, markerOurs) {
			return lines[:i]
		}
	}
	return lines
}

func trimEcho(before, after []string, resolution string) string {
	res := strings.Split(resolution, "\n")
	for k := min(len(before), len(res)-1); k >= minEcho; k-- {
		if slices.Equal(before[len(before)-k:], res[:k]) && meaningful(res[:k]) {
			res = res[k:]
			break
		}
	}
	for k := min(len(after), len(res)-1); k >= minEcho; k-- {
		if slices.Equal(after[:k], res[len(res)-k:]) && meaningful(res[len(res)-k:]) {
			res = res[:len(res)-k]
			break
		}
	}
	return strings.Join(res, "\n")
}

func meaningful(lines []string) bool {
	for _, l := range lines {
		for _, r := range l {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return true
			}
		}
	}
	return false
}
