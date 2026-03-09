import VRAMControls from './VRAMControls';
import VRAMResults from './VRAMResults';
import type { VRAMControlsState, VRAMResultsState } from './useVRAMState';
import type { ContextInfo } from '../../lib/context';

interface VRAMCalculatorPanelProps {
  controlsProps: VRAMControlsState;
  resultsProps?: VRAMResultsState | null;
  variant?: 'form' | 'compact';
  contextInfo?: ContextInfo | null;
  /** When true, only results are rendered (controls are managed externally). */
  hideControls?: boolean;
}

export default function VRAMCalculatorPanel({
  controlsProps,
  resultsProps,
  variant = 'compact',
  contextInfo,
  hideControls,
}: VRAMCalculatorPanelProps) {
  return (
    <>
      {!hideControls && (
        <div style={variant === 'compact' ? { marginBottom: '24px' } : undefined}>
          <VRAMControls
            {...controlsProps}
            variant={variant}
            contextInfo={contextInfo}
          />
        </div>
      )}

      {resultsProps && (
        <VRAMResults
          totalVram={resultsProps.vramResult.totalVram}
          slotMemory={resultsProps.vramResult.slotMemory}
          kvPerSlot={resultsProps.vramResult.kvPerSlot}
          kvPerTokenPerLayer={resultsProps.vramResult.kvPerTokenPerLayer}
          input={resultsProps.input}
          moe={resultsProps.moe}
          weights={resultsProps.weights}
          modelWeightsGPU={resultsProps.vramResult.modelWeightsGPU}
          modelWeightsCPU={resultsProps.vramResult.modelWeightsCPU}
          computeBufferEst={resultsProps.vramResult.computeBufferEst}
          alwaysActiveGPUBytes={resultsProps.vramResult.alwaysActiveGPUBytes}
          alwaysActiveCPUBytes={resultsProps.vramResult.alwaysActiveCPUBytes}
          expertGPUBytes={resultsProps.vramResult.expertGPUBytes}
          expertCPUBytes={resultsProps.vramResult.expertCPUBytes}
          gpuLayers={resultsProps.gpuLayers}
          expertLayersOnGPU={resultsProps.expertLayersOnGPU}
          kvCacheOnCPU={resultsProps.kvCacheOnCPU}
          kvCpuBytes={resultsProps.vramResult.kvCpuBytes}
          totalSystemRamEst={resultsProps.vramResult.totalSystemRamEst}
          perDevice={resultsProps.perDevice}
          deviceCount={resultsProps.deviceCount}
          systemRAMBytes={resultsProps.systemRAMBytes}
          gpuTotalBytes={resultsProps.gpuTotalBytes}
          gpuDevices={resultsProps.gpuDevices}
          tensorSplit={resultsProps.tensorSplit}
        />
      )}
    </>
  );
}
