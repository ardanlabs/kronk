package toolapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

func (a *app) listDevices(ctx context.Context, r *http.Request) web.Encoder {
	return DevicesResponse(devices.List())
}
