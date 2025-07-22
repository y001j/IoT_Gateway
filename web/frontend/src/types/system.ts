export interface SystemStatus {
  status: string;
  uptime: number;
  version: string;
  build_time: string;
  go_version: string;
  cpu_usage: number;
  memory_usage: number;
  disk_usage: number;
  goroutines: number;
  nats_connected: boolean;
  jetstream_enabled: boolean;
  active_plugins: number;
  active_rules: number;
}

export interface SystemMetrics {
  timestamp: string;
  cpu_percent: number;
  memory_percent: number;
  memory_used: number;
  memory_total: number;
  disk_percent: number;
  disk_used: number;
  disk_total: number;
  network_rx: number;
  network_tx: number;
  process_count: number;
  thread_count: number;
}

export interface HealthCheck {
  status: string;
  checks: {
    [key: string]: {
      status: string;
      message: string;
      timestamp: string;
    };
  };
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