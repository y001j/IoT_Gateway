// 基础API响应类型
export interface BaseResponse {
  success: boolean;
  message?: string;
  code?: number;
  timestamp?: string;
}

// 分页查询参数
export interface PaginationParams {
  page: number;
  pageSize: number;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
}

// 分页响应
export interface PaginatedResponse<T> extends BaseResponse {
  data: {
    items: T[];
    total: number;
    page: number;
    pageSize: number;
    totalPages: number;
  };
}

// 查询参数基础类型
export interface QueryParams {
  [key: string]: string | number | boolean | undefined;
}

// 错误响应
export interface ErrorResponse extends BaseResponse {
  success: false;
  error: {
    type: string;
    details?: Record<string, string | number | boolean>;
  };
}

// 状态枚举
export type Status = 'success' | 'error' | 'warning' | 'info';

// 时间范围
export interface TimeRange {
  start: string;
  end: string;
}

// 排序选项
export interface SortOption {
  field: string;
  order: 'asc' | 'desc';
}

// 过滤选项
export interface FilterOption {
  field: string;
  operator: 'eq' | 'ne' | 'gt' | 'lt' | 'gte' | 'lte' | 'contains' | 'startsWith' | 'endsWith' | 'in';
  value: string | number | boolean | null;
}

// API请求配置
export interface RequestConfig {
  timeout?: number;
  retries?: number;
  headers?: Record<string, string>;
}

// 操作结果
export interface OperationResult {
  success: boolean;
  message?: string;
  data?: Record<string, unknown>;
}