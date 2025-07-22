# IoT Gateway Web UI 文档

## 概述

IoT Gateway Web UI 是一个基于现代 Web 技术栈构建的管理界面，为 IoT Gateway 系统提供完整的可视化管理功能。系统采用前后端分离架构，支持实时数据监控、插件管理、规则配置、告警处理等核心功能。

## 技术栈

### 前端技术栈
- **React 18**: 用户界面构建框架
- **TypeScript**: 类型安全的 JavaScript 超集
- **Ant Design**: 企业级 UI 组件库
- **Zustand**: 轻量级状态管理
- **React Router**: 前端路由管理
- **Axios**: HTTP 客户端
- **ECharts**: 数据可视化图表
- **Monaco Editor**: 代码编辑器
- **Vite**: 现代化构建工具

### 后端技术栈
- **Go**: 主要编程语言
- **Gin**: Web 框架
- **NATS**: 消息总线
- **SQLite**: 认证数据存储
- **JWT**: 身份认证
- **WebSocket**: 实时通信

## 系统架构

### 整体架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│                 │    │                 │    │                 │
│   Web Frontend  │◄──►│   Web Backend   │◄──►│ Gateway Runtime │
│   (React SPA)   │    │   (Gin API)     │    │ (Core Service)  │
│                 │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│                 │    │                 │    │                 │
│   Browser       │    │   SQLite DB     │    │   NATS Server   │
│   (Chrome/FF)   │    │   (Auth Data)   │    │ (Message Bus)   │
│                 │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### 模块架构

```
Web UI 系统
├── 前端模块 (React)
│   ├── 页面组件 (Pages)
│   ├── 布局组件 (Layout)
│   ├── 业务组件 (Components)
│   ├── 图表组件 (Charts)
│   ├── 服务层 (Services)
│   ├── 状态管理 (Store)
│   └── 工具函数 (Utils)
│
├── 后端模块 (Go)
│   ├── API 路由 (Routes)
│   ├── 处理器 (Handlers)
│   ├── 服务层 (Services)
│   ├── 数据模型 (Models)
│   ├── 中间件 (Middleware)
│   └── 工具函数 (Utils)
│
└── 通信层
    ├── HTTP API
    ├── WebSocket
    └── NATS 消息
```

## 核心功能

### 1. 用户认证与授权
- JWT Token 认证
- 基于角色的访问控制 (RBAC)
- 自动 Token 刷新机制
- 安全的登录/登出

### 2. 实时仪表板
- 系统状态实时监控
- 设备连接状态显示
- 系统资源使用监控
- 实时数据流可视化
- 告警信息实时推送

### 3. 插件管理
- 插件列表查看
- 插件状态控制 (启动/停止/重启)
- 插件配置管理
- 插件日志查看
- 插件性能统计

### 4. 规则引擎管理
- 规则创建与编辑
- 规则条件配置
- 规则动作设定
- 规则启用/禁用
- 规则执行监控

### 5. 告警管理
- 告警规则配置
- 告警历史查看
- 告警确认与处理
- 通知渠道管理
- 告警统计分析

### 6. 系统监控
- 适配器状态监控
- 数据流监控
- 性能指标监控
- 连接诊断
- 健康检查

### 7. 系统设置
- 系统配置管理
- 用户信息管理
- 安全设置
- 系统重启

## 目录结构

### 前端项目结构
```
web/frontend/
├── public/                 # 静态资源
├── src/
│   ├── components/         # 通用组件
│   │   ├── charts/        # 图表组件
│   │   ├── layout/        # 布局组件
│   │   ├── plugins/       # 插件相关组件
│   │   └── router/        # 路由组件
│   ├── hooks/             # 自定义 Hooks
│   ├── pages/             # 页面组件
│   ├── services/          # API 服务
│   ├── store/             # 状态管理
│   ├── types/             # TypeScript 类型定义
│   ├── App.tsx            # 主应用组件
│   └── main.tsx           # 应用入口
├── package.json           # 依赖配置
├── tsconfig.json          # TypeScript 配置
└── vite.config.ts         # Vite 构建配置
```

### 后端项目结构
```
internal/web/
├── api/                   # API 处理器
│   ├── handlers.go        # 基础处理器
│   ├── auth_handler.go    # 认证处理器
│   ├── plugin_handler.go  # 插件处理器
│   ├── rule_handler.go    # 规则处理器
│   ├── alert_handler.go   # 告警处理器
│   ├── system_handler.go  # 系统处理器
│   ├── websocket_handler.go # WebSocket 处理器
│   └── routes.go          # 路由配置
├── middleware/            # 中间件
│   ├── auth.go           # 认证中间件
│   ├── cors.go           # CORS 中间件
│   ├── logger.go         # 日志中间件
│   └── recovery.go       # 错误恢复中间件
├── models/               # 数据模型
│   ├── auth.go          # 认证模型
│   ├── plugin.go        # 插件模型
│   ├── rule.go          # 规则模型
│   ├── system.go        # 系统模型
│   └── common.go        # 通用模型
├── services/            # 业务服务
│   ├── auth_service.go  # 认证服务
│   ├── plugin_service.go # 插件服务
│   ├── rule_service.go  # 规则服务
│   ├── alert_service.go # 告警服务
│   ├── system_service.go # 系统服务
│   └── services.go      # 服务管理
└── utils/               # 工具函数
    └── jwt.go           # JWT 工具
```

