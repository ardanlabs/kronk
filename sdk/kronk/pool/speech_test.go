package pool_test

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// TestSpeechPoolLifecycle exercises both speech bundles installed by
// make install-test-models. Missing fixtures fail rather than skip.
func TestSpeechPoolLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("native speech lifecycle skipped in short mode")
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"ggml-org/chatterbox-turbo-Q8_0",
		"ggml-org/Qwen3-TTS-12Hz-1.7B-Base-Q8_0",
	} {
		t.Run(id, func(t *testing.T) {
			mp, err := mdls.FullPath(id)
			if err != nil || len(mp.ModelFiles) == 0 || mp.ProjFile == "" {
				t.Fatalf("missing speech fixture %s: %v; run make install-test-models", id, err)
			}
			testSpeechPoolLifecycle(t, mp)
		})
	}
}

func testSpeechPoolLifecycle(t *testing.T, mp models.Path) {
	t.Helper()
	mdls, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mdls.Path(), "test", "speech")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"speech.gguf": mp.ModelFiles[0], "mmproj-speech.gguf": mp.ProjFile} {
		if err := os.Symlink(source, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	logger := func(context.Context, string, ...any) {}
	if err := mdls.BuildIndex(logger, false); err != nil {
		t.Fatal(err)
	}
	newPool := func(t *testing.T, snapshot resman.Snapshot) *pool.Pool {
		t.Helper()
		rm, err := resman.New(resman.Config{Snapshot: snapshot})
		if err != nil {
			t.Fatal(err)
		}
		p, err := pool.New(pool.Config{Log: logger, Models: mdls, Resman: rm})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := p.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		})
		return p
	}
	const id = "test/speech"
	t.Run("cold-load-reuse-and-release", func(t *testing.T) {
		p := newPool(t, resman.FromDevices(devices.List()))
		krn, err := p.AquireModel(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		cfg := krn.ModelConfig()
		if !krn.ModelInfo().IsSpeechModel || cfg.ContextWindow() != 2048 || cfg.NSeqMax() != 1 || cfg.SpeechMaxFrames() != 512 || cfg.ProjFile == "" {
			t.Fatalf("pooled speech settings: %+v", cfg)
		}
		usage := p.ResourceManager().Usage()
		if len(usage.Reservations) != 1 || usage.RAMUsed+usage.Reservations[0].VRAMBytes <= 0 {
			t.Fatalf("missing speech reservation: %+v", usage)
		}
		t.Logf("cold reservation: RAM=%d VRAM=%d", usage.RAMUsed, usage.Reservations[0].VRAMBytes)
		cached, err := p.AquireModel(t.Context(), id)
		if err != nil || cached != krn || len(p.ResourceManager().Usage().Reservations) != 1 {
			t.Fatalf("cache hit changed handle or reservation: %v", err)
		}
		response, err := krn.Speech(t.Context(), model.SpeechRequest{Input: "Mary had a little lamb, its fleece was white as snow."})
		if err != nil {
			t.Fatal(err)
		}
		wav := response.Audio
		if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" ||
			binary.LittleEndian.Uint16(wav[20:22]) != 1 || binary.LittleEndian.Uint16(wav[22:24]) != 1 ||
			binary.LittleEndian.Uint16(wav[34:36]) != 16 || response.SampleRate != 24000 ||
			response.Samples <= 0 || int64(len(wav)) != 44+2*response.Samples {
			t.Fatalf("invalid pooled WAV: rate=%d samples=%d bytes=%d", response.SampleRate, response.Samples, len(wav))
		}
		t.Logf("pooled WAV: rate=%d samples=%d bytes=%d", response.SampleRate, response.Samples, len(wav))
		if err := p.InvalidateSync(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		if _, exists := p.GetExisting(id); exists || len(p.ResourceManager().Usage().Reservations) != 0 || p.ResourceManager().Usage().RAMUsed != 0 {
			t.Fatal("eviction retained the speech handle or reservation")
		}
		if _, err := krn.Speech(t.Context(), model.SpeechRequest{Input: "Hello."}); err == nil {
			t.Fatal("evicted speech handle still accepts requests")
		}
	})
	t.Run("busy-protection-and-idle-eviction", func(t *testing.T) {
		p := newPool(t, resman.FromDevices(devices.List()))
		if _, err := p.AquireModel(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		// Probe the reservation, then allow one bundle but not two. The item
		// cap remains ten, so only memory pressure can trigger this eviction.
		footprint := p.ResourceManager().Usage().RAMUsed
		if err := p.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		snapshot := resman.FromDevices(devices.List())
		snapshot.RAMBytes = footprint * 3 / 2
		p = newPool(t, snapshot)
		krn, err := p.AquireModel(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		usage := p.ResourceManager().Usage()
		if usage.RAMUsed <= 0 || usage.RAMBudget < usage.RAMUsed || usage.RAMBudget >= 2*usage.RAMUsed {
			t.Fatalf("test budget must fit exactly one bundle: %+v", usage)
		}
		cfg := krn.ModelConfig()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		done := make(chan struct{})
		var speechErr error
		go func() {
			defer close(done)
			_, speechErr = krn.Speech(ctx, model.SpeechRequest{Input: strings.Repeat("We are playing Connect Four together. Take your time to look at the board before choosing a column. You can win by connecting four pieces in a row, but you also need to block your opponent. Think about the center of the board and watch for diagonal threats. ", 3)})
		}()
		t.Cleanup(func() {
			cancel()
			<-done
		})
		// Wait for the public active-use counter, not a scheduler detail or a
		// guessed delay. The long request keeps work active during planning.
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for krn.ActiveStreams() == 0 {
			select {
			case <-done:
				t.Fatalf("speech ended before becoming active: %v", speechErr)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-ticker.C:
			}
		}
		key := id + "/playground/replacement"
		if _, err := p.AquireCustom(t.Context(), key, cfg); !errors.Is(err, pool.ErrServerBusy) {
			select {
			case <-done:
				t.Fatalf("speech ended during competing acquisition: %v; acquire error: %v", speechErr, err)
			default:
			}
			t.Fatalf("eviction during speech: got %v, want ErrServerBusy", err)
		}
		if err := p.InvalidateSync(t.Context(), id); err == nil {
			t.Fatal("explicit invalidation evicted active speech")
		}
		if existing, ok := p.GetExisting(id); !ok || existing != krn || len(p.ResourceManager().Usage().Reservations) != 1 {
			t.Fatal("busy eviction lost the handle or reservation")
		}
		cancel()
		<-done
		if !errors.Is(speechErr, context.Canceled) || krn.ActiveStreams() != 0 {
			t.Fatalf("canceled pooled speech: err=%v active=%d", speechErr, krn.ActiveStreams())
		}
		replacement, err := p.AquireCustom(t.Context(), key, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := p.GetExisting(id); exists || replacement == krn {
			t.Fatal("idle speech handle was not replaced")
		}
		usage = p.ResourceManager().Usage()
		if len(usage.Reservations) != 1 || usage.Reservations[0].Key != key {
			t.Fatalf("eviction retained the old reservation: %+v", usage)
		}
		if response, err := replacement.Speech(t.Context(), model.SpeechRequest{Input: "Mary had a little lamb, its fleece was white as snow."}); err != nil || len(response.Audio) <= 44 {
			t.Fatalf("replacement speech: bytes=%d err=%v", len(response.Audio), err)
		}
	})
	t.Run("insufficient-budget", func(t *testing.T) {
		snapshot := resman.FromDevices(devices.List())
		snapshot.RAMBytes = 1 << 20
		p := newPool(t, snapshot)
		if _, err := p.AquireModel(t.Context(), id); !errors.Is(err, pool.ErrNoCapacity) {
			t.Fatalf("got %v, want ErrNoCapacity", err)
		}
		if _, exists := p.GetExisting(id); exists || len(p.ResourceManager().Usage().Reservations) != 0 {
			t.Fatal("rejected speech load retained a handle or reservation")
		}
	})
	t.Run("failed-load-rolls-back", func(t *testing.T) {
		p := newPool(t, resman.FromDevices(devices.List()))
		cfg, err := p.ResolvedKronkConfig(id)
		if err != nil {
			t.Fatal(err)
		}
		// The bundle can be planned, but native loading cannot resolve this
		// projector device. Failure must release the pre-load reservation.
		cfg.ProjDevice = "speech-test-missing-device"
		key := id + "/playground/failed-load"
		if _, err := p.AquireCustom(t.Context(), key, cfg); err == nil || !strings.Contains(err.Error(), "resolve projector device") {
			t.Fatalf("got %v, want native projector device failure", err)
		}
		if _, exists := p.GetExisting(key); exists || len(p.ResourceManager().Usage().Reservations) != 0 || p.ResourceManager().Usage().RAMUsed != 0 {
			t.Fatal("failed speech load retained a handle or reservation")
		}
		cfg.ProjDevice = ""
		if _, err := p.AquireCustom(t.Context(), key, cfg); err != nil {
			t.Fatalf("retry after failed speech load: %v", err)
		}
		if err := p.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(p.ResourceManager().Usage().Reservations) != 0 || p.ResourceManager().Usage().RAMUsed != 0 {
			t.Fatal("shutdown retained speech reservations")
		}
	})
}
