package transcribe_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/bucky/tests/testlib"
)

// Test_PooledTranscribe fires NSeqMax goroutines concurrently against
// the same Whisper handle. They share the same model weights but
// each must acquire a distinct whisper.State from the internal pool.
// All goroutines must complete and produce identical transcripts.
//
// The test also actively validates concurrency in two ways:
//
//  1. ActiveStreams is sampled while the parallel run is in flight
//     and must observe the counter reach numInstances. A serializing
//     wrapper would never let it rise above 1.
//
//  2. The wall-clock elapsed for the parallel run is compared
//     against a baseline single-shot Transcribe. With a real state
//     pool the parallel run completes in roughly one baseline; a
//     serialized pool would take numInstances baselines. The
//     assertion threshold has generous headroom for GPU jitter.
func Test_PooledTranscribe(t *testing.T) {
	const numInstances = 2

	// concurrencyFactor is the upper bound on (parallel wall-clock /
	// single-shot baseline) below which we accept the run as truly
	// concurrent. Serial execution would land at ~numInstances; a
	// perfectly parallel run at ~1.0. The 1.5x bound tolerates GPU
	// kernel-launch overhead and Metal queue contention while still
	// catching any regression that serializes the pool.
	const concurrencyFactor = 1.5

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	w, err := bucky.New(model.WithConfig(model.Config{
		ModelPath: testlib.MPTinyEn.ModelFiles[0],
		UseGPU:    true,
		NSeqMax:   numInstances,
	}))
	if err != nil {
		t.Fatalf("Failed to create whisper handle with NSeqMax=%d: %v", numInstances, err)
	}
	defer w.Unload(ctx)

	t.Logf("Testing pooled transcribe with NSeqMax=%d", numInstances)

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	// Single-shot baseline. Whisper compiles its Metal kernels and
	// allocates compute buffers lazily on the first call, so run two
	// warm-up passes and time the third to keep the baseline honest.
	for range 2 {
		if _, err := w.Transcribe(ctx, samples, model.WithLanguage("en")); err != nil {
			t.Fatalf("warm-up Transcribe: %v", err)
		}
	}
	baselineStart := time.Now()
	if _, err := w.Transcribe(ctx, samples, model.WithLanguage("en")); err != nil {
		t.Fatalf("baseline Transcribe: %v", err)
	}
	baseline := time.Since(baselineStart)
	t.Logf("single-shot baseline: %s", baseline)

	// Parallel run. The barrier releases every goroutine at the
	// same instant so the wall-clock measurement captures the
	// overlap, not goroutine scheduling skew.
	var wg sync.WaitGroup
	wg.Add(numInstances)

	startBarrier := make(chan struct{})
	durations := make([]time.Duration, numInstances)
	results := make([]string, numInstances)
	errors := make([]error, numInstances)

	for i := range numInstances {
		go func(idx int) {
			defer wg.Done()

			<-startBarrier

			start := time.Now()

			tr, err := w.Transcribe(ctx, samples, model.WithLanguage("en"))
			if err != nil {
				errors[idx] = fmt.Errorf("goroutine %d: %w", idx, err)
				return
			}

			durations[idx] = time.Since(start)
			results[idx] = tr.Text
		}(i)
	}

	parallelStart := time.Now()
	close(startBarrier)

	// Sample ActiveStreams while the parallel batch is in flight.
	// A pool that serializes would never let the counter rise above
	// 1; a real pool must reach numInstances on at least one sample.
	peak := pollPeakActiveStreams(w, &wg)

	wg.Wait()
	parallelElapsed := time.Since(parallelStart)

	for i, err := range errors {
		if err != nil {
			t.Errorf("Request %d failed: %v", i, err)
		}
	}

	if t.Failed() {
		return
	}

	first := results[0]
	if first == "" {
		t.Fatalf("Request 0: empty transcript")
	}
	for i, r := range results[1:] {
		if r != first {
			t.Errorf("Request %d: got %q, want %q", i+1, r, first)
		}
	}
	testlib.AssertTranscriptContains(t, first, "ask not")

	for i, d := range durations {
		t.Logf("Request %d completed in %s", i, d)
	}

	if got := w.ActiveStreams(); got != 0 {
		t.Errorf("ActiveStreams after wait: got %d, want 0", got)
	}

	// Concurrency check 1: peak in-flight count.
	if peak < numInstances {
		t.Errorf("peak ActiveStreams during parallel run: got %d, want %d (pool may be serializing)",
			peak, numInstances)
	}
	t.Logf("peak ActiveStreams observed: %d", peak)

	// Concurrency check 2: wall-clock speedup vs baseline.
	//
	// This check is meaningful only when there is spare compute for
	// the parallel run to consume — i.e. on a GPU. On CPU-only
	// hardware a single Transcribe already saturates every core, so
	// running NSeqMax in parallel takes ~NSeqMax × baseline even
	// when the pool is genuinely concurrent. Check 1 (peak
	// ActiveStreams) is the authoritative signal in that case.
	//
	// GitHub Actions runners have no GPU, so skip the wall-clock
	// assertion there and rely on the in-flight ActiveStreams probe.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Logf("parallel wall-clock: %s, baseline: %s (GITHUB_ACTIONS=true: skipping CPU-bound wall-clock check)",
			parallelElapsed, baseline)
	} else {
		threshold := time.Duration(float64(baseline) * concurrencyFactor)
		t.Logf("parallel wall-clock: %s, baseline: %s, threshold: %s (factor %.2fx)",
			parallelElapsed, baseline, threshold, concurrencyFactor)
		if parallelElapsed > threshold {
			t.Errorf("parallel wall-clock %s exceeded %.2fx baseline (%s); "+
				"expected concurrent execution but the pool appears to be serializing",
				parallelElapsed, concurrencyFactor, threshold)
		}
	}

	t.Logf("All %d concurrent transcribe requests completed successfully", numInstances)
}

