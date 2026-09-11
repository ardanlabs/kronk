import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../services/api';
import type { ImageEditRequest, ImageGenerationResponse, ImageProgressEvent, MalinaModelEntry } from '../types';
import { FieldLabel } from './ParamTooltips';

const STORAGE_KEY = 'kronk_image_generator_model';
const MAX_IMAGE_BYTES = 25 * 1024 * 1024;

type GenerationMode = 'text' | 'image';

interface SourceImage {
  file: File;
  objectURL: string;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export default function ImageGenerator() {
  const [models, setModels] = useState<MalinaModelEntry[]>([]);
  const [modelsLoading, setModelsLoading] = useState(true);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [selectedModel, setSelectedModel] = useState(
    () => localStorage.getItem(STORAGE_KEY) || '',
  );
  const [prompt, setPrompt] = useState('');
  const [mode, setMode] = useState<GenerationMode>('text');
  const [source, setSource] = useState<SourceImage | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [negativePrompt, setNegativePrompt] = useState('');
  const [size, setSize] = useState('');
  const [steps, setSteps] = useState(20);
  const [cfgScale, setCFGScale] = useState(7);
  const [seed, setSeed] = useState(-1);
  const [strength, setStrength] = useState(0.75);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ImageGenerationResponse | null>(null);
  const [progress, setProgress] = useState<ImageProgressEvent | null>(null);
  const [progressConnected, setProgressConnected] = useState(false);
  const [progressError, setProgressError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadModels = useCallback(async () => {
    setModelsLoading(true);
    setModelsError(null);
    try {
      const response = await api.listMalinaModels();
      setModels(response.models ?? []);
    } catch (err) {
      setModelsError((err as Error).message);
      setModels([]);
    } finally {
      setModelsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadModels();
  }, [loadModels]);

  useEffect(() => api.streamImageProgress(
    (event) => {
      setProgressConnected(true);
      setProgressError(null);
      if (event.status === 'progress') setProgress(event);
    },
    (message) => {
      setProgressConnected(false);
      setProgressError(message);
    },
  ), []);

  useEffect(() => {
    if (models.length === 0) return;
    if (!models.some((model) => model.id === selectedModel)) {
      setSelectedModel(models[0].id);
    }
  }, [models, selectedModel]);

  useEffect(() => {
    if (selectedModel) localStorage.setItem(STORAGE_KEY, selectedModel);
  }, [selectedModel]);

  useEffect(() => {
    return () => {
      if (source) URL.revokeObjectURL(source.objectURL);
    };
  }, [source]);

  const selected = useMemo(
    () => models.find((model) => model.id === selectedModel),
    [models, selectedModel],
  );
  const image = result?.data[0];
  const canSubmit = !!selectedModel && !!prompt.trim() && (mode === 'text' || !!source) && !submitting;

  const acceptFile = useCallback((file: File) => {
    if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
      setError('Source image must be a PNG or JPEG.');
      return;
    }
    if (file.size > MAX_IMAGE_BYTES) {
      setError('Source image exceeds the 25 MB limit.');
      return;
    }
    setSource({ file, objectURL: URL.createObjectURL(file) });
    setError(null);
    setResult(null);
  }, []);

  const clearSource = useCallback(() => {
    setSource(null);
    setResult(null);
  }, []);

  const handleSubmit = useCallback(async (event: React.FormEvent) => {
    event.preventDefault();
    if (!canSubmit) return;

    setSubmitting(true);
    setError(null);
    setResult(null);
    setProgress(null);
    try {
      const request: ImageEditRequest = {
        model: selectedModel,
        prompt: prompt.trim(),
        negative_prompt: negativePrompt.trim() || undefined,
        size: size || undefined,
        steps,
        cfg_scale: cfgScale,
        seed,
      };
      if (mode === 'image' && source) {
        request.strength = strength;
        setResult(await api.editImage(request, source.file));
      } else {
        setResult(await api.generateImage(request));
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }, [canSubmit, cfgScale, mode, negativePrompt, prompt, seed, selectedModel, size, source, steps, strength]);

  const downloadImage = useCallback(() => {
    if (!image) return;
    const link = document.createElement('a');
    link.href = `data:image/png;base64,${image.b64_json}`;
    link.download = `${selectedModel}-${image.seed}.png`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  }, [image, selectedModel]);

  return (
    <div className="image-generator-page">
      <div className="page-header">
        <h2>Image Generator</h2>
        <p>Generate a PNG from a text prompt using an installed Malina model.</p>
      </div>

      {modelsError && (
        <div className="alert alert-error">Failed to load Malina models: {modelsError}</div>
      )}
      {!modelsLoading && models.length === 0 && !modelsError && (
        <div className="alert alert-error">
          No Malina models installed. Install one with <code>kronk malina model pull --local sd-1.5</code>.
        </div>
      )}

      <div className="image-generator-grid">
        <form className="image-generator-card" onSubmit={handleSubmit}>
          <h3>Generate</h3>

          <FieldLabel tooltipKey="imageGeneratorMode">Mode</FieldLabel>
          <div className="image-generator-mode" role="group" aria-label="Generation mode">
            <button
              className={`btn ${mode === 'text' ? 'btn-primary' : 'btn-secondary'}`}
              type="button"
              onClick={() => setMode('text')}
              disabled={submitting}
            >
              Text to image
            </button>
            <button
              className={`btn ${mode === 'image' ? 'btn-primary' : 'btn-secondary'}`}
              type="button"
              onClick={() => setMode('image')}
              disabled={submitting}
            >
              Image to image
            </button>
          </div>

          <FieldLabel htmlFor="image-generator-model" tooltipKey="imageGeneratorModel">
            Model
          </FieldLabel>
          <select
            id="image-generator-model"
            className="form-select"
            value={selectedModel}
            onChange={(event) => setSelectedModel(event.target.value)}
            disabled={modelsLoading || models.length === 0 || submitting}
          >
            {modelsLoading && <option>Loading…</option>}
            {!modelsLoading && models.length === 0 && <option>No models available</option>}
            {models.map((model) => (
              <option key={model.id} value={model.id}>{model.id}</option>
            ))}
          </select>
          {selected && (
            <p className="image-generator-model-info">
              {selected.description} ({formatBytes(selected.size)})
            </p>
          )}

          {mode === 'image' && (
            <>
              <FieldLabel tooltipKey="imageGeneratorSource">Source image</FieldLabel>
              {!source ? (
                <div
                  className={`image-generator-drop ${dragOver ? 'image-generator-drop-over' : ''}`}
                  onDragOver={(event) => { event.preventDefault(); setDragOver(true); }}
                  onDragLeave={() => setDragOver(false)}
                  onDrop={(event) => {
                    event.preventDefault();
                    setDragOver(false);
                    const file = event.dataTransfer.files?.[0];
                    if (file) acceptFile(file);
                  }}
                  onClick={() => fileInputRef.current?.click()}
                >
                  <strong>Drop a PNG or JPEG here</strong>
                  <span>or click to choose an image</span>
                </div>
              ) : (
                <div className="image-generator-source-preview">
                  <img src={source.objectURL} alt="Source" />
                  <div>
                    <strong>{source.file.name}</strong>
                    <span>{formatBytes(source.file.size)}</span>
                    <button className="btn-link" type="button" onClick={clearSource}>remove</button>
                  </div>
                </div>
              )}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg"
                hidden
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (file) acceptFile(file);
                  event.target.value = '';
                }}
              />
            </>
          )}

          <FieldLabel htmlFor="image-generator-prompt" tooltipKey="imageGeneratorPrompt">
            Prompt
          </FieldLabel>
          <textarea
            id="image-generator-prompt"
            className="image-generator-prompt"
            rows={7}
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="A lighthouse on a rocky coast at sunset"
            disabled={submitting}
          />

          <details className="image-generator-settings">
            <summary>Generation settings</summary>

            <FieldLabel htmlFor="image-generator-negative-prompt" tooltipKey="imageGeneratorNegativePrompt">
              Negative prompt
            </FieldLabel>
            <textarea
              id="image-generator-negative-prompt"
              className="image-generator-prompt"
              rows={3}
              value={negativePrompt}
              onChange={(event) => setNegativePrompt(event.target.value)}
              placeholder="blurry, distorted, artifacts"
              disabled={submitting}
            />

            <FieldLabel htmlFor="image-generator-size" tooltipKey="imageGeneratorSize">Size</FieldLabel>
            <select id="image-generator-size" className="form-select" value={size} onChange={(event) => setSize(event.target.value)} disabled={submitting}>
              <option value="">Automatic</option>
              <option value="512x512">512×512</option>
              <option value="768x512">768×512</option>
              <option value="512x768">512×768</option>
              <option value="1024x1024">1024×1024</option>
            </select>

            <div className="image-generator-settings-grid">
              <div>
                <FieldLabel htmlFor="image-generator-steps" tooltipKey="imageGeneratorSteps">Steps</FieldLabel>
                <input id="image-generator-steps" className="form-input" type="number" min={1} max={1000} value={steps} onChange={(event) => setSteps(Number(event.target.value))} disabled={submitting} />
              </div>
              <div>
                <FieldLabel htmlFor="image-generator-cfg" tooltipKey="imageGeneratorCFGScale">CFG scale</FieldLabel>
                <input id="image-generator-cfg" className="form-input" type="number" min={0.1} step={0.1} value={cfgScale} onChange={(event) => setCFGScale(Number(event.target.value))} disabled={submitting} />
              </div>
              <div>
                <FieldLabel htmlFor="image-generator-seed" tooltipKey="imageGeneratorSeed">Seed</FieldLabel>
                <input id="image-generator-seed" className="form-input" type="number" min={-1} step={1} value={seed} onChange={(event) => setSeed(Number(event.target.value))} disabled={submitting} />
              </div>
              {mode === 'image' && (
                <div>
                  <FieldLabel htmlFor="image-generator-strength" tooltipKey="imageGeneratorStrength">Strength</FieldLabel>
                  <input id="image-generator-strength" className="form-input" type="number" min={0.01} max={1} step={0.01} value={strength} onChange={(event) => setStrength(Number(event.target.value))} disabled={submitting} />
                </div>
              )}
            </div>
          </details>

          <button className="btn btn-primary" type="submit" disabled={!canSubmit}>
            {submitting ? 'Generating…' : mode === 'image' ? 'Transform Image' : 'Generate Image'}
          </button>
          {error && <div className="alert alert-error">{error}</div>}
        </form>

        <section className="image-generator-card image-generator-preview">
          <div className="image-generator-preview-header">
            <h3>Result</h3>
            {image && (
              <button className="btn btn-secondary" type="button" onClick={downloadImage}>
                Download PNG
              </button>
            )}
          </div>

          <div className="image-generator-progress" role="status" aria-live="polite">
            <div className="image-generator-progress-header">
              <strong>Global Malina activity</strong>
              <span>
                {progress?.percent !== undefined
                  ? `${progress.percent.toFixed(progress.percent < 10 ? 1 : 0)}%`
                  : progressConnected ? 'Listening' : 'Unavailable'}
              </span>
            </div>
            <progress max={100} value={progress?.percent ?? 0} />
            <small>
              {progress && progress.step !== undefined && progress.steps !== undefined
                ? `Latest update: ${progress.step} / ${progress.steps}${progress.seconds_per_step ? ` · ${progress.seconds_per_step.toFixed(2)} s/step` : ''}. `
                : progressError ? `${progressError}. ` : 'Waiting for model loading or generation progress. '}
              This server-wide feed is not tied to this request.
            </small>
          </div>

          {submitting && <div className="image-generator-empty">Generating image…</div>}
          {!submitting && !image && (
            <div className="image-generator-empty">Your generated image will appear here.</div>
          )}
          {image && (
            <>
              <img
                className="image-generator-image"
                src={`data:image/png;base64,${image.b64_json}`}
                alt={prompt.trim() || 'Generated image'}
              />
              <div className="image-generator-meta">
                <span>{image.width}×{image.height}</span>
                {image.seed >= 0 && <span>Seed {image.seed}</span>}
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
