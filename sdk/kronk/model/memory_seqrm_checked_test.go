package model

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// parseModelPackage parses every non-test .go file in this package's directory
// and returns the file set plus the parsed files, sorted by name for stable
// output. Comments are retained so doc-comment assertions can use the result.
//
// `go test` runs with the working directory set to the package directory, so
// "." is this package's source directory.
func parseModelPackage(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)

	if len(names) == 0 {
		t.Fatal("no non-test .go files found in the package directory")
	}

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
	}

	return fset, files
}

// isLlamaCall reports whether call is a call to llama.<name>.
func isLlamaCall(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}

	pkg, ok := sel.X.(*ast.Ident)

	return ok && pkg.Name == "llama"
}

// isNegativeOne reports whether expr is the literal -1, including when it is
// wrapped in a single-argument conversion such as llama.Pos(-1).
func isNegativeOne(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return isNegativeOne(e.X)

	case *ast.CallExpr:
		// Conversion, e.g. llama.Pos(-1).
		if len(e.Args) != 1 {
			return false
		}
		return isNegativeOne(e.Args[0])

	case *ast.UnaryExpr:
		if e.Op != token.SUB {
			return false
		}
		lit, ok := e.X.(*ast.BasicLit)

		return ok && lit.Kind == token.INT && lit.Value == "1"
	}

	return false
}

// posOf renders a node's position as file:line relative to the package
// directory.
func posOf(fset *token.FileSet, n ast.Node) string {
	p := fset.Position(n.Pos())

	return fmt.Sprintf("%s:%d", filepath.Base(p.Filename), p.Line)
}

// TestMemorySeqRmPartialRangeResultIsChecked pins findings2 §2.
//
// llama.MemorySeqRm returns (bool, error). Its yzma doc comment
// (yzma pkg/llama/memory.go:101-105) states:
//
//	"Returns false if a partial sequence cannot be removed.
//	 Removing a whole sequence never fails."
//
// On a hybrid target llama_memory_hybrid::seq_rm consults the recurrent cache
// first; llama_memory_recurrent::seq_rm refuses a mid-sequence range unless
// n_rs_seq > 0 and the hybrid wrapper then returns false having mutated
// NOTHING — not even the attention KV. A partial-range call whose result is
// discarded therefore silently leaves rejected draft tokens in the KV while the
// slot's nPast rewinds past them. llama.cpp appends rather than overwrites by
// (seq, pos), so the sequence carries phantom tokens for the rest of the
// request: a repeat-loop generator.
//
// This test locates the calls by AST rather than by line number so it survives
// line drift. Whole-sequence wipes (p0 == -1 && p1 == -1) are exempt because
// the documented contract says they cannot fail.
//
// Today the offending sites are the partial-range calls in
// batchgen_speculative.go (draft-KV trims and the hybrid rollback fallback) and
// batchgen_finish.go (the draft-KV trim on slot finish). Each must capture the
// bool and act on false — fail the slot or disable speculation for the request.
func TestMemorySeqRmPartialRangeResultIsChecked(t *testing.T) {
	fset, files := parseModelPackage(t)

	var (
		discardedPartial []string
		exemptWholeSeq   []string
		allCallSites     []string
	)

	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			// An ExprStmt wrapping the call means both return values were
			// discarded; anywhere else (assignment, if-statement init, ...)
			// the result is at least bound to something.
			stmt, ok := n.(*ast.ExprStmt)
			if !ok {
				return true
			}

			call, ok := stmt.X.(*ast.CallExpr)
			if !ok || !isLlamaCall(call, "MemorySeqRm") {
				return true
			}

			// Signature: MemorySeqRm(mem, seqID, p0, p1).
			if len(call.Args) == 4 && isNegativeOne(call.Args[2]) && isNegativeOne(call.Args[3]) {
				exemptWholeSeq = append(exemptWholeSeq, posOf(fset, call))
				return true
			}

			discardedPartial = append(discardedPartial, posOf(fset, call))

			return true
		})

		// Count every appearance so the test fails loudly if the matcher
		// stops recognising the call entirely.
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isLlamaCall(call, "MemorySeqRm") {
				return true
			}
			allCallSites = append(allCallSites, posOf(fset, call))

			return true
		})
	}

	if len(allCallSites) == 0 {
		t.Fatal("no llama.MemorySeqRm call sites found: the AST matcher is stale, " +
			"fix it before trusting this test")
	}

	if len(exemptWholeSeq) == 0 {
		t.Errorf("no whole-sequence llama.MemorySeqRm(mem, seq, -1, -1) call sites found; " +
			"the -1/-1 exemption is untested, so the matcher may be misclassifying calls")
	}

	if len(discardedPartial) > 0 {
		t.Errorf("llama.MemorySeqRm called with a partial range and both return values "+
			"discarded at %d site(s):\n\t%s\n\n"+
			"MemorySeqRm returns (bool, error) and the bool is false when a partial range "+
			"cannot be removed. On a hybrid target (llama_memory_hybrid::seq_rm) that false "+
			"is returned after mutating nothing, so the rejected draft tokens stay in the KV "+
			"while nPast rewinds past them and the sequence is silently corrupted.\n"+
			"Each site must bind the bool and handle false (fail the slot or disable "+
			"speculation for the request). Whole-sequence wipes (-1, -1) are exempt: "+
			"per the yzma contract they never fail.\n"+
			"Exempt whole-sequence sites seen: %s",
			len(discardedPartial), strings.Join(discardedPartial, "\n\t"),
			strings.Join(exemptWholeSeq, ", "))
	}
}
