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

interface ConditionItem {
  type: 'simple' | 'expression';
  // Simple condition fields
  field?: string;
  operator?: string;
  value?: any;
  // Expression condition field
  expression?: string;
}

interface ConditionState {
  conditionType: string;
  simpleCondition: SimpleCondition;
  andConditions: ConditionItem[];
  orConditions: ConditionItem[];
  expression: string;
}

const ConditionForm: React.FC<ConditionFormProps> = ({ value, onChange }) => {
  // 使用ref来避免无限循环
  const lastValueRef = useRef<Condition | undefined>();
  const stateRef = useRef<ConditionState>({
    conditionType: 'simple',
    simpleCondition: { field: '', operator: 'eq', value: '' },
    andConditions: [],
    orConditions: [],
    expression: ''
  });

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

  // 常用字段（包含复合数据类型支持）
  const commonFields = [
    // 基础数据字段
    'device_id', 'key', 'value', 'timestamp', 'quality', 'unit',
    'temperature', 'humidity', 'pressure', 'status',
    
    // 复合数据字段 - GPS/地理位置
    'latitude', 'longitude', 'altitude', 'accuracy', 'speed', 'heading',
    'elevation_category', 'speed_category',
    
    // 复合数据字段 - 三轴向量
    'x', 'y', 'z', 'magnitude', 'x_ratio', 'y_ratio', 'z_ratio', 'dominant_axis',
    
    // 复合数据字段 - 颜色
    'r', 'g', 'b', 'a', 'hue', 'saturation', 'lightness',
    
    // 复合数据字段 - 通用向量
    'dimension', 'norm', 'dominant_dimension', 'dominant_value',
    
    // 复合数据字段 - 数组/矩阵
    'size', 'length', 'rows', 'cols', 'data_type', 'numeric_count', 'null_count',
    
    // 复合数据字段 - 时间序列
    'duration', 'avg_interval', 'trend', 'trend_slope'
  ];

  // 深度比较函数
  const deepEqual = useCallback((obj1: any, obj2: any): boolean => {
    if (obj1 === obj2) return true;
    if (!obj1 || !obj2) return obj1 === obj2;
    if (typeof obj1 !== typeof obj2) return false;
    if (typeof obj1 !== 'object') return obj1 === obj2;
    
    const keys1 = Object.keys(obj1);
    const keys2 = Object.keys(obj2);
    if (keys1.length !== keys2.length) return false;
    
    for (let key of keys1) {
      if (!keys2.includes(key)) return false;
      if (!deepEqual(obj1[key], obj2[key])) return false;
    }
    return true;
  }, []);

  // 使用useMemo来同步外部状态，完全避免useEffect
  const currentState = useMemo((): ConditionState => {
    
    // 如果没有值，返回默认状态
    if (!value) {
      const defaultState: ConditionState = {
        conditionType: 'simple',
        simpleCondition: { field: '', operator: 'eq', value: '' },
        andConditions: [],
        orConditions: [],
        expression: ''
      };
      lastValueRef.current = undefined;
      stateRef.current = defaultState;
      return defaultState;
    }

    // 深度比较，只有真正变化时才重新计算
    if (!deepEqual(value, lastValueRef.current)) {
      
      let newState: ConditionState;
      
      if (value.type === 'simple') {
        newState = {
          conditionType: 'simple',
          simpleCondition: {
            field: value.field || '',
            operator: value.operator || 'eq',
            value: value.value || ''
          },
          andConditions: [],
          orConditions: [],
          expression: ''
        };
      } else if (value.type === 'and' && value.and) {
        newState = {
          conditionType: 'and',
          simpleCondition: { field: '', operator: 'eq', value: '' },
          andConditions: value.and.map(cond => {
            if (cond.type === 'simple') {
              return {
                type: 'simple',
                field: cond.field || '',
                operator: cond.operator || 'eq',
                value: cond.value || ''
              };
            } else if (cond.type === 'expression') {
              return {
                type: 'expression',
                expression: cond.expression || ''
              };
            }
            return {
              type: 'simple',
              field: '',
              operator: 'eq',
              value: ''
            };
          }),
          orConditions: [],
          expression: ''
        };
      } else if (value.type === 'or' && value.or) {
        newState = {
          conditionType: 'or',
          simpleCondition: { field: '', operator: 'eq', value: '' },
          andConditions: [],
          orConditions: value.or.map(cond => {
            if (cond.type === 'simple') {
              return {
                type: 'simple',
                field: cond.field || '',
                operator: cond.operator || 'eq',
                value: cond.value || ''
              };
            } else if (cond.type === 'expression') {
              return {
                type: 'expression',
                expression: cond.expression || ''
              };
            }
            return {
              type: 'simple',
              field: '',
              operator: 'eq',
              value: ''
            };
          }),
          expression: ''
        };
      } else if (value.type === 'expression') {
        newState = {
          conditionType: 'expression',
          simpleCondition: { field: '', operator: 'eq', value: '' },
          andConditions: [],
          orConditions: [],
          expression: value.expression || ''
        };
      } else {
        // 默认状态
        newState = {
          conditionType: 'simple',
          simpleCondition: { field: '', operator: 'eq', value: '' },
          andConditions: [],
          orConditions: [],
          expression: ''
        };
      }
      
      lastValueRef.current = JSON.parse(JSON.stringify(value));
      stateRef.current = newState;
      return newState;
    }
    
    return stateRef.current;
  }, [value, deepEqual]);

  // 构建条件对象
  const buildCondition = useCallback((state: ConditionState): Condition => {
    let result: Condition;
    
    if (state.conditionType === 'simple') {
      result = {
        type: 'simple',
        field: state.simpleCondition.field,
        operator: state.simpleCondition.operator,
        value: state.simpleCondition.value
      };
    } else if (state.conditionType === 'and') {
      result = {
        type: 'and',
        and: state.andConditions.map(cond => {
          if (cond.type === 'expression') {
            return {
              type: 'expression',
              expression: cond.expression
            };
          }
          return {
            type: 'simple',
            field: cond.field,
            operator: cond.operator,
            value: cond.value
          };
        })
      };
    } else if (state.conditionType === 'or') {
      result = {
        type: 'or',
        or: state.orConditions.map(cond => {
          if (cond.type === 'expression') {
            return {
              type: 'expression',
              expression: cond.expression
            };
          }
          return {
            type: 'simple',
            field: cond.field,
            operator: cond.operator,
            value: cond.value
          };
        })
      };
    } else if (state.conditionType === 'expression') {
      result = {
        type: 'expression',
        expression: state.expression
      };
    } else {
      result = { type: 'simple', field: '', operator: 'eq', value: '' };
    }
    
    return result;
  }, []);

  // 处理条件类型变更
  const handleTypeChange = useCallback((type: string) => {
    // 只有在条件类型真正改变时才处理
    if (currentState.conditionType !== type) {
      
      const newState: ConditionState = { ...currentState };
      newState.conditionType = type;
      
      // 重置其他状态
      if (type === 'and' && newState.andConditions.length === 0) {
        newState.andConditions = [
          { type: 'simple', field: '', operator: 'eq', value: '' },
          { type: 'simple', field: '', operator: 'eq', value: '' }
        ];
      } else if (type === 'or' && newState.orConditions.length === 0) {
        newState.orConditions = [
          { type: 'simple', field: '', operator: 'eq', value: '' },
          { type: 'simple', field: '', operator: 'eq', value: '' }
        ];
      }
      
      stateRef.current = newState;
      const condition = buildCondition(newState);
      onChange?.(condition);
    }
  }, [currentState, buildCondition, onChange]);

  // 渲染简单条件表单
  const renderSimpleCondition = (
    condition: SimpleCondition,
    onChange: ((index: number, field: keyof SimpleCondition, value: any) => void) | ((field: keyof SimpleCondition, value: any) => void),
    index: number = 0
  ) => (
    <Row gutter={8} key={index}>
      <Col span={7}>
        <Select
          placeholder="选择字段"
          value={condition.field}
          onChange={(value) => {
            if (onChange.length === 3) {
              // 处理 (index, field, value) 形式
              (onChange as (index: number, field: keyof SimpleCondition, value: any) => void)(index, 'field', value);
            } else {
              // 处理 (field, value) 形式  
              (onChange as (field: keyof SimpleCondition, value: any) => void)('field', value);
            }
          }}
          style={{ width: '100%' }}
          showSearch
          allowClear
        >
          <Select.OptGroup label="📊 基础数据字段">
            <Option key="device_id" value="device_id">设备ID</Option>
            <Option key="key" value="key">数据键</Option>
            <Option key="value" value="value">数据值</Option>
            <Option key="timestamp" value="timestamp">时间戳</Option>
            <Option key="quality" value="quality">质量码</Option>
            <Option key="unit" value="unit">单位</Option>
          </Select.OptGroup>
          
          <Select.OptGroup label="🌡️ 传感器数据">
            <Option key="temperature" value="temperature">温度</Option>
            <Option key="humidity" value="humidity">湿度</Option>
            <Option key="pressure" value="pressure">压力</Option>
            <Option key="status" value="status">状态</Option>
          </Select.OptGroup>
          
          <Select.OptGroup label="📍 GPS位置数据">
            <Option key="latitude" value="latitude">纬度</Option>
            <Option key="longitude" value="longitude">经度</Option>
            <Option key="altitude" value="altitude">海拔</Option>
            <Option key="accuracy" value="accuracy">GPS精度</Option>
            <Option key="speed" value="speed">移动速度</Option>
            <Option key="heading" value="heading">方向角</Option>
            <Option key="elevation_category" value="elevation_category">海拔等级</Option>
            <Option key="speed_category" value="speed_category">速度等级</Option>
          </Select.OptGroup>
          
          <Select.OptGroup label="📐 三轴向量数据">
            <Option key="x" value="x">X轴数值</Option>
            <Option key="y" value="y">Y轴数值</Option>
            <Option key="z" value="z">Z轴数值</Option>
            <Option key="magnitude" value="magnitude">向量模长</Option>
            <Option key="x_ratio" value="x_ratio">X轴比例</Option>
            <Option key="y_ratio" value="y_ratio">Y轴比例</Option>
            <Option key="z_ratio" value="z_ratio">Z轴比例</Option>
            <Option key="dominant_axis" value="dominant_axis">主导轴</Option>
          </Select.OptGroup>
          
          <Select.OptGroup label="🎨 颜色数据">
            <Option key="r" value="r">红色分量</Option>
            <Option key="g" value="g">绿色分量</Option>
            <Option key="b" value="b">蓝色分量</Option>
            <Option key="a" value="a">透明度</Option>
            <Option key="hue" value="hue">色相</Option>
            <Option key="saturation" value="saturation">饱和度</Option>
            <Option key="lightness" value="lightness">亮度</Option>
          </Select.OptGroup>
          
          <Select.OptGroup label="🔢 向量/数组/矩阵">
            <Option key="dimension" value="dimension">维度</Option>
            <Option key="size" value="size">大小</Option>
            <Option key="length" value="length">长度</Option>
            <Option key="rows" value="rows">行数</Option>
            <Option key="cols" value="cols">列数</Option>
            <Option key="norm" value="norm">范数</Option>
            <Option key="dominant_dimension" value="dominant_dimension">主导维度</Option>
            <Option key="data_type" value="data_type">数据类型</Option>
            <Option key="numeric_count" value="numeric_count">数值数量</Option>
            <Option key="null_count" value="null_count">空值数量</Option>
          </Select.OptGroup>
          
          <Select.OptGroup label="📈 时间序列">
            <Option key="duration" value="duration">总时长</Option>
            <Option key="avg_interval" value="avg_interval">平均间隔</Option>
            <Option key="trend" value="trend">趋势</Option>
            <Option key="trend_slope" value="trend_slope">趋势斜率</Option>
          </Select.OptGroup>
        </Select>
      </Col>
      <Col span={6}>
        <Select
          value={condition.operator}
          onChange={(value) => {
            if (onChange.length === 3) {
              (onChange as (index: number, field: keyof SimpleCondition, value: any) => void)(index, 'operator', value);
            } else {
              (onChange as (field: keyof SimpleCondition, value: any) => void)('operator', value);
            }
          }}
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
            onChange={(value) => {
              if (onChange.length === 3) {
                (onChange as (index: number, field: keyof SimpleCondition, value: any) => void)(index, 'value', value);
              } else {
                (onChange as (field: keyof SimpleCondition, value: any) => void)('value', value);
              }
            }}
            style={{ width: '100%' }}
          />
        ) : ['gt', 'gte', 'lt', 'lte'].includes(condition.operator) ? (
          <InputNumber
            placeholder="数值"
            value={condition.value}
            onChange={(value) => {
              if (onChange.length === 3) {
                (onChange as (index: number, field: keyof SimpleCondition, value: any) => void)(index, 'value', value);
              } else {
                (onChange as (field: keyof SimpleCondition, value: any) => void)('value', value);
              }
            }}
            style={{ width: '100%' }}
          />
        ) : (
          <Input
            placeholder="比较值"
            value={condition.value}
            onChange={(e) => {
              if (onChange.length === 3) {
                (onChange as (index: number, field: keyof SimpleCondition, value: any) => void)(index, 'value', e.target.value);
              } else {
                (onChange as (field: keyof SimpleCondition, value: any) => void)('value', e.target.value);
              }
            }}
          />
        )}
      </Col>
    </Row>
  );

  // 渲染复合条件表单（支持简单条件和表达式条件）
  const renderCompoundCondition = (
    condition: ConditionItem,
    onChange: (index: number, field: keyof SimpleCondition | 'type' | 'expression', value: any) => void,
    index: number,
    onDelete?: () => void
  ) => (
    <div key={index} style={{ border: '1px solid #d9d9d9', padding: 12, borderRadius: 6, marginBottom: 8 }}>
      <Row gutter={8} style={{ marginBottom: 8 }}>
        <Col span={6}>
          <Select
            placeholder="条件类型"
            value={condition.type}
            onChange={(value) => onChange(index, 'type', value)}
            style={{ width: '100%' }}
          >
            <Option key="simple" value="simple">
              <Space>
                <Tag color="blue" size="small">简单</Tag>
                字段条件
              </Space>
            </Option>
            <Option key="expression" value="expression">
              <Space>
                <Tag color="purple" size="small">表达式</Tag>
                自定义表达式
              </Space>
            </Option>
          </Select>
        </Col>
        <Col span={15}>
          {condition.type === 'simple' ? (
            <Row gutter={4}>
              <Col span={8}>
                <Select
                  placeholder="选择字段"
                  value={condition.field}
                  onChange={(value) => onChange(index, 'field', value)}
                  style={{ width: '100%' }}
                  showSearch
                  allowClear
                >
                  <Select.OptGroup label="📊 基础数据字段">
                    <Option key="device_id" value="device_id">设备ID</Option>
                    <Option key="key" value="key">数据键</Option>
                    <Option key="value" value="value">数据值</Option>
                    <Option key="timestamp" value="timestamp">时间戳</Option>
                    <Option key="quality" value="quality">质量码</Option>
                    <Option key="unit" value="unit">单位</Option>
                  </Select.OptGroup>
                  
                  <Select.OptGroup label="🌡️ 传感器数据">
                    <Option key="temperature" value="temperature">温度</Option>
                    <Option key="humidity" value="humidity">湿度</Option>
                    <Option key="pressure" value="pressure">压力</Option>
                    <Option key="status" value="status">状态</Option>
                  </Select.OptGroup>
                  
                  <Select.OptGroup label="📍 GPS位置数据">
                    <Option key="latitude" value="latitude">纬度</Option>
                    <Option key="longitude" value="longitude">经度</Option>
                    <Option key="altitude" value="altitude">海拔</Option>
                    <Option key="speed" value="speed">移动速度</Option>
                    <Option key="heading" value="heading">方向角</Option>
                  </Select.OptGroup>
                  
                  <Select.OptGroup label="📐 三轴向量数据">
                    <Option key="x" value="x">X轴数值</Option>
                    <Option key="y" value="y">Y轴数值</Option>
                    <Option key="z" value="z">Z轴数值</Option>
                    <Option key="magnitude" value="magnitude">向量模长</Option>
                    <Option key="dominant_axis" value="dominant_axis">主导轴</Option>
                  </Select.OptGroup>
                  
                  <Select.OptGroup label="🎨 颜色数据">
                    <Option key="r" value="r">红色分量</Option>
                    <Option key="g" value="g">绿色分量</Option>
                    <Option key="b" value="b">蓝色分量</Option>
                    <Option key="hue" value="hue">色相</Option>
                  </Select.OptGroup>
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
              <Col span={10}>
                {['exists'].includes(condition.operator || '') ? (
                  <Input disabled placeholder="无需填写" />
                ) : ['in'].includes(condition.operator || '') ? (
                  <Select
                    mode="tags"
                    placeholder="输入多个值"
                    value={Array.isArray(condition.value) ? condition.value : []}
                    onChange={(value) => onChange(index, 'value', value)}
                    style={{ width: '100%' }}
                  />
                ) : ['gt', 'gte', 'lt', 'lte'].includes(condition.operator || '') ? (
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
            </Row>
          ) : (
            <TextArea
              rows={2}
              placeholder="例如: temperature > 30 && magnitude > 10.0 || hue between 120,240"
              value={condition.expression}
              onChange={(e) => onChange(index, 'expression', e.target.value)}
            />
          )}
        </Col>
        <Col span={3}>
          {onDelete && (
            <Button
              type="text"
              danger
              icon={<DeleteOutlined />}
              onClick={onDelete}
            />
          )}
        </Col>
      </Row>
      {condition.type === 'expression' && (
        <div style={{ fontSize: 12, color: '#666' }}>
          支持的运算符：&amp;&amp; (且), || (或), ! (非), ==, !=, &gt;, &gt;=, &lt;, &lt;=
        </div>
      )}
    </div>
  );

  // 处理简单条件变更
  const handleSimpleConditionChange = useCallback((field: keyof SimpleCondition, value: any) => {
    const newState: ConditionState = { ...currentState };
    newState.simpleCondition = { ...newState.simpleCondition, [field]: value };
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 处理AND条件变更
  const handleAndConditionChange = useCallback((index: number, field: keyof SimpleCondition | 'type' | 'expression', value: any) => {
    const newState: ConditionState = { ...currentState };
    newState.andConditions = [...newState.andConditions];
    
    if (field === 'type') {
      // 更改条件类型，重置相关字段
      if (value === 'expression') {
        newState.andConditions[index] = {
          type: 'expression',
          expression: ''
        };
      } else {
        newState.andConditions[index] = {
          type: 'simple',
          field: '',
          operator: 'eq',
          value: ''
        };
      }
    } else {
      newState.andConditions[index] = { ...newState.andConditions[index], [field]: value };
    }
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 处理OR条件变更
  const handleOrConditionChange = useCallback((index: number, field: keyof SimpleCondition | 'type' | 'expression', value: any) => {
    const newState: ConditionState = { ...currentState };
    newState.orConditions = [...newState.orConditions];
    
    if (field === 'type') {
      // 更改条件类型，重置相关字段
      if (value === 'expression') {
        newState.orConditions[index] = {
          type: 'expression',
          expression: ''
        };
      } else {
        newState.orConditions[index] = {
          type: 'simple',
          field: '',
          operator: 'eq',
          value: ''
        };
      }
    } else {
      newState.orConditions[index] = { ...newState.orConditions[index], [field]: value };
    }
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 处理表达式变更
  const handleExpressionChange = useCallback((value: string) => {
    const newState: ConditionState = { ...currentState };
    newState.expression = value;
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 添加AND条件
  const addAndCondition = useCallback(() => {
    const newState: ConditionState = { ...currentState };
    newState.andConditions = [...newState.andConditions, { type: 'simple', field: '', operator: 'eq', value: '' }];
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 删除AND条件
  const removeAndCondition = useCallback((index: number) => {
    const newState: ConditionState = { ...currentState };
    newState.andConditions = newState.andConditions.filter((_, i) => i !== index);
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 添加OR条件
  const addOrCondition = useCallback(() => {
    const newState: ConditionState = { ...currentState };
    newState.orConditions = [...newState.orConditions, { type: 'simple', field: '', operator: 'eq', value: '' }];
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  // 删除OR条件
  const removeOrCondition = useCallback((index: number) => {
    const newState: ConditionState = { ...currentState };
    newState.orConditions = newState.orConditions.filter((_, i) => i !== index);
    
    stateRef.current = newState;
    const condition = buildCondition(newState);
    onChange?.(condition);
  }, [currentState, buildCondition, onChange]);

  return (
    <Card title="触发条件配置" size="small">
      <div>
        <Form.Item label="条件类型">
          <Select value={currentState.conditionType} onChange={handleTypeChange}>
            <Option key="simple" value="simple">
              <Space>
                <Tag color="blue">简单</Tag>
                单个字段条件
              </Space>
            </Option>
            <Option key="and" value="and">
              <Space>
                <Tag color="green">逻辑与</Tag>
                所有条件都满足
              </Space>
            </Option>
            <Option key="or" value="or">
              <Space>
                <Tag color="orange">逻辑或</Tag>
                任一条件满足
              </Space>
            </Option>
            <Option key="expression" value="expression">
              <Space>
                <Tag color="purple">表达式</Tag>
                自定义表达式
              </Space>
            </Option>
          </Select>
        </Form.Item>

        {currentState.conditionType === 'simple' && (
          <Form.Item label="条件设置">
            {renderSimpleCondition(currentState.simpleCondition, handleSimpleConditionChange, 0)}
          </Form.Item>
        )}

        {currentState.conditionType === 'and' && (
          <Form.Item label={
            <Space>
              逻辑与条件
              <Tooltip title="所有条件都必须满足才会触发规则">
                <QuestionCircleOutlined />
              </Tooltip>
            </Space>
          }>
            <Space direction="vertical" style={{ width: '100%' }}>
              {currentState.andConditions.map((condition, index) => (
                <div key={index}>
                  {index > 0 && (
                    <div style={{ textAlign: 'center', margin: '8px 0' }}>
                      <Tag color="green">且</Tag>
                    </div>
                  )}
                  {renderCompoundCondition(
                    condition, 
                    handleAndConditionChange, 
                    index,
                    index > 1 ? () => removeAndCondition(index) : undefined
                  )}
                </div>
              ))}
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={addAndCondition}
                style={{ width: '100%' }}
              >
                添加条件
              </Button>
            </Space>
          </Form.Item>
        )}

        {currentState.conditionType === 'or' && (
          <Form.Item label={
            <Space>
              逻辑或条件
              <Tooltip title="任意一个条件满足就会触发规则">
                <QuestionCircleOutlined />
              </Tooltip>
            </Space>
          }>
            <Space direction="vertical" style={{ width: '100%' }}>
              {currentState.orConditions.map((condition, index) => (
                <div key={index}>
                  {index > 0 && (
                    <div style={{ textAlign: 'center', margin: '8px 0' }}>
                      <Tag color="orange">或</Tag>
                    </div>
                  )}
                  {renderCompoundCondition(
                    condition, 
                    handleOrConditionChange, 
                    index,
                    index > 1 ? () => removeOrCondition(index) : undefined
                  )}
                </div>
              ))}
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={addOrCondition}
                style={{ width: '100%' }}
              >
                添加条件
              </Button>
            </Space>
          </Form.Item>
        )}

        {currentState.conditionType === 'expression' && (
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
              placeholder="例如: temperature > 30 && magnitude > 10.0 || hue between 120,240"
              value={currentState.expression}
              onChange={(e) => handleExpressionChange(e.target.value)}
            />
            <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
              支持的运算符：&amp;&amp; (且), || (或), ! (非), ==, !=, &gt;, &gt;=, &lt;, &lt;=
            </div>
          </Form.Item>
        )}
      </div>
    </Card>
  );
};

export default ConditionForm;