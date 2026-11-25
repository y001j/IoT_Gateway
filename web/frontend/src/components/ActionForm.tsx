import React, { useMemo, useRef, useCallback } from 'react';
import {
  Form,
  Select,
  Input,
  InputNumber,
  Card,
  Button,
  Space,
  Tag,
  Row,
  Col,
  Switch,
  Tooltip,
  Divider,
  Typography
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  FireOutlined,
  FilterOutlined,
  ForwardOutlined,
  FunctionOutlined,
  QuestionCircleOutlined,
  SwapOutlined
} from '@ant-design/icons';
import type { Action } from '../types/rule';

import { cfgString, cfgNumber, cfgOptNumber, cfgObject } from '../utils/ruleData';
const { Option } = Select;
const { TextArea } = Input;
const { Text } = Typography;

interface ActionFormProps {
  value?: Action[];
  onChange?: (value: Action[]) => void;
}

interface ActionConfig {
  type: string;
  config: Record<string, unknown>;
  async?: boolean;
  timeout?: string;
  retry?: number;
}

const ActionForm: React.FC<ActionFormProps> = ({ value, onChange }) => {
  // 使用ref来避免无限循环
  const lastValueRef = useRef<Action[] | undefined>();
  const actionsRef = useRef<ActionConfig[]>([]);

  // 通用的添加标签组件
  const renderAddTagsInput = (action: ActionConfig, index: number, title: string = "添加标签") => {
    return (
      <Form.Item 
        label={
          <Space>
            {title}
            <Tooltip title="为数据添加自定义标签，格式为键值对">
              <QuestionCircleOutlined style={{ color: '#1890ff' }} />
            </Tooltip>
          </Space>
        }
      >
        <TextArea
          rows={3}
          placeholder={'JSON格式的标签，如:\n{\n  "processed": "true",\n  "source": "rule_engine",\n  "stage": "action"\n}'}
          value={action.config.add_tags ? JSON.stringify(action.config.add_tags, null, 2) : ''}
          onChange={(e) => {
            try {
              if (e.target.value.trim() === '') {
                updateActionConfig(index, 'add_tags', undefined);
              } else {
                const tags = JSON.parse(e.target.value);
                updateActionConfig(index, 'add_tags', tags);
              }
            } catch {
              // 暂时保持原值，不更新配置直到JSON有效
            }
          }}
        />
        <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
          为数据添加标签，便于后续处理和标识
        </div>
      </Form.Item>
    );
  };

  // 深度比较函数
  const deepEqual = useCallback((obj1: unknown, obj2: unknown): boolean => {
    if (obj1 === obj2) return true;
    if (!obj1 || !obj2) return obj1 === obj2;
    if (typeof obj1 !== typeof obj2) return false;
    if (typeof obj1 !== 'object') return obj1 === obj2;
    
    const keys1 = Object.keys(obj1);
    const keys2 = Object.keys(obj2);
    if (keys1.length !== keys2.length) return false;
    
    for (const key of keys1) {
      if (!keys2.includes(key)) return false;
      if (!deepEqual(obj1[key], obj2[key])) return false;
    }
    return true;
  }, []);

  // 使用useMemo来同步外部状态，完全避免useEffect
  const currentActions = useMemo(() => {
    
    // 如果没有值，返回默认动作
    if (!value || value.length === 0) {
      const defaultActions = [{ type: 'alert', config: {}, async: false, timeout: '30s', retry: 0 }];
      lastValueRef.current = [];
      actionsRef.current = defaultActions;
      return defaultActions;
    }

    // 深度比较，只有真正变化时才重新计算
    if (!deepEqual(value, lastValueRef.current)) {
      const mappedActions = value.map((action, _index) => {
        // 深拷贝配置，确保数据不丢失
        const config = JSON.parse(JSON.stringify(action.config || {}));
        
        return {
          type: action.type,
          config: config,
          async: action.async,
          timeout: action.timeout,
          retry: action.retry
        };
      });
      lastValueRef.current = JSON.parse(JSON.stringify(value));
      actionsRef.current = mappedActions;
      return mappedActions;
    }
    return actionsRef.current;
  }, [value, deepEqual]);

  // 动作类型配置
  const actionTypes = [
    { 
      value: 'alert', 
      label: '告警通知', 
      icon: <FireOutlined style={{ color: '#ff4d4f' }} />,
      description: '发送告警消息'
    },
    { 
      value: 'transform', 
      label: '数据转换', 
      icon: <SwapOutlined style={{ color: '#1890ff' }} />,
      description: '转换数据格式或值'
    },
    { 
      value: 'filter', 
      label: '数据过滤', 
      icon: <FilterOutlined style={{ color: '#722ed1' }} />,
      description: '过滤或丢弃数据'
    },
    { 
      value: 'aggregate', 
      label: '数据聚合', 
      icon: <FunctionOutlined style={{ color: '#52c41a' }} />,
      description: '聚合计算统计值'
    },
    { 
      value: 'forward', 
      label: '数据转发', 
      icon: <ForwardOutlined style={{ color: '#fa8c16' }} />,
      description: '转发到外部系统'
    }
  ];

  // 辅助函数：转换为回调格式
  const convertToCallbackFormat = useCallback((actions: ActionConfig[]): Action[] => {
    return actions.map(action => ({
      type: action.type,
      config: action.config,
      async: action.async || false,
      timeout: action.timeout || '30s',
      retry: action.retry || 0
    }));
  }, []);

  // 添加动作
  const addAction = useCallback(() => {
    const newActions = [...currentActions, { type: 'alert', config: {}, async: false, timeout: '30s', retry: 0 }];
    actionsRef.current = newActions;
    const newActionsForCallback = convertToCallbackFormat(newActions);
    onChange?.(newActionsForCallback);
  }, [currentActions, onChange, convertToCallbackFormat]);

  // 删除动作
  const removeAction = useCallback((index: number) => {
    const newActions = currentActions.filter((_, i) => i !== index);
    actionsRef.current = newActions;
    const newActionsForCallback = convertToCallbackFormat(newActions);
    onChange?.(newActionsForCallback);
  }, [currentActions, onChange, convertToCallbackFormat]);

  // 更新动作
  const updateAction = useCallback((index: number, field: keyof ActionConfig, value: unknown) => {
    const newActions = [...currentActions];
    newActions[index] = { ...newActions[index], [field]: value };
    actionsRef.current = newActions;
    const newActionsForCallback = convertToCallbackFormat(newActions);
    onChange?.(newActionsForCallback);
  }, [currentActions, onChange, convertToCallbackFormat]);

  // 更新动作配置
  const updateActionConfig = useCallback((index: number, key: string, value: unknown) => {
    const newActions = [...currentActions];
    newActions[index] = {
      ...newActions[index],
      config: { ...newActions[index].config, [key]: value }
    };
    actionsRef.current = newActions;
    
    // 使用新的actions数据立即触发变更
    const newActionsForCallback = convertToCallbackFormat(newActions);
    onChange?.(newActionsForCallback);
  }, [currentActions, onChange, convertToCallbackFormat]);

  // 渲染告警配置
  const renderAlertConfig = (action: ActionConfig, index: number) => {
    // 处理通知渠道的兼容性：支持两种格式
    // 格式1: ["console", "webhook"] (简单数组)
    // 格式2: [{"type": "console"}, {"type": "webhook", "config": {...}}] (对象数组)
    let channelsValue = [];
    if (action.config.channels) {
      if (Array.isArray(action.config.channels)) {
        channelsValue = action.config.channels.map(channel => {
          if (typeof channel === 'string') {
            return channel; // 简单字符串格式
          } else if (typeof channel === 'object' && channel.type) {
            return channel.type; // 对象格式，提取type字段
          }
          return channel;
        });
      }
    }

    return (
      <div>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item label="告警级别">
              <Select
                value={cfgString(action.config.level, 'warning')}
                onChange={(value) => updateActionConfig(index, 'level', value)}
              >
                <Option key="info" value="info">信息</Option>
                <Option key="warning" value="warning">警告</Option>
                <Option key="error" value="error">错误</Option>
                <Option key="critical" value="critical">严重</Option>
              </Select>
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item label="限流时间">
              <Input
                placeholder="如 5m, 1h"
                value={cfgString(action.config.throttle)}
                onChange={(e) => updateActionConfig(index, 'throttle', e.target.value)}
              />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item label="告警消息">
          <TextArea
            rows={2}
            placeholder="支持变量: {{.DeviceID}}, {{.Key}}, {{.Value}}"
            value={cfgString(action.config.message)}
            onChange={(e) => updateActionConfig(index, 'message', e.target.value)}
          />
        </Form.Item>
        <Form.Item label="通知渠道">
          <Select
            mode="multiple"
            placeholder="选择通知渠道"
            value={channelsValue}
            onChange={(value) => updateActionConfig(index, 'channels', value)}
          >
            <Option key="console" value="console">控制台</Option>
            <Option key="webhook" value="webhook">Webhook</Option>
            <Option key="email" value="email">邮件</Option>
            <Option key="sms" value="sms">短信</Option>
          </Select>
        </Form.Item>
        
        {renderAddTagsInput(action, index, "告警标签")}
      </div>
    );
  };

  // 渲染转换配置
  const renderTransformConfig = (action: ActionConfig, index: number) => {
    // 标准格式：只使用 parameters 嵌套结构，但兼容旧格式
    const transformType = cfgString(action.config.type, 'scale');
    
    // 兼容旧格式：如果没有parameters但有直接的参数，则从根级别读取
    let parameters: Record<string, unknown> = cfgObject(action.config.parameters);
    
    // 兼容旧的表达式格式：expression直接在config下
    if (!parameters.expression && action.config.expression) {
      parameters = { ...parameters, expression: action.config.expression };
    }
    
    // 兼容其他旧格式参数
    if (!parameters.factor && action.config.factor !== undefined) {
      parameters = { ...parameters, factor: action.config.factor };
    }
    if (!parameters.offset && action.config.offset !== undefined) {
      parameters = { ...parameters, offset: action.config.offset };
    }
    
    // 更新参数的辅助函数 - 只使用标准格式
    const updateParameter = (key: string, value: unknown) => {
      const newParams = { ...parameters, [key]: value };
      updateActionConfig(index, 'parameters', newParams);
    };

    // 根据转换类型渲染不同的参数输入
    const renderParameterInputs = () => {
      switch (transformType) {
        case 'identity':
          return (
            <div style={{ 
              padding: '12px', 
              background: '#f5f5f5', 
              borderRadius: '6px',
              fontSize: '12px',
              color: '#666'
            }}>
              💡 提示：identity转换保持原始数据不变，主要用于为数据添加标签而不修改数值。
            </div>
          );
          
        case 'scale':
          return (
            <Form.Item label="缩放因子">
              <InputNumber
                placeholder="乘数，如 2.0"
                value={cfgOptNumber(parameters.factor)}
                onChange={(value) => updateParameter('factor', value)}
                style={{ width: '100%' }}
                step={0.1}
              />
              <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
                将原始值乘以此因子
              </div>
            </Form.Item>
          );

        case 'offset':
          return (
            <Form.Item label="偏移量">
              <InputNumber
                placeholder="偏移值，如 32"
                value={cfgOptNumber(parameters.offset)}
                onChange={(value) => updateParameter('offset', value)}
                style={{ width: '100%' }}
                step={0.1}
              />
              <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
                将原始值加上此偏移量
              </div>
            </Form.Item>
          );

        case 'expression':
          return (
            <Form.Item label="表达式">
              <Input
                placeholder="数学表达式，如 x * 1.8 + 32"
                value={cfgString(parameters.expression)}
                onChange={(e) => updateParameter('expression', e.target.value)}
              />
              <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
                使用 x 表示原始值，支持 +、-、*、/、()、函数调用如 abs(x)、sqrt(x)
              </div>
            </Form.Item>
          );

        case 'unit_convert':
          return (
            <>
              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item label="源单位">
                    <Select
                      placeholder="选择源单位"
                      value={cfgString(parameters.from) || undefined}
                      onChange={(value) => updateParameter('from', value)}
                      style={{ width: '100%' }}
                    >
                      <Option key="from-C" value="C">摄氏度 (°C)</Option>
                      <Option key="from-F" value="F">华氏度 (°F)</Option>
                      <Option key="from-K" value="K">开尔文 (K)</Option>
                      <Option key="from-m" value="m">米 (m)</Option>
                      <Option key="from-ft" value="ft">英尺 (ft)</Option>
                      <Option key="from-kg" value="kg">千克 (kg)</Option>
                      <Option key="from-lb" value="lb">磅 (lb)</Option>
                    </Select>
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="目标单位">
                    <Select
                      placeholder="选择目标单位"
                      value={cfgString(parameters.to) || undefined}
                      onChange={(value) => updateParameter('to', value)}
                      style={{ width: '100%' }}
                    >
                      <Option key="to-C" value="C">摄氏度 (°C)</Option>
                      <Option key="to-F" value="F">华氏度 (°F)</Option>
                      <Option key="to-K" value="K">开尔文 (K)</Option>
                      <Option key="to-m" value="m">米 (m)</Option>
                      <Option key="to-ft" value="ft">英尺 (ft)</Option>
                      <Option key="to-kg" value="kg">千克 (kg)</Option>
                      <Option key="to-lb" value="lb">磅 (lb)</Option>
                    </Select>
                  </Form.Item>
                </Col>
              </Row>
            </>
          );

        case 'lookup': {
          // 支持两种数据结构：parameters.table 和 顶级的 lookup_table
          const lookupTable = parameters.table || action.config.lookup_table || {};
          const defaultValue = cfgString(parameters.default) || cfgString(action.config.default_value);
          
          return (
            <>
              <Form.Item label="查找表">
                <TextArea
                  rows={4}
                  placeholder='JSON格式的映射表，如: {"0": "正常", "1": "警告", "2": "错误"}'
                  value={lookupTable ? JSON.stringify(lookupTable, null, 2) : ''}
                  onChange={(e) => {
                    try {
                      const table = JSON.parse(e.target.value || '{}');
                      // 同时更新两种格式以保持兼容性
                      updateParameter('table', table);
                      updateActionConfig(index, 'lookup_table', table);
                    } catch {
                      // 忽略JSON解析错误
                    }
                  }}
                />
              </Form.Item>
              <Form.Item label="默认值">
                <Input
                  placeholder="未找到映射时的默认值"
                  value={defaultValue}
                  onChange={(e) => {
                    const value = e.target.value;
                    // 同时更新两种格式以保持兼容性
                    updateParameter('default', value);
                    updateActionConfig(index, 'default_value', value);
                  }}
                />
              </Form.Item>
            </>
          );
        }

        case 'round':
          return (
            <Form.Item label="小数位数">
              <InputNumber
                placeholder="保留的小数位数"
                value={cfgNumber(parameters.decimals, 0)}
                onChange={(value) => updateParameter('decimals', value)}
                min={0}
                max={10}
                style={{ width: '100%' }}
              />
            </Form.Item>
          );

        case 'clamp':
          return (
            <Row gutter={8}>
              <Col span={12}>
                <Form.Item label="最小值">
                  <InputNumber
                    placeholder="数值下限"
                    value={cfgOptNumber(parameters.min)}
                    onChange={(value) => updateParameter('min', value)}
                    style={{ width: '100%' }}
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item label="最大值">
                  <InputNumber
                    placeholder="数值上限"
                    value={cfgOptNumber(parameters.max)}
                    onChange={(value) => updateParameter('max', value)}
                    style={{ width: '100%' }}
                  />
                </Form.Item>
              </Col>
            </Row>
          );

        case 'format':
          return (
            <Form.Item label="格式字符串">
              <Input
                placeholder="格式化模板，如 %.2f"
                value={cfgString(parameters.format)}
                onChange={(e) => updateParameter('format', e.target.value)}
              />
              <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
                使用 Go 语言格式化语法
              </div>
            </Form.Item>
          );

        case 'map':
          return (
            <Form.Item label="映射表">
              <TextArea
                rows={4}
                placeholder='JSON格式的映射表，如: {"high": 1, "low": 0}'
                value={parameters.mapping ? JSON.stringify(parameters.mapping, null, 2) : ''}
                onChange={(e) => {
                  try {
                    const mapping = JSON.parse(e.target.value || '{}');
                    updateParameter('mapping', mapping);
                  } catch {
                    // 忽略JSON解析错误
                  }
                }}
              />
            </Form.Item>
          );

        default:
          return null;
      }
    };

    return (
      <div>
        <Form.Item label="转换类型">
          <Select
            placeholder="选择转换类型"
            value={transformType}
            onChange={(value) => updateActionConfig(index, 'type', value)}
            style={{ width: '100%' }}
          >
            <Option key="identity" value="identity">保持原值（用于添加标签）</Option>
            <Option key="scale" value="scale">数值缩放</Option>
            <Option key="offset" value="offset">数值偏移</Option>
            <Option key="expression" value="expression">表达式计算</Option>
            <Option key="unit_convert" value="unit_convert">单位转换</Option>
            <Option key="lookup" value="lookup">查找表映射</Option>
            <Option key="round" value="round">四舍五入</Option>
            <Option key="clamp" value="clamp">数值限幅</Option>
            <Option key="format" value="format">格式化</Option>
            <Option key="map" value="map">值映射</Option>
          </Select>
        </Form.Item>

        {renderParameterInputs()}

        <Form.Item 
          label={
            <Space>
              输出字段名
              <Tooltip title="不设置时将直接覆盖原始字段的值，原始数据将丢失。建议设置新的字段名以保留原始数据。">
                <QuestionCircleOutlined style={{ color: '#1890ff' }} />
              </Tooltip>
            </Space>
          }
        >
          <Input
            placeholder="转换后的字段名，如 temperature_fahrenheit（推荐设置）"
            value={cfgString(action.config.output_key)}
            onChange={(e) => updateActionConfig(index, 'output_key', e.target.value)}
          />
          <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
            {action.config.output_key ? 
              '✅ 将创建新字段，原始数据保留' : 
              '⚠️ 留空将直接覆盖原始字段的值，原始数据丢失'
            }
          </div>
        </Form.Item>

        <Form.Item label="输出数据类型">
          <Select
            placeholder="选择输出数据类型"
            value={action.config.output_type || undefined}
            onChange={(value) => updateActionConfig(index, 'output_type', value)}
            allowClear
          >
            <Option key="string" value="string">字符串</Option>
            <Option key="int" value="int">整数</Option>
            <Option key="float" value="float">浮点数</Option>
            <Option key="bool" value="bool">布尔值</Option>
          </Select>
          <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
            不指定则保持原数据类型
          </div>
        </Form.Item>

        <Form.Item label="数值精度">
          <InputNumber
            placeholder="小数位数（仅对数值类型有效）"
            value={cfgOptNumber(action.config.precision)}
            onChange={(value) => updateActionConfig(index, 'precision', value)}
            min={0}
            max={10}
            style={{ width: '100%' }}
          />
          <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
            仅对数值类型有效，不设置则使用默认精度
          </div>
        </Form.Item>

        <Form.Item label="错误处理策略">
          <Select
            placeholder="转换出错时的处理方式"
            value={cfgString(action.config.error_action, 'error')}
            onChange={(value) => updateActionConfig(index, 'error_action', value)}
          >
            <Option key="error-handling" value="error">抛出错误</Option>
            <Option key="ignore" value="ignore">忽略错误，返回原值</Option>
            <Option key="default" value="default">使用默认值</Option>
          </Select>
        </Form.Item>

        {action.config.error_action === 'default' && (
          <Form.Item label="默认值">
            <Input
              placeholder="错误时使用的默认值"
              value={action.config.default_value !== undefined ? String(action.config.default_value) : ''}
              onChange={(e) => updateActionConfig(index, 'default_value', e.target.value)}
            />
          </Form.Item>
        )}

        {/* 专门的添加标签功能 */}
        <Form.Item 
          label={
            <Space>
              添加标签
              <Tooltip title="为数据添加自定义标签，这些标签将附加到数据点上，用于数据分类和过滤">
                <QuestionCircleOutlined />
              </Tooltip>
            </Space>
          }
        >
          <TextArea
            rows={3}
            placeholder={`JSON格式的标签，如:
{
  "processed": "true",
  "transform_type": "${action.config.type || 'unknown'}",
  "stage": "transform"
}`}
            value={action.config.add_tags ? JSON.stringify(action.config.add_tags, null, 2) : ''}
            onChange={(e) => {
              try {
                if (e.target.value.trim() === '') {
                  updateActionConfig(index, 'add_tags', undefined);
                } else {
                  const tags = JSON.parse(e.target.value);
                  updateActionConfig(index, 'add_tags', tags);
                }
              } catch {
                // 暂时保持原值，不更新配置直到JSON有效
              }
            }}
          />
          <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
            💡 提示: 可以添加 transform_type、validation、processed 等标签来标记数据处理状态
          </div>
        </Form.Item>

        <Form.Item label="发布主题">
          <Input
            placeholder="NATS发布主题（可选），如 iot.data.transformed"
            value={cfgString(action.config.publish_subject)}
            onChange={(e) => updateActionConfig(index, 'publish_subject', e.target.value)}
          />
          <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
            支持变量模板，如 iot.data.{`{{.DeviceID}}`}
          </div>
        </Form.Item>
      </div>
    );
  };

  // 渲染过滤配置
  const renderFilterConfig = (action: ActionConfig, index: number) => {
    // 处理后端数据结构兼容
    const parameters = cfgObject(action.config.parameters);
    const filterType = cfgString(action.config.type) || cfgString(parameters.type) || 'range';
    
    // 兼容旧的数据结构，将根级别的配置迁移到parameters中
    const getParameterValue = (key: string) => {
      return parameters[key] !== undefined ? parameters[key] : action.config[key];
    };
    
    const updateFilterConfig = (key: string, value: unknown) => {
      // 更新parameters对象结构
      const newParameters = { ...parameters, [key]: value };
      updateActionConfig(index, 'parameters', newParameters);
      
      // 同时保持根级别的兼容性（前端显示用）
      updateActionConfig(index, key, value);
    };
    
    return (
    <div>
      <Form.Item label="过滤类型">
        <Select
          value={filterType}
          onChange={(value) => {
            updateActionConfig(index, 'type', value);
            // 同时更新parameters中的type
            updateFilterConfig('type', value);
          }}
        >
          <Option key="range" value="range">范围过滤</Option>
          <Option key="duplicate" value="duplicate">去重过滤</Option>
          <Option key="rate_limit" value="rate_limit">速率限制</Option>
          <Option key="pattern" value="pattern">模式匹配过滤</Option>
          <Option key="null" value="null">空值过滤</Option>
          <Option key="threshold" value="threshold">阈值过滤</Option>
          <Option key="time_window" value="time_window">时间窗口过滤</Option>
          <Option key="quality" value="quality">数据质量过滤</Option>
          <Option key="change_rate" value="change_rate">变化率过滤</Option>
          <Option key="statistical_anomaly" value="statistical_anomaly">统计异常过滤</Option>
          <Option key="consecutive" value="consecutive">连续异常过滤</Option>
        </Select>
      </Form.Item>

      {filterType === 'range' && (
        <div>
          <Form.Item label="过滤字段">
            <Input
              placeholder="要过滤的数据字段，如 value 或 temperature"
              value={cfgString(getParameterValue('field'))}
              onChange={(e) => updateFilterConfig('field', e.target.value)}
            />
            <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
              留空则对主数值字段进行过滤
            </div>
          </Form.Item>
          
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item label="最小值">
                <InputNumber
                  placeholder="范围最小值"
                  value={getParameterValue('min') !== undefined ? Number(getParameterValue('min')) : undefined}
                  onChange={(value) => updateFilterConfig('min', value)}
                  style={{ width: '100%' }}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="最大值">
                <InputNumber
                  placeholder="范围最大值"
                  value={getParameterValue('max') !== undefined ? Number(getParameterValue('max')) : undefined}
                  onChange={(value) => updateFilterConfig('max', value)}
                  style={{ width: '100%' }}
                />
              </Form.Item>
            </Col>
          </Row>
          
          <Form.Item label="过滤动作" tooltip="当数值在设定范围内时的处理方式">
            <Select
              value={getParameterValue('action') || action.config.action || 'drop'}
              onChange={(value) => {
                updateActionConfig(index, 'action', value);
                updateFilterConfig('action', value);
              }}
            >
              <Option key="pass" value="pass">通过数据（保留符合条件的数据）</Option>
              <Option key="drop" value="drop">丢弃数据（删除符合条件的数据）</Option>
            </Select>
          </Form.Item>
        </div>
      )}

      {filterType === 'rate_limit' && (
        <div>
          <Form.Item label="速率限制" tooltip="限制在指定时间窗口内通过的数据量">
            <Row gutter={8}>
              <Col span={12}>
                <InputNumber
                  placeholder="数量"
                  value={getParameterValue('rate') !== undefined ? Number(getParameterValue('rate')) : undefined}
                  onChange={(value) => updateFilterConfig('rate', value)}
                  min={1}
                  style={{ width: '100%' }}
                />
              </Col>
              <Col span={12}>
                <Input
                  placeholder="时间窗口，如 1m, 30s, 1h"
                  value={cfgString(getParameterValue('window'))}
                  onChange={(e) => updateFilterConfig('window', e.target.value)}
                />
              </Col>
            </Row>
            <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
              例如: 10次/分钟，设置数量为10，时间窗口为1m
            </div>
          </Form.Item>
        </div>
      )}

      {filterType === 'duplicate' && (
        <div>
          <Form.Item label="去重字段">
            <Input
              placeholder="用于去重的字段名，留空则使用完整数据进行去重"
              value={cfgString(getParameterValue('field'))}
              onChange={(e) => updateFilterConfig('field', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="去重时间窗口">
            <Input
              placeholder="时间窗口，如 5m, 1h，在此时间内的重复数据将被过滤"
              value={cfgString(getParameterValue('window')) || cfgString(getParameterValue('ttl'))}
              onChange={(e) => {
                updateFilterConfig('window', e.target.value);
                updateFilterConfig('ttl', e.target.value);
              }}
            />
            <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
              不设置则使用全局去重（内存中永久保存去重状态）
            </div>
          </Form.Item>
        </div>
      )}

      {filterType === 'pattern' && (
        <div>
          <Form.Item label="匹配模式">
            <Input
              placeholder="正则表达式或字符串模式"
              value={cfgString(getParameterValue('pattern'))}
              onChange={(e) => updateFilterConfig('pattern', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="匹配字段">
            <Input
              placeholder="要匹配的字段名，默认为value"
              value={cfgString(getParameterValue('field'))}
              onChange={(e) => updateFilterConfig('field', e.target.value)}
            />
          </Form.Item>
        </div>
      )}

      {filterType === 'null' && (
        <div>
          <Form.Item label="空值处理">
            <Select
              value={getParameterValue('action') || action.config.action || 'drop'}
              onChange={(value) => {
                updateActionConfig(index, 'action', value);
                updateFilterConfig('action', value);
              }}
            >
              <Option key="drop" value="drop">丢弃空值数据</Option>
              <Option key="pass" value="pass">保留空值数据</Option>
              <Option key="fill" value="fill">填充默认值</Option>
            </Select>
          </Form.Item>
          {(getParameterValue('action') === 'fill' || action.config.action === 'fill') && (
            <Form.Item label="默认值">
              <Input
                placeholder="空值时的填充值"
                value={cfgString(getParameterValue('default_value'))}
                onChange={(e) => updateFilterConfig('default_value', e.target.value)}
              />
            </Form.Item>
          )}
        </div>
      )}

      {filterType === 'threshold' && (
        <div>
          <Form.Item label="阈值设置">
            <Row gutter={8}>
              <Col span={12}>
                <InputNumber
                  placeholder="阈值"
                  value={getParameterValue('threshold') !== undefined ? Number(getParameterValue('threshold')) : undefined}
                  onChange={(value) => updateFilterConfig('threshold', value)}
                  style={{ width: '100%' }}
                />
              </Col>
              <Col span={12}>
                <Select
                  placeholder="比较方式"
                  value={cfgString(getParameterValue('operator'), 'gt')}
                  onChange={(value) => updateFilterConfig('operator', value)}
                >
                  <Option key="gt" value="gt">大于 (&gt;)</Option>
                  <Option key="gte" value="gte">大于等于 (&ge;)</Option>
                  <Option key="lt" value="lt">小于 (&lt;)</Option>
                  <Option key="lte" value="lte">小于等于 (&le;)</Option>
                  <Option key="eq" value="eq">等于 (=)</Option>
                  <Option key="ne" value="ne">不等于 (&ne;)</Option>
                </Select>
              </Col>
            </Row>
          </Form.Item>
        </div>
      )}

      {filterType === 'time_window' && (
        <div>
          <Form.Item label="时间窗口">
            <Input
              placeholder="时间窗口大小，如 5m, 1h, 30s"
              value={cfgString(getParameterValue('window'))}
              onChange={(e) => updateFilterConfig('window', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="窗口内最大数量">
            <InputNumber
              placeholder="时间窗口内允许的最大数据量"
              value={getParameterValue('max_count') !== undefined ? Number(getParameterValue('max_count')) : undefined}
              onChange={(value) => updateFilterConfig('max_count', value)}
              min={1}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </div>
      )}

      {filterType === 'quality' && (
        <div>
          <Form.Item label="质量检查类型">
            <Select
              mode="multiple"
              placeholder="选择数据质量检查项"
              value={getParameterValue('checks') || []}
              onChange={(value) => updateFilterConfig('checks', value)}
            >
              <Option key="range" value="range">范围检查</Option>
              <Option key="type" value="type">数据类型检查</Option>
              <Option key="format" value="format">格式检查</Option>
              <Option key="completeness" value="completeness">完整性检查</Option>
            </Select>
          </Form.Item>
        </div>
      )}

      {filterType === 'change_rate' && (
        <div>
          <Form.Item label="变化率阈值">
            <InputNumber
              placeholder="变化率阈值（百分比）"
              value={getParameterValue('rate_threshold') !== undefined ? Number(getParameterValue('rate_threshold')) : undefined}
              onChange={(value) => updateFilterConfig('rate_threshold', value)}
              min={0}
              max={100}
              formatter={value => `${value}%`}
              parser={value => Number((value ?? '').replace('%', '')) || 0}
              style={{ width: '100%' }}
            />
          </Form.Item>
          <Form.Item label="时间窗口">
            <Input
              placeholder="计算变化率的时间窗口，如 1m"
              value={cfgString(getParameterValue('window'))}
              onChange={(e) => updateFilterConfig('window', e.target.value)}
            />
          </Form.Item>
        </div>
      )}

      {filterType === 'statistical_anomaly' && (
        <div>
          <Form.Item label="异常检测方法">
            <Select
              value={cfgString(getParameterValue('method'), 'z_score')}
              onChange={(value) => updateFilterConfig('method', value)}
            >
              <Option key="z_score" value="z_score">Z-Score (标准差)</Option>
              <Option key="iqr" value="iqr">IQR (四分位数间距)</Option>
              <Option key="mad" value="mad">MAD (中位绝对偏差)</Option>
            </Select>
          </Form.Item>
          <Form.Item label="异常阈值">
            <InputNumber
              placeholder="异常检测阈值"
              value={getParameterValue('threshold') !== undefined ? Number(getParameterValue('threshold')) : 2.5}
              onChange={(value) => updateFilterConfig('threshold', value)}
              step={0.1}
              min={0.1}
              max={10}
              style={{ width: '100%' }}
            />
          </Form.Item>
          <Form.Item label="统计窗口大小">
            <InputNumber
              placeholder="用于统计的数据点数量"
              value={getParameterValue('window_size') !== undefined ? Number(getParameterValue('window_size')) : 20}
              onChange={(value) => updateFilterConfig('window_size', value)}
              min={5}
              max={1000}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </div>
      )}

      {filterType === 'consecutive' && (
        <div>
          <Form.Item label="连续次数">
            <InputNumber
              placeholder="连续异常的次数阈值"
              value={getParameterValue('count') !== undefined ? Number(getParameterValue('count')) : 3}
              onChange={(value) => updateFilterConfig('count', value)}
              min={2}
              max={100}
              style={{ width: '100%' }}
            />
          </Form.Item>
          <Form.Item label="异常条件">
            <Input
              placeholder="定义异常的条件表达式"
              value={cfgString(getParameterValue('condition'))}
              onChange={(e) => updateFilterConfig('condition', e.target.value)}
            />
            <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
              例如: value &gt; 100 或 value &lt; 0
            </div>
          </Form.Item>
        </div>
      )}
      
      {renderAddTagsInput(action, index, "过滤标签")}
    </div>
  );
  };

  // 渲染聚合配置
  const renderAggregateConfig = (action: ActionConfig, index: number) => {
    
    // 兼容后端配置字段映射 - 支持多种数据结构
    const windowSize = cfgOptNumber(action.config.size) ?? cfgOptNumber(action.config.window_size);
    
    // 处理输出字段名的多种数据结构
    let outputKey = cfgString(action.config.output_key) || cfgString(action.config.output_field);
    
    // 检查是否存在 output 对象结构（后端存储格式）
    if (!outputKey) {
      outputKey = cfgString(cfgObject(action.config.output).key_template);
    }
    
    return (
      <div>
        <Form.Item label="窗口大小">
          <InputNumber
            placeholder="数据点数量"
            value={windowSize}
            onChange={(value) => {
              // 优先更新 window_size（后端使用的字段）
              updateActionConfig(index, 'window_size', value);
              // 如果原来有 size 字段也更新它
              if (action.config.size !== undefined) {
                updateActionConfig(index, 'size', value);
              }
            }}
            min={1}
            style={{ width: '100%' }}
            addonAfter="个数据点"
          />
          <div style={{ fontSize: '12px', color: '#666', marginTop: '4px' }}>
            基于数据点计数的滑动窗口，如设置为10表示统计最近10个数据点
          </div>
        </Form.Item>

      <Form.Item label={
        <Space>
          聚合函数
          <Tooltip title="支持28个聚合函数，包括基础统计、百分位数、数据质量、变化检测等">
            <QuestionCircleOutlined style={{ color: '#1890ff' }} />
          </Tooltip>
        </Space>
      }>
        <Select
          mode="multiple"
          placeholder="选择聚合函数（支持搜索）"
          value={action.config.functions || []}
          onChange={(value) => updateActionConfig(index, 'functions', value)}
          showSearch
          filterOption={(input, option) =>
            option?.children?.toString().toLowerCase().includes(input.toLowerCase()) ||
            option?.value?.toString().toLowerCase().includes(input.toLowerCase())
          }
          style={{ width: '100%' }}
        >
          {/* 基础统计函数 */}
          <Select.OptGroup label="📊 基础统计">
            <Option key="count" value="count">计数 (count)</Option>
            <Option key="sum" value="sum">求和 (sum)</Option>
            <Option key="avg" value="avg">平均值 (avg)</Option>
            <Option key="mean" value="mean">平均值 (mean)</Option>
            <Option key="average" value="average">平均值 (average)</Option>
            <Option key="min" value="min">最小值 (min)</Option>
            <Option key="max" value="max">最大值 (max)</Option>
            <Option key="median" value="median">中位数 (median)</Option>
            <Option key="first" value="first">首个值 (first)</Option>
            <Option key="last" value="last">最后值 (last)</Option>
          </Select.OptGroup>

          {/* 分布统计函数 */}
          <Select.OptGroup label="📈 分布统计">
            <Option key="stddev" value="stddev">标准差 (stddev)</Option>
            <Option key="std" value="std">标准差 (std)</Option>
            <Option key="variance" value="variance">方差 (variance)</Option>
            <Option key="volatility" value="volatility">波动率 (volatility)</Option>
            <Option key="cv" value="cv">变异系数 (cv)</Option>
          </Select.OptGroup>

          {/* 百分位数函数 */}
          <Select.OptGroup label="📊 百分位数">
            <Option key="p25" value="p25">25百分位 (p25)</Option>
            <Option key="p50" value="p50">50百分位 (p50)</Option>
            <Option key="p75" value="p75">75百分位 (p75)</Option>
            <Option key="p90" value="p90">90百分位 (p90)</Option>
            <Option key="p95" value="p95">95百分位 (p95)</Option>
            <Option key="p99" value="p99">99百分位 (p99)</Option>
          </Select.OptGroup>

          {/* 数据质量函数 */}
          <Select.OptGroup label="🔍 数据质量">
            <Option key="null_rate" value="null_rate">空值率 (null_rate)</Option>
            <Option key="completeness" value="completeness">完整性 (completeness)</Option>
            <Option key="outlier_count" value="outlier_count">异常值计数 (outlier_count)</Option>
          </Select.OptGroup>

          {/* 变化检测函数 */}
          <Select.OptGroup label="📉 变化检测">
            <Option key="change" value="change">变化量 (change)</Option>
            <Option key="change_rate" value="change_rate">变化率 (change_rate)</Option>
          </Select.OptGroup>

          {/* 阈值监控函数 */}
          <Select.OptGroup label="⚡ 阈值监控">
            <Option key="above_count" value="above_count">超过阈值计数 (above_count)</Option>
            <Option key="below_count" value="below_count">低于阈值计数 (below_count)</Option>
            <Option key="in_range_count" value="in_range_count">范围内计数 (in_range_count)</Option>
          </Select.OptGroup>
        </Select>
        <div style={{ fontSize: '12px', color: '#666', marginTop: '8px' }}>
          💡 提示：支持多选和搜索，包括基础统计、百分位数、数据质量检测等28个函数。<br/>
          📊 <strong>基础统计</strong>: count, sum, avg, min, max 等日常统计<br/>
          📈 <strong>分布统计</strong>: stddev, variance, volatility 等离散度指标<br/>
          📊 <strong>百分位数</strong>: p25, p50, p75, p90, p95, p99 性能监控关键指标<br/>
          🔍 <strong>数据质量</strong>: null_rate, completeness, outlier_count 数据健康度<br/>
          📉 <strong>变化检测</strong>: change, change_rate 趋势分析<br/>
          ⚡ <strong>阈值监控</strong>: above_count, below_count, in_range_count 需配置阈值参数
        </div>
      </Form.Item>

      <Form.Item label="分组字段">
        <Select
          mode="tags"
          placeholder="按字段分组"
          value={action.config.group_by || []}
          onChange={(value) => updateActionConfig(index, 'group_by', value)}
        >
          <Option key="device_id" value="device_id">设备ID</Option>
          <Option key="key" value="key">数据键</Option>
          <Option key="tags" value="tags">标签</Option>
        </Select>
      </Form.Item>

      <Form.Item 
        label={
          <Space>
            输出字段名
            <Tooltip title="聚合结果的字段名。支持模板变量，如 {{.Key}}_stats。留空将使用默认字段名。">
              <QuestionCircleOutlined style={{ color: '#1890ff' }} />
            </Tooltip>
          </Space>
        }
      >
        <Input
          key={`output-field-${index}-${outputKey || 'empty'}`}
          placeholder="如 {{.Key}}_stats（支持模板变量）"
          value={outputKey || ''}
          onChange={(e) => {
            const value = e.target.value;
            
            // 如果存在 output 对象结构，优先更新它（后端使用的格式）
            if (action.config.output && typeof action.config.output === 'object') {
              updateActionConfig(index, 'output', {
                ...action.config.output,
                key_template: value
              });
            } else {
              // 否则更新 output_key 
              updateActionConfig(index, 'output_key', value);
            }
            
            // 同时更新可能的别名字段
            if (action.config.output_field !== undefined) {
              updateActionConfig(index, 'output_field', value);
            }
          }}
        />
      </Form.Item>

      <Form.Item label="窗口类型" tooltip="计数窗口：基于数据点数量；时间窗口：基于时间范围">
        <Select
          value={cfgString(action.config.window_type, 'count')}
          onChange={(value) => updateActionConfig(index, 'window_type', value)}
        >
          <Option value="count">计数窗口</Option>
          <Option value="time">时间窗口</Option>
        </Select>
      </Form.Item>
      
      {action.config.window_type === 'time' && (
        <Form.Item 
          label="时间窗口" 
          tooltip="时间格式：1s, 30s, 1m, 5m, 1h等"
        >
          <Input
            placeholder="如: 1m, 30s, 1h"
            value={cfgString(action.config.window)}
            onChange={(e) => updateActionConfig(index, 'window', e.target.value)}
          />
        </Form.Item>
      )}
      
      {/* 阈值监控函数的参数配置 */}
      <Form.Item 
        label="上限阈值" 
        tooltip="用于above_count、in_range_count等阈值监控函数"
      >
        <InputNumber
          style={{ width: '100%' }}
          placeholder="可选，用于阈值监控函数"
          value={cfgOptNumber(action.config.upper_limit)}
          onChange={(value) => updateActionConfig(index, 'upper_limit', value)}
        />
      </Form.Item>
      
      <Form.Item 
        label="下限阈值" 
        tooltip="用于below_count、in_range_count等阈值监控函数"
      >
        <InputNumber
          style={{ width: '100%' }}
          placeholder="可选，用于阈值监控函数"
          value={cfgOptNumber(action.config.lower_limit)}
          onChange={(value) => updateActionConfig(index, 'lower_limit', value)}
        />
      </Form.Item>
      
      <Form.Item 
        label="异常值阈值" 
        tooltip="用于outlier_count函数，标准差的倍数，如2.0表示2倍标准差"
      >
        <InputNumber
          min={0}
          step={0.1}
          style={{ width: '100%' }}
          placeholder="如：2.0表示2倍标准差"
          value={cfgOptNumber(action.config.outlier_threshold)}
          onChange={(value) => updateActionConfig(index, 'outlier_threshold', value)}
        />
      </Form.Item>
      
      <Form.Item 
        label="转发结果" 
        tooltip="是否将聚合结果转发到NATS消息总线"
      >
        <Switch
          checked={action.config.forward !== false}
          onChange={(checked) => updateActionConfig(index, 'forward', checked)}
          checkedChildren="是"
          unCheckedChildren="否"
        />
      </Form.Item>
      
      {renderAddTagsInput(action, index, "聚合标签")}
    </div>
    );
  };

  // 渲染转发配置
  const renderForwardConfig = (action: ActionConfig, index: number) => (
    <div>
      <Form.Item label="转发目标">
        <Select
          value={cfgString(action.config.target_type, 'http')}
          onChange={(value) => updateActionConfig(index, 'target_type', value)}
        >
          <Option key="http" value="http">HTTP接口</Option>
          <Option key="file" value="file">文件</Option>
          <Option key="mqtt" value="mqtt">MQTT</Option>
          <Option key="kafka" value="kafka">Kafka</Option>
        </Select>
      </Form.Item>

      {action.config.target_type === 'http' && (
        <div>
          <Form.Item label="URL地址">
            <Input
              placeholder="https://api.example.com/data"
              value={cfgString(action.config.url)}
              onChange={(e) => updateActionConfig(index, 'url', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="HTTP方法">
            <Select
              value={cfgString(action.config.method, 'POST')}
              onChange={(value) => updateActionConfig(index, 'method', value)}
            >
              <Option key="POST" value="POST">POST</Option>
              <Option key="PUT" value="PUT">PUT</Option>
              <Option key="PATCH" value="PATCH">PATCH</Option>
            </Select>
          </Form.Item>
          <Form.Item label="请求头">
            <TextArea
              rows={2}
              placeholder='JSON格式，如: {"Content-Type": "application/json"}'
              value={action.config.headers ? JSON.stringify(action.config.headers, null, 2) : ''}
              onChange={(e) => {
                try {
                  const headers = JSON.parse(e.target.value || '{}');
                  updateActionConfig(index, 'headers', headers);
                } catch {
                  // 忽略JSON解析错误
                }
              }}
            />
          </Form.Item>
        </div>
      )}

      {action.config.target_type === 'file' && (
        <Form.Item label="文件路径">
          <Input
            placeholder="/var/log/iot_data.log"
            value={cfgString(action.config.path)}
            onChange={(e) => updateActionConfig(index, 'path', e.target.value)}
          />
        </Form.Item>
      )}

      {action.config.target_type === 'mqtt' && (
        <div>
          <Form.Item label="MQTT代理">
            <Input
              placeholder="tcp://localhost:1883"
              value={cfgString(action.config.broker)}
              onChange={(e) => updateActionConfig(index, 'broker', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="主题模板">
            <Input
              placeholder="iot/{{.DeviceID}}/{{.Key}}"
              value={cfgString(action.config.topic)}
              onChange={(e) => updateActionConfig(index, 'topic', e.target.value)}
            />
          </Form.Item>
        </div>
      )}

      <Row gutter={16}>
        <Col span={12}>
          <Form.Item label="批处理大小">
            <InputNumber
              placeholder="1"
              value={cfgNumber(action.config.batch_size, 1)}
              onChange={(value) => updateActionConfig(index, 'batch_size', value)}
              min={1}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item label="超时时间">
            <Input
              placeholder="30s"
              value={cfgString(action.config.timeout)}
              onChange={(e) => updateActionConfig(index, 'timeout', e.target.value)}
            />
          </Form.Item>
        </Col>
      </Row>
      
      {renderAddTagsInput(action, index, "转发标签")}
    </div>
  );

  // 渲染动作配置表单
  const renderActionConfig = (action: ActionConfig, index: number) => {
    switch (action.type) {
      case 'alert':
        return renderAlertConfig(action, index);
      case 'transform':
        return renderTransformConfig(action, index);
      case 'filter':
        return renderFilterConfig(action, index);
      case 'aggregate':
        return renderAggregateConfig(action, index);
      case 'forward':
        return renderForwardConfig(action, index);
      default:
        return <div>请选择动作类型</div>;
    }
  };

  return (
    <Card title="执行动作配置" size="small">
      <Space direction="vertical" style={{ width: '100%' }}>
        {currentActions.map((action, index) => (
          <Card
            key={index}
            size="small"
            title={
              <Space>
                {actionTypes.find(t => t.value === action.type)?.icon}
                <span>动作 {index + 1}</span>
                <Tag color="blue">
                  {actionTypes.find(t => t.value === action.type)?.label || action.type}
                </Tag>
              </Space>
            }
            extra={
              currentActions.length > 1 && (
                <Button
                  type="text"
                  danger
                  size="small"
                  icon={<DeleteOutlined />}
                  onClick={() => removeAction(index)}
                />
              )
            }
          >
            <div>
              <Form.Item label="动作类型">
                <Select
                  value={action.type}
                  onChange={(value) => {
                    // 只有在动作类型真正改变时才重置配置
                    if (action.type !== value) {
                      // 根据动作类型设置默认配置
                      let defaultConfig = {};
                      switch (value) {
                        case 'alert':
                          defaultConfig = {
                            level: 'warning',
                            message: '告警信息',
                            channels: ['console']
                          };
                          break;
                        case 'transform':
                          defaultConfig = {
                            type: 'scale',
                            parameters: { factor: 1.0 },
                            output_key: '',
                            output_type: 'float',
                            precision: 2,
                            error_action: 'default',
                            default_value: 0
                          };
                          break;
                        case 'filter':
                          defaultConfig = {
                            type: 'range',
                            min: 0,
                            max: 100,
                            drop_on_match: false
                          };
                          break;
                        case 'aggregate':
                          defaultConfig = {
                            window_size: 10,
                            window_type: 'count',
                            functions: ['avg', 'min', 'max'],
                            group_by: ['device_id'],
                            output_key: 'aggregated_value',
                            forward: true
                          };
                          break;
                        case 'forward':
                          defaultConfig = {
                            target_type: 'http',
                            url: '',
                            method: 'POST',
                            batch_size: 1,
                            timeout: '30s'
                          };
                          break;
                        default:
                          defaultConfig = {};
                      }
                      
                      // 同时更新类型和配置，确保原子性
                      const newActions = [...currentActions];
                      newActions[index] = {
                        ...newActions[index],
                        type: value,
                        config: defaultConfig
                      };
                      actionsRef.current = newActions;
                      
                      const newActionsForCallback = convertToCallbackFormat(newActions);
                      onChange?.(newActionsForCallback);
                    }
                  }}
                  style={{ width: '100%' }}
                >
                  {actionTypes.map(type => (
                    <Option key={type.value} value={type.value}>
                      <Space>
                        {type.icon}
                        {type.label}
                        <Text type="secondary">- {type.description}</Text>
                      </Space>
                    </Option>
                  ))}
                </Select>
              </Form.Item>

              {renderActionConfig(action, index)}

              <Divider size="small" />

              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item label={
                    <Space>
                      异步执行
                      <Tooltip title="是否异步执行此动作">
                        <QuestionCircleOutlined />
                      </Tooltip>
                    </Space>
                  }>
                    <Switch
                      checked={action.async}
                      onChange={(checked) => updateAction(index, 'async', checked)}
                      checkedChildren="是"
                      unCheckedChildren="否"
                    />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item label="超时时间">
                    <Input
                      placeholder="30s"
                      value={action.timeout}
                      onChange={(e) => updateAction(index, 'timeout', e.target.value)}
                    />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item label="重试次数">
                    <InputNumber
                      value={action.retry}
                      onChange={(value) => updateAction(index, 'retry', value || 0)}
                      min={0}
                      max={10}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
              </Row>
            </div>
          </Card>
        ))}

        <Button
          type="dashed"
          icon={<PlusOutlined />}
          onClick={addAction}
          style={{ width: '100%' }}
        >
          添加动作
        </Button>
      </Space>
    </Card>
  );
};

export default ActionForm;