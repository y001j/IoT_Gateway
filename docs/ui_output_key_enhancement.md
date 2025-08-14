# 前端输出字段名提示功能增强

## 📋 概述

为了帮助用户理解数据转换中【输出字段名】字段的重要性，我们在前端UI中添加了详细的提示信息。

## 🎯 功能特性

### 1. **智能提示标签**
- 添加了问号图标(QuestionCircleOutlined)，悬停显示详细说明
- 提示内容："不设置时将直接覆盖原始字段的值，原始数据将丢失。建议设置新的字段名以保留原始数据。"

### 2. **动态状态指示**
根据用户是否填写输出字段名，显示不同的状态提示：
- ✅ **已设置**："将创建新字段，原始数据保留"  
- ⚠️ **未设置**："留空将直接覆盖原始字段的值，原始数据丢失"

### 3. **优化的占位符文本**
- 从 `"转换后的字段名，如 temperature_fahrenheit"` 
- 更新为 `"转换后的字段名，如 temperature_fahrenheit（推荐设置）"`

## 🔧 技术实现

### Transform 动作配置
```tsx
<Form.Item 
  label={
    <Space>
      输出字段名
      <Tooltip title="不设置时将直接覆盖原始字段的值，原始数据将丢失。建议设置新的字段名以保留原始数据。">
        <QuestionCircleOutlined style={{ color: '#1890ff' }} />
      </Tooltip>
    </Space>
  }
>
  <Input
    placeholder="转换后的字段名，如 temperature_fahrenheit（推荐设置）"
    value={action.config.output_key || ''}
    onChange={(e) => updateActionConfig(index, 'output_key', e.target.value)}
  />
  <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
    {action.config.output_key ? 
      '✅ 将创建新字段，原始数据保留' : 
      '⚠️ 留空将直接覆盖原始字段的值，原始数据丢失'
    }
  </div>
</Form.Item>
```

### Aggregate 动作配置
```tsx
<Form.Item 
  label={
    <Space>
      输出字段名
      <Tooltip title="聚合结果的字段名。支持模板变量，如 {{.Key}}_stats。留空将使用默认字段名。">
        <QuestionCircleOutlined style={{ color: '#1890ff' }} />
      </Tooltip>
    </Space>
  }
>
  <Input
    placeholder="如 {{.Key}}_stats（支持模板变量）"
    value={outputKey || ''}
    // ... onChange handler
  />
</Form.Item>
```

## 💡 用户体验改进

### 之前
- 用户不清楚留空输出字段名的后果
- 可能意外丢失原始数据
- 缺少最佳实践指导

### 现在  
- 🎯 **明确提示**：悬停查看详细说明
- ⚡ **实时反馈**：根据输入状态显示不同提示
- 📚 **最佳实践**：推荐设置输出字段名
- 🔍 **清晰标识**：用图标和颜色区分状态

## 📊 数据处理行为说明

### 设置输出字段名的情况
```json
// 配置
{
  "type": "transform",
  "config": {
    "type": "scale",
    "parameters": { "factor": 2.0 },
    "output_key": "temperature_scaled"
  }
}

// 结果：原始数据保留
原始: { "temperature": 25.5, "device_id": "sensor_01" }
处理后: { 
  "temperature": 25.5,           // 原始数据保留
  "temperature_scaled": 51.0,    // 新字段
  "device_id": "sensor_01" 
}
```

### 不设置输出字段名的情况  
```json
// 配置
{
  "type": "transform",
  "config": {
    "type": "scale", 
    "parameters": { "factor": 2.0 }
    // output_key 未设置
  }
}

// 结果：原始数据丢失
原始: { "temperature": 25.5, "device_id": "sensor_01" }
处理后: { 
  "temperature": 51.0,           // 原始值被覆盖
  "device_id": "sensor_01" 
}
```

## ✅ 更新内容

1. **Transform 动作**：增强输出字段名提示
2. **Aggregate 动作**：增强输出字段名提示  
3. **动态反馈**：基于用户输入显示状态
4. **用户教育**：清晰说明数据处理行为

这些改进帮助用户做出明智的配置决策，避免意外的数据丢失。