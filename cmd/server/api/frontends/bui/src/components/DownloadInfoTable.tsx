import type { DownloadMeta } from '../contexts/DownloadContext';

function urlBaseName(url: string): string {
  try {
    const pathname = new URL(url).pathname;
    return pathname.split('/').pop() || url;
  } catch {
    return url.split('/').pop() || url;
  }
}

interface Props {
  meta: DownloadMeta;
  // urls, when provided, is the authoritative list of shard URLs known
  // up front (e.g. from the resolver). It overrides the streamed
  // model_urls so the row count always matches the true number of files
  // even before the per-file SSE meta arrives.
  urls?: string[];
}

export default function DownloadInfoTable({ meta, urls }: Props) {
  const shardUrls = urls && urls.length > 0 ? urls : meta.model_urls;
  const total = shardUrls.length;
  const isSplit = total > 1;

  return (
    <table className="flags-table">
      <tbody>
        {meta.model_id && (
          <tr>
            <th>Model ID</th>
            <td><code>{meta.model_id}</code></td>
          </tr>
        )}
        {!isSplit && shardUrls.length === 1 && (
          <tr>
            <th>Model URL</th>
            <td><a href={shardUrls[0]} target="_blank" rel="noopener noreferrer"><code>{urlBaseName(shardUrls[0])}</code></a></td>
          </tr>
        )}
        {isSplit && Array.from({ length: total }, (_, i) => {
          const url = shardUrls[i];
          return (
            <tr key={url ?? `pending-${i}`}>
              <th>Split {i + 1} of {total}</th>
              <td>
                {url
                  ? <a href={url} target="_blank" rel="noopener noreferrer"><code>{urlBaseName(url)}</code></a>
                  : <em>pending…</em>}
              </td>
            </tr>
          );
        })}
        {meta.proj_url && (
          <tr>
            <th>Projection URL</th>
            <td><a href={meta.proj_url} target="_blank" rel="noopener noreferrer"><code>{urlBaseName(meta.proj_url)}</code></a></td>
          </tr>
        )}
      </tbody>
    </table>
  );
}
