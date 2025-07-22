# 指标系统重构说明

## 问题描述
Web UI部分组件仍然尝试通过8081端口的API获取指标数据，导致连接失败。正确的方法是通过8080端口的metrics端点获取数据。

## 解决方案

### 1. 重构监控页面 (MonitoringPage.tsx)

**修改前:**
- 完全依赖 `monitoringService` (8081端口API)
- 获取概览数据、适配器状态、连接器状态都通过监控API

**修改后:**
- 优先使用 `lightweightMetricsService` (8080端口metrics)
- 创建 `loadOverviewFromMetrics()` 函数从轻量级指标获取概览数据
- 监控API仅用于获取详细的适配器/连接器列表（如果可用）
- 降级策略：如果监控API不可用，显示空列表但不影响主要功能

### 2. 重构数据流图表 (DataFlowChart.tsx)

**修改前:**
- 完全依赖 `monitoringService.getDataFlowMetrics()`

**修改后:**
- 优先使用 `lightweightMetricsService.getLightweightMetrics()`
- 基于轻量级指标创建基础数据流数据
- 降级策略：如果轻量级指标失败，再尝试监控API

### 3. 网络配置优化

**添加了:**
- 环境变量配置支持 (`.env` 文件)
- 网络故障排除指南
- 网络连接测试脚本
- 详细的代理配置和错误处理

## 数据流变化

### 修改前
```
前端 → Vite代理 → 8081端口API → 监控服务 → 数据
```

### 修改后
```
前端 → Vite代理 → 8080端口metrics → 网关服务 → 轻量级指标数据
       ↓ (降级)
       → 8081端口API → 监控服务 → 详细数据 (如果可用)
```

## 关键功能保留

### ✅ 正常工作的功能
- 系统概览 (使用轻量级指标)
- 实时指标监控 (RealTimeMetrics)
- 系统指标图表 (SystemMetricsChart)
- 数据流图表 (基于轻量级指标)
- 连接器/适配器统计 (total_sinks, running_sinks等)

### ⚠️ 需要监控API的功能
- 详细的适配器/连接器列表
- 适配器诊断信息
- 连接测试和重启功能

## 测试验证

### 检查轻量级指标
```bash
curl http://localhost:8080/metrics | jq '.gateway'
```

### 验证前端代理
```bash
curl http://localhost:3000/metrics | jq '.gateway.total_sinks'
```

### 运行网络测试
```bash
cd web/frontend
node test-network.js
```

## 预期结果

1. **连接器指标显示正常** - 应该能看到 `total_sinks: 6, running_sinks: 1`
2. **减少8081端口依赖** - 主要功能不再依赖监控API
3. **更好的错误处理** - 即使监控API不可用，核心功能仍能正常工作
4. **性能改进** - 直接从8080端口获取数据，减少中间层

## 注意事项

- 轻量级指标提供的是聚合数据，不包含详细的适配器/连接器列表
- 如需详细的设备信息，仍需要实现8081端口的监控API
- 当前方案在监控API不可用时提供优雅降级