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
      setGpuLayers(input.block_count ?? 0);
      setExpertLayersOnGPU(input.block_count ?? 0);
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

    const blockCount = serverResponse.input.block_count;
    if (!blockCount || blockCount <= 0) return;

    const isMoEResult = serverResponse.moe?.is_moe === true && serverResponse.weights != null;

    // Determine available capacity per GPU.
    const hasPerGpuInfo = gpuDevices.length > 0;
    const combinedFreeBytes = hasPerGpuInfo
      ? gpuDevices.reduce((sum, d) => sum + d.free_bytes, 0)
      : gpuTotalBytes;

    if (combinedFreeBytes <= 0) {
      setGpuLayers(blockCount);
      setExpertLayersOnGPU(blockCount);
      return;
    }

    const input = { ...serverResponse.input, context_window: contextWindow, bytes_per_element: bytesPerElement, slots };

    const fitsInVRAM = (v: ReturnType<typeof calculateVRAM>) => {
      if (hasPerGpuInfo && gpuCount > 1) {
        const perDev = calculatePerDeviceVRAM(v.modelWeightsGPU, v.kvVramBytes, v.computeBufferEst, gpuCount, []);
        return perDev.every((dev, i) => {
          const cap = gpuDevices[i]?.free_bytes ?? 0;
          return cap > 0 ? dev.totalBytes <= cap * 0.95 : true;
        });
      }
      return v.totalVram <= combinedFreeBytes * 0.95;
    };

    if (isMoEResult) {
      // MoE auto-fit: try expert offloading first (all layers on GPU,
      // maximize expert layers). Falls back to layer offloading if the
      // always-active weights alone don't fit.
      let best = { ngl: blockCount, experts: 0 };

      // Expert offloading: gpuLayers = blockCount, find max expertLayersOnGPU.
      let bestExperts = -1;
      for (let experts = blockCount; experts >= 0; experts--) {
        const v = calculateVRAM(input, { weights: serverResponse.weights, moe: serverResponse.moe, gpuLayers: blockCount, expertLayersOnGPU: experts, kvCacheOnCPU });
        if (fitsInVRAM(v)) {
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
          if (fitsInVRAM(v)) {
            best = { ngl, experts: ngl };
            break;
          }
        }
      }

      setGpuLayers(best.ngl);
      setExpertLayersOnGPU(best.experts);
    } else {
      // Dense auto-fit: optimize gpuLayers.
      let bestGpuLayers = 0;
      for (let ngl = 0; ngl <= blockCount; ngl++) {
        const v = calculateVRAM(input, { gpuLayers: ngl, kvCacheOnCPU });
        if (fitsInVRAM(v)) bestGpuLayers = ngl;
      }
      setGpuLayers(bestGpuLayers);
      setExpertLayersOnGPU(0);
    }
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
      {
        weights: serverResponse?.weights ?? null,
        moe: serverResponse?.moe ?? null,
        gpuLayers,
        expertLayersOnGPU,
        kvCacheOnCPU,
      },
    );
  }, [vramInput, contextWindow, bytesPerElement, slots, gpuLayers, expertLayersOnGPU, kvCacheOnCPU, serverResponse?.weights, serverResponse?.moe]);

  const parsedTensorSplit = useMemo(() => {
    if (!tensorSplit) return [];
    return tensorSplit.split(',').map(s => parseFloat(s.trim())).filter(n => !isNaN(n));
  }, [tensorSplit]);

  const perDevice = useMemo(() => {
    if (!vramResult) return undefined;
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
    gpuLayers,
    onGpuLayersChange: setGpuLayers,
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
    gpuLayers,
    expertLayersOnGPU,
    kvCacheOnCPU,
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
