package gpt_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPGPTChat.ModelFiles) == 0 {
		fmt.Println("model gpt-oss-20b-Q8_0 not downloaded, skipping gpt tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
