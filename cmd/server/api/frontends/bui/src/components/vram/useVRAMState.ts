import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import type { VRAMCalculatorResponse, DeviceInfo } from '../../types';
import { calculateVRAM, calculatePerDeviceVRAM } from './calculate';
import type { VRAMResult } from './calculate';
import { useDevicesInfo } from './devices';

export interface UseVRAMStateOptions {
  initialContextWindow?: number;
  initialBytesPerElement?: number;
  initialSlots?: number;
  /** When provided, the hook seeds controls from this response (used by embedded views). */
  serverResponse?: VRAMCalculatorResponse | null;
  /** When true, enables custom GPU memory, system RAM, and GPU count overrides (standalone calculator only). */
  enableHardwareOverrides?: boolean;
}

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

export interface VRAMResultsState {
  vramResult: VRAMResult;
  input: ReturnType<typeof mergedInput>;
  moe: VRAMCalculatorResponse['moe'];
  weights: VRAMCalculatorResponse['weights'];
  gpuLayers: number;
  expertLayersOnGPU: number;
  kvCacheOnCPU: boolean;
  perDevice: ReturnType<typeof calculatePerDeviceVRAM> | undefined;
  deviceCount: number;
  systemRAMBytes: number | undefined;
  gpuTotalBytes: number;
  gpuDevices: DeviceInfo[];
  tensorSplit: string;
  isHardwareOverridden: boolean;
}

function mergedInput(
  base: VRAMCalculatorResponse['input'],
  ctx: number,
  bpe: number,
  slots: number,
) {
  return { ...base, context_window: ctx, bytes_per_element: bpe, slots };
}

