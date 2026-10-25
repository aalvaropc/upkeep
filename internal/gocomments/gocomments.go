package gocomments

import (
	"go/scanner"
	"go/token"
	"strings"
)

const embedDirective = "//go:embed "

func Count(src []byte) int {
	file := token.NewFileSet().AddFile("", -1, len(src))
	var sc scanner.Scanner
	sc.Init(file, src, func(token.Position, string) {}, scanner.ScanComments)
	n := 0
	for {
		_, tok, lit := sc.Scan()
		if tok == token.EOF {
			return n
		}
		if tok == token.COMMENT && !strings.HasPrefix(lit, embedDirective) {
			n++
		}
	}
}

func Added(before, after []byte) int {
	return max(0, Count(after)-Count(before))
}
