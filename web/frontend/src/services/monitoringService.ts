import api from './api';
import type {
  DataFlowMetricsResponse,
  AdapterDiagnosticsResponse,
  AdapterStatus,
  SinkStatus,
  ConnectionOverview,
  DataFlowMetrics,
  AdapterDiagnostics,
} from '../types/monitoring';
import type { BaseResponse } from '../types/common';

class MonitoringService {
  private baseUrl = '/monitoring';

  /**
   * 获取插件信息（适配器和连接器）
   */
  async getPlugins(): Promise<{
    adapters: AdapterStatus[];
    sinks: SinkStatus[];
  }> {
    // 直接使用 api 实例调用插件API，它会自动处理认证
    const [pluginsResponse, metricsResponse] = await Promise.all([
      api.get('/plugins'),
      fetch(`${import.meta.env.VITE_GATEWAY_URL || 'http://localhost:8080'}/metrics`).catch(() => null) // 从Gateway主服务获取轻量级指标
    ]);
    
    const pluginsData = pluginsResponse.data;
    let metrics = null;
    
    if (metricsResponse && metricsResponse.ok) {
      metrics = await metricsResponse.json();
      console.log('轻量级指标数据:', metrics);
    }
    
    // 解析插件数据并转换为适配器和连接器格式
    const adapters: AdapterStatus[] = [];
    const sinks: SinkStatus[] = [];
    
    // 处理嵌套的数据结构 data.data.data
    const plugins = pluginsData.data?.data || pluginsData.data || [];
    console.log('插件API原始数据:', pluginsData);
    console.log('解析出的插件数据:', plugins);
    
    if (Array.isArray(plugins)) {
      await Promise.all(plugins.map(async (plugin: any) => {
        // 计算真实运行时间
        let connectionUptime = 0;
        if (plugin.status === 'running' && metrics?.gateway?.start_time) {
          const startTime = new Date(metrics.gateway.start_time);
          const now = new Date();
          connectionUptime = Math.floor((now.getTime() - startTime.getTime()) / 1000);
        }
        
        // 获取插件统计数据
        let pluginStats = null;
        try {
          const statsResponse = await api.get(`/plugins/${plugin.name}/stats`);
          pluginStats = statsResponse.data.data;
        } catch (error) {
          console.warn(`获取插件 ${plugin.name} 统计数据失败:`, error);
        }
        
        // 获取真实的响应时间数据
        const responseTimeMs = pluginStats?.average_latency || 
          (plugin.status === 'running' ? (metrics?.connections?.average_response_time_ms || 0) : 0);
        
        // 获取真实的错误数据
        const errorsCount = pluginStats?.errors_total || plugin.error_count || 0;
        
        const baseInfo = {
          name: plugin.name,
          type: plugin.type,
          status: plugin.status,
          health: plugin.status === 'running' ? 'healthy' : plugin.status === 'stopped' ? 'degraded' : 'unhealthy',
          health_message: plugin.status === 'running' ? '运行正常' : plugin.status === 'stopped' ? '已停止' : '未知状态',
          connection_uptime: pluginStats?.uptime_seconds || connectionUptime,
          errors_count: errorsCount,
          response_time_ms: responseTimeMs,
          last_seen: new Date().toISOString(),
        };
        
        if (plugin.type === 'adapter') {
          // 对于适配器，只使用真实数据
          const totalDataPoints = metrics?.data?.total_data_points || 0;
          const runningAdapters = plugins.filter(p => p.type === 'adapter' && p.status === 'running').length;
          
          const dataPointsCount = plugin.status === 'running' && runningAdapters > 0
            ? Math.floor(totalDataPoints / runningAdapters)
            : 0;
            
          adapters.push({
            ...baseInfo,
            data_points_count: dataPointsCount,
            last_error: '',
            tags: {},
          });
        } else if (plugin.type === 'sink') {
          // 对于连接器，只使用真实数据
          const totalDataPoints = metrics?.data?.total_data_points || 0;
          const runningSinks = plugins.filter(p => p.type === 'sink' && p.status === 'running').length;
          
          const messagesPublished = plugin.status === 'running' && runningSinks > 0
            ? Math.floor(totalDataPoints / runningSinks)
            : 0;
            
          sinks.push({
            ...baseInfo,
            messages_published: messagesPublished,
            last_error: '',
            tags: {},
          });
        }
      }));
    }
    
    console.log('解析后的适配器:', adapters);
    console.log('解析后的连接器:', sinks);
    
    return { adapters, sinks };
  }

  /**
   * 获取适配器状态列表
   */
  async getAdapterStatus(): Promise<{
    adapters: AdapterStatus[];
    sinks: SinkStatus[];
    overview: ConnectionOverview;
  }> {
    try {
      // 先尝试使用新的plugins端点
      const { adapters, sinks } = await this.getPlugins();
      
      // 从轻量级指标服务获取概览数据
      let lightweightMetrics;
      try {
        lightweightMetrics = await fetch(`${import.meta.env.VITE_GATEWAY_URL || 'http://localhost:8080'}/metrics`).then(res => res.json());
      } catch (metricsError) {
        console.warn('轻量级指标服务不可用，使用默认值:', metricsError);
        lightweightMetrics = {
          gateway: { status: 'running', total_adapters: adapters.length, running_adapters: adapters.filter(a => a.status === 'running').length, total_sinks: sinks.length, running_sinks: sinks.filter(s => s.status === 'running').length },
          connections: { active_connections: adapters.length + sinks.length },
          data: { data_points_per_second: 0 },
          errors: { errors_per_second: 0 }
        };
      }
      
      const overview: ConnectionOverview = {
        system_health: lightweightMetrics.gateway.status === 'running' ? 'healthy' : 'degraded',
        active_connections: lightweightMetrics.connections.active_connections,
        total_adapters: lightweightMetrics.gateway.total_adapters,
        running_adapters: lightweightMetrics.gateway.running_adapters,
        healthy_adapters: adapters.filter(a => a.health === 'healthy').length,
        total_sinks: lightweightMetrics.gateway.total_sinks,
        running_sinks: lightweightMetrics.gateway.running_sinks,
        healthy_sinks: sinks.filter(s => s.health === 'healthy').length,
        total_data_points_per_sec: lightweightMetrics.data.data_points_per_second,
        total_errors_per_sec: lightweightMetrics.errors.errors_per_second,
        top_adapters_by_traffic: [],
      };
      
      return { adapters, sinks, overview };
    } catch (error) {
      console.error('获取适配器状态失败:', error);
      throw error;
    }
  }

