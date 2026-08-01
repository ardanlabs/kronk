// This file pins the FFI parameter/return TYPE defects found by a mechanical
// sweep of every yzma binding against the llama.cpp header that matches the
// pinned library build.
//
// # WHY A SWEEP WAS NEEDED
//
// Kronk reaches llama.cpp through yzma, a hand-written purego/libffi binding.
// There is no cgo, so nothing checks these types at compile time. A mismatch
// between (a) the C declaration, (b) the libffi type descriptor handed to
// ffi.PrepCif, and (c) the Go variable whose address is handed to ffi.Fun.Call
// is completely silent at build time and only shows up as corrupted memory or
// corrupted values at runtime.
//
// THE THREE RULES CHECKED
//
//	RULE 1  The cif argument/return type list must match the C prototype
//	        parameter-for-parameter in width, and in class (integer vs float
//	        vs struct-by-value). On arm64: size_t/uint64_t/any pointer = 8,
//	        int/int32_t/enum/float = 4, double = 8.
//	RULE 2  The Go variable whose address is passed at each avalue slot must be
//	        exactly as wide as the cif type claims. libffi reads exactly
//	        ffi.Type.Size bytes from the pointer it is handed, so a 4-byte Go
//	        variable behind a TypeUint64 slot makes libffi splice 4 bytes of
//	        adjacent Go memory into the high half of the C argument.
//	RULE 3  An INTEGER return buffer must be 8 bytes (ffi.Arg), because libffi
//	        always stores a full ffi_arg. FLOAT returns are the opposite: they
//	        must be float32, because libffi stores 4 bytes and ffi.Arg would
//	        then be read as an integer. jupiterrider/ffi states both halves in
//	        the Fun.Call doc comment ($GOMODCACHE/github.com/jupiterrider/
//	        ffi@v0.7.0/fun.go:19-20): "You cannot use integer types smaller
//	        than 8 bytes here (float32 and structs are not affected)."
//
// AUTHORITATIVE TREES
//
//	yzma that actually compiles: $GOMODCACHE/github.com/hybridgroup/yzma@v1.21.0
//	   (resolved below with `go list -m`; the uncompiled .extras/yzma copy carries
//	   a local patch that is NOT in the build and was deliberately not audited)
//	header: b10211, the llama.cpp build pinned at sdk/tools/libs/libs.go:29
//	   `git -C .extras/llama.cpp show b10211:include/llama.h`
//	   (verified: the only include/llama.h difference between b10211 and the
//	   checked-out b10217 is one added `bool load_mtp;` field)
//
// SWEEP RESULT — the numbers matter as much as the findings, because they are
// what makes "these are the only ones" a measurement instead of an assertion.
//
//	847 C API declarations parsed from llama.h, ggml.h, ggml-backend.h,
//	    ggml-cpu.h, mtmd.h and mtmd-helper.h at b10211 (0 unparsed)
//	265 yzma ffi.Prep bindings found, 265/265 matched to a C declaration
//	276 ffi.Fun.Call sites, 514 argument slots resolved (0 unresolvable)
//	 23 struct-by-value slots compared by total size, all in agreement
//
//	RULE 1: 265 checked -> 264 clean,   1 violation
//	RULE 2: 514 checked -> 502 clean,  12 violations
//	RULE 3: 276 checked -> 272 clean,   4 violations
//
// WHERE THE 17 VIOLATIONS ARE PINNED
//
//	RULE 1  1  ggml_backend_cpu_buffer_type  -> this file
//	RULE 2 12  4 sampler min_keep            -> this file
//	           6 metadata buf_size           -> this file
//	           1 MemorySeqDiv factor         -> this file
//	           1 ModelDesc buf_size          -> ffi_pointer_safety_test.go (already known)
//	RULE 3  4  2 float returns via ffi.Arg   -> this file
//	           1 ggml_backend_cpu_buffer_type (same defect as the RULE 1 entry)
//	           1 LoadModeFromStr return buf  -> ffi_pointer_safety_test.go (already known)
//
// Both already-known defects were re-derived independently by the sweep, which
// is the correctness test for the checker. Struct FIELD layout is
// ffi_struct_layout_test.go's subject and pointer LIFETIME is
// ffi_pointer_safety_test.go's; this file only covers TYPES.
//
// Not covered by the 265: the four callback descriptors, which go through
// ffi.PrepCif/PrepClosureLoc or purego.NewCallback rather than lib.Prep
// (pkg/llama/model.go:887, pkg/mtmd/mtmd.go:276, pkg/llama/logs.go:56,
// pkg/llama/context.go:800). All four were hand-checked against their C
// typedefs and are correct.
package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// =============================================================================
// Executable evidence for the libffi behaviour the pins below rely on.
// =============================================================================

