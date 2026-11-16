package runtimegrants

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ProductionContext is the build of the container image: linux/amd64,
// CGO disabled, no build tags (infra/docker/Dockerfile). Test files and
// integration-tagged files are therefore never part of the inventory.
func ProductionContext() build.Context {
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "linux", "amd64", false
	ctx.BuildTags, ctx.ToolTags = nil, append([]string(nil), build.Default.ToolTags...)
	return ctx
}

// Package is one module package in a main package's import closure.
type Package struct {
	ImportPath string
	Dir        string
	Files      []string
	// EmbeddedSQL lists `.sql` files the package embeds; their statements are
	// not Go literals, so ScanPackage refuses them unless they are the
	// migration chain, which only the owner runs.
	EmbeddedSQL []string
}

// MigrationChain is the one package whose embedded SQL never runs as the
// runtime login.
const MigrationChain = "/db/migrations"

// Closure returns the module packages that the named main packages import,
// transitively, under ctx, sorted by import path. Standard-library and
// third-party packages carry no application SQL and are excluded.
func Closure(ctx build.Context, moduleRoot, modulePath string, mains ...string) ([]Package, error) {
	if len(mains) == 0 {
		return nil, errors.New("runtimegrants: no main package")
	}
	seen := map[string]*Package{}
	var visit func(string) error
	visit = func(path string) error {
		if seen[path] != nil {
			return nil
		}
		if path != modulePath && !strings.HasPrefix(path, modulePath+"/") {
			return nil
		}
		dir := filepath.Join(moduleRoot, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(path, modulePath), "/")))
		pkg, err := ctx.ImportDir(dir, 0)
		if err != nil {
			return fmt.Errorf("runtimegrants: import %s: %w", path, err)
		}
		entry := &Package{ImportPath: path, Dir: dir}
		for _, name := range pkg.GoFiles {
			entry.Files = append(entry.Files, filepath.Join(dir, name))
		}
		sort.Strings(entry.Files)
		for _, pattern := range pkg.EmbedPatterns {
			matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pattern)))
			if err != nil {
				return fmt.Errorf("runtimegrants: embed pattern %s: %w", pattern, err)
			}
			for _, match := range matches {
				if strings.HasSuffix(match, ".sql") {
					entry.EmbeddedSQL = append(entry.EmbeddedSQL, match)
				}
			}
		}
		sort.Strings(entry.EmbeddedSQL)
		seen[path] = entry
		for _, imported := range pkg.Imports {
			if err := visit(imported); err != nil {
				return err
			}
		}
		return nil
	}
	for _, main := range mains {
		if err := visit(modulePath + "/" + strings.TrimPrefix(main, "./")); err != nil {
			return nil, err
		}
	}
	out := make([]Package, 0, len(seen))
	for _, pkg := range seen {
		out = append(out, *pkg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ImportPath < out[j].ImportPath })
	return out, nil
}

// ScanPackage adds every string constant expression of the package's files
// to u: literals, constant concatenations and same-package string constants.
func ScanPackage(u *Usage, pkg Package) error {
	if len(pkg.EmbeddedSQL) > 0 && !strings.HasSuffix(pkg.ImportPath, MigrationChain) {
		return fmt.Errorf("runtimegrants: %s embeds SQL files that the inventory does not analyse", pkg.ImportPath)
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(pkg.Files))
	for _, name := range pkg.Files {
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("runtimegrants: parse %s: %w", name, err)
		}
		files = append(files, file)
	}
	constants := map[string]ast.Expr{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if i < len(value.Values) {
						constants[name.Name] = value.Values[i]
					}
				}
			}
		}
	}
	folder := folder{constants: constants, active: map[string]bool{}}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			expr, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			switch e := expr.(type) {
			case *ast.BasicLit:
				if text, ok := folder.fold(e); ok {
					u.AddSQL(text)
				}
			case *ast.BinaryExpr:
				if e.Op != token.ADD {
					return true
				}
				if text, ok := folder.fold(e); ok {
					u.AddSQL(text)
					return false
				}
				// A partly dynamic concatenation keeps its constant pieces
				// together so a statement split around a variable is still
				// analysed as one statement.
				if pieces := folder.pieces(e); len(pieces) > 1 {
					u.AddSQL(strings.Join(pieces, " "))
				}
			case *ast.Ident:
				if text, ok := folder.fold(e); ok {
					u.AddSQL(text)
				}
			}
			return true
		})
	}
	return nil
}

type folder struct {
	constants map[string]ast.Expr
	active    map[string]bool
}

func (f folder) fold(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(e.Value)
		return text, err == nil
	case *ast.ParenExpr:
		return f.fold(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := f.fold(e.X)
		if !ok {
			return "", false
		}
		right, ok := f.fold(e.Y)
		return left + right, ok
	case *ast.Ident:
		value, ok := f.constants[e.Name]
		if !ok || f.active[e.Name] {
			return "", false
		}
		f.active[e.Name] = true
		defer delete(f.active, e.Name)
		return f.fold(value)
	}
	return "", false
}

func (f folder) pieces(expr ast.Expr) []string {
	if e, ok := expr.(*ast.BinaryExpr); ok && e.Op == token.ADD {
		if text, ok := f.fold(e); ok {
			return []string{text}
		}
		return append(f.pieces(e.X), f.pieces(e.Y)...)
	}
	if text, ok := f.fold(expr); ok {
		return []string{text}
	}
	return nil
}
