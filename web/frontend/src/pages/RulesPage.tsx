import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Tag,
  Space,
  Modal,
  Form,
  Input,
  Select,
  Switch,
  message,
  Popconfirm,
  Tooltip,
  Row,
  Col,
  Typography,
  Drawer,
  Tabs,
  Descriptions,
  InputNumber,
  Segmented,
  Alert
} from 'antd';
import type { ColumnsType, TablePaginationConfig } from 'antd/es/table';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  EyeOutlined,
  SettingOutlined,
  CopyOutlined,
  CheckCircleOutlined,
  QuestionCircleOutlined,
  BookOutlined,
  CodeOutlined,
  FormOutlined
} from '@ant-design/icons';
import { ruleService } from '../services/ruleService';
import type {
  Rule,
  RuleListRequest,
  Action,
  Condition
} from '../types/rule';
import RuleHelp from '../components/RuleHelp';
import ConditionForm from '../components/ConditionForm';
import ActionForm from '../components/ActionForm';
import RuleTemplates from '../components/RuleTemplates';
import DataTypeSelector, { type DataTypeOption } from '../components/DataTypeSelector';
import ComplexDataRuleEditor from '../components/ComplexDataRuleEditor';
import GeospatialRuleEditor from '../components/GeospatialRuleEditor';
import Vector3DRuleEditor from '../components/Vector3DRuleEditor';
import GenericVectorRuleEditor from '../components/GenericVectorRuleEditor';
import VisualRuleEditor from '../components/VisualRuleEditor';

const { Title, Text } = Typography;
const { Search } = Input;
const { Option } = Select;
const { TextArea } = Input;

