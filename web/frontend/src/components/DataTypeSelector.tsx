import React from 'react';
import {
  Card,
  Row,
  Col,
  Tabs,
  Avatar,
  Typography,
  Tag,
  Space
} from 'antd';
import {
  NumberOutlined,
  CheckCircleOutlined,
  EnvironmentOutlined,
  BorderOutlined,
  BgColorsOutlined,
  OrderedListOutlined,
  LineChartOutlined,
  TableOutlined,
  ClockCircleOutlined
} from '@ant-design/icons';

const { Text, Title } = Typography;
const { TabPane } = Tabs;

export interface DataTypeOption {
  type: 'simple' | 'complex';
  category: string;
  key: string;
  name: string;
  icon: React.ReactNode;
  description: string;
  examples: string[];
  color: string;
}

const dataTypeOptions: DataTypeOption[] = [
  // 单点数据类型
  {
    type: 'simple',
    category: 'basic',
    key: 'numeric',
    name: '数值数据',
    icon: <NumberOutlined />,
    description: '温度、湿度、压力等单一数值',
    examples: ['temperature: 25.5', 'humidity: 60', 'pressure: 1013'],
    color: '#1890ff'
  },
  {
    type: 'simple',
    category: 'basic',
    key: 'status',
    name: '状态数据',
    icon: <CheckCircleOutlined />,
    description: '开关状态、设备状态等布尔或枚举值',
    examples: ['status: "online"', 'enabled: true', 'level: "warning"'],
    color: '#52c41a'
  },

  // 复合数据类型
  {
    type: 'complex',
    category: 'geospatial',
    key: 'location',
    name: 'GPS位置',
    icon: <EnvironmentOutlined />,
    description: '经纬度、海拔、速度等地理位置信息',
    examples: [
      'location: {lat: 39.9042, lng: 116.4074}',
      'gps: {lat: 39.9042, lng: 116.4074, alt: 50, speed: 60}'
    ],
    color: '#fa8c16'
  },
  {
    type: 'complex',
    category: 'vector',
    key: 'vector3d',
    name: '三维向量',
    icon: <BorderOutlined />,
    description: '加速度、磁场、陀螺仪等三维向量数据',
    examples: [
      'acceleration: {x: 1.2, y: -0.8, z: 9.8}',
      'magnetic: {x: 25, y: -30, z: 45}'
    ],
    color: '#722ed1'
  },
  {
    type: 'complex',
    category: 'visual',
    key: 'color',
    name: '颜色数据',
    icon: <BgColorsOutlined />,
    description: 'RGB、HSL颜色值和颜色空间数据',
    examples: [
      'color: {r: 255, g: 128, b: 0}',
      'hsl: {h: 30, s: 100, l: 50}'
    ],
    color: '#eb2f96'
  },
  {
    type: 'complex',
    category: 'array',
    key: 'vector_array',
    name: '向量数组',
    icon: <OrderedListOutlined />,
    description: '传感器阵列、频谱数据等数组类型',
    examples: [
      'spectrum: [1.2, 2.5, 1.8, 3.1, 2.7]',
      'sensors: [25.1, 25.3, 24.9, 25.2]'
    ],
    color: '#13c2c2'
  },
  {
    type: 'complex',
    category: 'matrix',
    key: 'matrix',
    name: '矩阵数据',
    icon: <TableOutlined />,
    description: '二维矩阵数据、图像数据等',
    examples: [
      'matrix: [[1,2,3], [4,5,6], [7,8,9]]',
      'image: {rows: 100, cols: 100, data: [...]}'
    ],
    color: '#2f54eb'
  },
  {
    type: 'complex',
    category: 'timeseries',
    key: 'timeseries',
    name: '时间序列',
    icon: <LineChartOutlined />,
    description: '历史趋势、采样序列等时间序列数据',
    examples: [
      'trend: {timestamps: [...], values: [...]}',
      'history: {interval: "1m", data: [...]}'
    ],
    color: '#f5222d'
  }
];

