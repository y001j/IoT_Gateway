# IoT 网关系统

一个基于 Go 语言构建的强大而灵活的物联网网关系统，能够处理来自各种物联网设备的数据并将其路由到多个目标。系统采用插件化架构，使用 NATS 作为消息总线，并包含强大的规则引擎用于实时数据处理。

## 主要特性

- **统一应用程序**：单一可执行文件同时提供物联网数据处理和Web管理界面
- **插件化架构**：支持内置和外部插件（适配器和连接器）
- **协议支持**：Modbus、MQTT、HTTP 等多种南向协议
- **多种输出**：MQTT、InfluxDB、Redis、WebSocket、NATS JetStream、控制台等连接器
- **规则引擎**：基于事件的数据处理，支持复杂条件和动作
- **Web界面**：基于 React 的现代化 Web 管理界面
- **REST API**：完整的 API 用于程序化控制
- **实时监控**：基于 WebSocket 的实时数据流
- **热重载**：运行时配置更新，无需重启
- **身份认证**：基于 JWT 的身份认证和基于角色的访问控制

## 系统架构

### 核心组件

1. **运行时**：主要编排层，管理所有服务和 NATS 连接
2. **插件管理器**：处理内置和外部插件的生命周期
3. **规则引擎**：基于事件的数据处理，支持可配置规则
4. **南向适配器**：从各种物联网协议采集数据
5. **北向连接器**：将数据输出到多个目标
6. **Web服务**：HTTP API 和 Web 管理界面

### 数据流

```
物联网设备 → 南向适配器 → NATS 消息总线 → 规则引擎 → 北向连接器
                                    ↓
                              Web界面和API
```

## 快速开始

### 系统要求

- Go 1.19+ (从源码构建时需要)
- NATS 服务器 (默认使用内嵌版本)

### 安装步骤

1. **克隆仓库**
   ```bash
   git clone <仓库地址>
   cd "IoT Gateway"
   ```

2. **构建应用程序**
   ```bash
   go build -o bin/gateway cmd/gateway/main.go
   ```

3. **使用默认配置运行**
   ```bash
   ./bin/gateway -config config.yaml
   ```

### 首次运行

1. **启动网关**
   ```bash
   ./bin/gateway -config config.yaml
   ```

2. **访问Web界面**
   - 在浏览器中打开 `http://localhost:8081`
   - 使用默认凭据登录：`admin/admin123`
   - **重要**：生产环境中请更改默认密码！

3. **监控数据流**
   - 物联网数据处理：`http://localhost:8080/metrics`
   - NATS 主题：使用 `nats sub "iot.data.>"` 监控消息

## 配置说明

### 主配置文件 (`config.yaml`)

```yaml
gateway:
  id: "gateway-001"
  http_port: 8080           # 指标和健康检查端口
  log_level: "info"
  nats_url: "embedded"      # 使用内嵌 NATS 服务器
  plugins_dir: "./plugins"

# 数据输入源
southbound:
  adapters:
    - name: "modbus-sensor"
      type: "modbus"
      config:
        mode: "tcp"
        address: "127.0.0.1:502"
        # ... modbus 配置

# 数据输出目标
northbound:
  sinks:
    - name: "mqtt-cloud"
      type: "mqtt"
      config:
        broker: "tcp://localhost:1883"
        # ... mqtt 配置

# 基于规则的数据处理
rule_engine:
  enabled: true
  rules: []

# Web界面和API
web_ui:
  enabled: true
  port: 8081
  auth:
    enabled: true
    jwt_secret: "请在生产环境中更改"
    default_admin:
      username: "admin"
      password: "admin123"

# 身份认证数据库
database:
  sqlite:
    path: "./data/auth.db"
```

### 支持的协议

#### 南向适配器 (数据输入)
- **Modbus**：支持 TCP/RTU，可配置寄存器
- **MQTT**：订阅 MQTT 主题
- **HTTP**：RESTful 数据接入端点
- **Mock**：用于测试的模拟数据

#### 北向连接器 (数据输出)
- **MQTT**：发布到 MQTT 代理
- **InfluxDB**：时间序列数据库存储
- **Redis**：键值存储和发布/订阅
- **WebSocket**：实时Web客户端
- **JetStream**：NATS 持久化消息
- **Console**：控制台调试输出

## 规则引擎

规则引擎通过可配置规则实时处理数据：

### 规则结构
```json
{
  "id": "temperature-alert",
  "name": "高温报警",
  "enabled": true,
  "conditions": {
    "type": "simple",
    "field": "temperature",
    "operator": "gt",
    "value": 30.0
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "message": "温度过高：{{.temperature}}°C",
        "channels": ["console", "webhook"]
      }
    }
  ]
}
```

### 动作类型
- **Alert**：多渠道通知（控制台、Webhook、邮件、短信）
- **Transform**：数据转换（缩放、偏移、格式转换）
- **Filter**：数据过滤（去重、范围检查）
- **Aggregate**：时间窗口内的统计操作
- **Forward**：将数据路由到外部系统

详细文档请参考 [README_RULE_ENGINE.md](README_RULE_ENGINE.md)。

## Web管理界面

内置的Web界面提供：

- **仪表板**：实时系统概览和指标
- **插件管理**：配置和监控适配器/连接器
- **规则编辑器**：创建和管理处理规则
- **系统设置**：网关配置和状态
- **身份认证**：用户管理和访问控制

