import React, { useState, useCallback } from 'react';
import {
  Card,
  Form,
  Select,
  Input,
  InputNumber,
  Space,
  Button,
  Row,
  Col,
  Tag,
  Tooltip,
  Tabs,
  Switch,
  Alert
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  QuestionCircleOutlined,
  EnvironmentOutlined,
  BorderOutlined,
  BgColorsOutlined,
  OrderedListOutlined,
  TableOutlined,
  LineChartOutlined
} from '@ant-design/icons';
import type { Condition } from '../types/rule';
import type { DataTypeOption } from './DataTypeSelector';

const { Option } = Select;
const { TextArea } = Input;
const { TabPane } = Tabs;

interface ComplexConditionFormProps {
  dataType: DataTypeOption;
  value?: Condition;
  onChange?: (condition: Condition) => void;
}

interface ComplexConditionItem {
  type: 'field' | 'expression' | 'spatial';
  field?: string;
  operator?: string;
  value?: any;
  expression?: string;
  spatial_type?: string;
  spatial_config?: any;
}

const ComplexConditionForm: React.FC<ComplexConditionFormProps> = ({
  dataType,
  value,
  onChange
}) => {
  const [conditionType, setConditionType] = useState<'simple' | 'and' | 'or' | 'expression'>('simple');
  const [conditions, setConditions] = useState<ComplexConditionItem[]>([]);

  // 根据数据类型获取可用字段
  const getFieldsForDataType = (dataType: DataTypeOption) => {
    switch (dataType.category) {
      case 'geospatial':
        return [
          { group: 'GPS基础字段', fields: [
            { key: 'latitude', name: '纬度', type: 'number' },
            { key: 'longitude', name: '经度', type: 'number' },
            { key: 'altitude', name: '海拔', type: 'number' },
            { key: 'accuracy', name: 'GPS精度', type: 'number' },
            { key: 'speed', name: '移动速度', type: 'number' },
            { key: 'heading', name: '方向角', type: 'number' }
          ]},
          { group: 'GPS扩展字段', fields: [
            { key: 'distance_to_origin', name: '到原点距离', type: 'number' },
            { key: 'elevation_category', name: '海拔等级', type: 'string' },
            { key: 'speed_category', name: '速度等级', type: 'string' },
            { key: 'in_bounds', name: '在边界内', type: 'boolean' }
          ]}
        ];
      case 'vector':
        return [
          { group: '向量分量', fields: [
            { key: 'x', name: 'X轴数值', type: 'number' },
            { key: 'y', name: 'Y轴数值', type: 'number' },
            { key: 'z', name: 'Z轴数值', type: 'number' }
          ]},
          { group: '向量属性', fields: [
            { key: 'magnitude', name: '向量模长', type: 'number' },
            { key: 'x_ratio', name: 'X轴比例', type: 'number' },
            { key: 'y_ratio', name: 'Y轴比例', type: 'number' },
            { key: 'z_ratio', name: 'Z轴比例', type: 'number' },
            { key: 'dominant_axis', name: '主导轴', type: 'string' }
          ]}
        ];
      case 'visual':
        return [
          { group: 'RGB颜色', fields: [
            { key: 'r', name: '红色分量', type: 'number' },
            { key: 'g', name: '绿色分量', type: 'number' },
            { key: 'b', name: '蓝色分量', type: 'number' },
            { key: 'a', name: '透明度', type: 'number' }
          ]},
          { group: 'HSL颜色', fields: [
            { key: 'hue', name: '色相', type: 'number' },
            { key: 'saturation', name: '饱和度', type: 'number' },
            { key: 'lightness', name: '亮度', type: 'number' }
          ]}
        ];
      case 'array':
        return [
          { group: '数组属性', fields: [
            { key: 'size', name: '数组大小', type: 'number' },
            { key: 'length', name: '数组长度', type: 'number' },
            { key: 'data_type', name: '元素类型', type: 'string' },
            { key: 'numeric_count', name: '数值元素数量', type: 'number' },
            { key: 'null_count', name: '空值数量', type: 'number' }
          ]},
          { group: '统计属性', fields: [
            { key: 'mean', name: '平均值', type: 'number' },
            { key: 'median', name: '中位数', type: 'number' },
            { key: 'std', name: '标准差', type: 'number' },
            { key: 'min', name: '最小值', type: 'number' },
            { key: 'max', name: '最大值', type: 'number' }
          ]}
        ];
      case 'matrix':
        return [
          { group: '矩阵结构', fields: [
            { key: 'rows', name: '行数', type: 'number' },
            { key: 'cols', name: '列数', type: 'number' },
            { key: 'size', name: '元素总数', type: 'number' },
            { key: 'data_type', name: '数据类型', type: 'string' }
          ]},
          { group: '矩阵属性', fields: [
            { key: 'determinant', name: '行列式', type: 'number' },
            { key: 'trace', name: '矩阵迹', type: 'number' },
            { key: 'rank', name: '矩阵秩', type: 'number' },
            { key: 'is_square', name: '是否方阵', type: 'boolean' }
          ]}
        ];
      case 'timeseries':
        return [
          { group: '时间序列属性', fields: [
            { key: 'duration', name: '总时长', type: 'number' },
            { key: 'data_points', name: '数据点数量', type: 'number' },
            { key: 'avg_interval', name: '平均间隔', type: 'number' },
            { key: 'start_time', name: '开始时间', type: 'string' },
            { key: 'end_time', name: '结束时间', type: 'string' }
          ]},
          { group: '趋势分析', fields: [
            { key: 'trend', name: '趋势方向', type: 'string' },
            { key: 'trend_slope', name: '趋势斜率', type: 'number' },
            { key: 'seasonality', name: '季节性', type: 'boolean' },
            { key: 'volatility', name: '波动性', type: 'number' }
          ]}
        ];
      default:
        return [];
    }
  };

  // 获取操作符列表
  const getOperators = (fieldType: string) => {
    const baseOperators = [
      { value: 'eq', label: '等于 (=)', types: ['number', 'string', 'boolean'] },
      { value: 'ne', label: '不等于 (≠)', types: ['number', 'string', 'boolean'] },
      { value: 'gt', label: '大于 (>)', types: ['number'] },
      { value: 'gte', label: '大于等于 (≥)', types: ['number'] },
      { value: 'lt', label: '小于 (<)', types: ['number'] },
      { value: 'lte', label: '小于等于 (≤)', types: ['number'] },
      { value: 'contains', label: '包含', types: ['string'] },
      { value: 'regex', label: '正则匹配', types: ['string'] },
      { value: 'in', label: '在数组中', types: ['number', 'string'] },
      { value: 'exists', label: '字段存在', types: ['number', 'string', 'boolean'] }
    ];

    return baseOperators.filter(op => op.types.includes(fieldType));
  };

  // 渲染字段选择器
  const renderFieldSelector = (condition: ComplexConditionItem, index: number) => {
    const fieldGroups = getFieldsForDataType(dataType);
    
    return (
      <Select
        placeholder="选择字段"
        value={condition.field}
        onChange={(value) => updateCondition(index, 'field', value)}
        style={{ width: '100%' }}
        showSearch
      >
        {fieldGroups.map(group => (
          <Select.OptGroup key={group.group} label={group.group}>
            {group.fields.map(field => (
              <Option key={field.key} value={field.key}>
                <Space>
                  {field.name}
                  <Tag size="small" color={field.type === 'number' ? 'blue' : field.type === 'string' ? 'green' : 'orange'}>
                    {field.type}
                  </Tag>
                </Space>
              </Option>
            ))}
          </Select.OptGroup>
        ))}
      </Select>
    );
  };

  // 渲染值输入器
  const renderValueInput = (condition: ComplexConditionItem, index: number) => {
    const fieldGroups = getFieldsForDataType(dataType);
    const allFields = fieldGroups.flatMap(g => g.fields);
    const fieldInfo = allFields.find(f => f.key === condition.field);
    const fieldType = fieldInfo?.type || 'string';

    if (condition.operator === 'exists') {
      return <Input disabled placeholder="无需填写" />;
    }

    if (condition.operator === 'in') {
      return (
        <Select
          mode="tags"
          placeholder="输入多个值"
          value={Array.isArray(condition.value) ? condition.value : []}
          onChange={(value) => updateCondition(index, 'value', value)}
          style={{ width: '100%' }}
        />
      );
    }

    if (fieldType === 'number') {
      return (
        <InputNumber
          placeholder="数值"
          value={condition.value}
          onChange={(value) => updateCondition(index, 'value', value)}
          style={{ width: '100%' }}
        />
      );
    }

    if (fieldType === 'boolean') {
      return (
        <Select
          value={condition.value}
          onChange={(value) => updateCondition(index, 'value', value)}
          style={{ width: '100%' }}
        >
          <Option value={true}>是/True</Option>
          <Option value={false}>否/False</Option>
        </Select>
      );
    }

    return (
      <Input
        placeholder="比较值"
        value={condition.value}
        onChange={(e) => updateCondition(index, 'value', e.target.value)}
      />
    );
  };

  // 更新条件
  const updateCondition = useCallback((index: number, field: string, value: any) => {
    const newConditions = [...conditions];
    newConditions[index] = { ...newConditions[index], [field]: value };
    setConditions(newConditions);
    
    // 构建并触发onChange
    buildAndEmitCondition(conditionType, newConditions);
  }, [conditions, conditionType]);

  // 构建条件对象
  const buildAndEmitCondition = useCallback((type: string, conditionList: ComplexConditionItem[]) => {
    let result: Condition;

    if (type === 'simple') {
      const condition = conditionList[0];
      if (!condition) return;
      
      result = {
        type: 'simple',
        field: condition.field,
        operator: condition.operator,
        value: condition.value
      };
    } else if (type === 'and') {
      result = {
        type: 'and',
        and: conditionList.map(cond => ({
          type: 'simple',
          field: cond.field,
          operator: cond.operator,
          value: cond.value
        }))
      };
    } else if (type === 'or') {
      result = {
        type: 'or',
        or: conditionList.map(cond => ({
          type: 'simple',
          field: cond.field,
          operator: cond.operator,
          value: cond.value
        }))
      };
    } else {
      result = { type: 'simple', field: '', operator: 'eq', value: '' };
    }

    onChange?.(result);
  }, [onChange]);

  // 添加条件
  const addCondition = () => {
    const newConditions = [...conditions, { type: 'field', field: '', operator: 'eq', value: '' }];
    setConditions(newConditions);
    buildAndEmitCondition(conditionType, newConditions);
  };

  // 删除条件
  const removeCondition = (index: number) => {
    const newConditions = conditions.filter((_, i) => i !== index);
    setConditions(newConditions);
    buildAndEmitCondition(conditionType, newConditions);
  };

  // 渲染数据类型专用的空间条件编辑器
  const renderSpatialConditions = () => {
    if (dataType.category !== 'geospatial') return null;

    return (
      <TabPane tab="空间条件" key="spatial">
        <Alert
          message="空间条件"
          description="基于地理位置的空间关系条件，如距离、区域包含等"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        
        <Card size="small" title="距离条件">
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item label="参考纬度">
                <InputNumber
                  placeholder="如 39.9042"
                  style={{ width: '100%' }}
                  precision={6}
                />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="参考经度">
                <InputNumber
                  placeholder="如 116.4074"
                  style={{ width: '100%' }}
                  precision={6}
                />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="距离阈值">
                <InputNumber
                  placeholder="千米"
                  style={{ width: '100%' }}
                  addonAfter="km"
                />
              </Form.Item>
            </Col>
          </Row>
          
          <Form.Item label="距离条件">
            <Select placeholder="选择距离条件">
              <Option value="within">在范围内</Option>
              <Option value="outside">在范围外</Option>
              <Option value="approaching">正在接近</Option>
              <Option value="moving_away">正在远离</Option>
            </Select>
          </Form.Item>
        </Card>
      </TabPane>
    );
  };

  // 初始化条件
  React.useEffect(() => {
    if (!conditions.length) {
      setConditions([{ type: 'field', field: '', operator: 'eq', value: '' }]);
    }
  }, [conditions.length]);

  return (
    <Card 
      title={
        <Space>
          {dataType.icon}
          {dataType.name} 触发条件配置
        </Space>
      } 
      size="small"
    >
      <Tabs defaultActiveKey="basic">
        <TabPane tab="基础条件" key="basic">
          <Form.Item label="条件逻辑">
            <Select value={conditionType} onChange={setConditionType}>
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
            </Select>
          </Form.Item>

          <Space direction="vertical" style={{ width: '100%' }}>
            {conditions.map((condition, index) => (
              <Card key={index} size="small" style={{ border: '1px solid #d9d9d9' }}>
                {index > 0 && (
                  <div style={{ textAlign: 'center', marginBottom: 8 }}>
                    <Tag color={conditionType === 'and' ? 'green' : 'orange'}>
                      {conditionType === 'and' ? '且' : '或'}
                    </Tag>
                  </div>
                )}
                
                <Row gutter={8}>
                  <Col span={8}>
                    {renderFieldSelector(condition, index)}
                  </Col>
                  <Col span={6}>
                    <Select
                      value={condition.operator}
                      onChange={(value) => updateCondition(index, 'operator', value)}
                      style={{ width: '100%' }}
                    >
                      {getOperators('number').map(op => (
                        <Option key={op.value} value={op.value}>
                          {op.label}
                        </Option>
                      ))}
                    </Select>
                  </Col>
                  <Col span={7}>
                    {renderValueInput(condition, index)}
                  </Col>
                  <Col span={3}>
                    {(conditionType !== 'simple' || conditions.length > 1) && (
                      <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => removeCondition(index)}
                      />
                    )}
                  </Col>
                </Row>
              </Card>
            ))}

            {conditionType !== 'simple' && (
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={addCondition}
                style={{ width: '100%' }}
              >
                添加条件
              </Button>
            )}
          </Space>
        </TabPane>

        {renderSpatialConditions()}

        <TabPane tab="高级表达式" key="expression">
          <Alert
            message="表达式条件"
            description={`使用表达式编写复杂的${dataType.name}条件判断逻辑`}
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
          />
          
          <Form.Item 
            label={
              <Space>
                表达式条件
                <Tooltip title={`使用${dataType.name}字段编写复杂表达式`}>
                  <QuestionCircleOutlined />
                </Tooltip>
              </Space>
            }
          >
            <TextArea
              rows={4}
              placeholder={`例如: latitude > 39.9 && longitude < 116.5 && magnitude > 5.0`}
              style={{ fontFamily: 'monospace' }}
            />
            <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
              支持的运算符：&amp;&amp; (且), || (或), ! (非), ==, !=, &gt;, &gt;=, &lt;, &lt;=
            </div>
          </Form.Item>
        </TabPane>
      </Tabs>
    </Card>
  );
};

export default ComplexConditionForm;