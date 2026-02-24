import React from 'react';
import type { AutoTestTrialResult } from '../types';
import type { ConfigTrialResult } from '../contexts/AutoTestRunnerContext';

// ---------------------------------------------------------------------------
// Public interfaces
// ---------------------------------------------------------------------------

export interface CellMeta {
  isPending: boolean;
  isInProgress: boolean;
  isBest: boolean;
  index: number;
}

export interface ColumnDef<Row = any> {
  id: string;
  title: string;
  sortable?: boolean;
  getValue: (row: Row) => number | string | undefined;
  renderCell: (row: Row, meta: CellMeta) => React.ReactNode;
}

export interface SweepModeDescriptor {
  kind: string;
  label: string;
}

export const SWEEP_MODES: SweepModeDescriptor[] = [
  { kind: 'sampling', label: 'Sampling Sweep' },
  { kind: 'config', label: 'Config Sweep' },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

export function formatMs(ms: number): string {
  if (!Number.isFinite(ms)) return '—';
  const total = Math.max(0, Math.round(ms));
  if (total < 1000) return `${total}ms`;
  const hours = Math.floor(total / 3600000);
  const minutes = Math.floor((total % 3600000) / 60000);
  const seconds = Math.floor((total % 60000) / 1000);
  const millis = total % 1000;
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s ${millis}ms`;
  if (minutes > 0) return `${minutes}m ${seconds}s ${millis}ms`;
  return `${seconds}s ${millis}ms`;
}

export function scoreColor(score: number): string {
  if (score >= 80) return '#2e7d32';
  if (score >= 50) return '#f9a825';
  return '#c62828';
}

export function scoreColorSafe(score: number | undefined): string {
  return scoreColor(score ?? 0);
}

export function formatScore(score: number | undefined): string {
  return score !== undefined && Number.isFinite(score) ? score.toFixed(1) : '—';
}

export function getScenarioScore(trial: AutoTestTrialResult, id: 'chat' | 'tool_call'): number | undefined {
  return trial.scenarioResults.find(r => r.scenarioId === id)?.score;
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

function partialTPS(row: AutoTestTrialResult): number | undefined {
  return row.scenarioResults.find(r => r.avgTPS !== undefined)?.avgTPS;
}

function durationMs(row: AutoTestTrialResult): number | undefined {
  const startMs = row.startedAt ? Date.parse(row.startedAt) : NaN;
  if (!Number.isFinite(startMs)) return undefined;
  const endMs = row.finishedAt ? Date.parse(row.finishedAt) : Date.now();
  return endMs - startMs;
}

function statusText(row: AutoTestTrialResult, meta: CellMeta, hasError: boolean, errorMsg?: string): string {
  if (hasError) return errorMsg ? `Error: ${errorMsg}` : 'Error';
  if (row.status === 'failed') return 'Failed';
  if (meta.isInProgress) return 'Running…';
  if (row.status === 'skipped') return 'Skipped';
  if (row.status === 'queued') return 'Queued';
  if (meta.isPending) return '…';
  return 'OK';
}

function statusStyle(row: AutoTestTrialResult, meta: CellMeta, hasError: boolean): React.CSSProperties {
  if (hasError) return { color: '#c62828', fontSize: '0.85em' };
  if (row.status === 'failed') return { color: '#c62828', fontSize: '0.85em' };
  if (meta.isInProgress) return { color: '#1565c0' };
  if (row.status === 'skipped') return { color: '#9e9e9e', fontStyle: 'italic', fontSize: '0.85em' };
  if (row.status === 'queued') return { color: '#999' };
  if (meta.isPending) return { color: '#666' };
  return { color: '#2e7d32' };
}

// ---------------------------------------------------------------------------
// Column builders – shared metrics
// ---------------------------------------------------------------------------

export const FILL_LEVELS = ['0%', '20%', '50%', '80%'] as const;
type FillLevel = typeof FILL_LEVELS[number];

export function sharedMetricColumns<R extends AutoTestTrialResult>(): ColumnDef<R>[] {
  const tpsByFill: ColumnDef<R>[] = FILL_LEVELS.map((level: FillLevel) => ({
    id: `tps_${level.replace('%', '')}`,
    title: `TPS @${level}`,
    sortable: true,
    getValue: (row) => row.avgTPSByFill?.[level],
    renderCell: (row, meta) => {
      if (meta.isPending) return '…';
      return row.avgTPSByFill?.[level]?.toFixed(1) ?? '—';
    },
  }));

  const ttftByFill: ColumnDef<R>[] = FILL_LEVELS.map((level: FillLevel) => ({
    id: `ttft_${level.replace('%', '')}`,
    title: `TTFT @${level}`,
    sortable: true,
    getValue: (row) => row.avgTTFTByFill?.[level],
    renderCell: (row, meta) => {
      if (meta.isPending) return '…';
      return row.avgTTFTByFill?.[level] !== undefined ? formatMs(row.avgTTFTByFill[level]) : '—';
    },
  }));

  return [
    {
      id: 'avg_tps',
      title: 'Avg TPS',
      sortable: true,
      getValue: (row) => row.avgTPS,
      renderCell: (row, meta) => {
        if (meta.isPending) {
          const p = partialTPS(row);
          return p !== undefined ? `~${p.toFixed(1)}` : '…';
        }
        return row.avgTPS?.toFixed(1) ?? '—';
      },
    },
    {
      id: 'avg_ttft',
      title: 'Avg TTFT',
      sortable: true,
      getValue: (row) => row.avgTTFT,
      renderCell: (row, meta) => {
        if (meta.isPending) return '…';
        return row.avgTTFT !== undefined ? formatMs(row.avgTTFT) : '—';
      },
    },
    ...tpsByFill,
    ...ttftByFill,
  ];
}

// ---------------------------------------------------------------------------
// Column builders – status & duration (used by both modes)
// ---------------------------------------------------------------------------

function samplingStatusColumn(): ColumnDef<AutoTestTrialResult> {
  return {
    id: 'status',
    title: 'Status',
    sortable: true,
    getValue: (row) => row.status,
    renderCell: (row, meta) => (
      <span style={statusStyle(row, meta, row.status === 'failed')}>
        {statusText(row, meta, row.status === 'failed')}
      </span>
    ),
  };
}

function configStatusColumn(): ColumnDef<ConfigTrialResult> {
  return {
    id: 'status',
    title: 'Status',
    sortable: true,
    getValue: (row) => (row.error ? 'error' : row.status),
    renderCell: (row, meta) => (
      <span style={statusStyle(row, meta, !!row.error)}>
        {statusText(row, meta, !!row.error, row.error)}
      </span>
    ),
  };
}

function durationColumn<R extends AutoTestTrialResult>(): ColumnDef<R> {
  return {
    id: 'duration',
    title: 'Duration',
    sortable: true,
    getValue: (row) => durationMs(row),
    renderCell: (row) => {
      const ms = durationMs(row);
      return ms !== undefined ? formatMs(ms) : '—';
    },
  };
}

// ---------------------------------------------------------------------------
// Column builders – sampling parameters
// ---------------------------------------------------------------------------

export function samplingParamColumns(): ColumnDef<AutoTestTrialResult>[] {
  return [
    {
      id: 'temperature',
      title: 'Temperature',
      sortable: true,
      getValue: (row) => row.candidate.temperature,
      renderCell: (row) => row.candidate.temperature ?? '—',
    },
    {
      id: 'top_p',
      title: 'Top P',
      sortable: true,
      getValue: (row) => row.candidate.top_p,
      renderCell: (row) => row.candidate.top_p ?? '—',
    },
    {
      id: 'top_k',
      title: 'Top K',
      sortable: true,
      getValue: (row) => row.candidate.top_k,
      renderCell: (row) => row.candidate.top_k ?? '—',
    },
    {
      id: 'min_p',
      title: 'Min P',
      sortable: true,
      getValue: (row) => row.candidate.min_p,
      renderCell: (row) => row.candidate.min_p ?? '—',
    },
  ];
}

// ---------------------------------------------------------------------------
// Column builders – sampling scores
// ---------------------------------------------------------------------------

export function samplingScoreColumns(): ColumnDef<AutoTestTrialResult>[] {
  return [
    {
      id: 'chat_score',
      title: 'Chat Score',
      sortable: true,
      getValue: (row) => getScenarioScore(row, 'chat'),
      renderCell: (row, meta) => {
        const partial = meta.isInProgress ? getScenarioScore(row, 'chat') : undefined;
        if (meta.isPending) {
          if (partial !== undefined) {
            return <span style={{ color: scoreColor(partial), opacity: 0.7 }}>~{partial.toFixed(1)}</span>;
          }
          return '…';
        }
        const score = getScenarioScore(row, 'chat');
        return <span style={{ color: scoreColor(score ?? 0) }}>{score?.toFixed(1) ?? '—'}</span>;
      },
    },
    {
      id: 'tool_score',
      title: 'Tool Score',
      sortable: true,
      getValue: (row) => getScenarioScore(row, 'tool_call'),
      renderCell: (row, meta) => {
        const partial = meta.isInProgress ? getScenarioScore(row, 'tool_call') : undefined;
        if (meta.isPending) {
          if (partial !== undefined) {
            return <span style={{ color: scoreColor(partial), opacity: 0.7 }}>~{partial.toFixed(1)}</span>;
          }
          return '…';
        }
        const score = getScenarioScore(row, 'tool_call');
        return <span style={{ color: scoreColor(score ?? 0) }}>{score?.toFixed(1) ?? '—'}</span>;
      },
    },
    {
      id: 'total_score',
      title: 'Total Score',
      sortable: true,
      getValue: (row) => row.totalScore,
      renderCell: (row, meta) => {
        if (meta.isPending) return '…';
        return <span style={{ color: scoreColor(row.totalScore ?? 0) }}>{row.totalScore?.toFixed(1) ?? '—'}</span>;
      },
    },
  ];
}

// ---------------------------------------------------------------------------
// Column builders – config parameters
// ---------------------------------------------------------------------------

export function configParamColumns(): ColumnDef<ConfigTrialResult>[] {
  return [
    {
      id: 'context_window',
      title: 'Context Window',
      sortable: true,
      getValue: (row) => row.config?.['context_window'],
      renderCell: (row) => row.config?.['context_window'] ?? '—',
    },
    {
      id: 'nbatch',
      title: 'NBatch',
      sortable: true,
      getValue: (row) => row.config?.nbatch,
      renderCell: (row) => row.config?.nbatch ?? '—',
    },
    {
      id: 'nubatch',
      title: 'NUBatch',
      sortable: true,
      getValue: (row) => row.config?.nubatch,
      renderCell: (row) => row.config?.nubatch ?? '—',
    },
    {
      id: 'nseq_max',
      title: 'NSeqMax',
      sortable: true,
      getValue: (row) => row.config?.['nseq_max'],
      renderCell: (row) => row.config?.['nseq_max'] ?? '—',
    },
    {
      id: 'flash_attention',
      title: 'Flash Attn',
      sortable: true,
      getValue: (row) => row.config?.['flash_attention'],
      renderCell: (row) => row.config?.['flash_attention'] ?? '—',
    },
    {
      id: 'cache_type',
      title: 'KV Cache',
      sortable: true,
      getValue: (row) => row.config?.['cache_type'],
      renderCell: (row) => row.config?.['cache_type'] ?? '—',
    },
    {
      id: 'cache_mode',
      title: 'Cache',
      sortable: true,
      getValue: (row) => row.config?.['cache_mode'],
      renderCell: (row) => {
        const mode = row.config?.['cache_mode'];
        if (!mode) return '—';
        if (mode === 'none') return 'None';
        return mode.toUpperCase();
      },
    },
  ];
}

// ---------------------------------------------------------------------------
// Composite column sets
// ---------------------------------------------------------------------------

export function buildSamplingColumns(): ColumnDef<AutoTestTrialResult>[] {
  return [
    ...samplingParamColumns(),
    samplingStatusColumn(),
    durationColumn<AutoTestTrialResult>(),
    ...samplingScoreColumns(),
    ...sharedMetricColumns<AutoTestTrialResult>(),
  ];
}

export function buildConfigColumns(): ColumnDef<ConfigTrialResult>[] {
  return [
    ...configParamColumns(),
    configStatusColumn(),
    durationColumn<ConfigTrialResult>(),
    ...sharedMetricColumns<ConfigTrialResult>(),
  ];
}