export default function useVRAMState(opts: UseVRAMStateOptions = {}) {
  const {
    initialContextWindow = 32768,
    initialBytesPerElement = 1,
    initialSlots = 1,
    serverResponse,
    enableHardwareOverrides = false,
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

  // ── Auto-fit: per-GPU capacity check ─────────────────────────────────────
  // Track which inputs were used for the last auto-fit so we re-run when any
  // parameter that affects fit (context window, bpe, slots, kvCacheOnCPU,
  // deviceCount) changes, but avoid infinite loops by comparing the computed key.
  const autoFitKeyRef = useRef('');
  useEffect(() => {
    if (!serverResponse) return;
    if (effectiveGpuDevices.length === 0 && effectiveDeviceCount <= 0) return;

    const fitDeviceCount = Math.max(1, effectiveDeviceCount);
    const selectedGpuDevices = effectiveGpuDevices.slice(0, fitDeviceCount);

    const fitKey = `${serverResponse.input.block_count}|${contextWindow}|${bytesPerElement}|${slots}|${kvCacheOnCPU}|${fitDeviceCount}|${effectiveGpuTotalBytes}|${effectiveDeviceCount}|${selectedGpuDevices.map(d => d.free_bytes).join(',')}|${effectiveSystemRAMBytes ?? 0}|${parsedTensorSplit.join(',')}`;
    if (fitKey === autoFitKeyRef.current) return;
    autoFitKeyRef.current = fitKey;

    const blockCount = serverResponse.input.block_count;
    if (!blockCount || blockCount <= 0) return;

    const isMoEResult = serverResponse.moe?.is_moe === true && serverResponse.weights != null;

    // Determine available capacity using only the selected GPUs.
    const hasPerGpuInfo = selectedGpuDevices.length === fitDeviceCount && selectedGpuDevices.length > 0;
    const combinedFreeBytes = hasPerGpuInfo
      ? selectedGpuDevices.reduce((sum, d) => sum + d.free_bytes, 0)
      : effectiveGpuTotalBytes;

    if (combinedFreeBytes <= 0) {
      // No GPU capacity detected — keep everything on GPU (the seeded defaults).
      return;
    }

    const input = { ...serverResponse.input, context_window: contextWindow, bytes_per_element: bytesPerElement, slots };

    const fitsInHardware = (v: ReturnType<typeof calculateVRAM>) => {
      let fitsGpu: boolean;
      if (hasPerGpuInfo && fitDeviceCount > 1) {
        const perDev = calculatePerDeviceVRAM(v.modelWeightsGPU, v.kvVramBytes, v.computeBufferEst, fitDeviceCount, parsedTensorSplit);
        fitsGpu = perDev.every((dev, i) => {
          const cap = selectedGpuDevices[i]?.free_bytes ?? 0;
          return cap > 0 && dev.totalBytes <= cap * 0.95;
        });
      } else {
        const singleCap = hasPerGpuInfo
          ? (selectedGpuDevices[0]?.free_bytes ?? 0)
          : combinedFreeBytes;
        fitsGpu = singleCap > 0 && v.totalVram <= singleCap * 0.95;
      }

      const fitsRam = !effectiveSystemRAMBytes || effectiveSystemRAMBytes <= 0
        ? true
        : (v.totalSystemRamEst ?? 0) <= effectiveSystemRAMBytes * 0.95;

      return fitsGpu && fitsRam;
    };

    if (isMoEResult) {
      // MoE auto-fit: try expert offloading first (all layers on GPU,
      // maximize expert layers). Falls back to layer offloading if the
      // always-active weights alone don't fit.
      let best = { ngl: 0, experts: 0 };

      // Expert offloading: gpuLayers = blockCount, find max expertLayersOnGPU.
      let bestExperts = -1;
      for (let experts = blockCount; experts >= 0; experts--) {
        const v = calculateVRAM(input, { weights: serverResponse.weights, moe: serverResponse.moe, gpuLayers: blockCount, expertLayersOnGPU: experts, kvCacheOnCPU });
        if (fitsInHardware(v)) {
          bestExperts = experts;
          break;
        }
      }

      if (bestExperts >= 0) {
        best = { ngl: blockCount, experts: bestExperts };
      } else {
        // Expert offloading can't fit even with 0 experts — fall back to
        // layer offloading where expert layers follow GPU layers.
        for (let ngl = blockCount; ngl >= 0; ngl--) {
          const v = calculateVRAM(input, { weights: serverResponse.weights, moe: serverResponse.moe, gpuLayers: ngl, expertLayersOnGPU: ngl, kvCacheOnCPU });
          if (fitsInHardware(v)) {
            best = { ngl, experts: ngl };
            break;
          }
        }
      }

      setGpuLayers(best.ngl);
      setExpertLayersOnGPU(best.experts);
    } else {
      // Dense auto-fit: find maximum gpuLayers that fits.
      let bestGpuLayers = 0;
      for (let ngl = 0; ngl <= blockCount; ngl++) {
        const v = calculateVRAM(input, { gpuLayers: ngl, kvCacheOnCPU });
        if (fitsInHardware(v)) bestGpuLayers = ngl;
      }
      setGpuLayers(bestGpuLayers);
      setExpertLayersOnGPU(0);
    }
  }, [serverResponse, effectiveDeviceCount, effectiveGpuTotalBytes, effectiveGpuDevices, effectiveSystemRAMBytes, contextWindow, bytesPerElement, slots, kvCacheOnCPU, parsedTensorSplit]);

  // ── Derived calculations ─────────────────────────────────────────────────
  const vramInput = serverResponse?.input;
  const isMoE = serverResponse?.moe?.is_moe === true && serverResponse?.weights != null;

  const vramResult = useMemo(() => {
    if (!vramInput) return null;
    return calculateVRAM(
      { ...vramInput, context_window: contextWindow, bytes_per_element: bytesPerElement, slots },
      {
        weights: serverResponse?.weights ?? null,
        moe: serverResponse?.moe ?? null,
        gpuLayers,
        expertLayersOnGPU,
        kvCacheOnCPU,
      },
    );
  }, [vramInput, contextWindow, bytesPerElement, slots, gpuLayers, expertLayersOnGPU, kvCacheOnCPU, serverResponse?.weights, serverResponse?.moe]);

  const perDevice = useMemo(() => {
    if (!vramResult) return undefined;
    return calculatePerDeviceVRAM(vramResult.modelWeightsGPU, vramResult.kvVramBytes, vramResult.computeBufferEst, effectiveDeviceCount, parsedTensorSplit);
  }, [vramResult, effectiveDeviceCount, parsedTensorSplit]);

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
    input: mergedInput(vramInput, contextWindow, bytesPerElement, slots),
    moe: serverResponse?.moe,
    weights: serverResponse?.weights,
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
