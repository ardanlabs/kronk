import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import type { VRAMCalculatorResponse, DeviceInfo, PerDeviceVRAM, VRAMRequest, FitAssessment } from '../../types';
import { api } from '../../services/api';
import { useDevicesInfo } from './devices';
import { EXPERTS_ALL_ON_GPU } from './constants';

export interface UseVRAMStateOptions {
  initialContextWindow?: number;
  initialBytesPerElement?: number;
  initialSlots?: number;
  /** When provided, the hook seeds controls from this response (used by embedded views). */
  serverResponse?: VRAMCalculatorResponse | null;
  /** When true, enables custom GPU memory, system RAM, and GPU count overrides (standalone calculator only). */
  enableHardwareOverrides?: boolean;
  /** HuggingFace URL to use for incremental recomputes (catalog / standalone). */
  modelUrl?: string;
  /** Resolved HuggingFace file URLs to use for split-model recomputes. */
  modelUrls?: string[];
  /** Local model id to use for incremental recomputes (model details tab). */
  modelId?: string;
}

/** Debounce window for slider/input changes that trigger a server recompute. */
const RECOMPUTE_DEBOUNCE_MS = 150;

/** Parse a GB string input into bytes, returning undefined for empty/invalid. */
function parseGBToBytes(value: string): number | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const n = parseFloat(trimmed);
  if (isNaN(n) || n <= 0) return undefined;
  return Math.round(n * 1024 * 1024 * 1024);
}

function isInvalidGBInput(value: string): boolean {
  const trimmed = value.trim();
  if (!trimmed) return false;
  const n = parseFloat(trimmed);
  return isNaN(n) || n <= 0;
}

function gpuLayersForControl(gpuLayers: number, blockCount: number): number {
  if (gpuLayers === -1) return 0;
  if (gpuLayers === 0) return blockCount;
  return gpuLayers;
}

export interface VRAMControlsState {
  contextWindow: number;
  onContextWindowChange: (v: number) => void;
  bytesPerElement: number;
  onBytesPerElementChange: (v: number) => void;
  slots: number;
  onSlotsChange: (v: number) => void;
  maxDeviceCount: number | undefined;
  isMoE: boolean;
  blockCount: number | undefined;
  gpuLayers: number;
  onGpuLayersChange: (v: number) => void;
  expertLayersOnGPU: number;
  onExpertLayersOnGPUChange: (v: number) => void;
  kvCacheOnCPU: boolean;
  onKvCacheOnCPUChange: (v: boolean) => void;
  hasSWA: boolean;
  swaFull: boolean;
  onSwaFullChange: (v: boolean) => void;
  deviceCount: number;
  onDeviceCountChange: (v: number) => void;
  tensorSplit: string;
  onTensorSplitChange: (v: string) => void;
  showHardwareOverrides: boolean;
  gpuMemoryOverrideGB: string;
  onGpuMemoryOverrideGBChange: (v: string) => void;
  gpuMemoryOverrideInvalid: boolean;
  systemMemoryOverrideGB: string;
  onSystemMemoryOverrideGBChange: (v: string) => void;
  systemMemoryOverrideInvalid: boolean;
  deviceCountOverride: number | null;
  onDeviceCountOverrideChange: (v: number | null) => void;
  detectedGpuTotalBytes: number;
  detectedSystemRAMBytes: number | undefined;
  detectedDeviceCount: number | undefined;
  canAutoFit: boolean;
  autoFitDisabled: boolean;
  autoFitting: boolean;
  autoFitError: string | null;
  onAutoFit: () => void;
}

/**
 * Mirror of the Go vram.Result fields the UI consumes. Values come straight
 * from the /v1/models/vram response so the FE never recomputes the math.
 */
export interface VRAMResultView {
  totalVram: number;
  slotMemory: number;
  kvPerSlot: number;
  kvPerTokenPerLayer: number;
  modelWeightsGPU: number;
  modelWeightsCPU: number;
  computeBufferEst: number;
  alwaysActiveGPUBytes: number;
  alwaysActiveCPUBytes: number;
  expertGPUBytes: number;
  expertCPUBytes: number;
  kvVramBytes: number;
  kvCpuBytes: number;
  totalSystemRamEst: number;
  unifiedFootprint: number;
}

