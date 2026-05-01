import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import type { VRAMCalculatorResponse, DeviceInfo, PerDeviceVRAM, VRAMRequest } from '../../types';
import { api } from '../../services/api';
import { useDevicesInfo } from './devices';

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
  /** Local model id to use for incremental recomputes (model details tab). */
  modelId?: string;
  /** Bearer token for authenticated catalog/standalone calculator calls. */
  authToken?: string;
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
}

export interface VRAMResultsState {
  vramResult: VRAMResultView;
  input: VRAMCalculatorResponse['input'];
  moe: VRAMCalculatorResponse['moe'];
  weights: VRAMCalculatorResponse['weights'];
  gpuLayers: number;
  expertLayersOnGPU: number;
  kvCacheOnCPU: boolean;
  perDevice: PerDeviceVRAM[] | undefined;
  deviceCount: number;
  systemRAMBytes: number | undefined;
  gpuTotalBytes: number;
  gpuDevices: DeviceInfo[];
  tensorSplit: string;
  isHardwareOverridden: boolean;
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
  };
}

export default function useVRAMState(opts: UseVRAMStateOptions = {}) {
  const {
    initialContextWindow = 32768,
    initialBytesPerElement = 1,
    initialSlots = 1,
    serverResponse,
    enableHardwareOverrides = false,
    modelUrl,
    modelId,
    authToken,
  } = opts;

  // ── Control state ────────────────────────────────────────────────────────
  const [contextWindow, setContextWindow] = useState(initialContextWindow);
  const [bytesPerElement, setBytesPerElement] = useState(initialBytesPerElement);
  const [slots, setSlots] = useState(initialSlots);
  const [gpuLayers, setGpuLayers] = useState(0);
  const [expertLayersOnGPU, setExpertLayersOnGPU] = useState(0);
  const [kvCacheOnCPU, setKvCacheOnCPU] = useState(false);
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

  const effectiveGpuTotalBytes = useMemo(() => {
    if (gpuMemoryOverrideBytes != null) return gpuMemoryOverrideBytes;
    if (enableHardwareOverrides && deviceCountOverride != null && detectedGpuCount && detectedGpuCount > 0) {
      const perGpu = detectedGpuTotalBytes / detectedGpuCount;
      return Math.round(perGpu * deviceCountOverride);
    }
    return detectedGpuTotalBytes;
  }, [gpuMemoryOverrideBytes, enableHardwareOverrides, deviceCountOverride, detectedGpuCount, detectedGpuTotalBytes]);

  const effectiveSystemRAMBytes = systemMemoryOverrideBytes ?? detectedSystemRAMBytes;

  const effectiveGpuDevices: DeviceInfo[] = useMemo(() => {
    if (!hasGpuOverrides) {
      return detectedGpuDevices.slice(0, effectiveDeviceCount);
    }
    const perGpu = effectiveDeviceCount > 0 ? Math.floor(effectiveGpuTotalBytes / effectiveDeviceCount) : 0;
    return Array.from({ length: effectiveDeviceCount }, (_, i) => ({
      index: i,
      name: `GPU ${i}`,
      type: 'gpu_cuda' as const,
      free_bytes: perGpu,
      total_bytes: perGpu,
    }));
  }, [hasGpuOverrides, detectedGpuDevices, effectiveDeviceCount, effectiveGpuTotalBytes]);

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
    const input = serverResponse.input;
    if (input) {
      setContextWindow(input.context_window);
      setBytesPerElement(input.bytes_per_element);
      setSlots(input.slots);
      setGpuLayers(input.block_count ?? 0);
      setExpertLayersOnGPU(input.block_count ?? 0);
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
    if (!modelUrl && !modelId) return;
    if (!liveResponse && !serverResponse) return;

    // The seed effect above sets contextWindow/bpe/slots/gpuLayers from the
    // serverResponse. Skip the first recompute so we don't immediately
    // re-fetch the same values.
    if (!initialSeededRef.current) {
      initialSeededRef.current = true;
      return;
    }

    const id = ++latestRequestIdRef.current;

    const handle = setTimeout(async () => {
      const req: VRAMRequest = {
        model_url: modelUrl,
        model_id: modelId,
        context_window: contextWindow,
        bytes_per_element: bytesPerElement,
        slots,
        gpu_layers: gpuLayers,
        expert_layers_on_gpu: expertLayersOnGPU,
        kv_cache_on_cpu: kvCacheOnCPU,
        device_count: effectiveDeviceCount,
        tensor_split: parsedTensorSplit.length > 0 ? parsedTensorSplit : undefined,
      };
      try {
        const resp = await api.calculateVRAM(req, authToken);
        if (latestRequestIdRef.current === id) {
          setLiveResponse(resp);
        }
      } catch {
        // Ignore transient errors; the previous result remains visible.
      }
    }, RECOMPUTE_DEBOUNCE_MS);

    return () => clearTimeout(handle);
    // We intentionally do not depend on serverResponse here; that is handled
    // by the seed effect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    modelUrl, modelId, authToken,
    contextWindow, bytesPerElement, slots,
    gpuLayers, expertLayersOnGPU, kvCacheOnCPU,
    effectiveDeviceCount, parsedTensorSplit,
  ]);

  // ── Auto-fit (server-side single call) ──────────────────────────────────
  const autoFitKeyRef = useRef('');
  useEffect(() => {
    if (!modelUrl && !modelId) return;
    if (!liveResponse && !serverResponse) return;
    if (effectiveGpuDevices.length === 0 && effectiveDeviceCount <= 0) return;

    const fitDeviceCount = Math.max(1, effectiveDeviceCount);
    const selectedGpuDevices = effectiveGpuDevices.slice(0, fitDeviceCount);
    const baseInput = (liveResponse ?? serverResponse)!.input;
    const fitKey = `${baseInput.block_count}|${contextWindow}|${bytesPerElement}|${slots}|${kvCacheOnCPU}|${fitDeviceCount}|${effectiveGpuTotalBytes}|${selectedGpuDevices.map(d => d.free_bytes).join(',')}|${effectiveSystemRAMBytes ?? 0}|${parsedTensorSplit.join(',')}`;
    if (fitKey === autoFitKeyRef.current) return;
    autoFitKeyRef.current = fitKey;

    const blockCount = baseInput.block_count;
    if (!blockCount || blockCount <= 0) return;

    const hasPerGpuInfo = selectedGpuDevices.length === fitDeviceCount && selectedGpuDevices.length > 0;
    const combinedFreeBytes = hasPerGpuInfo
      ? selectedGpuDevices.reduce((sum, d) => sum + d.free_bytes, 0)
      : effectiveGpuTotalBytes;
    if (combinedFreeBytes <= 0) return;

    const id = ++latestRequestIdRef.current;
    const req: VRAMRequest = {
      model_url: modelUrl,
      model_id: modelId,
      context_window: contextWindow,
      bytes_per_element: bytesPerElement,
      slots,
      kv_cache_on_cpu: kvCacheOnCPU,
      device_count: fitDeviceCount,
      tensor_split: parsedTensorSplit.length > 0 ? parsedTensorSplit : undefined,
      auto_fit: true,
      gpu_free_bytes: hasPerGpuInfo ? selectedGpuDevices.map(d => d.free_bytes) : undefined,
      system_ram_bytes: effectiveSystemRAMBytes,
    };

    api.calculateVRAM(req, authToken)
      .then(resp => {
        if (latestRequestIdRef.current !== id) return;
        setLiveResponse(resp);
        // Apply the server's chosen offload values as new control state so
        // subsequent recomputes start from the auto-fit baseline.
        setGpuLayers(resp.input.gpu_layers ?? 0);
        setExpertLayersOnGPU(resp.input.expert_layers_on_gpu ?? 0);
      })
      .catch(() => { /* ignore */ });
  }, [
    modelUrl, modelId, authToken,
    liveResponse, serverResponse,
    effectiveDeviceCount, effectiveGpuTotalBytes, effectiveGpuDevices, effectiveSystemRAMBytes,
    contextWindow, bytesPerElement, slots, kvCacheOnCPU, parsedTensorSplit,
  ]);

  // ── Derived view ────────────────────────────────────────────────────────
  const activeResponse = liveResponse ?? serverResponse ?? null;
  const vramInput = activeResponse?.input;
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
  };

  const resultsProps: VRAMResultsState | null = vramResult && vramInput ? {
    vramResult,
    input: vramInput,
    moe: activeResponse?.moe,
    weights: activeResponse?.weights,
    gpuLayers,
    expertLayersOnGPU,
    kvCacheOnCPU,
    perDevice,
    deviceCount: effectiveDeviceCount,
    systemRAMBytes: effectiveSystemRAMBytes,
    gpuTotalBytes: effectiveGpuTotalBytes,
    gpuDevices: effectiveGpuDevices,
    tensorSplit,
    isHardwareOverridden,
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
