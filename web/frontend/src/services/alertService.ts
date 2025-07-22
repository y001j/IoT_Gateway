import api from './api';
import type {
  Alert,
  AlertCreateRequest,
  AlertUpdateRequest,
  AlertListRequest,
  AlertListResponse,
  AlertResponse,
  AlertStats,
  AlertStatsResponse,
  AlertRule,
  AlertRuleCreateRequest,
  AlertRuleUpdateRequest,
  AlertRuleListResponse,
  AlertRuleResponse,
  AlertRuleTestResponse,
  NotificationChannel,
  NotificationChannelCreateRequest,
  NotificationChannelUpdateRequest,
  NotificationChannelListResponse,
  NotificationChannelResponse,
} from '../types/alert';

class AlertService {
  private readonly baseUrl = '/alerts';

  // Alert management
  async getAlerts(params: AlertListRequest): Promise<{ alerts: Alert[]; total: number; page: number; pageSize: number }> {
    const searchParams = new URLSearchParams({
      page: params.page.toString(),
      page_size: params.pageSize.toString(),
    });

    if (params.level) searchParams.append('level', params.level);
    if (params.status) searchParams.append('status', params.status);
    if (params.source) searchParams.append('source', params.source);
    if (params.search) searchParams.append('search', params.search);
    if (params.startTime) searchParams.append('start_time', params.startTime.toISOString());
    if (params.endTime) searchParams.append('end_time', params.endTime.toISOString());

    try {
      const fullUrl = `${this.baseUrl}?${searchParams}`;
      console.log('📡 发送告警列表请求:', fullUrl);
      console.log('🌐 API基础URL:', api.defaults.baseURL);
      console.log('🔑 当前认证状态:', !!api.defaults.headers.common?.Authorization);
      
      // 记录请求开始时间
      const startTime = Date.now();
      console.log('⏰ 请求开始:', new Date().toISOString());
      
      const response = await api.get<AlertListResponse>(fullUrl);
      
      const duration = Date.now() - startTime;
      console.log('⏰ 请求完成，耗时:', duration, 'ms');
      console.log('✅ 告警列表响应状态:', response.status);
      console.log('✅ 告警列表响应头:', response.headers);
      console.log('✅ 告警列表响应数据:', response.data);
      
      // 转换时间字段名称
      const { alerts, ...rest } = response.data.data;
      const transformedAlerts = alerts.map((alert: any) => ({
        ...alert,
        createdAt: new Date(alert.created_at || alert.createdAt || ''),
        updatedAt: new Date(alert.updated_at || alert.updatedAt || ''),
        acknowledgedAt: alert.acknowledged_at ? new Date(alert.acknowledged_at) : undefined,
        resolvedAt: alert.resolved_at ? new Date(alert.resolved_at) : undefined,
      }));
      
      return { alerts: transformedAlerts, ...rest };
    } catch (error: any) {
      console.error('❌ 告警列表请求失败 - 完整错误信息:', {
        message: error.message,
        code: error.code,
        name: error.name,
        status: error.response?.status,
        statusText: error.response?.statusText,
        data: error.response?.data,
        headers: error.response?.headers,
        config: {
          url: error.config?.url,
          method: error.config?.method,
          baseURL: error.config?.baseURL,
          timeout: error.config?.timeout,
          headers: error.config?.headers
        },
        request: {
          readyState: error.request?.readyState,
          status: error.request?.status,
          statusText: error.request?.statusText,
          responseURL: error.request?.responseURL
        }
      });
      throw error;
    }
  }

  async getAlert(id: string): Promise<Alert> {
    const response = await api.get<AlertResponse>(`${this.baseUrl}/${id}`);
    const alert: any = response.data.data;
    return {
      ...alert,
      createdAt: new Date(alert.created_at || alert.createdAt || ''),
      updatedAt: new Date(alert.updated_at || alert.updatedAt || ''),
      acknowledgedAt: alert.acknowledged_at ? new Date(alert.acknowledged_at) : undefined,
      resolvedAt: alert.resolved_at ? new Date(alert.resolved_at) : undefined,
    };
  }

  async createAlert(data: AlertCreateRequest): Promise<Alert> {
    const response = await api.post<AlertResponse>(this.baseUrl, data);
    return response.data.data;
  }

  async updateAlert(id: string, data: AlertUpdateRequest): Promise<Alert> {
    const response = await api.put<AlertResponse>(`${this.baseUrl}/${id}`, data);
    return response.data.data;
  }

  async deleteAlert(id: string): Promise<void> {
    await api.delete(`${this.baseUrl}/${id}`);
  }

