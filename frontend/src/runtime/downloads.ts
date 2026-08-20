export type DownloadSource = 'learning' | 'ministry' | 'export' | 'other';
export type DownloadKind = 'pdf' | 'document' | 'spreadsheet' | 'presentation' | 'image' | 'video' | 'audio' | 'text' | 'archive' | 'file';

export type DownloadResourceInput = {
  id?: string | number;
  key?: string;
  title?: string;
  name?: string;
  original_name?: string;
  filename?: string;
  url: string;
  type?: string;
  category?: string;
  mime_type?: string;
  mimeType?: string;
  file_size?: string | number;
  size?: string | number;
  source?: DownloadSource;
};

export type DownloadResource = {
  key: string;
  name: string;
  title: string;
  url: string;
  kind: DownloadKind;
  mimeType: string;
  size: number;
  source: DownloadSource;
};

export type ParsedContentRange = {
  start: number;
  end: number;
  total: number;
};

export const MAX_MANAGED_DOWNLOAD_BYTES = 1024 * 1024 * 1024;
export const MAX_DOWNLOAD_BATCH_SIZE = 50;

const kindExtensions: Array<[DownloadKind, RegExp]> = [
  ['pdf', /\.pdf$/i],
  ['document', /\.(doc|docx|odt|rtf)$/i],
  ['spreadsheet', /\.(csv|ods|xls|xlsm|xlsx)$/i],
  ['presentation', /\.(odp|ppt|pptx)$/i],
  ['image', /\.(avif|bmp|gif|jpe?g|png|svg|tiff?|webp)$/i],
  ['video', /\.(m4v|mkv|mov|mp4|webm)$/i],
  ['audio', /\.(aac|flac|m4a|ma4|mp3|ogg|opus|wav|weba)$/i],
  ['text', /\.(markdown|md|txt)$/i],
  ['archive', /\.(7z|gz|rar|tar|tgz|zip)$/i],
];

export function normalizeDownloadResource(input: DownloadResourceInput): DownloadResource {
  const url = normalizeDownloadURL(input.url);
  const source = input.source || 'other';
  const title = String(input.title || input.original_name || input.filename || input.name || '未命名资源').trim();
  const name = sanitizeDownloadFilename(
    input.original_name || input.filename || input.name || filenameFromURL(url) || title,
  );
  const mimeType = String(input.mime_type || input.mimeType || '').trim().toLowerCase();
  const rawSize = Number(input.file_size ?? input.size ?? 0);
  const size = Number.isSafeInteger(rawSize) && rawSize > 0 ? rawSize : 0;
  const rawIdentity = String(input.key || input.id || url);
  const identity = rawIdentity.startsWith(`${source}:`) ? rawIdentity.slice(source.length + 1) : rawIdentity;

  return {
    key: `${source}:${identity}`,
    name,
    title: title || name,
    url,
    kind: inferDownloadKind(name, mimeType, input.type || input.category),
    mimeType,
    size,
    source,
  };
}

export function normalizeDownloadURL(value: unknown): string {
  const raw = String(value || '').trim();
  if (!raw) throw new Error('download_url_missing');
  if (raw.startsWith('/') && !raw.startsWith('//')) return raw;
  if (typeof window !== 'undefined') {
    const parsed = new URL(raw, window.location.origin);
    if (parsed.origin === window.location.origin && ['http:', 'https:'].includes(parsed.protocol)) {
      return `${parsed.pathname}${parsed.search}${parsed.hash}`;
    }
  }
  throw new Error('download_url_not_allowed');
}