export interface VRAMResultsState {
  vramResult: VRAMResultView;
  input: VRAMCalculatorResponse['input'];
  moe: VRAMCalculatorResponse['moe'];
  weights: VRAMCalculatorResponse['weights'];
  gpuLayers: number;
  expertLayersOnGPU: number;
  kvCacheOnCPU: boolean;
  swaFull: boolean;
  perDevice: PerDeviceVRAM[] | undefined;
  deviceCount: number;
  systemRAMBytes: number | undefined;
  gpuTotalBytes: number;
  gpuDevices: DeviceInfo[];
  tensorSplit: string;
  isHardwareOverridden: boolean;
  isUnifiedMemory: boolean;
  fitAssessment: FitAssessment | undefined;
  /** True while a debounced recompute is in flight after a control change. */
  recomputing: boolean;
}

function viewFromResponse(resp: VRAMCalculatorResponse): VRAMResultView {
  return {
    totalVram: resp.total_vram,
    slotMemory: resp.slot_memory,
    kvPerSlot: resp.kv_per_slot,
    kvPerTokenPerLayer: resp.kv_per_token_per_layer,
    modelWeightsGPU: resp.model_weights_gpu ?? 0,
    modelWeightsCPU: resp.model_weights_cpu ?? 0,
    computeBufferEst: resp.compute_buffer_est ?? 0,
    alwaysActiveGPUBytes: resp.always_active_gpu_bytes ?? 0,
    alwaysActiveCPUBytes: resp.always_active_cpu_bytes ?? 0,
    expertGPUBytes: resp.expert_gpu_bytes ?? 0,
    expertCPUBytes: resp.expert_cpu_bytes ?? 0,
    kvVramBytes: resp.kv_vram_bytes ?? 0,
    kvCpuBytes: resp.kv_cpu_bytes ?? 0,
    totalSystemRamEst: resp.total_system_ram_est ?? 0,
    unifiedFootprint: resp.unified_footprint,
  };
}

