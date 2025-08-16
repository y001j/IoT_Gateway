# 聚合函数详解

> 版本：v1.2 &nbsp;&nbsp; 作者：IoT Gateway Team &nbsp;&nbsp; 日期：2025-08-14

## 1. 概述

IoT Gateway 规则引擎提供了28种强大的聚合函数，支持实时数据分析、统计计算和异常检测。这些函数分为五大类：基础统计、百分位数、数据质量、变化检测和阈值监控。

### 聚合函数总览

| 类别 | 函数数量 | 主要用途 |
|------|---------|---------|
| 基础统计 | 13个 | 基本统计分析 |
| 百分位数 | 6个 | 分布分析和异常检测 |
| 数据质量 | 3个 | 数据完整性评估 |
| 变化检测 | 4个 | 趋势和波动分析 |
| 阈值监控 | 3个 | 范围和限值监控 |

## 2. 基础统计函数 (13个)

### 2.1 基本计数和求和

#### `count` - 计数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["count"],
    "window_type": "count",
    "size": 100
  }
}
```
- **用途**: 统计数据点数量
- **返回**: 整数，窗口内数据点总数
- **场景**: 数据流量监控、设备活跃度统计

#### `sum` - 求和
```json
{
  "type": "aggregate", 
  "config": {
    "functions": ["sum"],
    "window_type": "time",
    "window": "1h"
  }
}
```
- **用途**: 累计数值总和
- **返回**: 浮点数，窗口内所有数值之和
- **场景**: 能耗统计、流量累计、产量汇总

### 2.2 平均值计算

#### `avg` / `mean` / `average` - 平均值
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["avg"],
    "window_type": "count", 
    "size": 50
  }
}
```
- **用途**: 计算平均值
- **返回**: 浮点数，算术平均值
- **场景**: 温度平均值、响应时间均值

#### `median` - 中位数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["median"],
    "window_type": "time",
    "window": "5m"
  }
}
```
- **用途**: 计算中位数（50%分位数）
- **返回**: 浮点数，排序后的中间值
- **场景**: 抗异常值的中心趋势分析

### 2.3 极值函数

#### `min` - 最小值
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["min"],
    "window_type": "count",
    "size": 20
  }
}
```
- **用途**: 找出最小值
- **返回**: 浮点数，窗口内最小值
- **场景**: 最低温度、最小压力监控

#### `max` - 最大值
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["max"],
    "window_type": "time", 
    "window": "10m"
  }
}
```
- **用途**: 找出最大值
- **返回**: 浮点数，窗口内最大值
- **场景**: 峰值监控、最高温度告警

### 2.4 离散度分析

#### `stddev` / `std` - 标准差
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["stddev"],
    "window_type": "count",
    "size": 100
  }
}
```
- **用途**: 计算标准差，衡量数据散布程度
- **返回**: 浮点数，标准差值
- **场景**: 稳定性分析、质量控制

#### `variance` - 方差
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["variance"],
    "window_type": "time",
    "window": "1h"
  }
}
```
- **用途**: 计算方差，标准差的平方
- **返回**: 浮点数，方差值
- **场景**: 波动性分析、风险评估

### 2.5 时序函数

#### `first` - 首值
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["first"],
    "window_type": "time",
    "window": "1h"
  }
}
```
- **用途**: 获取窗口内第一个数据点的值
- **返回**: 原始数据类型，第一个值
- **场景**: 初始状态记录、基准值设定

#### `last` - 末值
```json
{
  "type": "aggregate", 
  "config": {
    "functions": ["last"],
    "window_type": "count",
    "size": 10
  }
}
```
- **用途**: 获取窗口内最后一个数据点的值
- **返回**: 原始数据类型，最后一个值
- **场景**: 当前状态获取、最新读数

## 3. 百分位数函数 (6个)

百分位数函数用于分析数据分布特征，特别适用于性能监控和异常检测。

### 3.1 四分位数