  async acknowledgeAlert(id: string, comment?: string): Promise<void> {
    await api.post(`${this.baseUrl}/${id}/acknowledge`, { comment });
  }

  async resolveAlert(id: string, comment?: string): Promise<void> {
    await api.post(`${this.baseUrl}/${id}/resolve`, { comment });
  }

  async getAlertStats(): Promise<AlertStats> {
    try {
      console.log('📡 发送告警统计请求:', `${this.baseUrl}/stats`);
      const response = await api.get<AlertStatsResponse>(`${this.baseUrl}/stats`);
      console.log('✅ 告警统计响应:', response.data);
      return response.data.data;
    } catch (error) {
      console.error('❌ 告警统计请求失败:', error);
      throw error;
    }
  }

  // Alert Rules management
  async getAlertRules(): Promise<AlertRule[]> {
    try {
      console.log('📡 发送告警规则请求:', `${this.baseUrl}/rules`);
      const response = await api.get<AlertRuleListResponse>(`${this.baseUrl}/rules`);
      console.log('✅ 告警规则响应:', response.data);
      return response.data.data;
    } catch (error) {
      console.error('❌ 告警规则请求失败:', error);
      throw error;
    }
  }

  async createAlertRule(data: AlertRuleCreateRequest): Promise<AlertRule> {
    const response = await api.post<AlertRuleResponse>(`${this.baseUrl}/rules`, data);
    return response.data.data;
  }

  async updateAlertRule(id: string, data: AlertRuleUpdateRequest): Promise<AlertRule> {
    const response = await api.put<AlertRuleResponse>(`${this.baseUrl}/rules/${id}`, data);
    return response.data.data;
  }

  async deleteAlertRule(id: string): Promise<void> {
    await api.delete(`${this.baseUrl}/rules/${id}`);
  }

  async testAlertRule(id: string, testData: Record<string, any>): Promise<{ ruleId: string; triggered: boolean; message: string; testedAt: Date }> {
    const response = await api.post<AlertRuleTestResponse>(`${this.baseUrl}/rules/${id}/test`, { data: testData });
    return response.data.data;
  }

  // Notification Channels management
  async getNotificationChannels(): Promise<NotificationChannel[]> {
    try {
      console.log('📡 发送通知渠道请求:', `${this.baseUrl}/channels`);
      const response = await api.get<NotificationChannelListResponse>(`${this.baseUrl}/channels`);
      console.log('✅ 通知渠道响应:', response.data);
      return response.data.data;
    } catch (error) {
      console.error('❌ 通知渠道请求失败:', error);
      throw error;
    }
  }

  async createNotificationChannel(data: NotificationChannelCreateRequest): Promise<NotificationChannel> {
    const response = await api.post<NotificationChannelResponse>(`${this.baseUrl}/channels`, data);
    return response.data.data;
  }

  async updateNotificationChannel(id: string, data: NotificationChannelUpdateRequest): Promise<NotificationChannel> {
    const response = await api.put<NotificationChannelResponse>(`${this.baseUrl}/channels/${id}`, data);
    return response.data.data;
  }

  async deleteNotificationChannel(id: string): Promise<void> {
    await api.delete(`${this.baseUrl}/channels/${id}`);
  }

  async testNotificationChannel(id: string): Promise<void> {
    await api.post(`${this.baseUrl}/channels/${id}/test`);
  }

  // Helper methods for alert level and status handling
  getAlertLevelColor(level: string): string {
    switch (level) {
      case 'info':
        return 'blue';
      case 'warning':
        return 'orange';
      case 'error':
        return 'red';
      case 'critical':
        return 'red';
      default:
        return 'default';
    }
  }

  getAlertStatusColor(status: string): string {
    switch (status) {
      case 'active':
        return 'red';
      case 'acknowledged':
        return 'orange';
      case 'resolved':
        return 'green';
      default:
        return 'default';
    }
  }

  getAlertLevelText(level: string): string {
    switch (level) {
      case 'info':
        return '信息';
      case 'warning':
        return '警告';
      case 'error':
        return '错误';
      case 'critical':
        return '严重';
      default:
        return level;
    }
  }

  getAlertStatusText(status: string): string {
    switch (status) {
      case 'active':
        return '活跃';
      case 'acknowledged':
        return '已确认';
      case 'resolved':
        return '已解决';
      default:
        return status;
    }
  }

  formatAlertData(data: Record<string, any>): string {
    try {
      return JSON.stringify(data, null, 2);
    } catch {
      return String(data);
    }
  }
}

export const alertService = new AlertService();
export default alertService;