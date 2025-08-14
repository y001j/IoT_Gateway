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
        window_size: '窗口大小',
        functions: '聚合函数，支持28个函数：基础统计(count,sum,avg,min,max)、百分位数(p25,p50,p75,p90,p95,p99)、数据质量(null_rate,completeness)、变化检测(change,change_rate)、阈值监控(above_count,below_count)等',
        group_by: '分组字段',
        output_key: '输出字段名',
        upper_limit: '上限阈值（用于阈值监控函数）',
        lower_limit: '下限阈值（用于阈值监控函数）',
        outlier_threshold: '异常值阈值（用于outlier_count函数）'
      },
      example: {
        type: 'aggregate',
        config: {
          window_type: 'count',
          window_size: 10,
          functions: ['avg', 'max', 'min', 'p90', 'null_rate'],
          group_by: ['device_id'],
          output_key: 'aggregated_value',
          upper_limit: 100,
          lower_limit: 0,
          forward: true
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
    // 基础数据字段
    { field: 'device_id', description: '设备ID标识符' },
    { field: 'key', description: '数据点键名，如 temperature, humidity' },
    { field: 'value', description: '数据点数值' },
    { field: 'timestamp', description: '数据时间戳' },
    { field: 'tags', description: '设备标签信息' },
    { field: 'quality', description: '数据质量标识' },
    { field: 'unit', description: '数据单位' },
    
    // GPS/地理位置数据字段
    { field: 'latitude', description: '纬度 (-90 ~ 90)' },
    { field: 'longitude', description: '经度 (-180 ~ 180)' },
    { field: 'altitude', description: '海拔高度 (米)' },
    { field: 'accuracy', description: 'GPS定位精度 (米)' },
    { field: 'speed', description: '移动速度 (km/h)' },
    { field: 'heading', description: '方向角 (度)' },
    { field: 'elevation_category', description: '海拔等级分类' },
    { field: 'speed_category', description: '速度等级分类' },
    
    // 三轴向量数据字段
    { field: 'x', description: 'X轴数值' },
    { field: 'y', description: 'Y轴数值' },
    { field: 'z', description: 'Z轴数值' },
    { field: 'magnitude', description: '向量模长/幅度' },
    { field: 'x_ratio', description: 'X轴比例分量' },
    { field: 'y_ratio', description: 'Y轴比例分量' },
    { field: 'z_ratio', description: 'Z轴比例分量' },
    { field: 'dominant_axis', description: '主导轴 (x/y/z)' },
    
    // 颜色数据字段
    { field: 'r', description: '红色分量 (0-255)' },
    { field: 'g', description: '绿色分量 (0-255)' },
    { field: 'b', description: '蓝色分量 (0-255)' },
    { field: 'a', description: '透明度 (0-255)' },
    { field: 'hue', description: '色相 (0-360度)' },
    { field: 'saturation', description: '饱和度 (0-1)' },
    { field: 'lightness', description: '亮度 (0-1)' },
    
    // 通用向量/数组/矩阵字段
    { field: 'dimension', description: '向量维度' },
    { field: 'size', description: '数组大小' },
    { field: 'length', description: '数据长度' },
    { field: 'rows', description: '矩阵行数' },
    { field: 'cols', description: '矩阵列数' },
    { field: 'norm', description: '向量范数/模长' },
    { field: 'dominant_dimension', description: '主导维度索引' },
    { field: 'data_type', description: '元素数据类型' },
    { field: 'numeric_count', description: '数值元素数量' },
    { field: 'null_count', description: '空值数量' },
    
    // 时间序列数据字段
    { field: 'duration', description: '时间序列总时长' },
    { field: 'avg_interval', description: '平均采样间隔' },
    { field: 'trend', description: '趋势方向 (increasing/decreasing/stable)' },
    { field: 'trend_slope', description: '趋势斜率' }
  ];

  return (
    <Drawer
      title="规则配置帮助"
      width={800}
      open={visible}
      onClose={onClose}
      styles={{ body: { padding: 24 } }}
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

        <Panel header="🔗 复合数据格式支持" key="4">
          <Alert
            message="IoT Gateway 复合数据格式全面支持"
            description="系统现已支持7种复合数据格式，自动解析并提取衍生字段，可直接在规则条件中使用"
            type="success"
            style={{ marginBottom: 16 }}
          />
          
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Card size="small">
              <Title level={5}>📍 GPS/地理位置数据 (location)</Title>
              <Paragraph type="secondary">
                包含纬度、经度、海拔、精度、速度、方向角等字段，自动计算海拔等级和速度等级
              </Paragraph>
              <Space wrap>
                <Tag color="blue">latitude</Tag>
                <Tag color="blue">longitude</Tag>
                <Tag color="blue">altitude</Tag>
                <Tag color="blue">accuracy</Tag>
                <Tag color="blue">speed</Tag>
                <Tag color="blue">heading</Tag>
                <Tag color="cyan">elevation_category</Tag>
                <Tag color="cyan">speed_category</Tag>
              </Space>
              <pre style={{ 
                background: '#f5f5f5', 
                padding: 8, 
                borderRadius: 4,
                fontSize: 11,
                marginTop: 8
              }}>
{`示例规则: latitude > 39.9 && longitude > 116.3 && speed > 60`}
              </pre>
            </Card>

            <Card size="small">
              <Title level={5}>📐 三轴向量数据 (vector3d)</Title>
              <Paragraph type="secondary">
                适用于加速度计、陀螺仪、磁力计等传感器，自动计算模长和主导轴
              </Paragraph>
              <Space wrap>
                <Tag color="purple">x</Tag>
                <Tag color="purple">y</Tag>
                <Tag color="purple">z</Tag>
                <Tag color="orange">magnitude</Tag>
                <Tag color="orange">dominant_axis</Tag>
                <Tag color="geekblue">x_ratio</Tag>
                <Tag color="geekblue">y_ratio</Tag>
                <Tag color="geekblue">z_ratio</Tag>
              </Space>
              <pre style={{ 
                background: '#f5f5f5', 
                padding: 8, 
                borderRadius: 4,
                fontSize: 11,
                marginTop: 8
              }}>
{`示例规则: magnitude > 10.0 && dominant_axis == "z"`}
              </pre>
            </Card>

            <Card size="small">
              <Title level={5}>🎨 颜色数据 (color)</Title>
              <Paragraph type="secondary">
                RGB颜色数据，自动计算HSL色彩空间的色相、饱和度、亮度
              </Paragraph>
              <Space wrap>
                <Tag color="red">r</Tag>
                <Tag color="green">g</Tag>
                <Tag color="blue">b</Tag>
                <Tag color="gray">a</Tag>
                <Tag color="magenta">hue</Tag>
                <Tag color="orange">saturation</Tag>
                <Tag color="gold">lightness</Tag>
              </Space>
              <pre style={{ 
                background: '#f5f5f5', 
                padding: 8, 
                borderRadius: 4,
                fontSize: 11,
                marginTop: 8
              }}>
{`示例规则: hue >= 120 && hue <= 240 && saturation > 0.5`}
              </pre>
            </Card>

            <Card size="small">
              <Title level={5}>🔢 向量/数组/矩阵数据</Title>
              <Paragraph type="secondary">
                支持通用向量、数组、矩阵和时间序列数据，自动计算统计指标和结构特征
              </Paragraph>
              <Space wrap>
                <Tag color="volcano">dimension</Tag>
                <Tag color="volcano">size</Tag>
                <Tag color="volcano">length</Tag>
                <Tag color="lime">norm</Tag>
                <Tag color="lime">dominant_dimension</Tag>
                <Tag color="cyan">numeric_count</Tag>
                <Tag color="cyan">null_count</Tag>
                <Tag color="purple">trend</Tag>
                <Tag color="purple">trend_slope</Tag>
              </Space>
              <pre style={{ 
                background: '#f5f5f5', 
                padding: 8, 
                borderRadius: 4,
                fontSize: 11,
                marginTop: 8
              }}>
{`示例规则: dimension > 3 && norm > 1.0 && trend == "increasing"`}
              </pre>
            </Card>

            <Alert
              message="使用提示"
              description={
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  <li>复合数据字段可直接在条件表达式中使用，无需特殊语法</li>
                  <li>系统自动解析复合数据并提取所有可用的衍生字段</li>
                  <li>支持与传统数据字段混合使用，如 temperature &gt; 30 &amp;&amp; magnitude &gt; 10</li>
                  <li>聚合函数同样支持复合数据字段的统计分析</li>
                </ul>
              }
              type="info"
              style={{ marginTop: 16 }}
            />
          </Space>
        </Panel>

        <Panel header="📊 聚合函数详解" key="5">
          <Alert
            message="28个聚合函数完整支持"
            description="规则引擎现已支持28个聚合函数，涵盖基础统计、百分位数、数据质量、变化检测和阈值监控等各个方面"
            type="success"
            style={{ marginBottom: 16 }}
          />
          
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Card size="small">
              <Title level={5}>📊 基础统计函数 (10个)</Title>
              <Space wrap>
                <Tag color="blue">count</Tag>
                <Tag color="blue">sum</Tag>
                <Tag color="blue">avg/mean/average</Tag>
                <Tag color="blue">min</Tag>
                <Tag color="blue">max</Tag>
                <Tag color="blue">median</Tag>
                <Tag color="blue">first</Tag>
                <Tag color="blue">last</Tag>
                <Tag color="blue">stddev/std</Tag>
                <Tag color="blue">variance</Tag>
              </Space>
              <Paragraph type="secondary" style={{ marginTop: 8 }}>
                日常统计分析的核心函数，包括计数、求和、平均值、极值、中位数和离散度指标
              </Paragraph>
            </Card>

            <Card size="small">
              <Title level={5}>📈 分布统计函数 (2个)</Title>
              <Space wrap>
                <Tag color="geekblue">volatility</Tag>
                <Tag color="geekblue">cv</Tag>
              </Space>
              <Paragraph type="secondary" style={{ marginTop: 8 }}>
                高级统计指标：volatility（波动率）= 变异系数×100，cv（变异系数）= 标准差/均值
              </Paragraph>
            </Card>

            <Card size="small">
              <Title level={5}>📊 百分位数函数 (6个)</Title>
              <Space wrap>
                <Tag color="cyan">p25</Tag>
                <Tag color="cyan">p50</Tag>
                <Tag color="cyan">p75</Tag>
                <Tag color="cyan">p90</Tag>
                <Tag color="cyan">p95</Tag>
                <Tag color="cyan">p99</Tag>
              </Space>
              <Paragraph type="secondary" style={{ marginTop: 8 }}>
                性能监控关键指标，用于延迟、响应时间等分布分析。p90/p95/p99常用于SLA监控
              </Paragraph>
            </Card>

            <Card size="small">
              <Title level={5}>🔍 数据质量函数 (3个)</Title>
              <Space wrap>
                <Tag color="orange">null_rate</Tag>
                <Tag color="orange">completeness</Tag>
                <Tag color="orange">outlier_count</Tag>
              </Space>
              <Paragraph type="secondary" style={{ marginTop: 8 }}>
                数据健康度指标：null_rate（空值比例）、completeness（完整性=1-null_rate）、outlier_count（异常值数量，需配置outlier_threshold）
              </Paragraph>
            </Card>

            <Card size="small">
              <Title level={5}>📉 变化检测函数 (2个)</Title>
              <Space wrap>
                <Tag color="purple">change</Tag>
                <Tag color="purple">change_rate</Tag>
              </Space>
              <Paragraph type="secondary" style={{ marginTop: 8 }}>
                趋势分析：change（绝对变化量）= 最新值-第一个值，change_rate（变化率）= change/第一个值×100%
              </Paragraph>
            </Card>

            <Card size="small">
              <Title level={5}>⚡ 阈值监控函数 (3个)</Title>
              <Space wrap>
                <Tag color="red">above_count</Tag>
                <Tag color="red">below_count</Tag>
                <Tag color="red">in_range_count</Tag>
              </Space>
              <Paragraph type="secondary" style={{ marginTop: 8 }}>
                阈值监控：需配置upper_limit和/或lower_limit参数。用于统计超标数据点数量
              </Paragraph>
            </Card>

            <Alert
              message="使用提示"
              description={
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  <li>支持多选：可同时选择多个函数进行并行计算</li>
                  <li>搜索功能：在选择器中输入关键词快速查找函数</li>
                  <li>参数配置：阈值监控函数需要配置相应的阈值参数</li>
                  <li>性能优化：内置增量统计算法，支持滑动窗口和累积模式</li>
                </ul>
              }
              type="info"
              style={{ marginTop: 16 }}
            />
          </Space>
        </Panel>

        <Panel header="⚡ 动作类型配置" key="6">
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

        <Panel header="💡 配置技巧" key="7">
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

        <Panel header="📝 完整示例" key="8">
          <Card style={{ marginBottom: 16 }}>
            <Title level={5}>复合数据示例：GPS位置监控</Title>
            <pre style={{ 
              background: '#f5f5f5', 
              padding: 16, 
              borderRadius: 4,
              fontSize: 12,
              lineHeight: 1.5
            }}>
{`{
  "name": "车辆位置和速度监控",
  "description": "监控车辆GPS位置、速度和区域限制",
  "priority": 90,
  "enabled": true,
  "conditions": {
    "type": "expression",
    "expression": "(latitude > 39.9 && longitude > 116.3) && (speed > 80 || altitude < 0)"
  },
  "actions": [
    {
      "type": "alert",
      "config": {
        "level": "warning",
        "message": "车辆{{.DeviceID}}位置异常: 纬度{{.latitude}}, 经度{{.longitude}}, 速度{{.speed}}km/h",
        "channels": ["console", "webhook"],
        "throttle": "2m"
      }
    },
    {
      "type": "aggregate",
      "config": {
        "window_size": 20,
        "functions": ["avg", "max", "p90"],
        "group_by": ["device_id"],
        "output_key": "location_stats"
      }
    }
  ]
}`}
            </pre>
          </Card>

          <Card style={{ marginBottom: 16 }}>
            <Title level={5}>聚合函数示例：传感器性能监控</Title>
            <pre style={{ 
              background: '#f5f5f5', 
              padding: 16, 
              borderRadius: 4,
              fontSize: 12,
              lineHeight: 1.5
            }}>
{`{
  "name": "传感器性能监控",
  "description": "监控传感器数据质量和性能指标",
  "priority": 80,
  "enabled": true,
  "conditions": {
    "type": "simple",
    "field": "key",
    "operator": "eq",
    "value": "temperature"
  },
  "actions": [
    {
      "type": "aggregate",
      "config": {
        "window_type": "count",
        "window_size": 20,
        "functions": ["avg", "min", "max", "p90", "p95", "null_rate", "change_rate", "above_count"],
        "group_by": ["device_id"],
        "output_key": "sensor_stats",
        "upper_limit": 50,
        "lower_limit": 10,
        "outlier_threshold": 2.0,
        "forward": true
      }
    }
  ]
}`}
            </pre>
          </Card>

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