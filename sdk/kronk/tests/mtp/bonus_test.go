package mtp_test

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

// specSourceRel is the file under test, relative to the module root.
const specSourceRel = "sdk/kronk/model/batchgen_speculative.go"

// findModuleFile walks up from the working directory until it finds a
// directory containing rel, and returns the joined path. Walking beats
// runtime.Caller here because -trimpath rewrites compiled-in paths, and it
// beats GITHUB_WORKSPACE because it works for a plain `go test ./...`.
func findModuleFile(t *testing.T, rel string) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate %s by walking up from the test directory", rel)
		}
		dir = parent
	}
}

// findFunc returns the top-level function declaration named name.
func findFunc(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}

	t.Fatalf("function %s not found in %s", name, specSourceRel)

	return nil
}

// findBonusTokenBlock returns the `if accepted == nDraft { ... }` statement
// inside fn — the block that samples the bonus token after a fully accepted
// speculative round.
func findBonusTokenBlock(t *testing.T, fn *ast.FuncDecl) *ast.IfStmt {
	t.Helper()

	var found *ast.IfStmt

	ast.Inspect(fn, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}

		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || bin.Op != token.EQL {
			return true
		}

		x, xok := bin.X.(*ast.Ident)
		y, yok := bin.Y.(*ast.Ident)
		if xok && yok && x.Name == "accepted" && y.Name == "nDraft" {
			found = ifStmt
		}

		return true
	})

	if found == nil {
		t.Fatalf("could not find the `if accepted == nDraft` bonus-token block in "+
			"verifySpeculativeTokens (%s). If the block was renamed or restructured, "+
			"update this test to point at whatever now samples the bonus token.", specSourceRel)
	}

	return found
}

// callNames returns every function/method name called anywhere under n, as
// the trailing identifier of the call target (so `llama.SamplerSample` and
// `s.grammarSampler.SampleWithGrammar` come back as "SamplerSample" and
// "SampleWithGrammar").
func callNames(n ast.Node) map[string][]token.Pos {
	out := map[string][]token.Pos{}

	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fn := call.Fun.(type) {
		case *ast.Ident:
			out[fn.Name] = append(out[fn.Name], call.Pos())
		case *ast.SelectorExpr:
			out[fn.Sel.Name] = append(out[fn.Sel.Name], call.Pos())
		}

		return true
	})

	return out
}

// positions renders fset positions as "file:line" for a failure message.
func positions(fset *token.FileSet, ps []token.Pos) string {
	var out []string
	for _, p := range ps {
		pos := fset.Position(p)
		out = append(out, filepath.Base(pos.Filename)+":"+strconv.Itoa(pos.Line))
	}

	return strings.Join(out, ", ")
}

// =========================================================================

// TestMTPBonusTokenGoesThroughSampler pins the second half of the
// bonus-token bug: even when no grammar is attached, the token emitted after
// a fully accepted speculative round skips the entire sampler chain.
//
// sdk/kronk/model/batchgen_speculative.go:353-356 forces greedy = true for
// every MTP request. The `case greedy:` arm of the `if accepted == nDraft`
// block at :582-602 then does:
//
//	maskSuppressTokenLogits(targetLogits, e.model.suppressTokens)
//	bonusToken = argmax(targetLogits)          // :594-595
//
// so that token is a raw argmax over unfiltered target logits — no
// repeat/frequency/presence penalties, no DRY, no top-k/top-p/min-p, no XTC,
// no temperature, no dist sampler, no grammar. At the measured ~0.71
// acceptance rate with defMTPNDraft = 2 that is roughly 15-30% of every
// emitted token stream. The in-loop verify branch at :425-467 already does
// the right thing; this block was simply not taught the same lesson.
//
// Upstream llama.cpp routes every position including the bonus through the
// same chain — .extras/llama.cpp/common/sampling.cpp:646-674,
// common_sampler_sample_and_accept_n.
//
// This is a source-analysis test on purpose. The behavioural alternatives —
// "a repeat_penalty of 1.9 over 512 tokens should suppress repetition" or
// "high temperature should produce more diversity than this" — are all
// distributional claims over a sampled LLM, and none of them separate the
// bug from ordinary model behaviour at a sample size that fits in a test
// suite. A flaky test is worse than no test, so this asserts the invariant
// directly and deterministically instead. The grammar half of the same bug
// IS testable behaviourally and is covered by TestMTPGrammarBonusToken.
//
// NOTE: this test needs no model, but it lives in package mtp_test whose
// TestMain skips the package when no MTP GGUF is present. Move it to
// sdk/kronk/model if you want it to run on every CI machine.
func TestMTPBonusTokenGoesThroughSampler(t *testing.T) {
	src := findModuleFile(t, specSourceRel)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}

	fn := findFunc(t, file, "verifySpeculativeTokens")
	block := findBonusTokenBlock(t, fn)

	blockLine := fset.Position(block.Pos()).Line
	calls := callNames(block)

	// The invariant: nothing in the bonus-token block may pick a token by
	// raw argmax. Every emitted token must come from the slot's sampler
	// chain, exactly like the in-loop verify branch at :425-467.
	if ps, ok := calls["argmax"]; ok {
		t.Errorf(`bonus-token block at %s:%d calls argmax() at %s.

The token emitted after a fully accepted speculative round is therefore chosen
by raw argmax over unfiltered target logits: no repeat/frequency/presence
penalty, no DRY, no top-k/top-p/min-p, no XTC, no temperature, no dist sampler
and no grammar. greedy is forced true for EVERY MTP request at
%s:353-356, so this arm is taken on every fully accepted
round — roughly 15-30%% of emitted tokens at the measured ~0.71 acceptance
with defMTPNDraft = 2.

Fix: mirror the in-loop branch at %s:425-467 —

    switch {
    case s.grammarSampler != nil && s.reasonFlag == 0:
        bonusToken = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(nDraft))
    default:
        bonusToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(nDraft))
    }

Upstream does the same for every position including the bonus:
.extras/llama.cpp/common/sampling.cpp:646-674 (common_sampler_sample_and_accept_n).`,
			specSourceRel, blockLine, positions(fset, ps), specSourceRel, specSourceRel)
	}

	// And the grammar must be consulted on every path out of this block.
	if _, ok := calls["SampleWithGrammar"]; !ok {
		t.Errorf(`bonus-token block at %s:%d never calls SampleWithGrammar.

At least one path out of this block emits a token without asking the grammar
sampler, so a grammar-constrained request can emit a token the grammar forbids.
That token later reaches grammarSampler.Accept (grammar.go:436) via
handleSampledToken (batchgen_tokens.go:60), where llama_grammar_accept_token
throws an uncaught C++ std::runtime_error and aborts the process.

Every arm of this block must go through
s.grammarSampler.SampleWithGrammar when s.grammarSampler != nil &&
s.reasonFlag == 0, and llama.SamplerSample otherwise — same as :425-467.`,
			specSourceRel, blockLine)
	}

	if _, ok := calls["SamplerSample"]; !ok {
		t.Errorf(`bonus-token block at %s:%d never calls llama.SamplerSample.

The non-grammar path must still run the slot's sampler chain (penalties, DRY,
top-k/top-p/min-p, XTC, temperature, dist) rather than reading logits directly.`,
			specSourceRel, blockLine)
	}
}
