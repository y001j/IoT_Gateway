import React, { useState, useEffect } from 'react';
import {
  Modal,
  Form,
  Input,
  InputNumber,
  Switch,
  Card,
  Row,
  Col,
  Space,
  Button,
  Typography,
  Divider,
  Alert,
  Tag,
  Breadcrumb,
  Tabs,
  Tooltip,
  Progress
} from 'antd';
import {
  ArrowLeftOutlined,
  BorderOutlined,
  EyeOutlined,
  CheckCircleOutlined,
  BookOutlined,
  FunctionOutlined,
  AimOutlined,
  SwapOutlined,
  RotateRightOutlined
} from '@ant-design/icons';
import type { Rule, Action, Condition } from '../types/rule';
import type { DataTypeOption } from './DataTypeSelector';
import VectorActionEditor from './VectorActionEditor';

const { Title, Text } = Typography;
const { TextArea } = Input;
const { TabPane } = Tabs;

interface VectorRuleEditorProps {
  visible: boolean;
  dataType: DataTypeOption | null;
  rule?: Rule | null;
  isEditing: boolean;
  onSave: (rule: Partial<Rule>) => void;
  onCancel: () => void;
  onBack: () => void;
}

interface VectorRuleFormData {
  name: string;
  description: string;
  enabled: boolean;
  priority: number;
  conditions?: Condition;
  actions?: Action[];
  tags?: Array<{key: string; value: string}>;
}

interface Vector3DInput {
  x: number;
  y: number;
  z: number;
}

interface VectorCondition {
  type: 'magnitude_based' | 'direction_based' | 'component_based' | 'angle_based';
  // Magnitude-based
  magnitude_operator?: 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'range';
  magnitude_value?: number;
  magnitude_range?: { min: number; max: number };
  // Direction-based
  reference_vector?: Vector3DInput;
  angle_threshold?: number;
  angle_operator?: 'gt' | 'lt' | 'eq';
  // Component-based
  component?: 'x' | 'y' | 'z';
  component_operator?: 'gt' | 'gte' | 'lt' | 'lte' | 'eq';
  component_value?: number;
  // Advanced
  coordinate_system?: '3d' | '2d_xy' | '2d_xz' | '2d_yz';
}

