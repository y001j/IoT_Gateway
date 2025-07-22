import React, { useState } from 'react';
import {
  Modal,
  Card,
  Row,
  Col,
  Tag,
  Typography,
  Space,
  Button,
  Input,
  Select,
  Divider,
  Alert
} from 'antd';
import {
  FireOutlined,
  SwapOutlined,
  FilterOutlined,
  FunctionOutlined,
  ForwardOutlined,
  ThunderboltOutlined,
  SafetyOutlined,
  MonitorOutlined,
  AlertOutlined,
  SettingOutlined
} from '@ant-design/icons';
import type { Rule, Condition, Action } from '../types/rule';

const { Title, Text, Paragraph } = Typography;
const { Search } = Input;
const { Option } = Select;

interface RuleTemplate {
  id: string;
  name: string;
  description: string;
  category: string;
  icon: React.ReactNode;
  difficulty: 'beginner' | 'intermediate' | 'advanced';
  tags: string[];
  rule: Partial<Rule>;
}

interface RuleTemplatesProps {
  visible: boolean;
  onClose: () => void;
  onSelect: (template: RuleTemplate) => void;
}

const RuleTemplates: React.FC<RuleTemplatesProps> = ({ visible, onClose, onSelect }) => {
  const [searchText, setSearchText] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [selectedDifficulty, setSelectedDifficulty] = useState<string>('all');

  // 规则模板定义
  const templates: RuleTemplate[] = [
    {
      id: 'temperature_alert',
      name: '温度告警监控',
      description: '监控设备温度，超过阈值时发送告警通知',
      category: 'monitoring',
      icon: <ThunderboltOutlined style={{ color: '#ff4d4f' }} />,
      difficulty: 'beginner',
      tags: ['温度', '告警', '监控'],
      rule: {
        name: '温度告警监控',
        description: '当设备温度超过指定阈值时发送告警',
        priority: 100,
        enabled: true,
        conditions: {
          type: 'and',
          and: [
            {
              type: 'simple',
              field: 'key',
              operator: 'eq',
              value: 'temperature'
            },
            {
              type: 'simple',
              field: 'value',
              operator: 'gt',
              value: 35
            }
          ]
        } as Condition,
        actions: [
          {
            type: 'alert',
            config: {
              level: 'warning',
              message: '设备{{.DeviceID}}温度过高: {{.Value}}°C',
              channels: ['console', 'webhook']
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'humidity_control',
      name: '湿度控制规则',
      description: '根据湿度数据自动调节设备状态',
      category: 'automation',
      icon: <SettingOutlined style={{ color: '#1890ff' }} />,
      difficulty: 'intermediate',
      tags: ['湿度', '自动化', '控制'],
      rule: {
        name: '湿度自动控制',
        description: '根据湿度变化自动调节设备工作状态',
        priority: 80,
        enabled: true,
        conditions: {
          type: 'or',
          or: [
            {
              type: 'simple',
              field: 'value',
              operator: 'gt',
              value: 80
            },
            {
              type: 'simple',
              field: 'value',
              operator: 'lt',
              value: 30
            }
          ]
        } as Condition,
        actions: [
          {
            type: 'transform',
            config: {
              add_tags: {
                control_action: 'humidity_adjust',
                timestamp: '{{.Timestamp}}'
              }
            }
          },
          {
            type: 'forward',
            config: {
              target_type: 'http',
              url: 'http://localhost:8080/api/device/control',
              method: 'POST',
              headers: {
                'Content-Type': 'application/json'
              }
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'data_aggregation',
      name: '数据聚合统计',
      description: '对传感器数据进行实时聚合计算',
      category: 'analytics',
      icon: <FunctionOutlined style={{ color: '#52c41a' }} />,
      difficulty: 'intermediate',
      tags: ['聚合', '统计', '分析'],
      rule: {
        name: '传感器数据聚合',
        description: '每10个数据点计算平均值、最大值和最小值',
        priority: 50,
        enabled: true,
        conditions: {
          type: 'simple',
          field: 'key',
          operator: 'in',
          value: ['temperature', 'humidity', 'pressure']
        } as Condition,
        actions: [
          {
            type: 'aggregate',
            config: {
              window_type: 'count',
              size: 10,
              functions: ['avg', 'max', 'min'],
              group_by: ['device_id', 'key'],
              output_key: '{{.Key}}_stats',
              forward: true
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'error_detection',
      name: '设备故障检测',
      description: '检测设备异常状态并记录故障信息',
      category: 'monitoring',
      icon: <AlertOutlined style={{ color: '#fa8c16' }} />,
      difficulty: 'advanced',
      tags: ['故障', '检测', '异常'],
      rule: {
        name: '设备故障检测',
        description: '检测设备状态异常并触发故障处理流程',
        priority: 150,
        enabled: true,
        conditions: {
          type: 'or',
          or: [
            {
              type: 'simple',
              field: 'status',
              operator: 'eq',
              value: 'error'
            },
            {
              type: 'simple',
              field: 'quality',
              operator: 'lt',
              value: 50
            },
            {
              type: 'expression',
              expression: 'temperature > 60 || humidity > 95'
            }
          ]
        } as Condition,
        actions: [
          {
            type: 'alert',
            config: {
              level: 'critical',
              message: '设备{{.DeviceID}}发生故障: {{.Status}}',
              channels: ['console', 'webhook', 'email']
            }
          },
          {
            type: 'transform',
            config: {
              add_tags: {
                alert_type: 'device_fault',
                severity: 'critical',
                processed_at: '{{.Timestamp}}'
              }
            }
          },
          {
            type: 'forward',
            config: {
              target_type: 'file',
              path: '/var/log/device_faults.log'
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'data_filtering',
      name: '数据质量过滤',
      description: '过滤低质量数据，只保留有效数据',
      category: 'processing',
      icon: <FilterOutlined style={{ color: '#722ed1' }} />,
      difficulty: 'beginner',
      tags: ['过滤', '质量', '清洗'],
      rule: {
        name: '数据质量过滤',
        description: '过滤质量低于阈值的数据点',
        priority: 200,
        enabled: true,
        conditions: {
          type: 'simple',
          field: 'quality',
          operator: 'exists',
          value: null
        } as Condition,
        actions: [
          {
            type: 'filter',
            config: {
              type: 'range',
              min: 70,
              max: 100,
              drop_on_match: false
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'data_conversion',
      name: '单位转换规则',
      description: '自动转换数据单位（如摄氏度转华氏度）',
      category: 'processing',
      icon: <SwapOutlined style={{ color: '#1890ff' }} />,
      difficulty: 'intermediate',
      tags: ['转换', '单位', '格式'],
      rule: {
        name: '温度单位转换',
        description: '将摄氏度转换为华氏度',
        priority: 60,
        enabled: true,
        conditions: {
          type: 'and',
          and: [
            {
              type: 'simple',
              field: 'key',
              operator: 'eq',
              value: 'temperature'
            },
            {
              type: 'simple',
              field: 'unit',
              operator: 'eq',
              value: 'celsius'
            }
          ]
        } as Condition,
        actions: [
          {
            type: 'transform',
            config: {
              field: 'value',
              scale_factor: 1.8,
              offset: 32,
              precision: 1,
              add_tags: {
                unit: 'fahrenheit',
                converted: 'true'
              }
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'security_monitoring',
      name: '安全监控规则',
      description: '监控异常访问和安全事件',
      category: 'security',
      icon: <SafetyOutlined style={{ color: '#f5222d' }} />,
      difficulty: 'advanced',
      tags: ['安全', '监控', '异常'],
      rule: {
        name: '安全事件监控',
        description: '检测潜在的安全威胁和异常行为',
        priority: 180,
        enabled: true,
        conditions: {
          type: 'or',
          or: [
            {
              type: 'simple',
              field: 'event_type',
              operator: 'eq',
              value: 'unauthorized_access'
            },
            {
              type: 'simple',
              field: 'failed_attempts',
              operator: 'gt',
              value: 5
            }
          ]
        } as Condition,
        actions: [
          {
            type: 'alert',
            config: {
              level: 'critical',
              message: '检测到安全威胁: {{.EventType}}',
              channels: ['console', 'webhook', 'email', 'sms']
            }
          },
          {
            type: 'forward',
            config: {
              target_type: 'http',
              url: 'https://security.example.com/api/incidents',
              method: 'POST'
            }
          }
        ] as Action[]
      }
    },
    {
      id: 'performance_monitoring',
      name: '性能监控规则',
      description: '监控系统性能指标和资源使用情况',
      category: 'monitoring',
      icon: <MonitorOutlined style={{ color: '#13c2c2' }} />,
      difficulty: 'intermediate',
      tags: ['性能', '监控', '资源'],
      rule: {
        name: '系统性能监控',
        description: '监控CPU、内存使用率异常',
        priority: 90,
        enabled: true,
        conditions: {
          type: 'and',
          and: [
            {
              type: 'simple',
              field: 'key',
              operator: 'in',
              value: ['cpu_usage', 'memory_usage']
            },
            {
              type: 'simple',
              field: 'value',
              operator: 'gt',
              value: 85
            }
          ]
        } as Condition,
        actions: [
          {
            type: 'alert',
            config: {
              level: 'warning',
              message: '系统{{.Key}}过高: {{.Value}}%',
              channels: ['console', 'webhook'],
              throttle: '5m'
            }
          },
          {
            type: 'aggregate',
            config: {
              window_type: 'time',
              size: '1m',
              functions: ['avg', 'max'],
              group_by: ['device_id'],
              output_key: 'performance_stats'
            }
          }
        ] as Action[]
      }
    }
  ];

  // 分类定义
  const categories = [
    { value: 'all', label: '全部分类' },
    { value: 'monitoring', label: '监控告警' },
    { value: 'automation', label: '自动化控制' },
    { value: 'analytics', label: '数据分析' },
    { value: 'processing', label: '数据处理' },
    { value: 'security', label: '安全监控' }
  ];

  // 难度定义
  const difficulties = [
    { value: 'all', label: '全部难度' },
    { value: 'beginner', label: '初级', color: 'green' },
    { value: 'intermediate', label: '中级', color: 'orange' },
    { value: 'advanced', label: '高级', color: 'red' }
  ];

  // 过滤模板
  const filteredTemplates = templates.filter(template => {
    const matchesSearch = searchText === '' || 
      template.name.toLowerCase().includes(searchText.toLowerCase()) ||
      template.description.toLowerCase().includes(searchText.toLowerCase()) ||
      template.tags.some(tag => tag.toLowerCase().includes(searchText.toLowerCase()));
    
    const matchesCategory = selectedCategory === 'all' || template.category === selectedCategory;
    const matchesDifficulty = selectedDifficulty === 'all' || template.difficulty === selectedDifficulty;
    
    return matchesSearch && matchesCategory && matchesDifficulty;
  });

  // 处理模板选择
  const handleSelectTemplate = (template: RuleTemplate) => {
    onSelect(template);
    onClose();
  };

  // 获取难度标签颜色
  const getDifficultyColor = (difficulty: string) => {
    const difficultyConfig = difficulties.find(d => d.value === difficulty);
    return difficultyConfig?.color || 'default';
  };

  return (
    <Modal
      title="选择规则模板"
      open={visible}
      onCancel={onClose}
      footer={null}
      width={1000}
      bodyStyle={{ maxHeight: '70vh', overflowY: 'auto' }}
    >
      <Alert
        message="规则模板库"
        description="选择预定义的规则模板快速开始，您可以在此基础上进行修改和定制。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      {/* 筛选条件 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Row gutter={16} align="middle">
          <Col span={8}>
            <Search
              placeholder="搜索模板名称、描述或标签"
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              allowClear
            />
          </Col>
          <Col span={6}>
            <Select
              placeholder="选择分类"
              value={selectedCategory}
              onChange={setSelectedCategory}
              style={{ width: '100%' }}
            >
              {categories.map(cat => (
                <Option key={cat.value} value={cat.value}>{cat.label}</Option>
              ))}
            </Select>
          </Col>
          <Col span={6}>
            <Select
              placeholder="选择难度"
              value={selectedDifficulty}
              onChange={setSelectedDifficulty}
              style={{ width: '100%' }}
            >
              {difficulties.map(diff => (
                <Option key={diff.value} value={diff.value}>{diff.label}</Option>
              ))}
            </Select>
          </Col>
          <Col span={4}>
            <Text type="secondary">
              共 {filteredTemplates.length} 个模板
            </Text>
          </Col>
        </Row>
      </Card>

      {/* 模板列表 */}
      <Row gutter={[16, 16]}>
        {filteredTemplates.map(template => (
          <Col key={template.id} span={12}>
            <Card
              hoverable
              size="small"
              title={
                <Space>
                  {template.icon}
                  <span>{template.name}</span>
                  <Tag color={getDifficultyColor(template.difficulty)}>
                    {difficulties.find(d => d.value === template.difficulty)?.label}
                  </Tag>
                </Space>
              }
              extra={
                <Button
                  type="primary"
                  size="small"
                  onClick={() => handleSelectTemplate(template)}
                >
                  使用模板
                </Button>
              }
              style={{ height: 200 }}
            >
              <Paragraph
                ellipsis={{ rows: 2, expandable: false }}
                style={{ marginBottom: 12, minHeight: 40 }}
              >
                {template.description}
              </Paragraph>
              
              <Space wrap style={{ marginBottom: 8 }}>
                {template.tags.map(tag => (
                  <Tag key={tag} size="small">{tag}</Tag>
                ))}
              </Space>
              
              <Divider style={{ margin: '8px 0' }} />
              
              <Row justify="space-between" align="middle">
                <Col>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {categories.find(c => c.value === template.category)?.label}
                  </Text>
                </Col>
                <Col>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    优先级: {template.rule.priority}
                  </Text>
                </Col>
              </Row>
            </Card>
          </Col>
        ))}
      </Row>

      {filteredTemplates.length === 0 && (
        <div style={{ textAlign: 'center', padding: '40px 0' }}>
          <Text type="secondary">没有找到匹配的模板</Text>
        </div>
      )}
    </Modal>
  );
};

export default RuleTemplates;