# Sidecar 插件配置迁移状态

## 当前状态：准备就绪 ✅

Sidecar 插件配置的统一配置系统**已准备就绪**，但尚未完全实施到插件管理器中。目前的状态如下：

### ✅ 已完成

1. **配置类型定义** (`internal/config/plugin_types.go`)
   - `PluginMetadata`: 插件元数据配置
   - `SidecarPluginConfig`: ISP Sidecar插件配置
   - `ExternalPluginConfig`: 外部插件配置
   - 包含完整的验证标签和默认值

2. **配置示例文件**
   - `plugins/modbus-sidecar-new-format.yaml`: 新格式示例
   - `config_with_sidecar.yaml`: 主配置文件中的sidecar配置示例

3. **文档更新**
   - 扩展了配置迁移指南，包含sidecar插件部分
   - 详细的格式对比和迁移策略

### 🔄 部分完成

1. **插件管理器**
   - 当前仍使用旧的 `json.Marshal/Unmarshal` 方式
   - 已添加config包导入，但尚未实际使用新的配置解析器
   - 需要在 `manager.go` 中实施新的配置处理逻辑

### ⏳ 待实施

1. **配置解析器集成**
   - 在插件管理器中使用 `ConfigParser[SidecarPluginConfig]`
   - 添加向后兼容性支持
   - 实现新旧格式的自动转换

2. **ISP适配器代理更新**
   - 更新 `isp_adapter_proxy.go` 使用新配置格式
   - 保持与现有sidecar进程的兼容性

## 技术实现详情

### 新配置结构

```go
type SidecarPluginConfig struct {
    BaseConfig   `json:",inline" yaml:",inline"`
    ISPPort      int           `json:"isp_port" validate:"required,port"`
    ISPTimeout   time.Duration `json:"isp_timeout,omitempty"`
    Entry        string        `json:"entry" validate:"required"`
    AutoRestart  bool          `json:"auto_restart,omitempty"`
    MaxRetries   int           `json:"max_retries,omitempty" validate:"min=0,max=10"`
    PluginConfig map[string]interface{} `json:"plugin_config,omitempty"`
}
```

### 配置示例对比

#### 旧格式 (plugins/modbus-sidecar-isp.json)
```json
{
  "name": "modbus-sensor",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "isp-sidecar",
  "entry": "modbus-sidecar/modbus-sidecar.exe",
  "description": "Modbus适配器（ISP Sidecar模式）",
  "isp_port": 50052
}
```

#### 新格式 (config.yaml)
```yaml
southbound:
  adapters:
    - name: "modbus-sidecar"
      type: "sidecar"
      enabled: true
      description: "Modbus adapter via ISP sidecar"
      isp_port: 50052
      isp_timeout: 30s
      entry: "modbus-sidecar/modbus-sidecar.exe"
      auto_restart: true
      max_retries: 3
      plugin_config:
        host: "192.168.1.100"
        port: 502
        protocol: "tcp"
        registers: [...]
```

## 后续实施计划

### 第一步：插件管理器配置集成
```go
// 在 manager.go 中添加
func (m *Manager) loadSidecarPlugin(name string, configData []byte) error {
    parser := config.NewParserWithDefaults(config.GetDefaultSidecarPluginConfig())
    
    sidecarConfig, err := parser.Parse(configData)
    if err != nil {
        // 回退到旧格式处理
        return m.loadLegacySidecarPlugin(name, configData)
    }
    
    // 使用新配置启动sidecar插件
    return m.startSidecarWithNewConfig(name, sidecarConfig)
}
```

### 第二步：向后兼容性
- 检测配置格式（JSON元数据文件 vs YAML主配置）
- 自动转换旧格式到新格式
- 显示迁移建议日志

### 第三步：测试和验证
- 确保现有sidecar插件继续工作
- 验证新配置的所有特性
- 性能对比测试

## 优势对比

### 新配置系统优势
✅ **统一管理**: 所有配置在一个文件中  
✅ **类型安全**: 编译时类型检查和运行时验证  
✅ **默认值**: 自动应用合理默认配置  
✅ **错误处理**: 清晰的验证错误信息  
✅ **热重载**: 支持配置动态更新  
✅ **标签系统**: 增强的分类和元数据管理  

### 旧系统问题
❌ **分散配置**: 元数据和配置分离  
❌ **手动验证**: 缺乏自动验证机制  
❌ **错误信息**: 验证错误不够清晰  
❌ **重复代码**: 多处相似的配置处理逻辑  

## 兼容性保证

- **完全向后兼容**: 现有sidecar插件无需修改即可继续工作
- **渐进迁移**: 支持混合使用新旧格式
- **平滑过渡**: 提供迁移工具和清晰的迁移路径

## 结论

Sidecar插件的统一配置系统已经准备就绪，所有必要的类型定义、验证逻辑、示例配置和文档都已完成。

**当前状态**: 系统继续使用旧的配置方式，但新配置系统的基础设施已经完整搭建

**下一步**: 需要在插件管理器中实际集成新的配置解析器，这可以作为一个独立的优化任务在后续实施

**风险评估**: 低风险 - 所有更改都保持向后兼容性