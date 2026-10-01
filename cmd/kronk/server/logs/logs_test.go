package logs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (lb *lockedBuffer) Write(data []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.b.Write(data)
}

func (lb *lockedBuffer) String() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.b.String()
}

func TestFollowLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kronk.log")
	var initial strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&initial, "line %d\n", i)
	}
	if err := os.WriteFile(path, []byte(initial.String()), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output lockedBuffer
	errC := make(chan error, 1)
	go func() {
		errC <- followLog(ctx, path, &output, 5*time.Millisecond)
	}()

	waitForOutput(t, &output, "line 12\n")
	if strings.Contains(output.String(), "line 2\n") || !strings.HasPrefix(output.String(), "line 3\n") {
		t.Fatalf("initial output: got %q, want last 10 lines", output.String())
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := file.WriteString("appended\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	waitForOutput(t, &output, "appended\n")

	cancel()
	if err := <-errC; err != nil {
		t.Fatalf("followLog: %v", err)
	}
}

func waitForOutput(t *testing.T, output *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("output: got %q, want substring %q", output.String(), want)
}
