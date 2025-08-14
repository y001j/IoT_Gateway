# Transform转换规则测试指南

## 📋 概述

本文档介绍IoT Gateway数据转换规则的全面测试套件，包括功能测试、性能测试、错误处理测试和集成测试。

## 🧪 测试套件组成

### 1. 功能测试套件 (`cmd/test/transform_tests.go`)

**覆盖范围:**
- ✅ 9种转换类型全覆盖 (scale, offset, expression, unit_convert, lookup, round, clamp, format, map)
- ✅ 3种错误处理策略 (error, ignore, default)
- ✅ NATS消息发布集成测试
- ✅ 边界条件和极值测试
- ✅ 并发安全性测试

**测试用例统计:**
- 基础功能测试: 45+ 测试用例
- 错误处理测试: 15+ 测试用例
- 边界条件测试: 10+ 测试用例
- NATS集成测试: 8+ 测试用例

### 2. 性能测试套件 (`cmd/test/transform_performance_tests.go`)

**测试类型:**
- 📊 基准性能测试 (吞吐量、延迟分析)
- 💪 压力测试 (高并发负载)
- 🧠 内存压力测试 (内存使用分析)
- ⏰ 长时间稳定性测试 (5分钟持续负载)

**性能指标:**
- 吞吐量 (ops/sec)
- 平均延迟、P95、P99延迟 (μs)
- 内存使用量 (MB)
- 成功率 (%)

### 3. 测试数据集 (`test_data/transform_test_scenarios.json`)

**场景覆盖:**
- 🌡️ 温度单位转换综合测试
- 🧮 复杂数学表达式测试
- 🔄 设备状态映射测试
- 📏 长度重量单位转换测试
- 📝 字符串格式化测试
- ⚠️ 错误处理机制测试
- 📡 NATS消息发布测试
- ⚖️ 边界条件和极值测试

## 🚀 快速开始

### 环境要求

- Go 1.19或更高版本
- NATS Server (可选，用于消息发布测试)
- 8GB以上内存 (推荐，用于性能测试)

### 运行基础功能测试

#### Windows环境:
```bash
# 运行完整测试套件
.\run_transform_tests.bat

# 或手动运行
go run cmd/test/transform_tests.go
```

#### Linux/Mac环境:
```bash
# 添加执行权限
chmod +x run_transform_tests.sh

# 运行完整测试套件
./run_transform_tests.sh

# 或手动运行
go run cmd/test/transform_tests.go
```

### 运行性能测试

```bash
# 编译性能测试程序
go build -o bin/transform_performance_tests cmd/test/transform_performance_tests.go

# 运行性能测试
./bin/transform_performance_tests
```

## 📊 测试结果解读

### 功能测试结果示例

```
🔍 测试 scale 转换
   数值缩放转换的各种场景
   --------------------------------------------------
   ✅ 通过: 5/5 (100.0%)
   ⏱️ 耗时: 15.2ms

📊 总体统计
=========
测试总数: 67
通过: 65 (97.0%)
失败: 2 (3.0%)
总耗时: 1.2s

✅ 功能覆盖率:
=============
转换类型覆盖: 9/9 (100%)
  ✅ Scale数值缩放
  ✅ Expression表达式计算
  ✅ Unit Convert单位转换
  ... (其他类型)

错误处理策略覆盖: 3/3 (100%)
  ✅ Error - 抛出错误
  ✅ Ignore - 忽略错误  
  ✅ Default - 使用默认值

🎉 优秀! 成功率: 97.0% - Transform转换规则功能完备且稳定
```

### 性能测试结果示例

```
🏆 性能测试结果汇总:
===================
测试名称                        总操作数      成功率      吞吐量     P95延迟     内存使用
------------------------------  ------------ ------------ ------------ ------------ ------------
Scale缩放转换基准测试             125,432      100.0%     8,341/sec    125.3μs      2.1MB
Expression表达式计算基准测试        98,765       99.9%     6,584/sec    189.7μs      3.4MB
Unit Convert单位转换基准测试        87,543       99.8%     5,836/sec    156.2μs      2.8MB

📈 性能指标分析:
===============
总操作数量: 311,740
总体成功率: 99.90%
平均吞吐量: 6,920 ops/sec
最大吞吐量: 8,341 ops/sec

🏅 优秀! 吞吐量超过5万ops/sec，满足中高性能需求
```

## 🔧 测试配置说明

### 基础转换配置示例

#### Scale数值缩放
```json
{
  "type": "scale",
  "parameters": {
    "factor": 2.5
  },
  "output_key": "scaled_value",
  "precision": 2,
  "error_action": "error"
}
```

#### Expression表达式计算
```json
{
  "type": "expression", 
  "parameters": {
    "expression": "x * 1.8 + 32"
  },
  "output_key": "fahrenheit",
  "precision": 1,
  "publish_subject": "iot.temperature.converted"
}
```

#### Unit Convert单位转换
```json
{
  "type": "unit_convert",
  "parameters": {
    "from": "C",
    "to": "F"
  },
  "output_key": "temperature_f",
  "precision": 1
}
```

### 高级配置选项

| 配置项 | 类型 | 说明 | 示例 |
|--------|------|------|------|
| `type` | string | 转换类型 | "scale", "expression", etc. |
| `parameters` | object | 转换参数 | {"factor": 2.0} |
| `output_key` | string | 输出字段名 | "temperature_f" |
| `output_type` | string | 输出数据类型 | "float", "int", "string", "bool" |
| `precision` | int | 数值精度 | 2 (保留2位小数) |
| `error_action` | string | 错误处理策略 | "error", "ignore", "default" |
| `default_value` | any | 默认值 | 0.0 |
| `publish_subject` | string | NATS发布主题 | "iot.data.converted" |

