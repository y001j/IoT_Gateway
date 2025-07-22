# NATS订阅器Sink使用指南

## 概述

NATS订阅器Sink是一个高度可配置的northbound sink，它可以从NATS消息总线订阅不同类型的数据，并将这些数据转发到其他sink进行处理。这使得数据流更加灵活，可以实现复杂的数据路由和处理逻辑。

## 主要特性

### 1. 多数据类型支持
- **原始数据** (`raw`): 从adapter发布的原始设备数据
- **规则数据** (`rule`): 规则引擎处理后的数据
- **告警数据** (`alert`): 系统和规则引擎产生的告警
- **系统数据** (`system`): 系统事件和状态信息

### 2. 灵活的订阅配置
- 支持通配符主题订阅
- 支持队列组负载均衡
- 可配置多个订阅源
- 支持主题级别的数据转换

### 3. 强大的数据处理能力
- 批处理支持
- 数据过滤和转换
- 多目标sink转发
- 实时和延迟处理模式

### 4. 高性能设计
- 异步消息处理
- 可配置的缓冲区大小
- 批量数据转发
- 内存优化的数据通道

## 配置结构

### 基本配置
```yaml
northbound:
  sinks:
    - name: "my_nats_subscriber"
      type: "nats_subscriber"
      batch_size: 50
      params:
        # NATS订阅器特定配置
        subscriptions: []      # 订阅配置列表
        target_sinks: []       # 目标sink配置列表
        buffer_size: 1000      # 内部缓冲区大小
        batch_size: 50         # 批处理大小
        flush_interval: "1s"   # 刷新间隔
        filter_rules: []       # 可选的过滤规则
```

### 订阅配置
```yaml
subscriptions:
  - subject: "iot.data.>"           # NATS主题（支持通配符）
    data_type: "raw"               # 数据类型：raw/rule/alert/system
    enabled: true                  # 是否启用
    queue_group: "processors"      # 可选的队列组
    transform:                     # 可选的数据转换
      static_tags:
        source: "sensor"
        category: "environmental"
    tags:                          # 附加标签
      priority: "high"
```

### 目标Sink配置
```yaml
target_sinks:
  - name: "console_output"
    type: "console"
    params:
      format: "json"
  
  - name: "influxdb_storage"
    type: "influxdb"
    params:
      url: "http://localhost:8086"
      token: "your-token"
      org: "your-org"
      bucket: "sensor_data"
```

### 过滤规则配置
```yaml
filter_rules:
  - field: "device_id"             # 字段名
    operator: "contains"           # 操作符：eq/ne/gt/lt/contains/regex
    value: "sensor"               # 比较值
    action: "include"             # 动作：include/exclude
  
  - field: "value"
    operator: "gt"
    value: 0
    action: "include"
```

## 支持的NATS主题

基于当前系统的NATS主题结构，以下是可以订阅的主要主题：

### 1. 原始数据主题
- `iot.data.>` - 所有原始设备数据
- `iot.data.{device_id}.>` - 特定设备的所有数据
- `iot.data.{device_id}.{key}` - 特定设备的特定数据键
- `iot.data.*.temperature` - 所有设备的温度数据
- `iot.data.*.humidity` - 所有设备的湿度数据

### 2. 告警主题
- `iot.alerts.triggered` - 所有告警
- `iot.alerts.triggered.warning` - 警告级别告警
- `iot.alerts.triggered.critical` - 严重级别告警
- `iot.alerts.triggered.error` - 错误级别告警
- `iot.alerts.triggered.info` - 信息级别告警

### 3. 规则引擎主题
- `iot.rules.>` - 规则引擎所有事件
- `iot.rules.executed` - 规则执行事件
- `iot.rules.matched` - 规则匹配事件

### 4. 系统主题
- `iot.system.>` - 所有系统事件
- `iot.system.startup` - 系统启动事件
- `iot.system.shutdown` - 系统关闭事件
- `iot.system.config_change` - 配置变更事件

