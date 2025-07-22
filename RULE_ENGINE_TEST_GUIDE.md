# IoT Gateway 规则引擎测试指南

## 📋 概述

本指南介绍如何使用专门的测试配置来验证和演示 IoT Gateway 规则引擎的功能。

## 🗂️ 测试文件结构

```
IoT Gateway/
├── config_rule_engine_test.yaml           # 主测试配置文件
├── rules/test_comprehensive_rules.json    # 综合测试规则
├── run_rule_engine_demo.sh               # 演示启动脚本
├── run_rule_engine_tests.sh              # 测试执行脚本
└── cmd/test/                              # 测试代码
    ├── simple_rule_tests.go              # 简化功能测试
    ├── rule_engine_basic_tests.go        # 基础概念测试
    └── integration_concept_tests.go      # 集成概念测试
```

## 🚀 快速开始

### 1. 运行完整演示

```bash
# 启动交互式演示
./run_rule_engine_demo.sh
```

演示选项：
- **选项 1**: 启动网关并运行2分钟测试（推荐）
- **选项 2**: 仅启动网关服务（手动控制）
- **选项 3**: 运行规则引擎测试套件
- **选项 4**: 显示配置信息详情

### 2. 运行测试套件

```bash
# 运行所有测试
./run_rule_engine_tests.sh

# 或单独运行特定测试
go run cmd/test/simple_rule_tests.go           # 简化测试
go run cmd/test/rule_engine_basic_tests.go     # 基础测试
go run cmd/test/integration_concept_tests.go   # 集成测试
```

### 3. 手动启动网关

```bash
# 编译网关
go build -o bin/gateway cmd/gateway/main.go

# 使用测试配置启动
./bin/gateway -config config_rule_engine_test.yaml
```

## 📊 测试配置说明

### 核心配置特性

```yaml
# 规则引擎优化配置
rule_engine:
  worker_pool:
    size: 8                    # 8个并行工作器
    batch_size: 50            # 批处理50个消息
  
  expression_engine:
    enable_cache: true         # 表达式缓存
    cache_size: 10000         # 缓存10k表达式
  
  aggregation:
    enable_incremental: true   # O(1)增量统计
    default_ttl: "10m"        # 10分钟状态TTL
  
  monitoring:
    enabled: true             # 全链路监控
    error_threshold: 0.05     # 5%错误率阈值
```

### 数据源配置

#### 温度传感器组（10个设备）
- **频率**: 每秒1次数据
- **数据**: 温度 15-45°C，湿度 30-90%
- **异常**: 2%异常数据率

#### 压力传感器组（5个设备）
- **频率**: 每2秒1次数据
- **数据**: 压力 900-1100hPa，海拔 0-2000m
- **异常**: 1%异常数据率

#### 高频振动传感器组（20个设备）
- **频率**: 每100ms1次数据（10Hz）
- **数据**: 振动 0-10g，转速 1000-5000rpm
- **用途**: 性能压力测试

## 🎯 测试规则详解

### 1. 阈值报警规则

```json
{
  "id": "temperature_high_alert",
  "conditions": {
    "expression": "key == 'temperature' && value > 40"
  },
  "actions": [{
    "type": "alert",
    "config": {
      "level": "critical",
      "message": "设备 {{.device_id}} 温度过高: {{.value}}°C"
    }
  }]
}
```

**测试目标**: 验证基础条件匹配和报警功能

### 2. 数据聚合规则

```json
{
  "id": "temperature_aggregation", 
  "actions": [{
    "type": "aggregate",
    "config": {
      "window_size": 10,
      "functions": ["avg", "min", "max", "stddev"],
      "group_by": ["device_id"]
    }
  }]
}
```

**测试目标**: 验证增量统计和滑动窗口功能

### 3. 复杂表达式规则

```json
{
  "conditions": {
    "expression": "value > avg(last_values, 5) + 2*stddev(last_values, 5) && time_range(9, 17)"
  }
}
```

**测试目标**: 验证表达式引擎高级功能

### 4. 性能基准规则

```json
{
  "id": "performance_benchmark",
  "conditions": {
    "expression": "contains(device_id, 'vibration_sensor_')"
  }
}
```

**测试目标**: 高频数据处理性能验证

## 📈 监控和调试

### Web界面
- **主界面**: http://localhost:8081
- **API**: http://localhost:8081/api/rules
- **健康检查**: http://localhost:8080/health

### WebSocket实时监控
```javascript
// 连接WebSocket监控
const ws = new WebSocket('ws://localhost:8090/ws/rules');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Rule triggered:', data);
};
```