export default function useVRAMState(opts: UseVRAMStateOptions = {}) {
  const {
    initialContextWindow = 131072,
    initialBytesPerElement = 2,
    initialSlots = 1,
    serverResponse,
    enableHardwareOverrides = false,
    modelUrl,
    modelUrls,
    modelId,
  } = opts;

  // ── Control state ────────────────────────────────────────────────────────
  const [contextWindow, setContextWindow] = useState(initialContextWindow);
  const [bytesPerElement, setBytesPerElement] = useState(initialBytesPerElement);
  const [slots, setSlots] = useState(initialSlots);
  const [gpuLayers, setGpuLayers] = useState(0);
  // Default to the "all experts on GPU" sentinel so the first request
  // mirrors the runtime baseline. After the first response the seed
  // effect below replaces this with the actual block_count, and the
  // slider then drives real 0..blockCount values.
  const [expertLayersOnGPU, setExpertLayersOnGPU] = useState<number>(EXPERTS_ALL_ON_GPU);
  const [kvCacheOnCPU, setKvCacheOnCPU] = useState(false);
  const [swaFull, setSwaFull] = useState(true);
  const [deviceCount, setDeviceCount] = useState(1);
  const [tensorSplit, setTensorSplit] = useState('');

  // ── Hardware override state (standalone calculator only) ───────────────
  const [gpuMemoryOverrideGB, setGpuMemoryOverrideGB] = useState('');
  const [systemMemoryOverrideGB, setSystemMemoryOverrideGB] = useState('');
  const [deviceCountOverride, setDeviceCountOverride] = useState<number | null>(null);

  // ── Device info (shared hook) ──────────────────────────────────────────
  const devInfo = useDevicesInfo();
  const detectedGpuCount = devInfo?.gpuCount;
  const detectedGpuTotalBytes = devInfo?.gpuVramBytes ?? 0;
  const detectedSystemRAMBytes = devInfo?.ramBytes;
  const detectedGpuDevices = devInfo?.gpuDevices ?? [];

  // ── Effective hardware (overrides take precedence) ─────────────────────
  const gpuMemoryOverrideBytes = enableHardwareOverrides ? parseGBToBytes(gpuMemoryOverrideGB) : undefined;
  const systemMemoryOverrideBytes = enableHardwareOverrides ? parseGBToBytes(systemMemoryOverrideGB) : undefined;
  const effectiveDeviceCount = (enableHardwareOverrides && deviceCountOverride != null)
    ? deviceCountOverride
    : deviceCount;
  const hasGpuOverrides = enableHardwareOverrides && (
    gpuMemoryOverrideBytes != null || deviceCountOverride != null
  );
  const isHardwareOverridden = hasGpuOverrides || (
    enableHardwareOverrides && systemMemoryOverrideBytes != null
  );

  const effectiveGpuTotalBytes = gpuMemoryOverrideBytes ?? detectedGpuTotalBytes;

  const effectiveSystemRAMBytes = systemMemoryOverrideBytes ?? detectedSystemRAMBytes;

  const effectiveGpuDevices: DeviceInfo[] = useMemo(() => {
    if (gpuMemoryOverrideBytes != null) return [];
    return detectedGpuDevices.slice(0, effectiveDeviceCount);
  }, [gpuMemoryOverrideBytes, detectedGpuDevices, effectiveDeviceCount]);
  const isUnifiedMemory = !hasGpuOverrides && devInfo?.unifiedMemory === true;

  // Track whether the user has manually changed the GPU count so we don't
  // overwrite their selection when device info updates.
  const userSetDeviceCountRef = useRef(false);

  const handleDeviceCountChange = useCallback((v: number) => {
    userSetDeviceCountRef.current = true;
    setDeviceCount(v);
  }, []);

  useEffect(() => {
    if (!devInfo || devInfo.gpuCount <= 0) return;
    if (enableHardwareOverrides && deviceCountOverride != null) return;
    setDeviceCount(prev => {
      if (!userSetDeviceCountRef.current) return devInfo.gpuCount;
      return Math.min(Math.max(1, prev), devInfo.gpuCount);
    });
  }, [devInfo, enableHardwareOverrides, deviceCountOverride]);

  // ── Seed from server response (embedded views) ───────────────────────────
  const prevResponseRef = useRef<VRAMCalculatorResponse | null>(null);
  useEffect(() => {
    if (!serverResponse || serverResponse === prevResponseRef.current) return;
    prevResponseRef.current = serverResponse;
    setAutoFitError(null);
    const input = serverResponse.input;
    if (input) {
      setContextWindow(input.context_window);
      setBytesPerElement(input.bytes_per_element);
      setSlots(input.slots);
      setGpuLayers(gpuLayersForControl(input.gpu_layers, input.block_count));
      setExpertLayersOnGPU(input.expert_layers_on_gpu);
      setSwaFull(input.swa_full ?? true);
    }
  }, [serverResponse]);

  const parsedTensorSplit = useMemo(() => {
    if (!tensorSplit) return [];
    return tensorSplit.split(',').map(s => parseFloat(s.trim())).filter(n => !isNaN(n));
  }, [tensorSplit]);

  // ── Server-driven recompute (debounced) ─────────────────────────────────
  // Latest response from /v1/models/vram for the current control values.
  // serverResponse provides the initial value; everything after that is
  // produced by the recompute effect below.
  const [liveResponse, setLiveResponse] = useState<VRAMCalculatorResponse | null>(serverResponse ?? null);

  // recomputing is true between the moment a control changes and the
  // moment the matching server response lands. The BUI uses this to
  // dim the VRAM total / show a small spinner so the user knows the
  // displayed number lags the slider by a network round-trip.
  const [recomputing, setRecomputing] = useState(false);
  const [autoFitting, setAutoFitting] = useState(false);
  const [autoFitError, setAutoFitError] = useState<string | null>(null);

  useEffect(() => {
    if (serverResponse) setLiveResponse(serverResponse);
  }, [serverResponse]);

  // Track the most recent request payload so we can ignore stale responses
  // when the user changes inputs faster than the network round trip.
  const latestRequestIdRef = useRef(0);

  // Skip the very first recompute when serverResponse already provides the
  // initial values for the current controls.
  const initialSeededRef = useRef(false);

  useEffect(() => {
    // Need a model identifier to recompute.
    if (!modelUrl && !modelId && !modelUrls?.length) return;
    if (!liveResponse && !serverResponse) return;

    // A seed response with a hardware assessment is already complete. Older
    // seed responses need one recompute so Go can provide that assessment.
    if (!initialSeededRef.current) {
      initialSeededRef.current = true;
      if (serverResponse?.fit_assessment) return;
    }

    setAutoFitError(null);
    const id = ++latestRequestIdRef.current;
    setRecomputing(true);

    const handle = setTimeout(async () => {
      const selectedGpuDevices = effectiveGpuDevices.slice(0, effectiveDeviceCount);
      const hasPerGpuInfo = selectedGpuDevices.length === effectiveDeviceCount && selectedGpuDevices.length > 0;
      const req: VRAMRequest = {
        model_url: modelUrl,
        model_urls: modelUrls,
        model_id: modelId,
        context_window: contextWindow,
        bytes_per_element: bytesPerElement,
        slots,
        gpu_layers: gpuLayers === 0 ? -1 : gpuLayers,
        expert_layers_on_gpu: expertLayersOnGPU,
        kv_cache_on_cpu: kvCacheOnCPU,
        swa_full: swaFull,
        device_count: effectiveDeviceCount,
        tensor_split: parsedTensorSplit.length > 0 ? parsedTensorSplit : undefined,
        gpu_free_bytes: hasPerGpuInfo ? selectedGpuDevices.map(device => device.free_bytes) : undefined,
        gpu_capacity_bytes: effectiveGpuTotalBytes,
        system_ram_bytes: effectiveSystemRAMBytes,
        unified_memory: isUnifiedMemory,
      };
      try {
        const resp = await api.calculateVRAM(req);
        if (latestRequestIdRef.current === id) {
          setLiveResponse(resp);
          setRecomputing(false);
        }
      } catch {
        // Ignore transient errors; the previous result remains visible.
        if (latestRequestIdRef.current === id) {
          setRecomputing(false);
        }
      }
    }, RECOMPUTE_DEBOUNCE_MS);

    return () => clearTimeout(handle);
    // We intentionally do not depend on serverResponse here; that is handled
    // by the seed effect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    modelUrl, modelUrls, modelId,
    contextWindow, bytesPerElement, slots,
    gpuLayers, expertLayersOnGPU, kvCacheOnCPU, swaFull,
    effectiveDeviceCount, parsedTensorSplit, effectiveGpuDevices,
    effectiveGpuTotalBytes, effectiveSystemRAMBytes, isUnifiedMemory,
  ]);

  // ── Explicit auto-fit ───────────────────────────────────────────────────
  const handleAutoFit = useCallback(async () => {
    const source = liveResponse ?? serverResponse;
    if (recomputing || !source || (!modelUrl && !modelId && !modelUrls?.length)) return;

    const blockCount = source.input.block_count;
    if (!blockCount || blockCount <= 0) return;

    setAutoFitError(null);

    const fitDeviceCount = Math.max(1, effectiveDeviceCount);
    const selectedGpuDevices = effectiveGpuDevices.slice(0, fitDeviceCount);
    const hasPerGpuInfo = selectedGpuDevices.length === fitDeviceCount && selectedGpuDevices.length > 0;

    const id = ++latestRequestIdRef.current;
    setAutoFitting(true);
    const req: VRAMRequest = {
      model_url: modelUrl,
      model_urls: modelUrls,
      model_id: modelId,
      context_window: contextWindow,
      bytes_per_element: bytesPerElement,
      slots,
      kv_cache_on_cpu: kvCacheOnCPU,
      swa_full: swaFull,
      device_count: fitDeviceCount,
      tensor_split: parsedTensorSplit.length > 0 ? parsedTensorSplit : undefined,
      auto_fit: true,
      gpu_free_bytes: hasPerGpuInfo ? selectedGpuDevices.map(device => device.free_bytes) : undefined,
      gpu_capacity_bytes: effectiveGpuTotalBytes,
      system_ram_bytes: effectiveSystemRAMBytes,
      unified_memory: isUnifiedMemory,
    };

    try {
      const resp = await api.calculateVRAM(req);
      if (latestRequestIdRef.current !== id) return;

      if (resp.auto_fit_succeeded !== true) {
        setAutoFitError(isUnifiedMemory
          ? 'No placement fits in the available unified memory. The current placement was preserved.'
          : 'No placement fits in the selected GPU and system memory. The current placement was preserved.');
        return;
      }

      setLiveResponse(resp);
      setGpuLayers(gpuLayersForControl(resp.input.gpu_layers, blockCount));
      setExpertLayersOnGPU(resp.input.expert_layers_on_gpu);
    } catch (err) {
      if (latestRequestIdRef.current === id) {
        setAutoFitError(err instanceof Error ? err.message : 'Unable to find a fitting placement.');
      }
    } finally {
      setAutoFitting(false);
    }
  }, [
    liveResponse, serverResponse, modelUrl, modelUrls, modelId,
    recomputing, isUnifiedMemory, effectiveSystemRAMBytes, effectiveDeviceCount,
    effectiveGpuDevices, effectiveGpuTotalBytes, contextWindow,
    bytesPerElement, slots, kvCacheOnCPU, swaFull, parsedTensorSplit,
  ]);

  // ── Derived view ────────────────────────────────────────────────────────
  const activeResponse = liveResponse ?? serverResponse ?? null;
  const vramInput = activeResponse?.input;
  const hasSWA = (vramInput?.sliding_window ?? 0) > 0 && (vramInput?.sliding_window_layers ?? 0) > 0;
  const isMoE = activeResponse?.moe?.is_moe === true && activeResponse?.weights != null;
  const vramResult = useMemo<VRAMResultView | null>(() => {
    if (!activeResponse) return null;
    return viewFromResponse(activeResponse);
  }, [activeResponse]);
  const perDevice = activeResponse?.per_device;

  // ── Public interface ─────────────────────────────────────────────────────
  const controlsProps: VRAMControlsState = {
    contextWindow,
    onContextWindowChange: setContextWindow,
    bytesPerElement,
    onBytesPerElementChange: setBytesPerElement,
    slots,
    onSlotsChange: setSlots,
    maxDeviceCount: detectedGpuCount,
    isMoE,
    blockCount: vramInput?.block_count,
    gpuLayers,
    onGpuLayersChange: setGpuLayers,
    expertLayersOnGPU,
    onExpertLayersOnGPUChange: setExpertLayersOnGPU,
    kvCacheOnCPU,
    onKvCacheOnCPUChange: setKvCacheOnCPU,
    hasSWA,
    swaFull,
    onSwaFullChange: setSwaFull,
    deviceCount: effectiveDeviceCount,
    onDeviceCountChange: handleDeviceCountChange,
    tensorSplit,
    onTensorSplitChange: setTensorSplit,
    showHardwareOverrides: enableHardwareOverrides,
    gpuMemoryOverrideGB,
    onGpuMemoryOverrideGBChange: setGpuMemoryOverrideGB,
    gpuMemoryOverrideInvalid: isInvalidGBInput(gpuMemoryOverrideGB),
    systemMemoryOverrideGB,
    onSystemMemoryOverrideGBChange: setSystemMemoryOverrideGB,
    systemMemoryOverrideInvalid: isInvalidGBInput(systemMemoryOverrideGB),
    deviceCountOverride,
    onDeviceCountOverrideChange: setDeviceCountOverride,
    detectedGpuTotalBytes,
    detectedSystemRAMBytes,
    detectedDeviceCount: detectedGpuCount,
    canAutoFit: Boolean(activeResponse && (modelUrl || modelId || modelUrls?.length)),
    autoFitDisabled: recomputing,
    autoFitting,
    autoFitError,
    onAutoFit: () => { void handleAutoFit(); },
  };

  const resultsProps: VRAMResultsState | null = vramResult && vramInput ? {
    vramResult,
    input: vramInput,
    moe: activeResponse?.moe,
    weights: activeResponse?.weights,
    gpuLayers,
    expertLayersOnGPU,
    kvCacheOnCPU,
    swaFull,
    perDevice,
    deviceCount: effectiveDeviceCount,
    systemRAMBytes: effectiveSystemRAMBytes,
    gpuTotalBytes: effectiveGpuTotalBytes,
    gpuDevices: effectiveGpuDevices,
    tensorSplit,
    isHardwareOverridden,
    isUnifiedMemory,
    fitAssessment: activeResponse?.fit_assessment,
    recomputing,
  } : null;

  return {
    controlsProps,
    resultsProps,
    isMoE,
    maxGpuCount: detectedGpuCount,
    gpuTotalBytes: effectiveGpuTotalBytes,
    systemRAM: effectiveSystemRAMBytes,
    gpuDevices: effectiveGpuDevices,
  };
}
