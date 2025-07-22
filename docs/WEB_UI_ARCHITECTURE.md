# IoT Gateway Web UI 架构详解

## 架构概述

IoT Gateway Web UI 采用现代化的前后端分离架构，提供高性能、可扩展的 Web 管理界面。系统设计遵循单一职责原则，通过清晰的分层架构实现各模块的解耦。

## 系统分层架构

### 1. 表示层 (Presentation Layer)
```
┌─────────────────────────────────────────────────────────────┐
│                    表示层 (React Frontend)                   │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   页面组件     │  │   布局组件     │  │   业务组件     │   │
│  │   (Pages)     │  │   (Layout)    │  │ (Components)  │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   图表组件     │  │   状态管理     │  │   路由管理     │   │
│  │   (Charts)    │  │   (Store)     │  │   (Router)    │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**职责:**
- 用户界面展示
- 用户交互处理
- 状态管理
- 路由导航
- 数据可视化

**技术栈:**
- React 18 + TypeScript
- Ant Design UI组件
- Zustand 状态管理
- React Router 路由
- ECharts 图表库

### 2. 服务层 (Service Layer)
```
┌─────────────────────────────────────────────────────────────┐
│                     服务层 (API Services)                   │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   认证服务     │  │   插件服务     │  │   规则服务     │   │
│  │ AuthService   │  │ PluginService │  │ RuleService   │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   告警服务     │  │   系统服务     │  │   监控服务     │   │
│  │ AlertService  │  │ SystemService │  │MonitorService │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**职责:**
- API 请求封装
- 数据转换处理
- 错误处理
- 缓存管理
- 实时数据处理

### 3. API 层 (API Layer)
```
┌─────────────────────────────────────────────────────────────┐
│                     API 层 (HTTP/WebSocket)                 │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   RESTful API │  │   WebSocket   │  │   中间件       │   │
│  │   (Gin)       │  │   (实时通信)   │  │ (Middleware)  │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   JWT认证     │  │   CORS配置    │  │   日志记录     │   │
│  │ (Auth Guard)  │  │ (Cross-Origin)│  │  (Logging)    │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**职责:**
- HTTP API 路由
- WebSocket 连接管理
- 请求认证授权
- 跨域处理
- 请求日志记录

### 4. 业务逻辑层 (Business Logic Layer)
```
┌─────────────────────────────────────────────────────────────┐
│                   业务逻辑层 (Services)                     │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   认证业务     │  │   插件业务     │  │   规则业务     │   │
│  │ AuthBusiness  │  │ PluginBusiness│  │ RuleBusiness  │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   告警业务     │  │   系统业务     │  │   监控业务     │   │
│  │ AlertBusiness │  │ SystemBusiness│  │MonitorBusiness│   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**职责:**
- 业务规则实现
- 数据验证
- 权限检查
- 业务流程控制
- 跨模块协调

### 5. 数据层 (Data Layer)
```
┌─────────────────────────────────────────────────────────────┐
│                      数据层 (Data Access)                   │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   SQLite DB   │  │   文件存储     │  │   内存缓存     │   │
│  │  (用户认证)    │  │ (配置/规则)    │  │  (临时数据)    │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   NATS 消息   │  │   插件管理器   │  │   规则引擎     │   │
│  │  (实时数据)    │  │ (Plugin Mgr)  │  │ (Rule Engine) │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**职责:**
- 数据持久化
- 实时数据流
- 配置管理
- 缓存控制
- 数据同步

## 核心组件架构

### 1. 前端组件架构

#### 1.1 页面组件层次
```
App (应用根组件)
├── Router (路由配置)
│   ├── Login (登录页)
│   └── MainLayout (主布局)
│       ├── Dashboard (仪表板)
│       ├── PluginsPage (插件管理)
│       ├── RulesPage (规则管理)
│       ├── AlertsPage (告警管理)
│       ├── MonitoringPage (监控页面)
│       └── SystemPage (系统设置)
└── Global Components (全局组件)
    ├── ProtectedRoute (路由保护)
    ├── ErrorBoundary (错误边界)
    └── LoadingSpinner (加载指示器)
```

#### 1.2 状态管理架构
```typescript
// Zustand Store 架构
interface GlobalState {
  // 认证状态
  auth: {
    user: User | null;
    accessToken: string | null;
    refreshToken: string | null;
    isAuthenticated: boolean;
  };
  
  // 系统状态
  system: {
    status: SystemStatus;
    metrics: SystemMetrics;
    config: SystemConfig;
  };
  
  // 实时数据
  realtime: {
    iotData: IoTDataPoint[];
    systemUpdates: SystemUpdate[];
    ruleEvents: RuleEvent[];
    alerts: Alert[];
  };
}
```

### 2. 后端服务架构

#### 2.1 服务组织结构
```go
// 服务容器
type Services struct {
    Auth          *AuthService
    Plugin        *PluginService
    Rule          *RuleService
    Alert         *AlertService
    System        *SystemService
    Monitoring    *MonitoringService
}

