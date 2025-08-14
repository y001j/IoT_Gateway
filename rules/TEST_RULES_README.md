# IoT Gateway 测试规则说明

本文档介绍了为测试规则引擎功能而创建的各种测试规则。

## 📋 规则分类

### 1. Filter 规则 (数据过滤)

#### `temperature_filter_test.json`
- **功能**: 温度数据范围过滤、去重、速率限制
- **条件**: 温度传感器数据 (key='temperature', device_id 以 'temp_sensor' 开头)
- **过滤器**:
  - 范围检查: -40°C 到 80°C
  - 去重: 30秒窗口内，值变化小于0.1的数据
  - 速率限制: 60秒内最多10条数据
- **添加标签**: `filter_applied`, `quality_check`

#### `vibration_anomaly_filter.json`
- **功能**: 振动数据异常检测
- **条件**: 振动传感器数据 (device_id='vibration_sensor_01', key='vibration')
- **过滤器**:
  - Z-score异常检测 (阈值: 2.5)
  - 窗口大小: 20个样本
  - 突变检测: 50%变化阈值
- **添加标签**: `anomaly_detector`, `analysis_window`

#### `network_quality_filter.json`
- **功能**: 网络质量评估和过滤
- **条件**: 网络延迟数据 (key='network_latency')
- **过滤器**:
  - 质量等级: excellent(<10ms), good(<30ms), fair(<60ms), poor(<100ms)
  - 拒绝poor质量数据
  - 采样: 高质量10%保留，低质量80%保留
- **添加标签**: `quality_filter`, `filter_version`

### 2. Aggregate 规则 (数据聚合)

#### `temperature_statistics_aggregate.json`
- **功能**: 温度统计聚合分析
- **条件**: 温度传感器数据
- **聚合配置**:
  - 滑动窗口: 30个样本，10个样本间隔
  - 统计函数: count, sum, avg, min, max, stddev, variance, median, 分位数(p25,p75,p90,p95,p99), 变化率, 波动性
  - 质量指标: null_rate, completeness, outlier_count
- **添加标签**: `aggregation_type`, `window_type`, `analysis_level`

#### `vibration_performance_monitor.json`
- **功能**: 振动设备健康监控
- **条件**: 振动传感器的振动和转速数据
- **聚合配置**:
  - 累积窗口: 5分钟重置
  - 健康评分: 振动稳定性(40%) + 速度一致性(30%) + 运行时间(30%)
  - 阈值监控: 振动正常(<5), 警告(<8), 临界(<10)
- **添加标签**: `monitor_type`, `analysis_period`, `health_model`

#### `network_latency_trend_analysis.json`
- **功能**: 网络延迟趋势分析和预测
- **条件**: 网络延迟数据
- **聚合配置**:
  - 滑动窗口: 60个样本，15个样本间隔
  - 趋势分析: 检测增长、下降、稳定、震荡模式
  - 预测告警: 20%性能下降阈值
- **添加标签**: `analysis_type`, `prediction_enabled`, `trend_window`

### 3. Transform 规则 (数据转换)

#### `temperature_unit_conversion.json`
- **功能**: 温度单位转换 (摄氏度→华氏度)
- **条件**: 温度传感器数据
- **转换配置**:
  - 单位转换: C → F
  - 精度: 2位小数
  - 条件标签: 根据温度范围添加等级标签 (freezing, cold, cool, warm, hot)
- **添加标签**: `transform_type`, `original_unit`, `converted_unit`

#### `vibration_energy_calculation.json`
- **功能**: 振动能量计算
- **条件**: 振动传感器振动数据
- **转换配置**:
  - 表达式计算: `pow(x, 2) * 0.5` (动能公式)
  - 缩放: 放大1000倍
  - 归一化: 0-100范围
- **添加标签**: `calculation_type`, `algorithm`, `units`

#### `network_performance_index.json`
- **功能**: 网络性能指数计算
- **条件**: 网络延迟数据
- **转换配置**:
  - 性能评分: `max(0, 100 - (延迟/2))`
  - 等级映射: A(90-100), B(80-90), C(70-80), D(60-70), F(0-60)
  - 条件标签: 根据分数添加性能等级和SLA状态
- **添加标签**: `transform_type`, `algorithm`, `score_range`

#### `gps_distance_calculation.json`
- **功能**: GPS坐标处理和地理分析
- **条件**: GPS经纬度数据
- **转换配置**:
  - 坐标格式化: 6位小数精度
  - 地理围栏: 北京区域检测
  - 运动分析: 速度计算、方向跟踪、距离累积
- **添加标签**: `coordinate_format`, `precision`, `geo_processing`

### 4. 综合规则 (多动作组合)

#### `comprehensive_sensor_pipeline.json`
- **功能**: 完整数据处理管道
- **条件**: 所有传感器数据 (正则匹配设备ID)
- **动作链**:
  1. Filter: 数据质量检查
  2. Transform: 归一化处理
  3. Aggregate: 滑动窗口统计
  4. Alert: 阈值告警
- **特点**: 展示完整的数据处理流水线

#### `advanced_condition_testing.json`
- **功能**: 复杂条件逻辑测试
- **条件**: 3层嵌套逻辑 (AND → OR → AND/NOT + Expression)
- **动作**:
  - Transform: 条件化表达式转换
  - Forward: 多主题路由转发
- **特点**: 测试规则引擎的条件解析能力

## 🧪 测试建议

### 测试准备
1. 确保所有规则文件都在 `rules/` 目录下
2. 启动IoT Gateway系统
3. 确认MockAdapter正在生成测试数据

### 测试方法
1. **观察日志**: 查看规则执行和动作处理日志
2. **NATS监控**: 订阅相关主题查看处理后的数据
3. **性能测试**: 监控系统资源使用和处理延迟
4. **功能验证**: 验证每种动作类型的预期行为

### 测试命令
```bash
# 启动系统
go run cmd/gateway/main.go -config config_rule_engine_test.yaml

# 监控NATS消息
nats sub "iot.data.>"
nats sub "iot.rules.>"
nats sub "transformed.>"
nats sub "processed.>"

# 查看规则状态
curl http://localhost:8081/api/rules
```

### 预期结果
- **Filter规则**: 数据被正确过滤，添加质量标签
- **Aggregate规则**: 生成统计摘要和趋势分析
- **Transform规则**: 数据被转换，添加计算结果
- **综合规则**: 多个动作按序执行，数据经过完整处理链

## 📊 性能基准

- **规则数量**: 13个测试规则
- **条件复杂度**: 简单到3层嵌套
- **动作类型**: Filter, Aggregate, Transform, Alert, Forward
- **处理延迟**: 目标 < 10ms per rule
- **内存使用**: 监控聚合窗口内存占用

## 🔍 调试提示

1. **规则未触发**: 检查条件逻辑和数据匹配
2. **动作失败**: 查看错误日志和配置参数
3. **性能问题**: 调整窗口大小和聚合函数
4. **标签丢失**: 确认Transform Handler和SafeValueForJSON正常工作