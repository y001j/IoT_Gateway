# 规则引擎快速入门指南

## 前置条件

在开始使用规则引擎之前，请确保：

1. **Go环境**: Go 1.24.3 或更高版本
2. **NATS服务器**: 运行中的NATS服务器
3. **IoT Gateway核心模块**: Core Runtime、Plugin Manager、Southbound Adapters、Northbound Sinks

## 安装和配置

### 1. 克隆项目

```bash
git clone https://github.com/y001j/iot-gateway.git
cd iot-gateway
```

### 2. 安装依赖

```bash
go mod download
```

### 3. 启动NATS服务器

```bash
# 使用Docker
docker run -p 4222:4222 -p 8222:8222 nats:latest

# 或者本地安装
nats-server
```

### 4. 配置规则引擎

创建配置文件 `config/rules.yaml`:

```yaml
rule_engine:
  # 规则文件目录
  rules_dir: "./rules"
  
  # 文件监控
  watch_files: true
  watch_interval: "1s"
  
  # 性能参数
  max_concurrent_rules: 100
  action_timeout: "30s"
  evaluation_timeout: "5s"
  
  # NATS配置
  nats:
    servers: ["nats://localhost:4222"]
    input_subject: "iot.processed.*"
    output_subject: "iot.rules.*"
    queue_group: "rule_engine"
```

## 创建第一个规则

### 1. 创建规则目录

```bash
mkdir -p rules
```

### 2. 创建温度监控规则

创建文件 `rules/temperature_monitor.json`:

```json
[
  {
    "id": "temperature_alert",
    "name": "温度报警",
    "description": "当温度超过30度时发送报警",
    "enabled": true,
    "priority": 1,
    "conditions": {
      "type": "simple",
      "field": "key",
      "operator": "eq",
      "value": "temperature"
    },
    "actions": [
      {
        "type": "alert",
        "config": {
          "level": "warning",
          "message": "设备 {{.DeviceID}} 温度过高: {{.Value}}°C",
          "channels": [
            {
              "type": "console",
              "enabled": true
            }
          ]
        }
      }
    ]
  }
]
```

### 3. 创建主程序

创建文件 `cmd/rule_engine/main.go`:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/y001j/iot-gateway/internal/rules"
    "github.com/nats-io/nats.go"
)

func main() {
    // 连接NATS
    nc, err := nats.Connect("nats://localhost:4222")
    if err != nil {
        log.Fatal("连接NATS失败:", err)
    }
    defer nc.Close()
    
    // 创建规则管理器
    manager := rules.NewManager("rules/")
    
    // 加载规则
    if err := manager.LoadRules(); err != nil {
        log.Fatal("加载规则失败:", err)
    }
    
    log.Printf("已加载 %d 个规则", len(manager.ListRules()))
    
    // 启动规则引擎
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    if err := manager.Start(ctx); err != nil {
        log.Fatal("启动规则引擎失败:", err)
    }
    
    log.Println("规则引擎启动成功，等待数据...")
    
    // 等待退出信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("正在关闭规则引擎...")
    cancel()
}
```

### 4. 运行规则引擎

```bash
go run cmd/rule_engine/main.go
```

## 测试规则

### 1. 发送测试数据

使用NATS CLI工具发送测试数据：

```bash
# 安装NATS CLI
go install github.com/nats-io/natscli/nats@latest

# 发送温度数据（触发报警）
nats pub iot.processed.test '{
  "device_id": "sensor_001",
  "key": "temperature",
  "value": 35.5,
  "type": "float",
  "timestamp": "2024-01-01T12:00:00Z",
  "tags": {}
}'

