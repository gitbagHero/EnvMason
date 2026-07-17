package plan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The I13 Plan package may orchestrate the already-audited read-only report
// path, but it must not gain direct process, filesystem mutation, syscall or
// unsafe capabilities before I14.
func TestPlanPreviewHasNoDirectExecutionOrWriteCapability(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbiddenImports := map[string]bool{"os/exec": true, "syscall": true, "unsafe": true}
	forbiddenCalls := []string{"WriteFile(", "Create(", "CreateTemp(", "Mkdir(", "MkdirAll(", "Remove(", "RemoveAll(", "Rename(", "Chmod(", "Chown(", "Truncate(", "OpenFile("}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if forbiddenImports[name] {
				t.Errorf("%s imports forbidden capability %q", path, name)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			for _, forbidden := range forbiddenCalls {
				if selector.Sel.Name+"(" == forbidden {
					t.Errorf("%s calls forbidden write API %s", path, selector.Sel.Name)
				}
			}
			return true
		})
	}
}
