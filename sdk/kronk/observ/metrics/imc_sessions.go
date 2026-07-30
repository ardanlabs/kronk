package metrics

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// IMCSession describes the current scalar values for one bounded IMC cache
// entry.
type IMCSession struct {
	ModelID   string
	Entry     int
	State     string
	Messages  int
	Context   int
	Allocated int
	Window    int
	HasMedia  bool
	LastUsed  time.Time
}

// IMCSessionsProvider returns the current bounded IMC cache entries.
type IMCSessionsProvider func() []IMCSession

type imcSessionsCollector struct {
	mu         sync.RWMutex
	provider   IMCSessionsProvider
	generation uint64

	state       *prometheus.Desc
	messages    *prometheus.Desc
	context     *prometheus.Desc
	allocated   *prometheus.Desc
	window      *prometheus.Desc
	usedPercent *prometheus.Desc
	hasMedia    *prometheus.Desc
	lastUsed    *prometheus.Desc
}

func newIMCSessionsCollector() *imcSessionsCollector {
	labels := []string{"model_id", "entry"}

	c := imcSessionsCollector{
		state: prometheus.NewDesc(
			"imc_session_state",
			"Current IMC cache entry state; value is 1 for the state in the state label.",
			append(labels, "state"),
			nil,
		),
		messages: prometheus.NewDesc(
			"imc_session_messages",
			"Number of messages represented by the current IMC cache entry snapshot.",
			labels,
			nil,
		),
		context: prometheus.NewDesc(
			"imc_session_context_tokens",
			"Current physical context represented by the IMC cache entry, in tokens.",
			labels,
			nil,
		),
		allocated: prometheus.NewDesc(
			"imc_session_allocated_tokens",
			"Largest physical context retained by the IMC cache entry, in tokens.",
			labels,
			nil,
		),
		window: prometheus.NewDesc(
			"imc_session_window_tokens",
			"Configured context window for the IMC cache entry, in tokens.",
			labels,
			nil,
		),
		usedPercent: prometheus.NewDesc(
			"imc_session_used_percent",
			"Percentage of the configured context window used by the IMC cache entry.",
			labels,
			nil,
		),
		hasMedia: prometheus.NewDesc(
			"imc_session_has_media",
			"Whether the IMC cache entry contains media; 1 for yes and 0 for no.",
			labels,
			nil,
		),
		lastUsed: prometheus.NewDesc(
			"imc_session_last_used_timestamp_seconds",
			"Unix timestamp when the IMC cache entry was last used; 0 if never used.",
			labels,
			nil,
		),
	}

	return &c
}

func (c *imcSessionsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.state
	ch <- c.messages
	ch <- c.context
	ch <- c.allocated
	ch <- c.window
	ch <- c.usedPercent
	ch <- c.hasMedia
	ch <- c.lastUsed
}

func (c *imcSessionsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	provider := c.provider
	c.mu.RUnlock()

	if provider == nil {
		return
	}

	for _, session := range provider() {
		modelID := normalizeModelID(session.ModelID)
		entry := strconv.Itoa(session.Entry)

		c.collect(ch, c.state, 1, modelID, entry, session.State)
		c.collect(ch, c.messages, float64(session.Messages), modelID, entry)
		c.collect(ch, c.context, float64(session.Context), modelID, entry)
		c.collect(ch, c.allocated, float64(session.Allocated), modelID, entry)
		c.collect(ch, c.window, float64(session.Window), modelID, entry)

		usedPercent := 0.0
		if session.Window > 0 {
			usedPercent = float64(session.Context) / float64(session.Window) * 100
		}
		c.collect(ch, c.usedPercent, usedPercent, modelID, entry)

		hasMedia := 0.0
		if session.HasMedia {
			hasMedia = 1
		}
		c.collect(ch, c.hasMedia, hasMedia, modelID, entry)

		lastUsed := 0.0
		if !session.LastUsed.IsZero() {
			lastUsed = float64(session.LastUsed.UnixNano()) / float64(time.Second)
		}
		c.collect(ch, c.lastUsed, lastUsed, modelID, entry)
	}
}

func (c *imcSessionsCollector) collect(ch chan<- prometheus.Metric, desc *prometheus.Desc, value float64, labels ...string) {
	metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, labels...)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(desc, err)
		return
	}

	ch <- metric
}

func (c *imcSessionsCollector) register(provider IMCSessionsProvider) (func(), error) {
	if provider == nil {
		return nil, errors.New("register: provider is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.provider != nil {
		return nil, errors.New("register: IMC sessions provider already registered")
	}

	c.generation++
	generation := c.generation
	c.provider = provider

	var once sync.Once
	unregister := func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()

			if c.generation == generation {
				c.provider = nil
			}
		})
	}

	return unregister, nil
}