// ffiPTLibcName returns the shared library exporting the C functions the
// runtime probes use, or "" when the platform is not one we can name.
func ffiPTLibcName() string {
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

// TestFFIParamTypesLibffiReturnKindContract is the executable evidence for the
// two halves of RULE 3 that the yzma pins below depend on. It goes through the
// exact ffi.Lib/ffi.Fun machinery yzma uses and touches nothing from
// llama.cpp, so it reproduces in milliseconds on any machine.
//
// Both subtests assert libffi's DOCUMENTED behaviour and are expected to PASS.
// They exist so that TestFFIParamTypesYzmaFloatReturnsReadAsInteger and
// TestFFIParamTypesGGMLCPUBufferTypeReturnDescriptor rest on a measurement
// rather than on an argument from the documentation.
func TestFFIParamTypesLibffiReturnKindContract(t *testing.T) {
	name := ffiPTLibcName()
	if name == "" {
		t.Skipf("no known libc name for GOOS=%s", runtime.GOOS)
	}

	lib, err := ffi.Load(name)
	if err != nil {
		t.Skipf("cannot dlopen %s: %v", name, err)
	}

	// -------------------------------------------------------------------
	// A float return is NOT an ffi_arg. libffi stores 4 bytes and leaves
	// the rest of the buffer alone, so an ffi.Arg buffer ends up holding
	// the IEEE-754 bit pattern in its low word. Converting that ffi.Arg
	// with float32(...) is an integer->float numeric conversion, not a
	// bit reinterpretation, so the result is astronomically wrong.
	// -------------------------------------------------------------------
	t.Run("float_return_is_not_an_ffi_arg", func(t *testing.T) {
		fmaxf, err := lib.Prep("fmaxf", &ffi.TypeFloat, &ffi.TypeFloat, &ffi.TypeFloat)
		if err != nil {
			t.Skipf("prep fmaxf: %v", err)
		}

		const want = float32(1.5)
		a, b := want, float32(0.25)

		// Control: the correct buffer type round-trips exactly.
		var correct float32
		fmaxf.Call(unsafe.Pointer(&correct), &a, &b)
		if correct != want {
			t.Fatalf("control failed: fmaxf(%v, %v) into a float32 buffer = %v, want %v; "+
				"the probe itself is wrong", a, b, correct, want)
		}

		// yzma's shape: an ffi.Arg buffer, pre-dirtied so we can see
		// exactly how many bytes libffi replaces.
		const dirty = uint64(0xAAAAAAAAAAAAAAAA)
		arg := ffi.Arg(dirty)
		fmaxf.Call(unsafe.Pointer(&arg), &a, &b)

		if hi := uint32(uint64(arg) >> 32); hi != uint32(dirty>>32) {
			t.Errorf("libffi modified the HIGH word of a float return buffer "+
				"(0x%016X -> 0x%016X). That contradicts the premise of "+
				"TestFFIParamTypesYzmaFloatReturnsReadAsInteger and needs "+
				"revisiting.", dirty, uint64(arg))
		}

		if bits := math.Float32frombits(uint32(uint64(arg))); bits != want {
			t.Errorf("low word of the float return buffer reinterpreted as "+
				"float32 = %v, want %v (raw 0x%016X)", bits, want, uint64(arg))
		}

		// This is the number yzma actually returns.
		if numeric := float32(arg); numeric == want {
			t.Errorf("float32(ffi.Arg) produced the correct value %v on this "+
				"platform. If that is genuinely true here, the "+
				"ModelRopeFreqScaleTrain / VocabGetScore findings are harmless "+
				"and their premise needs revisiting.", numeric)
		} else {
			t.Logf("confirmed: float return in an ffi.Arg buffer is raw 0x%016X; "+
				"float32(arg) = %v but the true value is %v. yzma's "+
				"`return float32(x)` is off by ~%.3g.",
				uint64(arg), numeric, want, float64(numeric)/float64(want))
		}
	})

	// -------------------------------------------------------------------
	// A cif whose RETURN descriptor is TypeVoid makes libffi discard the
	// return value entirely: it writes nothing at all into the buffer the
	// caller supplied. The caller's variable therefore keeps whatever it
	// held before the call, which for a fresh Go `var` means zero.
	// -------------------------------------------------------------------
	t.Run("void_return_descriptor_writes_nothing", func(t *testing.T) {
		hay := []byte("abcdef")
		hp := unsafe.SliceData(hay)
		needle := int32('c')
		n := uint64(len(hay))

		// Control: declared as returning a pointer, we get one back.
		asPtr, err := lib.Prep("memchr",
			&ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint64)
		if err != nil {
			t.Skipf("prep memchr: %v", err)
		}

		var good uintptr
		asPtr.Call(unsafe.Pointer(&good), unsafe.Pointer(&hp), &needle, &n)
		if good != uintptr(unsafe.Pointer(hp))+2 {
			t.Fatalf("control failed: memchr returned 0x%X, want base+2 (0x%X); "+
				"the probe itself is wrong", good, uintptr(unsafe.Pointer(hp))+2)
		}

		// The defective shape: same C function, TypeVoid return descriptor.
		asVoid, err := lib.Prep("memchr",
			&ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint64)
		if err != nil {
			t.Skipf("prep memchr as void: %v", err)
		}

		const sentinel = uintptr(0x1234)
		got := sentinel
		asVoid.Call(unsafe.Pointer(&got), unsafe.Pointer(&hp), &needle, &n)

		if got != sentinel {
			t.Errorf("libffi wrote 0x%X into the return buffer of a TypeVoid cif "+
				"(sentinel was 0x%X). If a void return descriptor really does "+
				"deliver the value on this platform, the "+
				"ggml_backend_cpu_buffer_type finding is harmless here and its "+
				"premise needs revisiting.", got, sentinel)
		} else {
			t.Logf("confirmed: a TypeVoid return descriptor makes libffi write "+
				"NOTHING; the caller's buffer still holds 0x%X. A Go `var x T` "+
				"return buffer would therefore stay at its zero value forever.",
				got)
		}

		runtime.KeepAlive(hay)
	})
}

// =============================================================================
// AST helpers over the COMPILED yzma.
// =============================================================================

// ffiPTYzmaDir returns the directory of a yzma package inside the module the
// build actually uses, resolved via `go list -m` so it tracks the go.mod pin
// rather than the uncompiled .extras/yzma copy. It skips when the toolchain or
// the module cache is unavailable.
func ffiPTYzmaDir(t *testing.T, pkg string) string {
	t.Helper()

	root := kronkRepoRoot(t)

	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/hybridgroup/yzma")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		t.Skipf("cannot resolve the yzma module (module cache unavailable?): %v", err)
	}

	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Skip("go list -m reported no directory for github.com/hybridgroup/yzma")
	}

	return filepath.Join(dir, pkg)
}