export interface DataTypeSelectorProps {
  visible: boolean;
  onTypeSelect: (dataType: DataTypeOption) => void;
  onCancel: () => void;
}

const DataTypeSelector: React.FC<DataTypeSelectorProps> = ({
  visible,
  onTypeSelect,
  onCancel
}) => {
  const renderDataTypeCard = (option: DataTypeOption) => (
    <Col span={option.type === 'simple' ? 12 : 8} key={option.key}>
      <Card
        hoverable
        onClick={() => onTypeSelect(option)}
        className="data-type-card"
        style={{ 
          height: '100%',
          border: `2px solid ${option.color}20`,
          borderRadius: '8px'
        }}
        bodyStyle={{ padding: '16px' }}
      >
        <Card.Meta
          avatar={
            <Avatar 
              size={48} 
              icon={option.icon} 
              style={{ backgroundColor: option.color }}
            />
          }
          title={
            <Space>
              {option.name}
              <Tag color={option.color} size="small">
                {option.type === 'simple' ? '简单' : '复合'}
              </Tag>
            </Space>
          }
          description={
            <div>
              <Text style={{ fontSize: '13px', lineHeight: '1.4' }}>
                {option.description}
              </Text>
              <div style={{ marginTop: '8px' }}>
                {option.examples.map((example, i) => (
                  <Tag 
                    key={i} 
                    style={{ 
                      fontSize: '10px', 
                      margin: '2px',
                      backgroundColor: `${option.color}15`,
                      color: option.color,
                      border: `1px solid ${option.color}30`
                    }}
                  >
                    {example}
                  </Tag>
                ))}
              </div>
            </div>
          }
        />
      </Card>
    </Col>
  );

  if (!visible) return null;

  return (
    <div 
      className="data-type-selector-overlay"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.5)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center'
      }}
      onClick={onCancel}
    >
      <Card
        style={{ 
          width: '90%', 
          maxWidth: '1200px',
          maxHeight: '90vh',
          overflow: 'auto'
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ marginBottom: '24px', textAlign: 'center' }}>
          <Title level={3} style={{ margin: 0 }}>
            <Space>
              <ClockCircleOutlined />
              选择数据类型
            </Space>
          </Title>
          <Text type="secondary">
            选择适合您数据结构的规则编辑器类型
          </Text>
        </div>

        <Tabs defaultActiveKey="simple" size="large">
          <TabPane 
            tab={
              <Space>
                <NumberOutlined />
                单点数据规则
                <Tag color="blue" size="small">简单高效</Tag>
              </Space>
            } 
            key="simple"
          >
            <div style={{ marginBottom: '16px' }}>
              <Text type="secondary">
                适用于处理单一数值、状态等简单数据类型的规则
              </Text>
            </div>
            <Row gutter={[16, 16]}>
              {dataTypeOptions
                .filter(opt => opt.type === 'simple')
                .map(renderDataTypeCard)
              }
            </Row>
          </TabPane>

          <TabPane 
            tab={
              <Space>
                <BorderOutlined />
                复合数据规则
                <Tag color="purple" size="small">功能强大</Tag>
              </Space>
            } 
            key="complex"
          >
            <div style={{ marginBottom: '16px' }}>
              <Text type="secondary">
                适用于处理GPS位置、向量、矩阵等复杂数据结构的高级规则
              </Text>
            </div>
            <Row gutter={[16, 16]}>
              {dataTypeOptions
                .filter(opt => opt.type === 'complex')
                .map(renderDataTypeCard)
              }
            </Row>
          </TabPane>
        </Tabs>

        <div style={{ 
          marginTop: '24px', 
          textAlign: 'center',
          padding: '16px',
          backgroundColor: '#f0f2f5',
          borderRadius: '6px'
        }}>
          <Text type="secondary" style={{ fontSize: '12px' }}>
            💡 提示：选择数据类型后将进入对应的专用规则编辑器，提供最佳的编辑体验
          </Text>
        </div>
      </Card>
    </div>
  );
};

export default DataTypeSelector;
export { dataTypeOptions };