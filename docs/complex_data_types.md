# 复合数据类型支持

> 版本：v1.2 &nbsp;&nbsp; 作者：IoT Gateway Team &nbsp;&nbsp; 日期：2025-08-14

## 1. 概述

IoT Gateway 支持多种复合数据类型的处理，包括GPS位置、3D向量、颜色数据、通用向量和数组等。这些复合数据类型可以被规则引擎高效处理，支持复杂的聚合计算和数据分析。

### 支持的复合数据类型

| 数据类型 | 标识符 | 描述 | 示例场景 |
|---------|-------|------|---------|
| GPS位置 | `location` | 包含经纬度、海拔、速度等信息 | 车辆跟踪、物流监控 |
| 3D向量 | `vector3d` | 三维坐标数据 | 加速度计、陀螺仪 |
| 颜色数据 | `color` | RGB/HSV颜色信息 | 光谱分析、色彩检测 |
| 通用向量 | `vector` | 多维数值向量 | 信号分析、传感器阵列 |
| 数组数据 | `array` | 同构数据数组 | 批量测量、时间序列 |

## 2. GPS位置数据 (Location)

### 数据结构
```json
{
  "device_id": "gps_tracker_01",
  "key": "location", 
  "value": {
    "latitude": 39.904200,
    "longitude": 116.407400,
    "altitude": 45.0,
    "speed": 25.5,
    "heading": 120.0,
    "accuracy": 3.0,
    "timestamp": "2024-08-14T10:30:00Z"
  },
  "type": "location",
  "timestamp": 1692012600000000000
}
```

### 字段说明
- `latitude`: 纬度 (-90 到 90)
- `longitude`: 经度 (-180 到 180)
- `altitude`: 海拔高度 (米)
- `speed`: 速度 (km/h)
- `heading`: 方向角 (0-360度)
- `accuracy`: 精度 (米)

### 规则示例
```json
{
  "id": "location_analysis_rule",
  "name": "GPS位置数据分析",
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "location"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "field": "value.latitude",
        "type": "round",
        "precision": 6,
        "output_key": "rounded_lat"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "size": 10,
        "functions": ["avg", "max", "min"],
        "group_by": ["device_id"],
        "output_key": "location_stats"
      }
    }
  ]
}
```

### 支持的聚合函数
- **基础统计**: avg, max, min, count, stddev
- **地理计算**: distance (两点间距离), bearing (方位角)
- **运动分析**: speed_avg, acceleration, trajectory_length

## 3. 3D向量数据 (Vector3D)

### 数据结构
```json
{
  "device_id": "accelerometer_01",
  "key": "acceleration",
  "value": {
    "x": 1.23,
    "y": -0.45,
    "z": 9.81,
    "magnitude": 9.95,
    "unit": "m/s²"
  },
  "type": "vector3d",
  "timestamp": 1692012600000000000
}
```

### 字段说明
- `x`, `y`, `z`: 三轴分量
- `magnitude`: 向量幅值 (√(x² + y² + z²))
- `unit`: 单位信息

### 规则示例
```json
{
  "id": "vector3d_magnitude_analysis",
  "name": "3D向量幅值分析",
  "conditions": {
    "type": "simple",
    "field": "key", 
    "operator": "eq",
    "value": "acceleration"
  },
  "actions": [
    {
      "type": "filter",
      "config": {
        "type": "range",
        "field": "value.magnitude",
        "min": 0.5,
        "max": 50.0
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "1m",
        "functions": ["avg", "max", "p95"],
        "group_by": ["device_id"]
      }
    }
  ]
}
```

### 支持的向量运算
- **幅值计算**: magnitude, norm
- **统计分析**: component_avg, component_max, component_variance
- **振动分析**: rms, peak_to_peak, frequency_domain

## 4. 颜色数据 (Color)

### 数据结构
```json
{
  "device_id": "rgb_sensor_01",
  "key": "color_reading",
  "value": {
    "rgb": {
      "r": 255,
      "g": 128, 
      "b": 64
    },
    "hsv": {
      "hue": 30.0,
      "saturation": 75.0,
      "value": 100.0
    },
    "brightness": 149,
    "dominant_color": "orange"
  },
  "type": "color",
  "timestamp": 1692012600000000000
}
```

### 字段说明
- `rgb`: RGB颜色值 (0-255)
- `hsv`: HSV颜色空间
- `brightness`: 亮度值
- `dominant_color`: 主要颜色名称

### 规则示例
```json
{
  "id": "color_analysis_rule",
  "name": "颜色数据分析",
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq", 
    "value": "color_reading"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "field": "value.hsv.hue",
        "type": "round",
        "precision": 1,
        "output_key": "rounded_hue"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "size": 20,
        "functions": ["avg", "stddev", "dominant_value"],
        "group_by": ["device_id"]
      }
    }
  ]
}
```

## 5. 通用向量数据 (Vector)

### 数据结构
```json
{
  "device_id": "sensor_array_01",
  "key": "signal_vector",
  "value": {
    "values": [1.2, 3.4, 5.6, 7.8, 9.0],
    "labels": ["ch1", "ch2", "ch3", "ch4", "ch5"],
    "dimension": 5,
    "unit": "voltage",
    "metadata": {
      "sampling_rate": 1000,
      "calibration": "2024-08-01"
    }
  },
  "type": "vector",
  "timestamp": 1692012600000000000
}
```