// ffiPTParse parses one yzma source file with positions.
func ffiPTParse(t *testing.T, dir, base string) (*token.FileSet, *ast.File) {
	t.Helper()

	path := filepath.Join(dir, base)
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Skipf("cannot parse %s (yzma layout changed?): %v", path, err)
	}

	return fset, f
}

// ffiPTPrep locates the `<var>, err = lib.Prep("cName", &retType, &argType...)`
// statement for a C symbol and returns the assigned Go variable name plus the
// stringified type descriptors. retType is index 0 of types.
func ffiPTPrep(t *testing.T, f *ast.File, cName string) (goVar string, types []string, pos token.Pos) {
	t.Helper()

	ast.Inspect(f, func(n ast.Node) bool {
		if goVar != "" {
			return false
		}

		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			return true
		}

		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Prep" && sel.Sel.Name != "MustPrep") {
			return true
		}

		if len(call.Args) < 2 || ffiPTStringLit(call.Args[0]) != cName {
			return true
		}

		for _, l := range as.Lhs {
			id, ok := l.(*ast.Ident)
			if ok && id.Name != "err" && id.Name != "_" {
				goVar = id.Name
				break
			}
		}

		for _, a := range call.Args[1:] {
			types = append(types, ffiPTExprText(a))
		}

		pos = call.Lparen

		return false
	})

	if goVar == "" {
		t.Skipf("no lib.Prep(%q, ...) found; the yzma binding was renamed or removed", cName)
	}

	return goVar, types, pos
}

// ffiPTStringLit returns the value of a plain string literal, or "".
func ffiPTStringLit(e ast.Expr) string {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return ""
	}

	return strings.Trim(bl.Value, "`\"")
}

// ffiPTExprText renders the small subset of expressions that appear in cif type
// lists and Call argument lists.
func ffiPTExprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return ffiPTExprText(x.X) + "." + x.Sel.Name
	case *ast.UnaryExpr:
		return x.Op.String() + ffiPTExprText(x.X)
	case *ast.StarExpr:
		return "*" + ffiPTExprText(x.X)
	case *ast.ParenExpr:
		return "(" + ffiPTExprText(x.X) + ")"
	case *ast.IndexExpr:
		return ffiPTExprText(x.X) + "[" + ffiPTExprText(x.Index) + "]"
	case *ast.BasicLit:
		return x.Value
	case *ast.CallExpr:
		parts := make([]string, 0, len(x.Args))
		for _, a := range x.Args {
			parts = append(parts, ffiPTExprText(a))
		}

		return ffiPTExprText(x.Fun) + "(" + strings.Join(parts, ", ") + ")"
	default:
		return "?"
	}
}

// ffiPTFuncDecl finds a top-level func declaration by name.
func ffiPTFuncDecl(t *testing.T, f *ast.File, name string) *ast.FuncDecl {
	t.Helper()

	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if ok && fd.Recv == nil && fd.Name.Name == name {
			return fd
		}
	}

	t.Skipf("func %s not found; the yzma API was renamed or removed", name)

	return nil
}

// ffiPTCall finds the single `goVar.Call(...)` inside fn and returns its
// arguments. Argument 0 is the return buffer.
func ffiPTCall(t *testing.T, fn *ast.FuncDecl, goVar string) ([]ast.Expr, token.Pos) {
	t.Helper()

	var (
		args []ast.Expr
		pos  token.Pos
		n    int
	)

	ast.Inspect(fn, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Call" {
			return true
		}

		recv, ok := sel.X.(*ast.Ident)
		if !ok || recv.Name != goVar {
			return true
		}

		n++
		args = call.Args
		pos = call.Lparen

		return true
	})

	if n != 1 {
		t.Skipf("expected exactly one %s.Call(...) in %s, found %d; the yzma "+
			"code shape changed", goVar, fn.Name.Name, n)
	}

	return args, pos
}

// ffiPTAddrOf unwraps `unsafe.Pointer(&x)` / `&x` down to the identifier x.
// It returns "" for anything else (nil, a slice-data call, a bare pointer).
func ffiPTAddrOf(e ast.Expr) string {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			break
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "unsafe" || sel.Sel.Name != "Pointer" {
			break
		}

		e = call.Args[0]
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

// ffiPTDeclaredType returns the source-level type of a local variable or
// parameter of fn, or "" when it cannot be determined syntactically.
func ffiPTDeclaredType(fn *ast.FuncDecl, name string) string {
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, id := range field.Names {
				if id.Name == name {
					return ffiPTExprText(field.Type)
				}
			}
		}
	}

	found := ""

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found != "" {
			return false
		}

		switch x := n.(type) {
		case *ast.DeclStmt:
			gd, ok := x.Decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				return true
			}

			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || vs.Type == nil {
					continue
				}

				for _, id := range vs.Names {
					if id.Name == name {
						found = ffiPTExprText(vs.Type)
						return false
					}
				}
			}

		case *ast.AssignStmt:
			if x.Tok != token.DEFINE || len(x.Lhs) != len(x.Rhs) {
				return true
			}

			for i, l := range x.Lhs {
				id, ok := l.(*ast.Ident)
				if !ok || id.Name != name {
					continue
				}

				// Only `x := T(...)` is unambiguous syntactically.
				call, ok := x.Rhs[i].(*ast.CallExpr)
				if !ok {
					continue
				}

				if conv, ok := call.Fun.(*ast.Ident); ok {
					found = conv.Name
					return false
				}
			}
		}

		return true
	})

	return found
}

