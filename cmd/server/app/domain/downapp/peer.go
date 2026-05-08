package downapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

// peerHTTPClient is the http.Client used for short JSON requests against
// peer Kronk servers. The 30s timeout keeps the BUI snappy if a peer is
// unreachable.
var peerHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
	},
}

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

// PeerModelDetail describes a single model advertised by a peer Kronk
// server. The shape mirrors the relevant fields of toolapp.ListModelDetail
// so the BUI can render the peer model grid the same way it renders the
// local model list.
type PeerModelDetail struct {
	ID            string `json:"id"`
	OwnedBy       string `json:"owned_by"`
	ModelFamily   string `json:"model_family"`
	Size          int64  `json:"size"`
	Validated     bool   `json:"validated"`
	HasProjection bool   `json:"has_projection"`
}

// PeerModelListResponse is the list of models returned by a peer
// /v1/models query, surfaced to the BUI through /v1/download/models/peer-models.
type PeerModelListResponse struct {
	Models []PeerModelDetail `json:"models"`
}

// Encode implements the encoder interface.
func (app PeerModelListResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// peerModelsRaw mirrors the JSON shape returned by the peer's /v1/models
// endpoint. Only the fields the BUI needs are decoded.
type peerModelsRaw struct {
	Data []struct {
		ID            string `json:"id"`
		OwnedBy       string `json:"owned_by"`
		ModelFamily   string `json:"model_family"`
		Size          int64  `json:"size"`
		Validated     bool   `json:"validated"`
		HasProjection bool   `json:"has_projection"`
	} `json:"data"`
}

// fetchPeerModels fetches the list of models advertised by the peer at
// host (in the form "ip:port") via its GET /v1/models endpoint.
func fetchPeerModels(ctx context.Context, host string) ([]PeerModelDetail, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("downapp: fetch-peer-models: host is required")
	}

	url := fmt.Sprintf("http://%s/v1/models", host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("downapp: fetch-peer-models: build request: %w", err)
	}

	resp, err := peerHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downapp: fetch-peer-models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("downapp: fetch-peer-models: peer returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw peerModelsRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("downapp: fetch-peer-models: decode: %w", err)
	}

	out := make([]PeerModelDetail, len(raw.Data))
	for i, m := range raw.Data {
		out[i] = PeerModelDetail{
			ID:            m.ID,
			OwnedBy:       m.OwnedBy,
			ModelFamily:   m.ModelFamily,
			Size:          m.Size,
			Validated:     m.Validated,
			HasProjection: m.HasProjection,
		}
	}

	return out, nil
}

// =============================================================================

func (a *app) listPeerModels(ctx context.Context, r *http.Request) web.Encoder {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		return errs.Errorf(errs.InvalidArgument, "host is required")
	}

	models, err := fetchPeerModels(ctx, host)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return PeerModelListResponse{Models: models}
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
