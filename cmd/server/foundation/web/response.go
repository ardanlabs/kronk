package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
)

// NoResponse tells the Respond function to not respond to the request. In these
// cases the app layer code has already done so. If err is non-nil, the response
// was committed before the request failed and middleware can still observe the
// application outcome without attempting a second response.
type NoResponse struct {
	err error
}

// NewNoResponse constructs a no response value.
func NewNoResponse() NoResponse {
	return NoResponse{}
}

// NewNoResponseError constructs a no response value that preserves an error
// that occurred after the response was committed.
func NewNoResponseError(err error) NoResponse {
	return NoResponse{err: err}
}

// Encode implements the Encoder interface.
func (NoResponse) Encode() ([]byte, string, error) {
	return nil, "", nil
}

// ResponseError returns an error that occurred after the response was
// committed. Middleware uses this for logging, metrics, and tracing.
func (nr NoResponse) ResponseError() error {
	return nr.err
}

// =============================================================================

type httpStatus interface {
	HTTPStatus() int
}

// Respond sends a response to the client.
func Respond(ctx context.Context, w http.ResponseWriter, resp Encoder) error {
	if _, ok := resp.(NoResponse); ok {
		return nil
	}

	// If the context has been canceled, it means the client is no longer
	// waiting for a response.
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("client disconnected, do not send response")
		}
	}

	statusCode := http.StatusOK

	switch v := resp.(type) {
	case httpStatus:
		statusCode = v.HTTPStatus()

	case error:
		statusCode = http.StatusInternalServerError

	default:
		if resp == nil {
			statusCode = http.StatusNoContent
		}
	}

	_, span := addSpan(ctx, "web.send.response", attribute.Int("status", statusCode))
	defer span.End()

	if statusCode == http.StatusNoContent {
		w.WriteHeader(statusCode)
		return nil
	}

	data, contentType, err := resp.Encode()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return fmt.Errorf("respond: encode: %w", err)
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(statusCode)

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("respond: write: %w", err)
	}

	return nil
}