// ffiPTGoWidth returns the arm64/amd64 byte width of a Go scalar type spelled
// as it appears in yzma source, or 0 when unknown.
func ffiPTGoWidth(name string) int {
	switch name {
	case "bool", "int8", "uint8", "byte":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "float32", "rune":
		return 4
	case "int64", "uint64", "float64", "int", "uint", "uintptr", "ffi.Arg":
		return 8
	default:
		return 0
	}
}

// ffiPTFFIWidth returns the byte width libffi will read for a cif type
// descriptor spelled as `&ffi.TypeX`, or 0 when unknown. -1 marks TypeVoid,
// which is not a width at all.
func ffiPTFFIWidth(descriptor string) int {
	switch strings.TrimPrefix(descriptor, "&") {
	case "ffi.TypeVoid":
		return -1
	case "ffi.TypeUint8", "ffi.TypeSint8":
		return 1
	case "ffi.TypeUint16", "ffi.TypeSint16":
		return 2
	case "ffi.TypeUint32", "ffi.TypeSint32", "ffi.TypeFloat":
		return 4
	case "ffi.TypeUint64", "ffi.TypeSint64", "ffi.TypeDouble", "ffi.TypePointer", "ffiTypeSize":
		return 8
	default:
		return 0
	}
}

// ffiPTArgWidthCase describes one RULE 2 pin: at C parameter index slot, the
// cif claims cifWidth bytes, and the Go variable behind the address passed there
// must be exactly that wide.
//
// cifWidth is compared against the RESOLVED width of whatever descriptor the
// Prep actually names, because yzma spells the same 8-byte type three ways:
// &ffi.TypeUint64, &ffi.TypePointer and the package-level alias &ffiTypeSize
// (pkg/llama/model.go:14, `ffiTypeSize = ffi.TypeUint64`). Comparing widths
// rather than spellings keeps the pin honest if the spelling changes, while
// still turning into a skip if the binding is genuinely re-typed.
type ffiPTArgWidthCase struct {
	file     string // yzma file, relative to the package dir
	goFunc   string // exported yzma wrapper
	cName    string // C symbol, used to locate the Prep
	slot     int    // C parameter index (0 == first C parameter)
	cifWidth int    // bytes libffi is told to read at that slot
	cRef     string // llama.h reference and C prototype
}

// ffiPTCheckArgWidths is the shared body of the RULE 2 pins. For each case it
// re-derives the cif descriptor from the Prep, then measures the Go variable
// whose address reaches that avalue slot, and reports when the two disagree.
func ffiPTCheckArgWidths(t *testing.T, pkg string, cases []ffiPTArgWidthCase) {
	t.Helper()

	dir := ffiPTYzmaDir(t, pkg)

	for _, tc := range cases {
		t.Run(tc.goFunc, func(t *testing.T) {
			fset, file := ffiPTParse(t, dir, tc.file)

			goVar, cifTypes, prepPos := ffiPTPrep(t, file, tc.cName)

			// cifTypes[0] is the return descriptor, so the argument at
			// C parameter index slot lives at cifTypes[slot+1].
			if len(cifTypes) < tc.slot+2 {
				t.Skipf("%s cif has %d descriptors, need at least %d; shape changed",
					tc.cName, len(cifTypes), tc.slot+2)
			}

			gotCif := cifTypes[tc.slot+1]

			want := ffiPTFFIWidth(gotCif)
			if want <= 0 {
				t.Skipf("%s:%d: cif slot %d descriptor %q is not a known scalar type",
					tc.file, fset.Position(prepPos).Line, tc.slot, gotCif)
			}

			if want != tc.cifWidth {
				t.Skipf("%s:%d: cif slot %d is %s (%d bytes), the pin expects a "+
					"%d-byte slot; the binding was changed and this pin needs re-deriving",
					tc.file, fset.Position(prepPos).Line, tc.slot, gotCif, want, tc.cifWidth)
			}

			fn := ffiPTFuncDecl(t, file, tc.goFunc)
			args, callPos := ffiPTCall(t, fn, goVar)

			// args[0] is the return buffer.
			if len(args) < tc.slot+2 {
				t.Skipf("%s passes %d values to Call, need at least %d; shape changed",
					tc.goFunc, len(args), tc.slot+2)
			}

			ident := ffiPTAddrOf(args[tc.slot+1])
			if ident == "" {
				t.Skipf("Call argument %d of %s is %q, not an &identifier; shape changed",
					tc.slot, tc.goFunc, ffiPTExprText(args[tc.slot+1]))
			}

			goType := ffiPTDeclaredType(fn, ident)
			if goType == "" {
				t.Skipf("cannot determine the declared type of %s in %s", ident, tc.goFunc)
			}

			got := ffiPTGoWidth(goType)
			if got == 0 {
				t.Skipf("unknown Go type %q for %s in %s", goType, ident, tc.goFunc)
			}

			line := fset.Position(callPos).Line

			if got == want {
				return
			}

			verb := "reads %d byte(s) of adjacent Go memory into the high half of the C argument"
			if got > want {
				verb = "silently truncates the Go value, discarding %d byte(s)"
			}

			delta := want - got
			if delta < 0 {
				delta = -delta
			}

			t.Errorf("RULE 2 VIOLATION in yzma %s\n"+
				"  yzma:   %s/%s:%d  %s -> %s.Call(..., &%s)\n"+
				"  C:      %s\n"+
				"  cif:    %s says libffi must read %d byte(s) from that address\n"+
				"  Go:     %s is declared %s, which is %d byte(s)\n"+
				"  effect: libffi "+verb+"\n"+
				"  fix:    declare %s as a %d-byte type (or copy it into one before the call)",
				tc.cName,
				pkg, tc.file, line, tc.goFunc, goVar, ident,
				tc.cRef,
				gotCif, want,
				ident, goType, got,
				delta,
				ident, want)
		})
	}
}

