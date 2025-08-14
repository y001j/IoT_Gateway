# 配置系统迁移指南

## 概述

IoT Gateway 已升级为统一配置系统，提供了更好的配置验证、默认值管理和错误处理。本指南将帮助您将现有配置迁移到新格式。

## 主要改进

### 1. 统一配置解析
- 所有适配器和接收器使用统一的配置解析器
- 自动验证配置字段和类型
- 提供清晰的错误信息和字段要求

### 2. 标准化配置结构
- 所有组件继承基础配置结构（name, type, enabled, description, tags）
- 统一的时间单位处理（支持 `5s`, `10m`, `1h` 等格式）
- 标准化的批处理和缓冲配置

### 3. 向后兼容性
- 支持旧配置格式的自动转换
- 在使用旧格式时会显示警告信息
- 逐步迁移，不强制立即升级

## 配置格式对比

### Modbus 适配器

#### 旧格式
```yaml
southbound:
  adapters:
    - name: "modbus-sensor"
      type: "modbus"
      config:
        name: "modbus-sensor"
        mode: "tcp"
        address: "127.0.0.1:502"
        timeout_ms: 5000
        interval_ms: 2000
        registers:
          - key: "temperature"
            device_id: 1
            function: 3
            address: 0
            quantity: 1
            type: "float"
            scale: 0.1
```

#### 新格式
```yaml
southbound:
  adapters:
    - name: "modbus-sensor"
      type: "modbus"
      enabled: true
      description: "Production Modbus sensor"
      interval: 2s
      timeout: 5s
      host: "127.0.0.1"
      port: 502
      protocol: "tcp"
      slave_id: 1
      registers:
        - address: 0
          type: "holding_register"
          data_type: "float32"
          device_id: "temperature-sensor-001"
          key: "temperature"
          scale: 0.1
          offset: 0.0
```

### HTTP 适配器

#### 旧格式
```yaml
- name: "weather-api"
  type: "http"
  config:
    name: "weather-api"
    device_id: "weather-001"
    interval_ms: 60000
    timeout_ms: 10000
    endpoints:
      - url: "https://api.weather.com/data"
        method: "GET"
        headers:
          Authorization: "Bearer TOKEN"
```

#### 新格式
```yaml
- name: "weather-api"
  type: "http"
  enabled: true
  description: "External weather API"
  interval: 60s
  timeout: 10s
  url: "https://api.weather.com/data"
  method: "GET"
  headers:
    Authorization: "Bearer TOKEN"
  parser:
    type: "json"
    json_path:
      temperature: "$.main.temp"
      humidity: "$.main.humidity"
```

### MQTT 接收器

#### 旧格式
```yaml
northbound:
  sinks:
    - name: "mqtt-cloud"
      type: "mqtt"
      config:
        name: "mqtt-cloud"
        batch_size: 10
        buffer_size: 1000
        params:
          broker: "tcp://mqtt.server:1883"
          client_id: "gateway-001"
          topic_tpl: "iot/data/%s/%s"
          qos: 1
```

#### 新格式
```yaml
northbound:
  sinks:
    - name: "mqtt-cloud"
      type: "mqtt"
      enabled: true
      description: "Cloud MQTT publisher"
      batch_size: 10
      buffer_size: 1000
      flush_timeout: 5s
      broker: "tcp://mqtt.server:1883"
      client_id: "gateway-001"
      topic: "iot/data"
      qos: 1
      retain: false
```

## 迁移步骤

### 步骤 1: 备份现有配置
```bash
cp config.yaml config.yaml.backup
```

### 步骤 2: 使用新格式创建配置
参考 `config_new_format.yaml` 示例文件，创建新的配置文件。

### 步骤 3: 验证配置
```bash
./bin/gateway -config config_new_format.yaml -validate-only
```

### 步骤 4: 逐步迁移
您可以混合使用新旧格式，系统会自动处理向后兼容性：
- 新格式配置将使用增强的验证和默认值
- 旧格式配置会显示警告但仍然工作
- 建议逐个组件迁移，确保稳定性

### 步骤 5: 测试和部署
在测试环境中验证新配置的功能，然后部署到生产环境。

## 新配置特性

### 1. 字段验证
新配置系统提供以下验证：
- `required`: 必填字段
- `min=N/max=N`: 数值范围验证
- `range=N-M`: 数值范围验证
- `oneof=a b c`: 枚举值验证
- `url`: URL格式验证
- `port`: 端口号验证（1-65535）

### 2. 默认值
每种配置类型都有预定义的默认值：
```go
config.GetDefaultModbusConfig()
config.GetDefaultHTTPConfig()
config.GetDefaultMQTTSinkConfig()
// ... 等等
```