// 服务配置
type ServiceConfig struct {
    Auth          *models.AuthConfig
    DBPath        string
    PluginManager *plugin.Manager
    RuleManager   *rules.Manager
}
```

#### 2.2 API 处理器架构
```go
// 处理器接口
type Handler interface {
    Setup(router *gin.RouterGroup)
}

// 具体处理器实现
type AuthHandler struct {
    service *AuthService
}

type PluginHandler struct {
    service *PluginService
}

type RuleHandler struct {
    service *RuleService
}
```

## 数据流架构

### 1. 请求响应流程
```
Frontend Request → API Gateway → Handler → Service → Data Source → Response
     ↑                                                                  ↓
     └──────────────── HTTP Response ←──────────────────────────────────┘
```

详细流程:
1. **前端发起请求**: React组件通过Service调用API
2. **API网关处理**: Gin路由匹配并应用中间件
3. **处理器调用**: Handler接收请求并参数验证
4. **业务服务处理**: Service执行业务逻辑
5. **数据源访问**: 访问数据库、文件、NATS等
6. **响应返回**: 层层返回响应数据到前端

### 2. 实时数据流程
```
NATS Message → WebSocket Handler → WebSocket Connection → Frontend
     ↑                                                         ↓
IoT Device ────→ Plugin Manager ────→ NATS Bus ────→ Real-time Update
```

详细流程:
1. **IoT设备数据**: 设备通过适配器发送数据
2. **消息总线**: 数据发布到NATS主题
3. **WebSocket订阅**: 后端订阅NATS消息
4. **实时推送**: 通过WebSocket推送到前端
5. **前端更新**: React组件实时更新界面

### 3. 认证授权流程
```
Login Request → Auth Handler → JWT Generate → Response with Tokens
     ↓
Store Tokens → API Request with Token → JWT Verify → Protected Resource
     ↓
Token Refresh → New Tokens → Continue Access
```

## 安全架构

### 1. 认证安全
- **JWT双Token机制**: Access Token + Refresh Token
- **Token自动刷新**: 前端自动处理Token过期
- **安全存储**: Token存储在内存中，避免XSS攻击
- **登出清理**: 主动清理所有认证信息

### 2. 授权控制
- **RBAC模型**: 基于角色的访问控制
- **API级权限**: 每个API端点权限检查
- **路由保护**: 前端路由级别权限控制
- **操作级授权**: 敏感操作额外权限验证

### 3. 传输安全
- **HTTPS强制**: 生产环境强制HTTPS
- **CORS配置**: 严格的跨域资源共享策略
- **请求验证**: 输入参数严格验证
- **SQL注入防护**: 参数化查询防止注入

## 扩展性架构

### 1. 微服务准备
- **模块化设计**: 各功能模块独立可分离
- **接口标准化**: 统一的API接口规范
- **配置外部化**: 支持外部配置管理
- **服务发现准备**: 为服务注册发现做准备

### 2. 插件化架构
- **前端插件**: 支持动态加载功能模块
- **后端扩展**: 通过接口扩展新功能
- **主题系统**: 支持自定义主题和样式
- **国际化**: 多语言支持架构

### 3. 性能扩展
- **水平扩展**: 支持多实例部署
- **缓存策略**: 多级缓存提升性能
- **异步处理**: 耗时操作异步化
- **连接池**: 数据库连接池管理

## 监控和诊断架构

### 1. 日志架构
```
Application Logs → Structured Logging → Log Aggregation → Analysis
     ↓
Error Tracking → Alert System → Notification → Response
```

### 2. 性能监控
- **前端性能**: 页面加载、渲染性能监控
- **API性能**: 请求响应时间、错误率监控
- **资源监控**: CPU、内存、网络使用监控
- **业务监控**: 用户行为、功能使用统计

### 3. 健康检查
- **系统健康**: 各服务组件健康状态
- **依赖检查**: 外部依赖服务可用性
- **自动恢复**: 故障自动检测和恢复
- **告警通知**: 异常情况及时通知

## 部署架构

### 1. 容器化部署
```dockerfile
# 前端容器
Frontend Container (Nginx + React Build)
    ↓
# 后端容器  
Backend Container (Go Binary + Config)
    ↓
# 数据容器
Data Container (SQLite + File Storage)
```

### 2. 配置管理
- **环境分离**: 开发、测试、生产环境配置
- **敏感信息**: 密钥、密码等安全信息管理
- **配置热更新**: 运行时配置动态更新
- **版本控制**: 配置版本化管理

### 3. 高可用部署
- **负载均衡**: 多实例负载分发
- **故障转移**: 自动故障检测和切换
- **数据备份**: 定期数据备份和恢复
- **灾难恢复**: 完整的灾难恢复方案

---

此架构文档详细描述了 IoT Gateway Web UI 的技术架构，为系统的开发、部署和维护提供了全面的技术指导。