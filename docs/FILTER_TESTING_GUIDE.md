# 过滤器测试指南

本文档说明如何全面测试IoT Gateway的增强过滤器功能。

## 📋 测试概述

### 新增过滤器类型

| 过滤器类型 | 功能描述 | 应用场景 |
|-----------|----------|----------|
| **质量过滤器** | 基于设备质量码过滤异常数据 | 工业设备状态监控 |
| **变化率过滤器** | 检测传感器数据变化率异常 | 传感器故障检测 |
| **统计异常过滤器** | 基于统计学的自适应异常检测 | 智能异常检测 |
| **连续异常过滤器** | 检测连续异常，减少误报 | 设备故障确认 |
| **范围过滤器** | 数值范围过滤 | 数据质量控制 |
| **重复过滤器** | 重复数据检测 | 数据去重 |

### 测试层级

```
📊 测试金字塔
├── 🔬 单元测试       - 过滤器逻辑验证
├── 🔗 集成测试       - 规则引擎集成
├── ⚡ 性能测试       - 吞吐量和延迟
├── 🔄 并发测试       - 线程安全
└── 🔴 端到端测试     - 实际数据流
```

## 🚀 快速开始

### 1. 快速验证
```bash
# 运行快速验证脚本
./quick_filter_test.sh
```

### 2. 完整测试
```bash
# 运行完整测试套件
./run_filter_tests.sh
```

### 3. 生成测试数据
```bash
# 生成测试数据
go run cmd/test/filter_data_generator.go
```

## 📁 测试文件结构

```
测试相关文件:
├── cmd/test/
│   ├── filter_tests.go              # 主要测试程序
│   └── filter_data_generator.go     # 测试数据生成器
├── configs/test/
│   ├── filter_test_rules.json       # 测试规则配置
│   └── filter_test_config.yaml      # 测试网关配置
├── test_data/
│   └── filter_test_scenarios.json   # 生成的测试场景
├── run_filter_tests.sh              # 完整测试脚本
├── quick_filter_test.sh             # 快速验证脚本
└── docs/
    ├── enhanced_filters.md          # 过滤器详细文档
    └── FILTER_TESTING_GUIDE.md     # 本文档
```

## 🧪 测试详情

### 单元测试

#### 1. 质量过滤器测试
```json
{
  "test_scenario": "quality_filter",
  "description": "测试基于设备质量码的数据过滤",
  "config": {
    "type": "quality",
    "parameters": {
      "allowed_quality": [0]
    },
    "action": "drop"
  },
  "test_cases": [
    {"quality": 0, "expected": "pass"},
    {"quality": 1, "expected": "drop"},
    {"quality": 2, "expected": "drop"}
  ]
}
```

#### 2. 变化率过滤器测试
```json
{
  "test_scenario": "change_rate_filter",
  "description": "测试传感器数据变化率异常检测",
  "config": {
    "type": "change_rate", 
    "parameters": {
      "max_change_rate": 10.0,
      "time_window": "30s"
    }
  },
  "test_cases": [
    {"change_rate": 5.0, "expected": "pass"},
    {"change_rate": 15.0, "expected": "drop"}
  ]
}
```

#### 3. 统计异常过滤器测试
```json
{
  "test_scenario": "statistical_anomaly_filter",
  "description": "测试基于统计学的异常检测",
  "config": {
    "type": "statistical_anomaly",
    "parameters": {
      "window_size": 50,
      "threshold": 2.0,
      "min_data_points": 10
    }
  },
  "test_data": "正态分布数据 + 异常值"
}
```

#### 4. 连续异常过滤器测试
```json
{
  "test_scenario": "consecutive_anomaly_filter",
  "description": "测试连续异常检测",
  "config": {
    "type": "consecutive_anomaly",
    "parameters": {
      "consecutive_count": 3,
      "threshold": 30.0,
      "comparison": "gt"
    }
  },
  "logic": "只有连续3次异常才触发过滤"
}
```

### 性能测试

#### 基准测试指标
- **吞吐量**: 目标 > 1000 ops/sec
- **延迟**: P99 < 10ms
- **内存使用**: 稳定，无泄漏
- **CPU使用**: < 30% (单核)

#### 测试数据量
- 小规模: 1,000 条记录
- 中规模: 10,000 条记录
- 大规模: 100,000 条记录

#### 并发测试
- 并发度: 10, 50, 100 goroutines
- 数据竞争检测
- 锁竞争分析

### 集成测试

#### 1. 规则引擎集成
```yaml
# 测试配置
rule_engine:
  enabled: true
  concurrent_workers: 4
  buffer_size: 1000
  enable_monitoring: true
```

#### 2. 数据流测试
```
数据流: Mock Adapter → Rule Engine → Filter → Console Sink
验证点: 
  - 数据正确过滤
  - 统计信息准确
  - 性能指标正常
```