## API 接口设计

### RESTful API 结构
```
/api/v1/
├── auth/                  # 认证相关
│   ├── POST /login        # 用户登录
│   ├── POST /refresh      # Token 刷新
│   ├── POST /logout       # 用户登出
│   ├── GET /profile       # 获取用户信息
│   └── PUT /profile       # 更新用户信息
├── plugins/               # 插件管理
│   ├── GET /              # 获取插件列表
│   ├── GET /:id           # 获取插件详情
│   ├── POST /:id/start    # 启动插件
│   ├── POST /:id/stop     # 停止插件
│   └── POST /:id/restart  # 重启插件
├── rules/                 # 规则管理
│   ├── GET /              # 获取规则列表
│   ├── POST /             # 创建规则
│   ├── GET /:id           # 获取规则详情
│   ├── PUT /:id           # 更新规则
│   └── DELETE /:id        # 删除规则
├── alerts/                # 告警管理
│   ├── GET /              # 获取告警列表
│   ├── POST /             # 创建告警
│   ├── GET /:id           # 获取告警详情
│   └── POST /:id/ack      # 确认告警
├── system/                # 系统管理
│   ├── GET /status        # 系统状态
│   ├── GET /metrics       # 系统指标
│   ├── GET /config        # 系统配置
│   └── POST /restart      # 系统重启
└── ws/                    # WebSocket
    └── GET /realtime      # 实时数据流
```

### WebSocket 消息类型
```typescript
interface WebSocketMessage {
  type: 'system_status' | 'iot_data' | 'rule_event' | 'alert' | 'plugin_status';
  data: any;
  timestamp: string;
}
```

## 数据流和通信

### 1. HTTP API 通信
- 前端使用 Axios 进行 HTTP 请求
- 自动添加 JWT Token 到请求头
- 自动处理 Token 刷新
- 统一错误处理

### 2. WebSocket 实时通信
- 建立持久连接用于实时数据推送
- 支持多种消息类型
- 自动重连机制
- 连接状态监控

### 3. NATS 消息总线
- 后端通过 NATS 获取系统实时数据
- 支持主题订阅和发布
- 可靠的消息传递
- 系统解耦

## 部署模式

### 1. 集成模式 (推荐)
Web UI 作为 Gateway 主服务的一部分运行：
```yaml
# config.yaml
web_ui:
  enabled: true
  port: 8081
```

### 2. 独立模式
Web UI 作为独立服务运行：
```bash
# 运行独立的 Web 服务
go run cmd/server.deprecated/main.go
```

## 安全性

### 1. 认证机制
- JWT Token 认证
- Access Token + Refresh Token 双Token机制
- Token 自动刷新
- 安全的密码存储

### 2. 授权控制
- 基于角色的访问控制
- API 级别的权限检查
- 前端路由权限保护
- 敏感操作权限限制

### 3. 安全传输
- HTTPS 支持
- CORS 配置
- 请求限流
- 输入验证

## 性能优化

### 1. 前端优化
- 组件懒加载
- 虚拟滚动
- 图表数据分页
- 缓存策略

### 2. 后端优化
- 连接池管理
- 数据库查询优化
- 内存缓存
- 压缩传输

### 3. 实时数据优化
- WebSocket 连接复用
- 数据去重
- 批量更新
- 限流控制

## 浏览器兼容性

### 支持的浏览器
- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

### 移动端支持
- 响应式设计
- 触摸友好界面
- 移动端优化布局

## 监控和日志

### 1. 前端监控
- 错误边界处理
- 性能监控
- 用户行为跟踪
- 控制台日志

### 2. 后端监控
- API 请求日志
- 错误日志记录
- 性能指标收集
- 健康检查端点

## 下一步规划

### 功能增强
1. 支持主题切换 (明/暗主题)
2. 国际化支持
3. 高级图表功能
4. 批量操作支持
5. 数据导出功能

### 技术优化
1. 微前端架构
2. PWA 支持
3. 离线功能
4. 性能进一步优化
5. 测试覆盖率提升

---

此文档提供了 IoT Gateway Web UI 的全面概述，包括架构设计、功能特性、实现细节等。更多具体的实现细节和使用说明请参考相关专项文档。