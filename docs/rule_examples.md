# 规则示例集合

> 版本：v1.2 &nbsp;&nbsp; 作者：IoT Gateway Team &nbsp;&nbsp; 日期：2025-08-14

本文档提供了IoT Gateway规则引擎的丰富示例，涵盖基础规则、复合数据处理、性能监控和高级分析场景。

## 1. 基础规则示例

### 1.1 温度告警规则
```json
{
  "id": "temperature_alert",
  "name": "温度告警规则",
  "description": "当温度超过40度时发送报警",
  "enabled": true,
  "priority": 100,
  "conditions": {
    "type": "and",
    "and": [
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "temperature"
      },
      {
        "type": "simple", 
        "field": "value",
        "operator": "gt",
        "value": 40
      }
    ]
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{.DeviceID}}温度过高: {{.Value}}°C",
        "channels": ["console", "webhook"],
        "throttle": "5m"
      }
    }
  ]
}
```

### 1.2 湿度范围检查
```json
{
  "id": "humidity_range_check",
  "name": "湿度范围检查",
  "description": "监控湿度是否在正常范围内",
  "enabled": true,
  "priority": 80,
  "conditions": {
    "type": "and",
    "and": [
      {
        "type": "simple",
        "field": "key", 
        "operator": "eq",
        "value": "humidity"
      },
      {
        "type": "or",
        "or": [
          {
            "type": "simple",
            "field": "value",
            "operator": "lt",
            "value": 30
          },
          {
            "type": "simple",
            "field": "value",
            "operator": "gt", 
            "value": 80
          }
        ]
      }
    ]
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{.DeviceID}}湿度异常: {{.Value}}%"
      }
    },
    {
      "type": "transform",
      "config": {
        "add_tags": {
          "status": "abnormal",
          "check_time": "{{now}}"
        }
      }
    }
  ]
}
```

### 1.3 设备状态监控
```json
{
  "id": "device_status_monitor",
  "name": "设备状态监控",
  "description": "监控设备连接状态和数据传输",
  "enabled": true,
  "priority": 90,
  "conditions": {
    "type": "or",
    "or": [
      {
        "type": "simple",
        "field": "key",
        "operator": "eq", 
        "value": "status"
      },
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "connection"
      }
    ]
  },
  "actions": [
    {
      "type": "filter",
      "config": {
        "type": "deduplication",
        "window": "1m",
        "key_fields": ["device_id", "key"]
      }
    },
    {
      "type": "forward",
      "config": {
        "subject": "iot.status.{{.DeviceID}}"
      }
    }
  ]
}
```

## 2. 聚合分析规则

### 2.1 温度统计分析
```json
{
  "id": "temperature_statistics",
  "name": "温度统计分析",
  "description": "计算温度的统计指标",
  "enabled": true,
  "priority": 70,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "temperature"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "10m",
        "functions": ["avg", "min", "max", "stddev", "p90", "p95"],
        "group_by": ["device_id", "location"],
        "output_key": "temp_stats",
        "forward": true
      }
    }
  ]
}
```

### 2.2 压力趋势分析
```json
{
  "id": "pressure_trend_analysis",
  "name": "压力趋势分析", 
  "description": "分析压力变化趋势和异常",
  "enabled": true,
  "priority": 75,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "pressure"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "size": 20,
        "functions": ["avg", "change", "change_rate", "volatility"],
        "group_by": ["device_id"],
        "output_key": "pressure_trend"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "info",
        "message": "设备{{.DeviceID}}压力趋势: 变化率{{.change_rate}}%",
        "condition": "{{.change_rate}} > 10 OR {{.change_rate}} < -10"
      }
    }
  ]
}
```

