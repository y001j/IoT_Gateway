import React, { useState, useEffect } from 'react';
import { Modal, Card, Form, Input, InputNumber, Select, Space, Typography, Row, Col, Divider, Button, Switch, Tabs, Alert, Tag, Breadcrumb } from 'antd';
import { EnvironmentOutlined, InfoCircleOutlined, EditOutlined, EyeOutlined, CodeOutlined, CheckCircleOutlined, GlobalOutlined } from '@ant-design/icons';
import { useBaseRuleEditor } from './base/BaseRuleEditor';
import ConditionBuilder from './base/ConditionBuilder';
import ActionFormBuilder from './base/ActionFormBuilder';
import ExpressionEditor from './base/ExpressionEditor';
import { Rule, Condition, Action } from '../types/rule';

const { Option } = Select;
const { Text, Title } = Typography;
const { TextArea } = Input;

export interface GeospatialRuleEditorProps {
  visible: boolean;
  onClose: () => void;
  onSave: (rule: Rule) => Promise<void>;
  rule?: Rule;
}

/**
 * 地理空间规则编辑器
 * 专门处理GPS/地理位置相关的复合数据规则
 */
const GeospatialRuleEditor: React.FC<GeospatialRuleEditorProps> = ({
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
    title: '地理空间规则编辑器',
    dataTypeName: '地理位置数据'
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
        return rule.data_type.type || 'location';
      }
    }
    
    if (rule.tags) {
      const tagDataType = rule.tags['data_type'] || rule.tags['data_category'];
      if (tagDataType) return tagDataType;
    }
    
    // 根据规则名称和描述推断
    const nameDesc = `${rule.name || ''} ${rule.description || ''}`.toLowerCase();
    if (nameDesc.includes('location') || nameDesc.includes('地理') || nameDesc.includes('gps') || nameDesc.includes('位置')) {
      return 'location';
    }
    if (nameDesc.includes('geo') || nameDesc.includes('坐标') || nameDesc.includes('latitude') || nameDesc.includes('longitude')) {
      return 'location';
    }
    
    return 'location'; // 默认为地理位置数据
  };

  // 计算最终数据类型
  const finalDataType = rule ? inferDataTypeFromRule(rule) : 'location';

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

  // 地理空间特定字段
  const geospatialFields = [
    { value: 'location.latitude', label: '纬度', description: '地理坐标纬度值 (-90 到 90)' },
    { value: 'location.longitude', label: '经度', description: '地理坐标经度值 (-180 到 180)' },
    { value: 'location.altitude', label: '海拔高度', description: '海拔高度值 (米)' },
    { value: 'location.accuracy', label: '定位精度', description: 'GPS定位精度 (米)' },
    { value: 'location.speed', label: '移动速度', description: '移动速度 (公里/小时)' },
    { value: 'location.heading', label: '方向角', description: '移动方向角度 (0-360度)' },
    { value: 'location.distance', label: '距离', description: '距离参考点的距离' },
    { value: 'location.zone', label: '地理区域', description: '地理围栏区域标识' }
  ];

  // 地理空间特定动作
  const geospatialActions = [
    {
      value: 'geo_transform',
      label: '地理数据转换',
      description: '地理坐标系转换、距离计算、区域判断',
      configSchema: {
        transform_type: {
          type: 'string' as const,
          label: '转换类型',
          required: true,
          options: [
            { value: 'distance_calculation', label: '距离计算' },
            { value: 'coordinate_conversion', label: '坐标系转换' },
            { value: 'geofence_check', label: '地理围栏检查' },
            { value: 'area_calculation', label: '面积计算' }
          ]
        },
        reference_point: {
          type: 'object' as const,
          label: '参考点坐标',
          description: '用于距离计算的参考点 (纬度,经度)',
          placeholder: '{"latitude": 39.9, "longitude": 116.4}'
        },
        geofence_zones: {
          type: 'array' as const,
          label: '地理围栏区域',
          description: '定义地理围栏的边界区域'
        }
      }
    },
    {
      value: 'geo_expression',
      label: '地理表达式操作',
      description: '使用表达式对地理数据进行复杂计算和处理',
      configSchema: {
        expression: {
          type: 'textarea' as const,
          label: '表达式',
          required: true,
          placeholder: 'distance(location.latitude, location.longitude, 39.9, 116.4)',
          description: '支持地理函数和数学运算的表达式'
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
            { value: 'object', label: '地理对象' }
          ],
          defaultValue: 'number'
        }
      }
    },
    {
      value: 'geo_alert',
      label: '地理位置告警',
      description: '基于地理位置的告警通知',
      configSchema: {
        alert_type: {
          type: 'string' as const,
          label: '告警类型',
          options: [
            { value: 'geofence_entry', label: '进入地理围栏' },
            { value: 'geofence_exit', label: '离开地理围栏' },
            { value: 'distance_threshold', label: '距离阈值告警' },
            { value: 'speed_limit', label: '超速告警' }
          ]
        },
        message: {
          type: 'textarea' as const,
          label: '告警消息',
          required: true,
          placeholder: '设备{{.DeviceID}}位置: 纬度{{.location.latitude}}, 经度{{.location.longitude}}'
        }
      }
    }
  ];

  // 地理空间表达式函数
  const geospatialFunctions = [
    {
      name: 'distance',
      description: '计算两点间距离',
      syntax: 'distance(lat1, lon1, lat2, lon2)',
      example: 'distance(location.latitude, location.longitude, 39.9, 116.4) > 1000',
      category: '地理函数',
      parameters: ['lat1', 'lon1', 'lat2', 'lon2']
    },
    {
      name: 'inGeofence',
      description: '检查是否在地理围栏内',
      syntax: 'inGeofence(lat, lon, zoneName)',
      example: 'inGeofence(location.latitude, location.longitude, "beijing_area")',
      category: '地理函数',
      parameters: ['lat', 'lon', 'zoneName']
    },
    {
      name: 'bearing',
      description: '计算两点间方位角',
      syntax: 'bearing(lat1, lon1, lat2, lon2)',
      example: 'bearing(location.latitude, location.longitude, 39.9, 116.4)',
      category: '地理函数',
      parameters: ['lat1', 'lon1', 'lat2', 'lon2']
    }
  ];

  // 地理空间变量
  const geospatialVariables = [
    { name: 'location.latitude', description: '当前纬度', type: 'number', example: 'location.latitude > 39.8' },
    { name: 'location.longitude', description: '当前经度', type: 'number', example: 'location.longitude < 116.5' },
    { name: 'location.altitude', description: '海拔高度', type: 'number', example: 'location.altitude > 100' },
    { name: 'location.accuracy', description: 'GPS精度', type: 'number', example: 'location.accuracy < 10' },
    { name: 'location.speed', description: '移动速度', type: 'number', example: 'location.speed > 60' },
    { name: 'location.heading', description: '移动方向', type: 'number', example: 'location.heading > 180' }
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

  const renderGeospatialConfig = () => (
    <Card title={<Space><EnvironmentOutlined />地理位置配置</Space>} style={{ marginTop: 16 }}>
      <Alert
        message="地理空间数据类型"
        description="此数据类型专门处理GPS、坐标、位置等地理空间数据，支持WGS84等多种坐标系统和地理围栏功能"
        type="info"
        showIcon
      />
    </Card>
  );

  const renderConditionsSection = () => (
    <Card title="地理位置条件" style={{ marginTop: 16 }}>
      <ConditionBuilder
        value={conditions}
        onChange={setConditions}
        availableFields={['device_id', 'key', 'value', 'timestamp']}
        customFieldOptions={geospatialFields}
        allowedOperators={['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'regex']}
        supportExpressions={true}
        dataTypeName="地理位置"
      />
      
      <div style={{ marginTop: 16 }}>
        <ExpressionEditor
          value={conditions?.type === 'expression' ? conditions.expression : ''}
          onChange={(expr) => setConditions({ type: 'expression', expression: expr })}
          dataType="geospatial"
          availableFunctions={geospatialFunctions}
          availableVariables={geospatialVariables}
          placeholder="输入地理位置表达式，例如：distance(location.latitude, location.longitude, 39.9, 116.4) > 1000"
          rows={3}
        />
      </div>
    </Card>
  );

  const renderActionsSection = () => (
    <Card title="地理位置动作" style={{ marginTop: 16 }}>
      <ActionFormBuilder
        value={actions}
        onChange={setActions}
        availableActionTypes={[]} // 移除普通动作类型，只使用专门的地理位置动作
        customActionOptions={geospatialActions}
        dataTypeName="地理位置"
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
                  <Input placeholder="请输入地理位置规则名称" />
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
                placeholder="请输入地理位置规则描述" 
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
          
          {/* 地理空间配置 */}
          {renderGeospatialConfig()}
        </Card>
      )
    },
    {
      key: 'conditions',
      label: <Space><GlobalOutlined />地理条件</Space>,
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
          <EnvironmentOutlined style={{ color: '#52c41a' }} />
          <span>地理空间规则编辑器</span>
          <Tag color="success">地理位置数据</Tag>
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

export default GeospatialRuleEditor;