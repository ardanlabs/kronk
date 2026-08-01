package model

// =============================================================================
// FFI POINTER / WIDTH SAFETY AUDIT
//
// yzma is a hand-written purego + libffi binding, NOT cgo. None of cgo's
// protections apply: there is no cgocheck, no automatic pinning, no compiler
// validation of what is handed across the boundary. Every width and lifetime
// rule is enforced only by hand, and a violation is invisible until it
// corrupts the heap.
//
// This file audits ONE rule family: the width contract on values whose
// ADDRESS is handed to libffi. libffi does not know the size of the Go
// variable behind a pointer; it uses only the ffi.Type recorded in the cif at
// Lib.Prep time.
//
//   RULE 1 (arguments). libffi reads exactly ffi.Type.Size bytes from each
//   avalue pointer. If the declared ffi type is WIDER than the Go variable,
//   libffi reads past the end of that Go object and the argument C receives
//   has adjacent Go heap memory in its high bytes.
//
//   RULE 2 (return values). For integer return types narrower than a register,
//   libffi stores a full ffi_arg (8 bytes on 64-bit) into the return buffer.
//   The binding documents this prohibition itself, at
//   $GOMODCACHE/github.com/jupiterrider/ffi@v0.7.0/fun.go:19-20:
//
//       "ret is a pointer to a variable that will hold the result of the
//        function call. [...] You cannot use integer types smaller than 8
//        bytes here (float32 and structs are not affected). Use [Arg] instead
//        and typecast afterwards."
//
//   Violating RULE 2 writes 4 bytes past the end of a Go variable.
//
// TestFFILibffiWidthContract proves both rules empirically against libc, with
// no llama.cpp library, no kronk.Init and no model load, so the mechanism is
// established independently of any model. The two Test...Yzma... functions then
// pin the two places in the COMPILED yzma where the rules are broken.
//
// WHICH yzma IS AUDITED. go.mod pins github.com/hybridgroup/yzma v1.21.0 and
// there is no replace directive and no go.work, so the code that actually
// compiles lives in the module cache, NOT in .extras/yzma. The .extras copy is
// a reference checkout and currently carries an uncommitted local patch (an
// added pMin parameter in pkg/llama/draft.go) that is not built. These tests
// therefore resolve the module directory with `go list -m` and assert over
// that, never over .extras.
//
// SCOPE. Struct LAYOUT (Go mirrors vs include/llama.h) is covered by
// ffi_struct_layout_test.go. This file is about the width of individual scalar
// values crossing the boundary, which layout equality does not catch.
// =============================================================================

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// ffiPtrLibcName returns the shared library that exports the C89 functions the
// width proofs use (abs, memchr), or "" when the platform is not one we know
// how to name.
func ffiPtrLibcName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libSystem.B.dylib"
	case "linux":
		return "libc.so.6"
	case "freebsd", "netbsd":
		return "libc.so.7"
	case "windows":
		return "msvcrt.dll"
	default:
		return ""
	}
}