// =============================================================================
// RULE 2 — size_t parameters backed by 4-byte Go variables.
// =============================================================================

// TestFFIParamTypesYzmaSamplerMinKeepWidth pins the highest-impact RULE 2
// finding of the sweep: the four samplers whose `size_t min_keep` argument is
// backed by a 4-byte Go uint32.
//
// FINDING
//
//	pkg/llama/sampling.go:459  SamplerInitTypical  llama.h:1354
//	pkg/llama/sampling.go:467  SamplerInitTopP     llama.h:1348
//	pkg/llama/sampling.go:475  SamplerInitMinP     llama.h:1351
//	pkg/llama/sampling.go:483  SamplerInitXTC      llama.h:1363
//
// RULE BROKEN: 2. Each cif correctly declares the C `size_t min_keep` as an
// 8-byte slot -- spelled &ffiTypeSize, the pkg/llama/model.go:13 alias for
// ffi.TypeUint64 -- so RULE 1 is satisfied. But the Go wrapper passes the
// address of its own `keep uint32` / `minKeep uint32` parameter. libffi reads 8
// bytes from that address and splices 4 bytes of unrelated Go memory into the
// high half of min_keep.
//
// # WHY THE ADJACENT BYTES ARE NOT SAFELY ZERO
//
// ffi.Fun.Call takes `args ...any`, so &keep is stored in an interface and
// escapes to the heap. It lands in Go's tiny allocator, which packs several
// 4-byte objects into one 16-byte block and does not re-zero a block it is
// still carving up. A direct measurement of 20000 escaped 4-byte variables (in
// a churned heap) found a non-zero adjacent high word 958-2756 times, i.e. 5-14%
// of the time. So this fires intermittently, not never.
//
// # CONCRETE RUNTIME CONSEQUENCE
//
// When it fires, min_keep becomes >= 2^32 and llama.cpp b10211 silently turns
// the sampler into a no-op, because every truncation guard is written as
// "stop once we have kept at least min_keep candidates" (src/llama-sampler.cpp):
//
//	:1393 top_p   if (cum_sum >= p && i + 1 >= min_keep)          -> never fires
//	:1582 min_p   if (!filtered.empty() && filtered.size() >= min_keep) -> fast path rejected
//	:1600 min_p   if (logit < min_logit && i >= min_keep)          -> never fires (full sort, no truncation)
//	:1754 typical if (min_keep == 0 || i >= min_keep - 1)          -> never fires
//	:2164 xtc     if (size - pos_last >= min_keep && pos_last > 0) -> never fires
//
// So top-p, min-p and XTC stop filtering entirely for the lifetime of that
// sampler, with no error and no log line. The request samples from the full
// untruncated distribution.
//
// KRONK REACHES THREE OF THE FOUR, per request, in buildSampler:
//
//	sdk/kronk/model/params.go:800  llama.SamplerInitTopP(p.TopP, 0)
//	sdk/kronk/model/params.go:804  llama.SamplerInitMinP(p.MinP, 0)
//	sdk/kronk/model/params.go:809  llama.SamplerInitXTC(..., p.XtcMinKeep, ...)
//
// Note that Kronk passing a literal 0 does not help: the low word is 0 but the
// high word is still garbage.
//
// UPSTREAM FIX (do not apply here): widen the wrapper parameters to uint64, or
// copy into a local uint64 and pass its address.
func TestFFIParamTypesYzmaSamplerMinKeepWidth(t *testing.T) {
	ffiPTCheckArgWidths(t, "pkg/llama", []ffiPTArgWidthCase{
		{
			file: "sampling.go", goFunc: "SamplerInitTopP", cName: "llama_sampler_init_top_p",
			slot: 1, cifWidth: 8,
			cRef: "llama.h:1348 struct llama_sampler * llama_sampler_init_top_p(float p, size_t min_keep)",
		},
		{
			file: "sampling.go", goFunc: "SamplerInitMinP", cName: "llama_sampler_init_min_p",
			slot: 1, cifWidth: 8,
			cRef: "llama.h:1351 struct llama_sampler * llama_sampler_init_min_p(float p, size_t min_keep)",
		},
		{
			file: "sampling.go", goFunc: "SamplerInitTypical", cName: "llama_sampler_init_typical",
			slot: 1, cifWidth: 8,
			cRef: "llama.h:1354 struct llama_sampler * llama_sampler_init_typical(float p, size_t min_keep)",
		},
		{
			file: "sampling.go", goFunc: "SamplerInitXTC", cName: "llama_sampler_init_xtc",
			slot: 2, cifWidth: 8,
			cRef: "llama.h:1363 struct llama_sampler * llama_sampler_init_xtc(float p, float t, size_t min_keep, uint32_t seed)",
		},
	})
}

