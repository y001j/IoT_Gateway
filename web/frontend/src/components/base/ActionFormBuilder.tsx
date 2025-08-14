import React, { useState, useEffect } from 'react';
import { Card, Form, Select, Input, InputNumber, Button, Space, Switch, Typography, Row, Col, Divider, Alert, Tag, Tooltip } from 'antd';
import { PlusOutlined, DeleteOutlined, BellOutlined, SwapOutlined, FilterOutlined, FunctionOutlined, SendOutlined, InfoCircleOutlined, SettingOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { Action } from '../../types/rule';

const { Option } = Select;
const { Text } = Typography;
const { TextArea } = Input;

export interface ActionFormBuilderProps {
  value?: Action[];
  onChange?: (actions: Action[]) => void;
  availableActionTypes?: string[];
  customActionOptions?: Array<{ 
    value: string; 
    label: string; 
    description?: string;
    configSchema?: ActionConfigSchema;
  }>;
  dataTypeName?: string;
}

interface ActionConfigSchema {
  [key: string]: {
    type: 'string' | 'number' | 'boolean' | 'object' | 'array' | 'textarea';
    label: string;
    required?: boolean;
    placeholder?: string;
    description?: string;
    defaultValue?: any;
    options?: Array<{ value: any; label: string }>;
    conditionalDisplay?: {
      dependsOn: string;
      showWhen: any[];
    };
  };
}

/**
 * 通用动作表单构建器组件
 * 支持多种动作类型的配置表单
 */
const ActionFormBuilder: React.FC<ActionFormBuilderProps> = ({
  value = [],
  onChange,
  availableActionTypes = ['alert', 'transform', 'filter', 'aggregate', 'forward'],
  customActionOptions = [],
  dataTypeName = '数据'
}) => {
  const [actions, setActions] = useState<Action[]>(value);


  // 基础动作类型定义
  const baseActionTypes = [
    {
      value: 'alert',
      label: '告警动作',
      description: '发送告警通知',
      configSchema: {
        level: {
          type: 'string' as const,
          label: '告警级别',
          required: true,
          options: [
            { value: 'info', label: '信息' },
            { value: 'warning', label: '警告' },
            { value: 'error', label: '错误' },
            { value: 'critical', label: '严重' }
          ],
          defaultValue: 'warning'
        },
        message: {
          type: 'textarea' as const,
          label: '告警消息',
          required: true,
          placeholder: '告警消息模板，支持变量：{{.DeviceID}}, {{.Value}}',
          description: '支持模板变量，如 {{.DeviceID}}, {{.Value}}, {{.Timestamp}}'
        },
        type: {
          type: 'string' as const,
          label: '通知类型',
          options: [
            { value: 'console', label: '控制台' },
            { value: 'webhook', label: 'Webhook' },
            { value: 'email', label: '邮件' },
            { value: 'sms', label: '短信' }
          ],
          defaultValue: 'console'
        }
      }
    },
    {
      value: 'transform',
      label: '数据转换',
      description: '转换或修改数据',
      configSchema: {
        output_key: {
          type: 'string' as const,
          label: '输出字段',
          placeholder: '转换后的字段名',
          description: '转换结果存储的字段名'
        },
        output_type: {
          type: 'string' as const,
          label: '输出类型',
          options: [
            { value: 'float', label: '浮点数' },
            { value: 'int', label: '整数' },
            { value: 'string', label: '字符串' },
            { value: 'boolean', label: '布尔值' }
          ],
          defaultValue: 'float'
        },
        type: {
          type: 'string' as const,
          label: '转换类型',
          required: true,
          options: [
            { value: 'scale', label: '数值缩放' },
            { value: 'offset', label: '数值偏移' },
            { value: 'expression', label: '表达式计算' },
            { value: 'lookup', label: '查找表映射' },
            { value: 'format', label: '格式化' }
          ]
        }
      }
    },
    {
      value: 'filter',
      label: '数据过滤',
      description: '过滤或筛选数据',
      configSchema: {
        type: {
          type: 'string' as const,
          label: '过滤类型',
          required: true,
          options: [
            { value: 'range', label: '范围过滤' },
            { value: 'pattern', label: '模式匹配' },
            { value: 'deduplication', label: '去重过滤' },
            { value: 'rate_limit', label: '速率限制' }
          ]
        },
        drop_on_match: {
          type: 'boolean' as const,
          label: '匹配时丢弃',
          description: 'true: 匹配条件时丢弃数据，false: 不匹配时丢弃',
          defaultValue: false
        }
      }
    },
    {
      value: 'aggregate',
      label: '数据聚合',
      description: '统计计算和聚合',
      configSchema: {
        functions: {
          type: 'array' as const,
          label: '聚合函数',
          required: true,
          description: '选择要计算的统计函数',
          options: [
            { value: 'count', label: '计数' },
            { value: 'sum', label: '求和' },
            { value: 'avg', label: '平均值' },
            { value: 'min', label: '最小值' },
            { value: 'max', label: '最大值' },
            { value: 'stddev', label: '标准差' },
            { value: 'p90', label: '90分位数' },
            { value: 'p95', label: '95分位数' }
          ]
        },
        window_type: {
          type: 'string' as const,
          label: '窗口类型',
          required: true,
          options: [
            { value: 'count', label: '计数窗口' },
            { value: 'time', label: '时间窗口' },
            { value: 'sliding', label: '滑动窗口' }
          ]
        },
        window_size: {
          type: 'number' as const,
          label: '窗口大小',
          placeholder: '窗口大小（数量或秒数）'
        }
      }
    },
    {
      value: 'forward',
      label: '数据转发',
      description: '转发数据到其他主题',
      configSchema: {
        subject: {
          type: 'string' as const,
          label: '目标主题',
          required: true,
          placeholder: 'iot.processed.{{device_id}}',
          description: '支持模板变量，如 {{device_id}}, {{key}}'
        }
      }
    }
  ];

  // 动作类型映射 - 将旧的通用动作类型映射到专用动作类型
  const actionTypeMapping: { [key: string]: { [key: string]: string } } = {
    // 视觉数据映射
    visual: {
      'alert': 'visual_alert',
      'transform': 'color_transform'
    },
    // 向量数据映射
    vector_generic: {
      'alert': 'generic_vector_alert', 
      'transform': 'generic_vector_transform'
    },
    // 地理数据映射
    geospatial: {
      'alert': 'geospatial_alert',
      'transform': 'geospatial_transform'
    },
    // 3D向量数据映射
    vector3d: {
      'alert': 'vector3d_alert',
      'transform': 'vector3d_transform'
    }
  };

  // 推断当前数据类型
  const inferDataType = (): string => {
    // 基于 customActionOptions 推断数据类型
    if (customActionOptions.length > 0) {
      const firstCustomAction = customActionOptions[0].value;
      if (firstCustomAction.includes('color') || firstCustomAction.includes('visual')) {
        return 'visual';
      }
      if (firstCustomAction.includes('vector') && firstCustomAction.includes('generic')) {
        return 'vector_generic';
      }
      if (firstCustomAction.includes('geospatial')) {
        return 'geospatial';
      }
      if (firstCustomAction.includes('vector3d')) {
        return 'vector3d';
      }
    }
    
    // 基于 dataTypeName 推断
    const dataType = dataTypeName?.toLowerCase() || '';
    if (dataType.includes('视觉') || dataType.includes('visual') || dataType.includes('颜色') || dataType.includes('color')) {
      return 'visual';
    }
    if (dataType.includes('通用向量') || dataType.includes('generic') || dataType.includes('vector')) {
      return 'vector_generic';
    }
    if (dataType.includes('地理') || dataType.includes('geospatial') || dataType.includes('位置') || dataType.includes('location')) {
      return 'geospatial';
    }
    if (dataType.includes('3d') || dataType.includes('向量3d') || dataType.includes('vector3d')) {
      return 'vector3d';
    }
    
    return 'unknown';
  };

  const currentDataType = inferDataType();

  // 创建一个稳定的动作类型转换函数
  const convertActionType = React.useCallback((actionType: string): string => {
    // 如果动作类型已经是专用类型，直接返回
    if (customActionOptions.some(option => option.value === actionType)) {
      return actionType;
    }
    
    // 尝试映射旧的通用动作类型到新的专用动作类型
    const mapping = actionTypeMapping[currentDataType];
    if (mapping && mapping[actionType]) {
      const mappedType = mapping[actionType];
      console.log(`ActionFormBuilder: 动作类型映射 "${actionType}" -> "${mappedType}" (数据类型: ${currentDataType})`);
      return mappedType;
    }
    
    return actionType;
  }, [customActionOptions, currentDataType]);

  // 合并动作类型选项
  const allActionTypes = [
    ...baseActionTypes.filter(action => availableActionTypes.includes(action.value)),
    ...customActionOptions
  ];

  // 处理传入的动作数据，应用类型转换
  useEffect(() => {
    console.log('ActionFormBuilder接收到新的value:', value);
    
    // 对传入的动作进行类型转换
    const convertedActions = value.map(action => {
      const convertedType = convertActionType(action.type);
      if (convertedType !== action.type) {
        console.log(`ActionFormBuilder: 转换动作类型 "${action.type}" -> "${convertedType}"`);
        return { ...action, type: convertedType };
      }
      return action;
    });
    
    setActions(convertedActions);
    
    // 使用异步方式通知父组件，避免在渲染期间更新状态
    if (convertedActions.some((action, index) => action.type !== value[index]?.type)) {
      console.log('ActionFormBuilder: 动作类型已转换，异步通知父组件');
      setTimeout(() => {
        onChange?.(convertedActions);
      }, 0);
    }
  }, [value, convertActionType]);

  const updateActions = (newActions: Action[]) => {
    console.log('ActionFormBuilder updateActions:', newActions);
    setActions(newActions);
    onChange?.(newActions);
  };

  const addAction = () => {
    const newAction: Action = {
      type: 'alert',
      config: {},
      async: false,
      timeout: '0s',
      retry: 0
    };
    updateActions([...actions, newAction]);
  };

  const removeAction = (index: number) => {
    const newActions = actions.filter((_, i) => i !== index);
    updateActions(newActions);
  };

  const updateAction = (index: number, updatedAction: Action) => {
    const newActions = [...actions];
    newActions[index] = updatedAction;
    updateActions(newActions);
  };

  const renderConfigField = (fieldKey: string, fieldSchema: ActionConfigSchema[string], value: any, onChange: (value: any) => void) => {
    const { type, label, required, placeholder, description, options, defaultValue } = fieldSchema;
    
    // 调试日志
    console.log(`渲染配置字段 ${fieldKey}:`, { type, value, defaultValue });

    switch (type) {
      case 'string':
        if (options) {
          return (
            <Select 
              placeholder={placeholder || `选择${label}`}
              value={value !== undefined ? value : defaultValue}
              onChange={(val) => {
                console.log(`Select字段 ${fieldKey} 变更:`, val);
                onChange(val);
              }}
              size="large"
            >
              {options.map(opt => (
                <Option key={opt.value} value={opt.value}>{opt.label}</Option>
              ))}
            </Select>
          );
        }
        return (
          <Input 
            placeholder={placeholder}
            value={value !== undefined ? value : defaultValue}
            onChange={(e) => {
              console.log(`Input字段 ${fieldKey} 变更:`, e.target.value);
              onChange(e.target.value);
            }}
            size="large"
          />
        );
      
      case 'textarea':
        return (
          <TextArea 
            rows={3}
            placeholder={placeholder}
            value={value !== undefined ? value : defaultValue}
            onChange={(e) => {
              console.log(`TextArea字段 ${fieldKey} 变更:`, e.target.value);
              onChange(e.target.value);
            }}
            size="large"
          />
        );
      
      case 'number':
        return (
          <InputNumber 
            style={{ width: '100%' }}
            placeholder={placeholder}
            value={value !== undefined ? value : defaultValue}
            onChange={(val) => {
              console.log(`InputNumber字段 ${fieldKey} 变更:`, val);
              onChange(val);
            }}
            size="large"
          />
        );
      
      case 'boolean':
        return (
          <Switch 
            checked={value !== undefined ? value : defaultValue}
            onChange={(checked) => {
              console.log(`Switch字段 ${fieldKey} 变更:`, checked);
              onChange(checked);
            }}
          />
        );
      
      case 'array':
        if (options) {
          return (
            <Select
              mode="multiple"
              placeholder={placeholder || `选择${label}`}
              value={value || []}
              onChange={(vals) => {
                console.log(`多选字段 ${fieldKey} 变更:`, vals);
                onChange(vals);
              }}
              size="large"
            >
              {options.map(opt => (
                <Option key={opt.value} value={opt.value}>{opt.label}</Option>
              ))}
            </Select>
          );
        }
        return (
          <TextArea 
            rows={2} 
            placeholder="每行一个值"
            value={value !== undefined ? value : defaultValue}
            onChange={(e) => {
              console.log(`数组TextArea字段 ${fieldKey} 变更:`, e.target.value);
              onChange(e.target.value);
            }}
            size="large"
          />
        );
      
      case 'object':
        return (
          <TextArea 
            rows={3}
            placeholder={placeholder || 'JSON格式的对象'}
            value={typeof value === 'object' ? JSON.stringify(value, null, 2) : (value || '')}
            onChange={(e) => {
              try {
                const parsed = JSON.parse(e.target.value);
                console.log(`对象字段 ${fieldKey} 变更:`, parsed);
                onChange(parsed);
              } catch (error) {
                console.log(`对象字段 ${fieldKey} 变更 (文本):`, e.target.value);
                onChange(e.target.value);
              }
            }}
            size="large"
          />
        );
      
      default:
        return (
          <Input 
            placeholder={placeholder}
            value={value !== undefined ? value : defaultValue}
            onChange={(e) => {
              console.log(`默认Input字段 ${fieldKey} 变更:`, e.target.value);
              onChange(e.target.value);
            }}
            size="large"
          />
        );
    }
  };

  const getConfigFieldIcon = (fieldKey: string) => {
    const iconMap: { [key: string]: any } = {
      level: <BellOutlined style={{ color: '#fa8c16' }} />,
      message: <InfoCircleOutlined style={{ color: '#1890ff' }} />,
      type: <SettingOutlined style={{ color: '#52c41a' }} />,
      output_key: <SendOutlined style={{ color: '#722ed1' }} />,
      output_type: <SwapOutlined style={{ color: '#13c2c2' }} />,
      transform_type: <FunctionOutlined style={{ color: '#eb2f96' }} />,
      subject: <SendOutlined style={{ color: '#fa541c' }} />,
      functions: <FunctionOutlined style={{ color: '#1890ff' }} />,
      window_type: <FilterOutlined style={{ color: '#52c41a' }} />,
      window_size: <InfoCircleOutlined style={{ color: '#fa8c16' }} />
    };
    return iconMap[fieldKey] || <SettingOutlined style={{ color: '#666666' }} />;
  };

  const renderActionConfig = (action: Action, actionIndex: number) => {
    const convertedType = convertActionType(action.type);
    const actionType = allActionTypes.find(type => type.value === convertedType);
    if (!actionType?.configSchema) return null;

    return (
      <Card 
        size="small" 
        style={{ 
          marginTop: 16,
          backgroundColor: '#fafcff',
          border: '1px solid #e8f4fd'
        }}
        title={
          <Space>
            <SettingOutlined style={{ color: '#1890ff' }} />
            <span>动作配置</span>
            <Tag color="blue">{actionType.label}</Tag>
          </Space>
        }
      >
        <Row gutter={[16, 16]}>
          {Object.entries(actionType.configSchema).map(([fieldKey, fieldSchema]) => {
            // 检查条件显示逻辑
            if (fieldSchema.conditionalDisplay) {
              const { dependsOn, showWhen } = fieldSchema.conditionalDisplay;
              const dependentValue = action.config?.[dependsOn];
              const shouldShow = showWhen.includes(dependentValue);
              
              if (!shouldShow) {
                return null; // 不显示该字段
              }
            }
            
            const colSpan = fieldSchema.type === 'textarea' ? 24 : 
                          fieldSchema.type === 'array' ? 24 : 
                          fieldSchema.type === 'boolean' ? 12 : 12;
            return (
              <Col span={colSpan} key={fieldKey}>
                <Form.Item
                  label={
                    <Space size="small">
                      {getConfigFieldIcon(fieldKey)}
                      <span>{fieldSchema.label}</span>
                      {fieldSchema.required && <Tag color="red" size="small">必填</Tag>}
                    </Space>
                  }
                >
                  {renderConfigField(
                    fieldKey,
                    fieldSchema,
                    action.config?.[fieldKey],
                    (value) => {
                      const updatedAction = {
                        ...action,
                        config: {
                          ...action.config,
                          [fieldKey]: value
                        }
                      };
                      updateAction(actionIndex, updatedAction);
                    }
                  )}
                  {fieldSchema.description && (
                    <div style={{ marginTop: 4 }}>
                      <Text type="secondary" style={{ fontSize: 11 }}>
                        <InfoCircleOutlined style={{ marginRight: 4 }} />
                        {fieldSchema.description}
                      </Text>
                    </div>
                  )}
                </Form.Item>
              </Col>
            );
          })}
        </Row>
      </Card>
    );
  };

  const getActionTypeIcon = (actionType: string) => {
    const iconMap: { [key: string]: any } = {
      alert: <BellOutlined style={{ color: '#fa8c16' }} />,
      transform: <SwapOutlined style={{ color: '#52c41a' }} />,
      filter: <FilterOutlined style={{ color: '#1890ff' }} />,
      aggregate: <FunctionOutlined style={{ color: '#722ed1' }} />,
      forward: <SendOutlined style={{ color: '#13c2c2' }} />
    };
    return iconMap[actionType] || <SettingOutlined style={{ color: '#666666' }} />;
  };

  const getActionTypeColor = (actionType: string) => {
    const colorMap: { [key: string]: string } = {
      alert: '#fa8c16',
      transform: '#52c41a',
      filter: '#1890ff',
      aggregate: '#722ed1',
      forward: '#13c2c2'
    };
    return colorMap[actionType] || '#666666';
  };

  const renderAction = (action: Action, index: number) => {
    // 先转换动作类型
    const convertedType = convertActionType(action.type);
    const actionType = allActionTypes.find(type => type.value === convertedType);
    const actionColor = getActionTypeColor(convertedType);
    
    // 不在渲染过程中直接更新状态，这会在useEffect中处理
    
    // 调试：如果找不到匹配的动作类型，输出日志
    if (!actionType) {
      console.warn(`ActionFormBuilder: 找不到动作类型 "${convertedType}" (原始: "${action.type}")，可用类型:`, allActionTypes.map(t => t.value));
      console.warn(`ActionFormBuilder: 当前数据类型: ${currentDataType}, 映射规则:`, actionTypeMapping[currentDataType]);
    }
    
    return (
      <Card
        key={index}
        size="small"
        style={{ 
          marginBottom: 16,
          border: `2px solid ${actionColor}20`,
          borderRadius: 8
        }}
        title={
          <Space>
            {getActionTypeIcon(convertedType)}
            <span>动作 {index + 1}</span>
            {actionType && (
              <Tag color={actionColor.replace('#', '')} style={{ marginLeft: 8 }}>
                {actionType.label}
              </Tag>
            )}
          </Space>
        }
        extra={
          actions.length > 1 && (
            <Tooltip title="删除这个动作">
              <Button
                type="link"
                icon={<DeleteOutlined />}
                danger
                size="small"
                onClick={() => removeAction(index)}
              >
                删除
              </Button>
            </Tooltip>
          )
        }
      >
        <Row gutter={[16, 16]}>
          <Col span={12}>
            <Form.Item label={
              <Space size="small">
                <SettingOutlined style={{ color: '#1890ff' }} />
                <span>动作类型</span>
              </Space>
            }>
              <Select
                value={convertedType}
                onChange={(value) => {
                  const selectedType = allActionTypes.find(type => type.value === value);
                  const updatedAction: Action = {
                    ...action,
                    type: value,
                    config: selectedType?.configSchema ? {} : action.config
                  };
                  updateAction(index, updatedAction);
                }}
                size="large"
                style={{ width: '100%' }}
                placeholder="请选择动作类型"
                showSearch
                optionFilterProp="children"
                optionLabelProp="label"
              >
                {allActionTypes.map(type => (
                  <Option key={type.value} value={type.value} label={type.label} title={type.description}>
                    <Space size="small">
                      {getActionTypeIcon(type.value)}
                      <span>{type.label}</span>
                      {type.description && (
                        <Text type="secondary" style={{ fontSize: '12px' }}>- {type.description}</Text>
                      )}
                    </Space>
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item label={
              <Space size="small">
                <PlayCircleOutlined style={{ color: '#52c41a' }} />
                <span>异步执行</span>
              </Space>
            }>
              <Switch
                checked={action.async}
                onChange={(checked) => updateAction(index, { ...action, async: checked })}
                checkedChildren="异步"
                unCheckedChildren="同步"
              />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item label={
              <Space size="small">
                <InfoCircleOutlined style={{ color: '#fa8c16' }} />
                <span>重试次数</span>
              </Space>
            }>
              <InputNumber
                min={0}
                max={10}
                value={action.retry}
                onChange={(value) => updateAction(index, { ...action, retry: value || 0 })}
                size="large"
                style={{ width: '100%' }}
              />
            </Form.Item>
          </Col>
        </Row>

        {/* 显示当前动作的配置预览 */}
        {action.type && (
          <Alert
            message="动作预览"
            description={
              <Space direction="vertical" size="small">
                <Tag color={getActionTypeColor(action.type).replace('#', '')} style={{ fontSize: '12px' }}>
                  类型: {actionType?.label || action.type}
                </Tag>
                <Space size="small">
                  <Tag color={action.async ? 'green' : 'blue'} style={{ fontSize: '11px' }}>
                    {action.async ? '异步执行' : '同步执行'}
                  </Tag>
                  {action.retry > 0 && (
                    <Tag color="orange" style={{ fontSize: '11px' }}>
                      重试 {action.retry} 次
                    </Tag>
                  )}
                </Space>
              </Space>
            }
            type="info"
            showIcon
            style={{ marginTop: 12, marginBottom: 8 }}
            size="small"
          />
        )}

        {renderActionConfig(action, index)}
      </Card>
    );
  };

  return (
    <Card 
      title={
        <Space>
          <SettingOutlined style={{ color: '#1890ff' }} />
          <span>{dataTypeName}动作配置</span>
          <Tag color="blue">规则执行</Tag>
        </Space>
      }
      style={{ 
        border: '1px solid #e8f4fd',
        borderRadius: 8,
        backgroundColor: '#fafcff'
      }}
    >
      <Alert
        message="动作执行说明"
        description="配置规则触发时执行的动作，支持多种动作类型和执行策略"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        size="small"
      />

      {actions.map((action, index) => renderAction(action, index))}

      <Button
        type="dashed"
        icon={<PlusOutlined />}
        onClick={addAction}
        style={{ 
          width: '100%',
          height: '48px',
          borderColor: '#1890ff',
          color: '#1890ff'
        }}
        size="large"
      >
        添加动作
      </Button>
    </Card>
  );
};

export default ActionFormBuilder;