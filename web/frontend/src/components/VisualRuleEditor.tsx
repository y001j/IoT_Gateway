import React, { useState, useEffect } from 'react';
import { Modal, Card, Form, Input, Select, Space, Typography, Row, Col, Divider, Button, InputNumber, Switch, ColorPicker, Slider, Tabs, Alert, Tag } from 'antd';
import { BgColorsOutlined, InfoCircleOutlined, EyeOutlined, EditOutlined, CodeOutlined, CheckCircleOutlined } from '@ant-design/icons';
import { useBaseRuleEditor } from './base/BaseRuleEditor';
import ConditionBuilder from './base/ConditionBuilder';
import ActionFormBuilder from './base/ActionFormBuilder';
import ExpressionEditor from './base/ExpressionEditor';
import { Rule, Condition, Action } from '../types/rule';

const { Option } = Select;
const { Text } = Typography;
const { TextArea } = Input;

export interface VisualRuleEditorProps {
  visible: boolean;
  onClose: () => void;
  onSave: (rule: Rule) => Promise<void>;
  rule?: Rule;
}

interface ColorData {
  r: number;
  g: number;
  b: number;
  a?: number;
}

/**
 * 视觉规则编辑器
 * 专门处理颜色数据和视觉数据的规则
 */
const VisualRuleEditor: React.FC<VisualRuleEditorProps> = ({
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
    title: '视觉规则编辑器',
    dataTypeName: '视觉数据'
  });

  // 自定义的取消处理函数，重置所有状态
  const handleCancel = () => {
    console.log('VisualRuleEditor handleCancel 被调用');
    
    // 重置所有本地状态
    setConditions(undefined);
    setActions([]);
    setActiveTab('basic');
    setJsonValue('');
    setJsonError('');
    
    // 调用基础的取消处理 - 这会重置表单并调用onClose
    console.log('调用 baseHandleCancel');
    baseHandleCancel();
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
        return rule.data_type.type || 'color';
      }
    }
    
    if (rule.tags) {
      const tagDataType = rule.tags['data_type'] || rule.tags['data_category'];
      if (tagDataType) return tagDataType;
    }
    
    // 根据规则名称和描述推断
    const nameDesc = `${rule.name || ''} ${rule.description || ''}`.toLowerCase();
    if (nameDesc.includes('color') || nameDesc.includes('颜色') || nameDesc.includes('rgb') || nameDesc.includes('hsl')) {
      return 'color';
    }
    if (nameDesc.includes('visual') || nameDesc.includes('视觉') || nameDesc.includes('brightness') || nameDesc.includes('亮度')) {
      return 'color';
    }
    
    return 'color'; // 默认为颜色数据
  };

  // 计算最终数据类型
  const finalDataType = rule ? inferDataTypeFromRule(rule) : 'color';

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

  // 视觉数据特定字段
  const visualFields = [
    { value: 'color.r', label: '红色分量 (R)', description: 'RGB颜色的红色分量 (0-255)' },
    { value: 'color.g', label: '绿色分量 (G)', description: 'RGB颜色的绿色分量 (0-255)' },
    { value: 'color.b', label: '蓝色分量 (B)', description: 'RGB颜色的蓝色分量 (0-255)' },
    { value: 'color.a', label: '透明度 (Alpha)', description: '颜色透明度 (0.0-1.0)' },
    
    { value: 'hsl.h', label: '色相 (Hue)', description: 'HSL颜色的色相值 (0-360)' },
    { value: 'hsl.s', label: '饱和度 (Saturation)', description: 'HSL颜色的饱和度 (0-100)' },
    { value: 'hsl.l', label: '亮度 (Lightness)', description: 'HSL颜色的亮度 (0-100)' },
    
    { value: 'hsv.h', label: 'HSV色相', description: 'HSV颜色的色相值 (0-360)' },
    { value: 'hsv.s', label: 'HSV饱和度', description: 'HSV颜色的饱和度 (0-100)' },
    { value: 'hsv.v', label: '明度 (Value)', description: 'HSV颜色的明度值 (0-100)' },
    
    { value: 'color.hex', label: 'HEX颜色值', description: '十六进制颜色表示 (#RRGGBB)' },
    { value: 'color.brightness', label: '整体亮度', description: '感知亮度值 (0.0-1.0)' },
    { value: 'color.contrast', label: '对比度', description: '相对于参考色的对比度' },
    { value: 'color.dominant', label: '主色调', description: '图像或区域的主色调分类' }
  ];

  // 视觉数据特定动作
  const visualActions = [
    {
      value: 'color_transform',
      label: '颜色转换',
      description: '颜色空间转换、色彩调整、格式转换',
      configSchema: {
        transform_type: {
          type: 'string' as const,
          label: '转换类型',
          required: true,
          options: [
            { value: 'color_space_convert', label: '颜色空间转换' },
            { value: 'brightness_adjust', label: '亮度调整' },
            { value: 'saturation_adjust', label: '饱和度调整' },
            { value: 'hue_shift', label: '色相偏移' },
            { value: 'contrast_adjust', label: '对比度调整' },
            { value: 'gamma_correction', label: '伽马校正' },
            { value: 'color_invert', label: '颜色反转' },
            { value: 'grayscale_convert', label: '灰度转换' }
          ]
        },
        source_color_space: {
          type: 'string' as const,
          label: '源颜色空间',
          options: [
            { value: 'RGB', label: 'RGB' },
            { value: 'HSL', label: 'HSL' },
            { value: 'HSV', label: 'HSV' },
            { value: 'CMYK', label: 'CMYK' },
            { value: 'LAB', label: 'LAB' }
          ],
          defaultValue: 'RGB'
        },
        target_color_space: {
          type: 'string' as const,
          label: '目标颜色空间',
          options: [
            { value: 'RGB', label: 'RGB' },
            { value: 'HSL', label: 'HSL' },
            { value: 'HSV', label: 'HSV' },
            { value: 'CMYK', label: 'CMYK' },
            { value: 'LAB', label: 'LAB' }
          ],
          defaultValue: 'HSL'
        },
        adjustment_value: {
          type: 'number' as const,
          label: '调整数值',
          placeholder: '调整强度 (-100到100)',
          description: '颜色调整的强度值'
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
      value: 'color_analysis',
      label: '颜色分析',
      description: '颜色相似度、主色调提取、色彩统计',
      configSchema: {
        analysis_type: {
          type: 'string' as const,
          label: '分析类型',
          required: true,
          options: [
            { value: 'similarity_check', label: '颜色相似度检查' },
            { value: 'dominant_color', label: '主色调提取' },
            { value: 'color_histogram', label: '颜色直方图' },
            { value: 'color_temperature', label: '色温分析' },
            { value: 'color_palette', label: '调色板生成' }
          ]
        },
        reference_color: {
          type: 'object' as const,
          label: '参考颜色',
          placeholder: '{"r": 255, "g": 0, "b": 0}',
          description: '用于相似度比较的参考颜色'
        },
        similarity_threshold: {
          type: 'number' as const,
          label: '相似度阈值',
          placeholder: '0.0-1.0之间的数值',
          description: '颜色相似度判断阈值'
        },
        distance_method: {
          type: 'string' as const,
          label: '距离算法',
          options: [
            { value: 'euclidean', label: '欧氏距离' },
            { value: 'manhattan', label: '曼哈顿距离' },
            { value: 'cosine', label: '余弦距离' },
            { value: 'cie2000', label: 'CIE2000色差' }
          ],
          defaultValue: 'euclidean'
        }
      }
    },
    {
      value: 'visual_expression',
      label: '视觉表达式操作',
      description: '使用表达式对视觉数据进行复杂计算和处理',
      configSchema: {
        expression: {
          type: 'textarea' as const,
          label: '表达式',
          required: true,
          placeholder: 'getBrightness(color.r, color.g, color.b) * 0.8',
          description: '支持颜色函数和数学运算的表达式'
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
            { value: 'string', label: '字符串' },
            { value: 'boolean', label: '布尔值' },
            { value: 'object', label: '对象' }
          ],
          defaultValue: 'number'
        }
      }
    },
    {
      value: 'visual_alert',
      label: '视觉告警',
      description: '基于视觉特征的告警通知',
      configSchema: {
        alert_type: {
          type: 'string' as const,
          label: '告警类型',
          options: [
            { value: 'color_change', label: '颜色变化告警' },
            { value: 'brightness_threshold', label: '亮度阈值告警' },
            { value: 'contrast_anomaly', label: '对比度异常告警' },
            { value: 'color_similarity', label: '颜色匹配告警' }
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
          placeholder: '设备{{.DeviceID}}颜色异常: R={{.color.r}}, G={{.color.g}}, B={{.color.b}}'
        }
      }
    }
  ];

  // 视觉表达式函数
  const visualFunctions = [
    {
      name: 'colorSimilarity',
      description: '计算两种颜色的相似度',
      syntax: 'colorSimilarity(r1, g1, b1, r2, g2, b2, method)',
      example: 'colorSimilarity(color.r, color.g, color.b, 255, 0, 0, "euclidean") > 0.8',
      category: '颜色函数',
      parameters: ['r1', 'g1', 'b1', 'r2', 'g2', 'b2', 'method']
    },
    {
      name: 'rgbToHsl',
      description: 'RGB转HSL颜色空间',
      syntax: 'rgbToHsl(r, g, b)',
      example: 'rgbToHsl(color.r, color.g, color.b)',
      category: '颜色函数',
      parameters: ['r', 'g', 'b']
    },
    {
      name: 'hslToRgb',
      description: 'HSL转RGB颜色空间',
      syntax: 'hslToRgb(h, s, l)',
      example: 'hslToRgb(hsl.h, hsl.s, hsl.l)',
      category: '颜色函数',
      parameters: ['h', 's', 'l']
    },
    {
      name: 'getBrightness',
      description: '计算颜色亮度',
      syntax: 'getBrightness(r, g, b)',
      example: 'getBrightness(color.r, color.g, color.b) > 0.5',
      category: '颜色函数',
      parameters: ['r', 'g', 'b']
    },
    {
      name: 'getContrast',
      description: '计算两种颜色的对比度',
      syntax: 'getContrast(r1, g1, b1, r2, g2, b2)',
      example: 'getContrast(color.r, color.g, color.b, 255, 255, 255) > 4.5',
      category: '颜色函数',
      parameters: ['r1', 'g1', 'b1', 'r2', 'g2', 'b2']
    },
    {
      name: 'hexToRgb',
      description: '十六进制转RGB',
      syntax: 'hexToRgb(hex)',
      example: 'hexToRgb(color.hex)',
      category: '颜色函数',
      parameters: ['hex']
    },
    {
      name: 'rgbToHex',
      description: 'RGB转十六进制',
      syntax: 'rgbToHex(r, g, b)',
      example: 'rgbToHex(color.r, color.g, color.b)',
      category: '颜色函数',
      parameters: ['r', 'g', 'b']
    }
  ];

  // 视觉变量
  const visualVariables = [
    { name: 'color.r', description: '红色分量', type: 'number', example: 'color.r > 128' },
    { name: 'color.g', description: '绿色分量', type: 'number', example: 'color.g > 128' },
    { name: 'color.b', description: '蓝色分量', type: 'number', example: 'color.b > 128' },
    { name: 'color.a', description: '透明度', type: 'number', example: 'color.a > 0.5' },
    { name: 'hsl.h', description: 'HSL色相', type: 'number', example: 'hsl.h > 180' },
    { name: 'hsl.s', description: 'HSL饱和度', type: 'number', example: 'hsl.s > 50' },
    { name: 'hsl.l', description: 'HSL亮度', type: 'number', example: 'hsl.l > 50' },
    { name: 'color.hex', description: 'HEX颜色', type: 'string', example: 'contains(color.hex, "FF")' },
    { name: 'color.brightness', description: '感知亮度', type: 'number', example: 'color.brightness > 0.5' },
    { name: 'color.contrast', description: '对比度', type: 'number', example: 'color.contrast > 4.5' }
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

  // 颜色选择器组件
  const renderColorPicker = (label: string, color: ColorData, onChange: (color: ColorData) => void) => (
    <Card size="small" style={{ marginBottom: 16 }}>
      <Space direction="vertical" style={{ width: '100%' }}>
        <Text strong>{label}</Text>
        <Row gutter={16}>
          <Col span={12}>
            <ColorPicker
              value={{
                r: color.r,
                g: color.g,
                b: color.b,
                a: color.a || 1
              }}
              onChange={(value) => {
                const rgba = value.toRgb();
                onChange({
                  r: rgba.r,
                  g: rgba.g,
                  b: rgba.b,
                  a: rgba.a
                });
              }}
              showText
              style={{ width: '100%' }}
            />
          </Col>
          <Col span={12}>
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              <div>
                <Text style={{ fontSize: 12 }}>R: </Text>
                <InputNumber
                  value={color.r}
                  onChange={(val) => onChange({ ...color, r: val || 0 })}
                  style={{ width: 60 }}
                  min={0}
                  max={255}
                  size="small"
                />
              </div>
              <div>
                <Text style={{ fontSize: 12 }}>G: </Text>
                <InputNumber
                  value={color.g}
                  onChange={(val) => onChange({ ...color, g: val || 0 })}
                  style={{ width: 60 }}
                  min={0}
                  max={255}
                  size="small"
                />
              </div>
              <div>
                <Text style={{ fontSize: 12 }}>B: </Text>
                <InputNumber
                  value={color.b}
                  onChange={(val) => onChange({ ...color, b: val || 0 })}
                  style={{ width: 60 }}
                  min={0}
                  max={255}
                  size="small"
                />
              </div>
            </Space>
          </Col>
        </Row>
      </Space>
    </Card>
  );

  const renderVisualConfig = () => (
    <Card title={<Space><BgColorsOutlined />视觉数据配置</Space>} style={{ marginTop: 16 }}>
      <Alert
        message="视觉数据类型"
        description="此数据类型专门处理颜色、图像等视觉相关数据，支持RGB、HSL、HSV等多种颜色空间的分析和转换"
        type="info"
        showIcon
      />
    </Card>
  );

  const renderConditionsSection = () => (
    <Card title="视觉数据条件" style={{ marginTop: 16 }}>
      <ConditionBuilder
        value={conditions}
        onChange={setConditions}
        availableFields={['device_id', 'key', 'value', 'timestamp']}
        customFieldOptions={visualFields}
        allowedOperators={['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'regex']}
        supportExpressions={true}
        dataTypeName="视觉数据"
      />
      
      <div style={{ marginTop: 16 }}>
        <ExpressionEditor
          value={conditions?.type === 'expression' ? conditions.expression : ''}
          onChange={(expr) => setConditions({ type: 'expression', expression: expr })}
          dataType="visual"
          availableFunctions={visualFunctions}
          availableVariables={visualVariables}
          placeholder="输入视觉数据表达式，例如：colorSimilarity(color.r, color.g, color.b, 255, 0, 0, 'euclidean') > 0.8"
          rows={3}
        />
      </div>
    </Card>
  );

  const renderActionsSection = () => (
    <Card title="视觉数据动作" style={{ marginTop: 16 }}>
      <ActionFormBuilder
        value={actions}
        onChange={setActions}
        availableActionTypes={[]} // 移除普通动作类型，只使用专门的视觉数据动作
        customActionOptions={visualActions}
        dataTypeName="视觉数据"
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
                  <Input placeholder="请输入视觉规则名称" />
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
                placeholder="请输入视觉规则描述" 
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
          
          {/* 视觉数据配置 */}
          {renderVisualConfig()}
        </Card>
      )
    },
    {
      key: 'conditions',
      label: <Space><BgColorsOutlined />视觉条件</Space>,
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
          <BgColorsOutlined style={{ color: '#722ed1' }} />
          <span>视觉规则编辑器</span>
          <Tag color="purple">视觉数据</Tag>
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

export default VisualRuleEditor;