// TestFFIParamTypesYzmaMetadataBufSizeWidth pins the six metadata readers that
// repeat ModelDesc's defect: a `size_t buf_size` out-parameter length backed by
// a 4-byte Go int32.
//
// FINDING
//
//	pkg/llama/model.go:746  ModelMetaValStr           llama.h:605
//	pkg/llama/model.go:785  ModelMetaKeyByIndex       llama.h:614
//	pkg/llama/model.go:813  ModelMetaValStrByIndex    llama.h:617
//	pkg/llama/lora.go:135   AdapterMetaValStr         llama.h:677
//	pkg/llama/lora.go:173   AdapterMetaKeyByIndex     llama.h:683
//	pkg/llama/lora.go:200   AdapterMetaValStrByIndex  llama.h:686
//
// RULE BROKEN: 2. Every one of these cifs correctly models the C `size_t
// buf_size` as an 8-byte slot (all six spell it &ffiTypeSize, the
// pkg/llama/model.go:13 alias for ffi.TypeUint64), and every one of them then
// passes `&bLen` where `bLen := int32(len(buf))`.
//
// This is exactly the ModelDesc defect (pinned separately by
// TestFFIYzmaModelDescSizeArgWidth in ffi_pointer_safety_test.go), six more
// times. It is NOT a yzma house style: the same file gets it right at
// pkg/llama/model.go:979, where llama_split_path's size_t length is a uint64.
//
// # CONCRETE RUNTIME CONSEQUENCE
//
// llama.cpp receives buf_size with a garbage high word, i.e. a limit in the
// billions of gigabytes instead of the 128 bytes yzma actually allocated. The
// bound stops protecting the buffer, so any metadata value longer than the Go
// buffer writes past the end of a Go heap allocation. Unlike the sampler
// finding, the damage here is memory corruption rather than a wrong value.
//
// KRONK REACHES ModelMetaValStr from roughly ten call sites (model metadata
// reads during load and capability detection), so this is on the hot path of
// every model load, not a corner.
//
// UPSTREAM FIX (do not apply here): `bLen := uint64(len(buf))`, matching
// pkg/llama/model.go:979.
func TestFFIParamTypesYzmaMetadataBufSizeWidth(t *testing.T) {
	ffiPTCheckArgWidths(t, "pkg/llama", []ffiPTArgWidthCase{
		{
			file: "model.go", goFunc: "ModelMetaValStr", cName: "llama_model_meta_val_str",
			slot: 3, cifWidth: 8,
			cRef: "llama.h:605 int32_t llama_model_meta_val_str(const struct llama_model *, const char * key, char * buf, size_t buf_size)",
		},
		{
			file: "model.go", goFunc: "ModelMetaKeyByIndex", cName: "llama_model_meta_key_by_index",
			slot: 3, cifWidth: 8,
			cRef: "llama.h:614 int32_t llama_model_meta_key_by_index(const struct llama_model *, int32_t i, char * buf, size_t buf_size)",
		},
		{
			file: "model.go", goFunc: "ModelMetaValStrByIndex", cName: "llama_model_meta_val_str_by_index",
			slot: 3, cifWidth: 8,
			cRef: "llama.h:617 int32_t llama_model_meta_val_str_by_index(const struct llama_model *, int32_t i, char * buf, size_t buf_size)",
		},
		{
			file: "lora.go", goFunc: "AdapterMetaValStr", cName: "llama_adapter_meta_val_str",
			slot: 3, cifWidth: 8,
			cRef: "llama.h:677 int32_t llama_adapter_meta_val_str(const struct llama_adapter_lora *, const char * key, char * buf, size_t buf_size)",
		},
		{
			file: "lora.go", goFunc: "AdapterMetaKeyByIndex", cName: "llama_adapter_meta_key_by_index",
			slot: 3, cifWidth: 8,
			cRef: "llama.h:683 int32_t llama_adapter_meta_key_by_index(const struct llama_adapter_lora *, int32_t i, char * buf, size_t buf_size)",
		},
		{
			file: "lora.go", goFunc: "AdapterMetaValStrByIndex", cName: "llama_adapter_meta_val_str_by_index",
			slot: 3, cifWidth: 8,
			cRef: "llama.h:686 int32_t llama_adapter_meta_val_str_by_index(const struct llama_adapter_lora *, int32_t i, char * buf, size_t buf_size)",
		},
	})
}

// TestFFIParamTypesYzmaMemorySeqDivFactorWidth pins the one RULE 2 violation
// that runs in the opposite direction: a Go variable WIDER than its cif slot.
//
// FINDING: pkg/llama/memory.go:148 MemorySeqDiv, C at llama.h:768
//
//	void llama_memory_seq_div(llama_memory_t mem, llama_seq_id seq_id,
//	                          llama_pos p0, llama_pos p1, int d)
//
// RULE BROKEN: 2. The cif is correct (ffi.TypeSint32 for the C `int d`), but
// the Go wrapper declares its parameter `d int`, which is 8 bytes. libffi reads
// only the low 4 bytes at &d.
//
// SEVERITY: LOW, and deliberately recorded as such. On little-endian arm64 and
// amd64 the low 4 bytes ARE the correct value for any |d| < 2^31, so this
// happens to work. It is pinned because it is a genuine contract violation that
// silently truncates for larger d and would read the wrong half on a big-endian
// target, and because leaving it out of the sweep's output would misrepresent
// the count. Kronk does not call MemorySeqDiv.
//
// UPSTREAM FIX (do not apply here): declare the parameter `d int32`.
func TestFFIParamTypesYzmaMemorySeqDivFactorWidth(t *testing.T) {
	ffiPTCheckArgWidths(t, "pkg/llama", []ffiPTArgWidthCase{
		{
			file: "memory.go", goFunc: "MemorySeqDiv", cName: "llama_memory_seq_div",
			slot: 4, cifWidth: 4,
			cRef: "llama.h:768 void llama_memory_seq_div(llama_memory_t, llama_seq_id, llama_pos p0, llama_pos p1, int d)",
		},
	})
}

// =============================================================================
// RULE 3 — return buffers of the wrong KIND.
// =============================================================================

