# IoT Gateway 规则引擎测试文档

## 📋 概述

本文档介绍如何使用提供的配置文件和脚本来测试 IoT Gateway 的规则引擎功能。

## 🗂️ 文件说明

### 配置文件
- **`config_rule_engine_test.yaml`** - 主测试配置文件，包含网关、规则引擎、数据源和输出配置
- **`test_rules_simple.json`** - 简化的测试规则集合，包含基本的规则类型

### 测试脚本
- **`quick_test_rules.sh`** - 快速测试脚本（10秒测试）
- **`test_gateway_rules.sh`** - 完整的测试脚本（可配置时长）
- **`validate_rule_engine.go`** - 配置验证工具

### 规则引擎功能测试
- **`cmd/test/simple_rule_tests.go`** - 简化功能测试
- **`cmd/test/rule_engine_basic_tests.go`** - 基础概念测试  
- **`cmd/test/integration_concept_tests.go`** - 集成概念测试

## 🚀 快速开始

### 1. 快速验证（推荐首次运行）

```bash
# 运行10秒快速测试
./quick_test_rules.sh
```

这个脚本会：
- ✅ 检查环境和依赖
- ✅ 验证配置文件格式
- ✅ 编译网关程序
- ✅ 运行10秒功能测试
- ✅ 分析测试结果

### 2. 完整功能测试

```bash
# 运行完整的规则引擎测试
./test_gateway_rules.sh
```

选择测试模式：
1. **自动测试模式**：启动网关，运行1分钟，自动停止
2. **手动控制模式**：启动网关，手动停止（Ctrl+C）
3. **配置检查模式**：仅验证配置和规则，不启动服务

### 3. 单独运行规则引擎算法测试

```bash
# 运行核心算法测试
go run cmd/test/simple_rule_tests.go

# 运行基础概念测试
go run cmd/test/rule_engine_basic_tests.go

# 运行集成概念测试
go run cmd/test/integration_concept_tests.go
```

## ⚙️ 配置说明

### 主要配置特性

```yaml
# 网关基础配置
gateway:
  name: "IoT Gateway Rule Engine Test"
  nats_url: "embedded"                    # 使用嵌入式NATS
  enable_metrics: true
  enable_profiling: true

# 规则引擎配置
rule_engine:
  enabled: true
  rules_dir: "./rules"                    # 外部规则文件目录
  subject: "iot.data.>"                   # 监听的NATS主题
```

### 数据源配置

配置包含3个Mock数据源：

1. **温度传感器** (`temp_sensor_01`)
   - 频率：1秒/次
   - 数据：温度(15-45°C)、湿度(30-90%)

2. **压力传感器** (`pressure_sensor_01`) 
   - 频率：2秒/次
   - 数据：压力(900-1100hPa)、海拔(0-2000m)

3. **振动传感器** (`vibration_sensor_01`)
   - 频率：100ms/次（高频测试）
   - 数据：振动(0-10g)、转速(1000-5000rpm)

### 规则配置

包含4个测试规则：

1. **温度报警** (`temp_alert_simple`)
   ```yaml
   条件: key == 'temperature' && value > 35
   动作: 发送报警消息
   ```

2. **湿度统计** (`humidity_stats`)
   ```yaml
   条件: key == 'humidity'
   动作: 计算5个数据点的统计信息
   ```

3. **振动检查** (`vibration_check`)
   ```yaml
   条件: key == 'vibration' && value > 7.0
   动作: 发送严重报警 + 数据转换
   ```

4. **压力过滤** (`pressure_filter`)
   ```yaml
   条件: key == 'pressure'
   动作: 范围过滤 + 数据标签添加
   ```

## 📊 监控和调试

### 日志文件
```bash
# 查看主日志
tail -f logs/gateway.log

# 查看快速测试日志
cat logs/quick_test.log
```

### Web界面（如果启用）
- **主界面**: http://localhost:8081
- **API接口**: http://localhost:8081/api/*

### WebSocket监控
```bash
# 测试WebSocket连接
curl --include \
     --no-buffer \
     --header "Connection: Upgrade" \
     --header "Upgrade: websocket" \
     --header "Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==" \
     --header "Sec-WebSocket-Version: 13" \
     http://localhost:8090/ws/rules
```

## 🔧 自定义配置

### 修改数据频率

编辑 `config_rule_engine_test.yaml`：

