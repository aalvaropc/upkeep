package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	found, err := scan(roots, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if found > 0 {
		fmt.Fprintf(os.Stderr, "%d comment(s) found; only //go:embed directives are allowed\n", found)
		os.Exit(1)
	}
}

func scan(roots []string, out io.Writer) (int, error) {
	fset := token.NewFileSet()
	found := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, group := range file.Comments {
				for _, c := range group.List {
					if allowed(c.Text) {
						continue
					}
					found++
					fmt.Fprintf(out, "%s: %s\n", fset.Position(c.Pos()), firstLine(c.Text))
				}
			}
			return nil
		})
		if err != nil {
			return found, err
		}
	}
	return found, nil
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor"
}

func allowed(text string) bool {
	return strings.HasPrefix(text, "//go:embed ")
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
