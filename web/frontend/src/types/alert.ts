// Alert types and interfaces

export interface Alert {
  id: string;
  title: string;
  description: string;
  level: 'info' | 'warning' | 'error' | 'critical';
  status: 'active' | 'acknowledged' | 'resolved';
  source: string;
  data: Record<string, string | number | boolean>;
  createdAt: Date;
  updatedAt: Date;
  acknowledgedAt?: Date;
  acknowledgedBy?: string;
  acknowledgedComment?: string;
  resolvedAt?: Date;
  resolvedBy?: string;
  resolvedComment?: string;
  
  // Rule engine related fields
  rule_id?: string;
  rule_name?: string;
  device_id?: string;
  key?: string;
  value?: unknown;
  tags?: Record<string, string>;
  notification_channels?: string[];
  priority?: number;
  auto_resolve?: boolean;
}

export interface AlertCreateRequest {
  title: string;
  description: string;
  level: string;
  source: string;
  data?: Record<string, string | number | boolean>;
}

export interface AlertUpdateRequest {
  title?: string;
  description?: string;
  level?: string;
  status?: string;
  data?: Record<string, string | number | boolean>;
}

export interface AlertListRequest {
  page: number;
  pageSize: number;
  level?: string;
  status?: string;
  source?: string;
  search?: string;
  startTime?: Date;
  endTime?: Date;
}

export interface AlertStats {
  total: number;
  active: number;
  acknowledged: number;
  resolved: number;
  byLevel: Record<string, number>;
  bySource: Record<string, number>;
  recentTrends: AlertTrend[];
}

export interface AlertTrend {
  date: Date;
  count: number;
}

// Alert Rules
export interface AlertRule {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  level: string;
  condition: AlertCondition;
  notificationChannels: string[];
  createdAt: Date;
  updatedAt: Date;
}

export interface AlertCondition {
  type: 'threshold' | 'absence' | 'change' | 'custom';
  field: string;
  operator: 'gt' | 'lt' | 'eq' | 'ne' | 'gte' | 'lte' | 'contains' | 'older_than';
  value: string | number | boolean | null;
  timeWindow?: string;
  aggregation?: 'avg' | 'sum' | 'count' | 'max' | 'min';
}

export interface AlertRuleCreateRequest {
  name: string;
  description: string;
  enabled: boolean;
  level: string;
  condition: AlertCondition;
  notificationChannels: string[];
}

export interface AlertRuleUpdateRequest {
  name?: string;
  description?: string;
  enabled?: boolean;
  level?: string;
  condition?: AlertCondition;
  notificationChannels?: string[];
}

export interface AlertRuleTestResponse {
  ruleId: string;
  triggered: boolean;
  message: string;
  testedAt: Date;
}

// Notification Channels
export interface NotificationChannel {
  id: string;
  name: string;
  type: 'email' | 'webhook' | 'sms' | 'slack' | 'dingtalk';
  enabled: boolean;
  config: Record<string, string | number | boolean>;
  createdAt: Date;
  updatedAt: Date;
}

export interface NotificationChannelCreateRequest {
  name: string;
  type: string;
  enabled: boolean;
  config: Record<string, string | number | boolean>;
}

export interface NotificationChannelUpdateRequest {
  name?: string;
  type?: string;
  enabled?: boolean;
  config?: Record<string, string | number | boolean>;
}

// API Response Types
export interface AlertListResponse {
  code: number;
  data: {
    alerts: Alert[];
    total: number;
    page: number;
    pageSize: number;
  };
}

export interface AlertResponse {
  code: number;
  data: Alert;
}

export interface AlertStatsResponse {
  code: number;
  data: AlertStats;
}

export interface AlertRuleListResponse {
  code: number;
  data: AlertRule[];
}

export interface AlertRuleResponse {
  code: number;
  data: AlertRule;
}

export interface NotificationChannelListResponse {
  code: number;
  data: NotificationChannel[];
}

export interface NotificationChannelResponse {
  code: number;
  data: NotificationChannel;
}

export interface AlertRuleTestResponse {
  code: number;
  data: {
    ruleId: string;
    triggered: boolean;
    message: string;
    testedAt: Date;
  };
}

// Alert level configurations
export const ALERT_LEVELS = [
  { value: 'info', label: '信息', color: 'blue' },
  { value: 'warning', label: '警告', color: 'orange' },
  { value: 'error', label: '错误', color: 'red' },
  { value: 'critical', label: '严重', color: 'red' },
] as const;

export const ALERT_STATUSES = [
  { value: 'active', label: '活跃', color: 'red' },
  { value: 'acknowledged', label: '已确认', color: 'orange' },
  { value: 'resolved', label: '已解决', color: 'green' },
] as const;

export const NOTIFICATION_CHANNEL_TYPES = [
  { value: 'email', label: '邮件', icon: '📧' },
  { value: 'webhook', label: 'Webhook', icon: '🔗' },
  { value: 'sms', label: '短信', icon: '📱' },
  { value: 'slack', label: 'Slack', icon: '💬' },
  { value: 'dingtalk', label: '钉钉', icon: '🔔' },
] as const;

export const ALERT_OPERATORS = [
  { value: 'gt', label: '大于 (>)' },
  { value: 'lt', label: '小于 (<)' },
  { value: 'eq', label: '等于 (=)' },
  { value: 'ne', label: '不等于 (≠)' },
  { value: 'gte', label: '大于等于 (≥)' },
  { value: 'lte', label: '小于等于 (≤)' },
  { value: 'contains', label: '包含' },
  { value: 'older_than', label: '早于' },
] as const;