// TestFFILibffiWidthContract is the executable evidence for RULE 1 and RULE 2
// described in the file header. It calls two libc functions through the exact
// same ffi.Lib/ffi.Fun machinery yzma uses, so whatever it shows about libffi
// is true of every yzma binding.
//
// It deliberately depends on NOTHING from llama.cpp: no library load, no
// kronk.Init, no GGUF. That matters because it makes the mechanism behind the
// two yzma defects reproducible on any machine in milliseconds.
//
// Both subtests assert libffi's DOCUMENTED behaviour, so they are expected to
// PASS. They exist to prove the rule is real, which is what makes the two
// pinning tests below more than an argument from the documentation.
func TestFFILibffiWidthContract(t *testing.T) {
	name := ffiPtrLibcName()
	if name == "" {
		t.Skipf("no known libc name for GOOS=%s", runtime.GOOS)
	}

	lib, err := ffi.Load(name)
	if err != nil {
		t.Skipf("cannot dlopen %s: %v", name, err)
	}

	// -------------------------------------------------------------------
	// RULE 2: a narrow integer return buffer is overwritten to 8 bytes.
	//
	// int abs(int) returns int32. We hand libffi an 8-byte return buffer
	// pre-filled with a recognisable pattern and check how much of it
	// libffi replaces. A correct 4-byte store would leave the high half
	// intact; an ffi_arg-sized store clears it.
	//
	// An 8-byte buffer is used (rather than a real int32 next to a
	// sentinel) so the observation does not depend on Go's field packing
	// or on unaligned-store behaviour.
	// -------------------------------------------------------------------
	t.Run("narrow_integer_return_writes_eight_bytes", func(t *testing.T) {
		absFn, err := lib.Prep("abs", &ffi.TypeSint32, &ffi.TypeSint32)
		if err != nil {
			t.Skipf("prep abs: %v", err)
		}

		const pattern = uint64(0xDEADBEEF00000000)

		box := pattern
		in := int32(-5)
		absFn.Call(unsafe.Pointer(&box), &in)

		if got := uint32(box); got != 5 {
			t.Fatalf("abs(-5) low word = %d, want 5 (the call itself did not work)", got)
		}

		high := uint32(box >> 32)
		switch high {
		case 0:
			t.Logf("confirmed RULE 2: libffi cleared the high 4 bytes "+
				"(buffer 0x%016X -> 0x%016X), i.e. it stored a full 8-byte "+
				"ffi_arg into what a caller may believe is a 4-byte slot", pattern, box)
		case uint32(pattern >> 32):
			t.Errorf("RULE 2 did NOT reproduce: libffi left the high 4 bytes "+
				"untouched (0x%016X). If this platform really only writes 4 "+
				"bytes for a Sint32 return, the LoadModeFromStr finding pinned "+
				"by TestFFIYzmaLoadModeFromStrReturnBufferWidth is harmless "+
				"here and that test's premise needs revisiting.", box)
		default:
			t.Errorf("unexpected return buffer 0x%016X after abs(-5)", box)
		}
	})

	// -------------------------------------------------------------------
	// RULE 1: a size_t argument backed by a 4-byte Go variable.
	//
	// void *memchr(const void *s, int c, size_t n) is the cleanest probe
	// available: it makes the value of n directly observable through the
	// return value, because it stops at the first match. We place the
	// needle beyond the intended limit, so "found" can only mean C
	// received an n larger than we intended to pass.
	//
	// This is precisely the shape of yzma's ModelDesc (see
	// TestFFIYzmaModelDescSizeArgWidth): a size_t parameter declared to
	// libffi as TypeUint64 but passed the address of an int32.
	// -------------------------------------------------------------------
	t.Run("narrow_size_arg_reads_adjacent_go_memory", func(t *testing.T) {
		memchr, err := lib.Prep("memchr",
			&ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint64)
		if err != nil {
			t.Skipf("prep memchr: %v", err)
		}

		// 64-byte haystack; the needle sits at 40, far beyond the limit
		// of 8 we intend to pass, but still inside the allocation, so C
		// always stops before leaving the buffer.
		const (
			hayLen    = 64
			needleAt  = 40
			intendedN = 8
		)
		hay := make([]byte, hayLen)
		hay[needleAt] = 0xAA
		hayPtr := unsafe.SliceData(hay)
		needle := int32(0xAA)

		// Control: an honest 8-byte length. C must not see the needle.
		var wide uint64 = intendedN
		var got *byte
		memchr.Call(unsafe.Pointer(&got), unsafe.Pointer(&hayPtr), &needle, &wide)
		if got != nil {
			t.Fatalf("control failed: memchr with a real uint64 n=%d found the "+
				"needle at %d; the probe itself is wrong", intendedN, needleAt)
		}

		// yzma's shape: the length variable is only 4 bytes wide, and the
		// 4 bytes of Go memory that follow it are non-zero.
		narrow := [2]uint32{intendedN, 0x11111111}
		got = nil
		memchr.Call(unsafe.Pointer(&got), unsafe.Pointer(&hayPtr), &needle, &narrow[0])

		if got == nil {
			t.Errorf("RULE 1 did NOT reproduce: memchr honoured n=%d even though "+
				"only a 4-byte Go variable was supplied for a TypeUint64 "+
				"parameter. If that is genuinely true on this platform, the "+
				"ModelDesc finding pinned by TestFFIYzmaModelDescSizeArgWidth "+
				"is harmless here and its premise needs revisiting.", intendedN)
			return
		}

		t.Logf("confirmed RULE 1: memchr found the needle at offset %d despite "+
			"an intended n=%d, because libffi read 8 bytes from a 4-byte Go "+
			"variable and C therefore saw n=0x%08X%08X",
			needleAt, intendedN, narrow[1], narrow[0])

		runtime.KeepAlive(hay)
	})
}

