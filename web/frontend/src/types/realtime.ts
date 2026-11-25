// WebSocket 实时推送的数据类型（见后端 internal/web/api/websocket_handler.go）
import type { SystemStatus } from './system';

/** 后端计算的派生值（display_value 用于展示，summary 为摘要文本） */
export interface DerivedValues {
  display_value?: unknown;
  summary?: string;
  [field: string]: unknown;
}

/** 复合数据展开后的分量信息 */
export interface ComponentInfo {
  parent_key: string;
  component_key: string;
  component_label: string;
  component_unit: string;
  component_color?: string;
}

/** 推送的增强 IoT 数据（enhanceIoTData） */
export interface EnhancedIoTData {
  device_id?: string;
  key?: string;
  value?: unknown;
  type?: string;
  timestamp?: string;
  quality?: number;
  tags?: Record<string, string>;
  data_type?: string;
  derived_values?: DerivedValues;
  component_info?: ComponentInfo;
  [field: string]: unknown;
}

/** 前端保存的一条 IoT 数据 */
export interface IoTDataPoint {
  timestamp: Date;
  subject: string;
  data: EnhancedIoTData & { data_type: string; derived_values: DerivedValues };
  raw_message?: unknown;
}

/** 系统指标（后端 SystemMetrics，另附前端换算的百分比字段） */
export interface RealtimeSystemMetrics {
  timestamp: Date | string;
  data_points_per_second?: number;
  active_connections?: number;
  error_rate?: number;
  response_time_avg?: number;
  cpu_usage?: number;
  memory_usage?: number;
  disk_usage?: number;
  network_in_bytes?: number;
  network_out_bytes?: number;
  cpu_percent: number;
  memory_percent: number;
  disk_percent: number;
  [field: string]: unknown;
}

/** 系统状态（后端 SystemStatus，推送的指标会合并进来） */
export type RealtimeSystemStatus = Partial<SystemStatus> & {
  cpu_percent?: number;
  memory_percent?: number;
  disk_percent?: number;
  [field: string]: unknown;
};

/** 实时告警事件 */
export interface RealtimeAlert {
  id: string | number;
  timestamp: string;
  type?: 'alert_created' | 'alert_resolved' | string;
  data?: Record<string, unknown>;
  level?: string;
  title?: string;
  message?: string;
  [field: string]: unknown;
}
