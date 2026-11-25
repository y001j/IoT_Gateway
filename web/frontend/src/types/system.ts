// 与后端 internal/web/models/system.go 的 SystemStatus 一致（/api/v1/system/status）
export interface SystemStatus {
  status: string;
  uptime: string;
  version: string;
  start_time: string;
  cpu_usage: number;
  memory_usage: number;
  disk_usage: number;
  network_in: number;
  network_out: number;
  active_connections: number;
  total_connections: number;
}

// 与后端 SystemMetrics 一致（/api/v1/system/metrics）
export interface SystemMetrics {
  timestamp: string;
  data_points_per_second: number;
  active_connections: number;
  error_rate: number;
  response_time_avg: number;
  memory_usage: number;
  cpu_usage: number;
  disk_usage: number;
  network_in_bytes: number;
  network_out_bytes: number;
}

export interface HealthCheckItem {
  name: string;
  status: string;
  message?: string;
  timestamp?: string;
}

// 与后端 /api/v1/system/health 一致：checks 是数组
export interface HealthCheck {
  status: string;
  service?: string;
  version?: string;
  timestamp?: string;
  checks: HealthCheckItem[];
}

export interface SystemConfig {
  gateway: {
    id: string;
    http_port: number;
    log_level: string;
    nats_url: string;
    plugins_dir: string;
    metrics: {
      enabled: boolean;
      port: number;
    };
  };
  nats: {
    enabled: boolean;
    embedded: boolean;
    host: string;
    port: number;
    cluster_port: number;
    monitor_port: number;
    jetstream: {
      enabled: boolean;
      store_dir: string;
      max_memory: number;
      max_file: number;
    };
    cluster: {
      enabled: boolean;
      name: string;
      routes: string[];
    };
    tls: {
      enabled: boolean;
      cert_file: string;
      key_file: string;
      ca_file: string;
    };
  };
  web_ui: {
    enabled: boolean;
    port: number;
    auth: {
      jwt_secret: string;
      token_duration: string;
      refresh_duration: string;
      max_login_attempts: number;
      lockout_duration: string;
      enable_two_factor: boolean;
      session_timeout: string;
      password_min_length: number;
      bcrypt_cost: number;
    };
  };
  database: {
    sqlite: {
      path: string;
      max_open_conns: number;
      max_idle_conns: number;
      conn_max_lifetime: string;
      conn_max_idle_time: string;
    };
  };
  security: {
    api_keys: {
      enabled: boolean;
      keys: Array<{
        name: string;
        key: string;
        enabled: boolean;
        expires_at?: string;
      }>;
    };
    https: {
      enabled: boolean;
      cert_file: string;
      key_file: string;
      redirect_http: boolean;
    };
    cors: {
      enabled: boolean;
      allowed_origins: string[];
      allowed_methods: string[];
      allowed_headers: string[];
      credentials: boolean;
    };
  };
  rules: {
    dir: string;
  };
}

export interface ConfigValidationResponse {
  valid: boolean;
  errors?: string[];
  warnings?: string[];
}