### Prometheus指标
- **指标端点**: http://localhost:9090/metrics
- **性能分析**: http://localhost:6060/debug/pprof/

### 日志文件
```bash
# 查看网关主日志
tail -f logs/gateway_rule_test.log

# 查看规则处理数据
tail -f logs/rule_engine_test_data.log

# 实时监控规则触发
grep "rule_triggered" logs/rule_engine_test_data.log
```

## 🧪 验证测试结果

### 1. 基础功能验证
```bash
# 检查规则加载
curl http://localhost:8081/api/rules | jq '.[] | .id'

# 检查数据流
curl http://localhost:8081/api/metrics | grep rule_execution
```

### 2. 性能指标验证
```bash
# 吞吐量检查
curl http://localhost:9090/metrics | grep iot_gateway_rule_engine_throughput

# 延迟检查  
curl http://localhost:9090/metrics | grep iot_gateway_rule_engine_latency
```

### 3. 错误率检查
```bash
# 错误统计
curl http://localhost:9090/metrics | grep iot_gateway_rule_engine_errors
```

## 🎛️ 自定义测试场景

### 修改数据频率
```yaml
# 在config_rule_engine_test.yaml中调整
southbound:
  adapters:
    - name: "mock_temperature_sensors"
      config:
        data_interval: "500ms"  # 改为500ms间隔
```

### 添加新规则
```json
// 在rules/test_comprehensive_rules.json中添加
{
  "id": "custom_test_rule",
  "conditions": {
    "expression": "your_condition_here"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "message": "自定义测试规则触发"
      }
    }
  ]
}
```

### 调整性能参数
```yaml
rule_engine:
  worker_pool:
    size: 16              # 增加工作器数量
    batch_size: 100       # 增加批处理大小
  
  expression_engine:
    cache_size: 50000     # 增加缓存大小
```

## 📊 预期测试结果

### 性能目标
- **吞吐量**: > 1,000 消息/秒
- **延迟**: P99 < 100ms
- **错误率**: < 1%
- **内存使用**: < 512MB

### 功能覆盖
- ✅ 表达式解析和执行
- ✅ 多种动作类型 (alert, aggregate, transform, filter)
- ✅ 数据聚合和统计
- ✅ 实时监控和指标
- ✅ 错误处理和恢复
- ✅ 高并发处理

## 🔧 故障排除

### 常见问题

1. **网关启动失败**
   ```bash
   # 检查端口占用
   netstat -tulpn | grep :8080
   
   # 检查配置文件语法
   go run -c config_rule_engine_test.yaml
   ```

2. **规则不触发**
   ```bash
   # 检查规则语法
   python3 -m json.tool rules/test_comprehensive_rules.json
   
   # 查看规则加载日志
   grep "rule.*loaded" logs/gateway_rule_test.log
   ```

3. **性能不达标**
   ```bash
   # 查看性能指标
   curl http://localhost:9090/metrics | grep rule_engine
   
   # 检查资源使用
   top -p $(pgrep gateway)
   ```

### 调试技巧

1. **启用详细日志**
   ```yaml
   logging:
     level: "debug"
   ```

2. **启用性能分析**
   ```yaml
   performance:
     enable_memory_profiling: true
     enable_goroutine_monitoring: true
   ```

3. **查看实时指标**
   ```bash
   # 使用watch命令监控
   watch -n 1 'curl -s http://localhost:9090/metrics | grep rule_engine'
   ```

## 📚 进阶使用

### 集成外部服务
```yaml
northbound:
  sinks:
    - name: "external_mqtt"
      type: "mqtt"
      enabled: true
      config:
        broker: "tcp://your-mqtt-broker:1883"
```

### 自定义表达式函数
```go
// 在表达式引擎中添加自定义函数
engine.RegisterFunction("custom_func", func(args ...interface{}) interface{} {
    // 自定义逻辑
    return result
})
```

### 扩展监控
```yaml
extensions:
  custom_metrics:
    enabled: true
    collectors: ["rule_performance", "data_quality"]
```

---

## 🎉 总结

这套测试配置提供了全面的 IoT Gateway 规则引擎功能验证，包括：

- **性能测试**: 高频数据流处理
- **功能测试**: 各种规则类型和动作
- **稳定性测试**: 错误处理和恢复
- **监控测试**: 实时指标和健康检查

通过这些测试，您可以验证规则引擎在生产环境中的表现，并根据实际需求调整配置参数。