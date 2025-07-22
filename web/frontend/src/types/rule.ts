export interface Rule {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  priority: number;
  version: number;
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
  config: Record<string, string | number | boolean>;
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