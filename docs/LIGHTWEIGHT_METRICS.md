# IoT Gateway 轻量级指标系统

## 概述

IoT Gateway 轻量级指标系统是一个高效、简洁的监控解决方案，专为IoT场景设计，提供实时的系统运行状态、性能指标和健康监控。相比传统的Prometheus方案，该系统更加轻量级，无需外部依赖，支持多种输出格式，易于集成和使用。

## 系统架构

### 核心组件

1. **轻量级指标收集器** (`internal/metrics/lightweight_metrics.go`)
   - 中心化的指标收集和管理
   - 支持多种指标类型
   - 实时数据更新
   - 多格式输出支持

2. **运行时集成** (`internal/core/runtime.go`)
   - 自动初始化指标收集器
   - 定期更新系统指标
   - 提供HTTP端点访问

3. **Web API集成** (`internal/web/api/system_handler.go`)
   - RESTful API接口
   - 认证和权限控制
   - 兼容现有系统

## 指标类型

### 1. 系统指标 (SystemMetrics)

监控系统基础资源使用情况：

```json
{
  "uptime_seconds": 3600.5,
  "memory_usage_bytes": 67108864,
  "cpu_usage_percent": 25.5,
  "goroutine_count": 42,
  "gc_pause_ms": 0.15,
  "heap_size_bytes": 134217728,
  "heap_in_use_bytes": 67108864,
  "version": "1.0.0",
  "go_version": "go1.21.0"
}
```

### 2. 网关指标 (GatewayMetrics)

监控IoT网关核心状态：

```json
{
  "status": "running",
  "start_time": "2024-01-15T10:30:00Z",
  "config_file": "./config.yaml",
  "plugins_directory": "./plugins",
  "total_adapters": 5,
  "running_adapters": 4,
  "total_sinks": 3,
  "running_sinks": 3,
  "nats_connected": true,
  "nats_connection_url": "nats://localhost:4222",
  "web_ui_port": 8081,
  "api_port": 8080
}
```

### 3. 数据处理指标 (DataMetrics)

监控数据流处理性能：

```json
{
  "total_data_points": 125000,
  "data_points_per_second": 100.5,
  "total_bytes_processed": 1048576,
  "bytes_per_second": 1024.0,
  "average_latency_ms": 15.5,
  "max_latency_ms": 250.0,
  "min_latency_ms": 2.5,
  "last_data_point_time": "2024-01-15T10:35:00Z",
  "data_queue_length": 25,
  "processing_errors_count": 5
}
```

### 4. 连接指标 (ConnectionMetrics)

监控网络连接状态：

```json
{
  "active_connections": 15,
  "total_connections": 1250,
  "failed_connections": 25,
  "connections_by_type": {
    "modbus": 5,
    "mqtt": 8,
    "http": 2
  },
  "connections_by_status": {
    "running": 12,
    "stopped": 2,
    "error": 1
  },
  "average_response_time_ms": 45.2,
  "connection_errors": 10,
  "reconnection_count": 3
}
```

### 5. 规则引擎指标 (RuleMetrics)

监控规则引擎执行状态：

```json
{
  "total_rules": 25,
  "enabled_rules": 20,
  "rules_matched": 1500,
  "actions_executed": 800,
  "actions_succeeded": 750,
  "actions_failed": 50,
  "average_execution_time_ms": 12.5,
  "rule_engine_status": "healthy",
  "last_rule_execution": "2024-01-15T10:34:55Z"
}
```

### 6. 性能指标 (PerformanceMetrics)

监控系统性能分布：

```json
{
  "throughput_per_second": 100.0,
  "p50_latency_ms": 15.0,
  "p95_latency_ms": 45.0,
  "p99_latency_ms": 120.0,
  "queue_length": 25,
  "processing_time": {
    "adapter_processing": 8.5,
    "rule_evaluation": 12.0,
    "sink_publishing": 5.2
  },
  "resource_utilization": {
    "cpu": 25.5,
    "memory": 45.8,
    "disk": 35.2
  }
}
```

### 7. 错误指标 (ErrorMetrics)

监控错误和异常情况：

```json
{
  "total_errors": 125,
  "errors_per_second": 0.5,
  "errors_by_type": {
    "connection": 45,
    "validation": 25,
    "timeout": 15,
    "system": 10
  },
  "errors_by_level": {
    "warning": 80,
    "error": 35,
    "critical": 10
  },
  "last_error": "Connection timeout to device 192.168.1.100",
  "last_error_time": "2024-01-15T10:34:30Z",
  "error_rate": 0.001,
  "recovery_count": 15
}
```

