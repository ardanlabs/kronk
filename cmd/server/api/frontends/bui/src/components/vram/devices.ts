import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { DeviceInfo, DevicesResponse } from '../../types';

// ── Device info types and parsing ───────────────────────────────────────────

export interface DevicesInfo {
  gpuCount: number;
  gpuType: string;
  gpuVramBytes: number;
  ramBytes: number;
  gpuDevices: DeviceInfo[];
}

export function getGpuDevices(devices: DeviceInfo[]): DeviceInfo[] {
  return devices.filter((d) => d.type.startsWith('gpu_'));
}

export function formatGpuType(type: string): string {
  return type
    .replace('gpu_', '')
    .replace('cuda', 'CUDA')
    .replace('metal', 'Metal')
    .replace('rocm', 'ROCm')
    .replace('vulkan', 'Vulkan');
}

export function parseDevicesInfo(
  resp: Pick<DevicesResponse, 'devices' | 'gpu_count' | 'gpu_total_bytes' | 'system_ram_bytes'>,
): DevicesInfo {
  const gpuDevices = getGpuDevices(resp.devices);
  const gpuType = gpuDevices.length > 0 ? formatGpuType(gpuDevices[0].type) : '';

  return {
    gpuCount: resp.gpu_count,
    gpuType,
    gpuVramBytes: resp.gpu_total_bytes,
    ramBytes: resp.system_ram_bytes,
    gpuDevices,
  };
}

// ── Shared device info hook ─────────────────────────────────────────────────

export function useDevicesInfo(): DevicesInfo | null {
  const [devicesInfo, setDevicesInfo] = useState<DevicesInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    api.getDevices()
      .then((resp) => {
        if (cancelled) return;
        setDevicesInfo(parseDevicesInfo(resp));
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  return devicesInfo;
}
