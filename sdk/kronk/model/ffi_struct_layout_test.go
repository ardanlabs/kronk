package model

import (
	"go/ast"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// FFI struct-layout audit: yzma's hand-written Go mirrors vs llama.cpp's
// include/llama.h at the vendored reference build.
//
// yzma is a purego/libffi binding, NOT cgo. Nothing checks its Go structs
// against the C ones: `llama_model_params` and `llama_context_params` are both
// passed BY VALUE through libffi (see .extras/yzma/pkg/llama/model.go:16-20
// `ffiTypeModelParams` and .extras/yzma/pkg/llama/context.go:12-31
// `ffiTypeContextParams`, which are the hand-maintained libffi element lists
// used for llama_model_load_from_file and llama_init_from_model). A field
// inserted upstream shifts every following field and silently scrambles the
// call, with no error message.
//
// The C layout used by these tests was verified empirically, not only read out
// of the header: a throwaway C probe compiled against
// .extras/llama.cpp/include/llama.h and linked against the shipped
// libllama.0.0.10211.dylib printed
//
//	sizeof(llama_model_params)   = 72   // vocab_only@64 check_tensors@65
//	                                   // use_extra_bufts@66 no_host@67
//	                                   // no_alloc@68 load_mtp@69
//	sizeof(llama_context_params) = 160  // n_ctx@0 ... ctx_other@152
//
// and llama_model_default_params() returned load_mtp = false, matching
// .extras/llama.cpp/src/llama-model.cpp:2393.
//
// VERIFIED CORRECT (checked field by field, so the negative result is on
// record and a future upstream insert is caught):
//
//   - llama_context_params (llama.h:350-409) vs llama.ContextParams
//     (.extras/yzma/pkg/llama/llama.go:335-378): all 36 fields, same order,
//     same widths, same padding (defrag_thold@80 + 4 pad bytes before
//     cb_eval@88; six bools at 128-133 + 6 pad bytes before samplers@136);
//     total 160 bytes on both sides. Pinned by
//     TestYzmaParamsStructsMirrorLlamaHeader/context_params.
//   - llama_batch (llama.h:255-264) = 56 bytes vs llama.Batch
//     (llama.go:286-294); llama_token_data_array (llama.h:228-235) = 32 bytes,
//     selected@16 sorted@24, vs llama.TokenDataArray (llama.go:278-283);
//     llama_sampler_chain_params (llama.h:445-447) = 1 byte vs
//     llama.SamplerChainParams (llama.go:405-407);
//     llama_model_quantize_params (llama.h:423-438) = 56 bytes / 14 members vs
//     llama.ModelQuantizeParams (llama.go:381-396) / ffiTypeModelQuantizeParams
//     (model.go:23-25); llama_logit_bias, llama_chat_message,
//     llama_sampler_seq_config: all match.
//   - Enums: llama_rope_scaling_type, llama_pooling_type,
//     llama_attention_type, llama_flash_attn_type (AUTO=-1/DISABLED=0/
//     ENABLED=1, llama.h:190-194), llama_load_mode, llama_context_type,
//     llama_ftype, ggml_type (0..30, 34, 35, 39) — numeric values all match.
//   - LLAMA_STATE_SEQ_FLAGS_PARTIAL_ONLY == 1 (llama.h:898); the three
//     llama_state_seq_*_ext signatures (llama.h:906-923) are registered with
//     the correct arity and a uint32 flags argument in
//     .extras/yzma/pkg/llama/state.go:165-175.
//   - Kronk builds both params structs from the llama.cpp getters, never from a
//     zero Go struct: modelCtxParams starts at llama.ContextDefaultParams()
//     (sdk/kronk/model/config.go:842) and buildModelParams at
//     llama.ModelDefaultParams() (sdk/kronk/model/model.go:429), and every
//     subsequent write is gated on an explicit config value.
// =============================================================================

// llamaHeaderPath returns the vendored llama.h, skipping the test when the
// .extras reference tree is not checked out (it is optional, so these tests
// must stay portable).
func llamaHeaderPath(t *testing.T, root string) string {
	t.Helper()

	path := filepath.Join(root, ".extras", "llama.cpp", "include", "llama.h")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("vendored llama.cpp reference not present at %s: %v", path, err)
	}

	return path
}