## 访问方式

### 1. Gateway主服务端点

通过网关主服务访问指标（默认端口8080）：

```bash
# JSON格式（默认）
curl http://localhost:8080/metrics
curl http://localhost:8080/metrics?format=json

# 纯文本格式
curl http://localhost:8080/metrics?format=text
```

### 2. Web API端点

通过Web API访问指标（需要认证）：

```bash
# 获取认证token
TOKEN=$(curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"password"}' \
  | jq -r '.data.token')

# 访问简化指标（通过SystemService）
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8081/api/v1/system/metrics

# 注意：轻量级指标的完整版本目前通过Gateway主服务提供
# 可以通过以下方式获取：
curl http://localhost:8080/metrics
curl http://localhost:8080/metrics?format=text
```

### 3. 健康检查端点

```bash
# 系统健康状态
curl http://localhost:8080/health

# 详细健康检查
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8081/api/v1/system/health
```

## 输出格式

### JSON格式

标准的JSON格式，便于程序处理：

```json
{
  "system": {
    "uptime_seconds": 3600.5,
    "memory_usage_bytes": 67108864,
    "cpu_usage_percent": 25.5
  },
  "gateway": {
    "status": "running",
    "total_adapters": 5,
    "running_adapters": 4
  },
  "last_updated": "2024-01-15T10:35:00Z"
}
```

### 纯文本格式

类似Prometheus的文本格式，便于监控系统集成：

```text
# IoT Gateway Metrics
# Generated at: 2024-01-15T10:35:00Z

# System Metrics
iot_gateway_uptime_seconds 3600.50
iot_gateway_memory_usage_bytes 67108864
iot_gateway_cpu_usage_percent 25.50

# Gateway Metrics
iot_gateway_status{status="running"} 1
iot_gateway_total_adapters 5
iot_gateway_running_adapters 4

# Data Processing Metrics
iot_gateway_total_data_points 125000
iot_gateway_data_points_per_second 100.50
```

## 配置

### 初始化配置

系统启动时自动初始化，可通过配置文件设置相关参数：

```yaml
gateway:
  id: "iot-gateway"
  http_port: 8080
  log_level: "info"
  metrics:
    enabled: true
    update_interval: "10s"
    retention_period: "24h"
```

### 运行时配置

可通过API动态更新部分配置：

```bash
# 更新指标收集间隔
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"metrics":{"update_interval":"5s"}}' \
  http://localhost:8081/api/v1/system/config
```

## 集成示例

### 1. 监控脚本

```bash
#!/bin/bash
# metrics_monitor.sh

GATEWAY_URL="http://localhost:8080"
WEB_API_URL="http://localhost:8081"

# 获取基础指标
echo "=== System Metrics ==="
curl -s "$GATEWAY_URL/metrics?format=text" | grep -E "(uptime|memory|cpu)"

# 获取网关状态
echo "=== Gateway Status ==="
curl -s "$GATEWAY_URL/metrics" | jq '.gateway | {status, running_adapters, running_sinks}'

# 检查错误率
echo "=== Error Rate ==="
curl -s "$GATEWAY_URL/metrics" | jq '.errors.error_rate'
```

### 2. Python集成

```python
import requests
import json
from datetime import datetime

class IoTGatewayMetrics:
    def __init__(self, gateway_url="http://localhost:8080"):
        self.gateway_url = gateway_url
    
    def get_metrics(self, format="json"):
        """获取指标数据"""
        response = requests.get(f"{self.gateway_url}/metrics", 
                              params={"format": format})
        if format == "json":
            return response.json()
        return response.text
    
    def get_system_health(self):
        """获取系统健康状态"""
        metrics = self.get_metrics()
        return {
            "status": metrics["gateway"]["status"],
            "uptime": metrics["system"]["uptime_seconds"],
            "error_rate": metrics["errors"]["error_rate"],
            "active_connections": metrics["connections"]["active_connections"]
        }
    
    def check_alerts(self):
        """检查告警条件"""
        metrics = self.get_metrics()
        alerts = []
        
        # 检查错误率
        if metrics["errors"]["error_rate"] > 0.05:
            alerts.append("High error rate detected")
        
        # 检查内存使用
        if metrics["system"]["memory_usage_bytes"] > 1024*1024*1024:
            alerts.append("High memory usage")
        
        return alerts

# 使用示例
monitor = IoTGatewayMetrics()
health = monitor.get_system_health()
alerts = monitor.check_alerts()

print(f"System Health: {health}")
if alerts:
    print(f"Alerts: {alerts}")
```

