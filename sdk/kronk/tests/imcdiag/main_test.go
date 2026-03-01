/*
IMC Diagnostic — multi-turn conversation logger for verifying IMC behavior.

Runs a single chat thread against each of the 4 IMC architecture/template
combinations, with full model-level logging captured to a file. Feed the
resulting log file to an AI for analysis.

Combinations tested:

	Combination               Model                              IMC Code Path
	------------------------  ---------------------------------  ------------------------------------
	Dense + Vision            M3 Qwen2.5-VL-3B-Instruct-Q8_0     Hash matching + media build/extend
	MoE + Vision              M4 Qwen3-VL-30B-A3B-Instruct-Q8_0  Hash matching + media build/extend
	Hybrid + Vision           M5 Qwen_Qwen3.5-35B-A3B-Q8_0       Hash matching + media build/extend + snapshot/restore
	MoE + Non-Deterministic   M2 gpt-oss-20b-Q8_0                Token prefix fallback

Conversation script (same for all models, image step skipped for non-vision):

	Turn 1: [sys + user]                                   → buildIMCCacheFromScratch
	Turn 2: [sys + user + asst + user]                     → hash match → extendIMCCache
	Turn 3: [... + asst + user+image]                      → hash match → media build/extend  (vision only)
	Turn 4: [... + asst + user]                            → hash match → extendIMCCache

Usage:

	IMC_DIAG_LOG=imc_diag.log go test -v -run TestDiag -timeout=30m ./sdk/kronk/tests/imcdiag/
	IMC_DIAG_LOG=imc_diag.log go test -v -run TestDiag_DenseVision -timeout=30m ./sdk/kronk/tests/imcdiag/
*/
package imcdiag_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

var (
	diagLog     model.Logger
	diagLogFile *os.File
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Println("skipping imcdiag tests in GitHub Actions")
		os.Exit(0)
	}

	// Require IMC_DIAG_LOG — the whole point is capturing logs.
	logPath := os.Getenv("IMC_DIAG_LOG")
	if logPath == "" {
		fmt.Println("IMC_DIAG_LOG not set — skipping imcdiag tests")
		fmt.Println("  IMC_DIAG_LOG=imc_diag.log go test -v -run TestDiag -timeout=30m ./sdk/kronk/tests/imcdiag/")
		os.Exit(0)
	}

	f, err := os.Create(logPath)
	if err != nil {
		fmt.Printf("imcdiag: unable to create log file %s: %v\n", logPath, err)
		os.Exit(1)
	}
	diagLogFile = f

	logger := slog.New(slog.NewTextHandler(f, nil))
	diagLog = func(ctx context.Context, msg string, args ...any) {
		logger.Info(msg, args...)
	}

	fmt.Printf("imcdiag: logging to %s\n", logPath)

	testlib.Setup()

	ctlg, err := catalog.New()
	if err != nil {
		fmt.Printf("imcdiag: unable to create catalog: %v\n", err)
		os.Exit(1)
	}

	if err := ctlg.Download(context.Background()); err != nil {
		fmt.Printf("imcdiag: unable to download catalog: %v\n", err)
		os.Exit(1)
	}

	if err := kronk.Init(); err != nil {
		fmt.Printf("imcdiag: unable to init kronk: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	diagLogFile.Close()

	os.Exit(code)
}
