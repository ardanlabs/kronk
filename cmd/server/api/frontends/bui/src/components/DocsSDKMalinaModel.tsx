import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsSDKMalinaModel() {
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
        <h2>MalinaModel Package</h2>
        <p>Package model configures and owns reusable stable-diffusion model contexts for the Malina SDK.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/malina/model"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-saveavi">
              <h4>SaveAVI</h4>
              <pre className="code-block">
                <code>func SaveAVI(filename string, frames []image.Image, fps int, quality int) (err error)</code>
              </pre>
              <p className="doc-description">SaveAVI writes standard Go images as a Motion-JPEG AVI file.</p>
            </div>

            <div className="doc-section" id="func-newconfig">
              <h4>NewConfig</h4>
              <pre className="code-block">
                <code>func NewConfig(opts ...Option) (Config, error)</code>
              </pre>
              <p className="doc-description">NewConfig constructs and validates Config.</p>
            </div>

            <div className="doc-section" id="func-newmodel">
              <h4>NewModel</h4>
              <pre className="code-block">
                <code>func NewModel(ctx context.Context, cfg Config) (*Model, error)</code>
              </pre>
              <p className="doc-description">NewModel loads one reusable native model context.</p>
            </div>

            <div className="doc-section" id="func-newupscaler">
              <h4>NewUpscaler</h4>
              <pre className="code-block">
                <code>func NewUpscaler(ctx context.Context, cfg UpscalerConfig) (*Upscaler, error)</code>
              </pre>
              <p className="doc-description">NewUpscaler loads a reusable ESRGAN model context.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-audio">
              <h4>Audio</h4>
              <pre className="code-block">
                <code>{`type Audio struct {
	SampleRate uint32
	Channels   uint32
	Data       []float32
}`}</code>
              </pre>
              <p className="doc-description">Audio contains generated interleaved floating-point samples.</p>
            </div>

            <div className="doc-section" id="type-cannyparams">
              <h4>CannyParams</h4>
              <pre className="code-block">
                <code>{`type CannyParams struct {
	HighThreshold float32
	LowThreshold  float32
	Weak          float32
	Strong        float32
	Inverse       bool
}`}</code>
              </pre>
              <p className="doc-description">CannyParams controls edge detection applied to a ControlNet image.</p>
            </div>

            <div className="doc-section" id="type-config">
              <h4>Config</h4>
              <pre className="code-block">
                <code>{`type Config struct {
	ModelPath                   string
	ClipLPath                   string
	ClipGPath                   string
	ClipVisionPath              string
	T5XXLPath                   string
	LLMPath                     string
	LLMVisionPath               string
	DiffusionModelPath          string
	HighNoiseDiffusionModelPath string
	EmbeddingsConnectorsPath    string
	VAEPath                     string
	AudioVAEPath                string
	TAESDPath                   string
	ControlNetPath              string
	MotionModulePath            string
	ADetailerPath               string
	PhotoMakerPath              string
	TensorTypeRules             string
	Concurrency                 int
	QueueDepth                  int
	AdmissionTimeout            time.Duration
	CPUThreads                  int32
	LinearScale                 float32
	AttnScale                   float32
}`}</code>
              </pre>
              <p className="doc-description">Config controls model loading and request admission. Concurrency controls the number of independently loaded contexts and simultaneous generations. QueueDepth controls how many calls are admitted to wait after every context is busy. ModelPath loads an all-in-one checkpoint. DiffusionModelPath and its companion paths configure a component model. At least one of ModelPath or DiffusionModelPath is required.</p>
            </div>

            <div className="doc-section" id="type-detailparams">
              <h4>DetailParams</h4>
              <pre className="code-block">
                <code>{`type DetailParams struct {
	Image          image.Image
	Prompt         string
	NegativePrompt string
	ExtraArgs      string
	Steps          int
	CFGScale       float32
	Seed           int64
}`}</code>
              </pre>
              <p className="doc-description">DetailParams controls one ADetailer face-refinement pass.</p>
            </div>

            <div className="doc-section" id="type-generateparams">
              <h4>GenerateParams</h4>
              <pre className="code-block">
                <code>{`type GenerateParams struct {
	Prompt          string
	NegativePrompt  string
	Width           int
	Height          int
	Steps           int
	CFGScale        float32
	Seed            int64
	InitImage       image.Image
	Strength        float32
	ControlImage    image.Image
	ControlStrength float32
	Canny           *CannyParams
}`}</code>
              </pre>
              <p className="doc-description">GenerateParams controls one text-to-image generation.</p>
            </div>

            <div className="doc-section" id="type-generatedimage">
              <h4>GeneratedImage</h4>
              <pre className="code-block">
                <code>{`type GeneratedImage struct {
	PNG    []byte
	Width  int
	Height int
	Seed   int64
}`}</code>
              </pre>
              <p className="doc-description">GeneratedImage contains an owned PNG and generation metadata.</p>
            </div>

            <div className="doc-section" id="type-generatedvideo">
              <h4>GeneratedVideo</h4>
              <pre className="code-block">
                <code>{`type GeneratedVideo struct {
	Frames []image.Image
	Audio  *Audio
	FPS    int
	Seed   int64
}`}</code>
              </pre>
              <p className="doc-description">GeneratedVideo contains owned frames and optional generated audio.</p>
            </div>

            <div className="doc-section" id="type-model">
              <h4>Model</h4>
              <pre className="code-block">
                <code>{`type Model struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Model owns exactly one native stable-diffusion context.</p>
            </div>

            <div className="doc-section" id="type-modelinfo">
              <h4>ModelInfo</h4>
              <pre className="code-block">
                <code>{`type ModelInfo struct {
	ModelPath          string
	DiffusionModelPath string
	MotionModulePath   string
	ADetailerPath      string
	ModelVersion       string
	CPUThreads         int32
}`}</code>
              </pre>
              <p className="doc-description">ModelInfo describes a loaded model.</p>
            </div>

            <div className="doc-section" id="type-option">
              <h4>Option</h4>
              <pre className="code-block">
                <code>{`type Option func(*Config)`}</code>
              </pre>
              <p className="doc-description">Option modifies Config.</p>
            </div>

            <div className="doc-section" id="type-upscaler">
              <h4>Upscaler</h4>
              <pre className="code-block">
                <code>{`type Upscaler struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Upscaler owns one native ESRGAN context.</p>
            </div>

            <div className="doc-section" id="type-upscalerconfig">
              <h4>UpscalerConfig</h4>
              <pre className="code-block">
                <code>{`type UpscalerConfig struct {
	ModelPath     string
	Direct        bool
	CPUThreads    int32
	TileSize      int32
	Backend       string
	ParamsBackend string
}`}</code>
              </pre>
              <p className="doc-description">UpscalerConfig controls loading a standalone ESRGAN upscaler.</p>
            </div>

            <div className="doc-section" id="type-videoparams">
              <h4>VideoParams</h4>
              <pre className="code-block">
                <code>{`type VideoParams struct {
	Prompt         string
	NegativePrompt string
	Width          int
	Height         int
	Steps          int
	Seed           int64
	Frames         int
	FPS            int
}`}</code>
              </pre>
              <p className="doc-description">VideoParams controls one AnimateDiff generation.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-detailparams-validate">
              <h4>DetailParams.Validate</h4>
              <pre className="code-block">
                <code>func (p DetailParams) Validate() error</code>
              </pre>
              <p className="doc-description">Validate checks whether parameters describe a supported ADetailer request.</p>
            </div>

            <div className="doc-section" id="method-generateparams-validate">
              <h4>GenerateParams.Validate</h4>
              <pre className="code-block">
                <code>func (p GenerateParams) Validate() error</code>
              </pre>
              <p className="doc-description">Validate checks whether parameters describe a supported image-generation request.</p>
            </div>

            <div className="doc-section" id="method-model-config">
              <h4>Model.Config</h4>
              <pre className="code-block">
                <code>func (m *Model) Config() Config</code>
              </pre>
              <p className="doc-description">Config returns immutable model configuration.</p>
            </div>

            <div className="doc-section" id="method-model-detail">
              <h4>Model.Detail</h4>
              <pre className="code-block">
                <code>func (m *Model) Detail(ctx context.Context, params DetailParams) (GeneratedImage, error)</code>
              </pre>
              <p className="doc-description">Detail detects faces and returns the final refined image.</p>
            </div>

            <div className="doc-section" id="method-model-generate">
              <h4>Model.Generate</h4>
              <pre className="code-block">
                <code>func (m *Model) Generate(ctx context.Context, params GenerateParams) (GeneratedImage, error)</code>
              </pre>
              <p className="doc-description">Generate runs synchronous text-to-image generation. Calls on this Model are serialized, while independent Model contexts may generate concurrently. Canceling ctx interrupts native generation and resets the context for reuse.</p>
            </div>

            <div className="doc-section" id="method-model-generatevideo">
              <h4>Model.GenerateVideo</h4>
              <pre className="code-block">
                <code>func (m *Model) GenerateVideo(ctx context.Context, params VideoParams) (GeneratedVideo, error)</code>
              </pre>
              <p className="doc-description">GenerateVideo runs synchronous AnimateDiff generation.</p>
            </div>

            <div className="doc-section" id="method-model-info">
              <h4>Model.Info</h4>
              <pre className="code-block">
                <code>func (m *Model) Info() ModelInfo</code>
              </pre>
              <p className="doc-description">Info returns descriptive model information.</p>
            </div>

            <div className="doc-section" id="method-model-stop">
              <h4>Model.Stop</h4>
              <pre className="code-block">
                <code>func (m *Model) Stop()</code>
              </pre>
              <p className="doc-description">Stop prevents generation calls that have not started from entering stable-diffusion.cpp and requests cancellation of an active generation.</p>
            </div>

            <div className="doc-section" id="method-model-unload">
              <h4>Model.Unload</h4>
              <pre className="code-block">
                <code>func (m *Model) Unload() error</code>
              </pre>
              <p className="doc-description">Unload releases the native context exactly once.</p>
            </div>

            <div className="doc-section" id="method-upscaler-config">
              <h4>Upscaler.Config</h4>
              <pre className="code-block">
                <code>func (u *Upscaler) Config() UpscalerConfig</code>
              </pre>
              <p className="doc-description">Config returns immutable upscaler configuration.</p>
            </div>

            <div className="doc-section" id="method-upscaler-factor">
              <h4>Upscaler.Factor</h4>
              <pre className="code-block">
                <code>func (u *Upscaler) Factor(ctx context.Context) (int, error)</code>
              </pre>
              <p className="doc-description">Factor returns the model's native upscale factor.</p>
            </div>

            <div className="doc-section" id="method-upscaler-unload">
              <h4>Upscaler.Unload</h4>
              <pre className="code-block">
                <code>func (u *Upscaler) Unload() error</code>
              </pre>
              <p className="doc-description">Unload releases the native upscaler context exactly once.</p>
            </div>

            <div className="doc-section" id="method-upscaler-upscale">
              <h4>Upscaler.Upscale</h4>
              <pre className="code-block">
                <code>func (u *Upscaler) Upscale(ctx context.Context, input image.Image) ([]image.Image, error)</code>
              </pre>
              <p className="doc-description">Upscale enlarges an image using the model's native scale factor. Cancellation cannot interrupt an ESRGAN call that has already entered native code.</p>
            </div>

            <div className="doc-section" id="method-videoparams-validate">
              <h4>VideoParams.Validate</h4>
              <pre className="code-block">
                <code>func (p VideoParams) Validate() error</code>
              </pre>
              <p className="doc-description">Validate checks whether parameters describe a supported video-generation request.</p>
            </div>
          </div>

          <div className="card" id="variables">
            <h3>Variables</h3>

            <div className="doc-section" id="var-variable">
              <h4>Variable</h4>
              <pre className="code-block">
                <code>{`var (`}</code>
              </pre>
              <p className="doc-description">// ErrInvalidRequest identifies invalid generation parameters. ErrInvalidRequest = errors.New("invalid generation request") // ErrNativeGeneration identifies a failure returned by stable-diffusion. ErrNativeGeneration = errors.New("native generation failed")</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-saveavi">SaveAVI</a></li>
                <li><a href="#func-newconfig">NewConfig</a></li>
                <li><a href="#func-newmodel">NewModel</a></li>
                <li><a href="#func-newupscaler">NewUpscaler</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-audio">Audio</a></li>
                <li><a href="#type-cannyparams">CannyParams</a></li>
                <li><a href="#type-config">Config</a></li>
                <li><a href="#type-detailparams">DetailParams</a></li>
                <li><a href="#type-generateparams">GenerateParams</a></li>
                <li><a href="#type-generatedimage">GeneratedImage</a></li>
                <li><a href="#type-generatedvideo">GeneratedVideo</a></li>
                <li><a href="#type-model">Model</a></li>
                <li><a href="#type-modelinfo">ModelInfo</a></li>
                <li><a href="#type-option">Option</a></li>
                <li><a href="#type-upscaler">Upscaler</a></li>
                <li><a href="#type-upscalerconfig">UpscalerConfig</a></li>
                <li><a href="#type-videoparams">VideoParams</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-detailparams-validate">DetailParams.Validate</a></li>
                <li><a href="#method-generateparams-validate">GenerateParams.Validate</a></li>
                <li><a href="#method-model-config">Model.Config</a></li>
                <li><a href="#method-model-detail">Model.Detail</a></li>
                <li><a href="#method-model-generate">Model.Generate</a></li>
                <li><a href="#method-model-generatevideo">Model.GenerateVideo</a></li>
                <li><a href="#method-model-info">Model.Info</a></li>
                <li><a href="#method-model-stop">Model.Stop</a></li>
                <li><a href="#method-model-unload">Model.Unload</a></li>
                <li><a href="#method-upscaler-config">Upscaler.Config</a></li>
                <li><a href="#method-upscaler-factor">Upscaler.Factor</a></li>
                <li><a href="#method-upscaler-unload">Upscaler.Unload</a></li>
                <li><a href="#method-upscaler-upscale">Upscaler.Upscale</a></li>
                <li><a href="#method-videoparams-validate">VideoParams.Validate</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#variables" className="doc-index-header">Variables</a>
              <ul>
                <li><a href="#var-variable">Variable</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
