import React, { useState, useEffect } from 'react';
import { Modal, Card, Form, Input, Select, Space, Typography, Row, Col, Divider, Button, InputNumber, Switch, Tabs, Alert, Tag } from 'antd';
import { FunctionOutlined, InfoCircleOutlined, EyeOutlined, EditOutlined, CodeOutlined, CheckCircleOutlined } from '@ant-design/icons';
import { useBaseRuleEditor } from './base/BaseRuleEditor';
import ConditionBuilder from './base/ConditionBuilder';
import ActionFormBuilder from './base/ActionFormBuilder';
import ExpressionEditor from './base/ExpressionEditor';
import { Rule, Condition, Action } from '../types/rule';

const { Option } = Select;
const { Text } = Typography;
const { TextArea } = Input;

export interface GenericVectorRuleEditorProps {
  visible: boolean;
  onClose: () => void;
  onSave: (rule: Rule) => Promise<void>;
  rule?: Rule;
}

/**
 * 通用向量规则编辑器
 * 处理任意维度的向量数据和数组数据的规则
 */
const GenericVectorRuleEditor: React.FC<GenericVectorRuleEditorProps> = ({
  visible,
  onClose,
  onSave,
  rule
}) => {
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
    title: '通用向量规则编辑器',
    dataTypeName: '通用向量数据'
  });

  // 自定义的取消处理函数，重置所有状态
  const handleCancel = () => {
    // 重置所有本地状态
    setConditions(undefined);
    setActions([]);
    setActiveTab('basic');
    setJsonValue('');
    setJsonError('');
    
    // 直接调用 onClose 而不是 baseHandleCancel，避免可能的双重调用
    onClose();
  };

  const [conditions, setConditions] = useState<Condition | undefined>(rule?.conditions);
  const [actions, setActions] = useState<Action[]>(rule?.actions || []);
  const [activeTab, setActiveTab] = useState<'basic' | 'conditions' | 'actions' | 'preview' | 'json'>('basic');
  const [jsonValue, setJsonValue] = useState<string>('');
  const [jsonError, setJsonError] = useState<string>('');

  // 智能数据类型推断
  const inferDataTypeFromRule = (rule: Rule): string => {
    if (rule.data_type) {
      if (typeof rule.data_type === 'string') {
        return rule.data_type;
      } else if (typeof rule.data_type === 'object' && rule.data_type !== null) {
        return rule.data_type.type || 'vector_generic';
      }
    }
    
    if (rule.tags) {
      const tagDataType = rule.tags['data_type'] || rule.tags['data_category'];
      if (tagDataType) return tagDataType;
    }
    
    // 根据规则名称和描述推断
    const nameDesc = `${rule.name || ''} ${rule.description || ''}`.toLowerCase();
    if (nameDesc.includes('vector') || nameDesc.includes('向量') || nameDesc.includes('array') || nameDesc.includes('数组')) {
      return 'vector_generic';
    }
    if (nameDesc.includes('generic') || nameDesc.includes('通用') || nameDesc.includes('sequence') || nameDesc.includes('序列')) {
      return 'vector_generic';
    }
    
    return 'vector_generic'; // 默认为通用向量数据
  };

  // 计算最终数据类型
  const finalDataType = rule ? inferDataTypeFromRule(rule) : 'vector_generic';

  useEffect(() => {
    if (visible && rule) {
      setConditions(rule.conditions);
      setActions(rule.actions || []);
      updateJsonValue();
    }
  }, [visible, rule]);

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
        tags: rule?.tags || {},
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
      if (parsedRule.name && parsedRule.conditions && parsedRule.actions) {
        setJsonError('');
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

  // 通用向量特定字段
  const genericVectorFields = [
    { value: 'sensor_vector.dimension', label: '向量维度', description: '向量的维数' },
    { value: 'sensor_vector.magnitude', label: '向量模长', description: '向量的欧几里德长度' },
    { value: 'sensor_vector.mean', label: '向量均值', description: '所有元素的平均值' },
    { value: 'sensor_vector.max', label: '向量最大值', description: '所有元素的最大值' },
    { value: 'sensor_vector.min', label: '向量最小值', description: '所有元素的最小值' },
    
    { value: 'signal_array.length', label: '数组长度', description: '数组元素个数' },
    { value: 'signal_array.sum', label: '数组和', description: '所有元素的总和' },
    { value: 'signal_array.avg', label: '数组平均值', description: '所有元素的平均值' },
    
    { value: 'measurement_vector.stddev', label: '向量标准差', description: '元素的标准偏差' },
    { value: 'measurement_vector.variance', label: '向量方差', description: '元素的方差' },
    
    { value: 'data_sequence[0]', label: '第一个元素', description: '向量/数组的第一个元素' },
    { value: 'data_sequence[1]', label: '第二个元素', description: '向量/数组的第二个元素' },
    { value: 'data_sequence[-1]', label: '最后一个元素', description: '向量/数组的最后一个元素' },
    
    { value: 'feature_vector.norm', label: '向量范数', description: '向量的2范数' },
    { value: 'values.size', label: '元素数量', description: '向量中元素的数量' }
  ];

  // 通用向量特定动作
  const genericVectorActions = [
    {
      value: 'generic_vector_transform',
      label: '通用向量转换',
      description: '向量模长、均值、元素提取等通用操作',
      configSchema: {
        transform_type: {
          type: 'string' as const,
          label: '转换类型',
          required: true,
          options: [
            { value: 'generic_magnitude', label: '通用向量模长' },
            { value: 'vector_mean', label: '向量元素均值' },
            { value: 'vector_sum', label: '向量元素求和' },
            { value: 'vector_max', label: '向量最大值' },
            { value: 'vector_min', label: '向量最小值' },
            { value: 'vector_stddev', label: '向量标准差' },
            { value: 'element_extract', label: '元素提取' },
            { value: 'subvector_extract', label: '子向量提取' }
          ]
        },
        vector_field: {
          type: 'string' as const,
          label: '向量字段',
          required: true,
          placeholder: 'sensor_vector, signal_array等',
          description: '要处理的向量字段名'
        },
        element_index: {
          type: 'number' as const,
          label: '元素索引',
          placeholder: '0, 1, 2...',
          description: '用于元素提取的索引位置',
          conditionalDisplay: {
            dependsOn: 'transform_type',
            showWhen: ['element_extract']
          }
        },
        start_index: {
          type: 'number' as const,
          label: '开始索引',
          placeholder: '子向量开始位置',
          description: '子向量提取的开始位置',
          conditionalDisplay: {
            dependsOn: 'transform_type',
            showWhen: ['subvector_extract']
          }
        },
        end_index: {
          type: 'number' as const,
          label: '结束索引',
          placeholder: '子向量结束位置',
          description: '子向量提取的结束位置',
          conditionalDisplay: {
            dependsOn: 'transform_type',
            showWhen: ['subvector_extract']
          }
        },
        output_key: {
          type: 'string' as const,
          label: '输出字段',
          placeholder: '转换后的字段名',
          description: '转换结果存储的字段名'
        }
      }
    },
    {
      value: 'generic_vector_expression',
      label: '通用向量表达式操作',
      description: '使用表达式对通用向量数据进行复杂计算和处理',
      configSchema: {
        expression: {
          type: 'textarea' as const,
          label: '表达式',
          required: true,
          placeholder: 'genericVectorMagnitude(sensor_vector) + vectorMean(signal_array)',
          description: '支持通用向量函数和数学运算的表达式'
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
    },
    {
      value: 'generic_vector_alert',
      label: '通用向量告警',
      description: '基于向量统计特性的告警',
      configSchema: {
        alert_type: {
          type: 'string' as const,
          label: '告警类型',
          options: [
            { value: 'magnitude_threshold', label: '模长阈值告警' },
            { value: 'mean_threshold', label: '均值阈值告警' },
            { value: 'element_threshold', label: '元素阈值告警' },
            { value: 'dimension_check', label: '维度检查告警' },
            { value: 'variance_anomaly', label: '方差异常告警' }
          ]
        },
        threshold: {
          type: 'number' as const,
          label: '告警阈值',
          required: true,
          placeholder: '触发告警的数值阈值'
        },
        message: {
          type: 'textarea' as const,
          label: '告警消息',
          required: true,
          placeholder: '设备{{.DeviceID}}向量{{.VectorField}}异常: 当前值{{.CurrentValue}}'
        }
      }
    }
  ];

  // 通用向量表达式函数
  const genericVectorFunctions = [
    {
      name: 'genericVectorMagnitude',
      description: '计算通用向量模长',
      syntax: 'genericVectorMagnitude(vector)',
      example: 'genericVectorMagnitude(sensor_vector) > 10.5',
      category: '向量函数',
      parameters: ['vector']
    },
    {
      name: 'vectorMean',
      description: '计算向量元素均值',
      syntax: 'vectorMean(vector)',
      example: 'vectorMean(signal_array) > 5.0',
      category: '向量函数',
      parameters: ['vector']
    },
    {
      name: 'vectorMax',
      description: '获取向量最大元素',
      syntax: 'vectorMax(vector)',
      example: 'vectorMax(measurement_vector) < 100',
      category: '向量函数',
      parameters: ['vector']
    },
    {
      name: 'vectorMin',
      description: '获取向量最小元素',
      syntax: 'vectorMin(vector)',
      example: 'vectorMin(measurement_vector) > -50',
      category: '向量函数',
      parameters: ['vector']
    },
    {
      name: 'vectorStddev',
      description: '计算向量标准差',
      syntax: 'vectorStddev(vector)',
      example: 'vectorStddev(sensor_data) > 2.0',
      category: '向量函数',
      parameters: ['vector']
    },
    {
      name: 'vectorDimension',
      description: '获取向量维度',
      syntax: 'vectorDimension(vector)',
      example: 'vectorDimension(feature_vector) == 128',
      category: '向量函数',
      parameters: ['vector']
    },
    {
      name: 'vectorElement',
      description: '访问向量指定元素',
      syntax: 'vectorElement(vector, index)',
      example: 'vectorElement(data_sequence, 0) > 0',
      category: '向量函数',
      parameters: ['vector', 'index']
    }
  ];

  // 通用向量变量
  const genericVectorVariables = [
    { name: 'sensor_vector', description: '传感器向量数据', type: 'vector', example: 'genericVectorMagnitude(sensor_vector) > 5.0' },
    { name: 'signal_array', description: '信号数组数据', type: 'array', example: 'vectorMean(signal_array) > 0' },
    { name: 'measurement_vector', description: '测量向量', type: 'vector', example: 'vectorMax(measurement_vector) < 100' },
    { name: 'data_sequence', description: '数据序列', type: 'array', example: 'vectorElement(data_sequence, 0) > threshold' },
    { name: 'feature_vector', description: '特征向量', type: 'vector', example: 'vectorDimension(feature_vector) == 64' },
    { name: 'values', description: '数值向量', type: 'vector', example: 'vectorStddev(values) < 1.0' }
  ];

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

  const renderGenericVectorConfig = () => (
    <Card title={<Space><FunctionOutlined />通用向量配置</Space>} style={{ marginTop: 16 }}>
      <Alert
        message="通用向量数据类型"
        description="此数据类型专门处理任意维度的向量、数组和序列数据，支持丰富的统计和数学运算功能"
        type="info"
        showIcon
      />
    </Card>
  );

  const renderConditionsSection = () => (
    <Card title="通用向量条件" style={{ marginTop: 16 }}>
      <ConditionBuilder
        value={conditions}
        onChange={setConditions}
        availableFields={['device_id', 'key', 'value', 'timestamp']}
        customFieldOptions={genericVectorFields}
        allowedOperators={['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'regex']}
        supportExpressions={true}
        dataTypeName="通用向量"
      />
      
      <div style={{ marginTop: 16 }}>
        <ExpressionEditor
          value={conditions?.type === 'expression' ? conditions.expression : ''}
          onChange={(expr) => setConditions({ type: 'expression', expression: expr })}
          dataType="vector_generic"
          availableFunctions={genericVectorFunctions}
          availableVariables={genericVectorVariables}
          placeholder="输入通用向量表达式，例如：genericVectorMagnitude(sensor_vector) > 10.0"
          rows={3}
        />
      </div>
    </Card>
  );

  const renderActionsSection = () => (
    <Card title="通用向量动作" style={{ marginTop: 16 }}>
      <ActionFormBuilder
        value={actions}
        onChange={setActions}
        availableActionTypes={[]} // 移除普通动作类型，只使用专门的通用向量动作
        customActionOptions={genericVectorActions}
        dataTypeName="通用向量"
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
                  <Input placeholder="请输入通用向量规则名称" />
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
                placeholder="请输入通用向量规则描述" 
                rows={3}
              />
            </Form.Item>
            
            <Form.Item
              label="启用状态"
              name="enabled"
              valuePropName="checked"
            >
              <Switch checkedChildren="启用" unCheckedChildren="禁用" />
            </Form.Item>
          </Form>
          
          {/* 通用向量配置 */}
          {renderGenericVectorConfig()}
        </Card>
      )
    },
    {
      key: 'conditions',
      label: <Space><FunctionOutlined />向量条件</Space>,
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
          <FunctionOutlined style={{ color: '#fa8c16' }} />
          <span>通用向量规则编辑器</span>
          <Tag color="orange">通用向量数据</Tag>
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

export default GenericVectorRuleEditor;