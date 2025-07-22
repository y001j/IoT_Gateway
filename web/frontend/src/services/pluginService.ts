import api from './api';
import type {
  Plugin,
  PluginListRequest,
  PluginListResponse,
  PluginConfigValidationResponse,
  PluginOperationResponse,
  PluginStats,
  PluginLog,
  PluginLogRequest,
  PluginAction
} from '../types/plugin';

export const pluginService = {
  // 获取插件列表
  async getPlugins(params: PluginListRequest): Promise<PluginListResponse> {
    const response = await api.get('/plugins', { params });
    return response.data.data || response.data;
  },

  // 获取单个插件详情
  async getPlugin(name: string): Promise<Plugin> {
    const response = await api.get(`/plugins/${name}`);
    return response.data.data || response.data;
  },

  // 启动插件
  async startPlugin(name: string): Promise<PluginOperationResponse> {
    const response = await api.post(`/plugins/${name}/start`);
    return response.data.data || response.data;
  },

  // 停止插件
  async stopPlugin(name: string): Promise<PluginOperationResponse> {
    const response = await api.post(`/plugins/${name}/stop`);
    return response.data.data || response.data;
  },

  // 重启插件
  async restartPlugin(name: string): Promise<PluginOperationResponse> {
    const response = await api.post(`/plugins/${name}/restart`);
    return response.data.data || response.data;
  },

  // 删除插件
  async deletePlugin(name: string): Promise<void> {
    await api.delete(`/plugins/${name}`);
  },

  // 获取插件配置
  async getPluginConfig(name: string): Promise<Record<string, any>> {
    const response = await api.get(`/plugins/${name}/config`);
    return response.data.data || response.data;
  },

  // 更新插件配置
  async updatePluginConfig(name: string, config: Record<string, any>): Promise<void> {
    await api.put(`/plugins/${name}/config`, { config });
  },

  // 验证插件配置
  async validatePluginConfig(name: string, config: Record<string, any>): Promise<PluginConfigValidationResponse> {
    const response = await api.post(`/plugins/${name}/config/validate`, { config });
    return response.data.data || response.data;
  },

  // 获取插件日志
  async getPluginLogs(name: string, params: PluginLogRequest): Promise<{ data: PluginLog[]; total: number }> {
    const response = await api.get(`/plugins/${name}/logs`, { params });
    const result = response.data.data || response.data;
    return {
      data: result.data || result,
      total: result.pagination?.total || result.length || 0
    };
  },

  // 获取插件统计
  async getPluginStats(name: string): Promise<PluginStats> {
    const response = await api.get(`/plugins/${name}/stats`);
    return response.data.data || response.data;
  },

  // 执行插件操作（通用方法）
  async executePluginAction(name: string, action: PluginAction): Promise<PluginOperationResponse> {
    switch (action) {
      case 'start':
        return this.startPlugin(name);
      case 'stop':
        return this.stopPlugin(name);
      case 'restart':
        return this.restartPlugin(name);
      default:
        throw new Error(`Unknown action: ${action}`);
    }
  }
}; 