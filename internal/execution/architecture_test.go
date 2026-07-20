package execution

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

func TestExecutionCapabilityIsConfinedAndNeverInvokesShell(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"sh"`) || strings.Contains(string(data), `"bash"`) || strings.Contains(string(data), `"powershell"`) || strings.Contains(string(data), `"cmd.exe"`) {
			t.Errorf("%s contains a shell executable literal", path)
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
			if name == "os/exec" && path != "runner.go" && path != "process_unix.go" && path != "process_windows.go" {
				t.Errorf("%s imports os/exec outside the single runner boundary", path)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "Command" {
				t.Errorf("%s uses exec.Command instead of context-bound CommandContext", path)
			}
			return true
		})
	}
}