# 发送正常温度数据（不触发报警）
nats pub iot.processed.test '{
  "device_id": "sensor_001",
  "key": "temperature",
  "value": 25.0,
  "type": "float",
  "timestamp": "2024-01-01T12:00:00Z",
  "tags": {}
}'
```

### 2. 查看输出

在规则引擎控制台中，您应该看到类似输出：

```
2024/01/01 12:00:00 规则引擎启动成功，等待数据...
2024/01/01 12:00:01 ALERT [WARNING] 设备 sensor_001 温度过高: 35.5°C
```

## 添加更多规则

### 1. 数据转发规则

创建文件 `rules/data_forward.json`:

```json
[
  {
    "id": "data_forward",
    "name": "数据转发",
    "description": "将所有传感器数据转发到文件",
    "enabled": true,
    "priority": 10,
    "conditions": {
      "type": "simple",
      "field": "device_id",
      "operator": "startswith",
      "value": "sensor_"
    },
    "actions": [
      {
        "type": "forward",
        "config": {
          "targets": [
            {
              "name": "data_log",
              "type": "file",
              "enabled": true,
              "config": {
                "path": "./logs/sensor_data.json",
                "format": "json",
                "append": true
              }
            }
          ]
        }
      }
    ]
  }
]
```

### 2. 数据转换规则

创建文件 `rules/data_transform.json`:

```json
[
  {
    "id": "celsius_to_fahrenheit",
    "name": "摄氏度转华氏度",
    "description": "将温度从摄氏度转换为华氏度",
    "enabled": true,
    "priority": 5,
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
          "field": "tags.unit",
          "operator": "eq",
          "value": "celsius"
        }
      ]
    },
    "actions": [
      {
        "type": "transform",
        "config": {
          "field": "value",
          "transforms": [
            {
              "type": "scale",
              "factor": 1.8
            },
            {
              "type": "offset",
              "value": 32
            }
          ],
          "output_field": "temperature_f"
        }
      }
    ]
  }
]
```

### 3. 重新加载规则

规则引擎支持热更新，保存新规则文件后会自动加载：

```bash
# 查看日志确认规则已加载
tail -f logs/rule_engine.log
```

## 监控和调试

### 1. 启用详细日志

修改配置文件启用调试日志：

```yaml
rule_engine:
  log_level: "debug"
  log_rule_execution: true
```

### 2. 监控规则执行

```bash
# 监控所有规则输出
nats sub "iot.rules.>"

# 监控特定设备的规则输出
nats sub "iot.rules.sensor_001"

# 监控错误
nats sub "iot.errors"
```

### 3. 检查规则状态

创建简单的状态检查工具：

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/y001j/iot-gateway/internal/rules"
)

func main() {
    manager := rules.NewManager("rules/")
    if err := manager.LoadRules(); err != nil {
        log.Fatal(err)
    }
    
    rulesList := manager.ListRules()
    fmt.Printf("规则总数: %d\n", len(rulesList))
    
    for _, rule := range rulesList {
        status := "禁用"
        if rule.Enabled {
            status = "启用"
        }
        fmt.Printf("- %s (%s): %s\n", rule.Name, rule.ID, status)
    }
}
```

## 常见问题

### 1. 规则不生效

**检查项目**:
- 规则是否启用 (`enabled: true`)
- 条件是否正确匹配数据
- 数据格式是否符合预期
- NATS连接是否正常

**调试方法**:
```bash
# 检查NATS连接
nats server check

# 查看规则引擎日志
tail -f logs/rule_engine.log

# 发送测试数据
nats pub iot.processed.test '{"device_id":"test","key":"debug","value":1}'
```

### 2. 性能问题

**优化建议**:
- 简化条件逻辑
- 使用异步动作
- 调整缓存大小
- 增加系统资源

### 3. 文件权限问题

```bash
# 确保目录权限
mkdir -p logs rules
chmod 755 logs rules

# 确保规则引擎有写入权限
touch logs/sensor_data.json
chmod 644 logs/sensor_data.json
```

## 下一步

现在您已经成功运行了规则引擎，可以：

1. **学习更多动作类型**: 查看 [规则引擎文档](rule_engine.md)
2. **查看完整示例**: 参考 `examples/rules/complete_examples.json`
3. **集成到现有系统**: 将规则引擎集成到您的IoT Gateway中
4. **自定义动作处理器**: 开发自定义的动作处理器
5. **性能调优**: 根据实际负载调整配置参数

## 支持

如果遇到问题：

1. 查看 [故障排除指南](rule_engine.md#故障排除)
2. 搜索现有的 [Issues](https://github.com/y001j/iot-gateway/issues)
3. 创建新的 Issue 描述问题
4. 联系维护团队获取支持 