// =============================================================================
// Source assertions over the COMPILED yzma.
// =============================================================================

// ffiPtrYzmaLlamaDir returns the directory of the yzma pkg/llama package that
// this module actually compiles against, resolved through `go list -m` so it
// tracks the go.mod pin rather than the uncompiled .extras/yzma copy. It skips
// the test when the toolchain or the module cache is unavailable.
func ffiPtrYzmaLlamaDir(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot determine this test file's location")
	}

	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/hybridgroup/yzma")
	cmd.Dir = filepath.Dir(thisFile)
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list -m github.com/hybridgroup/yzma: %v", err)
	}

	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Skip("go list -m returned no directory for github.com/hybridgroup/yzma")
	}

	llamaDir := filepath.Join(dir, "pkg", "llama")
	if _, err := os.Stat(llamaDir); err != nil {
		t.Skipf("yzma pkg/llama not readable at %s: %v", llamaDir, err)
	}

	return llamaDir
}

// ffiPtrParseFile parses one yzma source file.
func ffiPtrParseFile(t *testing.T, dir, base string) (*token.FileSet, *ast.File) {
	t.Helper()

	fset := token.NewFileSet()
	path := filepath.Join(dir, base)

	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Skipf("parse %s: %v", path, err)
	}

	return fset, f
}

// ffiPtrPrepArgTypes finds a `lib.Prep("cName", ...)` call anywhere in f and
// returns the ffi type names it declares. Index 0 is the return type, so
// index i+1 is C parameter i. Names are returned bare, e.g. "TypeUint64" for
// &ffi.TypeUint64 and "ffiTypeBatch" for &ffiTypeBatch.
//
// Returns nil when the symbol is not registered in this file.
func ffiPtrPrepArgTypes(f *ast.File, cName string) []string {
	var types []string

	ast.Inspect(f, func(n ast.Node) bool {
		if types != nil {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Prep" && sel.Sel.Name != "PrepVar") {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}

		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err != nil || s != cName {
			return true
		}

		found := make([]string, 0, len(call.Args)-1)
		for _, a := range call.Args[1:] {
			// PrepVar has an integer nFixedArgs before the return type.
			if bl, ok := a.(*ast.BasicLit); ok && bl.Kind == token.INT {
				continue
			}
			un, ok := a.(*ast.UnaryExpr)
			if !ok || un.Op != token.AND {
				found = append(found, "?")
				continue
			}
			switch x := un.X.(type) {
			case *ast.SelectorExpr:
				found = append(found, x.Sel.Name)
			case *ast.Ident:
				found = append(found, x.Name)
			default:
				found = append(found, "?")
			}
		}
		types = found

		return false
	})

	return types
}

// ffiPtrFuncDecl returns the top-level function named name.
func ffiPtrFuncDecl(f *ast.File, name string) *ast.FuncDecl {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == name {
			return fd
		}
	}

	return nil
}

// ffiPtrCallArgs returns the argument expressions of the first `X.Call(...)`
// inside fn. Index 0 is the return buffer; index i+1 is C parameter i.
func ffiPtrCallArgs(fn *ast.FuncDecl) []ast.Expr {
	var args []ast.Expr

	ast.Inspect(fn, func(n ast.Node) bool {
		if args != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Call" {
			args = call.Args
			return false
		}

		return true
	})

	return args
}

// ffiPtrAddrOfIdent unwraps `&x` and `unsafe.Pointer(&x)` to "x".
func ffiPtrAddrOfIdent(e ast.Expr) string {
	if call, ok := e.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Pointer" {
			e = call.Args[0]
		}
	}

	un, ok := e.(*ast.UnaryExpr)
	if !ok || un.Op != token.AND {
		return ""
	}

	id, ok := un.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return id.Name
}

