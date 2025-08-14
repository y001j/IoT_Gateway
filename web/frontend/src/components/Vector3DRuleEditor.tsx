import React, { useState, useEffect } from 'react';
import { Modal, Card, Form, Input, Select, Space, Typography, Row, Col, Divider, Button, InputNumber, Switch, Tabs, Alert, Tag } from 'antd';
import { ThunderboltOutlined, InfoCircleOutlined, EyeOutlined, EditOutlined, CodeOutlined, CheckCircleOutlined, FunctionOutlined } from '@ant-design/icons';
import { useBaseRuleEditor } from './base/BaseRuleEditor';
import ConditionBuilder from './base/ConditionBuilder';
import ActionFormBuilder from './base/ActionFormBuilder';
import ExpressionEditor from './base/ExpressionEditor';
import { Rule, Condition, Action } from '../types/rule';

const { Option } = Select;
const { Text } = Typography;
const { TextArea } = Input;

export interface Vector3DRuleEditorProps {
  visible: boolean;
  onClose: () => void;
  onSave: (rule: Rule) => Promise<void>;
  rule?: Rule;
}

/**
 * 3D向量规则编辑器
 * 专门处理3D向量数据（加速度、速度、位置、力等）的规则
 */
const Vector3DRuleEditor: React.FC<Vector3DRuleEditorProps> = ({
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
    title: '3D向量规则编辑器',
    dataTypeName: '3D向量数据'
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
        return rule.data_type.type || 'vector3d';
      }
    }
    
    if (rule.tags) {
      const tagDataType = rule.tags['data_type'] || rule.tags['data_category'];
      if (tagDataType) return tagDataType;
    }
    
    // 根据规则名称和描述推断
    const nameDesc = `${rule.name || ''} ${rule.description || ''}`.toLowerCase();
    if (nameDesc.includes('vector3d') || nameDesc.includes('三轴') || nameDesc.includes('xyz') || nameDesc.includes('三维')) {
      return 'vector3d';
    }
    if (nameDesc.includes('vector') || nameDesc.includes('向量') || nameDesc.includes('acceleration') || nameDesc.includes('加速度')) {
      return 'vector3d';
    }
    
    return 'vector3d'; // 默认为三轴向量数据
  };

  // 计算最终数据类型
  const finalDataType = rule ? inferDataTypeFromRule(rule) : 'vector3d';

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
        tags: {
          ...rule?.tags,
          data_type: finalDataType,
          data_category: 'vector'
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

  // 3D向量特定字段
  const vector3DFields = [
    { value: 'acceleration.x', label: '加速度X分量', description: '加速度向量的X轴分量 (m/s²)' },
    { value: 'acceleration.y', label: '加速度Y分量', description: '加速度向量的Y轴分量 (m/s²)' },
    { value: 'acceleration.z', label: '加速度Z分量', description: '加速度向量的Z轴分量 (m/s²)' },
    { value: 'acceleration.magnitude', label: '加速度模长', description: '加速度向量的模长' },
    
    { value: 'velocity.x', label: '速度X分量', description: '速度向量的X轴分量 (m/s)' },
    { value: 'velocity.y', label: '速度Y分量', description: '速度向量的Y轴分量 (m/s)' },
    { value: 'velocity.z', label: '速度Z分量', description: '速度向量的Z轴分量 (m/s)' },
    { value: 'velocity.magnitude', label: '速度模长', description: '速度向量的模长' },
    
    { value: 'position.x', label: '位置X坐标', description: '位置向量的X轴坐标 (m)' },
    { value: 'position.y', label: '位置Y坐标', description: '位置向量的Y轴坐标 (m)' },
    { value: 'position.z', label: '位置Z坐标', description: '位置向量的Z轴坐标 (m)' },
    
    { value: 'force.x', label: '力X分量', description: '力向量的X轴分量 (N)' },
    { value: 'force.y', label: '力Y分量', description: '力向量的Y轴分量 (N)' },
    { value: 'force.z', label: '力Z分量', description: '力向量的Z轴分量 (N)' },
    { value: 'force.magnitude', label: '合力大小', description: '力向量的模长' },
    
    { value: 'gyroscope.x', label: '陀螺仪X轴', description: '陀螺仪X轴角速度 (rad/s)' },
    { value: 'gyroscope.y', label: '陀螺仪Y轴', description: '陀螺仪Y轴角速度 (rad/s)' },
    { value: 'gyroscope.z', label: '陀螺仪Z轴', description: '陀螺仪Z轴角速度 (rad/s)' },
    
    { value: 'magnetometer.x', label: '磁力计X轴', description: '磁力计X轴测量值 (μT)' },
    { value: 'magnetometer.y', label: '磁力计Y轴', description: '磁力计Y轴测量值 (μT)' },
    { value: 'magnetometer.z', label: '磁力计Z轴', description: '磁力计Z轴测量值 (μT)' }
  ];

  // 3D向量特定动作
  const vector3DActions = [
    {
      value: 'vector3d_transform',
      label: '3D向量转换',
      description: '向量模长计算、分量提取、向量归一化等',
      configSchema: {
        transform_type: {
          type: 'string' as const,
          label: '转换类型',
          required: true,
          options: [
            { value: 'vector_magnitude', label: '向量模长计算' },
            { value: 'component_extract', label: '分量提取' },
            { value: 'vector_normalize', label: '向量归一化' },
            { value: 'vector_cross_product', label: '叉积计算' },
            { value: 'vector_dot_product', label: '点积计算' }
          ]
        },
        vector_field: {
          type: 'string' as const,
          label: '向量字段',
          required: true,
          placeholder: 'acceleration, velocity, position等',
          description: '要处理的向量字段名'
        },
        output_key: {
          type: 'string' as const,
          label: '输出字段',
          placeholder: '转换后的字段名',
          description: '转换结果存储的字段名'
        },
        component: {
          type: 'string' as const,
          label: '分量选择',
          options: [
            { value: 'x', label: 'X分量' },
            { value: 'y', label: 'Y分量' },
            { value: 'z', label: 'Z分量' }
          ],
          description: '用于分量提取时选择特定分量',
          conditionalDisplay: {
            dependsOn: 'transform_type',
            showWhen: ['component_extract']
          }
        },
        precision: {
          type: 'number' as const,
          label: '精度',
          placeholder: '小数点后位数',
          description: '计算结果的小数精度'
        }
      }
    },
    {
      value: 'vector3d_expression',
      label: '3D向量表达式操作',
      description: '使用表达式对3D向量数据进行复杂计算和处理',
      configSchema: {
        expression: {
          type: 'textarea' as const,
          label: '表达式',
          required: true,
          placeholder: 'vectorMagnitude(acceleration.x, acceleration.y, acceleration.z)',
          description: '支持3D向量函数和数学运算的表达式'
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
            { value: 'vector', label: '向量' },
            { value: 'boolean', label: '布尔值' },
            { value: 'string', label: '字符串' }
          ],
          defaultValue: 'number'
        }
      }
    },
    {
      value: 'vector3d_alert',
      label: '3D向量告警',
      description: '基于向量数据的告警通知',
      configSchema: {
        alert_type: {
          type: 'string' as const,
          label: '告警类型',
          options: [
            { value: 'magnitude_threshold', label: '模长阈值告警' },
            { value: 'component_threshold', label: '分量阈值告警' },
            { value: 'vector_angle', label: '向量角度告警' },
            { value: 'vibration_anomaly', label: '振动异常告警' }
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
          placeholder: '设备{{.DeviceID}}向量{{.VectorField}}异常: 当前值{{.CurrentValue}}，阈值{{.Threshold}}'
        }
      }
    }
  ];

  // 3D向量表达式函数
  const vector3DFunctions = [
    {
      name: 'vectorMagnitude',
      description: '计算3D向量模长',
      syntax: 'vectorMagnitude(x, y, z)',
      example: 'vectorMagnitude(acceleration.x, acceleration.y, acceleration.z) > 9.8',
      category: '向量函数',
      parameters: ['x', 'y', 'z']
    },
    {
      name: 'vectorAngle',
      description: '计算两向量夹角',
      syntax: 'vectorAngle(x1, y1, z1, x2, y2, z2)',
      example: 'vectorAngle(velocity.x, velocity.y, velocity.z, 1, 0, 0) < 45',
      category: '向量函数',
      parameters: ['x1', 'y1', 'z1', 'x2', 'y2', 'z2']
    },
    {
      name: 'vectorDot',
      description: '计算向量点积',
      syntax: 'vectorDot(x1, y1, z1, x2, y2, z2)',
      example: 'vectorDot(force.x, force.y, force.z, 0, 0, 1) > 0',
      category: '向量函数',
      parameters: ['x1', 'y1', 'z1', 'x2', 'y2', 'z2']
    },
    {
      name: 'vectorCross',
      description: '计算向量叉积模长',
      syntax: 'vectorCross(x1, y1, z1, x2, y2, z2)',
      example: 'vectorCross(velocity.x, velocity.y, velocity.z, acceleration.x, acceleration.y, acceleration.z)',
      category: '向量函数',
      parameters: ['x1', 'y1', 'z1', 'x2', 'y2', 'z2']
    }
  ];

  // 3D向量变量
  const vector3DVariables = [
    { name: 'acceleration.x', description: '加速度X分量', type: 'number', example: 'acceleration.x > 2.0' },
    { name: 'acceleration.y', description: '加速度Y分量', type: 'number', example: 'acceleration.y < -1.0' },
    { name: 'acceleration.z', description: '加速度Z分量', type: 'number', example: 'acceleration.z > 9.8' },
    { name: 'velocity.x', description: '速度X分量', type: 'number', example: 'velocity.x > 0' },
    { name: 'velocity.y', description: '速度Y分量', type: 'number', example: 'velocity.y < 0' },
    { name: 'velocity.z', description: '速度Z分量', type: 'number', example: 'velocity.z == 0' },
    { name: 'position.x', description: '位置X坐标', type: 'number', example: 'position.x > 100' },
    { name: 'position.y', description: '位置Y坐标', type: 'number', example: 'position.y < -50' },
    { name: 'position.z', description: '位置Z坐标', type: 'number', example: 'position.z > 0' },
    { name: 'force.magnitude', description: '合力大小', type: 'number', example: 'force.magnitude > 1000' },
    { name: 'gyroscope.magnitude', description: '角速度模长', type: 'number', example: 'gyroscope.magnitude > 5.0' }
  ];

  const handleSaveClick = async () => {
    updateJsonValue();
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

  const handleFormChange = () => {
    // 如果当前在JSON编辑或预览tab，立即更新JSON
    if (activeTab === 'json' || activeTab === 'preview') {
      setTimeout(updateJsonValue, 100);
    }
  };

  const renderVector3DConfig = () => (
    <Card title={<Space><ThunderboltOutlined />3D向量配置</Space>} style={{ marginTop: 16 }}>
      <Alert
        message="3D向量数据类型"
        description="此数据类型专门处理三轴向量数据，如加速度、速度、位置、力等，支持实时模长计算、分量提取和向量运算"
        type="info"
        showIcon
      />
    </Card>
  );

  const renderConditionsSection = () => (
    <Card title="3D向量条件" style={{ marginTop: 16 }}>
      <ConditionBuilder
        value={conditions}
        onChange={setConditions}
        availableFields={['device_id', 'key', 'value', 'timestamp']}
        customFieldOptions={vector3DFields}
        allowedOperators={['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'regex']}
        supportExpressions={true}
        dataTypeName="3D向量"
      />
      
      <div style={{ marginTop: 16 }}>
        <ExpressionEditor
          value={conditions?.type === 'expression' ? conditions.expression : ''}
          onChange={(expr) => setConditions({ type: 'expression', expression: expr })}
          dataType="vector3d"
          availableFunctions={vector3DFunctions}
          availableVariables={vector3DVariables}
          placeholder="输入3D向量表达式，例如：vectorMagnitude(acceleration.x, acceleration.y, acceleration.z) > 9.8"
          rows={3}
        />
      </div>
    </Card>
  );

  const renderActionsSection = () => (
    <Card title="3D向量动作" style={{ marginTop: 16 }}>
      <ActionFormBuilder
        value={actions}
        onChange={setActions}
        availableActionTypes={[]} // 移除普通动作类型，只使用专门的3D向量动作
        customActionOptions={vector3DActions}
        dataTypeName="3D向量"
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
                  <Input placeholder="请输入3D向量规则名称" />
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
                placeholder="请输入3D向量规则描述" 
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
          
          {/* 3D向量配置 */}
          {renderVector3DConfig()}
        </Card>
      )
    },
    {
      key: 'conditions',
      label: <Space><ThunderboltOutlined />向量条件</Space>,
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
          <ThunderboltOutlined style={{ color: '#1890ff' }} />
          <span>3D向量规则编辑器</span>
          <Tag color="blue" icon={<FunctionOutlined />}>
            3D向量数据
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

export default Vector3DRuleEditor;