### 2.3 数据质量监控
```json
{
  "id": "data_quality_monitor",
  "name": "数据质量监控",
  "description": "监控数据完整性和质量",
  "enabled": true,
  "priority": 60,
  "conditions": {
    "type": "simple",
    "field": "device_id",
    "operator": "startswith",
    "value": "sensor_"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "1h",
        "functions": ["count", "completeness", "null_rate", "outlier_count"],
        "group_by": ["device_id"],
        "outlier_threshold": 2.0,
        "output_key": "data_quality"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{.DeviceID}}数据质量告警: 完整性{{.completeness}}, 异常值{{.outlier_count}}个",
        "condition": "{{.completeness}} < 0.9 OR {{.outlier_count}} > 5"
      }
    }
  ]
}
```

## 3. 复合数据类型规则

### 3.1 GPS位置跟踪
```json
{
  "id": "gps_location_tracking",
  "name": "GPS位置跟踪",
  "description": "跟踪GPS位置变化和移动分析",
  "enabled": true,
  "priority": 85,
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
        "output_key": "lat_rounded"
      }
    },
    {
      "type": "transform",
      "config": {
        "field": "value.longitude", 
        "type": "round",
        "precision": 6,
        "output_key": "lng_rounded"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "info",
        "message": "设备{{.DeviceID}}位置: ({{.lat_rounded}}, {{.lng_rounded}}), 速度: {{.value.speed}}km/h",
        "condition": "{{.value.speed}} > 80"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "size": 10,
        "functions": ["avg", "max", "change"],
        "group_by": ["device_id"],
        "output_key": "location_stats"
      }
    }
  ]
}
```

### 3.2 3D加速度分析
```json
{
  "id": "accelerometer_analysis",
  "name": "3D加速度分析",
  "description": "分析三轴加速度数据和振动模式",
  "enabled": true,
  "priority": 80,
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
        "min": 0.1,
        "max": 100.0,
        "drop_on_match": false
      }
    },
    {
      "type": "transform",
      "config": {
        "field": "value.magnitude",
        "type": "round",
        "precision": 3,
        "output_key": "magnitude_rounded"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "5m",
        "functions": ["avg", "max", "stddev", "p95"],
        "group_by": ["device_id"],
        "output_key": "accel_stats"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{.DeviceID}}检测到强烈振动: {{.magnitude_rounded}}m/s²",
        "condition": "{{.magnitude_rounded}} > 20.0"
      }
    }
  ]
}
```

### 3.3 颜色光谱分析
```json
{
  "id": "color_spectrum_analysis",
  "name": "颜色光谱分析",
  "description": "分析RGB颜色传感器数据",
  "enabled": true,
  "priority": 70,
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
        "output_key": "hue_rounded"
      }
    },
    {
      "type": "transform",
      "config": {
        "field": "value.brightness",
        "type": "scale",
        "factor": 0.01,
        "output_key": "brightness_percent"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "size": 30,
        "functions": ["avg", "stddev", "dominant_value"],
        "group_by": ["device_id"],
        "output_key": "color_stats"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "info",
        "message": "设备{{.DeviceID}}颜色变化: 色相{{.hue_rounded}}°, 亮度{{.brightness_percent}}%",
        "condition": "{{.brightness_percent}} < 10"
      }
    }
  ]
}
```

## 4. 性能监控规则

### 4.1 响应时间监控
```json
{
  "id": "response_time_monitor",
  "name": "响应时间监控",
  "description": "监控系统响应时间性能",
  "enabled": true,
  "priority": 95,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "response_time"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "5m",
        "functions": ["avg", "p50", "p90", "p95", "p99", "max"],
        "group_by": ["service", "endpoint"],
        "output_key": "response_stats"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "critical",
        "message": "服务{{.service}}响应时间异常: P95={{.p95}}ms, P99={{.p99}}ms",
        "condition": "{{.p95}} > 1000 OR {{.p99}} > 5000",
        "throttle": "2m"
      }
    }
  ]
}
```

