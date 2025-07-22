import React from 'react';
import { 
  Drawer, 
  Typography, 
  Collapse, 
  Tag, 
  Space, 
  Card,
  Divider,
  Alert,
  Table
} from 'antd';
import {
  InfoCircleOutlined,
  FireOutlined,
  FilterOutlined,
  SwapOutlined,
  ForwardOutlined,
  FunctionOutlined
} from '@ant-design/icons';

const { Title, Text, Paragraph } = Typography;
const { Panel } = Collapse;

interface RuleHelpProps {
  visible: boolean;
  onClose: () => void;
}

const RuleHelp: React.FC<RuleHelpProps> = ({ visible, onClose }) => {
  const conditionTypes = [
    {
      type: 'simple',
      name: '简单条件',
      description: '基于单个字段的条件判断',
      example: {
        type: 'simple',
        field: 'temperature',
        operator: 'gt',
        value: 30
      }
    },
    {
      type: 'and',
      name: '逻辑与',
      description: '所有子条件都必须满足',
      example: {
        type: 'and',
        and: [
          { type: 'simple', field: 'temperature', operator: 'gt', value: 20 },
          { type: 'simple', field: 'humidity', operator: 'lt', value: 80 }
        ]
      }
    },
    {
      type: 'or',
      name: '逻辑或',
      description: '任意一个子条件满足即可',
      example: {
        type: 'or',
        or: [
          { type: 'simple', field: 'status', operator: 'eq', value: 'error' },
          { type: 'simple', field: 'status', operator: 'eq', value: 'warning' }
        ]
      }
    },
    {
      type: 'expression',
      name: '表达式',
      description: '使用增强表达式引擎进行复杂判断，支持数学函数和内置函数',
      example: {
        type: 'expression',
        expression: 'abs(value - 25) > 5 && contains(device_id, "sensor") && len(device_id) > 5'
      }
    }
  ];

  const operators = [
    { op: 'eq', name: '等于', description: '字段值等于指定值' },
    { op: 'ne', name: '不等于', description: '字段值不等于指定值' },
    { op: 'gt', name: '大于', description: '字段值大于指定值（数值比较）' },
    { op: 'gte', name: '大于等于', description: '字段值大于等于指定值' },
    { op: 'lt', name: '小于', description: '字段值小于指定值' },
    { op: 'lte', name: '小于等于', description: '字段值小于等于指定值' },
    { op: 'contains', name: '包含', description: '字段值包含指定子字符串' },
    { op: 'startswith', name: '开始于', description: '字段值以指定字符串开始' },
    { op: 'endswith', name: '结束于', description: '字段值以指定字符串结束' },
    { op: 'regex', name: '正则匹配', description: '字段值匹配正则表达式（支持缓存）' },
    { op: 'in', name: '在范围内', description: '字段值在指定数组中' },
    { op: 'exists', name: '字段存在', description: '检查字段是否存在' }
  ];

  const actionTypes = [
    {
      type: 'alert',
      name: '告警',
      icon: <FireOutlined style={{ color: '#ff4d4f' }} />,
      description: '发送告警通知',
      config: {
        level: 'warning | error | info | critical',
        message: '告警消息模板，支持变量 {{.DeviceID}}, {{.Key}}, {{.Value}}',
        channels: '通道配置数组，支持console, webhook, email, sms, nats',
        throttle: '限流时间，如 "5m"'
      },
      example: {
        type: 'alert',
        config: {
          level: 'warning',
          message: '设备{{.DeviceID}}温度异常: {{.Value}}°C',
          channels: [
            { type: 'console', enabled: true },
            { type: 'nats', enabled: true, config: { subject: 'iot.alerts.temperature' } }
          ]
        }
      }
    },
    {
      type: 'transform',
      name: '数据转换',
      icon: <SwapOutlined style={{ color: '#1890ff' }} />,
      description: '增强的数据转换处理，支持表达式计算和NATS发布',
      config: {
        type: 'scale | offset | expression | unit_convert | lookup | format',
        parameters: '转换参数配置',
        output_key: '输出字段名',
        publish_subject: 'NATS发布主题（可选）',
        precision: '数值精度'
      },
      example: {
        type: 'transform',
        config: {
          type: 'expression',
          parameters: {
            expression: '(value - 32) * 5 / 9'
          },
          output_key: 'celsius_temp',
          output_type: 'float',
          precision: 2,
          publish_subject: 'iot.data.converted'
        }
      }
    },
    {
      type: 'filter',
      name: '数据过滤',
      icon: <FilterOutlined style={{ color: '#722ed1' }} />,
      description: '过滤或丢弃数据',
      config: {
        type: 'range | dedup | rate_limit',
        drop_on_match: '匹配时是否丢弃数据',
        pass_on_match: '匹配时是否通过数据'
      },
      example: {
        type: 'filter',
        config: {
          type: 'range',
          min: 0,
          max: 100,
          drop_on_match: false
        }
      }
    },
    {
      type: 'aggregate',
      name: '数据聚合',
      icon: <FunctionOutlined style={{ color: '#52c41a' }} />,
      description: '对数据进行聚合计算',
      config: {
        window_type: 'time | count',
        size: '窗口大小',
        functions: '聚合函数: avg, sum, count, max, min',
        group_by: '分组字段',
        output_key: '输出字段名'
      },
      example: {
        type: 'aggregate',
        config: {
          window_type: 'count',
          size: 10,
          functions: ['avg', 'max', 'min'],
          group_by: ['device_id'],
          output_key: '{{.Key}}_stats'
        }
      }
    },
    {
      type: 'forward',
      name: '数据转发',
      icon: <ForwardOutlined style={{ color: '#fa8c16' }} />,
      description: '简化的NATS数据转发，专注于消息总线转发',
      config: {
        subject: 'NATS主题，支持变量模板',
        include_metadata: '是否包含元数据',
        transform_data: '数据转换配置'
      },
      example: {
        type: 'forward',
        config: {
          subject: 'iot.data.processed.{{.DeviceID}}',
          include_metadata: true,
          transform_data: {
            add_timestamp: true,
            add_rule_info: true
          }
        }
      }
    }
  ];

  const commonFields = [
    { field: 'device_id', description: '设备ID标识符' },
    { field: 'key', description: '数据点键名，如 temperature, humidity' },
    { field: 'value', description: '数据点数值' },
    { field: 'timestamp', description: '数据时间戳' },
    { field: 'tags', description: '设备标签信息' },
    { field: 'quality', description: '数据质量标识' },
    { field: 'unit', description: '数据单位' }
  ];

  return (
    <Drawer
      title="规则配置帮助"
      width={800}
      open={visible}
      onClose={onClose}
      bodyStyle={{ padding: 24 }}
    >
      <Alert
        message="增强规则引擎说明"
        description="规则引擎用于实时处理IoT数据流，支持复杂条件匹配和多种动作执行。包含表达式引擎、正则缓存、增强转换动作和规则执行事件发布等功能。"
        type="info"
        icon={<InfoCircleOutlined />}
        style={{ marginBottom: 24 }}
      />

      <Collapse defaultActiveKey={['1']} size="large">
        <Panel header="📋 常用数据字段" key="1">
          <Table
            dataSource={commonFields}
            columns={[
              { title: '字段名', dataIndex: 'field', key: 'field', width: 120 },
              { title: '说明', dataIndex: 'description', key: 'description' }
            ]}
            pagination={false}
            size="small"
          />
        </Panel>

        <Panel header="🎯 条件类型配置" key="2">
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            {conditionTypes.map(condition => (
              <Card key={condition.type} size="small">
                <Title level={5}>
                  <Tag color="blue">{condition.type}</Tag>
                  {condition.name}
                </Title>
                <Paragraph>{condition.description}</Paragraph>
                <Text strong>示例配置：</Text>
                <pre style={{ 
                  background: '#f5f5f5', 
                  padding: 12, 
                  borderRadius: 4,
                  marginTop: 8,
                  fontSize: 12
                }}>
                  {JSON.stringify(condition.example, null, 2)}
                </pre>
              </Card>
            ))}
          </Space>
        </Panel>

        <Panel header="⚖️ 比较操作符" key="3">
          <Space wrap>
            {operators.map(op => (
              <Card key={op.op} size="small" style={{ width: 200, marginBottom: 8 }}>
                <Tag color="geekblue">{op.op}</Tag>
                <Text strong>{op.name}</Text>
                <br />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {op.description}
                </Text>
              </Card>
            ))}
          </Space>
        </Panel>

        <Panel header="⚡ 动作类型配置" key="4">
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            {actionTypes.map(action => (
              <Card key={action.type} size="small">
                <Title level={5}>
                  <Space>
                    {action.icon}
                    <Tag color="green">{action.type}</Tag>
                    {action.name}
                  </Space>
                </Title>
                <Paragraph>{action.description}</Paragraph>
                
                <Divider size="small" />
                
                <Text strong>配置参数：</Text>
                <ul style={{ marginTop: 8, marginBottom: 12 }}>
                  {Object.entries(action.config).map(([key, value]) => (
                    <li key={key}>
                      <Text code>{key}</Text>: {value}
                    </li>
                  ))}
                </ul>
                
                <Text strong>示例配置：</Text>
                <pre style={{ 
                  background: '#f5f5f5', 
                  padding: 12, 
                  borderRadius: 4,
                  marginTop: 8,
                  fontSize: 12
                }}>
                  {JSON.stringify(action.example, null, 2)}
                </pre>
              </Card>
            ))}
          </Space>
        </Panel>

        <Panel header="💡 配置技巧" key="5">
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Alert
              message="优先级设置"
              description="数值越大优先级越高。建议：紧急告警 100+，重要处理 50-99，一般处理 1-49"
              type="info"
            />
            <Alert
              message="变量使用"
              description="在告警消息中可以使用 {{.DeviceID}}, {{.Key}}, {{.Value}} 等变量"
              type="info"
            />
            <Alert
              message="表达式引擎"
              description="支持数学函数：abs(), max(), min(), sqrt()；字符串函数：len(), contains(), startsWith()；时间函数：now(), timeFormat()"
              type="info"
            />
            <Alert
              message="性能优化"
              description="正则表达式自动缓存，字符串操作已优化，避免过于复杂的表达式以保证性能"
              type="warning"
            />
            <Alert
              message="规则事件"
              description="规则执行会自动发布到 iot.rules.* 主题，包含评估结果、执行时间等信息"
              type="success"
            />
            <Alert
              message="测试建议"
              description="创建规则后建议先禁用状态下测试，确认无误后再启用"
              type="success"
            />
          </Space>
        </Panel>

        <Panel header="📝 完整示例" key="6">
          <Card>
            <Title level={5}>增强规则示例：智能温度监控</Title>
            <pre style={{ 
              background: '#f5f5f5', 
              padding: 16, 
              borderRadius: 4,
              fontSize: 12,
              lineHeight: 1.5
            }}>
{`{
  "name": "智能温度监控与转换",
  "description": "监控温度，使用表达式条件判断，转换单位并发布",
  "priority": 100,
  "enabled": true,
  "conditions": {
    "type": "expression",
    "expression": "key == \\"temperature\\" && abs(value - 25) > 10 && contains(device_id, \\"sensor\\")"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "设备{{.DeviceID}}温度异常: {{.Value}}°C",
        "channels": [
          { "type": "console", "enabled": true },
          { 
            "type": "nats", 
            "enabled": true, 
            "config": { "subject": "iot.alerts.temperature" }
          }
        ]
      }
    },
    {
      "type": "transform",
      "config": {
        "type": "expression",
        "parameters": {
          "expression": "(value - 32) * 5 / 9"
        },
        "output_key": "celsius_temp",
        "output_type": "float",
        "precision": 2,
        "publish_subject": "iot.data.converted"
      }
    },
    {
      "type": "forward",
      "config": {
        "subject": "iot.data.processed.{{.DeviceID}}",
        "include_metadata": true,
        "transform_data": {
          "add_timestamp": true,
          "add_rule_info": true
        }
      }
    }
  ]
}`}
            </pre>
          </Card>
        </Panel>
      </Collapse>
    </Drawer>
  );
};

export default RuleHelp;