## 📝 自定义测试用例

### 添加新的转换类型测试

1. **扩展测试配置**: 在`transform_tests.go`中添加新的生成函数
```go
func generateNewTransformTests() TransformTestConfig {
    return TransformTestConfig{
        Name:          "新转换类型测试",
        Description:   "测试新转换类型的功能",
        TransformType: "new_transform",
        Config: map[string]interface{}{
            "type": "new_transform",
            "parameters": map[string]interface{}{
                "param1": "value1",
            },
        },
        TestData: []TransformTestData{
            // 测试数据
        },
        Expected: []TransformExpected{
            // 期望结果
        },
    }
}
```

2. **添加到测试列表**: 在`main`函数中添加新测试
```go
testConfigs := []TransformTestConfig{
    // ... 现有测试
    generateNewTransformTests(),
}
```

### 添加自定义测试场景

在`test_data/transform_test_scenarios.json`中添加新场景:

```json
{
  "name": "自定义测试场景",
  "description": "描述测试目的和场景",
  "transform_type": "custom",
  "test_cases": [
    {
      "name": "具体测试用例",
      "config": {
        // 转换配置
      },
      "test_data": [
        {
          "input": "输入值",
          "expected": "期望输出",
          "description": "测试描述"
        }
      ]
    }
  ]
}
```

## 🐛 问题排查

### 常见测试失败原因

1. **类型转换错误**
   - 原因: 输入数据类型不匹配
   - 解决: 检查测试数据的类型定义

2. **精度问题**
   - 原因: 浮点数精度比较失败
   - 解决: 调整精度设置或使用近似比较

3. **NATS连接失败**
   - 原因: NATS服务器未启动
   - 解决: 启动NATS服务器或在无NATS模式下运行

4. **内存不足**
   - 原因: 性能测试需要大量内存
   - 解决: 调整测试规模或增加系统内存

### 调试技巧

1. **开启详细日志**
```bash
# 设置日志级别为DEBUG
export LOG_LEVEL=debug
go run cmd/test/transform_tests.go
```

2. **单独运行特定测试**
```go
// 在代码中注释掉其他测试，只运行需要调试的测试
testConfigs := []TransformTestConfig{
    generateScaleTransformTests(), // 只运行这一个
}
```

3. **检查中间结果**
```go
// 在测试代码中添加调试输出
fmt.Printf("Debug: 输入=%v, 输出=%v, 期望=%v\n", 
    testData.Value, transformedPoint.Value, expected.ExpectedValue)
```

## 📈 性能优化建议

基于测试结果的优化建议:

1. **低于1万ops/sec**: 
   - 检查算法效率
   - 优化数据结构
   - 减少内存分配

2. **低于5万ops/sec**:
   - 添加表达式缓存
   - 异步NATS发布
   - 使用对象池

3. **内存使用过高**:
   - 检查内存泄漏
   - 优化数据结构大小
   - 调整垃圾回收参数

4. **延迟过高**:
   - 优化热路径代码
   - 减少同步操作
   - 使用批处理

## 🔄 持续集成

### CI/CD集成示例

```yaml
# .github/workflows/transform-tests.yml
name: Transform Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Setup Go
      uses: actions/setup-go@v3
      with:
        go-version: '1.19'
    
    - name: Install NATS Server
      run: |
        wget https://github.com/nats-io/nats-server/releases/download/v2.9.0/nats-server-v2.9.0-linux-amd64.tar.gz
        tar -xzf nats-server-v2.9.0-linux-amd64.tar.gz
        sudo mv nats-server-v2.9.0-linux-amd64/nats-server /usr/local/bin/
        nats-server --version
    
    - name: Start NATS Server
      run: nats-server &
    
    - name: Run Transform Tests
      run: |
        chmod +x run_transform_tests.sh
        ./run_transform_tests.sh
    
    - name: Upload Test Results
      uses: actions/upload-artifact@v3
      with:
        name: test-results
        path: test_results/
```

## 📚 进阶用法

### 批量性能基准测试

```bash
# 生成多个版本的性能对比报告
for version in v1.0 v1.1 v1.2; do
    git checkout $version
    go run cmd/test/transform_performance_tests.go > perf_$version.txt
done

# 分析性能回归
diff perf_v1.0.txt perf_v1.2.txt
```

### 自动化性能监控

```bash
#!/bin/bash
# 每日性能监控脚本

THRESHOLD=50000  # 吞吐量阈值

RESULT=$(go run cmd/test/transform_performance_tests.go | grep "最大吞吐量" | awk '{print $2}')
THROUGHPUT=${RESULT%/sec*}

if [ $THROUGHPUT -lt $THRESHOLD ]; then
    echo "⚠️ 性能警告: 吞吐量($THROUGHPUT) 低于阈值($THRESHOLD)"
    # 发送告警通知
else
    echo "✅ 性能正常: 吞吐量($THROUGHPUT) 满足要求"
fi
```

## 🎯 测试最佳实践

1. **测试驱动开发**: 先写测试用例，再实现功能
2. **边界值测试**: 重点测试边界条件和极值情况
3. **错误路径测试**: 确保错误处理逻辑正确
4. **性能回归测试**: 每次发版前运行性能测试
5. **持续监控**: 在生产环境中监控转换性能指标

## 📞 支持与反馈

如果在使用测试套件过程中遇到问题，请:

1. 检查本文档的问题排查章节
2. 查看测试日志和错误信息
3. 提交Issue并附带详细的错误信息和环境描述

---

**测试套件版本**: v1.0
**更新时间**: 2025-01-08
**维护者**: IoT Gateway Development Team