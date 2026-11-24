// 历史数据页使用的纯函数（可在 node:test 中单独测试）

export type Agg = 'avg' | 'min' | 'max' | 'last' | 'lttb';
export type Tier = 'auto' | 'raw' | 'rollup';

export interface HistoryQuery {
  deviceId: string;
  key: string;
  start?: Date | null;
  end?: Date | null;
  limit?: number;
  agg?: Agg;
  tier?: Tier;
}

export interface SeriesPoint {
  t: string;
  v: unknown;
  q?: number;
}

/** 构造 /data/history、/data/export 的查询字符串（时间使用 RFC3339，参数全部转义） */
export function buildHistoryParams(q: HistoryQuery): string {
  const p = new URLSearchParams();
  p.set('device_id', q.deviceId);
  p.set('key', q.key);
  if (q.start) p.set('start', q.start.toISOString());
  if (q.end) p.set('end', q.end.toISOString());
  if (q.limit && q.limit > 0) p.set('limit', String(Math.floor(q.limit)));
  if (q.agg) p.set('agg', q.agg);
  if (q.tier && q.tier !== 'auto') p.set('tier', q.tier);
  return p.toString();
}

/** 把点值转换为图表数值；bool 视为 0/1；无法转换时返回 null（图表中断开） */
export function toNumber(v: unknown): number | null {
  if (typeof v === 'number') return Number.isFinite(v) ? v : null;
  if (typeof v === 'boolean') return v ? 1 : 0;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }
  return null;
}

/** 序列是否可以画成折线图（至少一个点可转换为数值） */
export function isNumericSeries(points: SeriesPoint[]): boolean {
  return points.some((p) => toNumber(p.v) !== null);
}

/** 转换为 ECharts 时间轴数据 [毫秒时间戳, 数值|null] */
export function toChartData(points: SeriesPoint[]): Array<[number, number | null]> {
  return points.map((p) => [new Date(p.t).getTime(), toNumber(p.v)]);
}

/** 把点值格式化为表格文本（复合类型输出 JSON） */
export function formatPointValue(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'object') {
    try {
      return JSON.stringify(v);
    } catch {
      return String(v);
    }
  }
  return String(v);
}

export interface SinkStateInfo {
  text: string;
  color: string;
}

/** 缓冲连接器状态的中文名称和标签颜色 */
export function sinkStateInfo(state: string): SinkStateInfo {
  switch (state) {
    case 'realtime':
      return { text: '实时', color: 'green' };
    case 'replaying':
      return { text: '补发中', color: 'blue' };
    case 'buffering':
      return { text: '缓冲中', color: 'orange' };
    case 'paused':
      return { text: '已暂停', color: 'gold' };
    case 'stopped':
      return { text: '已停止', color: 'default' };
    default:
      return { text: state || '未知', color: 'default' };
  }
}

/** 数据层的中文名称 */
export function tierLabel(tier: string): string {
  if (tier === 'rollup') return '汇总层';
  if (tier === 'raw') return '原始数据';
  return tier || '-';
}

/** 积压时长的可读形式 */
export function formatLag(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '-';
  if (seconds < 60) return `${Math.round(seconds)} 秒`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} 分钟`;
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)} 小时`;
  return `${(seconds / 86400).toFixed(1)} 天`;
}