// ffiPtrLocalType reports the Go type name of local variable name inside fn,
// recognising the two forms yzma uses: `var name T` and `name := T(...)`.
func ffiPtrLocalType(fn *ast.FuncDecl, name string) string {
	var found string

	ast.Inspect(fn, func(n ast.Node) bool {
		if found != "" {
			return false
		}

		switch s := n.(type) {
		case *ast.ValueSpec:
			for _, id := range s.Names {
				if id.Name != name {
					continue
				}
				if t, ok := s.Type.(*ast.Ident); ok {
					found = t.Name
					return false
				}
			}

		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != name || i >= len(s.Rhs) {
					continue
				}
				if call, ok := s.Rhs[i].(*ast.CallExpr); ok {
					if t, ok := call.Fun.(*ast.Ident); ok {
						found = t.Name
						return false
					}
				}
			}
		}

		return true
	})

	return found
}

// ffiPtrGoWidth maps a Go type name to its size in bytes, resolving yzma's
// named scalar types (e.g. `type LoadMode int32`) through decls found in the
// package. Returns 0 when unknown.
func ffiPtrGoWidth(name string, named map[string]string) int {
	builtin := map[string]int{
		"int8": 1, "uint8": 1, "bool": 1, "byte": 1,
		"int16": 2, "uint16": 2,
		"int32": 4, "uint32": 4, "float32": 4, "rune": 4,
		"int64": 8, "uint64": 8, "float64": 8,
		"int": 8, "uint": 8, "uintptr": 8,
	}

	for range 8 { // follow a short alias chain
		if w, ok := builtin[name]; ok {
			return w
		}
		next, ok := named[name]
		if !ok {
			return 0
		}
		name = next
	}

	return 0
}

// ffiPtrNamedScalars collects `type X <ident>` declarations across the package
// so named types such as LoadMode, Token and Pos can be sized.
func ffiPtrNamedScalars(t *testing.T, dir string) map[string]string {
	t.Helper()

	out := map[string]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("read %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}

		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if id, ok := ts.Type.(*ast.Ident); ok {
					out[ts.Name.Name] = id.Name
				}
			}
		}
	}

	return out
}

// ffiPtrFFIWidth maps an ffi.TypeXxx name to its size in bytes. Returns 0 for
// struct types and anything unrecognised.
func ffiPtrFFIWidth(name string) int {
	return map[string]int{
		"TypeVoid":  0,
		"TypeUint8": 1, "TypeSint8": 1,
		"TypeUint16": 2, "TypeSint16": 2,
		"TypeUint32": 4, "TypeSint32": 4, "TypeFloat": 4,
		"TypeUint64": 8, "TypeSint64": 8, "TypePointer": 8, "TypeDouble": 8,
	}[name]
}

