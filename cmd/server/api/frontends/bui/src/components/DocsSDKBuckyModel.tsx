import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsSDKBuckyModel() {
  const location = useLocation();

  useEffect(() => {
    const container = document.querySelector('.main-content');
    if (!container) return;
    if (!location.hash) {
      container.scrollTo({ top: 0 });
      return;
    }
    const id = location.hash.slice(1);
    requestAnimationFrame(() => {
      const element = document.getElementById(id);
      if (!element) return;
      const containerRect = container.getBoundingClientRect();
      const elementRect = element.getBoundingClientRect();
      const offset = elementRect.top - containerRect.top + container.scrollTop;
      container.scrollTo({ top: offset - 20, behavior: 'smooth' });
    });
  }, [location.key, location.hash]);

  return (
    <div>
      <div className="page-header">
        <h2>BuckyModel Package</h2>
        <p>Package model provides the low-level API for working with whisper.cpp models via the github.com/ardanlabs/bucky FFI bindings. It owns the whisper.Context lifecycle, parameter translation, and the transcribe / language-detect primitives the high-level sdk/bucky package layers concurrency on top of.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/bucky/model"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-decode">
              <h4>Decode</h4>
              <pre className="code-block">
                <code>func Decode(ctx context.Context, r io.Reader) ([]float32, error)</code>
              </pre>
              <p className="doc-description">Decode reads audio in any format the bucky SDK supports and returns the 16 kHz mono float32 PCM that Transcribe expects. WAV, MP3, and FLAC are decoded in-process by the upstream github.com/ardanlabs/bucky/pkg/audio package. Anything else (WebM / Opus, MP4 / AAC, OGG, M4A, ...) is transcoded to WAV by shelling out to ffmpeg via sdk/bucky/ffmpeg. ffmpeg is located once on first use and reused for the lifetime of the process. When ffmpeg is not installed or the transcode fails, Decode returns an error that wraps audio.ErrUnsupportedFormat so callers that already match the upstream sentinel keep working and the user-visible error category remains "unsupported format".</p>
            </div>

            <div className="doc-section" id="func-decodechannels">
              <h4>DecodeChannels</h4>
              <pre className="code-block">
                <code>func DecodeChannels(ctx context.Context, r io.Reader) ([][]float32, error)</code>
              </pre>
              <p className="doc-description">DecodeChannels reads audio in any format the bucky SDK supports and returns one 16 kHz mono float32 PCM slice per source channel, ready for channel-separated (per-speaker) transcription. Mono input yields a one-element result. Native formats (WAV, FLAC, MP3) are decoded in-process via audio.DecodeRaw, which preserves the source channel layout: each channel is de-interleaved with audio.SplitChannels and resampled to 16 kHz with audio.ResampleLinear. Anything that requires ffmpeg (WebM / Opus, MP4 / AAC, OGG, M4A, ...) is downmixed to a single mono channel by the transcode, so such inputs also return a one-element result.</p>
            </div>

            <div className="doc-section" id="func-langid">
              <h4>LangID</h4>
              <pre className="code-block">
                <code>func LangID(lang string) int32</code>
              </pre>
              <p className="doc-description">LangID returns the whisper.cpp internal id for the supplied language code (e.g. "de" → 2). Returns -1 if the code is unknown. The bucky Init function must have been called before LangID, since the underlying FFI symbol is resolved by whisper.Load.</p>
            </div>

            <div className="doc-section" id="func-langmaxid">
              <h4>LangMaxID</h4>
              <pre className="code-block">
                <code>func LangMaxID() int32</code>
              </pre>
              <p className="doc-description">LangMaxID returns the largest language id whisper.cpp knows. The number of supported languages is LangMaxID()+1.</p>
            </div>

            <div className="doc-section" id="func-langstr">
              <h4>LangStr</h4>
              <pre className="code-block">
                <code>func LangStr(id int32) string</code>
              </pre>
              <p className="doc-description">LangStr returns the short language code for the supplied id (e.g. 2 → "de"). Returns "" if the id is invalid.</p>
            </div>

            <div className="doc-section" id="func-systeminfo">
              <h4>SystemInfo</h4>
              <pre className="code-block">
                <code>func SystemInfo() map[string]string</code>
              </pre>
              <p className="doc-description">SystemInfo returns the whisper.cpp system info string parsed into a key/value map for observability output. The format mirrors sdk/kronk's SystemInfo.</p>
            </div>

            <div className="doc-section" id="func-newmodel">
              <h4>NewModel</h4>
              <pre className="code-block">
                <code>func NewModel(ctx context.Context, cfg Config) (*Model, error)</code>
              </pre>
              <p className="doc-description">NewModel constructs a Model from cfg. ModelPath must be set; all other fields fall back to the defaults defined by Config.WithDefaults.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-audioformat">
              <h4>AudioFormat</h4>
              <pre className="code-block">
                <code>{`type AudioFormat struct {
	// SampleRate is the producer's sample rate in Hz, e.g. 48000. When
	// it differs from 16000 the input is resampled.
	SampleRate int

	// Channels is the number of interleaved channels, e.g. 2 for
	// stereo (L,R,L,R,...). More than one channel is downmixed to mono.
	Channels int

	// Sample is the byte layout of each sample.
	Sample SampleEncoding
}`}</code>
              </pre>
              <p className="doc-description">AudioFormat describes the raw PCM a producer delivers to FeedPCM. The engine always converts to its fixed internal format: 16 kHz, mono, float32 in [-1, 1].</p>
            </div>

            <div className="doc-section" id="type-channeltranscription">
              <h4>ChannelTranscription</h4>
              <pre className="code-block">
                <code>{`type ChannelTranscription struct {
	Channel int
	Transcription
}`}</code>
              </pre>
              <p className="doc-description">ChannelTranscription is the transcription of a single source channel in a channel-separated (diarized) result. Channel is the zero-based source channel index (0 = left, 1 = right for stereo).</p>
            </div>

            <div className="doc-section" id="type-config">
              <h4>Config</h4>
              <pre className="code-block">
                <code>{`type Config struct {
	// ModelPath is the absolute path to the GGML whisper model file
	// the handle will load via whisper.InitFromFileWithParams.
	ModelPath string

	// UseGPU enables GPU offload (Metal on darwin, CUDA / Vulkan on
	// linux+windows when libwhisper was built with the relevant
	// backend). Defaults to whisper.cpp's own default (true).
	UseGPU bool

	// FlashAttn enables the flash-attention kernel when supported by
	// the active backend. Defaults to false.
	FlashAttn bool

	// GPUDevice selects which GPU the model is offloaded to when
	// multiple devices are present. Defaults to 0.
	GPUDevice int32

	// NThreads is the default thread count attached to every
	// Transcribe call when no per-call override is supplied. A zero
	// value means whisper.cpp's own default (typically min(4, ncpu)).
	NThreads int32

	// NSeqMax sizes the model's internal whisper.State pool. Each
	// pooled state owns its own mel spectrogram, KV cache, and
	// compute buffer, so NSeqMax goroutines can run concurrent
	// transcribe / language-detect calls against the same Model.
	// Values <= 0 collapse to 1.
	NSeqMax int

	// Log is the logger the model uses for diagnostic output.
	// Defaults to applog.DiscardLogger when nil.
	Log applog.Logger
}`}</code>
              </pre>
              <p className="doc-description">Config carries the per-model whisper.cpp configuration. Fields are resolved through the functional Option pattern (NewConfig + WithX) at construction time and treated as read-only thereafter. ModelPath is required. The remaining fields all have sensible zero defaults that match whisper_context_default_params and the per-handle backpressure conventions used by sdk/kronk.</p>
            </div>

            <div className="doc-section" id="type-diarization">
              <h4>Diarization</h4>
              <pre className="code-block">
                <code>{`type Diarization struct {
	Channels []ChannelTranscription
	Segments []DiarizedSegment
}`}</code>
              </pre>
              <p className="doc-description">Diarization is the result of a channel-separated transcribe. Channels holds one transcription per non-empty source channel; Segments merges every channel's segments into a single list sorted by start time, each tagged with the channel (speaker) it came from.</p>
            </div>

            <div className="doc-section" id="type-diarizedsegment">
              <h4>DiarizedSegment</h4>
              <pre className="code-block">
                <code>{`type DiarizedSegment struct {
	Channel int
	Segment
}`}</code>
              </pre>
              <p className="doc-description">DiarizedSegment is one decoded segment tagged with the source channel it came from, so a merged multi-channel transcript can attribute each segment to a speaker.</p>
            </div>

            <div className="doc-section" id="type-event">
              <h4>Event</h4>
              <pre className="code-block">
                <code>{`type Event struct {
	Kind     EventKind
	Text     string
	StartMs  int64 // session-local; rebases to 0 after Reset by default
	EndMs    int64
	Segments []Segment
	Err      error // non-nil only when Kind == EventError
}`}</code>
              </pre>
              <p className="doc-description">Event is one item delivered on a Stream's Events channel. Text semantics depend on Kind (see EventPartial / EventFinal): a Partial carries the full replaceable hypothesis for the un-committed window, a Final carries the committed text to append. Consumers render live by replacing on Partial and committing on Final.</p>
            </div>

            <div className="doc-section" id="type-eventkind">
              <h4>EventKind</h4>
              <pre className="code-block">
                <code>{`type EventKind int`}</code>
              </pre>
              <p className="doc-description">EventKind classifies a transcript Event delivered on a Stream's channel.</p>
            </div>

            <div className="doc-section" id="type-model">
              <h4>Model</h4>
              <pre className="code-block">
                <code>{`type Model struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Model owns a single whisper.Context (the model weights) plus an internal statePool that allocates Config.NSeqMax whisper.State instances against that context. Each state carries its own mel spectrogram, KV cache, and compute buffer, so concurrent transcribe / language-detect calls can run in parallel against one set of shared weights. This mirrors how sdk/kronk/model handles embedding and rerank concurrency: one llama.Model + NSeqMax llama.Context instances behind a small pool.</p>
            </div>

            <div className="doc-section" id="type-modelinfo">
              <h4>ModelInfo</h4>
              <pre className="code-block">
                <code>{`type ModelInfo struct {
	ID             string
	Type           string
	IsMultilingual bool
	NVocab         int32
	NTextCtx       int32
	NAudioCtx      int32
	NMels          int32
}`}</code>
              </pre>
              <p className="doc-description">ModelInfo summarizes the static properties of a loaded whisper model. It is populated from whisper.Context accessor calls at construction time and never mutated thereafter.</p>
            </div>

            <div className="doc-section" id="type-option">
              <h4>Option</h4>
              <pre className="code-block">
                <code>{`type Option func(*Config)`}</code>
              </pre>
              <p className="doc-description">Option represents a functional option for configuring a Config.</p>
            </div>

            <div className="doc-section" id="type-resetconfig">
              <h4>ResetConfig</h4>
              <pre className="code-block">
                <code>{`type ResetConfig struct {
	// FlushPending, when true (default), runs one final transcribe over
	// any audio still in the buffer and emits the resulting Final
	// event(s) before clearing.
	FlushPending bool

	// RebaseTimestamps, when true (default), restarts StartMs/EndMs from
	// zero on subsequent events.
	RebaseTimestamps bool

	// KeepPromptTokens, when true, preserves the rolling prompt-token
	// history across the reset so the next window keeps linguistic
	// continuity ("rewind the audio buffer but keep context"). Default
	// false, which treats Reset as a hard session boundary (e.g. a topic
	// change) and clears the context. This is a no-op when the session
	// was created with prompt carryover disabled
	// (StreamConfig.DisablePromptCarryover): that global setting always
	// wins, so every window decodes independently regardless of this flag.
	KeepPromptTokens bool
}`}</code>
              </pre>
              <p className="doc-description">ResetConfig tunes Reset behavior. All fields are optional and have the defaults documented per field.</p>
            </div>

            <div className="doc-section" id="type-resetoption">
              <h4>ResetOption</h4>
              <pre className="code-block">
                <code>{`type ResetOption func(*ResetConfig)`}</code>
              </pre>
              <p className="doc-description">ResetOption is a functional option for ResetConfig.</p>
            </div>

            <div className="doc-section" id="type-sampleencoding">
              <h4>SampleEncoding</h4>
              <pre className="code-block">
                <code>{`type SampleEncoding int`}</code>
              </pre>
              <p className="doc-description">SampleEncoding identifies how raw PCM samples are laid out in bytes. Only little-endian encodings are listed because every mainstream capture stack (CoreAudio, WASAPI, ALSA/PulseAudio/PipeWire, Web Audio) emits little-endian on x86 and ARM.</p>
            </div>

            <div className="doc-section" id="type-segment">
              <h4>Segment</h4>
              <pre className="code-block">
                <code>{`type Segment struct {
	Index        int32
	StartMs      int64
	EndMs        int64
	Text         string
	NoSpeechProb float32
}`}</code>
              </pre>
              <p className="doc-description">Segment is one decoded segment from a Transcribe call.</p>
            </div>

            <div className="doc-section" id="type-stream">
              <h4>Stream</h4>
              <pre className="code-block">
                <code>{`type Stream struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Stream is a long-lived transcription session. It borrows one whisper.State from the model's pool for its entire lifetime and emits transcript Events incrementally as audio is fed in. A Stream is reusable indefinitely via Reset. Close must be called exactly once when done. Feed/FeedPCM are producer-side (single goroutine). Consumers must range over Events() until it is closed. Reset and Close are safe to call from any goroutine; both serialize with the worker.</p>
            </div>

            <div className="doc-section" id="type-streamconfig">
              <h4>StreamConfig</h4>
              <pre className="code-block">
                <code>{`type StreamConfig struct {
	// Language is the BCP-47 / ISO 639-1 language hint. When empty
	// whisper.cpp auto-detects once at the start of the session.
	Language string

	// InitialPrompt biases the decoder on the first window only.
	InitialPrompt string

	// Translate, when true, translates the source audio to English.
	Translate bool

	// NThreads overrides Config.NThreads for this session when > 0.
	NThreads int32

	// PartialEveryMs is the partial-emit cadence. 0 = 1000;
	// <0 disables partials (final-only mode).
	PartialEveryMs int

	// KeepMs is the trailing audio kept across commits so a word on the
	// boundary is not clipped. 0 = 300.
	KeepMs int

	// CommitEveryMs force-commits a Final on a fixed cadence even when no
	// silence is detected (e.g. continuous speech, or VAD disabled), so
	// Finals roll out steadily instead of waiting for MaxUtteranceMs.
	// Mirrors whisper.cpp stream's n_new_line cadence. 0 = 6000.
	CommitEveryMs int

	// MaxUtteranceMs force-flushes a Final if no silence is detected.
	// 0 = 25000.
	MaxUtteranceMs int

	// DisableVAD turns OFF the silence-gated commit. By default (zero
	// value) the stream uses a pure-Go energy-ratio voice-activity
	// detector (the same approach as whisper.cpp stream's vad_simple — no
	// model file, no extra inference) to commit a Final when the speaker
	// pauses, so cuts land in natural gaps instead of mid-word. Set it
	// true (via WithVAD(false)) to fall back to fixed-cadence commits on
	// CommitEveryMs / MaxUtteranceMs / Close only. The field is named for
	// the non-default state, matching the http.Transport.DisableKeepAlives
	// idiom, so the default-on behavior needs no sentinel.
	DisableVAD bool

	// VADThreshold is the energy-ratio threshold for the silence detector:
	// the trailing window is treated as silence when its mean energy falls
	// below VADThreshold * the whole window's mean energy. Lower = less
	// eager to cut. 0 = 0.6.
	VADThreshold float32

	// DisablePromptCarryover turns OFF the cross-window prompt-token
	// continuity. By default (zero value) each committed window's tail
	// tokens are harvested and seeded into the next window's decode, the
	// manual equivalent of whisper.cpp's condition_on_previous_text, so the
	// decoder keeps linguistic context across commits. The cost is that a
	// strong prior (e.g. a committed question) can condition a near-silent
	// trailing window into a plausible but hallucinated continuation. Set
	// it true (via WithPromptCarryover(false)) to decode every window
	// independently. The field is named for the non-default state, matching
	// the DisableVAD idiom, so the default-on behavior needs no sentinel.
	// InitialPrompt is unaffected; it still biases the first window.
	DisablePromptCarryover bool

	// NoSpeechThreshold overrides whisper.cpp's no-speech probability
	// threshold for every decode in the session when > 0 (library default
	// 0.6). A zero (or negative) value is the "unset" sentinel and leaves
	// the default in place; see TranscribeConfig.NoSpeechThreshold for why
	// >0 is a sufficient sentinel.
	NoSpeechThreshold float32

	// LogProbThreshold overrides whisper.cpp's average log-probability
	// acceptance threshold for every decode in the session when non-nil
	// (library default -1.0). nil leaves the default in place; it is a
	// pointer rather than a sentinel because 0 is a legitimate threshold
	// the caller must be able to set.
	LogProbThreshold *float32

	// EmitResetEvent, when true, emits an EventReset after Reset.
	// Default false. Useful when a different goroutine consumes Events
	// and keeps partial-text accumulators it must clear at a boundary.
	EmitResetEvent bool
}`}</code>
              </pre>
              <p className="doc-description">StreamConfig captures the per-stream settings NewStream consults. Defaults are applied for any zero-value field (see the per-field comments).</p>
            </div>

            <div className="doc-section" id="type-streamoption">
              <h4>StreamOption</h4>
              <pre className="code-block">
                <code>{`type StreamOption func(*StreamConfig)`}</code>
              </pre>
              <p className="doc-description">StreamOption is a functional option for StreamConfig.</p>
            </div>

            <div className="doc-section" id="type-transcribeconfig">
              <h4>TranscribeConfig</h4>
              <pre className="code-block">
                <code>{`type TranscribeConfig struct {
	// Language is the BCP-47 / ISO 639-1 language hint (e.g. "en",
	// "de"). When empty whisper.cpp auto-detects.
	Language string

	// InitialPrompt biases the decoder with prior context.
	InitialPrompt string

	// PromptTokens seeds the decoder with token ids harvested from a
	// prior decode, carrying linguistic context across windows. Used by
	// the streaming worker for cross-window continuity; empty for batch
	// transcription.
	PromptTokens []whisper.Token

	// Translate, when true, translates the source audio to English.
	Translate bool

	// NThreads overrides Config.NThreads for this call when > 0.
	NThreads int32

	// NoSpeechThreshold overrides whisper.cpp's no-speech probability
	// threshold when > 0 (the library default is 0.6). A segment whose
	// no-speech probability exceeds this is treated as silence during
	// the decode's temperature-fallback decision, reducing the chance a
	// near-silent window is decoded into hallucinated text. The value is
	// a probability in (0, 1]; a zero (or negative) value is the "unset"
	// sentinel and leaves the library default in place. A >0 sentinel is
	// sufficient here because 0 is not a useful threshold (it would flag
	// nearly every segment as silence), so it need not be expressible.
	NoSpeechThreshold float32

	// LogProbThreshold overrides whisper.cpp's average log-probability
	// acceptance threshold when non-nil (the library default is -1.0).
	// Decodes whose average token log-probability falls below this are
	// rejected and retried at a higher temperature. nil leaves the
	// library default in place. Unlike NoSpeechThreshold this is a
	// pointer rather than a >0/!=0 sentinel because 0 is a legitimate
	// (maximally strict) threshold the caller must be able to set.
	LogProbThreshold *float32

	// BeamSize, when > 0, switches the sampler to beam search with
	// the specified beam size. Defaults to greedy.
	BeamSize int32

	// NoTimestamps suppresses per-segment t0/t1 emission in the
	// rendered text output. Segment-level timestamps remain available
	// on each Segment value.
	NoTimestamps bool

	// OnSegment, when non-nil, is invoked once per decoded segment
	// after Full returns. The callback is synchronous and runs on the
	// caller's goroutine.
	OnSegment func(Segment)
}`}</code>
              </pre>
              <p className="doc-description">TranscribeConfig captures the per-call settings Transcribe consults. Defaults match the whisper.cpp greedy-sampling profile with progress / realtime printing disabled.</p>
            </div>

            <div className="doc-section" id="type-transcribeoption">
              <h4>TranscribeOption</h4>
              <pre className="code-block">
                <code>{`type TranscribeOption func(*TranscribeConfig)`}</code>
              </pre>
              <p className="doc-description">TranscribeOption is a functional option for TranscribeConfig.</p>
            </div>

            <div className="doc-section" id="type-transcription">
              <h4>Transcription</h4>
              <pre className="code-block">
                <code>{`type Transcription struct {
	Text     string
	Language string
	Duration float64
	Segments []Segment
}`}</code>
              </pre>
              <p className="doc-description">Transcription is the full result of a Transcribe call. Text is the concatenation of Segment.Text trimmed of leading and trailing whitespace. Language is the language code whisper.cpp detected (or the hint that was passed in). Duration is the length of the transcribed audio in seconds.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-config-withdefaults">
              <h4>Config.WithDefaults</h4>
              <pre className="code-block">
                <code>func (cfg Config) WithDefaults() Config</code>
              </pre>
              <p className="doc-description">WithDefaults returns cfg with the zero-valued fields filled in.</p>
            </div>

            <div className="doc-section" id="method-model-config">
              <h4>Model.Config</h4>
              <pre className="code-block">
                <code>func (m *Model) Config() Config</code>
              </pre>
              <p className="doc-description">Config returns the resolved Config the Model was built with (defaults applied).</p>
            </div>

            <div className="doc-section" id="method-model-detectlanguage">
              <h4>Model.DetectLanguage</h4>
              <pre className="code-block">
                <code>func (m *Model) DetectLanguage(ctx context.Context, samples []float32, withProbs bool) (string, []float32, error)</code>
              </pre>
              <p className="doc-description">DetectLanguage runs a short whisper pass on the supplied 16 kHz mono float32 PCM samples and returns the detected language code along with the per-language probability vector (length LangMaxID()+1) when withProbs is true. DetectLanguage acquires a whisper.State from the model's internal pool, so up to Config.NSeqMax goroutines may run DetectLanguage in parallel against the same Model.</p>
            </div>

            <div className="doc-section" id="method-model-modelinfo">
              <h4>Model.ModelInfo</h4>
              <pre className="code-block">
                <code>func (m *Model) ModelInfo() ModelInfo</code>
              </pre>
              <p className="doc-description">ModelInfo returns the static information about the loaded model.</p>
            </div>

            <div className="doc-section" id="method-model-newstream">
              <h4>Model.NewStream</h4>
              <pre className="code-block">
                <code>func (m *Model) NewStream(ctx context.Context, onClose func(), opts ...StreamOption) (*Stream, error)</code>
              </pre>
              <p className="doc-description">NewStream opens a streaming transcription session against the loaded model. It reserves one whisper.State from the pool for the lifetime of the stream; onClose, when non-nil, runs when the stream's worker exits (used by the bucky wrapper to release its backpressure slot). Caller must call Close exactly once.</p>
            </div>

            <div className="doc-section" id="method-model-transcribe">
              <h4>Model.Transcribe</h4>
              <pre className="code-block">
                <code>func (m *Model) Transcribe(ctx context.Context, samples []float32, opts ...TranscribeOption) (Transcription, error)</code>
              </pre>
              <p className="doc-description">Transcribe runs the whisper.cpp pipeline on the provided 16 kHz mono float32 PCM samples and returns the decoded text along with per-segment metadata. Transcribe acquires a whisper.State from the model's internal pool, so up to Config.NSeqMax goroutines may run Transcribe in parallel against the same Model. The acquired state is released back to the pool when Transcribe returns.</p>
            </div>

            <div className="doc-section" id="method-model-transcribechannels">
              <h4>Model.TranscribeChannels</h4>
              <pre className="code-block">
                <code>func (m *Model) TranscribeChannels(ctx context.Context, channels [][]float32, opts ...TranscribeOption) (Diarization, error)</code>
              </pre>
              <p className="doc-description">TranscribeChannels transcribes each supplied channel of 16 kHz mono float32 PCM separately and merges the results into a diarized transcript. Each channel is treated as one speaker, the common layout for call-center and meeting recordings where every participant has a dedicated channel. Pass the slices returned by DecodeChannels. Channels are transcribed sequentially. Each channel acquires and releases a whisper.State from the model's internal pool, so other goroutines may still run Transcribe against the same Model between channels. Empty channels are skipped.</p>
            </div>

            <div className="doc-section" id="method-model-transcribechannelsfile">
              <h4>Model.TranscribeChannelsFile</h4>
              <pre className="code-block">
                <code>func (m *Model) TranscribeChannelsFile(ctx context.Context, r io.Reader, opts ...TranscribeOption) (Diarization, error)</code>
              </pre>
              <p className="doc-description">TranscribeChannelsFile decodes audio from r into one 16 kHz mono float32 channel per source channel (via DecodeChannels) and then runs TranscribeChannels. Native multi-channel formats (WAV, FLAC) yield true per-channel diarization; formats that require ffmpeg are downmixed to a single channel before transcription.</p>
            </div>

            <div className="doc-section" id="method-model-transcribefile">
              <h4>Model.TranscribeFile</h4>
              <pre className="code-block">
                <code>func (m *Model) TranscribeFile(ctx context.Context, r io.Reader, opts ...TranscribeOption) (Transcription, error)</code>
              </pre>
              <p className="doc-description">TranscribeFile is a convenience wrapper that decodes audio from r via Decode and then runs Transcribe on the resulting samples. It is intended for HTTP handlers and CLI callers that have an io.Reader (form upload, file on disk) rather than pre-decoded PCM.</p>
            </div>

            <div className="doc-section" id="method-model-unload">
              <h4>Model.Unload</h4>
              <pre className="code-block">
                <code>func (m *Model) Unload(ctx context.Context) error</code>
              </pre>
              <p className="doc-description">Unload releases the state pool followed by the underlying whisper context. Unload is single-use per Model; subsequent calls return an error. The supplied ctx is accepted for parity with sdk/kronk.Model.Unload — whisper has no in-flight requests to drain at this layer because concurrency is owned by the sdk/bucky wrapper.</p>
            </div>

            <div className="doc-section" id="method-stream-close">
              <h4>Stream.Close</h4>
              <pre className="code-block">
                <code>func (s *Stream) Close() error</code>
              </pre>
              <p className="doc-description">Close performs one final flush over remaining audio, emits the resulting Final event, closes Events, and returns the whisper.State to the pool. It is idempotent and blocks until the worker has exited.</p>
            </div>

            <div className="doc-section" id="method-stream-events">
              <h4>Stream.Events</h4>
              <pre className="code-block">
                <code>func (s *Stream) Events() &lt;-chan Event</code>
              </pre>
              <p className="doc-description">Events returns the channel of transcript events. It is closed after Close finishes its final flush, or after an EventError. Range over it.</p>
            </div>

            <div className="doc-section" id="method-stream-feed">
              <h4>Stream.Feed</h4>
              <pre className="code-block">
                <code>func (s *Stream) Feed(ctx context.Context, samples []float32) error</code>
              </pre>
              <p className="doc-description">Feed pushes 16 kHz mono float32 PCM into the stream. It blocks, respecting ctx, when the internal buffer is full, back-pressuring a fast producer. Returns ctx.Err() on cancellation. The samples slice may be reused by the caller after Feed returns. This is the zero-conversion fast path: callers that already hold normalized 16 kHz mono float32 (browser Web Audio, CoreAudio) feed it directly. Callers with raw microphone PCM should use FeedPCM.</p>
            </div>

            <div className="doc-section" id="method-stream-feedpcm">
              <h4>Stream.FeedPCM</h4>
              <pre className="code-block">
                <code>func (s *Stream) FeedPCM(ctx context.Context, raw []byte, f AudioFormat) error</code>
              </pre>
              <p className="doc-description">FeedPCM is the raw-microphone adapter for Feed. A capture device never hands you 16 kHz mono float32; it hands you raw interleaved PCM at the hardware rate (commonly 48 kHz) as int16 or float32, in arbitrary-size blocks. FeedPCM interprets, downmixes, resamples, and normalizes raw into the 16 kHz mono float32 the engine consumes, then funnels it into the same buffer Feed writes to. All conversion is pure Go with NO ffmpeg or other subprocess, reusing the helpers already in github.com/ardanlabs/bucky/pkg/audio. The pipeline is: 1. interpret raw bytes -&gt; []float32 per f.Sample (int16 -&gt; v/32768, or reinterpret little-endian float32); 2. downmix f.Channels interleaved -&gt; mono via audio.DownmixToMono; 3. resample f.SampleRate -&gt; 16000 via a per-stream audio.Resampler, the stateful streaming resampler that carries fractional phase across calls so block seams introduce no discontinuity or drift (linear interpolation, no anti-alias filter — adequate for whisper, which is trained on 16 kHz); 4. normalize clamp to [-1, 1]. raw need not contain a whole number of frames; a partial trailing frame is buffered until the next call so block boundaries that fall mid-frame do not corrupt the stream.</p>
            </div>

            <div className="doc-section" id="method-stream-reset">
              <h4>Stream.Reset</h4>
              <pre className="code-block">
                <code>func (s *Stream) Reset(ctx context.Context, opts ...ResetOption) error</code>
              </pre>
              <p className="doc-description">Reset clears the audio buffer and rolling linguistic context so the same Stream can begin a fresh logical session WITHOUT releasing its pool slot or worker. The whisper.State, pool slot, Events channel, and ActiveStreams count all survive; the energy-VAD state is cleared. Behavior is tunable via ResetOption. Reset blocks until any in-flight decode finishes and the worker has applied the reset.</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-decode">Decode</a></li>
                <li><a href="#func-decodechannels">DecodeChannels</a></li>
                <li><a href="#func-langid">LangID</a></li>
                <li><a href="#func-langmaxid">LangMaxID</a></li>
                <li><a href="#func-langstr">LangStr</a></li>
                <li><a href="#func-systeminfo">SystemInfo</a></li>
                <li><a href="#func-newmodel">NewModel</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-audioformat">AudioFormat</a></li>
                <li><a href="#type-channeltranscription">ChannelTranscription</a></li>
                <li><a href="#type-config">Config</a></li>
                <li><a href="#type-diarization">Diarization</a></li>
                <li><a href="#type-diarizedsegment">DiarizedSegment</a></li>
                <li><a href="#type-event">Event</a></li>
                <li><a href="#type-eventkind">EventKind</a></li>
                <li><a href="#type-model">Model</a></li>
                <li><a href="#type-modelinfo">ModelInfo</a></li>
                <li><a href="#type-option">Option</a></li>
                <li><a href="#type-resetconfig">ResetConfig</a></li>
                <li><a href="#type-resetoption">ResetOption</a></li>
                <li><a href="#type-sampleencoding">SampleEncoding</a></li>
                <li><a href="#type-segment">Segment</a></li>
                <li><a href="#type-stream">Stream</a></li>
                <li><a href="#type-streamconfig">StreamConfig</a></li>
                <li><a href="#type-streamoption">StreamOption</a></li>
                <li><a href="#type-transcribeconfig">TranscribeConfig</a></li>
                <li><a href="#type-transcribeoption">TranscribeOption</a></li>
                <li><a href="#type-transcription">Transcription</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-config-withdefaults">Config.WithDefaults</a></li>
                <li><a href="#method-model-config">Model.Config</a></li>
                <li><a href="#method-model-detectlanguage">Model.DetectLanguage</a></li>
                <li><a href="#method-model-modelinfo">Model.ModelInfo</a></li>
                <li><a href="#method-model-newstream">Model.NewStream</a></li>
                <li><a href="#method-model-transcribe">Model.Transcribe</a></li>
                <li><a href="#method-model-transcribechannels">Model.TranscribeChannels</a></li>
                <li><a href="#method-model-transcribechannelsfile">Model.TranscribeChannelsFile</a></li>
                <li><a href="#method-model-transcribefile">Model.TranscribeFile</a></li>
                <li><a href="#method-model-unload">Model.Unload</a></li>
                <li><a href="#method-stream-close">Stream.Close</a></li>
                <li><a href="#method-stream-events">Stream.Events</a></li>
                <li><a href="#method-stream-feed">Stream.Feed</a></li>
                <li><a href="#method-stream-feedpcm">Stream.FeedPCM</a></li>
                <li><a href="#method-stream-reset">Stream.Reset</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