### API 端点

- `GET /api/v1/system/status` - 系统健康状态和指标
- `GET /api/v1/plugins` - 列出所有插件
- `POST /api/v1/plugins/{id}/start` - 启动插件
- `GET /api/v1/rules` - 列出处理规则
- `POST /api/v1/rules` - 创建新规则

## 开发指南

### 构建和测试

```bash
# 构建主应用程序
go build -o bin/gateway cmd/gateway/main.go

# 运行测试
go test ./...

# 使用详细日志运行
./bin/gateway -config config.yaml  # (log_level: "debug")
```

### 前端开发

```bash
cd web/frontend
npm install
npm run dev      # 开发服务器
npm run build    # 生产构建
npm run lint     # 代码检查
```

### 插件开发

#### 创建新的适配器
```go
type MyAdapter struct{}

func (a *MyAdapter) Name() string { return "my-adapter" }
func (a *MyAdapter) Init(config any) error { /* ... */ }
func (a *MyAdapter) Start(ctx context.Context) error { /* ... */ }
func (a *MyAdapter) Stop(ctx context.Context) error { /* ... */ }

func init() {
    southbound.RegisterAdapter("my-adapter", &MyAdapter{})
}
```

#### 创建新的连接器
```go
type MySink struct{}

func (s *MySink) Name() string { return "my-sink" }
func (s *MySink) Init(config any) error { /* ... */ }
func (s *MySink) Write(data []byte) error { /* ... */ }

func init() {
    northbound.RegisterSink("my-sink", &MySink{})
}
```

## 监控和调试

### 健康检查
```bash
# 网关健康状态
curl http://localhost:8080/health

# Web服务健康状态
curl http://localhost:8081/health
```

### NATS 监控
```bash
# 安装 NATS CLI
go install github.com/nats-io/natscli/nats@latest

# 监控数据流
nats sub "iot.data.>"

# 检查服务器信息
nats server info
```

### 日志分析
```bash
# 跟踪结构化日志输出
./bin/gateway -config config.yaml | jq

# 按级别过滤
./bin/gateway -config config.yaml | grep '"level":"error"'
```

## 生产部署

### 安全检查清单
- [ ] 更改默认管理员密码
- [ ] 更新 JWT 密钥
- [ ] 启用 HTTPS/TLS
- [ ] 配置防火墙规则
- [ ] 设置日志轮转
- [ ] 启用身份认证

### 性能调优
- 调整插件配置中的缓冲区大小
- 为连接器配置适当的批处理大小
- 监控 NATS JetStream 磁盘使用量
- 生产环境将日志级别设置为 "info" 或 "warn"

### Docker 部署
```dockerfile
FROM golang:1.19-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gateway cmd/gateway/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/gateway .
COPY --from=builder /app/config.yaml .
EXPOSE 8080 8081
CMD ["./gateway", "-config", "config.yaml"]
```

## 故障排除

### 常见问题

**网关无法启动**
- 检查 NATS 端口可用性 (4222)
- 验证配置文件语法
- 确保数据目录可写

**Web界面无法访问**
- 确认配置中 `web_ui.enabled: true`
- 检查端口 8081 可用性
- 验证身份认证设置

**插件加载失败**
- 检查插件目录权限
- 验证插件 JSON 配置
- 查看初始化错误日志

**数据流中断**
- 监控 NATS 主题：`nats sub "iot.data.>"`
- 通过 Web UI 检查适配器连接状态
- 验证连接器配置

### 日志位置
- 应用程序日志：stdout/stderr
- Web 访问日志：嵌入在应用程序日志中
- NATS 日志：嵌入在应用程序日志中
- 插件日志：通过 Web UI 访问

## 架构优势

经过合并优化后的架构具有以下优势：

### 简化部署
- **单一可执行文件**：不再需要分别管理Gateway和Server两个进程
- **统一配置**：所有配置集中在一个文件中
- **减少依赖**：降低了部署复杂度

### 性能提升
- **进程内通信**：Web服务直接访问插件管理器，无需网络调用
- **资源优化**：共享NATS连接和其他系统资源
- **启动速度**：单一进程启动更快

### 运维便利
- **日志统一**：所有组件日志集中输出
- **监控简化**：单一进程的监控和管理
- **故障排查**：减少了进程间通信的问题点

## 项目结构

```
IoT Gateway/
├── cmd/
│   ├── gateway/           # 主程序入口
│   └── server.deprecated/ # 已弃用的独立Web服务器
├── internal/
│   ├── core/             # 核心运行时和Web服务
│   ├── plugin/           # 插件管理
│   ├── rules/            # 规则引擎
│   ├── southbound/       # 南向适配器
│   ├── northbound/       # 北向连接器
│   └── web/              # Web API和服务
├── web/frontend/         # React前端应用
├── configs/              # 配置文件示例
├── plugins/              # 插件定义
├── rules/                # 规则定义
└── docs/                 # 详细文档
```

## 贡献指南

1. Fork 仓库
2. 创建功能分支
3. 进行更改
4. 为新功能添加测试
5. 提交 Pull Request

## 开源协议

[在此添加您的开源协议信息]

## 技术支持

- 文档：参见 `docs/` 目录
- 问题反馈：[创建 Issue](链接到问题页面)
- 讨论交流：[加入讨论](链接到讨论页面)