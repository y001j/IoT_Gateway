export interface Plugin {
  id?: number;
  name: string;
  type: string; // 'adapter' | 'sink'
  version: string;
  status: string; // 'running' | 'stopped' | 'error'
  description: string;
  author?: string;
  config?: Record<string, string | number | boolean>;
  enabled: boolean;
  path?: string;
  port?: number;
  last_start?: string;
  last_stop?: string;
  error_count?: number;
  created_at?: string;
  updated_at?: string;
}

export interface PluginListRequest {
  page: number;
  page_size: number;
  type?: string;
  status?: string;
  search?: string;
}

export interface PluginListResponse {
  data: Plugin[];
  pagination: {
    total: number;
    offset: number;
    limit: number;
  };
}

export interface PluginConfigRequest {
  config: Record<string, string | number | boolean>;
}

export interface PluginConfigValidationResponse {
  valid: boolean;
  errors?: string[];
}

export interface PluginOperationResponse {
  success: boolean;
  message: string;
  status: string;
}

export interface PluginStats {
  plugin_id: number;
  data_points_total: number;
  data_points_hour: number;
  errors_total: number;
  errors_hour: number;
  uptime_seconds: number;
  average_latency: number;
  memory_usage: number;
  cpu_usage: number;
  last_update: string;
}

export interface PluginLog {
  id: number;
  plugin_id: number;
  level: string;
  message: string;
  source: string;
  timestamp: string;
}

export interface PluginLogRequest {
  page: number;
  page_size: number;
  level?: string;
  source?: string;
  from?: string;
  to?: string;
}

export type PluginStatus = 'running' | 'stopped' | 'error';
export type PluginType = 'adapter' | 'sink';
export type PluginAction = 'start' | 'stop' | 'restart'; 