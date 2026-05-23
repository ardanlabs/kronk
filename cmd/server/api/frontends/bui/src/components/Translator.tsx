import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import { FieldLabel, labelWithTip } from './ParamTooltips';
import type {
  BuckyModelEntry,
  BuckyModelDetails,
  TranscriptionResponse,
} from '../types';

// Whisper supports ~99 languages. Listing the common ones plus a
// catch-all of ISO 639-1 codes covers the practical workshop case.
const LANGUAGES: { code: string; label: string }[] = [
  { code: '', label: 'Auto-detect' },
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Spanish' },
  { code: 'fr', label: 'French' },
  { code: 'de', label: 'German' },
  { code: 'it', label: 'Italian' },
  { code: 'pt', label: 'Portuguese' },
  { code: 'nl', label: 'Dutch' },
  { code: 'pl', label: 'Polish' },
  { code: 'ru', label: 'Russian' },
  { code: 'uk', label: 'Ukrainian' },
  { code: 'tr', label: 'Turkish' },
  { code: 'ar', label: 'Arabic' },
  { code: 'he', label: 'Hebrew' },
  { code: 'hi', label: 'Hindi' },
  { code: 'ja', label: 'Japanese' },
  { code: 'ko', label: 'Korean' },
  { code: 'zh', label: 'Chinese' },
  { code: 'vi', label: 'Vietnamese' },
  { code: 'th', label: 'Thai' },
  { code: 'id', label: 'Indonesian' },
  { code: 'sv', label: 'Swedish' },
  { code: 'no', label: 'Norwegian' },
  { code: 'da', label: 'Danish' },
  { code: 'fi', label: 'Finnish' },
  { code: 'cs', label: 'Czech' },
  { code: 'el', label: 'Greek' },
  { code: 'ro', label: 'Romanian' },
  { code: 'hu', label: 'Hungarian' },
];

const MAX_RECORD_SEC = 30;
const STORAGE_KEY = 'kronk_translator_model';

function isEnglishOnly(id: string): boolean {
  return /\.en\b|-en\b/i.test(id);
}

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function fmtSeconds(s: number): string {
  if (!isFinite(s) || s < 0) return '0s';
  const m = Math.floor(s / 60);
  const r = s - m * 60;
  if (m > 0) return `${m}m ${r.toFixed(1)}s`;
  return `${r.toFixed(1)}s`;
}

function srtTimestamp(sec: number): string {
  const total = Math.max(0, Math.round(sec * 1000));
  const h = Math.floor(total / 3600000);
  const m = Math.floor((total % 3600000) / 60000);
  const s = Math.floor((total % 60000) / 1000);
  const ms = total % 1000;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')},${String(ms).padStart(3, '0')}`;
}

function vttTimestamp(sec: number): string {
  return srtTimestamp(sec).replace(',', '.');
}

function buildSRT(r: TranscriptionResponse): string {
  return r.segments
    .map((s, i) => `${i + 1}\n${srtTimestamp(s.start)} --> ${srtTimestamp(s.end)}\n${s.text.trim()}\n`)
    .join('\n');
}

function buildVTT(r: TranscriptionResponse): string {
  const body = r.segments
    .map((s) => `${vttTimestamp(s.start)} --> ${vttTimestamp(s.end)}\n${s.text.trim()}\n`)
    .join('\n');
  return `WEBVTT\n\n${body}`;
}