### 4.2 错误率监控
```json
{
  "id": "error_rate_monitor",
  "name": "错误率监控",
  "description": "监控系统错误率",
  "enabled": true,
  "priority": 100,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "status_code"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "field": "value",
        "type": "expression",
        "expression": "if value >= 400 then 1 else 0 end",
        "output_key": "is_error"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "1m",
        "functions": ["count", "sum"],
        "group_by": ["service"],
        "output_key": "error_stats"
      }
    },
    {
      "type": "transform",
      "config": {
        "field": "sum",
        "type": "expression",
        "expression": "sum / count * 100",
        "output_key": "error_rate_percent"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "critical",
        "message": "服务{{.service}}错误率过高: {{.error_rate_percent}}%",
        "condition": "{{.error_rate_percent}} > 5.0"
      }
    }
  ]
}
```

### 4.3 吞吐量监控
```json
{
  "id": "throughput_monitor",
  "name": "吞吐量监控",
  "description": "监控系统吞吐量性能",
  "enabled": true,
  "priority": 80,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "request_count"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "1m",
        "functions": ["sum", "count", "avg"],
        "group_by": ["service"],
        "output_key": "throughput_stats"
      }
    },
    {
      "type": "transform",
      "config": {
        "field": "sum",
        "type": "expression",
        "expression": "sum / 60",
        "output_key": "requests_per_second"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "服务{{.service}}吞吐量: {{.requests_per_second}} req/s",
        "condition": "{{.requests_per_second}} < 10 OR {{.requests_per_second}} > 1000"
      }
    }
  ]
}
```

## 5. 业务逻辑规则

### 5.1 库存管理规则
```json
{
  "id": "inventory_management",
  "name": "库存管理规则",
  "description": "监控库存水平和自动补货",
  "enabled": true,
  "priority": 90,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "inventory_level"
  },
  "actions": [
    {
      "type": "filter",
      "config": {
        "type": "range",
        "field": "value",
        "min": 0,
        "max": 10000
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "1h",
        "functions": ["avg", "min", "change_rate"],
        "group_by": ["warehouse", "product_id"],
        "output_key": "inventory_stats"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "仓库{{.warehouse}}产品{{.product_id}}库存不足: 当前{{.value}}件",
        "condition": "{{.value}} < 100",
        "throttle": "30m"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "critical",
        "message": "仓库{{.warehouse}}产品{{.product_id}}库存告急: 仅剩{{.value}}件",
        "condition": "{{.value}} < 20"
      }
    }
  ]
}
```

### 5.2 能耗优化规则
```json
{
  "id": "energy_optimization",
  "name": "能耗优化规则",
  "description": "监控设备能耗并优化使用",
  "enabled": true,
  "priority": 75,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "power_consumption"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "15m",
        "functions": ["avg", "max", "sum", "p90"],
        "group_by": ["building", "floor", "device_type"],
        "output_key": "energy_stats"
      }
    },
    {
      "type": "transform",
      "config": {
        "field": "sum",
        "type": "expression",
        "expression": "sum * 0.12",
        "output_key": "cost_estimate"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "info",
        "message": "{{.building}}-{{.floor}}{{.device_type}}设备功耗异常: 平均{{.avg}}W, 峰值{{.max}}W",
        "condition": "{{.avg}} > 500 OR {{.max}} > 1000"
      }
    }
  ]
}
```

### 5.3 安全监控规则
```json
{
  "id": "security_monitoring",
  "name": "安全监控规则", 
  "description": "监控安全事件和异常行为",
  "enabled": true,
  "priority": 100,
  "conditions": {
    "type": "or",
    "or": [
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "login_attempt"
      },
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "access_denied"
      },
      {
        "type": "simple",
        "field": "key",
        "operator": "eq",
        "value": "suspicious_activity"
      }
    ]
  },
  "actions": [
    {
      "type": "filter",
      "config": {
        "type": "deduplication",
        "window": "5m",
        "key_fields": ["device_id", "user_id", "key"]
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "10m",
        "functions": ["count", "above_count"],
        "group_by": ["user_id", "source_ip"],
        "upper_limit": 5,
        "output_key": "security_stats"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "critical",
        "message": "安全告警: 用户{{.user_id}}从{{.source_ip}}异常活动{{.count}}次",
        "condition": "{{.count}} > 10 OR {{.above_count}} > 3",
        "channels": ["console", "webhook", "email"]
      }
    }
  ]
}
```