// cStructFieldNames returns the member names of the C struct named name, in
// declaration order, as written in the header at path.
//
// The grammar it needs to handle is small and fixed: one member per line,
// terminated by ';', with '//' comments and comment-only continuation lines
// interleaved. Every member name is the last identifier before the ';'.
func cStructFieldNames(t *testing.T, path string, name string) []string {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	open := regexp.MustCompile(`(?m)^\s*(?:typedef\s+)?struct\s+` + regexp.QuoteMeta(name) + `\s*\{\s*$`)
	loc := open.FindIndex(src)
	if loc == nil {
		t.Fatalf("%s: cannot find the opening line of struct %s", path, name)
	}

	body := string(src[loc[1]:])
	ident := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

	var fields []string

	for line := range strings.SplitSeq(body, "\n") {
		code := strings.TrimSpace(line)
		if i := strings.Index(code, "//"); i >= 0 {
			code = strings.TrimSpace(code[:i])
		}

		switch {
		case code == "":
			continue

		case strings.HasPrefix(code, "}"):
			if len(fields) == 0 {
				t.Fatalf("%s: struct %s parsed as empty", path, name)
			}

			return fields
		}

		if !strings.HasSuffix(code, ";") {
			t.Fatalf("%s: struct %s has a member line this parser cannot handle: %q", path, name, code)
		}

		names := ident.FindAllString(strings.TrimSuffix(code, ";"), -1)
		if len(names) == 0 {
			t.Fatalf("%s: struct %s member line has no identifier: %q", path, name, code)
		}

		fields = append(fields, names[len(names)-1])
	}

	t.Fatalf("%s: struct %s is never closed", path, name)

	return nil
}

// normalizeFieldName folds a C snake_case member name and a Go exported field
// name onto the same key, so NGpuLayers matches n_gpu_layers and KVUnified
// matches kv_unified.
func normalizeFieldName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// TestYzmaParamsStructsMirrorLlamaHeader pins the by-value params structs that
// yzma hands to llama_model_load_from_file and llama_init_from_model against
// the C declarations in the vendored llama.h, field for field and in order.
//
// FINDING (subtest "model_params"): llama.cpp added a sixth boolean,
// `bool load_mtp`, to `llama_model_params` — .extras/llama.cpp/include/llama.h:340,
// introduced by llama.cpp commit 82dbc4f01 ("llama : load MTP tensors only if
// they are really used", first tagged b10212). yzma mirrors only the five
// older booleans: .extras/yzma/pkg/llama/llama.go:309-325 (struct ModelParams,
// last field NoAlloc) and .extras/yzma/pkg/llama/model.go:16-20
// (ffiTypeModelParams, fifteen libffi elements where C now has sixteen).
// There is therefore no Go field that can carry load_mtp, and
// sdk/kronk/model/model.go:428-534 (buildModelParams) has nothing to set —
// grep confirms `load_mtp` appears nowhere in Kronk outside a JSON tag.
//
// CONCRETE FAILURE SCENARIO. Both layouts are 72 bytes (five bools end at
// offset 68 and tail-pad to 72; six bools end at 69 and tail-pad to 72), so
// nothing errors and no size assertion catches it — the C byte at offset 69 is
// simply unreachable Go padding, permanently zero. Against a llama.cpp >= b10212
// that means load_mtp = false on every llama_model_load_from_file. For the
// architecture of the user's model (general.architecture = qwen35moe, with
// qwen35moe.nextn_predict_layers = 1 and blk.40.nextn.* tensors present in the
// GGUF), .extras/llama.cpp/src/models/qwen35moe.cpp:45 then computes
// `mtp_flags = TENSOR_SKIP`, and .extras/llama.cpp/src/llama-model-loader.cpp:1100
// drops every blk.40.nextn.* tensor with "model has unused tensor ... ignoring".
// layer.nextn.eh_proj / enorm / hnorm stay nullptr, and the first MTP graph
// build — which Kronk requests by setting CtxType = llama.ContextTypeMTP at
// sdk/kronk/model/draft_mtp.go:87 and :316 — hits
// .extras/llama.cpp/src/models/qwen35moe.cpp:565-567
// `GGML_ASSERT(layer.nextn.eh_proj && "MTP block missing nextn.eh_proj")` and
// aborts the process.
//
// Today Kronk pins defaultVersion = "b10211" (sdk/tools/libs/libs.go:29), the
// last build before load_mtp existed, which is the only reason this is latent
// rather than a hard crash; see
// TestVendoredLlamaCppMatchesPinnedLibraryBuild for that gap.
//
// The subtest "context_params" asserts the same correspondence for
// llama_context_params (llama.h:350-409) vs llama.ContextParams
// (.extras/yzma/pkg/llama/llama.go:335-378). It passes today and exists so an
// upstream insert into the struct that carries rope scaling, KV cache types,
// n_ubatch, n_seq_max and the flash-attention mode cannot land unnoticed.
func TestYzmaParamsStructsMirrorLlamaHeader(t *testing.T) {
	root := kronkRepoRoot(t)
	header := llamaHeaderPath(t, root)

	tests := []struct {
		name   string
		cName  string
		goType reflect.Type

		// aliases maps a C member name to the Go field name when yzma chose a
		// spelling that does not fold onto the C one. Layout-neutral.
		aliases map[string]string
	}{
		{
			name:   "model_params",
			cName:  "llama_model_params",
			goType: reflect.TypeOf(llama.ModelParams{}),
		},
		{
			name:   "context_params",
			cName:  "llama_context_params",
			goType: reflect.TypeOf(llama.ContextParams{}),
			aliases: map[string]string{
				"flash_attn_type": "FlashAttentionType",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cFields := cStructFieldNames(t, header, tt.cName)

			goFields := make([]string, 0, tt.goType.NumField())
			for i := range tt.goType.NumField() {
				goFields = append(goFields, tt.goType.Field(i).Name)
			}

			for i, cField := range cFields {
				want := normalizeFieldName(cField)
				if alias, ok := tt.aliases[cField]; ok {
					want = normalizeFieldName(alias)
				}

				if i >= len(goFields) {
					t.Errorf("%s member %d %q (llama.h) has NO counterpart in %s: the Go mirror stops after %d fields, so the C bytes for %q are unreachable padding and can never be set",
						tt.cName, i, cField, tt.goType, len(goFields), cField)

					continue
				}

				if got := normalizeFieldName(goFields[i]); got != want {
					t.Errorf("%s member %d: llama.h says %q, %s field %d is %q — the by-value struct is shifted from here on",
						tt.cName, i, cField, tt.goType, i, goFields[i])
				}
			}

			if len(goFields) > len(cFields) {
				t.Errorf("%s: %s has %d fields but llama.h declares %d; the extra Go fields are written past the C struct",
					tt.cName, tt.goType, len(goFields), len(cFields))
			}
		})
	}
}

