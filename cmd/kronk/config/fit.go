package config

import (
	"fmt"
	"strconv"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// When the memory check says the answers don't fit, the review offers fixes:
// a smaller context, a q8_0 KV cache, fewer requests at the same time, or
// keep. Each fix is checked with the same analysis on the same model and
// hardware, and only offered when it then fits.

// Fix kinds.
const (
	fixSmallerContext = "context"
	fixQ8Cache        = "cache"
	fixFewerRequests  = "requests"
	fixQ8AndContext   = "cache+context"
)

// fix is one offered change and the memory it needs.
type fix struct {
	kind     string
	context  int
	requests int
	mem      int64
}

func (x fix) label() string {
	switch x.kind {
	case fixSmallerContext:
		return fmt.Sprintf("Use a smaller context: %d (fits, %s)", x.context, formatBytes(uint64(x.mem)))
	case fixQ8Cache:
		return fmt.Sprintf("Use a q8_0 KV cache (fits, %s)", formatBytes(uint64(x.mem)))
	case fixFewerRequests:
		return fmt.Sprintf("Run fewer requests at the same time: %d (fits, %s)", x.requests, formatBytes(uint64(x.mem)))
	case fixQ8AndContext:
		return fmt.Sprintf("Use a q8_0 KV cache and a %d context (fits, %s)", x.context, formatBytes(uint64(x.mem)))
	}
	return x.kind
}

// contextSizes are the sizes AutoTune picks from, largest first.
var contextSizes = []int{131072, 65536, 32768, 16384, 8192, 4096, 2048}

// fullPrecision are the KV cache types a q8_0 cache makes smaller.
var fullPrecision = map[string]bool{"f16": true, "bf16": true, "f32": true}

// fitFixes returns the fixes that make cfg fit, given the analysis of cfg.
// It returns none when cfg fits.
func fitFixes(info models.ModelInfo, devs devices.Devices, cfg models.ModelConfig, an models.Analysis) []fix {
	r := an.Recommended
	if r.Fits {
		return nil
	}

	try := func(c models.ModelConfig) (int64, bool) {
		got, err := models.AnalyzeWithConfig(info, devs, c)
		if err != nil {
			return 0, false
		}
		return got.Recommended.EstimatedVRAMBytes, got.Recommended.Fits
	}

	withContext := func(c models.ModelConfig, n int) models.ModelConfig {
		c.PtrContextWindow = &n
		return c
	}
	withCache := func(c models.ModelConfig) models.ModelConfig {
		c.CacheTypeK, c.CacheTypeV = model.GGMLTypeQ8_0, model.GGMLTypeQ8_0
		return c
	}

	// largestContext is the largest size below the current context that
	// fits with c, 0 when none does.
	largestContext := func(c models.ModelConfig) (int, int64) {
		for _, n := range contextSizes {
			if int64(n) >= r.ContextWindow {
				continue
			}
			if mem, ok := try(withContext(c, n)); ok {
				return n, mem
			}
		}
		return 0, 0
	}

	var out []fix

	if n, mem := largestContext(cfg); n > 0 {
		out = append(out, fix{kind: fixSmallerContext, context: n, mem: mem})
	}

	cacheFits := false
	if fullPrecision[r.CacheTypeK] {
		if mem, ok := try(withCache(cfg)); ok {
			cacheFits = true
			out = append(out, fix{kind: fixQ8Cache, mem: mem})
		}
	}

	// Classic speculation needs one request; the tool already sets it.
	classic := cfg.Speculation.String() == "classic"
	if r.NSeqMax > 1 && !classic {
		for n := int(r.NSeqMax) - 1; n >= 1; n-- {
			c := cfg
			c.PtrNSeqMax = &n
			if mem, ok := try(c); ok {
				out = append(out, fix{kind: fixFewerRequests, requests: n, mem: mem})
				break
			}
		}
	}

	if !cacheFits && fullPrecision[r.CacheTypeK] && len(out) == 0 {
		if n, mem := largestContext(withCache(cfg)); n > 0 {
			out = append(out, fix{kind: fixQ8AndContext, context: n, mem: mem})
		}
	}

	return out
}

// applyFix changes the answers the way the fix says, so the questions and
// the review show the new values.
func (a *answers) applyFix(f facts, x fix) {
	if x.kind == fixQ8Cache || x.kind == fixQ8AndContext {
		*a.sel["cache-type-k"] = "q8_0"
		*a.sel["cache-type-v"] = "" // same as keys
	}
	if x.kind == fixSmallerContext || x.kind == fixQ8AndContext {
		a.set(f, "context-window", strconv.Itoa(x.context))
	}
	if x.kind == fixFewerRequests {
		a.set(f, "nseq-max", strconv.Itoa(x.requests))
	}
}

// set answers key with value: the matching option when there is one,
// otherwise "Other value…" with the value typed in.
func (a *answers) set(f facts, key string, value string) {
	q := a.byKey[key]
	if q.options != nil {
		for _, o := range q.options(f) {
			if o.value == value {
				*a.sel[key] = value
				return
			}
		}
		*a.sel[key] = otherValue
		*a.other[key] = value
		return
	}
	*a.sel[key] = value
}
