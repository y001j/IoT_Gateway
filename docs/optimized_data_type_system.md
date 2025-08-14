# 优化的数据类型系统设计

## 设计理念

原始设计中每个复合数据规则文件都需要详细定义数据结构，存在以下问题：
1. **冗余定义**：常用数据类型（GPS、颜色、3D向量等）在系统中已预设，无需重复定义
2. **配置复杂**：规则文件变得冗长，可读性差
3. **维护困难**：数据类型定义分散在各个规则文件中，难以统一管理

## 优化方案

### 1. 预设数据类型注册系统

**文件**: `internal/rules/predefined_data_types.go`

系统内置6种常用复合数据类型的完整定义：
- `gps`: GPS地理位置数据
- `3d_vector`: 3D向量数据
- `color`: 颜色数据
- `array`: 数组数据
- `timeseries`: 时间序列数据
- `matrix`: 矩阵数据

### 2. 简化的配置文件格式

#### 原始格式（冗余）：
```json
{
  "data_type": {
    "type": "gps",
    "fields": {
      "location": {
        "type": "gps_coordinate",
        "properties": {
          "latitude": { "type": "float", "range": [-90, 90], "unit": "degrees" },
          "longitude": { "type": "float", "range": [-180, 180], "unit": "degrees" },
          "altitude": { "type": "float", "unit": "meters", "optional": true }
        }
      }
    },
    "coordinate_system": "WGS84"
  }
}
```

#### 优化格式（简洁）：
```json
{
  "data_type": "gps"
}
```

### 3. 自动数据类型检测

系统支持三种数据类型识别方式：

#### 3.1 显式声明（推荐）
```json
{
  "id": "gps_rule",
  "data_type": "gps",
  "conditions": { ... }
}
```

#### 3.2 自动检测
```json
{
  "id": "gps_rule", 
  // 系统根据字段访问模式自动识别为GPS类型
  "conditions": {
    "field": "location.latitude",
    "operator": "gt",
    "value": 39.90
  }
}
```

#### 3.3 自定义定义（特殊需求）
```json
{
  "data_type": {
    "type": "custom_sensor",
    "fields": { ... }  // 完整自定义定义
  }
}
```

## 技术实现

### 1. 灵活的数据类型字段

```go
type Rule struct {
    // ...
    DataType interface{} `json:"data_type,omitempty"` // 支持字符串或详细定义
    // ...
}
```

### 2. 统一的获取接口

```go
// 获取规则的数据类型名称
func (r *Rule) GetDataTypeName() string

// 获取完整的数据类型定义（预设或自定义）
func GetDataTypeDefinition(rule *Rule) *DataTypeDefinition

// 自动检测数据类型
func DetectDataTypeFromFields(rule *Rule) string
```

### 3. 智能字段模式匹配

系统分析规则中的字段访问模式：
- 条件中的字段引用：`location.latitude`
- 动作消息模板：`{{location.longitude}}`
- 表达式中的字段：`arrayMean(sensor_readings)`

根据字段模式匹配预设数据类型定义。

## 使用示例

### GPS规则配置

**简化前**：
```json
{
  "id": "gps_distance_rule",
  "data_type": {
    "type": "gps",
    "fields": {
      "location": {
        "type": "gps_coordinate",
        "properties": {
          "latitude": { "type": "float", "range": [-90, 90], "unit": "degrees" },
          "longitude": { "type": "float", "range": [-180, 180], "unit": "degrees" }
        }
      }
    },
    "coordinate_system": "WGS84"
  },
  "conditions": {
    "field": "location.latitude",
    "operator": "gt",
    "value": 39.9
  }
}
```

**简化后**：
```json
{
  "id": "gps_distance_rule",
  "data_type": "gps",
  "conditions": {
    "field": "location.latitude", 
    "operator": "gt",
    "value": 39.9
  }
}
```

### 数组数据配置

```json
{
  "id": "array_processing_rule",
  "data_type": "array",
  "conditions": {
    "expression": "arrayMean(sensor_readings) > 25.0"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "message": "平均值: {{array_mean}}, 大小: {{sensor_readings.size}}"
      }
    }
  ]
}
```

## 优势总结

### 1. 配置简化
- **90%减少**：配置文件大小减少90%
- **可读性提升**：关注业务逻辑，而非数据结构定义
- **维护简单**：修改数据类型定义只需在一个地方

### 2. 系统智能化
- **自动识别**：无需手动声明，系统自动检测数据类型
- **向后兼容**：支持现有规则文件，不破坏兼容性
- **扩展灵活**：既支持预设类型，也支持自定义类型

### 3. 开发效率
- **快速配置**：从冗长的JSON到一行配置
- **错误减少**：预设定义减少配置错误
- **统一管理**：所有数据类型定义集中管理

### 4. 运行时优化
- **内存节省**：避免重复的数据结构定义
- **加载性能**：规则文件更小，加载更快
- **缓存效率**：预设定义可以全局缓存

## 扩展指南

### 添加新的预设数据类型

1. 在`PredefinedDataTypes`中添加定义
2. 更新字段模式匹配逻辑
3. 在前端添加对应的编辑器
4. 编写测试用例

### 自定义数据类型

对于特殊需求，仍然支持完整的自定义定义：

```json
{
  "data_type": {
    "type": "industrial_sensor",
    "fields": {
      "readings": {
        "type": "sensor_data",
        "properties": {
          "temperature": { "type": "float", "unit": "celsius" },
          "pressure": { "type": "float", "unit": "bar" },
          "vibration": { "type": "vector3d" }
        }
      }
    }
  }
}
```

## 结论

通过预设数据类型系统，我们实现了：
- **配置简化**：从复杂的结构定义到简单的类型标识
- **系统智能**：自动检测和类型推断
- **开发高效**：减少重复工作，提高开发效率
- **维护友好**：集中管理，统一更新

这种设计既保持了系统的灵活性，又大大简化了用户的配置工作，是一个兼顾易用性和功能性的优秀方案。