// pollPeakActiveStreams samples w.ActiveStreams at a tight cadence
// until wg signals completion, returning the maximum value seen.
// It runs on the test goroutine so a missed peak is impossible —
// the polling loop exits only when every parallel call has returned.
func pollPeakActiveStreams(w *bucky.Bucky, wg *sync.WaitGroup) int {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	var peak int
	for {
		if n := w.ActiveStreams(); n > peak {
			peak = n
		}
		select {
		case <-done:
			return peak
		default:
			time.Sleep(100 * time.Microsecond)
		}
	}
}

// Test_ActiveStreams polls ActiveStreams while a Transcribe is
// in-flight and verifies the counter rises above zero, then returns
// to zero after the call completes.
func Test_ActiveStreams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	w, err := bucky.New(model.WithConfig(testlib.CfgTinyEn()))
	if err != nil {
		t.Fatalf("Failed to create whisper handle: %v", err)
	}
	defer w.Unload(ctx)

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	if got := w.ActiveStreams(); got != 0 {
		t.Fatalf("ActiveStreams before: got %d, want 0", got)
	}

	done := make(chan error, 1)
	go func() {
		_, err := w.Transcribe(ctx, samples, model.WithLanguage("en"))
		done <- err
	}()

	deadline := time.Now().Add(30 * time.Second)
	var sawActive bool
	for time.Now().Before(deadline) {
		if w.ActiveStreams() > 0 {
			sawActive = true
			break
		}
		select {
		case err := <-done:
			t.Fatalf("Transcribe finished before ActiveStreams was observed > 0 (err=%v)", err)
		case <-time.After(5 * time.Millisecond):
		}
	}
	if !sawActive {
		t.Fatalf("ActiveStreams: never observed > 0 during in-flight Transcribe")
	}

	if err := <-done; err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	if got := w.ActiveStreams(); got != 0 {
		t.Errorf("ActiveStreams after: got %d, want 0", got)
	}
}

// Test_UnloadDrain verifies that Unload waits for an in-flight
// Transcribe to complete, then frees the model. A second Unload on
// the now-freed handle must report the already-unloaded state.
func Test_UnloadDrain(t *testing.T) {
	w, err := bucky.New(model.WithConfig(testlib.CfgTinyEn()))
	if err != nil {
		t.Fatalf("Failed to create whisper handle: %v", err)
	}

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	done := make(chan error, 1)
	go func() {
		_, err := w.Transcribe(context.Background(), samples, model.WithLanguage("en"))
		done <- err
	}()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && w.ActiveStreams() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if w.ActiveStreams() == 0 {
		t.Fatalf("ActiveStreams: never observed > 0 before calling Unload")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := w.Unload(ctx); err != nil {
		t.Fatalf("Unload: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("in-flight Transcribe after Unload: %v", err)
	}

	err = w.Unload(context.Background())
	if err == nil {
		t.Fatalf("second Unload: got nil err, want already-unloaded error")
	}
	if !strings.Contains(err.Error(), "already unloaded") {
		t.Errorf("second Unload err: got %q, want substring %q", err.Error(), "already unloaded")
	}
}

// Test_UnloadTimeout verifies that Unload returns the
// active-streams error when its context fires before the in-flight
// Transcribe completes. The handle remains live so a subsequent
// Unload can drain it cleanly.
func Test_UnloadTimeout(t *testing.T) {
	w, err := bucky.New(model.WithConfig(testlib.CfgTinyEn()))
	if err != nil {
		t.Fatalf("Failed to create whisper handle: %v", err)
	}
	defer w.Unload(context.Background())

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	done := make(chan error, 1)
	go func() {
		_, err := w.Transcribe(context.Background(), samples, model.WithLanguage("en"))
		done <- err
	}()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && w.ActiveStreams() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if w.ActiveStreams() == 0 {
		t.Fatalf("ActiveStreams: never observed > 0 before calling Unload")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Unload(ctx)
	if err == nil {
		t.Fatalf("Unload with short timeout: got nil err, want timeout error")
	}
	if !strings.Contains(err.Error(), "too many active-streams") {
		t.Errorf("Unload err: got %q, want substring %q", err.Error(), "too many active-streams")
	}

	if err := <-done; err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
}
