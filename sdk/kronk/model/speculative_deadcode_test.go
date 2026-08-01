package model

import (
	"go/ast"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestSpecDraftProbsIsReachable pins findings2 §6h: the dense probabilistic
// verify path in batchgen_speculative.go is dead code.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_speculative.go:329
//     draftProbs := s.specDraftProbs
//   - sdk/kronk/model/batchgen_speculative.go:556-574
//     the `qDraft := draftProbs[i][draftToken]` residual-sampling branch
//   - sdk/kronk/model/batchgen_speculative.go:885
//     sampleAdjustedInto, only called from that branch
//
// slot.specDraftProbs (batchgen_slot.go:156) is declared as the per-position
// dense draft distribution used by rigorous speculative sampling, but every
// assignment in the package sets it to nil:
//
//	batchgen_speculative.go:184, :261, :264, :378 and batchgen_slot.go:330
//
// With no producer, draftProbs at :329 is always nil, the residual-sampling
// branch at :556-574 can never be taken, and sampleAdjustedInto at :885 is
// unreachable. The comment at :546-554 describes this branch as a live
// fallback, which is not true.
//
// This test walks the package AST rather than grepping so it survives line
// drift, and it reports the nil-only assignment sites it found so a maintainer
// can see the evidence. It passes once a producer assigns a real distribution,
// or once the field and its dead consumers are deleted (in which case the
// "field no longer exists" branch below reports that and the test should be
// removed with them).
func TestSpecDraftProbsIsReachable(t *testing.T) {
	fset, files := parseModelPackage(t)

	const field = "specDraftProbs"

	var (
		nilAssignments    []string
		nonNilAssignments []string
		referenced        bool
	)

	// assignsField reports whether lhs is a selector ending in .specDraftProbs.
	assignsField := func(lhs ast.Expr) bool {
		sel, ok := lhs.(*ast.SelectorExpr)

		return ok && sel.Sel.Name == field
	}

	isNil := func(expr ast.Expr) bool {
		ident, ok := expr.(*ast.Ident)

		return ok && ident.Name == "nil"
	}

	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				if node.Sel.Name == field {
					referenced = true
				}

			case *ast.AssignStmt:
				for i, lhs := range node.Lhs {
					if !assignsField(lhs) {
						continue
					}

					// A tuple assignment (len(Rhs) == 1, len(Lhs) > 1) cannot
					// be proven nil, so treat it as a real producer.
					if len(node.Rhs) != len(node.Lhs) || !isNil(node.Rhs[i]) {
						nonNilAssignments = append(nonNilAssignments, posOf(fset, lhs))
						continue
					}

					nilAssignments = append(nilAssignments, posOf(fset, lhs))
				}

			case *ast.KeyValueExpr:
				// Composite-literal form: slot{specDraftProbs: x}.
				key, ok := node.Key.(*ast.Ident)
				if !ok || key.Name != field {
					break
				}

				if isNil(node.Value) {
					nilAssignments = append(nilAssignments, posOf(fset, node))
					break
				}

				nonNilAssignments = append(nonNilAssignments, posOf(fset, node))
			}

			return true
		})
	}

	if !referenced {
		t.Fatalf("no reference to %s found in the package: the field was removed, "+
			"so delete this test along with it", field)
	}

	if len(nonNilAssignments) == 0 {
		t.Errorf("slot.%s is only ever assigned nil (%d site(s): %s), so the "+
			"probabilistic verify path is unreachable dead code.\n"+
			"batchgen_speculative.go:329 reads it into draftProbs, which is therefore always "+
			"nil; the residual-sampling branch at :556-574 (qDraft := draftProbs[i][draftToken]) "+
			"can never execute, and sampleAdjustedInto at :885 is never called. The comment at "+
			":546-554 describes this as a live fallback.\n"+
			"Either produce the dense draft distribution (so speculative sampling is actually "+
			"rigorous) or delete the field, the branch and sampleAdjustedInto.",
			field, len(nilAssignments), strings.Join(nilAssignments, ", "))
	}
}

// docIdentifierRefs matches "batchEngine.someMethod" style references inside a
// doc comment.
var docIdentifierRefs = regexp.MustCompile(`batchEngine\.([A-Za-z_][A-Za-z0-9_]*)`)

// TestChooseNDraftDocCommentReferencesRealMethods pins findings2 §8: the
// chooseNDraft doc comment cites a method that does not exist.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_speculative.go:133-134
//     "// The EMA is initialized to 1.0 at slot construction (see
//     //  batchEngine.newSlot) and PERSISTS across requests on the same slot,"
//
// There is no batchEngine.newSlot. Slots — including the specAccEMA: 1.0
// initialisation the comment is describing — are constructed inline in
// newBatchEngine (batchgen_engine.go:51-57). A reader chasing the cited symbol
// finds nothing, and the actual initialisation site is the one place a change
// to the EMA's starting value would have to land.
func TestChooseNDraftDocCommentReferencesRealMethods(t *testing.T) {
	fset, files := parseModelPackage(t)

	// Collect every method declared on batchEngine / *batchEngine.
	var methods []string
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}

			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}

			if ident, ok := recv.(*ast.Ident); ok && ident.Name == "batchEngine" {
				methods = append(methods, fn.Name.Name)
			}
		}
	}

	if len(methods) == 0 {
		t.Fatal("no methods found on batchEngine: the AST matcher is stale, " +
			"fix it before trusting this test")
	}

	var target *ast.FuncDecl
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "chooseNDraft" {
				target = fn
			}
		}
	}

	if target == nil {
		t.Fatal("chooseNDraft not found in the package")
	}

	if target.Doc == nil {
		t.Fatal("chooseNDraft has no doc comment")
	}

	for _, match := range docIdentifierRefs.FindAllStringSubmatch(target.Doc.Text(), -1) {
		if slices.Contains(methods, match[1]) {
			continue
		}

		t.Errorf("%s: chooseNDraft's doc comment references batchEngine.%s, "+
			"which does not exist.\n"+
			"Slots (and the specAccEMA: 1.0 initialisation this sentence describes) are "+
			"constructed inline in newBatchEngine, batchgen_engine.go:51-57. Point the "+
			"comment at newBatchEngine.",
			posOf(fset, target.Doc), match[1])
	}
}
