import { useState, useEffect, useRef, useMemo } from 'react';
import { api } from '../../services/api';
import type { VRAMCalculatorResponse, DeviceInfo } from '../../types';
import { calculateVRAM, calculatePerDeviceVRAM } from './calculate';
import type { VRAMResult } from './calculate';

export interface UseVRAMStateOptions {
  initialContextWindow?: number;
  initialBytesPerElement?: number;
  initialSlots?: number;
  /** When provided, the hook seeds controls from this response (used by embedded views). */
  serverResponse?: VRAMCalculatorResponse | null;
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
  expertLayersOnGPU: number;
  onExpertLayersOnGPUChange: (v: number) => void;
  kvCacheOnCPU: boolean;
  onKvCacheOnCPUChange: (v: boolean) => void;
  deviceCount: number;
  onDeviceCountChange: (v: number) => void;
  tensorSplit: string;
  onTensorSplitChange: (v: string) => void;
}

export interface VRAMResultsState {
  vramResult: VRAMResult;
  input: ReturnType<typeof mergedInput>;
  moe: VRAMCalculatorResponse['moe'];
  weights: VRAMCalculatorResponse['weights'];
  expertLayersOnGPU: number;
  perDevice: ReturnType<typeof calculatePerDeviceVRAM> | undefined;
  deviceCount: number;
  systemRAMBytes: number | undefined;
  gpuTotalBytes: number;
  gpuDevices: DeviceInfo[];
  tensorSplit: string;
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
    initialBytesPerElement = 2,
    initialSlots = 2,
    serverResponse,
  } = opts;

  // ── Control state ────────────────────────────────────────────────────────
  const [contextWindow, setContextWindow] = useState(initialContextWindow);
  const [bytesPerElement, setBytesPerElement] = useState(initialBytesPerElement);
  const [slots, setSlots] = useState(initialSlots);
  const [expertLayersOnGPU, setExpertLayersOnGPU] = useState(0);
  const [kvCacheOnCPU, setKvCacheOnCPU] = useState(false);
  const [deviceCount, setDeviceCount] = useState(1);
  const [tensorSplit, setTensorSplit] = useState('');

  // ── Device info (fetched once) ───────────────────────────────────────────
  const [maxGpuCount, setMaxGpuCount] = useState<number | undefined>(undefined);
  const [gpuTotalBytes, setGpuTotalBytes] = useState(0);
  const [systemRAM, setSystemRAM] = useState<number | undefined>(undefined);
  const [gpuDevices, setGpuDevices] = useState<DeviceInfo[]>([]);

  useEffect(() => {
    let cancelled = false;
    api.getDevices()
      .then((resp) => {
        if (cancelled) return;
        setMaxGpuCount(resp.gpu_count);
        setGpuTotalBytes(resp.gpu_total_bytes);
        setSystemRAM(resp.system_ram_bytes);
        setGpuDevices(resp.devices.filter(d => d.type.startsWith('gpu_')));
        if (resp.gpu_count > 0) {
          setDeviceCount(resp.gpu_count);
        }
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

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
    }
  }, [serverResponse]);

  // ── Auto-fit: per-GPU capacity check ─────────────────────────────────────
  const autoFitAppliedRef = useRef(false);
  useEffect(() => {
    if (!serverResponse || autoFitAppliedRef.current) return;
    if (gpuDevices.length === 0 && maxGpuCount === undefined) return;

    autoFitAppliedRef.current = true;

    const gpuCount = gpuDevices.length || maxGpuCount || 1;
    setDeviceCount(gpuCount);

    const isMoEResult = serverResponse.moe?.is_moe === true && serverResponse.weights != null;
    if (!isMoEResult) {
      setExpertLayersOnGPU(0);
      return;
    }

    const blockCount = serverResponse.input.block_count;
    if (!blockCount || blockCount <= 0) return;

    // Determine available capacity per GPU.
    const hasPerGpuInfo = gpuDevices.length > 0;
    const combinedFreeBytes = hasPerGpuInfo
      ? gpuDevices.reduce((sum, d) => sum + d.free_bytes, 0)
      : gpuTotalBytes;

    if (combinedFreeBytes <= 0) return;

    const input = { ...serverResponse.input, context_window: contextWindow, bytes_per_element: bytesPerElement, slots };
    let bestLayers = 0;

    for (let layers = 0; layers <= blockCount; layers++) {
      const v = calculateVRAM(input, serverResponse.weights, serverResponse.moe, layers, kvCacheOnCPU);

      if (hasPerGpuInfo && gpuCount > 1) {
        // Per-GPU fit check: compute per-device allocation and verify
        // every GPU stays within 95% of its free capacity.
        const perDev = calculatePerDeviceVRAM(v.modelWeightsGPU, v.kvVramBytes, v.computeBufferEst, gpuCount, []);
        const fitsAll = perDev.every((dev, i) => {
          const cap = gpuDevices[i]?.free_bytes ?? 0;
          return cap > 0 ? dev.totalBytes <= cap * 0.95 : true;
        });
        if (fitsAll) {
          bestLayers = layers;
        }
      } else {
        // Fallback: combined VRAM check.
        if (v.totalVram <= combinedFreeBytes * 0.95) {
          bestLayers = layers;
        }
      }
    }

    setExpertLayersOnGPU(bestLayers);
  }, [serverResponse, maxGpuCount, gpuTotalBytes, gpuDevices, contextWindow, bytesPerElement, slots, kvCacheOnCPU]);

  // Reset auto-fit when serverResponse identity changes (new model selected).
  useEffect(() => {
    autoFitAppliedRef.current = false;
  }, [serverResponse]);

  // ── Derived calculations ─────────────────────────────────────────────────
  const vramInput = serverResponse?.input;
  const isMoE = serverResponse?.moe?.is_moe === true && serverResponse?.weights != null;

  const vramResult = useMemo(() => {
    if (!vramInput) return null;
    return calculateVRAM(
      { ...vramInput, context_window: contextWindow, bytes_per_element: bytesPerElement, slots },
      serverResponse?.weights ?? null,
      serverResponse?.moe ?? null,
      expertLayersOnGPU,
      kvCacheOnCPU,
    );
  }, [vramInput, contextWindow, bytesPerElement, slots, expertLayersOnGPU, kvCacheOnCPU, serverResponse?.weights, serverResponse?.moe]);

  const parsedTensorSplit = useMemo(() => {
    if (!tensorSplit) return [];
    return tensorSplit.split(',').map(s => parseFloat(s.trim())).filter(n => !isNaN(n));
  }, [tensorSplit]);

  const perDevice = useMemo(() => {
    if (!vramResult || deviceCount <= 1) return undefined;
    return calculatePerDeviceVRAM(vramResult.modelWeightsGPU, vramResult.kvVramBytes, vramResult.computeBufferEst, deviceCount, parsedTensorSplit);
  }, [vramResult, deviceCount, parsedTensorSplit]);

  // ── Public interface ─────────────────────────────────────────────────────
  const controlsProps: VRAMControlsState = {
    contextWindow,
    onContextWindowChange: setContextWindow,
    bytesPerElement,
    onBytesPerElementChange: setBytesPerElement,
    slots,
    onSlotsChange: setSlots,
    maxDeviceCount: maxGpuCount,
    isMoE,
    blockCount: vramInput?.block_count,
    expertLayersOnGPU,
    onExpertLayersOnGPUChange: setExpertLayersOnGPU,
    kvCacheOnCPU,
    onKvCacheOnCPUChange: setKvCacheOnCPU,
    deviceCount,
    onDeviceCountChange: setDeviceCount,
    tensorSplit,
    onTensorSplitChange: setTensorSplit,
  };

  const resultsProps: VRAMResultsState | null = vramResult && vramInput ? {
    vramResult,
    input: mergedInput(vramInput, contextWindow, bytesPerElement, slots),
    moe: serverResponse?.moe,
    weights: serverResponse?.weights,
    expertLayersOnGPU,
    perDevice,
    deviceCount,
    systemRAMBytes: systemRAM,
    gpuTotalBytes,
    gpuDevices,
    tensorSplit,
  } : null;

  return {
    controlsProps,
    resultsProps,
    isMoE,
    maxGpuCount,
    gpuTotalBytes,
    systemRAM,
    gpuDevices,
  };
}
