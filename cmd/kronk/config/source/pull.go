package source

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/applog"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// PullCommand is the command that downloads the model c, for the user to run
// or copy.
func PullCommand(c Choice) string {
	switch c.Kind {
	case KindAudioModel:
		return "kronk bucky model pull --local " + c.ID
	case KindImageModel:
		return "kronk malina model pull --local " + c.ID
	}
	return "kronk model pull --local " + c.ID
}

// PullReportsProgress reports whether Pull calls progress for the model c.
// The whisper download reports none.
func PullReportsProgress(c Choice) bool {
	return c.Kind != KindAudioModel
}

// Pull downloads the model c into its models folder, the way PullCommand
// does. progress gets each file's progress. Nothing is logged, so the
// terminal UI stays intact. Cancel ctx to stop the download.
func (s *Source) Pull(ctx context.Context, c Choice, progress models.DownloadProgressFunc) error {
	switch c.Kind {
	case KindAudioModel:
		mdls, err := buckymodels.NewWithPaths(s.basePath)
		if err != nil {
			return fmt.Errorf("audio models: %w", err)
		}
		_, err = mdls.Download(ctx, applog.DiscardLogger, c.ID)
		return err

	case KindImageModel:
		mdls, err := malinamodels.NewWithPaths(s.basePath)
		if err != nil {
			return fmt.Errorf("image models: %w", err)
		}
		name, err := malinamodels.ParseBundleName(c.ID)
		if err != nil {
			return err
		}
		tracker := downloader.NewProgressReader(func(src string, current, total int64, mbPerSec float64, complete bool) {
			progress(models.DownloadProgress{Src: src, CurrentBytes: current, TotalBytes: total, MBPerSec: mbPerSec, Complete: complete})
		}, downloader.SizeIntervalMB10)
		_, err = mdls.DownloadBundleWithProgress(ctx, name, tracker)
		return err
	}

	_, err := s.mdls.DownloadWithProgress(ctx, applog.DiscardLogger, c.ID, progress)
	return err
}

// ModelDir is the folder the model c is installed in, empty when it isn't
// installed or can't be found.
func (s *Source) ModelDir(c Choice) string {
	var mp models.Path
	var err error

	switch c.Kind {
	case KindAudioModel:
		var mdls *buckymodels.Models
		if mdls, err = buckymodels.NewWithPaths(s.basePath); err == nil {
			mp, err = mdls.FullPath(c.ID)
		}
	case KindImageModel:
		var mdls *malinamodels.Models
		if mdls, err = malinamodels.NewWithPaths(s.basePath); err == nil {
			mp, err = mdls.FullPath(c.ID)
		}
	default:
		mp, err = s.mdls.FullPath(c.ID)
	}

	if err != nil || len(mp.ModelFiles) == 0 {
		return ""
	}
	return filepath.Dir(mp.ModelFiles[0])
}
