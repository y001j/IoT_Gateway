import api from './api';
import type {
  SystemStatus,
  SystemMetrics,
  HealthCheck,
  SystemConfig,
  ConfigValidationResponse
} from '../types/system';

import type { LightweightMetrics } from './lightweightMetricsService';
export const systemService = {
  // 获取系统状态
  async getStatus(): Promise<SystemStatus> {
    const response = await api.get('/system/status');
    return response.data.data;
  },

  // 获取系统指标
  async getMetrics(): Promise<SystemMetrics> {
    const response = await api.get('/system/metrics');
    return response.data.data;
  },

  // 获取健康检查
  async getHealth(): Promise<HealthCheck> {
    const response = await api.get('/system/health');
    return response.data.data;
  },

  // 获取系统配置
  async getConfig(): Promise<SystemConfig> {
    const response = await api.get('/system/config');
    return response.data.data;
  },

  // 更新系统配置
  async updateConfig(config: SystemConfig): Promise<void> {
    await api.put('/system/config', config);
  },

  // 验证配置
  async validateConfig(config: SystemConfig): Promise<ConfigValidationResponse> {
    const response = await api.post('/system/config/validate', config);
    return response.data.data;
  },

  // 重启系统
  async restart(delay?: number): Promise<void> {
    await api.post('/system/restart', { delay: delay || 0 });
  },

  // 获取轻量级指标 (代理到Gateway主服务)
  async getLightweightMetrics(): Promise<LightweightMetrics> {
    try {
      // 使用环境变量中配置的Gateway URL
      const gatewayUrl = import.meta.env.VITE_GATEWAY_URL || 'http://localhost:8080';
      const response = await fetch(`${gatewayUrl}/metrics`);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }
      return await response.json() as LightweightMetrics;
    } catch (error) {
      console.error('Failed to fetch lightweight metrics:', error);
      throw error;
    }
  }
};