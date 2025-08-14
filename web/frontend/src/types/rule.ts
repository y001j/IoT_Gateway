export interface Rule {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  priority: number;
  version: number;
  data_type?: string | any; // 数据类型：字符串或详细定义
  conditions: Condition;
  actions: Action[];
  tags?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface Condition {
  type?: string; // "simple", "expression", "lua"
  field?: string;
  operator?: string;
  value?: string | number | boolean | null;
  expression?: string;
  script?: string;
  and?: Condition[];
  or?: Condition[];
  not?: Condition;
}

export interface Action {
  type: string;
  config: Record<string, any>;
  async?: boolean;
  timeout?: string;
  retry?: number;
}

export interface RuleListRequest {
  page: number;
  page_size: number;
  search?: string;
  enabled?: boolean;
  priority?: number;
}

export interface RuleListResponse {
  data: Rule[];
  pagination: {
    total: number;
    offset: number;
    limit: number;
  };
}

export interface RuleCreateRequest {
  name: string;
  description: string;
  enabled: boolean;
  priority: number;
  conditions: Condition;
  actions: Action[];
  tags?: Record<string, string>;
}

export interface RuleUpdateRequest extends RuleCreateRequest {
  version: number;
}

export interface RuleValidationResponse {
  valid: boolean;
  errors?: string[];
  warnings?: string[];
}

export interface RuleTestRequest {
  rule: Rule;
  test_data: Record<string, unknown>;
}

export interface RuleTestResponse {
  matched: boolean;
  result: Record<string, unknown>;
  execution_time: number;
  errors?: string[];
}

export type RuleOperator = 'eq' | 'ne' | 'gt' | 'gte' | 'lt' | 'lte' | 'contains' | 'starts_with' | 'ends_with' | 'regex';
export type ActionType = 'alert' | 'transform' | 'filter' | 'aggregate' | 'forward';
export type AlertLevel = 'info' | 'warning' | 'error' | 'critical';

// 数据类型定义
export type DataType = 
  // 基础数据类型
  | 'int' | 'float' | 'bool' | 'string' | 'binary'
  // 复合数据类型
  | 'location' | 'vector3d' | 'color' | 'vector' | 'array' | 'matrix' | 'timeseries';

// 复合数据结构定义
export interface LocationData {
  latitude: number;    // 纬度 (-90 ~ 90)
  longitude: number;   // 经度 (-180 ~ 180)
  altitude?: number;   // 海拔 (米，可选)
  accuracy?: number;   // GPS精度 (米，可选)
  speed?: number;      // 移动速度 (km/h，可选)
  heading?: number;    // 方向角 (度，可选)
}

export interface Vector3D {
  x: number;  // X轴数值
  y: number;  // Y轴数值
  z: number;  // Z轴数值
}

export interface ColorData {
  r: number;  // 红色分量 (0-255)
  g: number;  // 绿色分量 (0-255)
  b: number;  // 蓝色分量 (0-255)
  a?: number; // 透明度 (0-255，可选)
}

export interface VectorData {
  values: number[];      // 向量分量值
  dimension: number;     // 向量维度
  labels?: string[];     // 维度标签（可选）
  unit?: string;         // 单位
}

export interface ArrayData {
  values: any[];         // 数组值
  data_type: string;     // 元素数据类型
  size: number;          // 数组大小
  unit?: string;         // 单位
  labels?: string[];     // 元素标签
}

export interface MatrixData {
  values: number[][];    // 矩阵值（行x列）
  rows: number;          // 行数
  cols: number;          // 列数
  unit?: string;         // 单位
}

export interface TimeSeriesData {
  timestamps: string[];  // 时间戳数组（ISO字符串）
  values: number[];      // 对应的数值数组
  unit?: string;         // 数值单位
  interval?: string;     // 采样间隔
}

// 复合数据点
export interface CompositeDataPoint {
  key: string;
  device_id: string;
  timestamp: string;
  type: DataType;
  value: LocationData | Vector3D | ColorData | VectorData | ArrayData | MatrixData | TimeSeriesData | any;
  quality: number;
  tags?: Record<string, string>;
}