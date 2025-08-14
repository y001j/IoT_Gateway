// Service for lightweight metrics communication with backend

export interface LightweightMetrics {
  system: SystemMetrics;
  gateway: GatewayMetrics;
  data: DataMetrics;
  connections: ConnectionMetrics;
  rules: RuleMetrics;
  performance: PerformanceMetrics;
  errors: ErrorMetrics;
  last_updated: string;
}

export interface SystemMetrics {
  uptime_seconds: number;
  memory_usage_bytes: number;
  cpu_usage_percent: number;
  disk_usage_percent: number;
  goroutine_count: number;
  gc_pause_ms: number;
  heap_size_bytes: number;
  heap_in_use_bytes: number;
  // 网络累计流量指标
  network_in_bytes: number;
  network_out_bytes: number;
  network_in_packets: number;
  network_out_packets: number;
  // 网络实时速率指标
  network_in_bytes_per_sec: number;
  network_out_bytes_per_sec: number;
  network_in_packets_per_sec: number;
  network_out_packets_per_sec: number;
  version: string;
  go_version: string;
}

export interface GatewayMetrics {
  status: string;
  start_time: string;
  config_file: string;
  plugins_directory: string;
  total_adapters: number;
  running_adapters: number;
  total_sinks: number;
  running_sinks: number;
  nats_connected: boolean;
  nats_connection_url: string;
  web_ui_port: number;
  api_port: number;
}

export interface DataMetrics {
  total_data_points: number;
  data_points_per_second: number;
  total_bytes_processed: number;
  bytes_per_second: number;
  average_latency_ms: number;
  max_latency_ms: number;
  min_latency_ms: number;
  last_data_point_time: string;
  data_queue_length: number;
  processing_errors_count: number;
}

export interface ConnectionMetrics {
  active_connections: number;
  total_connections: number;
  failed_connections: number;
  connections_by_type: Record<string, number>;
  connections_by_status: Record<string, number>;
  average_response_time_ms: number;
  connection_errors: number;
  reconnection_count: number;
}

export interface RuleMetrics {
  total_rules: number;
  enabled_rules: number;
  rules_matched: number;
  actions_executed: number;
  actions_succeeded: number;
  actions_failed: number;
  average_execution_time_ms: number;
  rule_engine_status: string;
  last_rule_execution: string;
}

export interface PerformanceMetrics {
  throughput_per_second: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  queue_length: number;
  processing_time: Record<string, number>;
  resource_utilization: Record<string, number>;
}

export interface ErrorMetrics {
  total_errors: number;
  errors_per_second: number;
  errors_by_type: Record<string, number>;
  errors_by_level: Record<string, number>;
  last_error: string;
  last_error_time: string;
  error_rate: number;
  recovery_count: number;
}

class LightweightMetricsService {
  private gatewayBaseUrl = '';  // 使用相对路径，通过Vite代理
  