function downloadBlob(filename: string, data: string, mime: string): void {
  const blob = new Blob([data], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

interface AudioInput {
  blob: Blob;
  filename: string;
  size: number;
  source: 'file' | 'mic';
  objectURL: string;
}

interface RunResult {
  response: TranscriptionResponse;
  wallMs: number;
  model: string;
  multilingual: boolean;
  translated: boolean;
  sourceHint: string;
  inputName: string;
  inputSize: number;
  inputSource: 'file' | 'mic';
}

export default function Translator() {
  const { token } = useToken();

  const [models, setModels] = useState<BuckyModelEntry[]>([]);
  const [modelsLoading, setModelsLoading] = useState(true);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [selectedModel, setSelectedModel] = useState<string>(
    () => localStorage.getItem(STORAGE_KEY) || '',
  );
  const [modelDetails, setModelDetails] = useState<BuckyModelDetails | null>(null);

  const [language, setLanguage] = useState<string>('');
  const [translate, setTranslate] = useState<boolean>(false);
  const [prompt, setPrompt] = useState<string>('');

  const [input, setInput] = useState<AudioInput | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [recording, setRecording] = useState(false);
  const [recordSec, setRecordSec] = useState(0);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const recordChunksRef = useRef<Blob[]>([]);
  const recordTimerRef = useRef<number | null>(null);
  const recordStartRef = useRef<number>(0);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<RunResult | null>(null);
  const [showSegments, setShowSegments] = useState(false);

  // -------------------------------------------------------------- models

  const loadModels = useCallback(async () => {
    setModelsLoading(true);
    setModelsError(null);
    try {
      const resp = await api.listBuckyModels();
      setModels(resp.models ?? []);
    } catch (err) {
      setModelsError((err as Error).message);
      setModels([]);
    } finally {
      setModelsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  useEffect(() => {
    if (models.length === 0) return;
    const valid = models.some((m) => m.id === selectedModel);
    if (!valid) {
      setSelectedModel(models[0].id);
    }
  }, [models, selectedModel]);

  useEffect(() => {
    if (!selectedModel) {
      setModelDetails(null);
      return;
    }
    localStorage.setItem(STORAGE_KEY, selectedModel);
    let cancelled = false;
    api
      .getBuckyModelDetails(selectedModel)
      .then((d) => { if (!cancelled) setModelDetails(d); })
      .catch(() => { if (!cancelled) setModelDetails(null); });
    return () => { cancelled = true; };
  }, [selectedModel]);

  const multilingual = useMemo(() => {
    if (modelDetails) return modelDetails.is_multilingual;
    return !isEnglishOnly(selectedModel);
  }, [modelDetails, selectedModel]);

  // English-only models cannot translate and cannot accept a non-en hint.
  useEffect(() => {
    if (!multilingual) {
      if (translate) setTranslate(false);
      if (language && language !== 'en') setLanguage('');
    }
  }, [multilingual, translate, language]);

  // ---------------------------------------------------------------- input

  const replaceInput = useCallback((next: AudioInput | null) => {
    setInput((prev) => {
      if (prev) URL.revokeObjectURL(prev.objectURL);
      return next;
    });
  }, []);

  useEffect(() => {
    return () => {
      if (input) URL.revokeObjectURL(input.objectURL);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const acceptFile = useCallback((f: File) => {
    replaceInput({
      blob: f,
      filename: f.name,
      size: f.size,
      source: 'file',
      objectURL: URL.createObjectURL(f),
    });
    setResult(null);
    setError(null);
  }, [replaceInput]);

  const onFileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    if (f) acceptFile(f);
    e.target.value = '';
  }, [acceptFile]);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const f = e.dataTransfer.files?.[0];
    if (f) acceptFile(f);
  }, [acceptFile]);

  // ---------------------------------------------------------------- mic

  const stopRecording = useCallback(() => {
    const rec = recorderRef.current;
    if (rec && rec.state !== 'inactive') {
      rec.stop();
    }
    if (recordTimerRef.current !== null) {
      window.clearInterval(recordTimerRef.current);
      recordTimerRef.current = null;
    }
  }, []);

  const startRecording = useCallback(async () => {
    setError(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const rec = new MediaRecorder(stream);
      recorderRef.current = rec;
      recordChunksRef.current = [];

      rec.ondataavailable = (e) => {
        if (e.data.size > 0) recordChunksRef.current.push(e.data);
      };

      rec.onstop = () => {
        stream.getTracks().forEach((t) => t.stop());
        const blob = new Blob(recordChunksRef.current, { type: rec.mimeType || 'audio/webm' });
        const ext = (rec.mimeType || 'audio/webm').includes('ogg') ? 'ogg' : 'webm';
        replaceInput({
          blob,
          filename: `mic-${new Date().toISOString().replace(/[:.]/g, '-')}.${ext}`,
          size: blob.size,
          source: 'mic',
          objectURL: URL.createObjectURL(blob),
        });
        setRecording(false);
        setRecordSec(0);
        recorderRef.current = null;
      };

      recordStartRef.current = Date.now();
      rec.start();
      setRecording(true);
      setResult(null);
      setRecordSec(0);

      recordTimerRef.current = window.setInterval(() => {
        const elapsed = (Date.now() - recordStartRef.current) / 1000;
        setRecordSec(elapsed);
        if (elapsed >= MAX_RECORD_SEC) {
          stopRecording();
        }
      }, 100);
    } catch (err) {
      setError(`Microphone error: ${(err as Error).message}`);
    }
  }, [replaceInput, stopRecording]);

  useEffect(() => {
    return () => {
      stopRecording();
    };
  }, [stopRecording]);

  // ---------------------------------------------------------------- submit

  const canSubmit = !!selectedModel && !!input && !submitting && !recording;

  const handleSubmit = useCallback(async () => {
    if (!input || !selectedModel) return;
    setSubmitting(true);
    setError(null);
    setResult(null);
    const t0 = performance.now();
    try {
      const resp = await api.transcribe(selectedModel, input.blob, {
        filename: input.filename,
        language: language || undefined,
        translate: translate || undefined,
        prompt: prompt || undefined,
        token: token || undefined,
      });
      const wallMs = performance.now() - t0;
      setResult({
        response: resp,
        wallMs,
        model: selectedModel,
        multilingual,
        translated: !!translate,
        sourceHint: language,
        inputName: input.filename,
        inputSize: input.size,
        inputSource: input.source,
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }, [input, selectedModel, language, translate, prompt, token, multilingual]);

  const handleClear = useCallback(() => {
    replaceInput(null);
    setResult(null);
    setError(null);
  }, [replaceInput]);

  // ---------------------------------------------------------------- render

  const wordCount = result ? result.response.text.trim().split(/\s+/).filter(Boolean).length : 0;
  const charCount = result ? result.response.text.length : 0;
  const rtf = result && result.wallMs > 0
    ? (result.response.duration * 1000) / result.wallMs
    : 0;

  return (
    <div className="translator-page">
      <div className="page-header">
        <h2>Translator</h2>
        <p>
          Transcribe speech to text or translate non-English speech into English using
          whisper.cpp. Upload an audio file or record up to {MAX_RECORD_SEC} seconds from
          your microphone.
        </p>
      </div>

      {modelsError && (
        <div className="alert alert-error">Failed to load whisper models: {modelsError}</div>
      )}

      {!modelsLoading && models.length === 0 && (
        <div className="alert alert-error">
          No whisper models installed. Go to <strong>Bucky → Models → List</strong> to download one.
        </div>
      )}

      <div className="translator-grid">
        <section className="translator-card">
          <h3>Settings</h3>

          <FieldLabel htmlFor="translator-model" tooltipKey="translatorModel">
            Model
          </FieldLabel>
          <select
            id="translator-model"
            className="form-select"
            value={selectedModel}
            onChange={(e) => setSelectedModel(e.target.value)}
            disabled={modelsLoading || models.length === 0}
          >
            {modelsLoading && <option>Loading…</option>}
            {!modelsLoading && models.length === 0 && <option>No models available</option>}
            {models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.id}{isEnglishOnly(m.id) ? ' (English-only)' : ''}
              </option>
            ))}
          </select>

          <FieldLabel htmlFor="translator-language" tooltipKey="translatorSourceLanguage">
            Source language
          </FieldLabel>
          <select
            id="translator-language"
            className="form-select"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            disabled={!multilingual}
          >
            {LANGUAGES.map((l) => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>

          <FieldLabel tooltipKey="translatorTranslate">
            <input
              type="checkbox"
              checked={translate}
              onChange={(e) => setTranslate(e.target.checked)}
              disabled={!multilingual}
            />{' '}
            Translate to English
          </FieldLabel>

          <FieldLabel htmlFor="translator-prompt" tooltipKey="translatorPrompt">
            Decoder prompt (optional)
          </FieldLabel>
          <textarea
            id="translator-prompt"
            className="translator-textarea"
            rows={3}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="e.g. proper nouns or technical terms to bias spelling"
          />
        </section>

        <section className="translator-card">
          <h3>Audio input</h3>

          <div
            className={`translator-drop ${dragOver ? 'translator-drop-over' : ''}`}
            onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            onClick={() => fileInputRef.current?.click()}
          >
            <div className="translator-drop-icon">🎵</div>
            <div>Drop audio file here or click to choose</div>
            <input
              ref={fileInputRef}
              type="file"
              accept="audio/*"
              style={{ display: 'none' }}
              onChange={onFileChange}
            />
          </div>

          <div className="translator-mic-row">
            {!recording ? (
              <button
                type="button"
                className="btn btn-secondary"
                onClick={startRecording}
                disabled={submitting}
              >
                ● Record from microphone
              </button>
            ) : (
              <button type="button" className="btn btn-primary" onClick={stopRecording}>
                ■ Stop recording ({recordSec.toFixed(1)}s / {MAX_RECORD_SEC}s)
              </button>
            )}
          </div>

          {input && (
            <div className="translator-input-preview">
              <div className="translator-input-meta">
                <strong>{input.filename}</strong>
                <span className="translator-input-source">
                  {input.source === 'mic' ? 'microphone' : 'file'}
                </span>
                <span>{fmtBytes(input.size)}</span>
                <button type="button" className="btn-link" onClick={handleClear}>
                  remove
                </button>
              </div>
              <audio src={input.objectURL} controls className="translator-audio" />
            </div>
          )}

          <div className="translator-submit-row">
            <button
              type="button"
              className="btn btn-primary"
              onClick={handleSubmit}
              disabled={!canSubmit}
            >
              {submitting ? 'Working…' : translate ? 'Translate' : 'Transcribe'}
            </button>
          </div>

          {error && <div className="alert alert-error">{error}</div>}
        </section>
      </div>

      {result && (
        <section className="translator-card translator-result">
          <div className="translator-result-header">
            <h3>{result.translated ? 'Translation' : 'Transcription'}</h3>
            <div className="translator-result-actions">
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => navigator.clipboard.writeText(result.response.text)}
              >
                Copy
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => downloadBlob('transcript.txt', result.response.text, 'text/plain')}
              >
                .txt
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => downloadBlob('transcript.srt', buildSRT(result.response), 'application/x-subrip')}
              >
                .srt
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => downloadBlob('transcript.vtt', buildVTT(result.response), 'text/vtt')}
              >
                .vtt
              </button>
            </div>
          </div>

          <pre className="translator-text">{result.response.text || '(empty)'}</pre>

          <table className="translator-meta">
            <tbody>
              <tr><td>Model</td><td>{result.model}{result.multilingual ? ' (multilingual)' : ' (English-only)'}</td></tr>
              <tr><td>Mode</td><td>{result.translated ? 'translate → English' : 'transcribe (source language)'}</td></tr>
              <tr><td>Source hint</td><td>{result.sourceHint || 'auto-detect'}</td></tr>
              <tr><td>Detected language</td><td>{result.response.language || '(unknown)'}</td></tr>
              <tr><td>Audio duration</td><td>{fmtSeconds(result.response.duration)}</td></tr>
              <tr><td>Wall time</td><td>{fmtSeconds(result.wallMs / 1000)}</td></tr>
              <tr>
                <td>{labelWithTip('Realtime factor', 'translatorRealtimeFactor')}</td>
                <td>{rtf > 0 ? `${rtf.toFixed(2)}×` : '—'}</td>
              </tr>
              <tr><td>Segments</td><td>{result.response.segments.length}</td></tr>
              <tr><td>Words / chars</td><td>{wordCount} / {charCount}</td></tr>
              <tr><td>Input</td><td>{result.inputName} · {fmtBytes(result.inputSize)} · {result.inputSource}</td></tr>
            </tbody>
          </table>

          <button
            type="button"
            className="btn-link"
            onClick={() => setShowSegments((v) => !v)}
          >
            {showSegments ? '▼' : '▶'} Segments ({result.response.segments.length})
          </button>
          {showSegments && (
            <table className="translator-segments">
              <thead>
                <tr>
                  <th>#</th>
                  <th>Start</th>
                  <th>End</th>
                  <th>Text</th>
                  <th>{labelWithTip('No-speech', 'translatorNoSpeechProb')}</th>
                </tr>
              </thead>
              <tbody>
                {result.response.segments.map((s) => (
                  <tr key={s.id}>
                    <td>{s.id}</td>
                    <td>{fmtSeconds(s.start)}</td>
                    <td>{fmtSeconds(s.end)}</td>
                    <td>{s.text.trim()}</td>
                    <td>{s.no_speech_prob.toFixed(3)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}
    </div>
  );
}
