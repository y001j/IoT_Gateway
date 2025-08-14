# 复合数据类型定义规范

## 概述

为了使IoT Gateway规则引擎和前端编辑器能够正确识别和处理复合数据，我们在规则配置文件中引入了 `data_type` 字段，用于定义数据的结构和类型信息。

## 数据类型定义结构

```json
{
  "data_type": {
    "type": "主数据类型",
    "fields": {
      "字段名": {
        "type": "字段类型",
        "properties": {
          "属性名": {
            "type": "属性数据类型",
            "range": [最小值, 最大值],
            "unit": "单位",
            "optional": true,
            "computed": true
          }
        }
      }
    },
    "additional_metadata": "类型特定的元数据"
  }
}
```

## 支持的数据类型

### 1. GPS/地理位置数据 (gps)

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
    "coordinate_system": "WGS84|GCJ02|BD09"
  }
}
```

**字段访问示例**：
- `location.latitude` - 纬度
- `location.longitude` - 经度  
- `location.altitude` - 海拔（可选）

### 2. 3D向量数据 (3d_vector)

```json
{
  "data_type": {
    "type": "3d_vector",
    "fields": {
      "acceleration": {
        "type": "vector3d",
        "properties": {
          "x": { "type": "float", "unit": "m/s²" },
          "y": { "type": "float", "unit": "m/s²" },
          "z": { "type": "float", "unit": "m/s²" },
          "magnitude": { "type": "float", "unit": "m/s²", "computed": true }
        }
      }
    }
  }
}
```

**字段访问示例**：
- `acceleration.x` - X轴分量
- `acceleration.y` - Y轴分量
- `acceleration.z` - Z轴分量
- `acceleration.magnitude` - 向量模长（计算得出）

### 3. 颜色数据 (color)

```json
{
  "data_type": {
    "type": "color",
    "fields": {
      "color": {
        "type": "color_data",
        "properties": {
          "r": { "type": "int", "range": [0, 255], "unit": "rgb" },
          "g": { "type": "int", "range": [0, 255], "unit": "rgb" },
          "b": { "type": "int", "range": [0, 255], "unit": "rgb" },
          "hue": { "type": "float", "range": [0, 360], "unit": "degrees", "computed": true },
          "saturation": { "type": "float", "range": [0, 1], "unit": "percentage", "computed": true },
          "lightness": { "type": "float", "range": [0, 1], "unit": "percentage", "computed": true }
        }
      }
    },
    "color_space": "RGB|HSL|HSV"
  }
}
```

**字段访问示例**：
- `color.r`, `color.g`, `color.b` - RGB值
- `color.hue` - 色相（计算得出）
- `color.saturation` - 饱和度（计算得出）
- `color.lightness` - 亮度（计算得出）

### 4. 数组数据 (array)

```json
{
  "data_type": {
    "type": "array",
    "fields": {
      "data_array": {
        "type": "numeric_array",
        "properties": {
          "values": { "type": "array[float]", "min_length": 0, "max_length": 1000 },
          "length": { "type": "int", "computed": true },
          "sum": { "type": "float", "computed": true },
          "average": { "type": "float", "computed": true },
          "min": { "type": "float", "computed": true },
          "max": { "type": "float", "computed": true }
        }
      }
    },
    "array_type": "numeric|string|object"
  }
}
```

### 5. 时间序列数据 (timeseries)

```json
{
  "data_type": {
    "type": "timeseries",
    "fields": {
      "series": {
        "type": "time_series",
        "properties": {
          "timestamps": { "type": "array[timestamp]" },
          "values": { "type": "array[float]" },
          "interval": { "type": "duration", "unit": "seconds", "computed": true },
          "trend": { "type": "float", "computed": true },
          "seasonality": { "type": "object", "computed": true }
        }
      }
    },
    "time_unit": "seconds|minutes|hours|days"
  }
}
```

### 6. 矩阵数据 (matrix)

```json
{
  "data_type": {
    "type": "matrix",
    "fields": {
      "matrix": {
        "type": "numeric_matrix",
        "properties": {
          "rows": { "type": "int", "computed": true },
          "cols": { "type": "int", "computed": true },
          "values": { "type": "array[array[float]]" },
          "determinant": { "type": "float", "computed": true },
          "rank": { "type": "int", "computed": true }
        }
      }
    },
    "matrix_type": "dense|sparse"
  }
}
```

## 字段属性说明

### 基础属性
- **type**: 数据类型 (int, float, string, boolean, array, object)
- **range**: 数值范围限制 [最小值, 最大值]
- **unit**: 数据单位 (degrees, meters, m/s², rgb, percentage等)
- **optional**: 是否可选字段 (默认false)
- **computed**: 是否为计算得出的字段 (默认false)

### 数组属性
- **min_length/max_length**: 数组长度限制
- **array_type**: 数组元素类型 (float, int, string等)

### 特殊元数据
- **coordinate_system**: GPS坐标系统
- **color_space**: 颜色空间
- **array_type**: 数组元素类型
- **time_unit**: 时间单位
- **matrix_type**: 矩阵类型

## 使用场景

### 1. 规则引擎处理
- **字段验证**: 根据range和type验证输入数据
- **单位转换**: 根据unit信息进行单位转换
- **计算字段**: 自动计算computed字段值

### 2. 前端编辑器
- **类型识别**: 根据data_type选择合适的编辑器
- **字段提示**: 显示可用字段列表和类型信息
- **输入验证**: 根据range和type限制用户输入

### 3. 数据展示
- **单位显示**: 在界面上显示数据单位
- **格式化**: 根据类型格式化数据显示
- **字段文档**: 提供字段说明和访问路径

## 最佳实践

1. **完整定义**: 为所有复合数据规则文件添加data_type定义
2. **准确类型**: 根据实际数据结构选择正确的数据类型
3. **合理范围**: 设置合理的数值范围限制
4. **清晰单位**: 明确标注数据单位
5. **计算标记**: 正确标记computed字段，避免用户输入

## 向后兼容性

- 没有data_type定义的规则文件仍然可以工作
- 前端编辑器会回退到通用编辑模式
- 规则引擎会使用默认的数据处理方式

## 扩展性

新的数据类型可以通过以下方式添加：
1. 定义新的type值
2. 设计对应的fields结构
3. 在前端添加专用编辑器
4. 在后端添加数据处理逻辑