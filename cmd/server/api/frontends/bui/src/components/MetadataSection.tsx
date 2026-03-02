import KeyValueTable from './KeyValueTable';
import type { KVRow } from './KeyValueTable';
import { fmtVal } from '../lib/format';

function titleCase(s: string): string {
  return s
    .split(' ')
    .filter(Boolean)
    .map(w => w.length <= 2 ? w.toUpperCase() : (w[0].toUpperCase() + w.slice(1)))
    .join(' ');
}

function humanizeKey(prefix: string, fullKey: string): string {
  const withoutPrefix = fullKey.startsWith(prefix + '.') ? fullKey.slice(prefix.length + 1) : fullKey;
  const spaced = withoutPrefix.replace(/[._]/g, ' ').replace(/\s+/g, ' ').trim();
  return titleCase(spaced);
}

interface GroupedEntries {
  prefix: string;
  entries: Array<[string, string]>;
}

function groupByPrefix(metadata: Record<string, string>): GroupedEntries[] {
  const groups: Record<string, Array<[string, string]>> = {};
  for (const [k, v] of Object.entries(metadata)) {
    const prefix = k.includes('.') ? k.split('.')[0] : 'other';
    (groups[prefix] ||= []).push([k, v]);
  }

  const order = (p: string) => (p === 'general' ? 0 : p === 'tokenizer' ? 1 : 10);
  return Object.entries(groups)
    .sort(([a], [b]) => (order(a) - order(b)) || a.localeCompare(b))
    .map(([prefix, entries]) => ({
      prefix,
      entries: entries.sort(([ka], [kb]) => ka.localeCompare(kb)),
    }));
}

interface MetadataSectionProps {
  metadata: Record<string, string>;
  excludeKeys?: string[];
}

export default function MetadataSection({ metadata, excludeKeys = [] }: MetadataSectionProps) {
  const filtered = Object.fromEntries(
    Object.entries(metadata).filter(([k]) => !excludeKeys.includes(k))
  );

  const grouped = groupByPrefix(filtered);

  if (grouped.length === 0) {
    return (
      <div className="empty-state">
        <p>No metadata available for this model.</p>
      </div>
    );
  }

  return (
    <div className="meta-sections">
      {grouped.map(g => {
        const rows: KVRow[] = g.entries.map(([key, value]) => ({
          key,
          label: humanizeKey(g.prefix, key),
          value: fmtVal(value),
        }));

        return (
          <section key={g.prefix} className="meta-section">
            <div className="meta-section-header">
              <h4 className="meta-section-title">{titleCase(g.prefix)}</h4>
            </div>
            <KeyValueTable rows={rows} />
          </section>
        );
      })}
    </div>
  );
}