## 6. 高级表达式规则

### 6.1 复杂条件表达式
```json
{
  "id": "complex_condition_rule",
  "name": "复杂条件表达式规则",
  "description": "使用表达式引擎的复杂条件判断",
  "enabled": true,
  "priority": 85,
  "conditions": {
    "type": "expression",
    "expression": "(temperature > 30 AND humidity < 40) OR (pressure < 950 AND contains(device_id, 'critical'))"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning", 
        "message": "设备{{.DeviceID}}环境异常: 温度{{.temperature}}°C, 湿度{{.humidity}}%, 压力{{.pressure}}hPa"
      }
    }
  ]
}
```

### 6.2 数学计算规则
```json
{
  "id": "mathematical_calculation",
  "name": "数学计算规则",
  "description": "使用数学表达式进行数据计算",
  "enabled": true,
  "priority": 70,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "regex",
    "value": "^(voltage|current)$"
  },
  "actions": [
    {
      "type": "transform",
      "config": {
        "field": "value",
        "type": "expression",
        "expression": "if key == 'voltage' then value * 1.1 else value * 0.9 end",
        "output_key": "calibrated_value"
      }
    },
    {
      "type": "transform",
      "config": {
        "type": "expression",
        "expression": "voltage * current",
        "output_key": "power",
        "require_fields": ["voltage", "current"]
      }
    }
  ]
}
```

## 7. 时间序列规则

### 7.1 时间窗口聚合
```json
{
  "id": "time_series_aggregation",
  "name": "时间序列聚合",
  "description": "按不同时间窗口进行数据聚合",
  "enabled": true,
  "priority": 60,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "cpu_usage"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "1m",
        "functions": ["avg", "max"],
        "group_by": ["host"],
        "output_key": "cpu_1m"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time", 
        "window": "5m",
        "functions": ["avg", "p95"],
        "group_by": ["host"],
        "output_key": "cpu_5m"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_type": "time",
        "window": "15m",
        "functions": ["avg", "change_rate"],
        "group_by": ["host"],
        "output_key": "cpu_15m"
      }
    }
  ]
}
```

### 7.2 趋势检测规则
```json
{
  "id": "trend_detection",
  "name": "趋势检测规则",
  "description": "检测数据的长期趋势变化",
  "enabled": true,
  "priority": 65,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "memory_usage"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "size": 60,
        "functions": ["change", "change_rate", "volatility"],
        "group_by": ["host"],
        "output_key": "memory_trend"
      }
    },
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "主机{{.host}}内存使用趋势异常: 变化率{{.change_rate}}%, 波动性{{.volatility}}",
        "condition": "abs({{.change_rate}}) > 20 OR {{.volatility}} > 10"
      }
    }
  ]
}
```

## 8. 规则最佳实践

### 8.1 性能优化技巧
1. **条件排序**: 将高选择性条件放在前面
2. **聚合窗口**: 根据数据频率合理设置窗口大小
3. **分组策略**: 避免过度细分的分组
4. **函数选择**: 优先使用增量算法的函数

### 8.2 规则管理建议
1. **命名规范**: 使用描述性的规则ID和名称
2. **优先级设置**: 重要规则设置高优先级
3. **规则分组**: 按业务模块组织规则
4. **版本控制**: 维护规则的版本历史

### 8.3 监控和调试
1. **规则执行统计**: 监控规则执行次数和耗时
2. **告警频率控制**: 使用throttle避免告警风暴
3. **数据质量检查**: 定期检查规则输出的数据质量
4. **性能分析**: 识别高耗时的规则并优化

---

这些规则示例展示了IoT Gateway规则引擎的强大能力，可以根据具体业务需求进行调整和扩展。通过合理组合条件、动作和聚合函数，能够实现复杂的物联网数据处理和分析需求。