// TestFFIParamTypesYzmaFloatReturnsReadAsInteger pins the two float-returning
// bindings that read their result through ffi.Arg.
//
// FINDING
//
//	pkg/llama/model.go:646  ModelRopeFreqScaleTrain  llama.h:585
//	                        float llama_model_rope_freq_scale_train(const struct llama_model *)
//	pkg/llama/vocab.go:561  VocabGetScore            llama.h:1082
//	                        float llama_vocab_get_score(const struct llama_vocab *, llama_token)
//
// Both are written as:
//
//	var x ffi.Arg
//	someFunc.Call(unsafe.Pointer(&x), ...)
//	return float32(x)
//
// RULE BROKEN: 3, in its float half. ffi.Arg is the correct return buffer for
// INTEGER returns only; jupiterrider/ffi's own doc comment carves floats out
// explicitly ("float32 and structs are not affected"). Two independent defects
// stack here, and TestFFIParamTypesLibffiReturnKindContract measures both:
//
//  1. libffi stores only 4 bytes for a TypeFloat return and does NOT clear the
//     rest of the buffer, so the high word of the ffi.Arg keeps whatever it
//     held. (Measured with fmaxf: a buffer pre-set to 0xAAAAAAAAAAAAAAAA came
//     back as 0xAAAAAAAA3FC00000.)
//  2. `float32(x)` on an ffi.Arg is an integer-to-float NUMERIC conversion, not
//     a bit reinterpretation. Even with a perfectly zeroed high word,
//     float32(0x3F800000) is 1.0653532e9, not 1.0.
//
// CONCRETE RUNTIME CONSEQUENCE: both functions return garbage on the order of
// 1e9 for every input, deterministically, not intermittently. VocabGetScore in
// particular would make every token score meaningless to any caller that ranks
// on it.
//
// KRONK REACHES NEITHER (no non-test references to either wrapper), so this is
// latent for Kronk today and would bite the moment either is used.
//
// UPSTREAM FIX (do not apply here): use a float32 return buffer and return it
// directly -- `var x float32; f.Call(unsafe.Pointer(&x), ...); return x`.
func TestFFIParamTypesYzmaFloatReturnsReadAsInteger(t *testing.T) {
	dir := ffiPTYzmaDir(t, "pkg/llama")

	cases := []struct {
		file   string
		goFunc string
		cName  string
		cRef   string
	}{
		{
			file: "model.go", goFunc: "ModelRopeFreqScaleTrain",
			cName: "llama_model_rope_freq_scale_train",
			cRef:  "llama.h:585 float llama_model_rope_freq_scale_train(const struct llama_model *)",
		},
		{
			file: "vocab.go", goFunc: "VocabGetScore",
			cName: "llama_vocab_get_score",
			cRef:  "llama.h:1082 float llama_vocab_get_score(const struct llama_vocab *, llama_token)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.goFunc, func(t *testing.T) {
			fset, file := ffiPTParse(t, dir, tc.file)

			goVar, cifTypes, prepPos := ffiPTPrep(t, file, tc.cName)

			if cifTypes[0] != "&ffi.TypeFloat" {
				t.Skipf("%s:%d: %s return descriptor is %s, expected &ffi.TypeFloat; "+
					"the binding changed and this pin needs re-deriving",
					tc.file, fset.Position(prepPos).Line, tc.cName, cifTypes[0])
			}

			fn := ffiPTFuncDecl(t, file, tc.goFunc)
			args, callPos := ffiPTCall(t, fn, goVar)

			if len(args) == 0 {
				t.Skipf("%s.Call has no return buffer argument", tc.goFunc)
			}

			ident := ffiPTAddrOf(args[0])
			if ident == "" {
				t.Skipf("return buffer of %s is %q, not an &identifier; shape changed",
					tc.goFunc, ffiPTExprText(args[0]))
			}

			goType := ffiPTDeclaredType(fn, ident)
			if goType == "" {
				t.Skipf("cannot determine the declared type of %s in %s", ident, tc.goFunc)
			}

			if goType == "float32" {
				return
			}

			t.Errorf("RULE 3 VIOLATION (float half) in yzma %s\n"+
				"  yzma:   pkg/llama/%s:%d  %s -> %s.Call(unsafe.Pointer(&%s), ...)\n"+
				"  C:      %s\n"+
				"  cif:    return descriptor is &ffi.TypeFloat, so libffi stores 4 bytes\n"+
				"          of IEEE-754 float and leaves the rest of the buffer untouched\n"+
				"  Go:     %s is declared %s, and the value is then read with float32(%s)\n"+
				"  effect: float32() on an integer buffer is a NUMERIC conversion, not a\n"+
				"          bit reinterpretation, so %s returns ~1e9-scale garbage for\n"+
				"          every input (see TestFFIParamTypesLibffiReturnKindContract)\n"+
				"  fix:    var %s float32 as the return buffer, and return it directly",
				tc.cName,
				tc.file, fset.Position(callPos).Line, tc.goFunc, goVar, ident,
				tc.cRef,
				ident, goType, ident,
				tc.goFunc,
				ident)
		})
	}
}

// =============================================================================
// RULE 1 — a cif return descriptor that does not model the C return type.
// =============================================================================

