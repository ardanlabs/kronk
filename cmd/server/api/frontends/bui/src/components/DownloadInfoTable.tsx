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
}

export default function DownloadInfoTable({ meta }: Props) {
  const total = Math.max(meta.fileTotal || 0, meta.model_urls.length);
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
        {!isSplit && meta.model_urls.length === 1 && (
          <tr>
            <th>Model URL</th>
            <td><a href={meta.model_urls[0]} target="_blank" rel="noopener noreferrer"><code>{urlBaseName(meta.model_urls[0])}</code></a></td>
          </tr>
        )}
        {isSplit && Array.from({ length: total }, (_, i) => {
          const url = meta.model_urls[i];
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
