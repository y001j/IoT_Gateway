import { BaseResponse } from './common';

// 适配器状态信息
export interface AdapterStatus {
  name: string;
  type: string;
  status: 'running' | 'stopped' | 'error';
  health: 'healthy' | 'degraded' | 'unhealthy' | 'unknown';
  health_message: string;
  start_time?: string;
  last_data_time?: string;
  connection_uptime: number;
  data_points_count: number;
  errors_count: number;
  last_error: string;
  response_time_ms: number;
  config?: Record<string, string | number | boolean>;
  tags: Record<string, string>;
}

// 连接器状态信息
export interface SinkStatus {
  name: string;
  type: string;
  status: 'running' | 'stopped' | 'error';
  health: 'healthy' | 'degraded' | 'unhealthy' | 'unknown';
  health_message: string;
  start_time?: string;
  last_publish_time?: string;
  connection_uptime: number;
  messages_published: number;
  errors_count: number;
  last_error: string;
  response_time_ms: number;
  config?: Record<string, string | number | boolean>;
  tags: Record<string, string>;
}

// 连接概览信息
export interface ConnectionOverview {
  total_adapters: number;
  running_adapters: number;
  healthy_adapters: number;
  total_sinks: number;
  running_sinks: number;
  healthy_sinks: number;
  total_data_points_per_sec: number;
  total_errors_per_sec: number;
  active_connections: number;
  system_health: 'healthy' | 'degraded' | 'unhealthy' | 'stopped';
  top_adapters_by_traffic: DataFlowMetrics[];
}

// 数据流指标
export interface DataFlowMetrics {
  adapter_name: string;
  device_id: string;
  key: string;
  data_points_per_sec: number;
  bytes_per_sec: number;
  latency_ms: number;
  error_rate: number;
  last_value: any; // 实际数据值
  last_timestamp: string; // 时间戳字符串
}

// 适配器诊断信息
export interface AdapterDiagnostics {
  adapter_name: string;
  connection_test?: ConnectionTestResult;
  config_validation?: ConfigValidationResult;
  health_checks: HealthCheckResult[];
  performance_test?: PerformanceTestResult;
  recommendations: string[];
}

// 连接测试结果
export interface ConnectionTestResult {
  success: boolean;
  response_time: number;
  error: string;
  details: Record<string, string | number | boolean>;
  timestamp: string;
}

// 配置验证结果
export interface ConfigValidationResult {
  valid: boolean;
  errors: string[];
  warnings: string[];
  suggestions: string[];
  timestamp: string;
}

// 健康检查结果
export interface HealthCheckResult {
  check_name: string;
  status: 'pass' | 'warn' | 'fail';
  message: string;
  duration: number;
  timestamp: string;
  details: Record<string, string | number | boolean>;
}

// 性能测试结果
export interface PerformanceTestResult {
  throughput_per_sec: number;
  avg_latency: number;
  max_latency: number;
  min_latency: number;
  error_rate: number;
  test_duration: number;
  sample_count: number;
  timestamp: string;
}

// API响应类型
export interface AdapterStatusResponse extends BaseResponse {
  data: {
    adapters: AdapterStatus[];
    sinks: SinkStatus[];
    overview: ConnectionOverview;
  };
}

export interface DataFlowMetricsResponse extends BaseResponse {
  data: {
    metrics: DataFlowMetrics[];
    time_range: string;
    granularity: string;
    total_points: number;
  };
}

export interface AdapterDiagnosticsResponse extends BaseResponse {
  data: AdapterDiagnostics;
}

// 状态和健康状态相关的配置
export const ADAPTER_STATUS_CONFIG = {
  running: { color: 'success', text: '运行中' },
  stopped: { color: 'default', text: '已停止' },
  error: { color: 'error', text: '错误' },
} as const;

export const HEALTH_STATUS_CONFIG = {
  healthy: { color: 'success', text: '健康' },
  degraded: { color: 'warning', text: '降级' },
  unhealthy: { color: 'error', text: '不健康' },
  unknown: { color: 'default', text: '未知' },
} as const;

export const SYSTEM_HEALTH_CONFIG = {
  healthy: { color: 'success', text: '系统健康' },
  degraded: { color: 'warning', text: '系统降级' },
  unhealthy: { color: 'error', text: '系统异常' },
  stopped: { color: 'default', text: '系统停止' },
} as const;

// 时间范围选项
export const TIME_RANGE_OPTIONS = [
  { label: '5分钟', value: '5m' },
  { label: '15分钟', value: '15m' },
  { label: '1小时', value: '1h' },
  { label: '6小时', value: '6h' },
  { label: '24小时', value: '24h' },
  { label: '7天', value: '7d' },
] as const;

// 监控指标类型
export type MetricType = 'data_points' | 'throughput' | 'latency' | 'errors' | 'uptime';

// 监控图表数据点
export interface MetricDataPoint {
  timestamp: string;
  value: number;
  label?: string;
}

// 监控图表配置
export interface ChartConfig {
  title: string;
  type: 'line' | 'bar' | 'area';
  unit: string;
  color: string;
  yAxisName: string;
}

// 实时监控数据
export interface RealtimeMonitoringData {
  adapters: AdapterStatus[];
  sinks: SinkStatus[];
  overview: ConnectionOverview;
  dataFlow: DataFlowMetrics[];
  timestamp: string;
}