```yaml
southbound:
  adapters:
    - name: "mock_temperature_sensors"
      config:
        interval_ms: 500  # 改为500ms间隔
```

### 添加新规则

在 `test_rules_simple.json` 中添加：

```json
{
  "id": "my_custom_rule",
  "name": "自定义规则",
  "enabled": true,
  "priority": 90,
  "conditions": {
    "type": "simple",
    "field": "value",
    "operator": "gt", 
    "value": 100
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "info",
        "message": "自定义规则触发: {{.Value}}"
      }
    }
  ]
}
```

### 启用MQTT输出

编辑配置文件中的MQTT输出：

```yaml
northbound:
  sinks:
    - name: "mqtt_output"
      type: "mqtt"
      enabled: true  # 改为true
      config:
        params:
          broker: "tcp://localhost:1883"  # 你的MQTT broker地址
```

## 🧪 验证测试结果

### 成功指标
- ✅ 网关启动无错误
- ✅ 规则加载成功
- ✅ 数据源正常生成数据
- ✅ 规则条件正确触发
- ✅ 输出正常工作

### 常见日志内容
```
正常启动日志：
- "Gateway starting..."
- "Rule engine enabled"
- "Loading rules from..."
- "Mock adapter started"

规则触发日志：
- "Rule triggered: temp_alert_simple"
- "Alert: 设备temp_sensor_01温度报警..."
- "Aggregate result: humidity_stats"
```

### 性能指标
- **启动时间**: < 5秒
- **内存使用**: < 100MB（简单测试）
- **CPU使用**: < 10%（正常负载）

## 🔍 故障排除

### 常见问题

1. **端口占用**
   ```bash
   # 检查端口占用
   netstat -tulpn | grep :8080
   netstat -tulpn | grep :8081
   ```

2. **权限问题**
   ```bash
   # 确保脚本可执行
   chmod +x *.sh
   ```

3. **Go版本问题**
   ```bash
   # 检查Go版本（需要1.19+）
   go version
   ```

4. **配置语法错误**
   ```bash
   # 使用验证工具检查
   go run validate_rule_engine.go
   ```

### 调试技巧

1. **启用详细日志**
   ```yaml
   gateway:
     log_level: "debug"  # 改为debug级别
   ```

2. **手动启动观察**
   ```bash
   # 前台运行，观察启动过程
   ./bin/gateway -config config_rule_engine_test.yaml
   ```

3. **逐步验证**
   ```bash
   # 1. 先验证配置
   ./bin/validate

   # 2. 再运行快速测试
   ./quick_test_rules.sh

   # 3. 最后运行完整测试
   ./test_gateway_rules.sh
   ```

## 📈 扩展测试

### 压力测试

修改配置以增加数据频率：

```yaml
southbound:
  adapters:
    - name: "mock_high_frequency"
      config:
        interval_ms: 10     # 100Hz高频数据
```

### 复杂规则测试

添加更复杂的规则条件：

```json
{
  "conditions": {
    "type": "and",
    "conditions": [
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "temperature"
      },
      {
        "type": "simple", 
        "field": "value",
        "operator": "gt",
        "value": 35
      }
    ]
  }
}
```

### 多设备测试

配置多个设备：

```yaml
southbound:
  adapters:
    - name: "mock_device_01"
      config:
        device_id: "sensor_001"
    - name: "mock_device_02" 
      config:
        device_id: "sensor_002"
```

## 📚 相关文档

- **规则引擎架构**: `docs/rule_engine/01_overview.md`
- **配置参考**: `docs/rule_engine/02_configuration.md`
- **动作类型说明**: `docs/rule_engine/03_actions.md`
- **API接口**: `docs/rule_engine/04_api_reference.md`
- **最佳实践**: `docs/rule_engine/05_best_practices.md`
- **测试报告**: `RULE_ENGINE_TEST_REPORT.md`

## ✅ 检查清单

在运行测试前，确保：

- [ ] Go 1.19+ 已安装
- [ ] 配置文件 `config_rule_engine_test.yaml` 存在
- [ ] 规则文件 `test_rules_simple.json` 存在
- [ ] 脚本具有执行权限 (`chmod +x *.sh`)
- [ ] 端口 8080, 8081, 8090 未被占用
- [ ] 有足够的磁盘空间用于日志

---

🎉 **准备就绪！现在可以运行 `./quick_test_rules.sh` 开始测试了！**