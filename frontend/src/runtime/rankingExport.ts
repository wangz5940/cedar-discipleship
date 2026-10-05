import { saveBlob } from './browserDownload';
import {
  availableStatisticsLegend,
  chartMemberLabel,
  statisticCount,
  statisticTotal,
  type RankingItem,
} from './statistics';

type ChartOptions = {
  title: string;
  subtitle: string;
  items: RankingItem[];
  activeKey?: string;
  taskTypes?: string[];
  colorScheme?: string;
};

const width = 1120;
const height = 720;

function escapeText(value: string): string {
  return value.replace(/[&<>"']/g, (char) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;',
  })[char]!);
}

export function rankingChartSVG({ title, subtitle, items, activeKey = 'all', taskTypes = [], colorScheme = 'default' }: ChartOptions): string {
  const left = 80;
  const right = 40;
  const top = 120;
  const chartWidth = width - left - right;
  const chartHeight = height - top - 120;
  const legend = availableStatisticsLegend(items, taskTypes, colorScheme).filter((part) => activeKey === 'all' || part.key === activeKey);
  const maxTotal = Math.max(1, ...items.map((item) => statisticTotal(item, activeKey)));
  const slotWidth = chartWidth / Math.max(1, items.length);
  const barWidth = Math.max(26, Math.min(42, slotWidth * 0.48));
  const legendSvg = legend.map((part, index) => `
    <g transform="translate(${left + index * (chartWidth / legend.length)}, 54)">
      <rect width="14" height="14" rx="4" fill="${part.color}" />
      <text x="24" y="12" font-size="16" fill="#3b4452">${part.label}</text>
    </g>`).join('');
  const bars = items.map((item, index) => {
    const x = left + slotWidth * index + (slotWidth - barWidth) / 2;
    let offset = 0;
    const segments = legend.map((part) => {
      const count = statisticCount(item, part.key);
      if (!count) return '';
      const segmentHeight = (count / maxTotal) * chartHeight;
      offset += segmentHeight;
      return `<rect x="${x}" y="${top + chartHeight - offset}" width="${barWidth}" height="${segmentHeight}" rx="8" fill="${part.color}" />`;
    }).join('');
    return `<g>
      <rect x="${x}" y="${top}" width="${barWidth}" height="${chartHeight}" rx="12" fill="rgba(15,23,42,0.05)" />
      ${segments}
      <text x="${x + barWidth / 2}" y="${top + chartHeight + 28}" text-anchor="middle" font-size="16" fill="#1f2937">${escapeText(chartMemberLabel(item))}</text>
      <text x="${x + barWidth / 2}" y="${top + chartHeight + 52}" text-anchor="middle" font-size="13" fill="#6b7280">${statisticTotal(item, activeKey)} 次</text>
    </g>`;
  }).join('');
  const grid = Array.from({ length: 5 }, (_, index) => {
    const y = top + (chartHeight / 4) * index;
    return `<g>
      <line x1="${left}" y1="${y}" x2="${width - right}" y2="${y}" stroke="rgba(15,23,42,0.08)" stroke-dasharray="6 6" />
      <text x="${left - 14}" y="${y + 5}" text-anchor="end" font-size="14" fill="#6b7280">${Math.round((maxTotal / 4) * (4 - index))}</text>
    </g>`;
  }).join('');
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
    <rect width="100%" height="100%" rx="32" fill="#ffffff"/>
    <text x="${left}" y="40" font-size="28" font-weight="700" fill="#111827">${escapeText(title)}</text>
    <text x="${left}" y="96" font-size="18" fill="#6b7280">${escapeText(subtitle)}</text>
    ${legendSvg}${grid}${bars}
  </svg>`;
}

export async function exportRankingPNG(options: ChartOptions & { filename: string }): Promise<void> {
  const url = URL.createObjectURL(new Blob([rankingChartSVG(options)], { type: 'image/svg+xml;charset=utf-8' }));
  try {
    const image = new Image();
    image.decoding = 'async';
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve();
      image.onerror = reject;
      image.src = url;
    });
    const canvas = document.createElement('canvas');
    canvas.width = width * 2;
    canvas.height = height * 2;
    const context = canvas.getContext('2d');
    if (!context) throw new Error('chart_canvas_unavailable');
    context.scale(2, 2);
    context.fillStyle = '#ffffff';
    context.fillRect(0, 0, width, height);
    context.drawImage(image, 0, 0, width, height);
    const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
    if (png) saveBlob(png, options.filename);
  } finally {
    URL.revokeObjectURL(url);
  }
}
