import api from './api';
import type {
  Rule,
  RuleListRequest,
  RuleListResponse,
  RuleCreateRequest,
  RuleUpdateRequest,
  RuleValidationResponse,
  RuleTestRequest,
  RuleTestResponse
} from '../types/rule';

export const ruleService = {
  // 获取规则列表
  async getRules(params: RuleListRequest): Promise<RuleListResponse> {
    const response = await api.get('/plugins/rules', { 
      params,
      headers: {
        'Cache-Control': 'no-cache',
        'Pragma': 'no-cache',
        'If-Modified-Since': '0'
      }
    });
    return response.data.data;
  },

  // 获取单个规则
  async getRule(id: string): Promise<Rule> {
    const response = await api.get(`/plugins/rules/${id}`, {
      headers: {
        'Cache-Control': 'no-cache',
        'Pragma': 'no-cache',
        'If-Modified-Since': '0'
      }
    });
    return response.data.data;
  },

  // 创建规则
  async createRule(rule: RuleCreateRequest): Promise<Rule> {
    const response = await api.post('/plugins/rules', rule);
    return response.data.data;
  },

  // 更新规则
  async updateRule(id: string, rule: RuleUpdateRequest): Promise<Rule> {
    const response = await api.put(`/plugins/rules/${id}`, rule);
    return response.data.data;
  },

  // 删除规则
  async deleteRule(id: string): Promise<void> {
    await api.delete(`/plugins/rules/${id}`);
  },

  // 启用规则
  async enableRule(id: string): Promise<void> {
    await api.post(`/plugins/rules/${id}/enable`);
  },

  // 禁用规则
  async disableRule(id: string): Promise<void> {
    await api.post(`/plugins/rules/${id}/disable`);
  },

  // 验证规则
  async validateRule(rule: Rule): Promise<RuleValidationResponse> {
    const response = await api.post('/plugins/rules/validate', rule);
    return response.data.data;
  },

  // 测试规则
  async testRule(request: RuleTestRequest): Promise<RuleTestResponse> {
    const response = await api.post('/plugins/rules/test', request);
    return response.data.data;
  },

  // 获取规则模板
  async getRuleTemplates(): Promise<any[]> {
    const response = await api.get('/plugins/rules/templates');
    return response.data.data;
  }
};