const VectorRuleEditor: React.FC<VectorRuleEditorProps> = ({
  visible,
  dataType,
  rule,
  isEditing,
  onSave,
  onCancel,
  onBack
}) => {
  const [form] = Form.useForm<VectorRuleFormData>();
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState<'conditions' | 'actions' | 'preview'>('conditions');
  const [conditionType, setConditionType] = useState<VectorCondition['type']>('magnitude_based');
  const [currentCondition, setCurrentCondition] = useState<VectorCondition>({
    type: 'magnitude_based',
    magnitude_operator: 'gt',
    magnitude_value: 1.0,
    coordinate_system: '3d'
  });
  const [currentActions, setCurrentActions] = useState<Action[]>([]);

  // 初始化表单数据
  useEffect(() => {
    if (rule) {
      form.setFieldsValue({
        name: rule.name,
        description: rule.description,
        enabled: rule.enabled,
        priority: rule.priority,
        tags: rule.tags ? Object.entries(rule.tags).map(([key, value]) => ({ key, value })) : []
      });
      
      // 解析向量数据特有的条件
      if (rule.conditions) {
        parseVectorCondition(rule.conditions);
      }
      setCurrentActions(rule.actions || []);
    } else if (dataType) {
      // 设置默认值
      form.setFieldsValue({
        enabled: true,
        priority: 100,
        tags: [
          { key: 'data_type', value: 'vector3d' },
          { key: 'data_category', value: 'vector' }
        ]
      });
      
      // 设置默认动作
      setCurrentActions([{
        type: 'vector_transform',
        config: {
          sub_type: 'magnitude',
          output_key: 'vector_magnitude'
        }
      }]);
    }
  }, [rule, dataType, form]);

  // 解析向量数据条件
  const parseVectorCondition = (condition: Condition) => {
    if (condition.type === 'simple' && condition.field === 'magnitude') {
      setConditionType('magnitude_based');
      setCurrentCondition({
        type: 'magnitude_based',
        magnitude_operator: condition.operator as any || 'gt',
        magnitude_value: typeof condition.value === 'number' ? condition.value : 1.0,
        coordinate_system: '3d'
      });
    } else if (condition.expression) {
      // 尝试解析表达式中的向量操作
      setConditionType('magnitude_based');
      setCurrentCondition({
        type: 'magnitude_based',
        magnitude_operator: 'gt',
        magnitude_value: 1.0,
        coordinate_system: '3d'
      });
    }
  };

  // 构建向量数据条件
  const buildVectorCondition = (): Condition => {
    switch (currentCondition.type) {
      case 'magnitude_based':
        if (currentCondition.magnitude_operator === 'range') {
          return {
            type: 'and',
            and: [
              {
                type: 'simple',
                field: 'magnitude',
                operator: 'gte',
                value: currentCondition.magnitude_range?.min || 0
              },
              {
                type: 'simple',
                field: 'magnitude',
                operator: 'lte',
                value: currentCondition.magnitude_range?.max || 10
              }
            ]
          };
        } else {
          return {
            type: 'simple',
            field: 'magnitude',
            operator: currentCondition.magnitude_operator || 'gt',
            value: currentCondition.magnitude_value || 1.0
          };
        }
        
      case 'component_based':
        return {
          type: 'simple',
          field: currentCondition.component || 'x',
          operator: currentCondition.component_operator || 'gt',
          value: currentCondition.component_value || 0
        };
        
      case 'direction_based':
        const ref = currentCondition.reference_vector || { x: 1, y: 0, z: 0 };
        return {
          type: 'expression',
          expression: `vector_angle(x, y, z, ${ref.x}, ${ref.y}, ${ref.z}) ${currentCondition.angle_operator || '<'} ${currentCondition.angle_threshold || 45}`
        };
        
      case 'angle_based':
        return {
          type: 'expression',
          expression: `vector_angle_with_axis(x, y, z, "z") ${currentCondition.angle_operator || '<'} ${currentCondition.angle_threshold || 90}`
        };
        
      default:
        return {
          type: 'simple',
          field: 'magnitude',
          operator: 'gt',
          value: 0
        };
    }
  };

  // 计算向量模长（用于预览）
  const calculateMagnitude = (vector: Vector3DInput): number => {
    return Math.sqrt(vector.x ** 2 + vector.y ** 2 + vector.z ** 2);
  };

  // 渲染向量输入组件
  const renderVectorInput = (
    label: string,
    value: Vector3DInput,
    onChange: (vector: Vector3DInput) => void,
    showMagnitude: boolean = true
  ) => {
    const magnitude = calculateMagnitude(value);
    
    return (
      <Card size="small" title={label}>
        <Row gutter={[8, 8]}>
          <Col span={8}>
            <Text strong>X轴:</Text>
            <InputNumber
              value={value.x}
              onChange={(val) => onChange({ ...value, x: val || 0 })}
              style={{ width: '100%', marginTop: 4 }}
              step={0.1}
              precision={3}
            />
          </Col>
          <Col span={8}>
            <Text strong>Y轴:</Text>
            <InputNumber
              value={value.y}
              onChange={(val) => onChange({ ...value, y: val || 0 })}
              style={{ width: '100%', marginTop: 4 }}
              step={0.1}
              precision={3}
            />
          </Col>
          <Col span={8}>
            <Text strong>Z轴:</Text>
            <InputNumber
              value={value.z}
              onChange={(val) => onChange({ ...value, z: val || 0 })}
              style={{ width: '100%', marginTop: 4 }}
              step={0.1}
              precision={3}
            />
          </Col>
        </Row>
        
        {showMagnitude && (
          <div style={{ marginTop: 8, padding: 8, background: '#f5f5f5', borderRadius: 4 }}>
            <Space>
              <Text type="secondary">向量模长: </Text>
              <Text strong>{magnitude.toFixed(3)}</Text>
              <Tooltip title="向量模长 = √(x² + y² + z²)">
                <Text type="secondary">(|v|)</Text>
              </Tooltip>
            </Space>
            <Progress
              percent={(magnitude / 10) * 100}
              showInfo={false}
              size="small"
              style={{ marginTop: 4 }}
            />
          </div>
        )}
      </Card>
    );
  };

  // 渲染条件编辑器
  const renderConditionEditor = () => {
    switch (currentCondition.type) {
      case 'magnitude_based':
        return (
          <Card title={<><AimOutlined /> 向量模长条件</>} size="small">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Row gutter={16}>
                <Col span={8}>
                  <Text strong>判断条件:</Text>
                  <Form.Item style={{ marginTop: 4 }}>
                    <Space.Compact>
                      <Button 
                        type={currentCondition.magnitude_operator === 'gt' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          magnitude_operator: 'gt'
                        })}
                      >
                        大于 &gt;
                      </Button>
                      <Button 
                        type={currentCondition.magnitude_operator === 'gte' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          magnitude_operator: 'gte'
                        })}
                      >
                        大于等于 ≥
                      </Button>
                      <Button 
                        type={currentCondition.magnitude_operator === 'lt' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          magnitude_operator: 'lt'
                        })}
                      >
                        小于 &lt;
                      </Button>
                    </Space.Compact>
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Text strong>阈值:</Text>
                  <InputNumber
                    value={currentCondition.magnitude_value}
                    onChange={(value) => setCurrentCondition({
                      ...currentCondition,
                      magnitude_value: value || 0
                    })}
                    style={{ width: '100%', marginTop: 4 }}
                    min={0}
                    step={0.1}
                    precision={3}
                    placeholder="1.0"
                  />
                </Col>
                <Col span={8}>
                  <Text strong>坐标系:</Text>
                  <Form.Item style={{ marginTop: 4 }}>
                    <Space.Compact>
                      <Button 
                        type={currentCondition.coordinate_system === '3d' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          coordinate_system: '3d'
                        })}
                      >
                        3D
                      </Button>
                      <Button 
                        type={currentCondition.coordinate_system === '2d_xy' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          coordinate_system: '2d_xy'
                        })}
                      >
                        XY平面
                      </Button>
                    </Space.Compact>
                  </Form.Item>
                </Col>
              </Row>
              
              <Alert
                message="向量模长"
                description={`当向量模长${currentCondition.magnitude_operator === 'gt' ? '大于' : currentCondition.magnitude_operator === 'gte' ? '大于等于' : '小于'} ${currentCondition.magnitude_value || 0} 时触发规则`}
                type="info"
                showIcon
                style={{ marginTop: 8 }}
              />
            </Space>
          </Card>
        );
        
      case 'component_based':
        return (
          <Card title={<><SwapOutlined /> 向量分量条件</>} size="small">
            <Row gutter={16}>
              <Col span={8}>
                <Text strong>分量选择:</Text>
                <Form.Item style={{ marginTop: 4 }}>
                  <Space.Compact>
                    <Button 
                      type={currentCondition.component === 'x' ? 'primary' : 'default'}
                      onClick={() => setCurrentCondition({
                        ...currentCondition,
                        component: 'x'
                      })}
                    >
                      X轴
                    </Button>
                    <Button 
                      type={currentCondition.component === 'y' ? 'primary' : 'default'}
                      onClick={() => setCurrentCondition({
                        ...currentCondition,
                        component: 'y'
                      })}
                    >
                      Y轴
                    </Button>
                    <Button 
                      type={currentCondition.component === 'z' ? 'primary' : 'default'}
                      onClick={() => setCurrentCondition({
                        ...currentCondition,
                        component: 'z'
                      })}
                    >
                      Z轴
                    </Button>
                  </Space.Compact>
                </Form.Item>
              </Col>
              <Col span={8}>
                <Text strong>条件:</Text>
                <Form.Item style={{ marginTop: 4 }}>
                  <Space.Compact>
                    <Button 
                      type={currentCondition.component_operator === 'gt' ? 'primary' : 'default'}
                      onClick={() => setCurrentCondition({
                        ...currentCondition,
                        component_operator: 'gt'
                      })}
                    >
                      &gt;
                    </Button>
                    <Button 
                      type={currentCondition.component_operator === 'lt' ? 'primary' : 'default'}
                      onClick={() => setCurrentCondition({
                        ...currentCondition,
                        component_operator: 'lt'
                      })}
                    >
                      &lt;
                    </Button>
                    <Button 
                      type={currentCondition.component_operator === 'eq' ? 'primary' : 'default'}
                      onClick={() => setCurrentCondition({
                        ...currentCondition,
                        component_operator: 'eq'
                      })}
                    >
                      =
                    </Button>
                  </Space.Compact>
                </Form.Item>
              </Col>
              <Col span={8}>
                <Text strong>值:</Text>
                <InputNumber
                  value={currentCondition.component_value}
                  onChange={(value) => setCurrentCondition({
                    ...currentCondition,
                    component_value: value || 0
                  })}
                  style={{ width: '100%', marginTop: 4 }}
                  step={0.1}
                  precision={3}
                  placeholder="0.0"
                />
              </Col>
            </Row>
          </Card>
        );
        
      case 'direction_based':
        return (
          <Card title={<><FunctionOutlined /> 向量方向条件</>} size="small">
            <Space direction="vertical" style={{ width: '100%' }}>
              {renderVectorInput(
                '参考方向向量', 
                currentCondition.reference_vector || { x: 1, y: 0, z: 0 },
                (vector) => setCurrentCondition({
                  ...currentCondition,
                  reference_vector: vector
                })
              )}
              
              <Row gutter={16}>
                <Col span={12}>
                  <Text strong>角度条件:</Text>
                  <Form.Item style={{ marginTop: 4 }}>
                    <Space.Compact>
                      <Button 
                        type={currentCondition.angle_operator === 'lt' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          angle_operator: 'lt'
                        })}
                      >
                        小于
                      </Button>
                      <Button 
                        type={currentCondition.angle_operator === 'gt' ? 'primary' : 'default'}
                        onClick={() => setCurrentCondition({
                          ...currentCondition,
                          angle_operator: 'gt'
                        })}
                      >
                        大于
                      </Button>
                    </Space.Compact>
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Text strong>角度阈值 (度):</Text>
                  <InputNumber
                    value={currentCondition.angle_threshold}
                    onChange={(value) => setCurrentCondition({
                      ...currentCondition,
                      angle_threshold: value || 0
                    })}
                    style={{ width: '100%', marginTop: 4 }}
                    min={0}
                    max={180}
                    step={1}
                    placeholder="45"
                  />
                </Col>
              </Row>
            </Space>
          </Card>
        );
        
      default:
        return (
          <Card title="条件配置" size="small">
            <Text type="secondary">请选择条件类型</Text>
          </Card>
        );
    }
  };

  // 生成预览数据
  const generatePreviewData = () => {
    const formData = form.getFieldsValue();
    const condition = buildVectorCondition();
    
    const previewRule = {
      name: formData.name || '',
      description: formData.description || '',
      enabled: formData.enabled ?? true,
      priority: formData.priority || 100,
      data_type: dataType?.key,
      data_category: 'vector',
      conditions: condition,
      actions: currentActions,
      tags: {
        ...formData.tags?.reduce((acc: any, item: any) => {
          if (item.key && item.value) {
            acc[item.key] = item.value;
          }
          return acc;
        }, {}),
        coordinate_system: currentCondition.coordinate_system
      }
    };
    
    return JSON.stringify(previewRule, null, 2);
  };

  // 保存规则
  const handleSave = async () => {
    try {
      setLoading(true);
      
      const formData = await form.validateFields();
      const condition = buildVectorCondition();
      
      if (!currentActions || currentActions.length === 0) {
        throw new Error('请配置至少一个动作');
      }

      const ruleData = {
        name: formData.name,
        description: formData.description,
        enabled: formData.enabled,
        priority: formData.priority,
        data_type: dataType?.key,
        data_category: 'vector',
        conditions: condition,
        actions: currentActions,
        tags: {
          ...formData.tags?.reduce((acc: any, item: any) => {
            if (item.key && item.value) {
              acc[item.key] = item.value;
            }
            return acc;
          }, {}),
          coordinate_system: currentCondition.coordinate_system
        }
      };

      await onSave(ruleData);
    } catch (error: any) {
      console.error('保存向量数据规则失败:', error);
    } finally {
      setLoading(false);
    }
  };

  if (!dataType) return null;

  return (
    <Modal
      title={
        <Space>
          <Button 
            type="text" 
            icon={<ArrowLeftOutlined />} 
            onClick={onBack}
            size="small"
          >
            返回
          </Button>
          <Divider type="vertical" />
          <Space direction="vertical" size={0}>
            <Space>
              <BorderOutlined />
              {isEditing ? '编辑' : '创建'} 三维向量数据规则
              <Tag color="#722ed1" icon={<FunctionOutlined />}>
                向量数据
              </Tag>
            </Space>
            <Breadcrumb separator="/">
              <Breadcrumb.Item>复合数据规则</Breadcrumb.Item>
              <Breadcrumb.Item>向量数据</Breadcrumb.Item>
              <Breadcrumb.Item>{dataType.name}</Breadcrumb.Item>
            </Breadcrumb>
          </Space>
        </Space>
      }
      open={visible}
      onOk={handleSave}
      onCancel={onCancel}
      width={1400}
      okText="保存规则"
      cancelText="取消"
      confirmLoading={loading}
      styles={{ body: { maxHeight: '75vh', overflowY: 'auto', padding: '0 24px' } }}
    >
      {/* 数据类型信息栏 */}
      <Alert
        message={
          <Space>
            <span>专用编辑器：</span>
            <strong>三维向量数据规则</strong>
            <span>-</span>
            <Text type="secondary">支持向量模长、方向角、分量判断等三维向量运算功能</Text>
          </Space>
        }
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Row gutter={24}>
        {/* 主编辑区域 */}
        <Col span={16}>
          <Form form={form} layout="vertical">
            {/* 基本信息 */}
            <Card title="基本信息" size="small" style={{ marginBottom: 16 }}>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    label="规则名称"
                    name="name"
                    rules={[{ required: true, message: '请输入规则名称' }]}
                  >
                    <Input placeholder="如：加速度异常检测" />
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
                label="规则描述"
                name="description"
                rules={[{ required: true, message: '请输入规则描述' }]}
              >
                <TextArea rows={2} placeholder="描述这个向量数据规则的用途和功能" />
              </Form.Item>

              <Form.Item label="启用状态" name="enabled" valuePropName="checked">
                <Switch checkedChildren="启用" unCheckedChildren="禁用" />
              </Form.Item>
            </Card>

            {/* 条件和动作配置 */}
            <Tabs activeKey={activeTab} onChange={(key) => setActiveTab(key as any)}>
              <TabPane tab={<><AimOutlined /> 触发条件</> } key="conditions">
                <Card size="small" title="向量条件类型选择" style={{ marginBottom: 16 }}>
                  <Space wrap>
                    <Button
                      type={conditionType === 'magnitude_based' ? 'primary' : 'default'}
                      icon={<AimOutlined />}
                      onClick={() => {
                        setConditionType('magnitude_based');
                        setCurrentCondition({ ...currentCondition, type: 'magnitude_based' });
                      }}
                    >
                      模长条件
                    </Button>
                    <Button
                      type={conditionType === 'component_based' ? 'primary' : 'default'}
                      icon={<SwapOutlined />}
                      onClick={() => {
                        setConditionType('component_based');
                        setCurrentCondition({ ...currentCondition, type: 'component_based' });
                      }}
                    >
                      分量条件
                    </Button>
                    <Button
                      type={conditionType === 'direction_based' ? 'primary' : 'default'}
                      icon={<FunctionOutlined />}
                      onClick={() => {
                        setConditionType('direction_based');
                        setCurrentCondition({ ...currentCondition, type: 'direction_based' });
                      }}
                    >
                      方向条件
                    </Button>
                    <Button
                      type={conditionType === 'angle_based' ? 'primary' : 'default'}
                      icon={<RotateRightOutlined />}
                      onClick={() => {
                        setConditionType('angle_based');
                        setCurrentCondition({ ...currentCondition, type: 'angle_based' });
                      }}
                    >
                      角度条件
                    </Button>
                  </Space>
                </Card>
                
                {renderConditionEditor()}
              </TabPane>

              <TabPane tab={<><CheckCircleOutlined /> 执行动作</> } key="actions">
                <VectorActionEditor
                  value={currentActions.length > 0 ? {
                    sub_type: currentActions[0].config.sub_type,
                    reference_vector: {
                      x: currentActions[0].config.reference_x || 0,
                      y: currentActions[0].config.reference_y || 0,
                      z: currentActions[0].config.reference_z || 0
                    },
                    rotation_axis: currentActions[0].config.rotation_axis,
                    rotation_angle: currentActions[0].config.rotation_angle,
                    custom_axis: {
                      x: currentActions[0].config.custom_axis_x || 0,
                      y: currentActions[0].config.custom_axis_y || 0,
                      z: currentActions[0].config.custom_axis_z || 1
                    },
                    normalize_magnitude: currentActions[0].config.normalize_magnitude,
                    output_key: currentActions[0].config.output_key
                  } : undefined}
                  onChange={(config) => {
                    const action: Action = {
                      type: 'vector_transform',
                      config: {
                        sub_type: config.sub_type,
                        reference_x: config.reference_vector?.x,
                        reference_y: config.reference_vector?.y,
                        reference_z: config.reference_vector?.z,
                        rotation_axis: config.rotation_axis,
                        rotation_angle: config.rotation_angle,
                        custom_axis_x: config.custom_axis?.x,
                        custom_axis_y: config.custom_axis?.y,
                        custom_axis_z: config.custom_axis?.z,
                        normalize_magnitude: config.normalize_magnitude,
                        output_key: config.output_key
                      }
                    };
                    setCurrentActions([action]);
                  }}
                />
              </TabPane>
            </Tabs>
          </Form>
        </Col>

        {/* 预览区域 */}
        <Col span={8}>
          <Card
            title={
              <Space>
                <EyeOutlined />
                实时预览
              </Space>
            }
            size="small"
            style={{ position: 'sticky', top: 0 }}
          >
            <pre
              style={{
                background: '#f5f5f5',
                padding: 12,
                borderRadius: 4,
                fontSize: 11,
                lineHeight: 1.4,
                maxHeight: '60vh',
                overflow: 'auto',
                margin: 0
              }}
            >
              {generatePreviewData()}
            </pre>
          </Card>
        </Col>
      </Row>
    </Modal>
  );
};

export default VectorRuleEditor;