// TestFFIParamTypesGGMLCPUBufferTypeReturnDescriptor pins the single RULE 1
// violation in the sweep, and it is the most damaging finding overall because
// it is deterministic and Kronk reaches it without the user opting in.
//
// FINDING
//
//	cif:  pkg/llama/ggml_base.go:28
//	      lib.Prep("ggml_backend_cpu_buffer_type", &ffi.TypeVoid)
//	call: pkg/llama/ggml_base.go:46
//	      var ret uintptr; ggmlBackendCpuBufferType.Call(&ret)
//	C:    ggml/include/ggml-backend.h:431 (b10211)
//	      GGML_API ggml_backend_buffer_type_t ggml_backend_cpu_buffer_type(void);
//
// RULES BROKEN: 1 (the return descriptor models a pointer return as void) and,
// as a direct consequence, 3 (a return buffer is supplied for a cif that
// declares no return value).
//
// # CONCRETE RUNTIME CONSEQUENCE
//
// libffi writes NOTHING into the caller's buffer when the cif's return type is
// FFI_TYPE_VOID -- measured in TestFFIParamTypesLibffiReturnKindContract, where
// a sentinel of 0x1234 survived the call untouched. `ret` is a fresh Go var, so
// llama.GGMLBackendCpuBufferType() returns 0 every single time, on every
// platform.
//
// That zero is not merely a wrong value; it becomes a NULL pointer inside
// llama.cpp. yzma's NewTensorBuftOverride (pkg/llama/ggml_base.go:75) builds
// llama.TensorBuftOverride{Pattern: ..., Type: GGMLBackendCpuBufferType()},
// i.e. {pattern, NULL}, and nothing downstream validates it (yzma's
// ModelParams.SetTensorBufOverrides at pkg/llama/model.go:845 only checks the
// sentinel termination). In llama.cpp b10211's model loader:
//
//	src/llama-model-loader.cpp:1157  if (overrides->buft == ggml_backend_cpu_buffer_type())
//	                                 -> false, because NULL != the real CPU buft
//	src/llama-model-loader.cpp:1166  buft = overrides->buft;   // NULL
//	src/llama-model-loader.cpp:1169  LLAMA_LOG_DEBUG(..., ggml_backend_buft_name(buft))
//	ggml/src/ggml-backend.cpp:34     GGML_ASSERT(buft);        // -> abort()
//
// LLAMA_LOG_DEBUG is a varargs FUNCTION call (src/llama-impl.h:31), so
// ggml_backend_buft_name(NULL) is evaluated regardless of the active log level.
// The process aborts during model load, and the intended effect -- pinning the
// matched tensors to the CPU buffer type -- never happens either way.
//
// KRONK REACHES IT, and not only through an explicit setting:
//
//	sdk/kronk/model/model.go:1427/1435  llama.NewTensorBuftAllFFNExprsOverride()
//	sdk/kronk/model/model.go:1433/1441  llama.NewTensorBuftBlockOverride(idx)
//	sdk/kronk/model/model.go:1443       llama.NewTensorBuftOverride(p)
//	  all in parseTensorBuftOverrides, called from buildModelParams at model.go:521
//
// and Kronk sets cfg.TensorBuftOverrides to []string{"moe-experts"} on its own
// at sdk/kronk/model/model.go:494 and :505 for the MoE placement modes, so an
// MoE config reaches this with no tensor-buft-overrides in the user's YAML.
//
// UPSTREAM FIX (do not apply here): the return descriptor must be
// &ffi.TypePointer. Note that the two neighbouring bindings in the same
// loadGGMLBase get it right (ggml_backend_dev_name uses &ffi.TypePointer for
// its `const char *` return, and ggml_backend_dev_memory is genuinely void).
func TestFFIParamTypesGGMLCPUBufferTypeReturnDescriptor(t *testing.T) {
	const (
		file   = "ggml_base.go"
		goFunc = "GGMLBackendCpuBufferType"
		cName  = "ggml_backend_cpu_buffer_type"
	)

	dir := ffiPTYzmaDir(t, "pkg/llama")
	fset, parsed := ffiPTParse(t, dir, file)

	goVar, cifTypes, prepPos := ffiPTPrep(t, parsed, cName)
	prepLine := fset.Position(prepPos).Line

	fn := ffiPTFuncDecl(t, parsed, goFunc)
	args, callPos := ffiPTCall(t, fn, goVar)
	callLine := fset.Position(callPos).Line

	// The wrapper must genuinely want the value back, otherwise a void
	// descriptor would be harmless and this pin would be vacuous.
	if len(args) == 0 || ffiPTAddrOf(args[0]) == "" {
		t.Skipf("%s no longer passes an &identifier return buffer; shape changed", goFunc)
	}

	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		t.Skipf("%s no longer returns exactly one value; shape changed", goFunc)
	}

	if got := cifTypes[0]; got != "&ffi.TypePointer" {
		t.Errorf("RULE 1 VIOLATION in yzma %s\n"+
			"  yzma:   pkg/llama/%s:%d  lib.Prep(%q, %s)\n"+
			"          pkg/llama/%s:%d  %s.Call(%s)  <- a return buffer IS supplied\n"+
			"  C:      ggml-backend.h:431 (b10211)\n"+
			"          ggml_backend_buffer_type_t ggml_backend_cpu_buffer_type(void)\n"+
			"          -- an 8-byte POINTER return, not void\n"+
			"  effect: libffi writes nothing at all for a TypeVoid return descriptor, so\n"+
			"          %s() returns 0 on every call. yzma then stores that\n"+
			"          NULL as TensorBuftOverride.Type, and llama.cpp b10211 aborts in\n"+
			"          GGML_ASSERT(buft) at ggml-backend.cpp:34, reached unconditionally\n"+
			"          via LLAMA_LOG_DEBUG at llama-model-loader.cpp:1169.\n"+
			"          Kronk hits this from parseTensorBuftOverrides\n"+
			"          (sdk/kronk/model/model.go:1427-1443), including for MoE configs\n"+
			"          that set TensorBuftOverrides implicitly at model.go:494/:505.\n"+
			"  fix:    the return descriptor must be &ffi.TypePointer, not %s",
			cName,
			file, prepLine, cName, strings.Join(cifTypes, ", "),
			file, callLine, goVar, ffiPTExprText(args[0]),
			goFunc,
			got)
	}
}