## 📊 测试报告

### 自动生成报告
测试完成后会生成以下报告：
- `logs/filter_tests/test_report_YYYYMMDD_HHMMSS.md`
- `logs/filter_tests/filter_test_YYYYMMDD_HHMMSS.log`

### 报告内容
```markdown
# 过滤器测试报告

## 测试概述
- 测试时间: 2025-08-08 12:00:00
- 测试环境: Linux/Windows
- Go版本: go1.21.0

## 测试结果
- 编译测试: ✅ 通过
- 单元测试: ✅ 45/45 通过
- 性能测试: ✅ 满足基准
- 并发测试: ✅ 无竞态条件
- 内存测试: ✅ 无泄漏

## 性能数据
| 测试项 | 结果 | 基准 | 状态 |
|--------|------|------|------|
| 吞吐量 | 2500 ops/sec | >1000 | ✅ |
| P99延迟 | 5ms | <10ms | ✅ |
| 内存使用 | 稳定 | 无泄漏 | ✅ |
```

## 🔧 故障排除

### 常见问题

#### 1. 编译错误
```bash
# 检查Go模块
go mod tidy
go mod verify

# 检查导入路径
grep -r "github.com/your-org" .
```

#### 2. 测试数据问题
```bash
# 重新生成测试数据
go run cmd/test/filter_data_generator.go

# 验证JSON格式
python3 -m json.tool test_data/filter_test_scenarios.json
```

#### 3. 性能问题
```bash
# 运行性能分析
go run -race cmd/test/filter_tests.go
go tool pprof cmd/test/filter_tests.go cpu.prof
```

#### 4. 配置问题
```bash
# 验证YAML配置
python3 -c "import yaml; yaml.safe_load(open('configs/test/filter_test_config.yaml'))"

# 验证JSON规则
python3 -m json.tool configs/test/filter_test_rules.json
```

### 调试技巧

#### 1. 启用调试日志
```yaml
gateway:
  log_level: "debug"
```

#### 2. 监控过滤器状态
```bash
# 通过WebSocket监控
curl ws://localhost:8082/filter-test-ws

# 通过REST API查询
curl http://localhost:8081/api/rules/status
```

#### 3. 查看详细统计
```bash
# 过滤器统计
curl http://localhost:8081/api/metrics/filters

# 规则执行统计
curl http://localhost:8081/api/rules/metrics
```

## 📈 性能调优

### 参数调优建议

#### 1. 质量过滤器
```json
{
  "parameters": {
    "allowed_quality": [0],  // 根据协议调整
    "cache_ttl": "300s"      // 缓存时间
  }
}
```

#### 2. 变化率过滤器
```json
{
  "parameters": {
    "max_change_rate": 15.0,  // 根据传感器特性
    "time_window": "30s",     // 时间窗口
    "cleanup_interval": "5m"  // 清理间隔
  }
}
```

#### 3. 统计异常过滤器
```json
{
  "parameters": {
    "window_size": 100,       // 窗口大小
    "threshold": 2.5,         // 标准差倍数
    "min_data_points": 20,    // 最小样本数
    "adaptive_threshold": true // 自适应阈值
  }
}
```

### 系统级优化

#### 1. 并发配置
```yaml
rule_engine:
  concurrent_workers: 8     # CPU核心数
  buffer_size: 2000        # 缓冲区大小
  batch_size: 100          # 批处理大小
```

#### 2. 内存优化
```yaml
gateway:
  gc_percent: 100          # GC触发百分比
  max_procs: 0            # 使用所有CPU
```

#### 3. 网络优化
```yaml
nats:
  max_payload: 1048576    # 1MB
  write_deadline: "10s"   # 写入超时
  ping_interval: "2m"     # 心跳间隔
```

## 🔍 监控和告警

### 关键指标

#### 1. 过滤器指标
- 处理速率 (ops/sec)
- 过滤比例 (%)
- 平均延迟 (ms)
- 错误率 (%)

#### 2. 系统指标
- CPU使用率 (%)
- 内存使用量 (MB)
- 网络I/O (bytes/sec)
- 磁盘I/O (IOPS)

### 告警配置
```yaml
alerts:
  - name: "过滤器处理延迟"
    condition: "latency_p99 > 50ms"
    action: "webhook"
    
  - name: "过滤器错误率"
    condition: "error_rate > 1%"
    action: "email"
```

## 📚 扩展阅读

- [过滤器详细文档](enhanced_filters.md)
- [规则引擎架构](rule_engine.md)
- [性能调优指南](performance_tuning.md)
- [监控和告警](monitoring.md)

---

📝 **注意**: 本文档会随着过滤器功能的演进而更新，建议定期查看最新版本。