const RulesPage: React.FC = () => {
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [filterEnabled, setFilterEnabled] = useState<boolean | undefined>();
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0
  });

  // 详情抽屉状态
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false);
  const [selectedRule, setSelectedRule] = useState<Rule | null>(null);

  // 编辑模态框状态
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editForm] = Form.useForm();

  // 帮助抽屉状态
  const [helpVisible, setHelpVisible] = useState(false);

  // 模板选择状态
  const [templatesVisible, setTemplatesVisible] = useState(false);

  // 数据类型选择状态
  const [dataTypeSelectorVisible, setDataTypeSelectorVisible] = useState(false);
  const [selectedDataType, setSelectedDataType] = useState<DataTypeOption | null>(null);
  
  // 复合数据规则编辑器状态
  const [complexRuleEditorVisible, setComplexRuleEditorVisible] = useState(false);
  
  // 专门化规则编辑器状态
  const [geospatialEditorVisible, setGeospatialEditorVisible] = useState(false);
  const [vector3dEditorVisible, setVector3dEditorVisible] = useState(false);
  const [genericVectorEditorVisible, setGenericVectorEditorVisible] = useState(false);
  const [visualEditorVisible, setVisualEditorVisible] = useState(false);

  // 表单模式状态
  const [formMode, setFormMode] = useState<'visual' | 'json'>('visual');

  // 结构化表单状态
  const [currentCondition, setCurrentCondition] = useState<Condition | undefined>();
  const [currentActions, setCurrentActions] = useState<Action[]>([]);

  // JSON预览状态
  const [jsonPreview, setJsonPreview] = useState<string>('');
  const [validationErrors, setValidationErrors] = useState<string[]>([]);

  // 验证条件数据结构
  const validateConditionStructure = (condition: Condition): boolean => {
    if (!condition || !condition.type) {
      console.error('条件缺少 type 字段');
      return false;
    }

    if (condition.type === 'simple') {
      if (condition.and || condition.or || condition.expression) {
        console.error('simple 类型条件不应包含 and、or 或 expression 字段', condition);
        return false;
      }
      return true;
    }

    if (condition.type === 'and') {
      if (!condition.and || !Array.isArray(condition.and)) {
        console.error('and 类型条件必须包含 and 数组', condition);
        return false;
      }
      if (condition.field || condition.operator || condition.value || condition.expression) {
        console.error('and 类型条件不应包含 field、operator、value 或 expression 字段', condition);
        return false;
      }
      return condition.and.every(validateConditionStructure);
    }

    if (condition.type === 'or') {
      if (!condition.or || !Array.isArray(condition.or)) {
        console.error('or 类型条件必须包含 or 数组', condition);
        return false;
      }
      if (condition.field || condition.operator || condition.value || condition.expression) {
        console.error('or 类型条件不应包含 field、operator、value 或 expression 字段', condition);
        return false;
      }
      return condition.or.every(validateConditionStructure);
    }

    if (condition.type === 'expression') {
      if (!condition.expression) {
        console.error('expression 类型条件必须包含 expression 字段', condition);
        return false;
      }
      if (condition.field || condition.operator || condition.value || condition.and || condition.or) {
        console.error('expression 类型条件不应包含其他字段', condition);
        return false;
      }
      return true;
    }

    console.error('未知的条件类型:', condition.type);
    return false;
  };

  // 获取规则列表
  const fetchRules = async () => {
    setLoading(true);
    try {
      const params: RuleListRequest = {
        page: pagination.current,
        page_size: pagination.pageSize,
        search: searchText || undefined,
        enabled: filterEnabled
      };

      const response = await ruleService.getRules(params);
      setRules(response.data);
      setPagination(prev => ({
        ...prev,
        total: response.pagination.total
      }));
    } catch (error: any) {
      message.error('获取规则列表失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 切换规则状态
  const toggleRuleStatus = async (rule: Rule) => {
    try {
      if (rule.enabled) {
        await ruleService.disableRule(rule.id);
        message.success('规则已禁用');
      } else {
        await ruleService.enableRule(rule.id);
        message.success('规则已启用');
      }
      // 添加延迟以确保状态同步
      await new Promise(resolve => setTimeout(resolve, 300));
      await fetchRules();
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 删除规则
  const handleDeleteRule = async (rule: Rule) => {
    try {
      await ruleService.deleteRule(rule.id);
      message.success('删除规则成功');
      // 添加延迟以确保状态同步
      await new Promise(resolve => setTimeout(resolve, 300));
      await fetchRules();
    } catch (error: any) {
      message.error('删除规则失败：' + (error.message || '未知错误'));
    }
  };

  // 显示规则详情
  const showRuleDetails = (rule: Rule) => {
    setSelectedRule(rule);
    setDetailDrawerVisible(true);
  };

  // 显示数据类型选择器
  const showDataTypeSelector = () => {
    setDataTypeSelectorVisible(true);
  };

  // 处理数据类型选择
  const handleDataTypeSelect = (dataType: DataTypeOption) => {
    setSelectedDataType(dataType);
    setDataTypeSelectorVisible(false);
    
    if (dataType.type === 'complex') {
      // 复合数据类型，根据类别选择专门化编辑器
      switch (dataType.category) {
        case 'geospatial':
          setGeospatialEditorVisible(true);
          break;
        case 'vector':
          setVector3dEditorVisible(true);
          break;
        case 'vector_generic':
          setGenericVectorEditorVisible(true);
          break;
        case 'visual':
          setVisualEditorVisible(true);
          break;
        default:
          // 其他复合数据类型使用通用编辑器
          setComplexRuleEditorVisible(true);
          break;
      }
    } else {
      // 简单数据类型，使用原有的编辑器
      showEditModal();
    }
  };

  // 显示编辑模态框
  const showEditModal = (rule?: Rule) => {
    setIsEditing(!!rule);
    if (rule) {
      setSelectedRule(rule);
      // 设置表单基本字段
      editForm.setFieldsValue({
        name: rule.name,
        description: rule.description,
        enabled: rule.enabled,
        priority: rule.priority,
        tags: rule.tags ? Object.entries(rule.tags).map(([key, value]) => ({ key, value })) : []
      });
      // 设置结构化表单数据
      setCurrentCondition(rule.conditions);
      setCurrentActions(rule.actions || []);
      // 设置JSON模式数据
      editForm.setFieldsValue({
        conditions: JSON.stringify(rule.conditions, null, 2),
        actions: JSON.stringify(rule.actions, null, 2)
      });
    } else {
      editForm.resetFields();
      const defaultCondition: Condition = {
        type: 'simple',
        field: 'key',
        operator: 'eq',
        value: 'temperature'
      };
      const defaultActions: Action[] = [{
        type: 'alert',
        config: {
          level: 'warning',
          message: '告警信息'
        }
      }];
      
      editForm.setFieldsValue({
        enabled: true,
        priority: 100,
        conditions: JSON.stringify(defaultCondition, null, 2),
        actions: JSON.stringify(defaultActions, null, 2)
      });
      setCurrentCondition(defaultCondition);
      setCurrentActions(defaultActions);
    }
    setFormMode('visual'); // 默认使用可视化模式
    setValidationErrors([]);
    updateJsonPreview();
    setEditModalVisible(true);
  };

  // 更新JSON预览
  const updateJsonPreview = () => {
    try {
      const values = editForm.getFieldsValue();
      let conditions, actions;
      
      if (formMode === 'visual') {
        conditions = currentCondition;
        actions = currentActions;
      } else {
        conditions = values.conditions ? JSON.parse(values.conditions) : undefined;
        actions = values.actions ? JSON.parse(values.actions) : [];
      }
      
      const preview = {
        name: values.name || '',
        description: values.description || '',
        enabled: values.enabled ?? true,
        priority: values.priority || 100,
        conditions,
        actions,
        tags: values.tags?.reduce((acc: any, item: any) => {
          if (item.key && item.value) {
            acc[item.key] = item.value;
          }
          return acc;
        }, {}) || {}
      };
      
      setJsonPreview(JSON.stringify(preview, null, 2));
      setValidationErrors([]);
    } catch (error: any) {
      setValidationErrors([error.message || 'JSON格式错误']);
    }
  };

  // 处理表单字段变更
  const handleFormChange = () => {
    updateJsonPreview();
  };

  // 处理条件变更
  const handleConditionChange = (condition: Condition) => {
    setCurrentCondition(condition);
    updateJsonPreview();
  };

  // 处理动作变更
  const handleActionsChange = (actions: Action[]) => {
    setCurrentActions(actions);
    updateJsonPreview();
  };

  // 处理模式切换
  const handleModeChange = (mode: 'visual' | 'json') => {
    if (mode === 'json' && formMode === 'visual') {
      // 从可视化模式切换到JSON模式，同步数据
      editForm.setFieldsValue({
        conditions: JSON.stringify(currentCondition, null, 2),
        actions: JSON.stringify(currentActions, null, 2)
      });
    } else if (mode === 'visual' && formMode === 'json') {
      // 从JSON模式切换到可视化模式，解析JSON数据
      try {
        const values = editForm.getFieldsValue();
        if (values.conditions) {
          const conditions = JSON.parse(values.conditions);
          setCurrentCondition(conditions);
        }
        if (values.actions) {
          const actions = JSON.parse(values.actions);
          setCurrentActions(actions);
        }
        setValidationErrors([]);
      } catch (error: any) {
        message.error('JSON格式错误，无法切换到可视化模式');
        return;
      }
    }
    setFormMode(mode);
    setTimeout(updateJsonPreview, 0);
  };

  // 保存规则
  const handleSaveRule = async () => {
    try {
      const values = await editForm.validateFields();
      
      let conditions, actions;
      if (formMode === 'visual') {
        conditions = currentCondition;
        actions = currentActions;
        
        // 验证条件数据结构
        if (conditions && !validateConditionStructure(conditions)) {
          message.error('条件数据结构不正确，无法保存');
          return;
        }
      } else {
        conditions = JSON.parse(values.conditions);
        actions = JSON.parse(values.actions);
        
        // 验证条件数据结构
        if (conditions && !validateConditionStructure(conditions)) {
          message.error('条件数据结构不正确，无法保存');
          return;
        }
      }
      
      // 基本数据验证
      if (!conditions) {
        message.error('请配置触发条件');
        return;
      }
      
      if (!actions || actions.length === 0) {
        message.error('请配置至少一个动作');
        return;
      }
      
      const tags = values.tags?.reduce((acc: any, item: any) => {
        if (item.key && item.value) {
          acc[item.key] = item.value;
        }
        return acc;
      }, {});

      const ruleData = {
        name: values.name,
        description: values.description,
        enabled: values.enabled,
        priority: values.priority,
        conditions,
        actions,
        tags
      };

      if (isEditing && selectedRule) {
        await ruleService.updateRule(selectedRule.id, {
          ...ruleData,
          version: selectedRule.version
        });
        message.success('规则更新成功');
      } else {
        await ruleService.createRule(ruleData);
        message.success('规则创建成功');
      }

      setEditModalVisible(false);
      // 添加延迟以确保后端文件监控和内存状态同步
      await new Promise(resolve => setTimeout(resolve, 500));
      await fetchRules();
    } catch (error: any) {
      console.error('保存规则失败:', error);
      
      if (error instanceof SyntaxError) {
        message.error('JSON 格式错误，请检查条件和动作配置');
      } else if (error.response) {
        // API 错误响应
        const errorMsg = error.response.data?.message || error.response.data?.error || error.response.statusText || '服务器错误';
        message.error(`保存规则失败: ${errorMsg} (状态码: ${error.response.status})`);
        
        // 详细错误信息输出到控制台
        console.error('API错误响应:', {
          status: error.response.status,
          data: error.response.data,
          headers: error.response.headers
        });
      } else if (error.request) {
        // 请求发送失败
        message.error('网络请求失败，请检查网络连接');
        console.error('网络请求失败:', error.request);
      } else {
        // 其他错误
        message.error('保存规则失败：' + (error.message || '未知错误'));
        console.error('其他错误:', error.message);
      }
    }
  };

  // 复制规则
  const copyRule = (rule: Rule) => {
    const newRule = {
      ...rule,
      name: `${rule.name} (副本)`,
      id: '' // 新规则使用空字符串ID
    };
    
    // 智能判断规则类型并使用相应编辑器
    if (isComplexDataRule(newRule)) {
      handleEditRule(newRule);
    } else {
      showEditModal(newRule);
    }
  };

  // 处理模板选择
  const handleTemplateSelect = (template: any) => {
    const templateRule = template.rule;
    
    // 设置表单基本字段
    editForm.setFieldsValue({
      name: templateRule.name,
      description: templateRule.description,
      enabled: templateRule.enabled ?? true,
      priority: templateRule.priority || 100,
      tags: templateRule.tags ? Object.entries(templateRule.tags).map(([key, value]) => ({ key, value })) : []
    });
    
    // 设置结构化表单数据
    setCurrentCondition(templateRule.conditions);
    setCurrentActions(templateRule.actions || []);
    
    // 设置JSON模式数据
    editForm.setFieldsValue({
      conditions: JSON.stringify(templateRule.conditions, null, 2),
      actions: JSON.stringify(templateRule.actions, null, 2)
    });
    
    setFormMode('visual'); // 使用可视化模式
    setValidationErrors([]);
    setIsEditing(false);
    setSelectedRule(null);
    
    setTimeout(updateJsonPreview, 0);
    setEditModalVisible(true);
    
    message.success(`已加载模板：${template.name}`);
  };

  // 处理复合数据规则保存
  const handleComplexRuleSave = async (ruleData: Partial<Rule>) => {
    try {
      if (isEditing && selectedRule) {
        await ruleService.updateRule(selectedRule.id, {
          ...ruleData,
          version: selectedRule.version
        });
        message.success('复合数据规则更新成功');
      } else {
        await ruleService.createRule(ruleData);
        message.success('复合数据规则创建成功');
      }

      setComplexRuleEditorVisible(false);
      setSelectedDataType(null);
      // 添加延迟以确保后端文件监控和内存状态同步
      await new Promise(resolve => setTimeout(resolve, 500));
      await fetchRules();
    } catch (error: any) {
      console.error('保存复合数据规则失败:', error);
      
      if (error.response) {
        const errorMsg = error.response.data?.message || error.response.data?.error || error.response.statusText || '服务器错误';
        message.error(`保存复合数据规则失败: ${errorMsg} (状态码: ${error.response.status})`);
      } else if (error.request) {
        message.error('网络请求失败，请检查网络连接');
      } else {
        message.error('保存复合数据规则失败：' + (error.message || '未知错误'));
      }
    }
  };

  // 处理复合数据规则编辑器取消
  const handleComplexRuleCancel = () => {
    setComplexRuleEditorVisible(false);
    setSelectedDataType(null);
    setSelectedRule(null);
    setIsEditing(false);
  };

  // 返回数据类型选择器
  const handleBackToDataTypeSelector = () => {
    // 关闭所有编辑器
    setComplexRuleEditorVisible(false);
    setGeospatialEditorVisible(false);
    setVector3dEditorVisible(false);
    setGenericVectorEditorVisible(false);
    setVisualEditorVisible(false);
    // 重新打开数据类型选择器
    setDataTypeSelectorVisible(true);
  };

  // 专门化规则编辑器的保存处理
  const handleSpecializedRuleSave = async (ruleData: Partial<Rule>) => {
    try {
      if (isEditing && selectedRule) {
        await ruleService.updateRule(selectedRule.id, {
          ...ruleData,
          version: selectedRule.version
        });
        message.success('复合数据规则更新成功');
      } else {
        await ruleService.createRule(ruleData);
        message.success('复合数据规则创建成功');
      }

      // 关闭所有专门化编辑器
      setGeospatialEditorVisible(false);
      setVector3dEditorVisible(false);
      setGenericVectorEditorVisible(false);
      setVisualEditorVisible(false);
      setComplexRuleEditorVisible(false);
      setSelectedDataType(null);
      
      // 添加延迟以确保后端文件监控和内存状态同步
      await new Promise(resolve => setTimeout(resolve, 500));
      await fetchRules();
    } catch (error: any) {
      console.error('保存专门化规则失败:', error);
      
      if (error.response) {
        const errorMsg = error.response.data?.message || error.response.data?.error || error.response.statusText || '服务器错误';
        message.error(`保存规则失败: ${errorMsg} (状态码: ${error.response.status})`);
      } else if (error.request) {
        message.error('网络请求失败，请检查网络连接');
      } else {
        message.error('保存规则失败：' + (error.message || '未知错误'));
      }
    }
  };

  // 专门化规则编辑器的取消处理
  const handleSpecializedRuleCancel = () => {
    console.log('handleSpecializedRuleCancel 被调用，当前状态:');
    console.log('- visualEditorVisible:', visualEditorVisible);
    console.log('- geospatialEditorVisible:', geospatialEditorVisible);
    console.log('- vector3dEditorVisible:', vector3dEditorVisible);
    
    setGeospatialEditorVisible(false);
    setVector3dEditorVisible(false);
    setGenericVectorEditorVisible(false);
    setVisualEditorVisible(false);
    setComplexRuleEditorVisible(false);
    setSelectedDataType(null);
    setSelectedRule(null);
    setIsEditing(false);
    
    console.log('已设置所有编辑器为不可见');
  };

  // 检测规则是否为复合数据规则 - 增强版本
  const isComplexDataRule = (rule: Rule): boolean => {
    // 优先级1: 检查规则的数据类型标记 (最准确的识别方式)
    // 检查根级别data_type字段和tags中的数据类型
    const rootDataType = rule.data_type;
    const tagDataType = rule.tags ? (rule.tags['data_type'] || rule.tags['data_category']) : null;
    const dataType = rootDataType || tagDataType;
    
    // 扩展支持的复合数据类型
    const supportedTypes = [
      'geospatial', 'vector', 'visual', 'array', 'matrix', 'timeseries',
      'location', 'vector3d', 'color', 'gps', 'vector_generic', 'mixed'  // 支持具体的数据类型
    ];
    
    if (dataType && supportedTypes.includes(dataType)) {
      console.log(`检测到复合数据规则 [${rule.name}]: 数据类型标记 = ${dataType}`);
      return true;
    }
    
    // 优先级2: 检查动作类型是否包含复合数据特有的动作
    const complexActionTypes = [
      // 地理空间动作
      'geo_transform', 'geo_aggregate', 'geo_filter', 'geospatial_transform',
      // 向量数据动作  
      'vector_transform', 'vector_aggregate', 'vector_filter',
      // 颜色/视觉数据动作
      'color_transform', 'color_aggregate', 'color_filter', 'visual_transform',
      // 数组数据动作
      'array_transform', 'array_aggregate', 'array_filter',
      // 矩阵数据动作
      'matrix_transform', 'matrix_aggregate', 'matrix_filter',
      // 时间序列数据动作
      'timeseries_transform', 'timeseries_aggregate', 'timeseries_filter'
    ];
    
    if (rule.actions && rule.actions.some(action => complexActionTypes.includes(action.type))) {
      const actionType = rule.actions.find(action => complexActionTypes.includes(action.type))?.type;
      console.log(`检测到复合数据规则 [${rule.name}]: 复合动作类型 = ${actionType}`);
      return true;
    }
    
    // 优先级3: 检查条件中是否包含复合数据字段
    const complexFields = [
      // GPS/地理数据字段
      'latitude', 'longitude', 'altitude', 'lat', 'lng', 'coord', 'location',
      // 向量数据字段
      'x', 'y', 'z', 'magnitude', 'direction', 'velocity', 'acceleration',
      // 颜色数据字段  
      'r', 'g', 'b', 'hue', 'saturation', 'lightness', 'alpha', 'rgb', 'hsl',
      // 数组/矩阵字段
      'rows', 'cols', 'size', 'length', 'dimensions', 'matrix', 'array',
      // 时间序列字段
      'duration', 'data_points', 'timestamps', 'series', 'trend'
    ];
    
    if (rule.conditions) {
      const checkConditionFields = (condition: Condition): boolean => {
        if (condition.field && complexFields.includes(condition.field)) {
          return true;
        }
        // 递归检查复合条件
        if (condition.and && condition.and.some(checkConditionFields)) {
          return true;
        }
        if (condition.or && condition.or.some(checkConditionFields)) {
          return true;
        }
        return false;
      };
      
      if (checkConditionFields(rule.conditions)) {
        const fieldName = extractComplexField(rule.conditions);
        console.log(`检测到复合数据规则 [${rule.name}]: 复合字段 = ${fieldName}`);
        return true;
      }
    }
    
    return false;
  };

  // 辅助函数：提取复合数据字段名称
  const extractComplexField = (condition: Condition): string => {
    const complexFields = [
      'latitude', 'longitude', 'altitude', 'lat', 'lng', 'coord', 'location',
      'x', 'y', 'z', 'magnitude', 'direction', 'velocity', 'acceleration',
      'r', 'g', 'b', 'hue', 'saturation', 'lightness', 'alpha', 'rgb', 'hsl',
      'rows', 'cols', 'size', 'length', 'dimensions', 'matrix', 'array',
      'duration', 'data_points', 'timestamps', 'series', 'trend'
    ];
    
    if (condition.field && complexFields.includes(condition.field)) {
      return condition.field;
    }
    
    if (condition.and) {
      for (const subCondition of condition.and) {
        const field = extractComplexField(subCondition);
        if (field) return field;
      }
    }
    
    if (condition.or) {
      for (const subCondition of condition.or) {
        const field = extractComplexField(subCondition);
        if (field) return field;
      }
    }
    
    return '未知复合字段';
  };

  // 智能推断数据类型
  const inferDataCategory = (rule: Rule): string => {
    // 优先级0: 检查根级别data_type字段
    const rootDataType = rule.data_type;
    if (rootDataType) {
      console.log(`规则 [${rule.name}] 检测到 data_type 字段:`, rootDataType);
      // 将具体类型映射到类别
      const typeMapping: Record<string, string> = {
        'location': 'geospatial',
        'vector3d': 'vector',
        'vector_generic': 'vector_generic',
        'gps': 'geospatial',
        'color': 'visual',
        'array': 'array',
        'matrix': 'matrix',
        'timeseries': 'timeseries',
        'mixed': 'mixed'
      };
      const mappedCategory = typeMapping[rootDataType] || rootDataType;
      console.log(`数据类型 ${rootDataType} 映射到类别:`, mappedCategory);
      return mappedCategory;
    }
    
    // 优先级1: 从tags中获取
    if (rule.tags && rule.tags['data_category']) {
      return rule.tags['data_category'];
    }
    
    // 优先级2: 从动作类型推断
    if (rule.actions && rule.actions.length > 0) {
      const actionType = rule.actions[0].type;
      if (actionType.includes('geo')) return 'geospatial';
      if (actionType.includes('vector')) return 'vector';
      if (actionType.includes('color') || actionType.includes('visual')) return 'visual';
      if (actionType.includes('array')) return 'array';
      if (actionType.includes('matrix')) return 'matrix';
      if (actionType.includes('timeseries') || actionType.includes('time')) return 'timeseries';
    }
    
    // 优先级3: 从条件字段推断
    if (rule.conditions) {
      const field = extractComplexField(rule.conditions);
      if (['latitude', 'longitude', 'altitude', 'lat', 'lng', 'coord', 'location'].includes(field)) {
        return 'geospatial';
      }
      if (['x', 'y', 'z', 'magnitude', 'direction', 'velocity', 'acceleration'].includes(field)) {
        return 'vector';
      }
      if (['r', 'g', 'b', 'hue', 'saturation', 'lightness', 'alpha', 'rgb', 'hsl'].includes(field)) {
        return 'visual';
      }
      if (['rows', 'cols', 'dimensions', 'matrix'].includes(field)) {
        return 'matrix';
      }
      if (['size', 'length', 'array'].includes(field)) {
        return 'array';
      }
      if (['duration', 'data_points', 'timestamps', 'series', 'trend'].includes(field)) {
        return 'timeseries';
      }
    }
    
    // 默认值
    return 'geospatial';
  };

  // 获取数据类型友好名称
  const getDataTypeFriendlyName = (category: string): string => {
    const nameMap: Record<string, string> = {
      'geospatial': 'GPS位置数据',
      'vector': '向量数据', 
      'visual': '颜色数据',
      'array': '数组数据',
      'matrix': '矩阵数据',
      'timeseries': '时间序列数据'
    };
    return nameMap[category] || '复合数据';
  };

  // 获取数据类型图标
  const getDataTypeIcon = (category: string) => {
    const iconMap: Record<string, React.ReactNode> = {
      'geospatial': <BookOutlined />,
      'vector': <FormOutlined />,
      'visual': <EyeOutlined />,
      'array': <CodeOutlined />,
      'matrix': <SettingOutlined />,
      'timeseries': <PlayCircleOutlined />
    };
    return iconMap[category] || <FormOutlined />;
  };

  // 获取数据类型颜色
  const getDataTypeColor = (category: string): string => {
    const colorMap: Record<string, string> = {
      'geospatial': '#52c41a',  // 绿色 - GPS
      'vector': '#1890ff',      // 蓝色 - 向量
      'visual': '#fa541c',      // 橙色 - 视觉
      'array': '#722ed1',       // 紫色 - 数组
      'matrix': '#eb2f96',      // 粉色 - 矩阵
      'timeseries': '#13c2c2'   // 青色 - 时间序列
    };
    return colorMap[category] || '#722ed1';
  };

  // 智能处理规则编辑
  const handleEditRule = (rule: Rule) => {
    if (isComplexDataRule(rule)) {
      // 复合数据规则，智能选择专门化编辑器
      setSelectedRule(rule);
      setIsEditing(true);
      
      // 智能推断数据类型
      let dataCategory = inferDataCategory(rule);
      let dataTypeName = getDataTypeFriendlyName(dataCategory);
      let dataTypeKey = rule.tags?.['data_type'] || `${dataCategory}_data`;
      
      // 创建临时数据类型选项
      const tempDataType: DataTypeOption = {
        type: 'complex',
        category: dataCategory,
        key: dataTypeKey,
        name: dataTypeName,
        icon: getDataTypeIcon(dataCategory),
        description: `检测到的${dataTypeName}规则`,
        examples: [],
        color: getDataTypeColor(dataCategory)
      };
      
      setSelectedDataType(tempDataType);
      
      // 根据数据类别选择对应的专门化编辑器
      console.log(`为规则 [${rule.name}] 选择编辑器，数据类别:`, dataCategory);
      switch (dataCategory) {
        case 'geospatial':
          console.log('使用地理数据专门编辑器');
          setGeospatialEditorVisible(true);
          break;
        case 'vector':
          console.log('使用3D向量专门编辑器');
          setVector3dEditorVisible(true);
          break;
        case 'vector_generic':
          console.log('使用通用向量专门编辑器');
          setGenericVectorEditorVisible(true);
          break;
        case 'visual':
          console.log('使用颜色数据专门编辑器');
          setVisualEditorVisible(true);
          break;
        case 'array':
        case 'matrix':
        case 'timeseries':
        case 'mixed':
          console.log('使用复合数据通用编辑器');
          setComplexRuleEditorVisible(true);
          break;
        default:
          console.log('使用复合数据通用编辑器（默认）');
          // 其他复合数据类型使用通用编辑器
          setComplexRuleEditorVisible(true);
          break;
      }
    } else {
      // 简单数据规则，使用原有编辑器
      showEditModal(rule);
    }
  };

  // 工具函数
  const getPriorityColor = (priority: number) => {
    if (priority >= 150) return 'red';
    if (priority >= 100) return 'orange';
    if (priority >= 50) return 'blue';
    return 'green';
  };

  const getActionTypeTag = (type: string) => {
    const colors: Record<string, string> = {
      alert: 'red',
      transform: 'blue',
      filter: 'purple',
      aggregate: 'cyan',
      forward: 'green'
    };
    return <Tag color={colors[type] || 'default'}>{type}</Tag>;
  };

  // 格式化动作配置显示
  const formatActionConfig = (action: Action): string => {
    const { type, config } = action;
    
    switch (type) {
      case 'aggregate':
        return `窗口大小: ${config.size || config.window_size || 'N/A'} 个数据点
聚合函数: ${Array.isArray(config.functions) ? config.functions.join(', ') : 'N/A'}
分组字段: ${Array.isArray(config.group_by) ? config.group_by.join(', ') : 'N/A'}
输出字段: ${config.output_key || 'N/A'}
转发结果: ${config.forward ? '是' : '否'}`;
      
      case 'transform':
        return `转换类型: ${config.type || 'N/A'}
目标字段: ${config.field || 'N/A'}
输出字段: ${config.output_key || 'N/A'}
缩放因子: ${config.factor || config.scale_factor || 'N/A'}
偏移量: ${config.offset || 'N/A'}
精度: ${config.precision || 'N/A'}
添加标签: ${config.add_tags ? JSON.stringify(config.add_tags) : 'N/A'}`;
      
      case 'alert':
        return `告警级别: ${config.level || 'N/A'}
告警消息: ${config.message || 'N/A'}
限流时间: ${config.throttle || 'N/A'}
通知渠道: ${Array.isArray(config.channels) ? config.channels.join(', ') : 'N/A'}`;
      
      case 'filter':
        return `过滤类型: ${config.type || 'N/A'}
最小值: ${config.min !== undefined ? config.min : 'N/A'}
最大值: ${config.max !== undefined ? config.max : 'N/A'}
匹配动作: ${config.drop_on_match ? '丢弃' : '通过'}
速率限制: ${config.rate || 'N/A'}
时间窗口: ${config.window || 'N/A'}`;
      
      case 'forward':
        return `转发目标: ${config.target_type || 'N/A'}
URL地址: ${config.url || 'N/A'}
HTTP方法: ${config.method || 'N/A'}
MQTT代理: ${config.broker || 'N/A'}
主题模板: ${config.topic || 'N/A'}
文件路径: ${config.path || 'N/A'}
批处理大小: ${config.batch_size || 'N/A'}
超时时间: ${config.timeout || 'N/A'}`;
      
      default:
        return JSON.stringify(config, null, 2);
    }
  };

  // 表格列定义
  const columns: ColumnsType<Rule> = [
    {
      title: '规则名称',
      dataIndex: 'name',
      key: 'name',
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (name: string, record: Rule) => {
        const isComplex = isComplexDataRule(record);
        const dataCategory = isComplex ? inferDataCategory(record) : null;
        
        return (
          <div>
            <div>
              <strong>{name}</strong>
              {isComplex && (
                <Tag 
                  size="small" 
                  color={getDataTypeColor(dataCategory!)} 
                  icon={getDataTypeIcon(dataCategory!)}
                  style={{ marginLeft: 8 }}
                >
                  {getDataTypeFriendlyName(dataCategory!)}
                </Tag>
              )}
            </div>
            <Text type="secondary" style={{ fontSize: 12 }}>{record.description}</Text>
          </div>
        );
      }
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 100,
      render: (priority: number) => (
        <Tag color={getPriorityColor(priority)}>{priority}</Tag>
      ),
      sorter: (a, b) => {
        // 首先按优先级排序（降序）
        if (a.priority !== b.priority) {
          return b.priority - a.priority;
        }
        // 优先级相同时按规则名称排序（升序）
        return a.name.localeCompare(b.name);
      },
      defaultSortOrder: 'ascend'
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 100,
      render: (enabled: boolean, record: Rule) => (
        <Switch
          checked={enabled}
          onChange={() => toggleRuleStatus(record)}
          checkedChildren="启用"
          unCheckedChildren="禁用"
        />
      ),
      filters: [
        { text: '已启用', value: true },
        { text: '已禁用', value: false }
      ]
    },
    {
      title: '动作类型',
      dataIndex: 'actions',
      key: 'actions',
      width: 150,
      render: (actions: Action[]) => (
        <Space wrap>
          {actions && actions.length > 0 ? (
            <>
              {actions.slice(0, 2).map((action, index) => (
                <Tooltip key={index} title={formatActionConfig(action)} overlayStyle={{ maxWidth: '400px' }}>
                  {getActionTypeTag(action.type)}
                </Tooltip>
              ))}
              {actions.length > 2 && (
                <Tag>+{actions.length - 2}</Tag>
              )}
            </>
          ) : (
            <Tag color="default">无动作</Tag>
          )}
        </Space>
      )
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
      render: (time: string) => new Date(time).toLocaleString()
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, record: Rule) => (
        <Space>
          <Tooltip title="查看详情">
            <Button
              type="link"
              icon={<EyeOutlined />}
              onClick={() => showRuleDetails(record)}
            />
          </Tooltip>
          
          <Tooltip title={
            isComplexDataRule(record) 
              ? `编辑复合规则 (${getDataTypeFriendlyName(inferDataCategory(record))})` 
              : "编辑规则"
          }>
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEditRule(record)}
            />
          </Tooltip>
          
          <Tooltip title="复制">
            <Button
              type="link"
              icon={<CopyOutlined />}
              onClick={() => copyRule(record)}
            />
          </Tooltip>
          
          <Popconfirm
            title="确定要删除这个规则吗？"
            description="删除后无法恢复"
            onConfirm={() => handleDeleteRule(record)}
            okText="确定"
            cancelText="取消"
          >
            <Tooltip title="删除">
              <Button
                type="link"
                danger
                icon={<DeleteOutlined />}
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      )
    }
  ];

  // 处理表格变化
  const handleTableChange = (paginationConfig: TablePaginationConfig, filters: any) => {
    setPagination(prev => ({
      ...prev,
      current: paginationConfig.current || 1,
      pageSize: paginationConfig.pageSize || 10
    }));
    
    if (filters.enabled !== undefined) {
      setFilterEnabled(filters.enabled?.[0]);
    }
  };

  // 初始化和依赖更新
  useEffect(() => {
    fetchRules();
  }, [pagination.current, pagination.pageSize, searchText, filterEnabled]);

  return (
    <div>
      <Title level={2}>规则管理</Title>
      
      {/* 操作栏 */}
      <Card style={{ marginBottom: 16 }}>
        <Row gutter={16} align="middle">
          <Col flex="auto">
            <Space>
              <Search
                placeholder="搜索规则名称或描述"
                allowClear
                style={{ width: 300 }}
                onSearch={setSearchText}
                onClear={() => setSearchText('')}
              />
              <Select
                placeholder="状态筛选"
                allowClear
                style={{ width: 120 }}
                value={filterEnabled}
                onChange={setFilterEnabled}
              >
                <Option value={true}>已启用</Option>
                <Option value={false}>已禁用</Option>
              </Select>
            </Space>
          </Col>
          <Col>
            <Space>
              <Button 
                icon={<QuestionCircleOutlined />} 
                onClick={() => setHelpVisible(true)}
              >
                帮助文档
              </Button>
              <Button icon={<PlayCircleOutlined />}>
                规则测试
              </Button>
              <Button 
                icon={<SettingOutlined />}
                onClick={() => setTemplatesVisible(true)}
              >
                模板库
              </Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={showDataTypeSelector}>
                创建规则
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 规则列表 */}
      <Card>
        <Table
          columns={columns}
          dataSource={rules}
          rowKey="id"
          loading={loading}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`
          }}
          onChange={handleTableChange}
        />
      </Card>

      {/* 规则详情抽屉 */}
      <Drawer
        title="规则详情"
        width={800}
        open={detailDrawerVisible}
        onClose={() => setDetailDrawerVisible(false)}
      >
        {selectedRule && (
          <Tabs
            defaultActiveKey="1"
            items={[
              {
                key: "1",
                label: "基本信息",
                children: (
                  <Descriptions title="基本信息" bordered>
                    <Descriptions.Item label="规则名称">{selectedRule.name}</Descriptions.Item>
                    <Descriptions.Item label="描述" span={2}>{selectedRule.description}</Descriptions.Item>
                    <Descriptions.Item label="状态">
                      {selectedRule.enabled ? 
                        <Tag color="green" icon={<CheckCircleOutlined />}>已启用</Tag> : 
                        <Tag color="red" icon={<PauseCircleOutlined />}>已禁用</Tag>
                      }
                    </Descriptions.Item>
                    <Descriptions.Item label="优先级">
                      <Tag color={getPriorityColor(selectedRule.priority)}>{selectedRule.priority}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="版本">v{selectedRule.version}</Descriptions.Item>
                    <Descriptions.Item label="创建时间">{new Date(selectedRule.created_at).toLocaleString()}</Descriptions.Item>
                    <Descriptions.Item label="更新时间">{new Date(selectedRule.updated_at).toLocaleString()}</Descriptions.Item>
                    <Descriptions.Item label="标签" span={2}>
                      {selectedRule.tags && Object.entries(selectedRule.tags).map(([key, value]) => (
                        <Tag key={key}>{key}: {value}</Tag>
                      ))}
                    </Descriptions.Item>
                  </Descriptions>
                )
              },
              {
                key: "2",
                label: "条件配置",
                children: (
                  <div>
                    <Title level={4}>触发条件</Title>
                    <pre style={{ background: '#f5f5f5', padding: 16, borderRadius: 4 }}>
                      {JSON.stringify(selectedRule.conditions, null, 2)}
                    </pre>
                  </div>
                )
              },
              {
                key: "3",
                label: "动作配置",
                children: (
                  <div>
                    <Title level={4}>执行动作</Title>
                    {selectedRule.actions && selectedRule.actions.length > 0 ? (
                      selectedRule.actions.map((action, index) => (
                        <Card key={index} size="small" style={{ marginBottom: 8 }}>
                          <Space>
                            {getActionTypeTag(action.type)}
                            <Text strong>动作 {index + 1}</Text>
                          </Space>
                          <pre style={{ background: '#f5f5f5', padding: 8, marginTop: 8, borderRadius: 4 }}>
                            {JSON.stringify(action.config, null, 2)}
                          </pre>
                        </Card>
                      ))
                    ) : (
                      <Text type="secondary">该规则暂无配置动作</Text>
                    )}
                  </div>
                )
              }
            ]}
          />
        )}
      </Drawer>

      {/* 编辑/创建规则模态框 */}
      <Modal
        title={
          <Space>
            {isEditing ? '编辑规则' : '创建规则'}
            <Button 
              type="link" 
              icon={<BookOutlined />} 
              onClick={() => setHelpVisible(true)}
              size="small"
            >
              帮助
            </Button>
          </Space>
        }
        open={editModalVisible}
        onOk={handleSaveRule}
        onCancel={() => setEditModalVisible(false)}
        width={1200}
        okText="保存"
        cancelText="取消"
        styles={{ body: { maxHeight: '70vh', overflowY: 'auto' } }}
      >
        <Row gutter={16}>
          <Col span={formMode === 'visual' ? 24 : 14}>
            {/* 模式切换 */}
            <Card size="small" style={{ marginBottom: 16 }}>
              <Row justify="space-between" align="middle">
                <Col>
                  <Segmented
                    value={formMode}
                    onChange={handleModeChange}
                    options={[
                      {
                        label: (
                          <Space>
                            <FormOutlined />
                            可视化编辑
                          </Space>
                        ),
                        value: 'visual'
                      },
                      {
                        label: (
                          <Space>
                            <CodeOutlined />
                            JSON编辑
                          </Space>
                        ),
                        value: 'json'
                      }
                    ]}
                  />
                </Col>
                <Col>
                  {validationErrors.length > 0 && (
                    <Alert
                      message="配置错误"
                      description={validationErrors.join(', ')}
                      type="error"
                      showIcon
                    />
                  )}
                </Col>
              </Row>
            </Card>

            <Form form={editForm} layout="vertical" onValuesChange={handleFormChange}>
              {/* 基本信息 */}
              <Card title="基本信息" size="small" style={{ marginBottom: 16 }}>
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item
                      label="规则名称"
                      name="name"
                      rules={[{ required: true, message: '请输入规则名称' }]}
                    >
                      <Input placeholder="输入规则名称" />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item
                      label="优先级"
                      name="priority"
                      rules={[{ required: true, message: '请输入优先级' }]}
                    >
                      <InputNumber 
                        placeholder="数值越大优先级越高" 
                        style={{ width: '100%' }}
                        min={1}
                        max={999}
                      />
                    </Form.Item>
                  </Col>
                </Row>
                
                <Form.Item
                  label="描述"
                  name="description"
                  rules={[{ required: true, message: '请输入规则描述' }]}
                >
                  <TextArea rows={2} placeholder="输入规则描述" />
                </Form.Item>
                
                <Form.Item label="启用状态" name="enabled" valuePropName="checked">
                  <Switch checkedChildren="启用" unCheckedChildren="禁用" />
                </Form.Item>
              </Card>

              {/* 条件和动作配置 */}
              {formMode === 'visual' ? (
                <>
                  <ConditionForm 
                    value={currentCondition} 
                    onChange={handleConditionChange}
                  />
                  <div style={{ marginBottom: 16 }} />
                  <ActionForm 
                    value={currentActions} 
                    onChange={handleActionsChange}
                  />
                </>
              ) : (
                <>
                  <Card title="JSON配置" size="small">
                    <Form.Item
                      label="触发条件 (JSON 格式)"
                      name="conditions"
                      rules={[
                        { required: true, message: '请输入触发条件' },
                        {
                          validator: (_, value) => {
                            try {
                              JSON.parse(value);
                              return Promise.resolve();
                            } catch {
                              return Promise.reject(new Error('JSON 格式错误'));
                            }
                          }
                        }
                      ]}
                    >
                      <TextArea
                        rows={8}
                        placeholder='{"type": "simple", "field": "key", "operator": "eq", "value": "temperature"}'
                        style={{ fontFamily: 'monospace' }}
                      />
                    </Form.Item>
                    
                    <Form.Item
                      label="执行动作 (JSON 格式)"
                      name="actions"
                      rules={[
                        { required: true, message: '请输入执行动作' },
                        {
                          validator: (_, value) => {
                            try {
                              JSON.parse(value);
                              return Promise.resolve();
                            } catch {
                              return Promise.reject(new Error('JSON 格式错误'));
                            }
                          }
                        }
                      ]}
                    >
                      <TextArea
                        rows={8}
                        placeholder='[{"type": "alert", "config": {"level": "warning", "message": "告警信息"}}]'
                        style={{ fontFamily: 'monospace' }}
                      />
                    </Form.Item>
                  </Card>
                </>
              )}
            </Form>
          </Col>
          
          {formMode === 'json' && (
            <Col span={10}>
              <Card title="实时预览" size="small" style={{ position: 'sticky', top: 0 }}>
                <pre style={{
                  background: '#f5f5f5',
                  padding: 12,
                  borderRadius: 4,
                  fontSize: 12,
                  lineHeight: 1.4,
                  maxHeight: '60vh',
                  overflow: 'auto'
                }}>
                  {jsonPreview}
                </pre>
              </Card>
            </Col>
          )}
        </Row>
      </Modal>

      {/* 帮助抽屉 */}
      <RuleHelp 
        visible={helpVisible} 
        onClose={() => setHelpVisible(false)} 
      />

      {/* 规则模板选择 */}
      <RuleTemplates
        visible={templatesVisible}
        onClose={() => setTemplatesVisible(false)}
        onSelect={handleTemplateSelect}
      />

      {/* 数据类型选择器 */}
      <DataTypeSelector
        visible={dataTypeSelectorVisible}
        onTypeSelect={handleDataTypeSelect}
        onCancel={() => setDataTypeSelectorVisible(false)}
      />

      {/* 复合数据规则编辑器 */}
      <ComplexDataRuleEditor
        visible={complexRuleEditorVisible}
        dataType={selectedDataType}
        rule={selectedRule}
        onSave={handleComplexRuleSave}
        onClose={handleComplexRuleCancel}
      />

      {/* GPS/地理数据专用规则编辑器 */}
      <GeospatialRuleEditor
        visible={geospatialEditorVisible}
        rule={selectedRule}
        onSave={handleSpecializedRuleSave}
        onClose={handleSpecializedRuleCancel}
      />

      {/* 3D向量数据专用规则编辑器 */}
      <Vector3DRuleEditor
        visible={vector3dEditorVisible}
        rule={selectedRule}
        mode={isEditing ? 'edit' : 'create'}
        onSave={handleSpecializedRuleSave}
        onClose={() => setVector3dEditorVisible(false)}
      />

      {/* 通用向量数据专用规则编辑器 */}
      <GenericVectorRuleEditor
        visible={genericVectorEditorVisible}
        rule={selectedRule}
        mode={isEditing ? 'edit' : 'create'}
        onSave={handleSpecializedRuleSave}
        onClose={() => setGenericVectorEditorVisible(false)}
      />

      {/* 颜色数据专用规则编辑器 */}
      <VisualRuleEditor
        visible={visualEditorVisible}
        rule={selectedRule}
        onSave={handleSpecializedRuleSave}
        onClose={handleSpecializedRuleCancel}
      />
    </div>
  );
};

export default RulesPage; 