### 字段说明
- `values`: 数值向量
- `labels`: 通道标签
- `dimension`: 向量维度
- `unit`: 测量单位
- `metadata`: 元数据信息

### 向量运算支持
```json
{
  "type": "aggregate",
  "config": {
    "window_type": "count",
    "size": 10,
    "functions": [
      "vector_avg",      // 向量平均值
      "vector_max",      // 各分量最大值
      "vector_norm",     // 向量范数
      "correlation",     // 分量间相关性
      "principal_component" // 主成分分析
    ]
  }
}
```

## 6. 数组数据 (Array)

### 数据结构
```json
{
  "device_id": "temp_array_01",
  "key": "temperature_array",
  "value": {
    "elements": [20.1, 21.3, 19.8, 22.5, 20.7, 21.1, 20.4, 22.0],
    "size": 8,
    "element_type": "float",
    "labels": ["zone1", "zone2", "zone3", "zone4", "zone5", "zone6", "zone7", "zone8"],
    "unit": "celsius",
    "statistics": {
      "min": 19.8,
      "max": 22.5,
      "avg": 21.0
    }
  },
  "type": "array",
  "timestamp": 1692012600000000000
}
```

### 数组操作支持
- **统计计算**: array_avg, array_max, array_min, array_stddev
- **分布分析**: percentiles (p25, p50, p75, p90, p95, p99)
- **异常检测**: outlier_count, anomaly_score
- **趋势分析**: trend_slope, seasonality

## 7. 复合数据类型的配置

### 南向适配器配置 (Mock示例)
```yaml
southbound:
  adapters:
    - name: "composite_mock_sensors"
      type: "composite_mock"
      config:
        composite_data_types:
          # GPS配置
          - device_id: "gps_tracker_01"
            key: "location"
            data_type: "location"
            location_config:
              start_latitude: 39.9042
              start_longitude: 116.4074
              simulate_movement: true
              
          # 3D向量配置  
          - device_id: "accelerometer_01"
            key: "acceleration"
            data_type: "vector3d"
            vector3d_config:
              x_range: [-10.0, 10.0]
              y_range: [-10.0, 10.0]
              z_range: [-10.0, 10.0]
              
          # 颜色数据配置
          - device_id: "rgb_sensor_01"
            key: "color_reading"
            data_type: "color"
            color_config:
              color_mode: "rainbow"
              brightness_range: [50, 255]
```

### 规则引擎配置
```yaml
rule_engine:
  enabled: true
  rules:
    - id: "composite_data_monitor"
      name: "复合数据综合监控"
      conditions:
        or:
          - field: "key"
            operator: "eq"
            value: "location"
          - field: "key"
            operator: "eq"
            value: "acceleration"
          - field: "key"
            operator: "eq"
            value: "color_reading"
      actions:
        - type: "aggregate"
          config:
            window_type: "time"
            window: "5m"
            functions: ["count", "avg", "p95"]
            group_by: ["device_id", "key"]
```

## 8. 性能优化

### 内存管理
- **滑动窗口**: 对于大数组数据，使用滑动窗口减少内存占用
- **数据压缩**: 对重复或相似数据进行压缩存储
- **批处理**: 批量处理复合数据，提高吞吐量

### 计算优化
- **增量计算**: 对聚合函数使用增量算法
- **并行处理**: 向量运算并行化处理
- **缓存机制**: 缓存中间计算结果

### 配置建议
```yaml
# 针对复合数据的优化配置
rule_engine:
  worker_pool:
    max_workers: 20           # 增加worker数量
    queue_size: 10000        # 增大队列容量
    batch_size: 50           # 增大批处理大小
  
  aggregate_manager:
    max_states: 15000        # 增大状态数量限制
    max_memory: "200MB"      # 增大内存限制
    cleanup_interval: "30s"  # 缩短清理间隔
```

## 9. 监控和调试

### 关键指标
- 复合数据处理延迟
- 聚合状态内存使用
- 数据丢弃率
- 计算错误率

### 调试工具
```bash
# 查看复合数据统计
curl http://localhost:8081/api/stats/complex-data

# 监控聚合状态
curl http://localhost:8081/api/rules/aggregate-states

# 性能分析
curl http://localhost:8081/api/stats/performance
```

## 10. 最佳实践

### 数据设计
1. **字段命名**: 使用清晰的字段命名约定
2. **单位统一**: 确保同类型数据使用相同单位
3. **精度控制**: 根据实际需求设置合适的数据精度

### 规则设计
1. **分层处理**: 将复杂规则拆分为多个简单规则
2. **条件优化**: 将高选择性条件放在前面
3. **聚合窗口**: 根据数据频率合理设置聚合窗口

### 性能优化
1. **批量处理**: 尽可能使用批量操作
2. **内存监控**: 定期监控内存使用情况
3. **负载均衡**: 合理分配计算负载

---

通过以上复合数据类型的支持，IoT Gateway 能够处理现代物联网场景中的各种复杂数据，提供强大的数据分析和处理能力。