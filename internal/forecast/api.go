package forecast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

const (
	SourceAPI       = "api"
	apiTimeout      = 3 * time.Minute
	maxDeclChars    = 240
	declFunc        = "func"
	declType        = "type"
	declValue       = "value"
	removedFormat   = "removed in %s"
	changedFormat   = "%s changed: %s → %s"
	changedInFormat = "%s changed in %s"
)

type reference struct {
	Symbol string
	Sub    string
	Ident  string
}

type decl struct {
	kind string
	key  string
	show string
}

type downloaded struct {
	Path    string
	Version string
	Dir     string
	Error   string
}

func modulePath(module, version string) string {
	v, err := pull.ParseVersion(version)
	if err != nil || gopkgSuffix.MatchString(module) || strings.HasSuffix(version, "+incompatible") {
		return module
	}
	base := module
	if i := strings.LastIndex(module, "/"); i >= 0 && majorSuffix.MatchString(module[i+1:]) {
		base = module[:i]
	}
	if v.Major >= 2 {
		return base + "/v" + strconv.Itoa(v.Major)
	}
	return base
}

func canonical(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func rootOf(importPath string, roots []string) string {
	best := ""
	for _, r := range roots {
		if within(importPath, r) && len(r) > len(best) {
			best = r
		}
	}
	return best
}

func references(dir string, roots ...string) []reference {
	seen := map[reference]bool{}
	selectors(dir, func(ip string) bool { return rootOf(ip, roots) != "" }, func(name, ip, sel string) {
		sub := strings.TrimPrefix(strings.TrimPrefix(ip, rootOf(ip, roots)), "/")
		seen[reference{Symbol: name + "." + sel, Sub: sub, Ident: sel}] = true
	})
	out := make([]reference, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b reference) int {
		if c := strings.Compare(a.Symbol, b.Symbol); c != 0 {
			return c
		}
		return strings.Compare(a.Sub, b.Sub)
	})
	return out
}

func hostPath(exec sandbox.Executor, dir string) string {
	if m, ok := exec.(interface{ HostPath(string) string }); ok {
		return m.HostPath(dir)
	}
	return dir
}

func modules(output string) []downloaded {
	var out []downloaded
	rest := output
	for {
		i := strings.Index(rest, "{")
		if i < 0 {
			return out
		}
		dec := json.NewDecoder(strings.NewReader(rest[i:]))
		var m downloaded
		if err := dec.Decode(&m); err != nil {
			rest = rest[i+1:]
			continue
		}
		if m.Path != "" {
			out = append(out, m)
		}
		rest = rest[i+int(dec.InputOffset()):]
	}
}

func (f Forecaster) download(ctx context.Context, wanted ...string) (map[string]string, error) {
	tmp, err := os.MkdirTemp("", "upkeep-api-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	res, err := f.Executor.Run(ctx, sandbox.Spec{
		Dir:     tmp,
		Args:    append([]string{"go", "mod", "download", "-json"}, wanted...),
		Network: true,
		Timeout: apiTimeout,
	})
	if err != nil {
		return nil, err
	}
	if res.TimedOut {
		return nil, errors.New("go mod download timed out")
	}
	found := map[string]string{}
	for _, m := range modules(res.Output) {
		if m.Error != "" {
			return nil, errors.New(m.Error)
		}
		if m.Dir != "" {
			found[m.Path+"@"+m.Version] = hostPath(f.Executor, m.Dir)
		}
	}
	for _, w := range wanted {
		if found[w] == "" {
			if line := strings.TrimSpace(firstLine(res.Output)); line != "" {
				return nil, fmt.Errorf("go mod download did not return %s: %s", w, line)
			}
			return nil, fmt.Errorf("go mod download did not return %s", w)
		}
	}
	return found, nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

func (f Forecaster) fromAPI(ctx context.Context, in Input, partial Forecast) (Forecast, error) {
	from, to := canonical(in.From), canonical(in.To)
	fromPath, toPath := modulePath(in.Module, from), modulePath(in.Module, to)
	refs := references(in.Dir, fromPath, toPath)
	if len(refs) == 0 {
		return partial, errors.New("the project uses no identifier from it")
	}
	fromMod, toMod := fromPath+"@"+from, toPath+"@"+to
	dirs, err := f.download(ctx, fromMod, toMod)
	if err != nil {
		return partial, err
	}
	changes, compared := compareAPI(dirs[fromMod], dirs[toMod], to, refs)
	if compared == 0 {
		return partial, fmt.Errorf("none of the identifiers the project uses was found in %s", from)
	}
	out := partial
	out.Source = SourceAPI
	out.Breaking = changes
	out.Summary = fmt.Sprintf("No release notes or migration guide were found, so the exported API of %s %s and %s was compared for the %d identifiers the project uses from it; %d changed.", in.Module, from, to, compared, len(changes))
	out.Files = importers(in.Dir, in.Module)
	out.Sources = []string{}
	out.FetchedAt = f.now().UTC()
	return out, nil
}

