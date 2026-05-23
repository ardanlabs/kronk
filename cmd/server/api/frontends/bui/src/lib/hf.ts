// Shared HuggingFace input helpers used by Model Pull and VRAM Calculator
// so both screens behave identically when parsing pasted URLs, matching
// quant shortcuts, and collapsing split-shard groups.

import type { HFRepoFile } from '../types';

// stripGGUF removes a trailing ".gguf" extension (case-insensitive) so the
// Model field always shows the bare id even when populated from a filename.
export function stripGGUF(name: string): string {
  return name.replace(/\.gguf$/i, '');
}

// modelIDFromFilename derives the canonical model id from a HuggingFace
// repo filename. Strips the .gguf extension and any "-NNNNN-of-NNNNN"
// split suffix so split shards produce the same model id as their
// non-split sibling.
export function modelIDFromFilename(filename: string): string {
  const base = filename.split('/').pop() ?? filename;
  return stripGGUF(base).replace(/-\d+-of-\d+$/, '');
}

// quantOnlyRe matches strings that look like *just* a GGUF quantization
// tag (no model id prefix) — e.g. "Q4_K_M", "Q8_0", "IQ3_M",
// "UD-Q4_K_M", "BF16", "F16", "F32".
const quantOnlyRe = /^(UD-)?((IQ|Q)\d+(_[A-Z0-9]+)*|BF16|F16|F32)$/i;

export function isQuantOnly(s: string): boolean {
  return quantOnlyRe.test(s.trim());
}

// isMMProjFile reports whether filename names a multi-modal projection
// file. Used to keep mmproj entries out of the quant-match candidates so
// the Model field shortcut never accidentally picks a projection.
export function isMMProjFile(filename: string): boolean {
  const base = (filename.split('/').pop() ?? filename).toLowerCase();
  return /(^|[-._])mmproj([-._]|$)/.test(base);
}

// matchesQuant reports whether filename ends with the given quant tag,
// ignoring split shard suffixes and the .gguf extension. Recognises both
// dash- and dot-separated quant suffixes ("model-Q4_K_M.gguf",
// "model.Q4_K_M.gguf") used by different community quantizers.
export function matchesQuant(filename: string, quant: string): boolean {
  const base = filename.split('/').pop() ?? filename;
  const noExt = stripGGUF(base);
  const noSplit = noExt.replace(/-\d+-of-\d+$/, '').toLowerCase();
  const q = quant.trim().toLowerCase();
  return noSplit.endsWith('-' + q) || noSplit.endsWith('.' + q) || noSplit === q;
}

// splitPaste tries to parse a pasted URL or shorthand into provider /
// family / model components. Returns null when the input doesn't look
// like a HuggingFace shape worth auto-splitting (so plain "unsloth"
// stays in the Provider field as the user typed it).
export function splitPaste(input: string): { provider: string; family: string; model: string } | null {
  const trimmed = input.trim();
  if (!trimmed) return null;

  let s = trimmed;
  for (const prefix of ['https://huggingface.co/', 'http://huggingface.co/', 'https://hf.co/', 'http://hf.co/', 'huggingface.co/', 'hf.co/']) {
    if (s.toLowerCase().startsWith(prefix)) {
      s = s.slice(prefix.length);
      break;
    }
  }

  const parts = s.split('/').filter((p) => p !== '');
  if (parts.length < 2) return null;

  const provider = parts[0];
  const family = parts[1];

  // owner/repo/[resolve|blob|tree]/<rev>/<file...>
  let filename = '';
  if (parts.length > 4 && (parts[2] === 'resolve' || parts[2] === 'blob' || parts[2] === 'tree')) {
    filename = parts.slice(4).join('/');
  } else if (parts.length > 2) {
    filename = parts.slice(2).join('/');
  }

  return { provider, family, model: filename ? modelIDFromFilename(filename) : '' };
}

// formatTotalSize renders a byte count using the same GB/MB/KB scale the
// server uses for HFRepoFile.size_str, so summed split totals match the
// per-shard formatting users see elsewhere in the BUI.
export function formatTotalSize(bytes: number): string {
  const gb = 1000 * 1000 * 1000;
  const mb = 1000 * 1000;
  const kb = 1000;
  if (bytes >= gb) return `${(bytes / gb).toFixed(1)} GB`;
  if (bytes >= mb) return `${(bytes / mb).toFixed(1)} MB`;
  if (bytes >= kb) return `${(bytes / kb).toFixed(1)} KB`;
  return `${bytes} B`;
}

// RepoFileRow is a collapsed picker row. Single-file entries map 1:1 to
// the underlying HFRepoFile; split-shard groups collapse into one row
// whose size is the sum of every shard.
export interface RepoFileRow {
  label: string;     // path displayed in the Filename column
  filename: string;  // representative shard passed to the action handler
  sizeStr: string;   // formatted total (sum across shards for splits)
  parts: number;     // shard count (1 for non-split entries)
}

// groupRepoFiles collapses "-NNNNN-of-NNNNN" split shards into one row
// per logical model so the picker shows the real total size and a single
// action button. Selecting any shard already pulls the whole set via
// modelIDFromFilename + the backend manifest, so the representative
// filename can be any shard in the group.
export function groupRepoFiles(files: HFRepoFile[]): RepoFileRow[] {
  const groups = new Map<string, HFRepoFile[]>();
  for (const f of files) {
    // Strip the split suffix while preserving the folder path so two
    // different folders never collide into one group.
    const key = f.filename.replace(/-\d+-of-\d+(\.gguf)$/i, '$1');
    const existing = groups.get(key);
    if (existing) existing.push(f);
    else groups.set(key, [f]);
  }

  const rows: RepoFileRow[] = [];
  for (const [key, group] of groups) {
    if (group.length === 1) {
      const f = group[0];
      rows.push({ label: f.filename, filename: f.filename, sizeStr: f.size_str, parts: 1 });
      continue;
    }
    group.sort((a, b) => a.filename.localeCompare(b.filename));
    const total = group.reduce((s, f) => s + f.size, 0);
    rows.push({ label: key, filename: group[0].filename, sizeStr: formatTotalSize(total), parts: group.length });
  }
  return rows;
}

// isSplitFilename reports whether a filename is part of a split-shard
// group ("-NNNNN-of-NNNNN.gguf"). Useful when callers need to know the
// representative shard came from a multi-file model.
export function isSplitFilename(filename: string): boolean {
  return /-\d+-of-\d+\.gguf$/i.test(filename);
}