### 5. 聚合数据主题
- `aggregated.>` - 所有聚合数据
- `aggregated.{device_id}.>` - 特定设备的聚合数据
- `energy.>` - 能耗分析数据
- `batch.>` - 批处理数据

## 使用场景

### 1. 数据分流和复制
将原始数据同时发送到多个目标系统：
```yaml
subscriptions:
  - subject: "iot.data.>"
    data_type: "raw"
    enabled: true

target_sinks:
  - name: "realtime_dashboard"
    type: "websocket"
    params:
      port: 8080
  
  - name: "historical_storage"
    type: "influxdb"
    params:
      url: "http://localhost:8086"
      bucket: "historical_data"
  
  - name: "backup_storage"
    type: "jetstream"
    params:
      stream_name: "BACKUP_STREAM"
```

### 2. 告警聚合和处理
收集所有告警并进行统一处理：
```yaml
subscriptions:
  - subject: "iot.alerts.triggered"
    data_type: "alert"
    enabled: true

target_sinks:
  - name: "alert_console"
    type: "console"
    params:
      format: "structured"
  
  - name: "alert_notification"
    type: "webhook"
    params:
      url: "https://alerts.example.com/webhook"
```

### 3. 特定设备数据处理
只处理特定设备或数据类型：
```yaml
subscriptions:
  - subject: "iot.data.sensor_*.temperature"
    data_type: "raw"
    enabled: true

filter_rules:
  - field: "value"
    operator: "gt"
    value: 50
    action: "include"

target_sinks:
  - name: "high_temp_alerts"
    type: "console"
    params:
      format: "alert"
```

### 4. 规则引擎输出后处理
处理规则引擎的输出数据：
```yaml
subscriptions:
  - subject: "iot.rules.>"
    data_type: "rule"
    enabled: true
  
  - subject: "aggregated.>"
    data_type: "raw"
    enabled: true

target_sinks:
  - name: "analytics_db"
    type: "influxdb"
    params:
      bucket: "analytics"
```

## 性能调优

### 1. 缓冲区大小调整
```yaml
params:
  buffer_size: 2000      # 增加缓冲区以处理高频数据
  batch_size: 100        # 增加批处理大小以提高吞吐量
  flush_interval: "500ms" # 减少刷新间隔以降低延迟
```

### 2. 队列组负载均衡
```yaml
subscriptions:
  - subject: "iot.data.>"
    data_type: "raw"
    enabled: true
    queue_group: "data_processors"  # 多个实例共享负载
```

### 3. 过滤优化
```yaml
filter_rules:
  # 在早期阶段过滤掉不需要的数据
  - field: "device_id"
    operator: "contains"
    value: "test"
    action: "exclude"
```

## 监控和调试

### 1. 日志监控
NATS订阅器sink会输出详细的日志信息：
- 订阅创建和状态
- 消息处理统计
- 错误和警告信息
- 性能指标

### 2. 健康检查
可以通过以下方式检查sink健康状态：
- 检查NATS连接状态
- 检查目标sink健康状态
- 监控数据处理队列长度

### 3. 性能指标
- 消息处理速率
- 批处理效率
- 错误率统计
- 内存使用情况

## 故障排除

### 1. 常见问题
- **连接失败**: 检查NATS服务器连接
- **订阅失败**: 检查主题权限和格式
- **数据丢失**: 检查缓冲区大小和处理速度
- **目标sink错误**: 检查目标sink配置和健康状态

### 2. 调试技巧
- 启用调试日志级别
- 使用控制台sink查看数据流
- 监控NATS服务器状态
- 检查过滤规则逻辑

## 最佳实践

1. **合理配置批处理大小**：根据数据量和延迟要求调整
2. **使用队列组**：在多实例部署中实现负载均衡
3. **设置合适的过滤规则**：减少不必要的数据处理
4. **监控性能指标**：及时发现性能瓶颈
5. **定期健康检查**：确保系统稳定运行

这个NATS订阅器sink为IoT Gateway提供了强大的数据路由和处理能力，使得系统架构更加灵活和可扩展。