import React, { useState } from 'react';
import { Card, Input, Button, Typography, Space, Tag, Collapse, List } from 'antd';
import { CodeOutlined, FunctionOutlined, InfoCircleOutlined } from '@ant-design/icons';

const { TextArea } = Input;
const { Text, Paragraph } = Typography;
const { Panel } = Collapse;

export interface ExpressionEditorProps {
  value?: string;
  onChange?: (expression: string) => void;
  dataType?: string;
  availableFunctions?: ExpressionFunction[];
  availableVariables?: ExpressionVariable[];
  placeholder?: string;
  rows?: number;
}

interface ExpressionFunction {
  name: string;
  description: string;
  syntax: string;
  example: string;
  category: string;
  parameters?: string[];
}

interface ExpressionVariable {
  name: string;
  description: string;
  type: string;
  example?: string;
}

/**
 * 表达式编辑器组件
 * 支持代码提示、函数帮助、语法检查
 */
const ExpressionEditor: React.FC<ExpressionEditorProps> = ({
  value = '',
  onChange,
  dataType = 'generic',
  availableFunctions = [],
  availableVariables = [],
  placeholder = '输入表达式...',
  rows = 4
}) => {
  const [showHelp, setShowHelp] = useState(false);

  // 基础函数库
  const baseFunctions: ExpressionFunction[] = [
    // 数学函数
    { name: 'abs', description: '绝对值', syntax: 'abs(number)', example: 'abs(-5) // 返回 5', category: '数学函数' },
    { name: 'max', description: '最大值', syntax: 'max(a, b)', example: 'max(10, 20) // 返回 20', category: '数学函数' },
    { name: 'min', description: '最小值', syntax: 'min(a, b)', example: 'min(10, 20) // 返回 10', category: '数学函数' },
    { name: 'sqrt', description: '平方根', syntax: 'sqrt(number)', example: 'sqrt(16) // 返回 4', category: '数学函数' },
    { name: 'pow', description: '幂运算', syntax: 'pow(base, exp)', example: 'pow(2, 3) // 返回 8', category: '数学函数' },
    { name: 'round', description: '四舍五入', syntax: 'round(number)', example: 'round(3.14) // 返回 3', category: '数学函数' },
    
    // 字符串函数
    { name: 'len', description: '字符串长度', syntax: 'len(string)', example: 'len("hello") // 返回 5', category: '字符串函数' },
    { name: 'contains', description: '包含检查', syntax: 'contains(str, substr)', example: 'contains("hello", "ell") // 返回 true', category: '字符串函数' },
    { name: 'startswith', description: '开头匹配', syntax: 'startswith(str, prefix)', example: 'startswith("hello", "he") // 返回 true', category: '字符串函数' },
    { name: 'endswith', description: '结尾匹配', syntax: 'endswith(str, suffix)', example: 'endswith("hello", "lo") // 返回 true', category: '字符串函数' },
    { name: 'upper', description: '转大写', syntax: 'upper(string)', example: 'upper("hello") // 返回 "HELLO"', category: '字符串函数' },
    { name: 'lower', description: '转小写', syntax: 'lower(string)', example: 'lower("HELLO") // 返回 "hello"', category: '字符串函数' },
    
    // 时间函数
    { name: 'now', description: '当前时间戳', syntax: 'now()', example: 'now() // 返回当前Unix时间戳', category: '时间函数' },
    { name: 'timeFormat', description: '时间格式化', syntax: 'timeFormat(timestamp, format)', example: 'timeFormat(now(), "2006-01-02 15:04:05")', category: '时间函数' }
  ];

  // 数据类型特定函数
  const getDataTypeFunctions = (type: string): ExpressionFunction[] => {
    switch (type) {
      case 'vector3d':
        return [
          { name: 'vectorMagnitude', description: '3D向量模长', syntax: 'vectorMagnitude(x, y, z)', example: 'vectorMagnitude(acceleration.x, acceleration.y, acceleration.z)', category: '向量函数' }
        ];
      case 'vector':
      case 'vector_generic':
        return [
          { name: 'genericVectorMagnitude', description: '通用向量模长', syntax: 'genericVectorMagnitude(vector)', example: 'genericVectorMagnitude(sensor_vector)', category: '向量函数' },
          { name: 'vectorMean', description: '向量元素平均值', syntax: 'vectorMean(vector)', example: 'vectorMean(sensor_vector)', category: '向量函数' }
        ];
      case 'array':
        return [
          { name: 'arraySum', description: '数组求和', syntax: 'arraySum(array)', example: 'arraySum(sensor_readings)', category: '数组函数' },
          { name: 'arrayMean', description: '数组平均值', syntax: 'arrayMean(array)', example: 'arrayMean(sensor_readings)', category: '数组函数' },
          { name: 'arrayMax', description: '数组最大值', syntax: 'arrayMax(array)', example: 'arrayMax(sensor_readings)', category: '数组函数' },
          { name: 'arrayMin', description: '数组最小值', syntax: 'arrayMin(array)', example: 'arrayMin(sensor_readings)', category: '数组函数' }
        ];
      case 'geospatial':
      case 'location':
        return [
          { name: 'distance', description: 'GPS距离计算', syntax: 'distance(lat1, lon1, lat2, lon2)', example: 'distance(location.latitude, location.longitude, 39.9, 116.4)', category: '地理函数' }
        ];
      default:
        return [];
    }
  };

  // 基础变量
  const baseVariables: ExpressionVariable[] = [
    { name: 'value', description: '当前数据值', type: 'any', example: 'value > 30' },
    { name: 'device_id', description: '设备ID', type: 'string', example: 'contains(device_id, "sensor")' },
    { name: 'key', description: '数据键名', type: 'string', example: 'key == "temperature"' },
    { name: 'timestamp', description: '时间戳', type: 'number', example: 'timestamp > now() - 3600' },
    { name: 'quality', description: '数据质量', type: 'number', example: 'quality == 0' }
  ];

  // 合并所有函数和变量
  const allFunctions = [
    ...baseFunctions,
    ...getDataTypeFunctions(dataType),
    ...availableFunctions
  ];

  const allVariables = [
    ...baseVariables,
    ...availableVariables
  ];

  // 按类别分组函数
  const functionsByCategory = allFunctions.reduce((acc, func) => {
    if (!acc[func.category]) {
      acc[func.category] = [];
    }
    acc[func.category].push(func);
    return acc;
  }, {} as Record<string, ExpressionFunction[]>);

  const insertText = (text: string) => {
    if (onChange) {
      const newValue = value + text;
      onChange(newValue);
    }
  };

  const renderFunctionHelp = () => (
    <Collapse size="small">
      {Object.entries(functionsByCategory).map(([category, functions]) => (
        <Panel header={category} key={category}>
          <List
            size="small"
            dataSource={functions}
            renderItem={(func) => (
              <List.Item
                actions={[
                  <Button
                    type="link"
                    size="small"
                    onClick={() => insertText(func.syntax)}
                  >
                    插入
                  </Button>
                ]}
              >
                <List.Item.Meta
                  title={
                    <Space>
                      <Text code>{func.name}</Text>
                      <Text type="secondary">{func.description}</Text>
                    </Space>
                  }
                  description={
                    <div>
                      <div><Text strong>语法：</Text><Text code>{func.syntax}</Text></div>
                      <div><Text strong>示例：</Text><Text code style={{ color: '#52c41a' }}>{func.example}</Text></div>
                    </div>
                  }
                />
              </List.Item>
            )}
          />
        </Panel>
      ))}
    </Collapse>
  );

  const renderVariableHelp = () => (
    <List
      size="small"
      header={<Text strong>可用变量</Text>}
      dataSource={allVariables}
      renderItem={(variable) => (
        <List.Item
          actions={[
            <Button
              type="link"
              size="small"
              onClick={() => insertText(variable.name)}
            >
              插入
            </Button>
          ]}
        >
          <List.Item.Meta
            title={
              <Space>
                <Text code>{variable.name}</Text>
                <Tag color="blue">{variable.type}</Tag>
                <Text type="secondary">{variable.description}</Text>
              </Space>
            }
            description={variable.example && (
              <div><Text strong>示例：</Text><Text code style={{ color: '#52c41a' }}>{variable.example}</Text></div>
            )}
          />
        </List.Item>
      )}
    />
  );

  return (
    <div>
      <Card
        title={
          <Space>
            <CodeOutlined />
            <Text>表达式编辑器</Text>
            <Button
              type="link"
              icon={<InfoCircleOutlined />}
              onClick={() => setShowHelp(!showHelp)}
            >
              {showHelp ? '隐藏帮助' : '显示帮助'}
            </Button>
          </Space>
        }
        size="small"
      >
        <TextArea
          value={value}
          onChange={(e) => onChange?.(e.target.value)}
          placeholder={placeholder}
          rows={rows}
          style={{ fontFamily: 'Consolas, Monaco, monospace' }}
        />
        
        {showHelp && (
          <div style={{ marginTop: 16 }}>
            <Card size="small" title={<Text><FunctionOutlined /> 函数参考</Text>}>
              {renderFunctionHelp()}
            </Card>
            
            <Card size="small" title="变量参考" style={{ marginTop: 16 }}>
              {renderVariableHelp()}
            </Card>
          </div>
        )}

        <div style={{ marginTop: 8 }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            支持JavaScript语法，使用 && (AND), || (OR), ! (NOT) 进行逻辑运算
          </Text>
        </div>
      </Card>
    </div>
  );
};

export default ExpressionEditor;