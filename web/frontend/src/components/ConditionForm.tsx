import React, { useState, useEffect } from 'react';
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
  Tooltip
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  QuestionCircleOutlined
} from '@ant-design/icons';
import type { Condition } from '../types/rule';

const { Option } = Select;
const { TextArea } = Input;

interface ConditionFormProps {
  value?: Condition;
  onChange?: (value: Condition) => void;
}

interface SimpleCondition {
  field: string;
  operator: string;
  value: any;
}

const ConditionForm: React.FC<ConditionFormProps> = ({ value, onChange }) => {
  const [conditionType, setConditionType] = useState<string>('simple');
  const [simpleCondition, setSimpleCondition] = useState<SimpleCondition>({
    field: '',
    operator: 'eq',
    value: ''
  });
  const [andConditions, setAndConditions] = useState<SimpleCondition[]>([]);
  const [orConditions, setOrConditions] = useState<SimpleCondition[]>([]);
  const [expression, setExpression] = useState<string>('');

  // 操作符选项
  const operators = [
    { value: 'eq', label: '等于 (=)', description: '字段值等于指定值' },
    { value: 'ne', label: '不等于 (≠)', description: '字段值不等于指定值' },
    { value: 'gt', label: '大于 (>)', description: '字段值大于指定值' },
    { value: 'gte', label: '大于等于 (≥)', description: '字段值大于等于指定值' },
    { value: 'lt', label: '小于 (<)', description: '字段值小于指定值' },
    { value: 'lte', label: '小于等于 (≤)', description: '字段值小于等于指定值' },
    { value: 'contains', label: '包含', description: '字符串包含子字符串' },
    { value: 'regex', label: '正则匹配', description: '正则表达式匹配' },
    { value: 'in', label: '在数组中', description: '值在指定数组中' },
    { value: 'exists', label: '字段存在', description: '检查字段是否存在' }
  ];

  // 常用字段
  const commonFields = [
    'device_id', 'key', 'value', 'timestamp', 'quality', 'unit',
    'temperature', 'humidity', 'pressure', 'status'
  ];

  // 初始化表单值
  useEffect(() => {
    if (value) {
      if (value.type === 'simple') {
        setConditionType('simple');
        setSimpleCondition({
          field: value.field || '',
          operator: value.operator || 'eq',
          value: value.value || ''
        });
      } else if (value.type === 'and' && value.and) {
        setConditionType('and');
        setAndConditions(value.and.map(cond => ({
          field: cond.field || '',
          operator: cond.operator || 'eq',
          value: cond.value || ''
        })));
      } else if (value.type === 'or' && value.or) {
        setConditionType('or');
        setOrConditions(value.or.map(cond => ({
          field: cond.field || '',
          operator: cond.operator || 'eq',
          value: cond.value || ''
        })));
      } else if (value.type === 'expression') {
        setConditionType('expression');
        setExpression(value.expression || '');
      }
    }
  }, [value]);

  // 构建条件对象
  const buildCondition = (): Condition => {
    if (conditionType === 'simple') {
      return {
        type: 'simple',
        field: simpleCondition.field,
        operator: simpleCondition.operator,
        value: simpleCondition.value
      };
    } else if (conditionType === 'and') {
      return {
        type: 'and',
        and: andConditions.map(cond => ({
          type: 'simple',
          field: cond.field,
          operator: cond.operator,
          value: cond.value
        }))
      };
    } else if (conditionType === 'or') {
      return {
        type: 'or',
        or: orConditions.map(cond => ({
          type: 'simple',
          field: cond.field,
          operator: cond.operator,
          value: cond.value
        }))
      };
    } else if (conditionType === 'expression') {
      return {
        type: 'expression',
        expression: expression
      };
    }
    return { type: 'simple', field: '', operator: 'eq', value: '' };
  };

  // 触发变更
  const triggerChange = () => {
    const condition = buildCondition();
    onChange?.(condition);
  };

  // 处理条件类型变更
  const handleTypeChange = (type: string) => {
    setConditionType(type);
    // 重置其他状态
    if (type === 'and' && andConditions.length === 0) {
      setAndConditions([
        { field: '', operator: 'eq', value: '' },
        { field: '', operator: 'eq', value: '' }
      ]);
    } else if (type === 'or' && orConditions.length === 0) {
      setOrConditions([
        { field: '', operator: 'eq', value: '' },
        { field: '', operator: 'eq', value: '' }
      ]);
    }
    setTimeout(triggerChange, 0);
  };

  // 渲染简单条件表单
  const renderSimpleCondition = (
    condition: SimpleCondition,
    onChange: (index: number, field: keyof SimpleCondition, value: any) => void,
    index: number = 0
  ) => (
    <Row gutter={8} key={index}>
      <Col span={7}>
        <Select
          placeholder="选择字段"
          value={condition.field}
          onChange={(value) => onChange(index, 'field', value)}
          style={{ width: '100%' }}
          showSearch
          allowClear
        >
          {commonFields.map(field => (
            <Option key={field} value={field}>{field}</Option>
          ))}
        </Select>
      </Col>
      <Col span={6}>
        <Select
          value={condition.operator}
          onChange={(value) => onChange(index, 'operator', value)}
          style={{ width: '100%' }}
        >
          {operators.map(op => (
            <Option key={op.value} value={op.value} title={op.description}>
              {op.label}
            </Option>
          ))}
        </Select>
      </Col>
      <Col span={8}>
        {['exists'].includes(condition.operator) ? (
          <Input disabled placeholder="无需填写" />
        ) : ['in'].includes(condition.operator) ? (
          <Select
            mode="tags"
            placeholder="输入多个值"
            value={Array.isArray(condition.value) ? condition.value : []}
            onChange={(value) => onChange(index, 'value', value)}
            style={{ width: '100%' }}
          />
        ) : ['gt', 'gte', 'lt', 'lte'].includes(condition.operator) ? (
          <InputNumber
            placeholder="数值"
            value={condition.value}
            onChange={(value) => onChange(index, 'value', value)}
            style={{ width: '100%' }}
          />
        ) : (
          <Input
            placeholder="比较值"
            value={condition.value}
            onChange={(e) => onChange(index, 'value', e.target.value)}
          />
        )}
      </Col>
      {index > 1 && (
        <Col span={3}>
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            onClick={() => {
              if (conditionType === 'and') {
                const newConditions = [...andConditions];
                newConditions.splice(index, 1);
                setAndConditions(newConditions);
              } else if (conditionType === 'or') {
                const newConditions = [...orConditions];
                newConditions.splice(index, 1);
                setOrConditions(newConditions);
              }
              setTimeout(triggerChange, 0);
            }}
          />
        </Col>
      )}
    </Row>
  );

  // 处理简单条件变更
  const handleSimpleConditionChange = (field: keyof SimpleCondition, value: any) => {
    const newCondition = { ...simpleCondition, [field]: value };
    setSimpleCondition(newCondition);
    setTimeout(triggerChange, 0);
  };

  // 处理AND条件变更
  const handleAndConditionChange = (index: number, field: keyof SimpleCondition, value: any) => {
    const newConditions = [...andConditions];
    newConditions[index] = { ...newConditions[index], [field]: value };
    setAndConditions(newConditions);
    setTimeout(triggerChange, 0);
  };

  // 处理OR条件变更
  const handleOrConditionChange = (index: number, field: keyof SimpleCondition, value: any) => {
    const newConditions = [...orConditions];
    newConditions[index] = { ...newConditions[index], [field]: value };
    setOrConditions(newConditions);
    setTimeout(triggerChange, 0);
  };

  return (
    <Card title="触发条件配置" size="small">
      <Form layout="vertical">
        <Form.Item label="条件类型">
          <Select value={conditionType} onChange={handleTypeChange}>
            <Option value="simple">
              <Space>
                <Tag color="blue">简单</Tag>
                单个字段条件
              </Space>
            </Option>
            <Option value="and">
              <Space>
                <Tag color="green">逻辑与</Tag>
                所有条件都满足
              </Space>
            </Option>
            <Option value="or">
              <Space>
                <Tag color="orange">逻辑或</Tag>
                任一条件满足
              </Space>
            </Option>
            <Option value="expression">
              <Space>
                <Tag color="purple">表达式</Tag>
                自定义表达式
              </Space>
            </Option>
          </Select>
        </Form.Item>

        {conditionType === 'simple' && (
          <Form.Item label="条件设置">
            {renderSimpleCondition(simpleCondition, (_, field, value) => 
              handleSimpleConditionChange(field, value)
            )}
          </Form.Item>
        )}

        {conditionType === 'and' && (
          <Form.Item label={
            <Space>
              逻辑与条件
              <Tooltip title="所有条件都必须满足才会触发规则">
                <QuestionCircleOutlined />
              </Tooltip>
            </Space>
          }>
            <Space direction="vertical" style={{ width: '100%' }}>
              {andConditions.map((condition, index) => (
                <div key={index}>
                  {index > 0 && (
                    <div style={{ textAlign: 'center', margin: '8px 0' }}>
                      <Tag color="green">且</Tag>
                    </div>
                  )}
                  {renderSimpleCondition(condition, handleAndConditionChange, index)}
                </div>
              ))}
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={() => {
                  setAndConditions([...andConditions, { field: '', operator: 'eq', value: '' }]);
                  setTimeout(triggerChange, 0);
                }}
                style={{ width: '100%' }}
              >
                添加条件
              </Button>
            </Space>
          </Form.Item>
        )}

        {conditionType === 'or' && (
          <Form.Item label={
            <Space>
              逻辑或条件
              <Tooltip title="任意一个条件满足就会触发规则">
                <QuestionCircleOutlined />
              </Tooltip>
            </Space>
          }>
            <Space direction="vertical" style={{ width: '100%' }}>
              {orConditions.map((condition, index) => (
                <div key={index}>
                  {index > 0 && (
                    <div style={{ textAlign: 'center', margin: '8px 0' }}>
                      <Tag color="orange">或</Tag>
                    </div>
                  )}
                  {renderSimpleCondition(condition, handleOrConditionChange, index)}
                </div>
              ))}
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={() => {
                  setOrConditions([...orConditions, { field: '', operator: 'eq', value: '' }]);
                  setTimeout(triggerChange, 0);
                }}
                style={{ width: '100%' }}
              >
                添加条件
              </Button>
            </Space>
          </Form.Item>
        )}

        {conditionType === 'expression' && (
          <Form.Item 
            label={
              <Space>
                表达式条件
                <Tooltip title="使用表达式语言编写复杂条件，如: temperature > 30 && humidity < 60">
                  <QuestionCircleOutlined />
                </Tooltip>
              </Space>
            }
          >
            <TextArea
              rows={3}
              placeholder="例如: temperature > 30 && humidity < 60"
              value={expression}
              onChange={(e) => {
                setExpression(e.target.value);
                setTimeout(triggerChange, 0);
              }}
            />
            <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
              支持的运算符：&amp;&amp; (且), || (或), ! (非), ==, !=, &gt;, &gt;=, &lt;, &lt;=
            </div>
          </Form.Item>
        )}
      </Form>
    </Card>
  );
};

export default ConditionForm;