// TestFFIYzmaModelDescSizeArgWidth pins FINDING 1, the one of the two width
// defects that Kronk actually reaches at runtime.
//
// FINDING. yzma's ModelDesc hands libffi the address of a 4-byte int32 for a
// parameter it has correctly declared as 8-byte TypeUint64:
//
//	$GOMODCACHE/github.com/hybridgroup/yzma@v1.21.0/pkg/llama/model.go:249
//	  lib.Prep("llama_model_desc", &ffi.TypeSint32,
//	           &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64)
//
//	.../pkg/llama/model.go:571-580  (func ModelDesc)
//	  buf  := make([]byte, 128)
//	  b    := unsafe.SliceData(buf)
//	  bLen := int32(len(buf))                       // <-- 4 bytes
//	  modelDescFunc.Call(unsafe.Pointer(&result),
//	      unsafe.Pointer(&model), unsafe.Pointer(&b), &bLen)
//
// C SIDE. .extras/llama.cpp/include/llama.h:621 declares
//
//	LLAMA_API int32_t llama_model_desc(const struct llama_model * model,
//	                                  char * buf, size_t buf_size);
//
// and .extras/llama.cpp/src/llama-model.cpp:2711 implements it as
//
//	return snprintf(buf, buf_size, "%s", model->desc().c_str());
//
// RULE. libffi reads exactly ffi.Type.Size bytes from each avalue pointer, so
// it reads 8 bytes from &bLen. TestFFILibffiWidthContract proves this against
// libc. The low half is 128; the high half is whatever Go heap memory follows
// bLen. buf_size therefore becomes 0xXXXXXXXX_00000080 — never smaller than
// 128, so snprintf's bound is effectively lost rather than tightened.
//
// FAILURE SCENARIO. Kronk calls this on every model load, from
// sdk/kronk/model/models.go:120 (`desc := llama.ModelDesc(model)` in
// toModelInfo). snprintf will then write the whole of model->desc() into a
// 128-byte GO HEAP allocation. For any model whose description exceeds 127
// bytes, C writes past the end of that Go object and corrupts whatever the
// allocator placed next — which is exactly how a later GC cycle comes to scan
// a clobbered pointer field and abort with "found pointer to free object" in
// runtime.bgsweep. Today the only thing preventing it is that descriptions
// happen to be short; nothing in the code enforces that. Secondarily,
// ModelDesc's own `string(buf[:int32(result)])` would panic with a slice
// bounds error once snprintf reports more than 128 bytes.
//
// This test FAILS while the defect is present and passes once yzma widens
// bLen to uint64 (or declares the parameter TypeUint32, which would not match
// the header).
func TestFFIYzmaModelDescSizeArgWidth(t *testing.T) {
	dir := ffiPtrYzmaLlamaDir(t)
	_, f := ffiPtrParseFile(t, dir, "model.go")

	const cName = "llama_model_desc"

	prep := ffiPtrPrepArgTypes(f, cName)
	if len(prep) < 4 {
		t.Skipf("%s: cannot locate the Prep registration for %s (found %v); "+
			"yzma's structure changed, re-audit by hand", dir, cName, prep)
	}

	// prep[0] is the return type; C parameter 2 (buf_size) is prep[3].
	sizeParamFFI := prep[3]
	wantWidth := ffiPtrFFIWidth(sizeParamFFI)
	if wantWidth == 0 {
		t.Skipf("unrecognised ffi type %q for %s parameter 2", sizeParamFFI, cName)
	}

	fn := ffiPtrFuncDecl(f, "ModelDesc")
	if fn == nil {
		t.Skip("func ModelDesc not found; yzma's structure changed, re-audit by hand")
	}

	args := ffiPtrCallArgs(fn)
	if len(args) < 4 {
		t.Skipf("ModelDesc: expected at least 4 Call arguments, got %d", len(args))
	}

	// args[0] is the return buffer, so C parameter 2 is args[3].
	ident := ffiPtrAddrOfIdent(args[3])
	if ident == "" {
		t.Skipf("ModelDesc: Call argument 3 is not a plain &variable; re-audit by hand")
	}

	goType := ffiPtrLocalType(fn, ident)
	if goType == "" {
		t.Skipf("ModelDesc: cannot resolve the Go type of %q; re-audit by hand", ident)
	}

	gotWidth := ffiPtrGoWidth(goType, ffiPtrNamedScalars(t, dir))
	if gotWidth == 0 {
		t.Skipf("ModelDesc: unknown Go type %q for %q", goType, ident)
	}

	if gotWidth < wantWidth {
		t.Errorf("yzma ModelDesc passes &%s (Go %s, %d bytes) for %s parameter "+
			"buf_size, which the cif declares as ffi.%s (%d bytes).\n"+
			"libffi reads %d bytes from a %d-byte Go variable, so C receives "+
			"buf_size = <adjacent Go heap>|%d instead of %d.\n"+
			"llama_model_desc then snprintf()s into yzma's 128-byte Go []byte "+
			"with no effective bound. Reached from Kronk on every model load at "+
			"sdk/kronk/model/models.go:120.\n"+
			"FIX (in yzma, not Kronk): declare the length as uint64, e.g.\n"+
			"    bLen := uint64(len(buf))",
			ident, goType, gotWidth, cName, sizeParamFFI, wantWidth,
			wantWidth, gotWidth, 128, 128)
	}
}

