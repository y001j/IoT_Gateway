import React, { useState, useEffect } from 'react';
import { Card, Form, Select, Input, InputNumber, Button, Space, Row, Col, Switch, Typography, Alert, Divider, Tag, Tooltip } from 'antd';
import { PlusOutlined, DeleteOutlined, InfoCircleOutlined, FilterOutlined, FunctionOutlined } from '@ant-design/icons';
import { Condition } from '../../types/rule';

const { Option } = Select;
const { Text } = Typography;
const { TextArea } = Input;

export interface ConditionBuilderProps {
  value?: Condition;
  onChange?: (condition: Condition | undefined) => void;
  availableFields?: string[];
  customFieldOptions?: Array<{ value: string; label: string; description?: string }>;
  allowedOperators?: string[];
  supportExpressions?: boolean;
  dataTypeName?: string;
}

interface ConditionBuilderState {
  condition?: Condition;
}

/**
 * 通用条件构建器组件
 * 支持简单条件、复合条件(AND/OR)、表达式条件
 */
const ConditionBuilder: React.FC<ConditionBuilderProps> = ({
  value,
  onChange,
  availableFields = ['device_id', 'key', 'value', 'timestamp', 'quality'],
  customFieldOptions = [],
  allowedOperators = ['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'startswith', 'endswith', 'regex'],
  supportExpressions = true,
  dataTypeName = '数据'
}) => {
  const [condition, setCondition] = useState<Condition | undefined>(value);

  useEffect(() => {
    setCondition(value);
  }, [value]);

  // 操作符选项
  const operatorOptions = [
    { value: 'eq', label: '等于 (=)', description: '字段值等于指定值' },
    { value: 'ne', label: '不等于 (≠)', description: '字段值不等于指定值' },
    { value: 'gt', label: '大于 (>)', description: '字段值大于指定值' },
    { value: 'gte', label: '大于等于 (≥)', description: '字段值大于等于指定值' },
    { value: 'lt', label: '小于 (<)', description: '字段值小于指定值' },
    { value: 'lte', label: '小于等于 (≤)', description: '字段值小于等于指定值' },
    { value: 'contains', label: '包含', description: '字段值包含指定文本' },
    { value: 'startswith', label: '开头匹配', description: '字段值以指定文本开头' },
    { value: 'endswith', label: '结尾匹配', description: '字段值以指定文本结尾' },
    { value: 'regex', label: '正则匹配', description: '字段值匹配正则表达式' }
  ].filter(opt => allowedOperators.includes(opt.value));

  // 条件类型选项
  const conditionTypeOptions = [
    { value: 'simple', label: '简单条件', description: '单个字段比较条件' },
    { value: 'and', label: '逻辑AND', description: '所有子条件都必须满足' },
    { value: 'or', label: '逻辑OR', description: '至少一个子条件满足' },
  ];

  if (supportExpressions) {
    conditionTypeOptions.push({
      value: 'expression',
      label: '表达式条件',
      description: '使用表达式语言进行复杂计算'
    });
  }

  // 合并字段选项
  const allFieldOptions = [
    ...availableFields.map(field => ({ value: field, label: field })),
    ...customFieldOptions
  ];

  const updateCondition = (newCondition: Condition | undefined) => {
    setCondition(newCondition);
    onChange?.(newCondition);
  };

  const handleTypeChange = (type: string) => {
    let newCondition: Condition;
    
    switch (type) {
      case 'simple':
        newCondition = {
          type: 'simple',
          field: '',
          operator: 'eq',
          value: ''
        };
        break;
      case 'and':
        newCondition = {
          type: 'and',
          and: [
            { type: 'simple', field: '', operator: 'eq', value: '' },
            { type: 'simple', field: '', operator: 'eq', value: '' }
          ]
        };
        break;
      case 'or':
        newCondition = {
          type: 'or',
          or: [
            { type: 'simple', field: '', operator: 'eq', value: '' },
            { type: 'simple', field: '', operator: 'eq', value: '' }
          ]
        };
        break;
      case 'expression':
        newCondition = {
          type: 'expression',
          expression: ''
        };
        break;
      default:
        newCondition = { type: 'simple', field: '', operator: 'eq', value: '' };
    }
    
    updateCondition(newCondition);
  };

  const handleSimpleConditionChange = (field: keyof Condition, value: any) => {
    if (!condition) return;
    
    const updated = { ...condition, [field]: value };
    updateCondition(updated);
  };

  const handleSubConditionChange = (index: number, subCondition: Condition, isAnd: boolean = true) => {
    if (!condition) return;
    
    const updated = { ...condition };
    const key = isAnd ? 'and' : 'or';
    const subConditions = updated[key] || [];
    subConditions[index] = subCondition;
    updated[key] = subConditions;
    updateCondition(updated);
  };

  const addSubCondition = (isAnd: boolean = true) => {
    if (!condition) return;
    
    const updated = { ...condition };
    const key = isAnd ? 'and' : 'or';
    const subConditions = updated[key] || [];
    // 默认添加简单条件，用户可以后续选择表达式类型
    subConditions.push({ type: 'simple', field: '', operator: 'eq', value: '' });
    updated[key] = subConditions;
    updateCondition(updated);
  };

  const removeSubCondition = (index: number, isAnd: boolean = true) => {
    if (!condition) return;
    
    const updated = { ...condition };
    const key = isAnd ? 'and' : 'or';
    const subConditions = updated[key] || [];
    subConditions.splice(index, 1);
    updated[key] = subConditions;
    updateCondition(updated);
  };

  const renderSimpleCondition = (cond?: Condition, onUpdate?: (condition: Condition) => void) => {
    const currentCondition = cond || condition;
    const handleUpdate = onUpdate || updateCondition;

    if (!currentCondition || currentCondition.type !== 'simple') return null;

    return (
      <Card size="small" style={{ backgroundColor: '#fafafa', marginBottom: 16 }}>
        <Row gutter={16}>
          <Col span={8}>
            <Form.Item label={
              <Space size="small">
                <FilterOutlined style={{ color: '#1890ff' }} />
                <span>字段</span>
              </Space>
            }>
              <Select
                placeholder="选择字段"
                value={currentCondition.field}
                onChange={(value) => handleUpdate({ ...currentCondition, field: value })}
                showSearch
                size="large"
                style={{ width: '100%' }}
                optionLabelProp="label"
              >
                {allFieldOptions.map(option => (
                  <Option key={option.value} value={option.value} label={option.label}>
                    <div style={{ padding: '6px 0', lineHeight: '1.4' }}>
                      <div><Text strong style={{ fontSize: '14px' }}>{option.label}</Text></div>
                      {option.description && (
                        <div><Text type="secondary" style={{ fontSize: '12px' }}>{option.description}</Text></div>
                      )}
                    </div>
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item label={
              <Space size="small">
                <FunctionOutlined style={{ color: '#52c41a' }} />
                <span>操作符</span>
              </Space>
            }>
              <Select
                placeholder="选择操作符"
                value={currentCondition.operator}
                onChange={(value) => handleUpdate({ ...currentCondition, operator: value })}
                size="large"
                style={{ width: '100%' }}
                optionLabelProp="label"
              >
                {operatorOptions.map(option => (
                  <Option key={option.value} value={option.value} label={option.label}>
                    <div style={{ padding: '6px 0', lineHeight: '1.4' }}>
                      <div><Text strong style={{ fontSize: '14px' }}>{option.label}</Text></div>
                      <div><Text type="secondary" style={{ fontSize: '12px' }}>{option.description}</Text></div>
                    </div>
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
          <Col span={10}>
            <Form.Item label={
              <Space size="small">
                <InfoCircleOutlined style={{ color: '#fa8c16' }} />
                <span>值</span>
              </Space>
            }>
              {currentCondition.operator === 'regex' ? (
                <Input
                  placeholder="输入正则表达式"
                  value={currentCondition.value}
                  onChange={(e) => handleUpdate({ ...currentCondition, value: e.target.value })}
                  size="large"
                  style={{ fontFamily: 'Monaco, Consolas, monospace' }}
                />
              ) : ['gt', 'gte', 'lt', 'lte'].includes(currentCondition.operator || '') ? (
                <InputNumber
                  placeholder="输入数字值"
                  value={currentCondition.value as number}
                  onChange={(value) => handleUpdate({ ...currentCondition, value })}
                  style={{ width: '100%' }}
                  size="large"
                  precision={2}
                />
              ) : (
                <Input
                  placeholder="输入比较值"
                  value={currentCondition.value}
                  onChange={(e) => handleUpdate({ ...currentCondition, value: e.target.value })}
                  size="large"
                />
              )}
            </Form.Item>
          </Col>
        </Row>
        
        {/* 显示当前条件的预览 */}
        {currentCondition.field && currentCondition.operator && currentCondition.value !== '' && (
          <Alert
            message="条件预览"
            description={
              <Tag color="blue" style={{ fontSize: '12px' }}>
                {currentCondition.field} {operatorOptions.find(op => op.value === currentCondition.operator)?.label} {currentCondition.value}
              </Tag>
            }
            type="info"
            showIcon
            style={{ marginTop: 8 }}
            size="small"
          />
        )}
      </Card>
    );
  };

  const renderCompoundCondition = (isAnd: boolean = true) => {
    if (!condition) return null;
    
    const key = isAnd ? 'and' : 'or';
    const subConditions = condition[key] || [];
    const label = isAnd ? 'AND条件' : 'OR条件';
    const description = isAnd ? '所有子条件都必须满足' : '至少一个子条件满足';
    const tagColor = isAnd ? 'blue' : 'green';

    return (
      <div>
        <Alert
          message={
            <Space>
              <Tag color={tagColor} icon={isAnd ? <FilterOutlined /> : <FunctionOutlined />}>
                {label}
              </Tag>
              <span>{description}</span>
              {supportExpressions && (
                <Tag color="green" size="small">支持表达式</Tag>
              )}
            </Space>
          }
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        
        {subConditions.map((subCondition, index) => (
          <Card
            key={index}
            size="small"
            style={{ 
              marginBottom: 16,
              border: `2px dashed ${isAnd ? '#1890ff' : '#52c41a'}`,
              borderRadius: 8
            }}
            title={
              <Space>
                <Tag color={tagColor}>条件 {index + 1}</Tag>
                {isAnd ? 'AND' : 'OR'}
              </Space>
            }
            extra={
              subConditions.length > 1 && (
                <Tooltip title="删除这个条件">
                  <Button
                    type="link"
                    icon={<DeleteOutlined />}
                    danger
                    size="small"
                    onClick={() => removeSubCondition(index, isAnd)}
                  >
                    删除
                  </Button>
                </Tooltip>
              )
            }
          >
            <ConditionBuilder
              value={subCondition}
              onChange={(updated) => updated && handleSubConditionChange(index, updated, isAnd)}
              availableFields={availableFields}
              customFieldOptions={customFieldOptions}
              allowedOperators={allowedOperators}
              supportExpressions={supportExpressions} // 继承父级的表达式支持设置
              dataTypeName={dataTypeName}
            />
          </Card>
        ))}
        
        <Button
          type="dashed"
          icon={<PlusOutlined />}
          onClick={() => addSubCondition(isAnd)}
          style={{ 
            width: '100%',
            height: '48px',
            borderColor: isAnd ? '#1890ff' : '#52c41a',
            color: isAnd ? '#1890ff' : '#52c41a'
          }}
          size="large"
        >
          添加{isAnd ? 'AND' : 'OR'}条件
        </Button>
      </div>
    );
  };

  const renderExpressionCondition = () => {
    if (!condition || condition.type !== 'expression') return null;

    return (
      <Card size="small" style={{ backgroundColor: '#f6ffed', marginBottom: 16 }}>
        <Alert
          message="表达式模式"
          description="使用表达式语言进行复杂计算，支持数学运算、字符串操作和逻辑判断"
          type="success"
          showIcon
          style={{ marginBottom: 16 }}
        />
        
        <Form.Item 
          label={
            <Space>
              <FunctionOutlined style={{ color: '#52c41a' }} />
              <span>{dataTypeName}表达式</span>
            </Space>
          }
        >
          <TextArea
            placeholder={`输入${dataTypeName}表达式，例如：value > 30 && contains(device_id, "sensor")`}
            value={condition.expression}
            onChange={(e) => handleSimpleConditionChange('expression', e.target.value)}
            rows={4}
            style={{
              fontFamily: 'Monaco, Consolas, "Courier New", monospace',
              fontSize: '13px',
              lineHeight: '1.5'
            }}
          />
        </Form.Item>
        
        <div style={{ marginTop: 12 }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            <InfoCircleOutlined style={{ marginRight: 4 }} />
            示例: value {'>'} 30 {'&&'} contains(device_id, &quot;sensor&quot;) {'||'} temperature {'<'} 0
          </Text>
        </div>
      </Card>
    );
  };

  if (!condition) {
    return (
      <Card 
        title={
          <Space>
            <FilterOutlined style={{ color: '#1890ff' }} />
            <span>设置条件</span>
          </Space>
        }
        style={{ 
          border: '2px dashed #d9d9d9',
          borderRadius: 8,
          backgroundColor: '#fafafa'
        }}
      >
        <Alert
          message="选择条件类型开始配置"
          description="选择适合的条件类型来定义规则的触发条件"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        
        <Form.Item label={
          <Space>
            <FunctionOutlined style={{ color: '#52c41a' }} />
            <span>条件类型</span>
          </Space>
        }>
          <Select
            placeholder="选择条件类型"
            onChange={handleTypeChange}
            size="large"
            style={{ width: '100%' }}
            optionLabelProp="label"
          >
            {conditionTypeOptions.map(option => (
              <Option key={option.value} value={option.value} label={option.label}>
                <div style={{ padding: '6px 0', lineHeight: '1.4' }}>
                  <div><Text strong style={{ fontSize: '14px' }}>{option.label}</Text></div>
                  <div><Text type="secondary" style={{ fontSize: '12px' }}>{option.description}</Text></div>
                </div>
              </Option>
            ))}
          </Select>
        </Form.Item>
      </Card>
    );
  }

  return (
    <Card 
      title={
        <Space>
          <FilterOutlined style={{ color: '#1890ff' }} />
          <span>规则条件</span>
          <Tag color="blue">{dataTypeName}</Tag>
        </Space>
      }
      style={{ 
        border: '1px solid #e8f4fd',
        borderRadius: 8,
        backgroundColor: '#fafcff'
      }}
    >
      <Form.Item label={
        <Space>
          <FunctionOutlined style={{ color: '#52c41a' }} />
          <span>条件类型</span>
        </Space>
      }>
        <Select
          value={condition.type}
          onChange={handleTypeChange}
          size="large"
          style={{ width: '100%' }}
          optionLabelProp="label"
        >
          {conditionTypeOptions.map(option => (
            <Option key={option.value} value={option.value} label={option.label}>
              <div style={{ padding: '6px 0', lineHeight: '1.4' }}>
                <div><Text strong style={{ fontSize: '14px' }}>{option.label}</Text></div>
                <div><Text type="secondary" style={{ fontSize: '12px' }}>{option.description}</Text></div>
              </div>
            </Option>
          ))}
        </Select>
      </Form.Item>

      <Divider style={{ margin: '16px 0' }} />

      {condition.type === 'simple' && renderSimpleCondition()}
      {condition.type === 'and' && renderCompoundCondition(true)}
      {condition.type === 'or' && renderCompoundCondition(false)}
      {condition.type === 'expression' && renderExpressionCondition()}
    </Card>
  );
};

export default ConditionBuilder;