#### `p25` - 第25百分位数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p25"],
    "window_type": "count",
    "size": 100
  }
}
```
- **用途**: 下四分位数，25%的数据小于此值
- **场景**: 性能基准线、低端分析

#### `p50` - 第50百分位数 (中位数)
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p50"],
    "window_type": "time",
    "window": "15m"
  }
}
```
- **用途**: 中位数，50%的数据小于此值
- **场景**: 典型值分析、抗异常值统计

#### `p75` - 第75百分位数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p75"],
    "window_type": "count",
    "size": 200
  }
}
```
- **用途**: 上四分位数，75%的数据小于此值
- **场景**: 高端性能分析

### 3.2 高百分位数

#### `p90` - 第90百分位数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p90"],
    "window_type": "time",
    "window": "30m"
  }
}
```
- **用途**: 90%的数据小于此值
- **场景**: 性能优化目标、SLA监控

#### `p95` - 第95百分位数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p95"],
    "window_type": "count",
    "size": 1000
  }
}
```
- **用途**: 95%的数据小于此值
- **场景**: 高级性能监控、响应时间SLA

#### `p99` - 第99百分位数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p99"],
    "window_type": "time",
    "window": "1h"
  }
}
```
- **用途**: 99%的数据小于此值
- **场景**: 极端值监控、尾延迟分析

### 百分位数组合分析
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["p50", "p90", "p95", "p99"],
    "window_type": "time",
    "window": "10m",
    "group_by": ["device_type"]
  }
}
```

## 4. 数据质量函数 (3个)

### 4.1 `null_rate` - 空值率
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["null_rate"],
    "window_type": "count",
    "size": 100
  }
}
```
- **用途**: 计算空值或无效值的比例
- **返回**: 0.0-1.0之间的浮点数，表示空值比例
- **场景**: 数据质量监控、传感器故障检测
- **示例**: 0.05表示5%的数据为空值

### 4.2 `completeness` - 完整性
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["completeness"],
    "window_type": "time", 
    "window": "1h"
  }
}
```
- **用途**: 计算数据完整性，等于1 - null_rate
- **返回**: 0.0-1.0之间的浮点数，表示数据完整度
- **场景**: 数据质量评估、SLA监控
- **示例**: 0.95表示95%的数据完整

### 4.3 `outlier_count` - 异常值计数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["outlier_count"],
    "window_type": "count",
    "size": 50,
    "outlier_threshold": 2.0
  }
}
```
- **用途**: 统计异常值数量（基于标准差阈值）
- **返回**: 整数，异常值数量
- **场景**: 异常检测、质量控制
- **参数**: `outlier_threshold` - 标准差倍数阈值（默认2.0）

## 5. 变化检测函数 (4个)

### 5.1 `change` - 变化量
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["change"],
    "window_type": "count",
    "size": 10
  }
}
```
- **用途**: 计算窗口内最后值与第一值的差
- **返回**: 浮点数，变化量（可为负）
- **场景**: 趋势分析、增量监控

### 5.2 `change_rate` - 变化率
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["change_rate"],
    "window_type": "time",
    "window": "5m"
  }
}
```
- **用途**: 计算相对变化率 = (末值-首值)/首值
- **返回**: 浮点数，变化率（可为负）
- **场景**: 百分比变化监控、增长率分析
- **示例**: 0.1表示增长10%，-0.05表示下降5%

### 5.3 `volatility` - 波动性
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["volatility"],
    "window_type": "count",
    "size": 20
  }
}
```
- **用途**: 计算数据波动性（相邻值变化的标准差）
- **返回**: 浮点数，波动性指标
- **场景**: 稳定性监控、市场分析

### 5.4 `cv` - 变异系数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["cv"],
    "window_type": "time",
    "window": "10m"
  }
}
```
- **用途**: 计算变异系数 = 标准差/平均值
- **返回**: 浮点数，相对变异程度
- **场景**: 相对稳定性比较、标准化波动分析

## 6. 阈值监控函数 (3个)

