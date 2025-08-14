import React, { useState, useEffect } from 'react';
import { Modal, Card, Form, Input, Select, Space, Typography, Row, Col, Divider, Button, InputNumber, Switch, Tag, Tabs, Alert } from 'antd';
import { DatabaseOutlined, InfoCircleOutlined, FunctionOutlined, EyeOutlined, EditOutlined, CodeOutlined, CheckCircleOutlined } from '@ant-design/icons';
import { useBaseRuleEditor } from './base/BaseRuleEditor';
import ConditionBuilder from './base/ConditionBuilder';
import ActionFormBuilder from './base/ActionFormBuilder';
import ExpressionEditor from './base/ExpressionEditor';
import { Rule, Condition, Action } from '../types/rule';

const { Option } = Select;
const { Text } = Typography;
const { TextArea } = Input;

export interface ComplexDataRuleEditorProps {
  visible: boolean;
  onClose: () => void;
  onSave: (rule: Rule) => Promise<void>;
  rule?: Rule;
  dataType?: string | { key: string }; // array, matrix, timeseries, mixed等，或者数据类型对象
}

/**
 * 复杂数据规则编辑器
 * 处理数组、矩阵、时间序列、混合类型等复杂数据结构的规则
 */
const ComplexDataRuleEditor: React.FC<ComplexDataRuleEditorProps> = ({
  visible,
  onClose,
  onSave,
  rule,
  dataType = 'array'
}) => {
  // 智能推断数据类型
  const inferDataTypeFromRule = (rule: Rule): string => {
    // 1. 从rule的data_type字段推断
    if (rule.data_type) {
      return rule.data_type;
    }
    
    // 2. 从tags推断
    if (rule.tags) {
      const tagDataType = rule.tags['data_type'] || rule.tags['data_category'];
      if (tagDataType) {
        return tagDataType;
      }
    }
    
    // 3. 从条件字段推断
    if (rule.conditions) {
      const conditionStr = JSON.stringify(rule.conditions).toLowerCase();
      if (conditionStr.includes('array') || conditionStr.includes('length') || conditionStr.includes('size')) {
        return 'array';
      }
      if (conditionStr.includes('matrix') || conditionStr.includes('rows') || conditionStr.includes('cols')) {
        return 'matrix';
      }
      if (conditionStr.includes('timeseries') || conditionStr.includes('trend') || conditionStr.includes('seasonality')) {
        return 'timeseries';
      }
      if (conditionStr.includes('mixed') || conditionStr.includes('nested') || conditionStr.includes('field_count')) {
        return 'mixed';
      }
    }
    
    // 4. 从动作类型推断
    if (rule.actions && rule.actions.length > 0) {
      const action = rule.actions[0];
      if (action.type.includes('array')) return 'array';
      if (action.type.includes('matrix')) return 'matrix';
      if (action.type.includes('timeseries')) return 'timeseries';
    }
    
    return 'array'; // 默认值
  };
  
  // 确定最终的数据类型
  const finalDataType = (() => {
    if (rule && visible) {
      // 编辑模式：从规则数据智能推断
      return inferDataTypeFromRule(rule);
    } else if (typeof dataType === 'string') {
      // 创建模式：使用传入的字符串类型
      return dataType;
    } else if (dataType && typeof dataType === 'object' && 'key' in dataType) {
      // 创建模式：使用传入的数据类型对象
      return dataType.key;
    } else {
      // 默认值
      return 'array';
    }
  })();
  const {
    form,
    saving,
    validationErrors,
    handleSave,
    handleCancel: baseHandleCancel
  } = useBaseRuleEditor({
    visible,
    onClose,
    onSave,
    rule,
    title: '复杂数据规则编辑器',
    dataTypeName: getDataTypeDisplayName(finalDataType)
  });

  // 自定义的取消处理函数，重置所有状态
  const handleCancel = () => {
    // 重置所有本地状态
    setConditions(undefined);
    setActions([]);
    setActiveTab('basic');
    setJsonValue('');
    setJsonError('');
    
    // 调用基础的取消处理
    baseHandleCancel();
  };

  const [conditions, setConditions] = useState<Condition | undefined>(rule?.conditions);
  const [actions, setActions] = useState<Action[]>(rule?.actions || []);
  const [activeTab, setActiveTab] = useState<'basic' | 'conditions' | 'actions' | 'preview' | 'json'>('basic');
  const [jsonValue, setJsonValue] = useState<string>('');
  const [jsonError, setJsonError] = useState<string>('');

  useEffect(() => {
    if (visible && rule) {
      console.log('复杂数据编辑器加载规则数据:', rule);
      console.log('加载的动作配置:', rule.actions);
      setConditions(rule.conditions);
      setActions(rule.actions || []);
      updateJsonValue();
    }
  }, [visible, rule]);

  // 监听actions变化，同步更新JSON
  useEffect(() => {
    if (activeTab === 'json' || activeTab === 'preview') {
      updateJsonValue();
    }
  }, [actions, conditions, activeTab]);

  const updateJsonValue = () => {
    try {
      const currentRule = {
        id: rule?.id || '',
        name: form.getFieldValue('name') || '',
        description: form.getFieldValue('description') || '',
        priority: form.getFieldValue('priority') || 50,
        enabled: form.getFieldValue('enabled') !== false,
        data_type: finalDataType,
        conditions: conditions,
        actions: actions,
        tags: {
          ...rule?.tags,
          data_type: finalDataType,
          data_category: 'complex'
        },
        version: rule?.version || 1,
        created_at: rule?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      };
      setJsonValue(JSON.stringify(currentRule, null, 2));
      setJsonError('');
    } catch (error) {
      setJsonError('JSON生成失败');
    }
  };

  const handleJsonChange = (value: string) => {
    setJsonValue(value);
    try {
      const parsedRule = JSON.parse(value);
      // 验证基本字段
      if (parsedRule.name && parsedRule.conditions && parsedRule.actions) {
        setJsonError('');
        // 更新表单和状态
        form.setFieldsValue({
          name: parsedRule.name,
          description: parsedRule.description,
          priority: parsedRule.priority,
          enabled: parsedRule.enabled
        });
        setConditions(parsedRule.conditions);
        setActions(parsedRule.actions || []);
      } else {
        setJsonError('JSON格式不完整，缺少必要字段');
      }
    } catch (error) {
      setJsonError('JSON格式错误');
    }
  };

  function getDataTypeDisplayName(type: string): string {
    const typeMap: Record<string, string> = {
      'array': '数组数据',
      'matrix': '矩阵数据',
      'timeseries': '时间序列数据',
      'mixed': '混合类型数据',
      'json': 'JSON数据',
      'nested': '嵌套数据结构',
      'graph': '图数据结构',
      'tree': '树形数据结构'
    };
    return typeMap[type] || '复杂数据';
  }

  // 根据数据类型获取字段选项
  const getFieldsForDataType = (type: string) => {
    switch (type) {
      case 'array':
        return [
          { value: 'array.length', label: '数组长度', description: '数组元素的总数' },
          { value: 'array.size', label: '数组大小', description: '数组的字节大小' },
          { value: 'array.sum', label: '数组求和', description: '数组所有元素的和' },
          { value: 'array.mean', label: '数组均值', description: '数组元素的平均值' },
          { value: 'array.median', label: '数组中位数', description: '数组的中位数' },
          { value: 'array.min', label: '数组最小值', description: '数组中的最小元素' },
          { value: 'array.max', label: '数组最大值', description: '数组中的最大元素' },
          { value: 'array.std', label: '标准差', description: '数组元素的标准偏差' },
          { value: 'array.variance', label: '方差', description: '数组元素的方差' },
          { value: 'array[0]', label: '第一个元素', description: '数组的第一个元素' },
          { value: 'array[-1]', label: '最后一个元素', description: '数组的最后一个元素' },
          { value: 'array.type', label: '元素类型', description: '数组元素的数据类型' },
          { value: 'array.unique_count', label: '唯一元素数', description: '数组中唯一元素的数量' },
          { value: 'array.null_count', label: '空值数量', description: '数组中空值的数量' }
        ];
      
      case 'matrix':
        return [
          { value: 'matrix.rows', label: '矩阵行数', description: '矩阵的行数' },
          { value: 'matrix.cols', label: '矩阵列数', description: '矩阵的列数' },
          { value: 'matrix.size', label: '矩阵大小', description: '矩阵的总元素数' },
          { value: 'matrix.rank', label: '矩阵秩', description: '矩阵的秩' },
          { value: 'matrix.determinant', label: '行列式', description: '矩阵的行列式值' },
          { value: 'matrix.trace', label: '矩阵迹', description: '矩阵对角线元素的和' },
          { value: 'matrix.norm', label: '矩阵范数', description: '矩阵的范数' },
          { value: 'matrix.condition', label: '条件数', description: '矩阵的条件数' },
          { value: 'matrix.eigenvalues', label: '特征值', description: '矩阵的特征值' },
          { value: 'matrix.is_symmetric', label: '是否对称', description: '矩阵是否为对称矩阵' },
          { value: 'matrix.is_positive_definite', label: '是否正定', description: '矩阵是否为正定矩阵' },
          { value: 'matrix[0,0]', label: '第一个元素', description: '矩阵(0,0)位置的元素' },
          { value: 'matrix.sparsity', label: '稀疏度', description: '矩阵的稀疏程度' }
        ];

      case 'timeseries':
        return [
          { value: 'timeseries.length', label: '序列长度', description: '时间序列的数据点数量' },
          { value: 'timeseries.duration', label: '时间跨度', description: '时间序列的总时长' },
          { value: 'timeseries.frequency', label: '采样频率', description: '数据采样的频率' },
          { value: 'timeseries.interval', label: '采样间隔', description: '相邻数据点的时间间隔' },
          { value: 'timeseries.trend', label: '趋势方向', description: '时间序列的整体趋势' },
          { value: 'timeseries.seasonality', label: '季节性', description: '是否具有季节性模式' },
          { value: 'timeseries.volatility', label: '波动性', description: '时间序列的波动程度' },
          { value: 'timeseries.autocorr', label: '自相关性', description: '时间序列的自相关系数' },
          { value: 'timeseries.stationarity', label: '平稳性', description: '时间序列的平稳性' },
          { value: 'timeseries.mean', label: '序列均值', description: '时间序列的平均值' },
          { value: 'timeseries.std', label: '序列标准差', description: '时间序列的标准偏差' },
          { value: 'timeseries.missing_rate', label: '缺失率', description: '缺失数据点的比例' },
          { value: 'timeseries.outlier_count', label: '异常点数', description: '时间序列中异常点的数量' }
        ];

      case 'mixed':
        return [
          { value: 'data.structure_type', label: '数据结构类型', description: '混合数据的主要结构类型' },
          { value: 'data.complexity', label: '复杂度', description: '数据结构的复杂程度' },
          { value: 'data.nested_levels', label: '嵌套层级', description: '数据嵌套的最大层数' },
          { value: 'data.field_count', label: '字段数量', description: '数据中字段的总数' },
          { value: 'data.type_diversity', label: '类型多样性', description: '包含的数据类型种类数' },
          { value: 'data.size_bytes', label: '数据大小', description: '数据的字节大小' },
          { value: 'data.null_fields', label: '空字段数', description: '值为空的字段数量' },
          { value: 'data.array_fields', label: '数组字段数', description: '包含的数组类型字段数' },
          { value: 'data.object_fields', label: '对象字段数', description: '包含的对象类型字段数' }
        ];

      default:
        return [
          { value: 'data.size', label: '数据大小', description: '数据结构的大小' },
          { value: 'data.type', label: '数据类型', description: '数据的类型标识' },
          { value: 'data.complexity', label: '复杂度', description: '数据结构的复杂程度' }
        ];
    }
  };

  // 根据数据类型获取动作选项
  const getActionsForDataType = (type: string) => {
    switch (type) {
      case 'array':
        return [
          {
            value: 'array_transform',
            label: '数组转换',
            description: '数组统计、排序、筛选等操作',
            configSchema: {
              transform_type: {
                type: 'string' as const,
                label: '转换类型',
                required: true,
                options: [
                  { value: 'statistics', label: '统计计算' },
                  { value: 'sort', label: '数组排序' },
                  { value: 'filter', label: '数组筛选' },
                  { value: 'slice', label: '数组切片' },
                  { value: 'reshape', label: '数组重塑' },
                  { value: 'normalize', label: '数据标准化' },
                  { value: 'aggregate', label: '聚合操作' },
                  { value: 'deduplication', label: '去重处理' }
                ]
              },
              functions: {
                type: 'array' as const,
                label: '统计函数',
                description: '选择要计算的统计函数',
                options: [
                  { value: 'mean', label: '平均值' },
                  { value: 'median', label: '中位数' },
                  { value: 'std', label: '标准差' },
                  { value: 'var', label: '方差' },
                  { value: 'min', label: '最小值' },
                  { value: 'max', label: '最大值' },
                  { value: 'sum', label: '求和' },
                  { value: 'count', label: '计数' }
                ]
              },
              start_index: {
                type: 'number' as const,
                label: '开始索引',
                description: '切片操作的开始位置'
              },
              end_index: {
                type: 'number' as const,
                label: '结束索引',
                description: '切片操作的结束位置'
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '转换结果的字段名',
                description: '转换结果存储的字段名'
              }
            }
          },
          {
            value: 'array_expression',
            label: '数组表达式操作',
            description: '使用表达式对数组数据进行复杂计算和处理',
            configSchema: {
              expression: {
                type: 'textarea' as const,
                label: '表达式',
                required: true,
                placeholder: 'arraySum(data_array) / arrayMean(data_array)',
                description: '支持数组函数和数学运算的表达式'
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '结果存储的字段名',
                description: '表达式计算结果存储的字段名'
              },
              output_type: {
                type: 'string' as const,
                label: '输出类型',
                options: [
                  { value: 'number', label: '数值' },
                  { value: 'array', label: '数组' },
                  { value: 'boolean', label: '布尔值' },
                  { value: 'string', label: '字符串' }
                ],
                defaultValue: 'number'
              }
            }
          }
        ];

      case 'matrix':
        return [
          {
            value: 'matrix_transform',
            label: '矩阵转换',
            description: '矩阵运算、分解、变换等操作',
            configSchema: {
              transform_type: {
                type: 'string' as const,
                label: '转换类型',
                required: true,
                options: [
                  { value: 'transpose', label: '矩阵转置' },
                  { value: 'inverse', label: '矩阵求逆' },
                  { value: 'multiply', label: '矩阵乘法' },
                  { value: 'decomposition', label: '矩阵分解' },
                  { value: 'eigendecomp', label: '特征分解' },
                  { value: 'svd', label: '奇异值分解' },
                  { value: 'flatten', label: '矩阵展平' },
                  { value: 'reshape', label: '矩阵重塑' }
                ]
              },
              decomp_method: {
                type: 'string' as const,
                label: '分解方法',
                options: [
                  { value: 'lu', label: 'LU分解' },
                  { value: 'qr', label: 'QR分解' },
                  { value: 'cholesky', label: 'Cholesky分解' },
                  { value: 'eigen', label: '特征分解' },
                  { value: 'svd', label: '奇异值分解' }
                ]
              },
              target_shape: {
                type: 'string' as const,
                label: '目标形状',
                placeholder: '例如：[3, 4]',
                description: '重塑后的矩阵形状'
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '转换结果的字段名',
                description: '矩阵操作结果的字段名'
              }
            }
          },
          {
            value: 'matrix_expression',
            label: '矩阵表达式操作',
            description: '使用表达式对矩阵数据进行复杂计算和处理',
            configSchema: {
              expression: {
                type: 'textarea' as const,
                label: '表达式',
                required: true,
                placeholder: 'matrixDeterminant(data_matrix) * matrixTrace(data_matrix)',
                description: '支持矩阵函数和数学运算的表达式'
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '结果存储的字段名',
                description: '表达式计算结果存储的字段名'
              },
              output_type: {
                type: 'string' as const,
                label: '输出类型',
                options: [
                  { value: 'number', label: '数值' },
                  { value: 'matrix', label: '矩阵' },
                  { value: 'array', label: '数组' },
                  { value: 'boolean', label: '布尔值' }
                ],
                defaultValue: 'number'
              }
            }
          }
        ];

      case 'timeseries':
        return [
          {
            value: 'timeseries_transform',
            label: '时间序列转换',
            description: '时间序列分析、预测、特征提取',
            configSchema: {
              transform_type: {
                type: 'string' as const,
                label: '转换类型',
                required: true,
                options: [
                  { value: 'trend_analysis', label: '趋势分析' },
                  { value: 'seasonality_decomp', label: '季节性分解' },
                  { value: 'smoothing', label: '数据平滑' },
                  { value: 'differencing', label: '差分处理' },
                  { value: 'resampling', label: '重采样' },
                  { value: 'interpolation', label: '数据插值' },
                  { value: 'outlier_detection', label: '异常检测' },
                  { value: 'feature_extraction', label: '特征提取' }
                ]
              },
              window_size: {
                type: 'number' as const,
                label: '窗口大小',
                description: '分析窗口的大小'
              },
              method: {
                type: 'string' as const,
                label: '处理方法',
                options: [
                  { value: 'moving_average', label: '移动平均' },
                  { value: 'exponential_smoothing', label: '指数平滑' },
                  { value: 'linear_interpolation', label: '线性插值' },
                  { value: 'spline_interpolation', label: '样条插值' }
                ]
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '转换结果的字段名',
                description: '时间序列分析结果的字段名'
              }
            }
          },
          {
            value: 'timeseries_expression',
            label: '时序表达式操作',
            description: '使用表达式对时间序列数据进行复杂计算和处理',
            configSchema: {
              expression: {
                type: 'textarea' as const,
                label: '表达式',
                required: true,
                placeholder: 'trendDirection(ts_data) == "increasing" && volatility(ts_data) < 0.1',
                description: '支持时序函数和数学运算的表达式'
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '结果存储的字段名',
                description: '表达式计算结果存储的字段名'
              },
              output_type: {
                type: 'string' as const,
                label: '输出类型',
                options: [
                  { value: 'number', label: '数值' },
                  { value: 'timeseries', label: '时间序列' },
                  { value: 'boolean', label: '布尔值' },
                  { value: 'string', label: '字符串' }
                ],
                defaultValue: 'number'
              }
            }
          }
        ];

      case 'mixed':
        return [
          {
            value: 'data_transform',
            label: '数据转换',
            description: '混合数据的解析、转换、重构',
            configSchema: {
              transform_type: {
                type: 'string' as const,
                label: '转换类型',
                required: true,
                options: [
                  { value: 'flatten', label: '数据展平' },
                  { value: 'normalize', label: '数据标准化' },
                  { value: 'extract_fields', label: '字段提取' },
                  { value: 'type_conversion', label: '类型转换' },
                  { value: 'structure_analysis', label: '结构分析' },
                  { value: 'validation', label: '数据验证' }
                ]
              },
              extract_paths: {
                type: 'array' as const,
                label: '提取路径',
                description: 'JSON路径表达式，如$.data.array[*]'
              },
              target_type: {
                type: 'string' as const,
                label: '目标类型',
                options: [
                  { value: 'array', label: '数组' },
                  { value: 'object', label: '对象' },
                  { value: 'string', label: '字符串' },
                  { value: 'number', label: '数字' }
                ]
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '转换结果的字段名',
                description: '数据转换结果的字段名'
              }
            }
          },
          {
            value: 'mixed_expression',
            label: '混合数据表达式操作',
            description: '使用表达式对混合数据进行复杂计算和处理',
            configSchema: {
              expression: {
                type: 'textarea' as const,
                label: '表达式',
                required: true,
                placeholder: 'fieldCount(json_data) > 5 && hasField(json_data, "timestamp")',
                description: '支持结构函数和数学运算的表达式'
              },
              output_key: {
                type: 'string' as const,
                label: '输出字段',
                placeholder: '结果存储的字段名',
                description: '表达式计算结果存储的字段名'
              },
              output_type: {
                type: 'string' as const,
                label: '输出类型',
                options: [
                  { value: 'number', label: '数值' },
                  { value: 'object', label: '对象' },
                  { value: 'array', label: '数组' },
                  { value: 'boolean', label: '布尔值' },
                  { value: 'string', label: '字符串' }
                ],
                defaultValue: 'boolean'
              }
            }
          }
        ];

      default:
        return [];
    }
  };

  // 根据数据类型获取表达式函数
  const getFunctionsForDataType = (type: string) => {
    const commonFunctions = [
      {
        name: 'size',
        description: '获取数据大小',
        syntax: 'size(data)',
        example: 'size(array) > 10',
        category: '通用函数',
        parameters: ['data']
      },
      {
        name: 'type',
        description: '获取数据类型',
        syntax: 'type(data)',
        example: 'type(data) == "array"',
        category: '通用函数',
        parameters: ['data']
      }
    ];

    switch (type) {
      case 'array':
        return [
          ...commonFunctions,
          {
            name: 'arraySum',
            description: '数组求和',
            syntax: 'arraySum(array)',
            example: 'arraySum(data_array) > 100',
            category: '数组函数',
            parameters: ['array']
          },
          {
            name: 'arrayMean',
            description: '数组平均值',
            syntax: 'arrayMean(array)',
            example: 'arrayMean(data_array) > 10.5',
            category: '数组函数',
            parameters: ['array']
          },
          {
            name: 'arrayMax',
            description: '数组最大值',
            syntax: 'arrayMax(array)',
            example: 'arrayMax(data_array) < 50',
            category: '数组函数',
            parameters: ['array']
          },
          {
            name: 'arrayMin',
            description: '数组最小值',
            syntax: 'arrayMin(array)',
            example: 'arrayMin(data_array) > 0',
            category: '数组函数',
            parameters: ['array']
          },
          {
            name: 'arrayStd',
            description: '数组标准差',
            syntax: 'arrayStd(array)',
            example: 'arrayStd(data_array) > 2.0',
            category: '数组函数',
            parameters: ['array']
          }
        ];

      case 'matrix':
        return [
          ...commonFunctions,
          {
            name: 'matrixRank',
            description: '矩阵秩',
            syntax: 'matrixRank(matrix)',
            example: 'matrixRank(data_matrix) == 3',
            category: '矩阵函数',
            parameters: ['matrix']
          },
          {
            name: 'matrixDeterminant',
            description: '矩阵行列式',
            syntax: 'matrixDeterminant(matrix)',
            example: 'matrixDeterminant(data_matrix) != 0',
            category: '矩阵函数',
            parameters: ['matrix']
          },
          {
            name: 'matrixTrace',
            description: '矩阵迹',
            syntax: 'matrixTrace(matrix)',
            example: 'matrixTrace(data_matrix) > 0',
            category: '矩阵函数',
            parameters: ['matrix']
          },
          {
            name: 'matrixNorm',
            description: '矩阵范数',
            syntax: 'matrixNorm(matrix, type)',
            example: 'matrixNorm(data_matrix, "frobenius") > 1.0',
            category: '矩阵函数',
            parameters: ['matrix', 'type']
          }
        ];

      case 'timeseries':
        return [
          ...commonFunctions,
          {
            name: 'trendDirection',
            description: '趋势方向',
            syntax: 'trendDirection(timeseries)',
            example: 'trendDirection(ts_data) == "increasing"',
            category: '时序函数',
            parameters: ['timeseries']
          },
          {
            name: 'seasonality',
            description: '季节性检测',
            syntax: 'seasonality(timeseries, period)',
            example: 'seasonality(ts_data, 24) > 0.5',
            category: '时序函数',
            parameters: ['timeseries', 'period']
          },
          {
            name: 'volatility',
            description: '波动性计算',
            syntax: 'volatility(timeseries)',
            example: 'volatility(ts_data) > 0.1',
            category: '时序函数',
            parameters: ['timeseries']
          },
          {
            name: 'autocorrelation',
            description: '自相关性',
            syntax: 'autocorrelation(timeseries, lag)',
            example: 'autocorrelation(ts_data, 1) > 0.8',
            category: '时序函数',
            parameters: ['timeseries', 'lag']
          }
        ];

      case 'mixed':
        return [
          ...commonFunctions,
          {
            name: 'fieldCount',
            description: '字段数量',
            syntax: 'fieldCount(data)',
            example: 'fieldCount(json_data) > 10',
            category: '结构函数',
            parameters: ['data']
          },
          {
            name: 'nestedLevels',
            description: '嵌套层级',
            syntax: 'nestedLevels(data)',
            example: 'nestedLevels(nested_data) <= 5',
            category: '结构函数',
            parameters: ['data']
          },
          {
            name: 'hasField',
            description: '字段存在检查',
            syntax: 'hasField(data, fieldPath)',
            example: 'hasField(json_data, "user.profile.name")',
            category: '结构函数',
            parameters: ['data', 'fieldPath']
          },
          {
            name: 'extractField',
            description: '提取字段值',
            syntax: 'extractField(data, fieldPath)',
            example: 'extractField(json_data, "$.items[0].price") > 100',
            category: '结构函数',
            parameters: ['data', 'fieldPath']
          }
        ];

      default:
        return commonFunctions;
    }
  };

  const handleSaveClick = async () => {
    updateJsonValue(); // 更新JSON预览
    await handleSave(
      () => conditions,
      () => actions
    );
  };

  // 处理tab切换时的逻辑
  const handleTabChange = (key: string) => {
    setActiveTab(key as 'basic' | 'conditions' | 'actions' | 'preview' | 'json');
    // 当切换到JSON编辑或预览tab时，立即更新JSON内容
    if (key === 'json' || key === 'preview') {
      setTimeout(updateJsonValue, 50);
    }
  };

  // 监听表单变化，更新JSON
  const handleFormChange = () => {
    // 如果当前在JSON编辑或预览tab，立即更新JSON
    if (activeTab === 'json' || activeTab === 'preview') {
      setTimeout(updateJsonValue, 100);
    }
  };

  const renderDataTypeConfig = () => (
    <Card title={<Space><DatabaseOutlined />复杂数据配置</Space>} style={{ marginTop: 16 }}>
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Form.Item label="数据类型">
            <Select value={finalDataType} disabled>
              <Option value="array">数组 (Array)</Option>
              <Option value="matrix">矩阵 (Matrix)</Option>
              <Option value="timeseries">时间序列 (TimeSeries)</Option>
              <Option value="mixed">混合类型 (Mixed)</Option>
              <Option value="json">JSON数据</Option>
              <Option value="nested">嵌套结构</Option>
            </Select>
          </Form.Item>
        </Col>
      </Row>

      {finalDataType === 'array' && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col span={12}>
            <Form.Item label="预期数组大小">
              <InputNumber
                placeholder="元素数量"
                style={{ width: '100%' }}
                min={0}
                max={1000000}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item label="元素类型">
              <Select defaultValue="number" placeholder="数组元素的数据类型">
                <Option value="number">数值类型</Option>
                <Option value="string">字符串类型</Option>
                <Option value="boolean">布尔类型</Option>
                <Option value="object">对象类型</Option>
                <Option value="mixed">混合类型</Option>
              </Select>
            </Form.Item>
          </Col>
        </Row>
      )}

      {finalDataType === 'matrix' && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col span={8}>
            <Form.Item label="矩阵类型">
              <Select defaultValue="dense" placeholder="矩阵存储类型">
                <Option value="dense">稠密矩阵</Option>
                <Option value="sparse">稀疏矩阵</Option>
                <Option value="symmetric">对称矩阵</Option>
                <Option value="diagonal">对角矩阵</Option>
              </Select>
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item label="预期维度">
              <Input placeholder="例如：3x4, 100x100" />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item label="数值精度">
              <Select defaultValue="float64" placeholder="数值精度">
                <Option value="int32">32位整数</Option>
                <Option value="float32">32位浮点</Option>
                <Option value="float64">64位浮点</Option>
                <Option value="complex">复数</Option>
              </Select>
            </Form.Item>
          </Col>
        </Row>
      )}

      {finalDataType === 'timeseries' && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col span={8}>
            <Form.Item label="时间粒度">
              <Select defaultValue="second" placeholder="时间序列的粒度">
                <Option value="millisecond">毫秒</Option>
                <Option value="second">秒</Option>
                <Option value="minute">分钟</Option>
                <Option value="hour">小时</Option>
                <Option value="day">天</Option>
              </Select>
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item label="预期长度">
              <InputNumber
                placeholder="数据点数量"
                style={{ width: '100%' }}
                min={1}
                max={10000000}
              />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item 
              label="是否实时"
              style={{ 
                display: 'flex', 
                alignItems: 'center',
                height: '40px' 
              }}
            >
              <Switch defaultChecked checkedChildren="实时" unCheckedChildren="离线" />
            </Form.Item>
          </Col>
        </Row>
      )}

      <div style={{ marginTop: 16 }}>
        <Text type="secondary">
          <InfoCircleOutlined /> {getDataTypeDisplayName(finalDataType)}支持复杂的数据结构分析和处理
        </Text>
      </div>
    </Card>
  );

  const renderConditionsSection = () => (
    <Card title={`${getDataTypeDisplayName(finalDataType)}条件`} style={{ marginTop: 16 }}>
      <ConditionBuilder
        value={conditions}
        onChange={setConditions}
        availableFields={['device_id', 'key', 'value', 'timestamp']}
        customFieldOptions={getFieldsForDataType(finalDataType)}
        allowedOperators={['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'exists', 'regex']}
        supportExpressions={true}
        dataTypeName={getDataTypeDisplayName(finalDataType)}
      />
      
      <div style={{ marginTop: 16 }}>
        <ExpressionEditor
          value={conditions?.type === 'expression' ? conditions.expression : ''}
          onChange={(expr) => setConditions({ type: 'expression', expression: expr })}
          dataType={finalDataType}
          availableFunctions={getFunctionsForDataType(finalDataType)}
          availableVariables={getFieldsForDataType(finalDataType).map(f => ({
            name: f.value,
            description: f.description,
            type: 'number',
            example: `${f.value} > 0`
          }))}
          placeholder={`输入${getDataTypeDisplayName(finalDataType)}表达式`}
          rows={3}
        />
      </div>
    </Card>
  );

  const renderActionsSection = () => (
    <Card title={`${getDataTypeDisplayName(finalDataType)}动作`} style={{ marginTop: 16 }}>
      <ActionFormBuilder
        value={actions}
        onChange={setActions}
        availableActionTypes={[]} // 移除普通动作类型，只使用专门的复合数据动作
        customActionOptions={getActionsForDataType(finalDataType)}
        dataTypeName={getDataTypeDisplayName(finalDataType)}
      />
    </Card>
  );

  const tabItems = [
    {
      key: 'basic',
      label: <Space><InfoCircleOutlined />基本信息</Space>,
      children: (
        <Card title="基本信息" style={{ border: 'none', boxShadow: 'none' }}>
          <Form 
            form={form}
            layout="vertical"
            initialValues={{
              priority: rule?.priority || 50,
              enabled: rule?.enabled !== false
            }}
            onValuesChange={handleFormChange}
          >
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label="规则名称"
                  name="name"
                  rules={[{ required: true, message: '请输入规则名称' }]}
                >
                  <Input placeholder={`请输入${getDataTypeDisplayName(finalDataType)}规则名称`} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label="优先级"
                  name="priority"
                  rules={[
                    { required: true, message: '请输入优先级' },
                    { type: 'number', min: 0, max: 100, message: '优先级必须在0-100之间' }
                  ]}
                >
                  <InputNumber min={0} max={100} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            
            <Form.Item
              label="规则描述"
              name="description"
            >
              <Input.TextArea 
                placeholder={`请输入${getDataTypeDisplayName(finalDataType)}规则描述`} 
                rows={3}
              />
            </Form.Item>
            
            <Form.Item
              label="启用状态"
              name="enabled"
              valuePropName="checked"
              style={{ 
                display: 'flex', 
                alignItems: 'center',
                height: '40px' 
              }}
            >
              <Switch checkedChildren="启用" unCheckedChildren="禁用" />
            </Form.Item>
          </Form>
          
          {/* 数据类型特定配置 */}
          {renderDataTypeConfig()}
        </Card>
      )
    },
    {
      key: 'conditions',
      label: <Space><DatabaseOutlined />复杂条件</Space>,
      children: renderConditionsSection()
    },
    {
      key: 'actions',
      label: <Space><CheckCircleOutlined />执行动作</Space>,
      children: renderActionsSection()
    },
    {
      key: 'preview',
      label: <Space><EyeOutlined />预览</Space>,
      children: (
        <Card title="规则预览" style={{ border: 'none', boxShadow: 'none' }}>
          <Alert
            message="规则JSON配置"
            description="以下是当前规则的完整JSON配置预览"
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
          />
          <pre
            style={{
              background: '#f5f5f5',
              padding: '16px',
              borderRadius: '6px',
              fontSize: '12px',
              lineHeight: '1.5',
              maxHeight: '400px',
              overflow: 'auto',
              border: '1px solid #d9d9d9'
            }}
          >
            {jsonValue}
          </pre>
        </Card>
      )
    },
    {
      key: 'json',
      label: <Space><CodeOutlined />JSON编辑</Space>,
      children: (
        <Card title="JSON直接编辑" style={{ border: 'none', boxShadow: 'none' }}>
          <Alert
            message="JSON编辑模式"
            description="可以直接编辑JSON配置，保存时会自动同步到表单"
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
          {jsonError && (
            <Alert
              message="JSON格式错误"
              description={jsonError}
              type="error"
              showIcon
              style={{ marginBottom: 16 }}
            />
          )}
          <TextArea
            value={jsonValue}
            onChange={(e) => handleJsonChange(e.target.value)}
            rows={20}
            style={{
              fontFamily: 'Monaco, Consolas, "Courier New", monospace',
              fontSize: '12px',
              lineHeight: '1.5'
            }}
            placeholder="输入或编辑JSON格式的规则配置..."
          />
          <div style={{ marginTop: 12 }}>
            <Space>
              <Button 
                size="small" 
                onClick={updateJsonValue}
                icon={<EditOutlined />}
              >
                从表单更新JSON
              </Button>
              <Button 
                size="small" 
                type="primary"
                ghost
                onClick={() => {
                  try {
                    const formatted = JSON.stringify(JSON.parse(jsonValue), null, 2);
                    setJsonValue(formatted);
                  } catch (error) {
                    // JSON格式无效，不执行格式化
                  }
                }}
                icon={<CodeOutlined />}
              >
                格式化JSON
              </Button>
            </Space>
          </div>
        </Card>
      )
    }
  ];

  return (
    <Modal
      title={
        <Space>
          <DatabaseOutlined style={{ color: '#1890ff' }} />
          <span>复杂数据规则编辑器</span>
          <Tag color="blue" icon={<FunctionOutlined />}>
            {getDataTypeDisplayName(finalDataType)}
          </Tag>
        </Space>
      }
      open={visible}
      onCancel={handleCancel}
      footer={
        <Space>
          <Button onClick={handleCancel} size="large">取消</Button>
          <Button 
            type="primary" 
            loading={saving}
            onClick={handleSaveClick}
            size="large"
            icon={<CheckCircleOutlined />}
          >
            保存规则
          </Button>
        </Space>
      }
      width={1200}
      centered
      destroyOnHidden
      style={{ 
        maxHeight: '90vh',
        top: 0
      }}
      styles={{
        body: { 
          padding: '20px',
          maxHeight: 'calc(90vh - 140px)', // 减去header和footer的高度
          overflowY: 'auto',
          overflowX: 'hidden'
        },
        header: { borderBottom: '1px solid #f0f0f0', paddingBottom: '16px' },
        content: {
          maxHeight: '90vh',
          display: 'flex',
          flexDirection: 'column'
        }
      }}
    >
      {/* 验证错误显示 */}
      {validationErrors.length > 0 && (
        <Alert
          message="验证错误"
          description={
            <ul style={{ margin: 0, paddingLeft: '20px' }}>
              {validationErrors.map((error, index) => (
                <li key={index}>{error}</li>
              ))}
            </ul>
          }
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          closable
        />
      )}

      <Tabs
        activeKey={activeTab}
        onChange={handleTabChange}
        items={tabItems}
        size="large"
      />
    </Modal>
  );
};

export default ComplexDataRuleEditor;