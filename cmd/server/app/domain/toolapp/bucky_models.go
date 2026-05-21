package toolapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// BuckyModelEntry describes a single installed whisper model.
type BuckyModelEntry struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// BuckyModelsResponse is the list of installed whisper models returned
// by the /v1/bucky/models endpoint.
type BuckyModelsResponse struct {
	Models []BuckyModelEntry `json:"models"`
}

// Encode implements the encoder interface.
func (b BuckyModelsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(b)
	return data, "application/json", err
}

// BuckyCatalogEntry describes a single bundled whisper model that
// callers may pull by short name.
type BuckyCatalogEntry struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Size string `json:"size"`
}

// BuckyCatalogResponse is the bundled short-name table returned by the
// /v1/bucky/models/catalog endpoint.
type BuckyCatalogResponse struct {
	Models []BuckyCatalogEntry `json:"models"`
}

// Encode implements the encoder interface.
func (b BuckyCatalogResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(b)
	return data, "application/json", err
}

// BuckyPullRequest is the body shape for /v1/bucky/models/pull.
type BuckyPullRequest struct {
	Source string `json:"source"`
}

// Decode implements the decoder interface.
func (b *BuckyPullRequest) Decode(data []byte) error {
	return json.Unmarshal(data, b)
}

// BuckyModelActionResponse is returned by mutating model endpoints
// (remove) to confirm which model was acted upon.
type BuckyModelActionResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// Encode implements the encoder interface.
func (b BuckyModelActionResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(b)
	return data, "application/json", err
}

// =============================================================================

func (a *app) listBuckyModels(ctx context.Context, r *http.Request) web.Encoder {
	files, err := a.buckyModels.Files()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve bucky model list: %s", err)
	}

	out := BuckyModelsResponse{Models: make([]BuckyModelEntry, len(files))}
	for i, f := range files {
		out.Models[i] = BuckyModelEntry{
			ID:       f.ID,
			Path:     f.Path,
			Size:     f.Size,
			Modified: f.Modified,
		}
	}

	return out
}

func (a *app) listBuckyCatalog(ctx context.Context, r *http.Request) web.Encoder {
	cat := buckymodels.Catalog()

	ids := make([]string, 0, len(cat))
	for id := range cat {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	out := BuckyCatalogResponse{Models: make([]BuckyCatalogEntry, len(ids))}
	for i, id := range ids {
		entry := cat[id]
		out.Models[i] = BuckyCatalogEntry{
			ID:   id,
			URL:  entry.URL,
			Size: entry.Size,
		}
	}

	return out
}

func (a *app) pullBuckyModel(ctx context.Context, r *http.Request) web.Encoder {
	var req BuckyPullRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		return errs.Errorf(errs.InvalidArgument, "source is required")
	}

	a.log.Info(ctx, "pull-bucky-model", "source", source)

	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(6 * time.Hour)); err != nil {
		a.log.Info(ctx, "pull-bucky-model", "set-write-deadline", "ERROR", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	// -------------------------------------------------------------------------

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				fmt.Fprintf(&sb, " %v[%v]", args[i], args[i+1])
			}
		}

		cleanMsg := strings.TrimPrefix(msg, "\r\x1b[K")

		clean := cleanMsg
		if sb.Len() > 0 {
			clean = fmt.Sprintf("%s:%s", cleanMsg, sb.String())
		}

		var ver string
		if m := reDownloadProgress.FindStringSubmatch(clean); m != nil {
			cur, _ := strconv.ParseInt(m[2], 10, 64)
			total, _ := strconv.ParseInt(m[3], 10, 64)
			mbps, _ := strconv.ParseFloat(m[4], 64)
			ver = toAppPullResponse(PullResponse{
				Status: clean,
				Progress: &PullProgress{
					Src:          m[1],
					CurrentBytes: cur * 1000 * 1000,
					TotalBytes:   total * 1000 * 1000,
					MBPerSec:     mbps,
					Complete:     total > 0 && cur >= total,
				},
			})
		} else {
			ver = toAppPullResponse(PullResponse{Status: clean})
		}

		a.log.Info(ctx, "pull-bucky-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	mp, err := a.buckyModels.Download(ctx, logger, source)
	if err != nil {
		ver := toAppPullResponse(PullResponse{Status: err.Error()})
		a.log.Info(ctx, "pull-bucky-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
		return web.NewNoResponse()
	}

	final := PullResponse{Status: "downloaded"}
	if len(mp.ModelFiles) > 0 {
		final.Status = fmt.Sprintf("downloaded:%s", mp.ModelFiles[0])
	}
	ver := toAppPullResponse(final)
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) removeBuckyModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	a.log.Info(ctx, "remove-bucky-model", "modelID", modelID)

	mp, err := a.buckyModels.FullPath(modelID)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := a.buckyModels.Remove(mp, a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "failed to remove bucky model: %s", err)
	}

	return BuckyModelActionResponse{Status: "removed", ID: modelID}
}