func compareAPI(fromDir, toDir, to string, refs []reference) ([]Change, int) {
	type api struct {
		decls   map[string]decl
		missing bool
		err     error
	}
	load := func(root, sub string, cache map[string]api) api {
		if a, ok := cache[sub]; ok {
			return a
		}
		dir := filepath.Join(root, filepath.FromSlash(sub))
		decls, err := exported(dir)
		a := api{decls: decls, err: err}
		if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
			a = api{decls: map[string]decl{}, missing: true}
		}
		cache[sub] = a
		return a
	}
	olds, news := map[string]api{}, map[string]api{}
	changes := []Change{}
	compared := 0
	for _, r := range refs {
		old := load(fromDir, r.Sub, olds)
		if old.err != nil || old.missing {
			continue
		}
		before, ok := old.decls[r.Ident]
		if !ok {
			continue
		}
		cur := load(toDir, r.Sub, news)
		if cur.err != nil {
			continue
		}
		compared++
		after, ok := cur.decls[r.Ident]
		if !ok {
			changes = append(changes, Change{Symbol: r.Symbol, Description: fmt.Sprintf(removedFormat, to)})
			continue
		}
		if d := describe(before, after, to); d != "" {
			changes = append(changes, Change{Symbol: r.Symbol, Description: d})
		}
	}
	return changes, compared
}

func describe(before, after decl, to string) string {
	if before.kind == after.kind && (before.key == "" || after.key == "" || before.key == after.key) {
		return ""
	}
	what := "signature"
	switch {
	case before.kind != after.kind:
		what = "declaration"
	case before.kind == declType:
		what = "definition"
	case before.kind == declValue:
		what = "type"
	}
	if len(before.show) > maxDeclChars || len(after.show) > maxDeclChars {
		return fmt.Sprintf(changedInFormat, what, to)
	}
	return fmt.Sprintf(changedFormat, what, before.show, after.show)
}

func exported(dir string) (map[string]decl, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	byPackage := map[string][]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, name); err != nil || !ok {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		byPackage[file.Name.Name] = append(byPackage[file.Name.Name], file)
	}
	pkg := ""
	for name, files := range byPackage {
		if name == "main" {
			continue
		}
		if pkg == "" || len(files) > len(byPackage[pkg]) || (len(files) == len(byPackage[pkg]) && name < pkg) {
			pkg = name
		}
	}
	if pkg == "" {
		return nil, fmt.Errorf("no Go package in %s", path.Base(filepath.ToSlash(dir)))
	}
	out := map[string]decl{}
	for _, file := range byPackage[pkg] {
		for _, d := range file.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					out[d.Name.Name] = funcDecl(d)
				}
			case *ast.GenDecl:
				genDecl(d, out)
			}
		}
	}
	return out, nil
}

func funcDecl(d *ast.FuncDecl) decl {
	tparams := fieldList(d.Type.TypeParams)
	full := &ast.FuncType{Params: d.Type.Params, Results: d.Type.Results}
	bare := &ast.FuncType{Params: unnamed(d.Type.Params), Results: unnamed(d.Type.Results)}
	head := "func " + d.Name.Name + tparams
	return decl{
		kind: declFunc,
		key:  normalized(head + strings.TrimPrefix(types.ExprString(bare), "func")),
		show: head + strings.TrimPrefix(types.ExprString(full), "func"),
	}
}

func genDecl(d *ast.GenDecl, out map[string]decl) {
	var last ast.Expr
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			sep := " "
			if s.Assign.IsValid() {
				sep = " = "
			}
			show := "type " + s.Name.Name + fieldList(s.TypeParams) + sep + types.ExprString(exportedFields(s.Type))
			out[s.Name.Name] = decl{kind: declType, key: normalized(show), show: show}
		case *ast.ValueSpec:
			typ := s.Type
			if d.Tok == token.CONST {
				switch {
				case s.Type != nil:
					last = s.Type
				case len(s.Values) > 0:
					last = nil
				}
				typ = last
			}
			for _, n := range s.Names {
				if !n.IsExported() {
					continue
				}
				show := d.Tok.String() + " " + n.Name
				key := ""
				if typ != nil {
					show += " " + types.ExprString(typ)
					key = normalized(types.ExprString(typ))
				}
				out[n.Name] = decl{kind: declValue, key: key, show: show}
			}
		}
	}
}

func normalized(s string) string {
	return strings.ReplaceAll(s, "interface{}", "any")
}

func unnamed(list *ast.FieldList) *ast.FieldList {
	if list == nil {
		return nil
	}
	out := &ast.FieldList{}
	for _, f := range list.List {
		for range max(len(f.Names), 1) {
			out.List = append(out.List, &ast.Field{Type: f.Type})
		}
	}
	return out
}

func fieldList(list *ast.FieldList) string {
	if list == nil || len(list.List) == 0 {
		return ""
	}
	parts := make([]string, 0, len(list.List))
	for _, f := range list.List {
		names := make([]string, 0, len(f.Names))
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		parts = append(parts, strings.TrimSpace(strings.Join(names, ", ")+" "+types.ExprString(f.Type)))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func exportedFields(expr ast.Expr) ast.Expr {
	st, ok := expr.(*ast.StructType)
	if !ok || st.Fields == nil {
		return expr
	}
	kept := &ast.FieldList{}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			kept.List = append(kept.List, f)
			continue
		}
		var names []*ast.Ident
		for _, n := range f.Names {
			if n.IsExported() {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			kept.List = append(kept.List, &ast.Field{Names: names, Type: f.Type})
		}
	}
	return &ast.StructType{Fields: kept}
}
