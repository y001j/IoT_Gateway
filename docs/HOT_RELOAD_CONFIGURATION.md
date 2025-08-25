# IoT Gateway 热加载配置开关功能

## 概述

IoT Gateway 现在支持通过配置文件来开启或关闭热加载功能。这对于某些关闭了文件变更检查通知功能的操作系统特别有用，可以避免运行时错误。

## 配置选项

### 1. 全局配置文件热加载

在 `config.yaml` 中配置全局配置文件的热加载：

```yaml
gateway:
  # 其他配置...
  
  # 热加载配置
  hot_reload:
    enabled: true                    # 启用/禁用配置文件热加载
    graceful_fallback: true          # 热加载失败时优雅降级
    retry_interval: "30s"            # 监控器启动失败后重试间隔
    max_retries: 3                   # 最大重试次数
```

### 2. 规则引擎热加载

在 `config.yaml` 中配置规则文件的热加载：

```yaml
rule_engine:
  enabled: true
  rules_dir: "./rules"
  subject: "iot.data.>"
  
  # 规则文件热加载配置
  hot_reload:
    enabled: true                    # 启用/禁用规则文件热加载
    graceful_fallback: true          # 热加载失败时优雅降级
    retry_interval: "30s"            # 文件监控器启动失败后重试间隔
    max_retries: 3                   # 最大重试次数
    debounce_delay: "100ms"          # 文件变更防抖延迟
```

## 配置参数详解

### `enabled` (bool)
- **默认值**: `true`
- **说明**: 控制是否启用热加载功能
- **设置为 false**: 完全禁用文件监控，不会尝试创建 fsnotify watcher

### `graceful_fallback` (bool)
- **默认值**: `true` 
- **说明**: 当热加载功能启动失败时是否优雅降级
- **设置为 true**: 热加载失败时记录警告但继续运行系统
- **设置为 false**: 热加载失败时返回错误，可能导致系统启动失败

### `retry_interval` (string)
- **默认值**: `"30s"`
- **说明**: 监控器启动失败后的重试间隔
- **格式**: 时间字符串，如 `"30s"`, `"1m"`, `"5m"`

### `max_retries` (int)
- **默认值**: `3`
- **说明**: 最大重试次数
- **设置为 0**: 不重试

### `debounce_delay` (string, 仅规则引擎)
- **默认值**: `"100ms"`
- **说明**: 文件变更事件的防抖延迟，防止频繁触发
- **格式**: 时间字符串，如 `"100ms"`, `"200ms"`, `"1s"`

## 使用场景

### 1. 禁用热加载的场景

当遇到以下情况时，建议禁用热加载：

- 操作系统关闭了 `inotify` 或文件系统事件通知
- 容器环境中文件监控不可用
- 嵌入式系统或资源受限环境
- 生产环境中希望避免文件监控的性能开销

**配置示例**:
```yaml
gateway:
  hot_reload:
    enabled: false
    graceful_fallback: true

rule_engine:
  hot_reload:
    enabled: false
    graceful_fallback: true
```

### 2. 启用优雅降级的场景

在不确定系统是否支持文件监控时，建议启用优雅降级：

```yaml
gateway:
  hot_reload:
    enabled: true
    graceful_fallback: true    # 失败时不影响系统启动

rule_engine:
  hot_reload:
    enabled: true
    graceful_fallback: true    # 失败时不影响规则引擎启动
```

### 3. 严格模式的场景

如果需要确保热加载功能正常工作，可以禁用优雅降级：

```yaml
gateway:
  hot_reload:
    enabled: true
    graceful_fallback: false   # 失败时返回错误

rule_engine:
  hot_reload:
    enabled: true
    graceful_fallback: false   # 失败时返回错误
```

## API 接口

### 规则引擎热加载控制

规则引擎服务提供了动态控制热加载的 API：

```go
// 获取当前热加载状态
status := ruleEngineService.GetHotReloadStatus()

// 动态启用/禁用热加载
err := ruleEngineService.SetHotReloadEnabled(false) // 禁用
err := ruleEngineService.SetHotReloadEnabled(true)  // 启用
```

## 日志输出

### 正常启动日志
```
INFO 规则文件热加载配置已设置 hot_reload_enabled=true graceful_fallback=true
INFO 规则文件监控器已启动 rules_dir=./rules
```

### 禁用热加载日志
```
INFO 规则文件热加载已禁用
INFO 配置文件热加载已禁用
```

### 优雅降级日志
```
ERROR 启动规则文件监控器失败 error="operation not permitted"
WARN 启用优雅降级，继续运行但不监控规则文件变更
```

## 测试

项目提供了测试脚本来验证热加载功能：

```bash
# 运行热加载功能测试
go run test_hotreload.go

# 使用禁用热加载的配置启动
./bin/gateway -config config_no_hotreload.yaml
```

## 注意事项

1. **性能影响**: 禁用热加载可以轻微提升性能，特别是在大量规则文件的场景下
2. **开发便利性**: 开发环境建议启用热加载以提高开发效率
3. **生产环境**: 生产环境可根据实际需求选择是否启用热加载
4. **容器部署**: 容器环境中建议禁用热加载，通过重启容器来更新配置
5. **文件权限**: 确保应用有足够权限访问配置文件和规则文件目录

## 错误排查

### 常见错误及解决方案

1. **`operation not permitted`**
   - 原因：系统不支持文件监控或权限不足
   - 解决：启用 `graceful_fallback` 或禁用 `enabled`

2. **`too many open files`**
   - 原因：系统文件描述符限制
   - 解决：增加系统限制或禁用热加载

3. **`no space left on device`**
   - 原因：inotify 监控数量达到系统限制
   - 解决：调整系统参数或禁用热加载

通过合理配置热加载选项，IoT Gateway 可以在各种环境中稳定运行。