### 6.1 `above_count` - 超上限计数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["above_count"],
    "window_type": "count",
    "size": 100,
    "upper_limit": 50.0
  }
}
```
- **用途**: 统计超过上限阈值的数据点数量
- **返回**: 整数，超限次数
- **场景**: 告警监控、阈值违规统计
- **参数**: `upper_limit` - 上限阈值

### 6.2 `below_count` - 低于下限计数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["below_count"],
    "window_type": "time",
    "window": "30m",
    "lower_limit": 10.0
  }
}
```
- **用途**: 统计低于下限阈值的数据点数量
- **返回**: 整数，低于下限次数
- **场景**: 最低值监控、不足告警
- **参数**: `lower_limit` - 下限阈值

### 6.3 `in_range_count` - 范围内计数
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["in_range_count"],
    "window_type": "count",
    "size": 50,
    "upper_limit": 80.0,
    "lower_limit": 20.0
  }
}
```
- **用途**: 统计在指定范围内的数据点数量
- **返回**: 整数，范围内次数
- **场景**: 正常范围监控、合格率统计
- **参数**: `upper_limit`, `lower_limit` - 上下限阈值

## 7. 聚合窗口配置

### 7.1 计数窗口
```json
{
  "window_type": "count",
  "size": 100
}
```
- **特点**: 固定数据点数量的滑动窗口
- **适用**: 数据频率稳定的场景
- **内存**: 需要存储完整的数据历史

### 7.2 时间窗口
```json
{
  "window_type": "time",
  "window": "5m"
}
```
- **特点**: 基于时间的滑动窗口
- **适用**: 数据频率不规律的场景
- **内存**: 根据数据频率动态调整

### 7.3 累积模式
```json
{
  "window_type": "cumulative"
}
```
- **特点**: 从启动开始累积所有数据
- **适用**: 长期趋势分析
- **内存**: 仅存储聚合结果，内存效率高

## 8. 性能特性

### 8.1 算法复杂度

| 函数类别 | 时间复杂度 | 空间复杂度 | 备注 |
|---------|------------|------------|------|
| 基础统计 | O(1) | O(1) | 增量算法 |
| 百分位数 | O(n log n) | O(n) | 需要排序 |
| 数据质量 | O(1) | O(1) | 计数器模式 |
| 变化检测 | O(1) | O(1) | 首末值比较 |
| 阈值监控 | O(1) | O(1) | 计数器模式 |

### 8.2 内存优化

#### 增量计算示例
```go
// 平均值增量计算
type IncrementalMean struct {
    count int64
    sum   float64
}

func (im *IncrementalMean) Add(value float64) {
    im.count++
    im.sum += value
}

func (im *IncrementalMean) Mean() float64 {
    if im.count == 0 {
        return 0
    }
    return im.sum / float64(im.count)
}
```

## 9. 组合使用示例

### 9.1 性能监控组合
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["avg", "p50", "p95", "p99", "max"],
    "window_type": "time",
    "window": "5m",
    "group_by": ["service", "endpoint"]
  }
}
```

### 9.2 质量监控组合
```json
{
  "type": "aggregate", 
  "config": {
    "functions": ["completeness", "outlier_count", "stddev"],
    "window_type": "count",
    "size": 100,
    "outlier_threshold": 2.5
  }
}
```

### 9.3 阈值监控组合
```json
{
  "type": "aggregate",
  "config": {
    "functions": ["above_count", "below_count", "in_range_count"],
    "window_type": "time", 
    "window": "1h",
    "upper_limit": 80.0,
    "lower_limit": 20.0
  }
}
```

## 10. 最佳实践

### 10.1 函数选择指南
- **基础监控**: avg, min, max, count
- **性能分析**: p50, p95, p99
- **质量评估**: completeness, outlier_count
- **趋势分析**: change, change_rate, volatility
- **告警监控**: above_count, below_count

### 10.2 窗口大小建议
- **实时监控**: 10-50个数据点或1-5分钟
- **短期分析**: 100-500个数据点或5-30分钟  
- **长期趋势**: 1000+个数据点或1小时+

### 10.3 性能优化
- 优先使用增量算法的函数（基础统计类）
- 避免在高频数据上使用百分位数函数
- 合理设置聚合窗口大小
- 使用分组聚合减少计算量

---

通过合理使用这28种聚合函数，IoT Gateway能够满足各种数据分析需求，为物联网应用提供强大的实时分析能力。