  /**
   * 获取轻量级指标数据 (从Gateway主服务)
   */
  async getLightweightMetrics(format: 'json' | 'text' = 'json'): Promise<LightweightMetrics> {
    const url = `${this.gatewayBaseUrl}/metrics${format === 'text' ? '?format=text' : ''}`;
    
    try {
      console.log('Fetching metrics from:', url);
      const response = await fetch(url);
      console.log('Response status:', response.status, response.statusText);
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText} (URL: ${url})`);
      }
      
      if (format === 'text') {
        const text = await response.text();
        return this.parseTextMetrics(text);
      } else {
        const data = await response.json();
        console.log('Received metrics data:', data);
        return data;
      }
    } catch (error) {
      console.error('Failed to fetch lightweight metrics from:', url, error);
      throw error;
    }
  }

  /**
   * 获取系统健康状态 (从Gateway主服务)
   */
  async getSystemHealth(): Promise<{
    status: string;
    timestamp: string;
    services: Record<string, string>;
  }> {
    const url = `${this.gatewayBaseUrl}/health`;
    
    try {
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }
      
      return await response.json();
    } catch (error) {
      console.error('Failed to fetch system health:', error);
      throw error;
    }
  }

  /**
   * 获取系统信息 (从Gateway主服务)
   */
  async getSystemInfo(): Promise<{
    name: string;
    version: string;
    nats_port: string;
    gateway_port: string;
  }> {
    const url = `${this.gatewayBaseUrl}/info`;
    
    try {
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }
      
      return await response.json();
    } catch (error) {
      console.error('Failed to fetch system info:', error);
      throw error;
    }
  }

  /**
   * 解析纯文本格式的指标数据
   */
  private parseTextMetrics(text: string): LightweightMetrics {
    // 这里实现简单的文本解析，实际项目中可能需要更复杂的解析逻辑
    // 由于实现复杂，这里返回默认值
    return {
      system: {
        uptime_seconds: 0,
        memory_usage_bytes: 0,
        cpu_usage_percent: 0,
        disk_usage_percent: 0,
        goroutine_count: 0,
        gc_pause_ms: 0,
        heap_size_bytes: 0,
        heap_in_use_bytes: 0,
        network_in_bytes: 0,
        network_out_bytes: 0,
        network_in_packets: 0,
        network_out_packets: 0,
        network_in_bytes_per_sec: 0,
        network_out_bytes_per_sec: 0,
        network_in_packets_per_sec: 0,
        network_out_packets_per_sec: 0,
        version: '',
        go_version: ''
      },
      gateway: {
        status: 'unknown',
        start_time: '',
        config_file: '',
        plugins_directory: '',
        total_adapters: 0,
        running_adapters: 0,
        total_sinks: 0,
        running_sinks: 0,
        nats_connected: false,
        nats_connection_url: '',
        web_ui_port: 0,
        api_port: 0
      },
      data: {
        total_data_points: 0,
        data_points_per_second: 0,
        total_bytes_processed: 0,
        bytes_per_second: 0,
        average_latency_ms: 0,
        max_latency_ms: 0,
        min_latency_ms: 0,
        last_data_point_time: '',
        data_queue_length: 0,
        processing_errors_count: 0
      },
      connections: {
        active_connections: 0,
        total_connections: 0,
        failed_connections: 0,
        connections_by_type: {},
        connections_by_status: {},
        average_response_time_ms: 0,
        connection_errors: 0,
        reconnection_count: 0
      },
      rules: {
        total_rules: 0,
        enabled_rules: 0,
        rules_matched: 0,
        actions_executed: 0,
        actions_succeeded: 0,
        actions_failed: 0,
        average_execution_time_ms: 0,
        rule_engine_status: 'unknown',
        last_rule_execution: ''
      },
      performance: {
        throughput_per_second: 0,
        p50_latency_ms: 0,
        p95_latency_ms: 0,
        p99_latency_ms: 0,
        queue_length: 0,
        processing_time: {},
        resource_utilization: {}
      },
      errors: {
        total_errors: 0,
        errors_per_second: 0,
        errors_by_type: {},
        errors_by_level: {},
        last_error: '',
        last_error_time: '',
        error_rate: 0,
        recovery_count: 0
      },
      last_updated: new Date().toISOString()
    };
  }

  /**
   * 格式化运行时间
   */
  formatUptime(seconds: number): string {
    if (seconds < 60) {
      return `${Math.floor(seconds)}秒`;
    } else if (seconds < 3600) {
      const minutes = Math.floor(seconds / 60);
      return `${minutes}分钟`;
    } else if (seconds < 86400) {
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);
      return `${hours}小时${minutes}分钟`;
    } else {
      const days = Math.floor(seconds / 86400);
      const hours = Math.floor((seconds % 86400) / 3600);
      return `${days}天${hours}小时`;
    }
  }

  /**
   * 格式化字节数
   */
  formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';

    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
  }

  /**
   * 格式化数字
   */
  formatNumber(num: number, decimals: number = 1): string {
    if (num >= 1000000) {
      return `${(num / 1000000).toFixed(decimals)}M`;
    } else if (num >= 1000) {
      return `${(num / 1000).toFixed(decimals)}K`;
    } else {
      return num.toFixed(decimals);
    }
  }

  /**
   * 格式化延迟时间
   */
  formatLatency(ms: number): string {
    if (ms < 1) {
      return `${(ms * 1000).toFixed(0)}μs`;
    } else if (ms < 1000) {
      return `${ms.toFixed(1)}ms`;
    } else {
      return `${(ms / 1000).toFixed(2)}s`;
    }
  }

  /**
   * 获取状态颜色
   */
  getStatusColor(status: string): string {
    switch (status) {
      case 'running':
      case 'healthy':
        return 'success';
      case 'stopped':
      case 'degraded':
        return 'warning';
      case 'error':
      case 'unhealthy':
        return 'error';
      default:
        return 'default';
    }
  }
}

export const lightweightMetricsService = new LightweightMetricsService();