// TestFFIYzmaLoadModeFromStrReturnBufferWidth pins FINDING 2.
//
// FINDING. yzma's LoadModeFromStr uses a 4-byte variable as the libffi return
// buffer for an integer-returning C function:
//
//	$GOMODCACHE/github.com/hybridgroup/yzma@v1.21.0/pkg/llama/backend.go:120
//	  lib.Prep("llama_load_mode_from_str", &ffi.TypeSint32, &ffi.TypePointer)
//
//	.../pkg/llama/backend.go:211-216
//	  func LoadModeFromStr(str string) LoadMode {
//	      var result LoadMode          // type LoadMode int32 -> 4 bytes
//	      s, _ := utils.BytePtrFromString(str)
//	      loadModeFromStrFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&s))
//	      return result
//	  }
//
// RULE. jupiterrider/ffi states the prohibition outright, in the doc comment
// on the very method being called here
// ($GOMODCACHE/github.com/jupiterrider/ffi@v0.7.0/fun.go:19-20): "You cannot
// use integer types smaller than 8 bytes here (float32 and structs are not
// affected). Use [Arg] instead and typecast afterwards." libffi stores a full
// ffi_arg — 8 bytes on a 64-bit target — for narrow integer returns.
// TestFFILibffiWidthContract proves this against libc.
//
// FAILURE SCENARIO. The call writes 4 bytes past the end of a Go variable. If
// `result` is heap-allocated (it escapes, because its address is boxed into
// ffi.Fun.Call's `ret any` parameter), those 4 bytes land in whatever the
// allocator placed after it, silently corrupting an unrelated Go object.
//
// Every other integer-returning binding in yzma routes through ffi.Arg
// correctly; this is the lone exception, which is why it reads as an oversight
// rather than a deliberate choice. No Kronk code path reaches
// LoadModeFromStr today (Kronk converts its own config enum via
// Config.LoadMode.ToYZMAType instead), so this is latent, not active — but it
// is one call away from becoming active and the fix is trivial.
//
// This test FAILS while the defect is present and passes once yzma uses
// ffi.Arg for the return buffer.
func TestFFIYzmaLoadModeFromStrReturnBufferWidth(t *testing.T) {
	dir := ffiPtrYzmaLlamaDir(t)
	_, f := ffiPtrParseFile(t, dir, "backend.go")

	fn := ffiPtrFuncDecl(f, "LoadModeFromStr")
	if fn == nil {
		t.Skip("func LoadModeFromStr not found; yzma's structure changed, re-audit by hand")
	}

	args := ffiPtrCallArgs(fn)
	if len(args) == 0 {
		t.Skip("LoadModeFromStr: no .Call found; re-audit by hand")
	}

	ident := ffiPtrAddrOfIdent(args[0])
	if ident == "" {
		t.Skip("LoadModeFromStr: return buffer is not a plain &variable; re-audit by hand")
	}

	goType := ffiPtrLocalType(fn, ident)
	if goType == "" {
		t.Skipf("LoadModeFromStr: cannot resolve the Go type of %q", ident)
	}

	// ffi.Arg is the sanctioned 8-byte carrier; anything else must be at
	// least register-wide.
	if goType == "Arg" {
		return
	}

	width := ffiPtrGoWidth(goType, ffiPtrNamedScalars(t, dir))
	if width == 0 {
		t.Skipf("LoadModeFromStr: unknown Go type %q for return buffer %q", goType, ident)
	}

	const registerWidth = 8
	if width < registerWidth {
		t.Errorf("yzma LoadModeFromStr uses &%s (Go %s, %d bytes) as the libffi "+
			"return buffer for the int-returning llama_load_mode_from_str.\n"+
			"libffi stores a full %d-byte ffi_arg there, writing %d bytes past "+
			"the end of that Go variable.\n"+
			"jupiterrider/ffi forbids this explicitly in the doc comment on "+
			"Fun.Call (ffi@v0.7.0/fun.go:19-20): integer return buffers narrower "+
			"than 8 bytes must use ffi.Arg.\n"+
			"FIX (in yzma, not Kronk): mirror every other integer binding, e.g.\n"+
			"    var result ffi.Arg\n"+
			"    loadModeFromStrFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&s))\n"+
			"    return LoadMode(int32(result))",
			ident, goType, width, registerWidth, registerWidth-width)
	}
}
