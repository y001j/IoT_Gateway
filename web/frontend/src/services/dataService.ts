import api from './api';
import { buildHistoryParams, type HistoryQuery, type SeriesPoint } from '../utils/history';

export interface SeriesInfo {
  device_id: string;
  key: string;
  subject: string;
  messages: number;
}

export interface SeriesResult {
  device_id: string;
  key: string;
  start: string;
  end: string;
  tier: string;
  points: SeriesPoint[];
  matched: number;
  scanned: number;
  truncated: boolean;
  downsampled: boolean;
  agg?: string;
}

export interface LatestPoint {
  key: string;
  device_id: string;
  timestamp: string;
  type: string;
  value: unknown;
  quality: number;
}

export interface BufferedSinkStats {
  name: string;
  state: string;
  durable: string;
  delivered_points: number;
  failed_batches: number;
  discarded_messages: number;
  pending: number;
  lost: number;
  lag_seconds: number;
  last_error?: string;
}

export interface StreamStatus {
  stream: string;
  messages: number;
  bytes: number;
  first_time: string;
  last_time: string;
  max_bytes: number;
  max_age: string;
  usage_ratio: number;
}

export interface RollupStats {
  stream: string;
  interval: string;
  processed_points: number;
  late_points: number;
  written_records: number;
  open_windows: number;
  records: number;
  bytes: number;
  last_error?: string;
}

export interface StorageAlert {
  kind: 'backlog' | 'data_lost' | 'disk' | string;
  sink?: string;
  level: 'warning' | 'critical' | string;
  message: string;
  value: number;
}

export interface StorageStatus {
  enabled: boolean;
  alerts?: StorageAlert[];
  stream?: StreamStatus;
  rollup?: RollupStats;
  sinks?: BufferedSinkStats[];
  error?: string;
}

export type SinkAction = 'pause' | 'resume' | 'skip' | 'replay';

export const dataService = {
  async listSeries(deviceId?: string, offset = 0, limit = 1000): Promise<{ series: SeriesInfo[]; total: number }> {
    const params = new URLSearchParams({ offset: String(offset), limit: String(limit) });
    if (deviceId) params.set('device_id', deviceId);
    const res = await api.get(`/data/series?${params.toString()}`);
    return res.data.data;
  },

  async latest(deviceId: string): Promise<LatestPoint[]> {
    const res = await api.get(`/data/latest?${new URLSearchParams({ device_id: deviceId }).toString()}`);
    return res.data.data.points ?? [];
  },

  async history(q: HistoryQuery): Promise<SeriesResult> {
    const res = await api.get(`/data/history?${buildHistoryParams(q)}`);
    return res.data.data;
  },

  /** 下载 CSV（通过 axios 携带令牌，再触发浏览器下载） */
  async exportCsv(q: HistoryQuery): Promise<{ truncated: boolean }> {
    const res = await api.get(`/data/export?${buildHistoryParams({ ...q, limit: undefined, agg: undefined })}`, {
      responseType: 'blob',
      timeout: 120000,
    });
    const disposition: string = res.headers['content-disposition'] ?? '';
    const match = /filename="([^"]+)"/.exec(disposition);
    const url = URL.createObjectURL(res.data as Blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = match?.[1] ?? 'history.csv';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    return { truncated: res.headers['x-truncated'] === 'true' };
  },

  async storageStatus(): Promise<StorageStatus> {
    const res = await api.get('/storage/status');
    return res.data.data;
  },

  async controlSink(name: string, action: SinkAction, fromTime?: Date): Promise<void> {
    const body = action === 'replay' && fromTime ? { from_time: fromTime.toISOString() } : {};
    await api.post(`/storage/sinks/${encodeURIComponent(name)}/${action}`, body);
  },
};
