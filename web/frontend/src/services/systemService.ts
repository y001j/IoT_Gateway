import api from './api';
import type {
  SystemStatus,
  SystemMetrics,
  HealthCheck,
  SystemConfig,
  ConfigValidationResponse
} from '../types/system';

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
  }
};