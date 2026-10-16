package model

import (
	"context"
	"fmt"
	"time"

	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// mediaSetup owns CPU preparation for one admitted request. Only the worker
// writes its native resources until done closes; only the batch loop mutates
// ready and consumes the result. The slot bounds worker concurrency and keeps
// its projector and model alive until the worker has returned.
type mediaSetup struct {
	done        chan struct{}
	cancel      context.CancelFunc
	ready       bool
	media       *imcMediaPreparation
	inputChunks mtmd.InputChunks
	bitmaps     []mtmd.Bitmap
	totalTokens int
	err         error
}

func (e *batchEngine) queueMediaSetup(s *slot) {
	job := s.job
	ctx, cancel := context.WithCancel(job.ctx)
	setup := mediaSetup{done: make(chan struct{}), cancel: cancel}
	s.mediaSetup = &setup
	mtmdCtx, seqID := s.mtmdCtx, s.seqID

	go func() {
		started := time.Now()
		defer func() {
			e.model.log(ctx, "media-setup", "status", "complete", "seq", seqID,
				"elapsed", fmtDur(time.Since(started)), "err", setup.err)
			close(setup.done)
			select {
			case e.wakeCh <- struct{}{}:
			default:
			}
		}()

		if job.imcMediaBuild || job.imcMediaAppend {
			// Position and cached-prefix selection belong to the batch loop,
			// after restore. Tokenization does not need either of them.
			setup.media, setup.err = e.model.newIMCMediaPreparation(ctx, job.imcMediaCacheD, seqID, mtmdCtx)
			return
		}
		setup.err = setup.prepare(ctx, mtmdCtx, job.prompt, job.media)
	}()
}

func (p *mediaSetup) prepare(ctx context.Context, mtmdCtx mtmd.Context, prompt string, media [][]byte) error {
	p.bitmaps = make([]mtmd.Bitmap, len(media))
	for i, med := range media {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(med) == 0 {
			return fmt.Errorf("media[%d] is empty", i)
		}
		bmp, err := newMediaBitmap(mtmdCtx, med)
		if err != nil {
			return fmt.Errorf("media[%d]: %w", i, err)
		}
		p.bitmaps[i] = bmp
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.inputChunks = mtmd.InputChunksInit()
	if err := tokenizeMedia(mtmdCtx, p.inputChunks, prompt, p.bitmaps); err != nil {
		return err
	}
	for i := range mtmd.InputChunksSize(p.inputChunks) {
		p.totalTokens += int(mtmd.InputChunkGetNTokens(mtmd.InputChunksGet(p.inputChunks, i)))
	}
	return ctx.Err()
}

// resumeMediaSetup never waits for CPU work. A cancelled request retains its
// slot until preparation returns, so neither slot reuse nor unload can free
// resources still borrowed by the worker.
func (e *batchEngine) resumeMediaSetup(buf []byte) {
	for _, s := range e.slots {
		p := s.mediaSetup
		if !s.active || p == nil || p.ready {
			continue
		}
		select {
		case <-p.done:
			p.ready = true
			if err := s.job.ctx.Err(); err != nil {
				e.finishSlot(s, err)
				continue
			}
			if p.err != nil {
				e.finishSlot(s, fmt.Errorf("prepare media: %w", p.err))
				continue
			}
			e.continueStartSlot(s, s.job, buf)
		default:
		}
	}
}

func (p *mediaSetup) free() {
	p.cancel()
	<-p.done
	if p.media != nil {
		p.media.free()
	}
	if p.inputChunks != 0 {
		mtmd.InputChunksFree(p.inputChunks)
	}
	for _, b := range p.bitmaps {
		if b != 0 {
			mtmd.BitmapFree(b)
		}
	}
}
