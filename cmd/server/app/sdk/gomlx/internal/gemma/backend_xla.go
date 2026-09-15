//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && arm64) || (windows && amd64))

package gemma

import _ "github.com/gomlx/go-xla/compute/xla"
