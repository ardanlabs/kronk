package downapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

// PeerBundleTagResponse describes a single bundle advertised by a peer
// Kronk server. Size and SHA256 are only populated when the peer has
// already built the cached zip for that bundle.
type PeerBundleTagResponse struct {
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
	Version   string `json:"version"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

// PeerBundleListResponse is the list of bundles returned by a peer
// /download/libs query, surfaced to the BUI through
// /v1/libs/peer-bundles.
type PeerBundleListResponse struct {
	Bundles []PeerBundleTagResponse `json:"bundles"`
}

// Encode implements the encoder interface.
func (app PeerBundleListResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppPeerBundleList(in []libs.PeerBundle) PeerBundleListResponse {
	out := PeerBundleListResponse{Bundles: make([]PeerBundleTagResponse, len(in))}

	for i, b := range in {
		out.Bundles[i] = PeerBundleTagResponse{
			Arch:      b.Arch,
			OS:        b.OS,
			Processor: b.Processor,
			Version:   b.Version,
			Size:      b.Size,
			SHA256:    b.SHA256,
		}
	}

	return out
}

// PeerPullEvent is the SSE payload emitted by /v1/libs/pull-from-peer as
// the peer transfer progresses through its phases. Status carries one of:
// "connecting", "metadata", "downloading", "downloaded", "verifying",
// "unzipping", "swapping", "complete", "error".
type PeerPullEvent struct {
	Status      string  `json:"status"`
	Arch        string  `json:"arch,omitempty"`
	OS          string  `json:"os,omitempty"`
	Processor   string  `json:"processor,omitempty"`
	Version     string  `json:"version,omitempty"`
	Bytes       int64   `json:"bytes,omitempty"`
	BytesTotal  int64   `json:"bytes_total,omitempty"`
	MBPerSecond float64 `json:"mb_per_second,omitempty"`
	Size        int64   `json:"size,omitempty"`
	SHA256      string  `json:"sha256,omitempty"`
	Error       string  `json:"error,omitempty"`
}

func toPeerPullEvent(p PeerPullEvent) string {
	d, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf("data: {\"status\":%q}\n", err.Error())
	}
	return fmt.Sprintf("data: %s\n", string(d))
}

// =============================================================================

func (a *app) listPeerLibsBundles(ctx context.Context, r *http.Request) web.Encoder {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		return errs.Errorf(errs.InvalidArgument, "host is required")
	}

	bundles, err := libs.FetchPeerBundles(ctx, host)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toAppPeerBundleList(bundles)
}

func (a *app) pullLibsFromPeer(ctx context.Context, r *http.Request) web.Encoder {
	if a.libs == nil {
		return errs.Errorf(errs.Internal, "libs not configured")
	}

	q := r.URL.Query()
	host := strings.TrimSpace(q.Get("host"))
	arch := q.Get("arch")
	opSys := q.Get("os")
	processor := q.Get("processor")

	if host == "" {
		return errs.Errorf(errs.InvalidArgument, "host is required")
	}
	if arch == "" || opSys == "" || processor == "" {
		return errs.Errorf(errs.InvalidArgument, "arch, os, and processor are required")
	}
	if !libs.IsSupported(arch, opSys, processor) {
		return errs.Errorf(errs.InvalidArgument, "unsupported combination arch=%q os=%q processor=%q", arch, opSys, processor)
	}

	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	emit := func(payload PeerPullEvent) {
		line := toPeerPullEvent(payload)
		a.log.Info(ctx, "pull-libs-from-peer", "info", strings.TrimSuffix(line, "\n"))
		fmt.Fprint(w, line)
		f.Flush()
	}

	progress := func(p libs.PullBundleProgress) {
		emit(PeerPullEvent{
			Status:      p.Phase,
			Arch:        arch,
			OS:          opSys,
			Processor:   processor,
			BytesTotal:  p.Total,
			Bytes:       p.Current,
			MBPerSecond: p.MBPerSecond,
			SHA256:      p.SHA256,
			Size:        p.Size,
		})
	}

	tag, err := a.libs.PullBundleFromPeer(ctx, host, arch, opSys, processor, progress)
	if err != nil {
		emit(PeerPullEvent{
			Status:    "error",
			Arch:      arch,
			OS:        opSys,
			Processor: processor,
			Error:     err.Error(),
		})
		return web.NewNoResponse()
	}

	emit(PeerPullEvent{
		Status:    "complete",
		Arch:      tag.Arch,
		OS:        tag.OS,
		Processor: tag.Processor,
		Version:   tag.Version,
	})

	return web.NewNoResponse()
}
