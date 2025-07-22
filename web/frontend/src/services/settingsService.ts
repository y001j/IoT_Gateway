import axios from 'axios';
import type {
  SettingsState,
  SettingsResponse,
  ConfigTestResult,
  GatewayConfig,
  NatsConfig,
  DatabaseConfig,
  SecurityConfig,
  MonitoringConfig,
  AlertConfig,
  LogConfig,
  User,
  UserRole,
  ApiPermissionConfig,
  AuditConfig,
  AuditLog,
  ApiKey,
  AlertChannel
} from '../types/settings';

const API_BASE = '/api/v1';

class SettingsService {
  // 获取所有设置
  async getAllSettings(): Promise<SettingsResponse<SettingsState>> {
    const response = await axios.get(`${API_BASE}/settings`);
    return response.data;
  }

  // 核心配置管理
  async getGatewayConfig(): Promise<SettingsResponse<GatewayConfig>> {
    const response = await axios.get(`${API_BASE}/settings/gateway`);
    return response.data;
  }

  async updateGatewayConfig(config: GatewayConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/gateway`, config);
    return response.data;
  }

  async getNatsConfig(): Promise<SettingsResponse<NatsConfig>> {
    const response = await axios.get(`${API_BASE}/settings/nats`);
    return response.data;
  }

  async updateNatsConfig(config: NatsConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/nats`, config);
    return response.data;
  }

  async getDatabaseConfig(): Promise<SettingsResponse<DatabaseConfig>> {
    const response = await axios.get(`${API_BASE}/settings/database`);
    return response.data;
  }

  async updateDatabaseConfig(config: DatabaseConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/database`, config);
    return response.data;
  }

  async getSecurityConfig(): Promise<SettingsResponse<SecurityConfig>> {
    const response = await axios.get(`${API_BASE}/settings/security`);
    return response.data;
  }

  async updateSecurityConfig(config: SecurityConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/security`, config);
    return response.data;
  }

  // API密钥管理
  async getApiKeys(): Promise<SettingsResponse<ApiKey[]>> {
    const response = await axios.get(`${API_BASE}/settings/api-keys`);
    return response.data;
  }

  async createApiKey(data: { name: string; permissions: string[]; expires_at?: string }): Promise<SettingsResponse<ApiKey>> {
    const response = await axios.post(`${API_BASE}/settings/api-keys`, data);
    return response.data;
  }

  async updateApiKey(id: string, data: Partial<ApiKey>): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/api-keys/${id}`, data);
    return response.data;
  }

  async deleteApiKey(id: string): Promise<SettingsResponse<void>> {
    const response = await axios.delete(`${API_BASE}/settings/api-keys/${id}`);
    return response.data;
  }

  // 监控与告警
  async getMonitoringConfig(): Promise<SettingsResponse<MonitoringConfig>> {
    const response = await axios.get(`${API_BASE}/settings/monitoring`);
    return response.data;
  }

  async updateMonitoringConfig(config: MonitoringConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/monitoring`, config);
    return response.data;
  }

  async getAlertConfig(): Promise<SettingsResponse<AlertConfig>> {
    const response = await axios.get(`${API_BASE}/settings/alerts`);
    return response.data;
  }

  async updateAlertConfig(config: AlertConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/alerts`, config);
    return response.data;
  }

  async getAlertChannels(): Promise<SettingsResponse<AlertChannel[]>> {
    const response = await axios.get(`${API_BASE}/settings/alert-channels`);
    return response.data;
  }

  async createAlertChannel(channel: Omit<AlertChannel, 'id'>): Promise<SettingsResponse<AlertChannel>> {
    const response = await axios.post(`${API_BASE}/settings/alert-channels`, channel);
    return response.data;
  }

  async updateAlertChannel(id: string, channel: Partial<AlertChannel>): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/alert-channels/${id}`, channel);
    return response.data;
  }

  async deleteAlertChannel(id: string): Promise<SettingsResponse<void>> {
    const response = await axios.delete(`${API_BASE}/settings/alert-channels/${id}`);
    return response.data;
  }

  async testAlertChannel(id: string): Promise<SettingsResponse<ConfigTestResult>> {
    const response = await axios.post(`${API_BASE}/settings/alert-channels/${id}/test`);
    return response.data;
  }

  async getLogConfig(): Promise<SettingsResponse<LogConfig>> {
    const response = await axios.get(`${API_BASE}/settings/logs`);
    return response.data;
  }

  async updateLogConfig(config: LogConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/logs`, config);
    return response.data;
  }

  // 用户与权限管理
  async getUsers(): Promise<SettingsResponse<User[]>> {
    const response = await axios.get(`${API_BASE}/users`);
    return response.data;
  }

  async createUser(data: { username: string; email: string; password: string; role_id: string }): Promise<SettingsResponse<User>> {
    const response = await axios.post(`${API_BASE}/users`, data);
    return response.data;
  }

  async updateUser(id: string, data: Partial<User>): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/users/${id}`, data);
    return response.data;
  }

  async deleteUser(id: string): Promise<SettingsResponse<void>> {
    const response = await axios.delete(`${API_BASE}/users/${id}`);
    return response.data;
  }

  async changePassword(id: string, data: { old_password: string; new_password: string }): Promise<SettingsResponse<void>> {
    const response = await axios.post(`${API_BASE}/users/${id}/change-password`, data);
    return response.data;
  }

  async getRoles(): Promise<SettingsResponse<UserRole[]>> {
    const response = await axios.get(`${API_BASE}/roles`);
    return response.data;
  }

  async createRole(role: Omit<UserRole, 'id'>): Promise<SettingsResponse<UserRole>> {
    const response = await axios.post(`${API_BASE}/roles`, role);
    return response.data;
  }

  async updateRole(id: string, role: Partial<UserRole>): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/roles/${id}`, role);
    return response.data;
  }

  async deleteRole(id: string): Promise<SettingsResponse<void>> {
    const response = await axios.delete(`${API_BASE}/roles/${id}`);
    return response.data;
  }

  async getApiPermissions(): Promise<SettingsResponse<ApiPermissionConfig>> {
    const response = await axios.get(`${API_BASE}/settings/api-permissions`);
    return response.data;
  }

  async updateApiPermissions(config: ApiPermissionConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/api-permissions`, config);
    return response.data;
  }

  // 审计日志
  async getAuditConfig(): Promise<SettingsResponse<AuditConfig>> {
    const response = await axios.get(`${API_BASE}/settings/audit`);
    return response.data;
  }

  async updateAuditConfig(config: AuditConfig): Promise<SettingsResponse<void>> {
    const response = await axios.put(`${API_BASE}/settings/audit`, config);
    return response.data;
  }

  async getAuditLogs(params?: {
    page?: number;
    page_size?: number;
    user_id?: string;
    action?: string;
    start_time?: string;
    end_time?: string;
  }): Promise<SettingsResponse<{ data: AuditLog[]; total: number }>> {
    const response = await axios.get(`${API_BASE}/audit-logs`, { params });
    return response.data;
  }

  // 配置测试
  async testNatsConnection(config: NatsConfig): Promise<SettingsResponse<ConfigTestResult>> {
    const response = await axios.post(`${API_BASE}/settings/test/nats`, config);
    return response.data;
  }

  async testDatabaseConnection(config: DatabaseConfig): Promise<SettingsResponse<ConfigTestResult>> {
    const response = await axios.post(`${API_BASE}/settings/test/database`, config);
    return response.data;
  }

  async testLogEndpoint(config: LogConfig): Promise<SettingsResponse<ConfigTestResult>> {
    const response = await axios.post(`${API_BASE}/settings/test/log-endpoint`, config);
    return response.data;
  }

  // 配置备份与恢复
  async backupConfig(): Promise<SettingsResponse<{ backup_id: string; download_url: string }>> {
    const response = await axios.post(`${API_BASE}/settings/backup`);
    return response.data;
  }

  async restoreConfig(backup_file: File): Promise<SettingsResponse<void>> {
    const formData = new FormData();
    formData.append('backup', backup_file);
    const response = await axios.post(`${API_BASE}/settings/restore`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    });
    return response.data;
  }

  // 系统信息
  async getSystemInfo(): Promise<SettingsResponse<{
    version: string;
    build_time: string;
    go_version: string;
    os: string;
    arch: string;
    uptime: string;
    memory_usage: number;
    cpu_usage: number;
  }>> {
    const response = await axios.get(`${API_BASE}/system/info`);
    return response.data;
  }
}

export const settingsService = new SettingsService();