// TestVendoredLlamaCppMatchesPinnedLibraryBuild pins the reference-vs-runtime
// build gap that lets an ABI change like `load_mtp` slip in unseen.
//
// FINDING. sdk/tools/libs/libs.go:29 pins the shared library Kronk downloads
// and dlopens to `defaultVersion = "b10211"`, and the installed library really
// is that build (~/.kronk/libraries/.../libllama.0.0.10211.dylib). The
// vendored reference tree that every layout audit reads —
// .extras/llama.cpp, whose include/llama.h is the authority for yzma's
// hand-mirrored structs — is checked out at b10217+1 commit
// (`git describe --tags` reports b10217-1-gde699957b), seven builds ahead.
//
// CONCRETE FAILURE SCENARIO. The two builds are not ABI-identical. `git diff
// b10211..HEAD -- include/llama.h` in the vendored tree is exactly one hunk:
// `+ bool load_mtp;` appended to llama_model_params (llama.h:340). So
// llama_model_params is 15 members in the library that actually runs and 16 in
// the header used to review yzma — which is why yzma's 15-member mirror both
// looks correct against the running library and is silently wrong for the
// header in the repo. Reviewing against the wrong header cuts both ways: a
// field added between the pinned build and the vendored tip reads as "yzma is
// missing a field" (harmless until the pin moves, then a hard abort — see
// TestYzmaParamsStructsMirrorLlamaHeader), and a field REMOVED or reordered
// upstream would read as "yzma is fine" while every argument after it is
// scrambled at runtime.
//
// The invariant is that the vendored reference is checked out at exactly the
// pinned build, so header-based audits describe the library that runs.
func TestVendoredLlamaCppMatchesPinnedLibraryBuild(t *testing.T) {
	root := kronkRepoRoot(t)

	tree := filepath.Join(root, ".extras", "llama.cpp")
	if _, err := os.Stat(filepath.Join(tree, ".git")); err != nil {
		t.Skipf("vendored llama.cpp git checkout not present at %s: %v", tree, err)
	}

	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not available: %v", err)
	}

	out, err := exec.Command(git, "-C", tree, "describe", "--tags").Output()
	if err != nil {
		t.Skipf("git describe in %s failed: %v", tree, err)
	}

	described := strings.TrimSpace(string(out))
	pinned := pinnedLibsDefaultVersion(t, root)

	build := regexp.MustCompile(`^b[0-9]+`).FindString(described)
	if build == "" {
		t.Fatalf("cannot read a llama.cpp build tag out of %q", described)
	}

	if build != pinned {
		t.Errorf("vendored .extras/llama.cpp is at build %s (git describe %q) but sdk/tools/libs/libs.go pins defaultVersion = %q: include/llama.h does not describe the libllama Kronk loads",
			build, described, pinned)
	}

	if described != build {
		t.Errorf("vendored .extras/llama.cpp is %s, i.e. ahead of tag %s: commits past the pinned build can add, remove or reorder struct members (b10212 appended llama_model_params.load_mtp) with nothing to catch it",
			described, build)
	}
}

// pinnedLibsDefaultVersion reads the unexported defaultVersion constant out of
// sdk/tools/libs/libs.go, so the assertion tracks the real pin instead of a
// copy of it.
func pinnedLibsDefaultVersion(t *testing.T, root string) string {
	t.Helper()

	path := filepath.Join(root, "sdk", "tools", "libs", "libs.go")
	file := parseKronkSource(t, token.NewFileSet(), path)

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}

		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range vs.Names {
				if name.Name != "defaultVersion" || i >= len(vs.Values) {
					continue
				}

				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("%s: defaultVersion is not a string literal", path)
				}

				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("%s: unquote defaultVersion: %v", path, err)
				}

				return v
			}
		}
	}

	t.Fatalf("%s: cannot find the defaultVersion constant", path)

	return ""
}
