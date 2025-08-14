# 复合数据专用Action分析与改进方案

## 现状分析

### 后端已实现的专用Action

#### GPS/Location数据专用Action
- ✅ `geo_distance` - 地理距离计算
- ✅ `geo_bearing` - 地理方位角计算  
- ✅ `geo_geofence` - 地理围栏检查
- ✅ `extract_field` - 提取Location字段

#### 3D向量数据专用Action
- ✅ `vector_magnitude` - 向量模长计算
- ✅ `vector_normalize` - 向量归一化
- ✅ `vector_stats` - 向量统计计算
- ✅ `vector_transform` - 向量变换(旋转、缩放)

#### 颜色数据专用Action
- ✅ `color_convert` - 颜色空间转换
- ✅ `extract_field` - 提取颜色字段

#### 数组/矩阵数据专用Action
- ✅ `array_aggregate` - 数组聚合操作
- ✅ `matrix_operation` - 矩阵操作
- ✅ `composite_to_array` - 复合数据转数组

#### 时间序列数据专用Action
- ✅ `timeseries_analysis` - 时间序列分析

### 前端ComplexActionForm现状

#### 优点
- ✅ 已按数据类型分类显示可用Action
- ✅ 有基础的Action类型定义
- ✅ GPS编辑器已集成

#### 问题
- ❌ Action子类型定义与后端实现不完全匹配
- ❌ 缺少专门的配置界面
- ❌ 未充分体现每种数据类型的特殊性

## 改进方案

### 1. 完善Action类型映射

#### GPS/Location Action映射
```typescript
// 前端定义
'geo_transform' -> {
  'distance': 'geo_distance',      // ✅ 已匹配
  'bearing': 'geo_bearing',        // ✅ 已匹配  
  'geofence': 'geo_geofence',      // ❌ 需添加
  'coordinate_convert': 'geo_coordinate_convert' // ❌ 需实现
}
```

#### 向量Action映射
```typescript
'vector_transform' -> {
  'magnitude': 'vector_magnitude',     // ✅ 已匹配
  'normalize': 'vector_normalize',     // ✅ 已匹配
  'projection': 'vector_projection',   // ❌ 需实现
  'rotation': 'vector_rotation',       // ❌ 需实现
  'cross_product': 'vector_cross',     // ❌ 需实现
  'dot_product': 'vector_dot'          // ❌ 需实现
}
```

### 2. 后端需要补充的Action

#### GPS/Location补充Action
```go
// geo_geofence - 地理围栏检查
func (h *TransformHandler) geoGeofenceTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)

// geo_coordinate_convert - 坐标系转换  
func (h *TransformHandler) geoCoordinateConvertTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)
```

#### 向量数据补充Action
```go
// vector_projection - 向量投影
func (h *TransformHandler) vectorProjectionTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)

// vector_cross - 向量叉积
func (h *TransformHandler) vectorCrossTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)

// vector_dot - 向量点积
func (h *TransformHandler) vectorDotTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)
```

#### 颜色数据补充Action
```go
// color_similarity - 颜色相似度计算
func (h *TransformHandler) colorSimilarityTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)

// color_extract_dominant - 主色调提取
func (h *TransformHandler) colorExtractDominantTransform(data model.CompositeData, params map[string]interface{}) (interface{}, error)
```

### 3. 前端专用编辑器扩展

#### 创建向量Action编辑器
```typescript
// VectorActionEditor.tsx
interface VectorActionConfig {
  sub_type: 'magnitude' | 'normalize' | 'projection' | 'rotation' | 'cross_product' | 'dot_product';
  reference_vector?: { x: number; y: number; z: number };
  rotation_axis?: 'x' | 'y' | 'z';
  rotation_angle?: number;
  output_key?: string;
}
```

#### 创建颜色Action编辑器  
```typescript
// ColorActionEditor.tsx
interface ColorActionConfig {
  sub_type: 'convert' | 'similarity' | 'extract_dominant' | 'brightness_adjust';
  target_color_space?: 'RGB' | 'HSL' | 'HSV' | 'CMYK';
  reference_color?: { r: number; g: number; b: number };
  output_key?: string;
}
```

#### 创建数组Action编辑器
```typescript
// ArrayActionEditor.tsx 
interface ArrayActionConfig {
  sub_type: 'aggregate' | 'transform' | 'filter' | 'sort' | 'slice';
  operation?: 'sum' | 'mean' | 'max' | 'min' | 'std' | 'median';
  filter_condition?: string;
  slice_start?: number;
  slice_end?: number;
  output_key?: string;
}
```

### 4. 条件处理专门化

#### GPS条件专门化
```typescript
// 空间条件
const spatialConditions = [
  { type: 'distance_from', name: '距离条件', params: ['reference_point', 'max_distance'] },
  { type: 'within_bounds', name: '边界条件', params: ['bounds'] },
  { type: 'speed_range', name: '速度条件', params: ['min_speed', 'max_speed'] },
  { type: 'elevation_range', name: '海拔条件', params: ['min_altitude', 'max_altitude'] }
];
```

#### 向量条件专门化
```typescript
// 向量条件
const vectorConditions = [
  { type: 'magnitude_range', name: '模长条件', params: ['min_magnitude', 'max_magnitude'] },
  { type: 'direction_similarity', name: '方向相似度', params: ['reference_vector', 'threshold'] },
  { type: 'axis_dominance', name: '轴向主导', params: ['dominant_axis'] }
];
```

#### 颜色条件专门化
```typescript
// 颜色条件
const colorConditions = [
  { type: 'color_similarity', name: '颜色相似度', params: ['reference_color', 'threshold'] },
  { type: 'brightness_range', name: '亮度范围', params: ['min_brightness', 'max_brightness'] },
  { type: 'hue_range', name: '色相范围', params: ['min_hue', 'max_hue'] }
];
```

## 实施计划

### Phase 1: 后端Action补充
1. 补充GPS地理围栏和坐标转换Action
2. 补充向量投影、旋转、叉积、点积Action  
3. 补充颜色相似度和主色调提取Action
4. 补充数组和矩阵专用Action

### Phase 2: 前端专用编辑器
1. 创建VectorActionEditor组件
2. 创建ColorActionEditor组件  
3. 创建ArrayActionEditor组件
4. 集成到ComplexActionForm中

### Phase 3: 条件处理专门化
1. 扩展ComplexConditionForm支持专用条件
2. 为每种数据类型添加专门的条件模式
3. 实现空间条件、向量条件、颜色条件编辑器

### Phase 4: 测试和优化
1. 创建各种复合数据的测试用例
2. 验证前后端Action匹配
3. 优化用户体验和性能

## 预期效果

### 用户体验提升
- 每种复合数据类型都有专门的操作界面
- 智能的字段提示和验证
- 可视化的参数配置

### 系统能力增强  
- 支持更多复合数据处理场景
- 更精确的数据分析能力
- 更灵活的规则配置

### 开发效率提升
- 标准化的Action实现模式  
- 可复用的编辑器组件
- 统一的测试和验证流程

这个改进方案将使IoT Gateway具备真正专业化的复合数据处理能力，让不同类型的复合数据都能得到最适合的处理方式。