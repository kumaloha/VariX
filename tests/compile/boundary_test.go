package compile_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovedCompilePackageDoesNotExist(t *testing.T) {
	root := repositoryRoot(t)
	for _, removed := range []string{"compilev2", "compilelegacy"} {
		if _, err := os.Stat(filepath.Join(root, removed)); err == nil {
			t.Fatalf("%s package directory still exists; current compile and verify code should use semantic package boundaries", removed)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s directory: %v", removed, err)
		}
	}
}

func TestNoCodeImportsRemovedCompilePackage(t *testing.T) {
	root := repositoryRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".gocache", ".tmp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range parsed.Imports {
			switch strings.Trim(imp.Path.Value, `"`) {
			case "github.com/kumaloha/VariX/varix/compilev2",
				"github.com/kumaloha/VariX/varix/compilelegacy":
				t.Fatalf("%s imports removed compile package %s", shortPath(root, path), strings.Trim(imp.Path.Value, `"`))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo imports: %v", err)
	}
}

func TestVerifyOwnsVerificationRuntimeAPI(t *testing.T) {
	root := repositoryRoot(t)
	compileDir := filepath.Join(root, "compile")
	verifyRuntimeNames := map[string]struct{}{
		"NewClientFromConfigNoVerify":           {},
		"NewClientFromConfigNoVerifyNoValidate": {},
		"EnableFactWebVerification":             {},
		"NewVerificationService":                {},
	}

	files, err := filepath.Glob(filepath.Join(compileDir, "*.go"))
	if err != nil {
		t.Fatalf("glob compile files: %v", err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, ok := verifyRuntimeNames[fn.Name.Name]; ok {
				t.Fatalf("compile package still declares verify runtime API %s in %s", fn.Name.Name, shortPath(root, file))
			}
		}
	}
}

func TestRuntimePackagesDoNotUseCompileAsModelUmbrella(t *testing.T) {
	root := repositoryRoot(t)
	for _, dir := range []string{"storage/contentstore", "memory", "server", "verify"} {
		assertDirDoesNotImport(t, root, dir, "github.com/kumaloha/VariX/varix/compile")
	}
}

func assertDirDoesNotImport(t *testing.T, root string, relDir string, forbidden string) {
	t.Helper()
	dir := filepath.Join(root, relDir)
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range parsed.Imports {
			if strings.Trim(imp.Path.Value, `"`) == forbidden {
				t.Fatalf("%s imports %s; shared DTOs belong in model, not compile", shortPath(root, path), forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s imports: %v", relDir, err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func shortPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
