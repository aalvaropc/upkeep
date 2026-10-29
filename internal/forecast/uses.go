package forecast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const maxSymbolsListed = 40

var majorSuffix = regexp.MustCompile(`^v[0-9]+$`)
var gopkgSuffix = regexp.MustCompile(`\.v[0-9]+$`)

func packageName(importPath string) string {
	parts := strings.Split(importPath, "/")
	last := parts[len(parts)-1]
	if majorSuffix.MatchString(last) && len(parts) > 1 {
		last = parts[len(parts)-2]
	}
	return gopkgSuffix.ReplaceAllString(last, "")
}

func within(importPath, module string) bool {
	return importPath == module || strings.HasPrefix(importPath, module+"/")
}

func selectors(dir string, match func(importPath string) bool, visit func(name, importPath, sel string)) {
	if dir == "" {
		return
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			return nil
		}
		names := map[string]string{}
		for _, imp := range file.Imports {
			ip, err := strconv.Unquote(imp.Path.Value)
			if err != nil || !match(ip) {
				continue
			}
			ip = path.Clean(ip)
			name := packageName(ip)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name != "_" && name != "." {
				names[name] = ip
			}
		}
		if len(names) == 0 {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && names[id.Name] != "" && id.Obj == nil {
				visit(id.Name, names[id.Name], sel.Sel.Name)
			}
			return true
		})
		return nil
	})
}

func uses(dir, module string) []string {
	seen := map[string]bool{}
	selectors(dir, func(ip string) bool { return within(ip, module) }, func(name, _, sel string) {
		seen[name+"."+sel] = true
	})
	if len(seen) == 0 {
		return nil
	}
	symbols := make([]string, 0, len(seen))
	for s := range seen {
		symbols = append(symbols, s)
	}
	slices.Sort(symbols)
	if len(symbols) > maxSymbolsListed {
		symbols = symbols[:maxSymbolsListed]
	}
	return symbols
}
