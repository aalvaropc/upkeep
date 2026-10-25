package jsonfix

import (
	"fmt"
	"strings"
)

func Repair(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString, escaped := false, false
	for _, r := range s {
		switch {
		case !inString:
			b.WriteRune(r)
			if r == '"' {
				inString = true
			}
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			b.WriteRune(r)
			escaped = true
		case r == '"':
			b.WriteRune(r)
			inString = false
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
