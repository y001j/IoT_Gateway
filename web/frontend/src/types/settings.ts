// 设置相关类型定义

// 网关配置
export interface GatewayConfig {
  http_port: number;
  https_port?: number;
  log_level: 'debug' | 'info' | 'warn' | 'error';
  nats_url: string;
  plugins_dir: string;
  max_connections: number;
  read_timeout: string;
  write_timeout: string;
}

// NATS配置
export interface NatsConfig {
  mode: 'embedded' | 'external';
  external_url?: string;
  embedded_port?: number;
  jetstream: {
    enabled: boolean;
    store_dir: string;
    max_memory: string;
    max_file: string;
  };
  cluster: {
    enabled: boolean;
    port?: number;
    routes?: string[];
  };
}

// 数据库配置
export interface DatabaseConfig {
  sqlite_path: string;
  connection_pool: {
    max_open_conns: number;
    max_idle_conns: number;
    conn_max_lifetime: string;
  };
  backup: {
    enabled: boolean;
    interval: string;
    retention: string;
  };
}

// 安全配置
export interface SecurityConfig {
  authentication: {
    enabled: boolean;
    method: 'jwt' | 'basic' | 'oauth';
    jwt_secret?: string;
    token_expire: string;
  };
  api_keys: ApiKey[];
  https: {
    enabled: boolean;
    cert_file?: string;
    key_file?: string;
    auto_cert?: boolean;
  };
  cors: {
    enabled: boolean;
    allowed_origins: string[];
    allowed_methods: string[];
  };
}

export interface ApiKey {
  id: string;
  name: string;
  key: string;
  permissions: string[];
  created_at: string;
  expires_at?: string;
  last_used?: string;
}

// 监控配置
export interface MonitoringConfig {
  metrics: {
    enabled: boolean;
    interval: string;
    retention: string;
    endpoint: string;
  };
  performance: {
    cpu_threshold: number;
    memory_threshold: number;
    disk_threshold: number;
    connection_threshold: number;
  };
  health_check: {
    enabled: boolean;
    interval: string;
    timeout: string;
  };
}

// 告警配置
export interface AlertConfig {
  channels: AlertChannel[];
  rules: AlertRule[];
  global_settings: {
    enabled: boolean;
    rate_limit: string;
    quiet_hours?: {
      enabled: boolean;
      start: string;
      end: string;
    };
  };
}

export interface AlertChannel {
  id: string;
  name: string;
  type: 'email' | 'sms' | 'webhook' | 'slack';
  enabled: boolean;
  config: Record<string, any>;
}

export interface AlertRule {
  id: string;
  name: string;
  condition: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  channels: string[];
  enabled: boolean;
}

// 日志配置
export interface LogConfig {
  level: 'debug' | 'info' | 'warn' | 'error';
  format: 'json' | 'text';
  output: string[];
  rotation: {
    enabled: boolean;
    max_size: string;
    max_age: string;
    max_backups: number;
  };
  remote: {
    enabled: boolean;
    endpoint?: string;
    format?: string;
    auth?: {
      type: string;
      config: Record<string, any>;
    };
  };
}

// 用户管理
export interface User {
  id: string;
  username: string;
  email: string;
  role: UserRole;
  role_id: string;
  created_at: string;
  last_login?: string;
  enabled: boolean;
}

export interface UserRole {
  id: string;
  name: string;
  permissions: Permission[];
  description?: string;
}

export interface Permission {
  id: string;
  name: string;
  resource: string;
  action: string;
  description?: string;
}

// API权限配置
export interface ApiPermissionConfig {
  rate_limiting: {
    enabled: boolean;
    global_rate: number;
    user_rate: number;
    api_key_rate: number;
  };
  access_control: {
    enabled: boolean;
    default_deny: boolean;
    rules: ApiAccessRule[];
  };
}

export interface ApiAccessRule {
  id: string;
  path: string;
  methods: string[];
  roles: string[];
  api_keys: string[];
  enabled: boolean;
}

// API权限相关类型
export interface ApiPermissionRule {
  id: string;
  name: string;
  path: string;
  method: string;
  roles: string[];
  enabled: boolean;
}

export interface ApiEndpoint {
  path: string;
  method: string;
  description: string;
  category: string;
}

// 审计日志
export interface AuditConfig {
  enabled: boolean;
  events: string[];
  retention: string;
  storage: {
    type: 'database' | 'file' | 'remote';
    config: Record<string, any>;
  };
}

export interface AuditLog {
  id: string;
  timestamp: string;
  user_id?: string;
  user_name?: string;
  action: string;
  resource: string;
  details: Record<string, any>;
  ip_address: string;
  user_agent: string;
}

// 完整设置状态
export interface SettingsState {
  gateway: GatewayConfig;
  nats: NatsConfig;
  database: DatabaseConfig;
  security: SecurityConfig;
  monitoring: MonitoringConfig;
  alerts: AlertConfig;
  logs: LogConfig;
  users: User[];
  roles: UserRole[];
  api_permissions: ApiPermissionConfig;
  audit: AuditConfig;
}

// API响应类型
export interface SettingsResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

// 配置测试结果
export interface ConfigTestResult {
  success: boolean;
  message: string;
  details?: Record<string, any>;
}