import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsSDKBucky() {
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
        <h2>Bucky Package</h2>
        <p>Package bucky is the high-level whisper SDK entry point. It mirrors the role sdk/kronk plays for the llama backend: cross-cutting initialization, library loading, and the Acquire / Transcribe surface.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/bucky"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-init">
              <h4>Init</h4>
              <pre className="code-block">
                <code>func Init(opts ...InitOption) error</code>
              </pre>
              <p className="doc-description">Init initializes the bucky / whisper backend. It registers the whisper backend with the cross-backend registry under backend.KindWhisper, then resolves the install path for the whisper.cpp shared library and loads it. If initialization fails, subsequent calls will retry — allowing libraries to be downloaded and loaded without restarting the server.</p>
            </div>

            <div className="doc-section" id="func-initialized">
              <h4>Initialized</h4>
              <pre className="code-block">
                <code>func Initialized() bool</code>
              </pre>
              <p className="doc-description">Initialized reports whether the bucky backend has been successfully initialized. This can be used to determine if the server is running in a degraded state due to missing whisper.cpp libraries.</p>
            </div>

            <div className="doc-section" id="func-langid">
              <h4>LangID</h4>
              <pre className="code-block">
                <code>func LangID(lang string) int32</code>
              </pre>
              <p className="doc-description">LangID returns the whisper.cpp internal id for the supplied language code (e.g. "de" → 2). Returns -1 if the code is unknown. Init must have been called before LangID, since the FFI symbol is resolved by whisper.Load.</p>
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

            <div className="doc-section" id="func-setfmtloggertraceid">
              <h4>SetFmtLoggerTraceID</h4>
              <pre className="code-block">
                <code>func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context</code>
              </pre>
              <p className="doc-description">SetFmtLoggerTraceID allows you to set a trace id on the context that can be included in FmtLogger output.</p>
            </div>

            <div className="doc-section" id="func-new">
              <h4>New</h4>
              <pre className="code-block">
                <code>func New(opts ...model.Option) (*Bucky, error)</code>
              </pre>
              <p className="doc-description">New provides the ability to use a whisper model in a concurrently safe way.</p>
            </div>

            <div className="doc-section" id="func-newwithcontext">
              <h4>NewWithContext</h4>
              <pre className="code-block">
                <code>func NewWithContext(ctx context.Context, opts ...model.Option) (*Bucky, error)</code>
              </pre>
              <p className="doc-description">NewWithContext provides the ability to use a whisper model in a concurrently safe way. The context is used to support logging trace ids during model loading.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-bucky">
              <h4>Bucky</h4>
              <pre className="code-block">
                <code>{`type Bucky struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Bucky provides a concurrently safe API for using whisper.cpp. Each Bucky owns one model.Model (which in turn owns one whisper.Context). The whisper context is single-stream so concurrent transcribes are bounded by a per-handle semaphore sized at construction time from Config.NSeqMax * Config.QueueDepth.</p>
            </div>

            <div className="doc-section" id="type-initoption">
              <h4>InitOption</h4>
              <pre className="code-block">
                <code>{`type InitOption func(*initOptions)`}</code>
              </pre>
              <p className="doc-description">InitOption represents options for configuring Init.</p>
            </div>

            <div className="doc-section" id="type-loglevel">
              <h4>LogLevel</h4>
              <pre className="code-block">
                <code>{`type LogLevel = applog.LogLevel`}</code>
              </pre>
              <p className="doc-description">LogLevel represents the logging level.</p>
            </div>

            <div className="doc-section" id="type-logger">
              <h4>Logger</h4>
              <pre className="code-block">
                <code>{`type Logger = applog.Logger`}</code>
              </pre>
              <p className="doc-description">Logger provides a function for logging messages from different APIs.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-bucky-activestreams">
              <h4>Bucky.ActiveStreams</h4>
              <pre className="code-block">
                <code>func (b *Bucky) ActiveStreams() int</code>
              </pre>
              <p className="doc-description">ActiveStreams returns the number of in-flight transcribe calls.</p>
            </div>

            <div className="doc-section" id="method-bucky-detectlanguage">
              <h4>Bucky.DetectLanguage</h4>
              <pre className="code-block">
                <code>func (b *Bucky) DetectLanguage(ctx context.Context, samples []float32, withProbs bool) (string, []float32, error)</code>
              </pre>
              <p className="doc-description">DetectLanguage runs a short whisper pass on the supplied 16 kHz mono float32 PCM samples and returns the detected language code along with the per-language probability vector (length LangMaxID()+1) when withProbs is true. DetectLanguage participates in the per-handle backpressure semaphore and blocks until a slot is available.</p>
            </div>

            <div className="doc-section" id="method-bucky-modelconfig">
              <h4>Bucky.ModelConfig</h4>
              <pre className="code-block">
                <code>func (b *Bucky) ModelConfig() model.Config</code>
              </pre>
              <p className="doc-description">ModelConfig returns a copy of the resolved configuration being used.</p>
            </div>

            <div className="doc-section" id="method-bucky-modelinfo">
              <h4>Bucky.ModelInfo</h4>
              <pre className="code-block">
                <code>func (b *Bucky) ModelInfo() model.ModelInfo</code>
              </pre>
              <p className="doc-description">ModelInfo returns the static information about the loaded model.</p>
            </div>

            <div className="doc-section" id="method-bucky-newstream">
              <h4>Bucky.NewStream</h4>
              <pre className="code-block">
                <code>func (b *Bucky) NewStream(ctx context.Context, opts ...model.StreamOption) (*model.Stream, error)</code>
              </pre>
              <p className="doc-description">NewStream opens a streaming transcription session against the loaded model. The session reserves one whisper.State for its lifetime and counts against ActiveStreams, so an open stream blocks Unload exactly like an in-flight Transcribe. The backpressure slot and pool state are both released when the returned stream's Close completes. Caller must call Close exactly once.</p>
            </div>

            <div className="doc-section" id="method-bucky-systeminfo">
              <h4>Bucky.SystemInfo</h4>
              <pre className="code-block">
                <code>func (b *Bucky) SystemInfo() map[string]string</code>
              </pre>
              <p className="doc-description">SystemInfo returns the whisper.cpp system info string parsed into a key/value map for observability output.</p>
            </div>

            <div className="doc-section" id="method-bucky-transcribe">
              <h4>Bucky.Transcribe</h4>
              <pre className="code-block">
                <code>func (b *Bucky) Transcribe(ctx context.Context, samples []float32, opts ...model.TranscribeOption) (model.Transcription, error)</code>
              </pre>
              <p className="doc-description">Transcribe runs the whisper.cpp pipeline on the provided 16 kHz mono float32 PCM samples and returns the decoded text along with per-segment metadata. The call participates in the per-handle backpressure semaphore and blocks until a slot is available.</p>
            </div>

            <div className="doc-section" id="method-bucky-transcribechannels">
              <h4>Bucky.TranscribeChannels</h4>
              <pre className="code-block">
                <code>func (b *Bucky) TranscribeChannels(ctx context.Context, channels [][]float32, opts ...model.TranscribeOption) (model.Diarization, error)</code>
              </pre>
              <p className="doc-description">TranscribeChannels transcribes each supplied channel of 16 kHz mono float32 PCM separately and merges the results into a single diarized transcript, treating one channel as one speaker. Pass the channels returned by model.DecodeChannels. The call participates in the per-handle backpressure semaphore and blocks until a slot is available.</p>
            </div>

            <div className="doc-section" id="method-bucky-transcribechannelsfile">
              <h4>Bucky.TranscribeChannelsFile</h4>
              <pre className="code-block">
                <code>func (b *Bucky) TranscribeChannelsFile(ctx context.Context, r io.Reader, opts ...model.TranscribeOption) (model.Diarization, error)</code>
              </pre>
              <p className="doc-description">TranscribeChannelsFile decodes audio from r preserving its channel layout and runs TranscribeChannels to produce a diarized transcript. Native multi-channel formats (WAV, FLAC) yield true per-channel diarization; formats that require ffmpeg are downmixed to a single channel. The call participates in the per-handle backpressure semaphore and blocks until a slot is available.</p>
            </div>

            <div className="doc-section" id="method-bucky-transcribefile">
              <h4>Bucky.TranscribeFile</h4>
              <pre className="code-block">
                <code>func (b *Bucky) TranscribeFile(ctx context.Context, r io.Reader, opts ...model.TranscribeOption) (model.Transcription, error)</code>
              </pre>
              <p className="doc-description">TranscribeFile decodes audio from r into 16 kHz mono float32 PCM (using ffmpeg when the input is a container the upstream pure-Go decoders do not handle) and then runs Transcribe on the resulting samples. The call participates in the per-handle backpressure semaphore and blocks until a slot is available.</p>
            </div>

            <div className="doc-section" id="method-bucky-unload">
              <h4>Bucky.Unload</h4>
              <pre className="code-block">
                <code>func (b *Bucky) Unload(ctx context.Context) error</code>
              </pre>
              <p className="doc-description">Unload will close down the loaded model. You should call this only when you are completely done using Bucky.</p>
            </div>
          </div>

          <div className="card" id="constants">
            <h3>Constants</h3>

            <div className="doc-section" id="const-logsilent">
              <h4>LogSilent</h4>
              <pre className="code-block">
                <code>{`const (
	LogSilent = applog.LogSilent
	LogNormal = applog.LogNormal
)`}</code>
              </pre>
              <p className="doc-description">Set of logging levels supported by whisper.cpp.</p>
            </div>

            <div className="doc-section" id="const-version">
              <h4>Version</h4>
              <pre className="code-block">
                <code>{`const Version = kronk.Version`}</code>
              </pre>
              <p className="doc-description">Version contains the current version of the bucky SDK package.</p>
            </div>
          </div>

          <div className="card" id="variables">
            <h3>Variables</h3>

            <div className="doc-section" id="var-discardlogger">
              <h4>DiscardLogger</h4>
              <pre className="code-block">
                <code>{`var DiscardLogger = applog.DiscardLogger`}</code>
              </pre>
              <p className="doc-description">DiscardLogger discards logging.</p>
            </div>

            <div className="doc-section" id="var-fmtlogger">
              <h4>FmtLogger</h4>
              <pre className="code-block">
                <code>{`var FmtLogger = applog.FmtLogger`}</code>
              </pre>
              <p className="doc-description">FmtLogger provides a basic logger that writes to stdout.</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-init">Init</a></li>
                <li><a href="#func-initialized">Initialized</a></li>
                <li><a href="#func-langid">LangID</a></li>
                <li><a href="#func-langmaxid">LangMaxID</a></li>
                <li><a href="#func-langstr">LangStr</a></li>
                <li><a href="#func-setfmtloggertraceid">SetFmtLoggerTraceID</a></li>
                <li><a href="#func-new">New</a></li>
                <li><a href="#func-newwithcontext">NewWithContext</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-bucky">Bucky</a></li>
                <li><a href="#type-initoption">InitOption</a></li>
                <li><a href="#type-loglevel">LogLevel</a></li>
                <li><a href="#type-logger">Logger</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-bucky-activestreams">Bucky.ActiveStreams</a></li>
                <li><a href="#method-bucky-detectlanguage">Bucky.DetectLanguage</a></li>
                <li><a href="#method-bucky-modelconfig">Bucky.ModelConfig</a></li>
                <li><a href="#method-bucky-modelinfo">Bucky.ModelInfo</a></li>
                <li><a href="#method-bucky-newstream">Bucky.NewStream</a></li>
                <li><a href="#method-bucky-systeminfo">Bucky.SystemInfo</a></li>
                <li><a href="#method-bucky-transcribe">Bucky.Transcribe</a></li>
                <li><a href="#method-bucky-transcribechannels">Bucky.TranscribeChannels</a></li>
                <li><a href="#method-bucky-transcribechannelsfile">Bucky.TranscribeChannelsFile</a></li>
                <li><a href="#method-bucky-transcribefile">Bucky.TranscribeFile</a></li>
                <li><a href="#method-bucky-unload">Bucky.Unload</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#constants" className="doc-index-header">Constants</a>
              <ul>
                <li><a href="#const-logsilent">LogSilent</a></li>
                <li><a href="#const-version">Version</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#variables" className="doc-index-header">Variables</a>
              <ul>
                <li><a href="#var-discardlogger">DiscardLogger</a></li>
                <li><a href="#var-fmtlogger">FmtLogger</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
