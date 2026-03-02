import React from 'react';

export interface KVRow {
  key?: React.Key;
  label: React.ReactNode;
  value: React.ReactNode;
}

export default function KeyValueTable({ rows }: { rows: KVRow[] }) {
  if (rows.length === 0) return null;

  return (
    <dl className="kv-table">
      {rows.map((r, i) => (
        <div className="kv-row" key={r.key ?? i}>
          <dt className="kv-label">{r.label}</dt>
          <dd className="kv-value">{r.value}</dd>
        </div>
      ))}
    </dl>
  );
}