export function sanitizeDownloadFilename(value: unknown): string {
  const pathParts = String(value || '').trim().replace(/\\/g, '/').split('/');
  const base = pathParts.at(-1) || 'download';
  const safe = base
    .replace(/[\u0000-\u001f\u007f]/g, '')
    .replace(/[<>:"/\\|?*]/g, '_')
    .replace(/[. ]+$/g, '')
    .trim();
  return safe || 'download';
}

export function inferDownloadKind(filename: string, mimeType = '', hint = ''): DownloadKind {
  const mime = mimeType.toLowerCase();
  const normalizedHint = String(hint || '').toLowerCase();
  if (mime.includes('pdf')) return 'pdf';
  if (mime.startsWith('image/')) return 'image';
  if (mime.startsWith('video/')) return 'video';
  if (mime.startsWith('audio/')) return 'audio';
  if (mime.startsWith('text/') || mime.includes('markdown')) return 'text';
  if (mime.includes('word') || mime.includes('opendocument.text')) return 'document';
  if (mime.includes('sheet') || mime.includes('excel') || mime.includes('opendocument.spreadsheet')) return 'spreadsheet';
  if (mime.includes('presentation') || mime.includes('powerpoint')) return 'presentation';
  if (mime.includes('zip') || mime.includes('compressed') || mime.includes('archive')) return 'archive';
  for (const [kind, pattern] of kindExtensions) {
    if (pattern.test(filename)) return kind;
  }
  if (normalizedHint === 'video') return 'video';
  if (normalizedHint === 'image' || normalizedHint === 'outline') return 'image';
  if (normalizedHint === 'markdown' || normalizedHint === 'text') return 'text';
  return 'file';
}

export function parseContentRange(value: string | null): ParsedContentRange | null {
  const match = String(value || '').match(/^bytes\s+(\d+)-(\d+)\/(\d+|\*)$/i);
  if (!match) return null;
  const start = Number(match[1]);
  const end = Number(match[2]);
  const total = match[3] === '*' ? 0 : Number(match[3]);
  if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || end < start) return null;
  if (total && (!Number.isSafeInteger(total) || total <= end)) return null;
  return { start, end, total };
}

export function formatDownloadSize(bytes: number): string {
  const value = Number.isFinite(bytes) && bytes > 0 ? bytes : 0;
  if (value < 1024) return `${Math.round(value)} B`;
  const units = ['KB', 'MB', 'GB'];
  let scaled = value;
  let unit = 'B';
  for (const candidate of units) {
    scaled /= 1024;
    unit = candidate;
    if (scaled < 1024 || candidate === 'GB') break;
  }
  const precision = scaled >= 10 || Number.isInteger(scaled) ? 0 : 1;
  return `${scaled.toFixed(precision)} ${unit}`;
}

export function formatDownloadProgress(received: number, total: number): string {
  if (total > 0) {
    return `${Math.min(100, Math.max(0, Math.round((received / total) * 100)))}%`;
  }
  return formatDownloadSize(received);
}

export function downloadErrorMessage(code: unknown): string {
  const value = String(code || '');
  const labels: Record<string, string> = {
    download_manager_not_ready: '下载中心正在初始化，请稍后重试',
    download_batch_too_large: '每次最多选择 50 个资源',
    download_queue_full: '下载队列最多保留 50 个任务，请先清理部分任务',
    download_file_too_large: '文件超过 1 GiB 管理上限',
    download_url_missing: '资源缺少下载地址',
    download_url_not_allowed: '仅支持下载本站资源',
    download_unauthorized: '登录状态已失效，请重新登录',
    download_forbidden: '当前账号没有下载权限',
    download_not_found: '资源不存在或已被删除',
    download_rate_limited: '请求过于频繁，请稍后继续',
    download_storage_insufficient: '设备可用存储空间不足',
    download_storage_read_failed: '无法读取已保存的下载分片',
    download_storage_write_failed: '无法保存下载分片，请检查设备存储空间',
    download_stream_unavailable: '浏览器无法读取下载流',
    download_invalid_range: '服务器返回了无效的续传范围',
  };
  if (labels[value]) return labels[value];
  if (value.startsWith('download_http_')) return `服务器请求失败（${value.slice(14)}）`;
  return '网络中断，请重试';
}

export function filenameFromDisposition(value: string | null, fallback: string): string {
  const disposition = String(value || '');
  const utf8 = disposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8?.[1]) {
    try {
      return sanitizeDownloadFilename(decodeURIComponent(utf8[1]));
    } catch {
      return sanitizeDownloadFilename(utf8[1]);
    }
  }
  const plain = disposition.match(/filename="?([^";]+)"?/i);
  return sanitizeDownloadFilename(plain?.[1] || fallback);
}

function filenameFromURL(value: string): string {
  const clean = value.split('#')[0].split('?')[0];
  const raw = clean.split('/').at(-1) || '';
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}