### 3. 时间格式
支持人性化的时间格式：
- `5s` = 5秒
- `10m` = 10分钟  
- `2h` = 2小时
- `1000ms` = 1000毫秒

### 4. 配置热重载
```go
configManager.Hot().Enable()
configManager.Watch("southbound.adapters", func(newConfig interface{}) {
    // 处理配置变更
})
```

## 常见问题

### Q: 旧配置还能继续使用吗？
A: 是的，系统完全向后兼容。旧配置会自动转换为新格式，但建议逐步迁移以获得更好的验证和错误处理。

### Q: 如何知道配置是否有效？
A: 新系统提供详细的验证错误信息，包括字段名称、期望值和实际值。

### Q: 迁移过程中出现问题怎么办？
A: 系统会记录详细的迁移日志，包括兼容性警告和转换信息。如果出现问题，可以回退到备份配置。

### Q: 新格式有什么性能优势？
A: 新系统减少了配置解析时的内存分配和重复代码，提供了更好的性能和内存使用。

## Sidecar 插件配置迁移

### 当前状态
Sidecar 插件配置**正在迁移中**，目前支持两种方式：

1. **旧格式**：使用独立的 JSON 文件（如 `plugins/modbus-sidecar-isp.json`）
2. **新格式**：集成到主配置文件中，支持统一验证和管理

### Sidecar 插件配置对比

#### 旧格式（plugins/modbus-sidecar-isp.json）
```json
{
  "name": "modbus-sensor",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "isp-sidecar",
  "entry": "modbus-sidecar/modbus-sidecar.exe",
  "description": "Modbus适配器（ISP Sidecar模式）",
  "isp_port": 50052
}
```

#### 新格式（集成到主配置文件）
```yaml
southbound:
  adapters:
    - name: "modbus-sidecar"
      type: "sidecar"
      enabled: true
      description: "Modbus adapter via ISP sidecar"
      tags:
        mode: "sidecar"
        protocol: "modbus"
      
      # ISP Sidecar 特定配置
      isp_port: 50052
      isp_timeout: 30s
      entry: "modbus-sidecar/modbus-sidecar.exe"
      auto_restart: true
      max_retries: 3
      
      # 传递给 sidecar 的配置
      plugin_config:
        host: "192.168.1.100"
        port: 502
        protocol: "tcp"
        slave_id: 1
        registers:
          - address: 0
            type: "holding_register"
            data_type: "float32"
            device_id: "sensor-001"
            key: "temperature"
```

### 新格式优势

1. **统一管理**：所有配置集中在一个文件中
2. **类型验证**：支持配置字段验证和类型检查
3. **默认值**：自动应用合理的默认配置
4. **错误处理**：提供清晰的配置错误信息
5. **热重载**：支持配置动态更新
6. **标签支持**：增强的元数据和分类管理

### 迁移策略

#### 阶段 1：并行支持（当前）
- 系统同时支持新旧两种格式
- 新配置优先级高于旧配置
- 使用旧格式时显示迁移提示

#### 阶段 2：渐进迁移
- 提供自动迁移工具
- 验证新配置的正确性
- 逐步废弃旧格式支持

#### 阶段 3：完全迁移
- 移除旧格式支持代码
- 统一使用新配置系统
- 优化性能和内存使用

### 外部插件支持
新系统还支持外部插件配置：

```yaml
southbound:
  adapters:
    - name: "custom-external"
      type: "external"
      enabled: true
      description: "Custom external plugin"
      command: "./plugins/custom/custom-adapter"
      args:
        - "--config"
        - "config.json"
      environment:
        LOG_LEVEL: "debug"
        DATA_DIR: "./data"
      working_dir: "./plugins/custom"
      timeout: 60s
      auto_restart: true
      max_retries: 5
```

## 技术实现

### 配置解析器
```go
type ConfigParser[T any] struct {
    defaults T
}

func (p *ConfigParser[T]) Parse(raw json.RawMessage) (*T, error)
func (p *ConfigParser[T]) Validate(config *T) error
```

### 验证标签
```go
type ModbusConfig struct {
    Host     string        `json:"host" validate:"required"`
    Port     int           `json:"port" validate:"port"`
    Interval time.Duration `json:"interval" validate:"min=100ms"`
    Protocol string        `json:"protocol" validate:"oneof=tcp rtu"`
}
```

### 使用示例
```go
parser := config.NewParserWithDefaults(config.GetDefaultModbusConfig())
cfg, err := parser.Parse(rawConfig)
if err != nil {
    log.Error().Err(err).Msg("配置解析失败")
    return err
}
```