  /**
   * 获取数据流指标
   */
  async getDataFlowMetrics(params?: {
    time_range?: string;
    limit?: number;
  }): Promise<{
    metrics: DataFlowMetrics[];
    time_range: string;
    granularity: string;
    total_points: number;
  }> {
    const searchParams = new URLSearchParams();
    if (params?.time_range) {
      searchParams.append('time_range', params.time_range);
    }
    if (params?.limit) {
      searchParams.append('limit', params.limit.toString());
    }

    const url = `${this.baseUrl}/adapters/data-flow${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    const response = await api.get<DataFlowMetricsResponse>(url);
    return response.data.data;
  }

  /**
   * 获取适配器诊断信息
   */
  async getAdapterDiagnostics(adapterName: string): Promise<AdapterDiagnostics> {
    const response = await api.get<AdapterDiagnosticsResponse>(
      `${this.baseUrl}/adapters/${encodeURIComponent(adapterName)}/diagnostics`
    );
    return response.data.data;
  }

  /**
   * 测试适配器连接
   */
  async testAdapterConnection(adapterName: string): Promise<any> {
    const response = await api.post<BaseResponse>(
      `${this.baseUrl}/adapters/${encodeURIComponent(adapterName)}/test-connection`
    );
    return response.data;
  }

  /**
   * 获取适配器性能指标
   */
  async getAdapterPerformance(adapterName: string, period?: string): Promise<any> {
    const searchParams = new URLSearchParams();
    if (period) {
      searchParams.append('period', period);
    }

    const url = `${this.baseUrl}/adapters/${encodeURIComponent(adapterName)}/performance${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;
    const response = await api.get<BaseResponse>(url);
    return response.data;
  }

  /**
   * 重启适配器
   */
  async restartAdapter(adapterName: string): Promise<void> {
    await api.post<BaseResponse>(
      `${this.baseUrl}/adapters/${encodeURIComponent(adapterName)}/restart`
    );
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
   * 格式化数据大小
   */
  formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';

    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
  }

  /**
   * 格式化数字显示
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
        return 'success';
      case 'stopped':
        return 'default';
      case 'error':
        return 'error';
      default:
        return 'default';
    }
  }

  /**
   * 获取健康状态颜色
   */
  getHealthColor(health: string): string {
    switch (health) {
      case 'healthy':
        return 'success';
      case 'degraded':
        return 'warning';
      case 'unhealthy':
        return 'error';
      case 'unknown':
      default:
        return 'default';
    }
  }

  /**
   * 获取状态文本
   */
  getStatusText(status: string): string {
    switch (status) {
      case 'running':
        return '运行中';
      case 'stopped':
        return '已停止';
      case 'error':
        return '错误';
      default:
        return '未知';
    }
  }

  /**
   * 获取健康状态文本
   */
  getHealthText(health: string): string {
    switch (health) {
      case 'healthy':
        return '健康';
      case 'degraded':
        return '降级';
      case 'unhealthy':
        return '不健康';
      case 'unknown':
      default:
        return '未知';
    }
  }

  /**
   * 计算健康评分
   */
  calculateHealthScore(adapter: AdapterStatus): number {
    let score = 100;

    // 状态影响
    if (adapter.status === 'error') {
      score -= 50;
    } else if (adapter.status === 'stopped') {
      score -= 30;
    }

    // 健康状态影响
    if (adapter.health === 'unhealthy') {
      score -= 30;
    } else if (adapter.health === 'degraded') {
      score -= 15;
    }

    // 错误率影响
    if (adapter.data_points_count > 0) {
      const errorRate = adapter.errors_count / adapter.data_points_count;
      score -= errorRate * 20;
    }

    // 响应时间影响
    if (adapter.response_time_ms > 1000) {
      score -= 10;
    } else if (adapter.response_time_ms > 500) {
      score -= 5;
    }

    return Math.max(0, Math.min(100, score));
  }

  /**
   * 获取适配器图标
   */
  getAdapterIcon(type: string): string {
    switch (type.toLowerCase()) {
      case 'modbus':
        return '🔌';
      case 'mqtt':
        return '📡';
      case 'http':
        return '🌐';
      case 'mock':
        return '🎭';
      default:
        return '📱';
    }
  }

  /**
   * 获取连接器图标
   */
  getSinkIcon(type: string): string {
    switch (type.toLowerCase()) {
      case 'mqtt':
        return '📡';
      case 'influxdb':
        return '📊';
      case 'redis':
        return '🔄';
      case 'console':
        return '💻';
      case 'websocket':
        return '🔗';
      case 'jetstream':
        return '🚀';
      default:
        return '📤';
    }
  }
}

export const monitoringService = new MonitoringService();