### 3. 前端集成

```javascript
// metrics.js
class MetricsClient {
    constructor(baseUrl = 'http://localhost:8081', token = null) {
        this.baseUrl = baseUrl;
        this.token = token;
    }
    
    async getMetrics() {
        const response = await fetch(`${this.baseUrl}/api/v1/system/metrics/lightweight`, {
            headers: {
                'Authorization': `Bearer ${this.token}`,
                'Content-Type': 'application/json'
            }
        });
        return response.json();
    }
    
    async getSystemStatus() {
        const metrics = await this.getMetrics();
        return {
            status: metrics.data.gateway.status,
            uptime: metrics.data.system.uptime_seconds,
            adapters: {
                total: metrics.data.gateway.total_adapters,
                running: metrics.data.gateway.running_adapters
            },
            performance: {
                dataPointsPerSecond: metrics.data.data.data_points_per_second,
                errorRate: metrics.data.errors.error_rate
            }
        };
    }
}

// 使用示例
const client = new MetricsClient('http://localhost:8081', 'your-jwt-token');
client.getSystemStatus().then(status => {
    console.log('System Status:', status);
    // 更新UI显示
});
```

## 性能优化

### 1. 指标收集优化

- 使用读写锁减少并发冲突
- 采用原子操作更新计数器
- 定期清理过期数据

### 2. 内存管理

- 限制历史数据保留数量
- 使用环形缓冲区存储时序数据
- 定期执行垃圾回收

### 3. 网络优化

- 支持HTTP压缩
- 缓存静态指标数据
- 使用连接池管理

## 故障排除

### 常见问题

1. **指标数据为空**
   - 检查指标收集器是否正确初始化
   - 确认系统启动时间足够长

2. **数据更新不及时**
   - 检查更新间隔设置
   - 确认没有阻塞的操作

3. **内存使用过高**
   - 调整历史数据保留策略
   - 检查是否有内存泄漏

### 调试命令

```bash
# 检查指标端点是否可用
curl -v http://localhost:8080/metrics

# 查看系统日志
tail -f /var/log/iot-gateway.log

# 检查进程状态
ps aux | grep iot-gateway

# 检查端口占用
netstat -tlnp | grep :8080
```

## 最佳实践

1. **监控策略**
   - 设置合理的告警阈值
   - 定期检查关键指标
   - 建立指标基线

2. **数据管理**
   - 定期备份重要指标
   - 实施数据保留策略
   - 监控存储空间使用

3. **性能优化**
   - 根据实际需求调整收集频率
   - 使用合适的输出格式
   - 实施缓存策略

4. **安全考虑**
   - 启用API认证
   - 限制访问权限
   - 监控异常访问

## 扩展开发

### 添加自定义指标

```go
// 扩展指标结构
type CustomMetrics struct {
    DeviceCount     int     `json:"device_count"`
    MessageQueue    int     `json:"message_queue"`
    ProcessingRate  float64 `json:"processing_rate"`
}

// 更新轻量级指标
func (m *LightweightMetrics) UpdateCustomMetrics(deviceCount, queueLength int, rate float64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 更新自定义指标
    m.CustomMetrics.DeviceCount = deviceCount
    m.CustomMetrics.MessageQueue = queueLength
    m.CustomMetrics.ProcessingRate = rate
    
    m.LastUpdated = time.Now()
}
```

### 集成外部监控

```go
// 支持外部监控系统
type MetricsExporter interface {
    Export(metrics *LightweightMetrics) error
}

type PrometheusExporter struct {
    endpoint string
}

func (e *PrometheusExporter) Export(metrics *LightweightMetrics) error {
    // 转换为Prometheus格式并发送
    return nil
}
```

## 总结

IoT Gateway轻量级指标系统提供了一个完整、高效的监控解决方案，具有以下特点：

- **轻量级**: 无外部依赖，资源消耗小
- **实时性**: 实时数据更新和查询
- **易集成**: 标准HTTP API，多格式支持
- **可扩展**: 支持自定义指标和外部集成
- **生产就绪**: 包含认证、权限、错误处理等

该系统为IoT Gateway提供了全面的可观测性，帮助用户更好地监控和管理系统运行状态。