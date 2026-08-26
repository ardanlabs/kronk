package modelprofile

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestConsumersDoNotInterpretArchitectureMetadata(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate boundary test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	sdkRoot := filepath.Join(root, "sdk")
	allowedDirectories := map[string]struct{}{
		filepath.Join(sdkRoot, "kronk/gguf"):         {},
		filepath.Join(sdkRoot, "kronk/modelprofile"): {},
		filepath.Join(sdkRoot, "kronk/parsers"):      {},
	}
	forbiddenCalls := map[string]struct{}{
		"DetectArchitecture":     {},
		"IsHybridArchitecture":   {},
		"IsVisionEncoder":        {},
		"ParseAttentionFacts":    {},
		"ParseRopeFacts":         {},
		"ResolveKVLengths":       {},
		"ParseInt64WithFallback": {},
	}

	err := filepath.WalkDir(sdkRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if _, allowed := allowedDirectories[path]; allowed {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		fileNode, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(fileNode, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.BasicLit:
				if node.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(node.Value)
				if err == nil && value == "general.architecture" {
					t.Errorf("%s interprets general.architecture outside modelprofile", path)
				}
			case *ast.SelectorExpr:
				if _, forbidden := forbiddenCalls[node.Sel.Name]; forbidden {
					t.Errorf("%s calls %s outside modelprofile", path, node.Sel.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", sdkRoot, err)
	}
}
