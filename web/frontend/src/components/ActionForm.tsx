import React, { useState, useEffect } from 'react';
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
  Switch,
  Tooltip,
  Divider,
  Typography
} from 'antd';

const { Text } = Typography;
import {
  PlusOutlined,
  DeleteOutlined,
  FireOutlined,
  FilterOutlined,
  ForwardOutlined,
  FunctionOutlined,
  QuestionCircleOutlined,
  SwapOutlined
} from '@ant-design/icons';
import type { Action } from '../types/rule';

const { Option } = Select;
const { TextArea } = Input;

interface ActionFormProps {
  value?: Action[];
  onChange?: (value: Action[]) => void;
}

interface ActionConfig {
  type: string;
  config: Record<string, any>;
  async?: boolean;
  timeout?: string;
  retry?: number;
}

const ActionForm: React.FC<ActionFormProps> = ({ value, onChange }) => {
  const [actions, setActions] = useState<ActionConfig[]>([]);

  // 动作类型配置
  const actionTypes = [
    { 
      value: 'alert', 
      label: '告警通知', 
      icon: <FireOutlined style={{ color: '#ff4d4f' }} />,
      description: '发送告警消息'
    },
    { 
      value: 'transform', 
      label: '数据转换', 
      icon: <SwapOutlined style={{ color: '#1890ff' }} />,
      description: '转换数据格式或值'
    },
    { 
      value: 'filter', 
      label: '数据过滤', 
      icon: <FilterOutlined style={{ color: '#722ed1' }} />,
      description: '过滤或丢弃数据'
    },
    { 
      value: 'aggregate', 
      label: '数据聚合', 
      icon: <FunctionOutlined style={{ color: '#52c41a' }} />,
      description: '聚合计算统计值'
    },
    { 
      value: 'forward', 
      label: '数据转发', 
      icon: <ForwardOutlined style={{ color: '#fa8c16' }} />,
      description: '转发到外部系统'
    }
  ];

  // 初始化
  useEffect(() => {
    if (value && value.length > 0) {
      setActions(value.map(action => ({
        type: action.type,
        config: action.config || {},
        async: action.async,
        timeout: action.timeout,
        retry: action.retry
      })));
    } else {
      setActions([{ type: 'alert', config: {}, async: false, timeout: '30s', retry: 0 }]);
    }
  }, [value]);

  // 触发变更
  const triggerChange = () => {
    const newActions: Action[] = actions.map(action => ({
      type: action.type,
      config: action.config,
      async: action.async || false,
      timeout: action.timeout || '30s',
      retry: action.retry || 0
    }));
    onChange?.(newActions);
  };

  // 添加动作
  const addAction = () => {
    setActions([...actions, { type: 'alert', config: {}, async: false, timeout: '30s', retry: 0 }]);
    setTimeout(triggerChange, 0);
  };

  // 删除动作
  const removeAction = (index: number) => {
    const newActions = actions.filter((_, i) => i !== index);
    setActions(newActions);
    setTimeout(triggerChange, 0);
  };

  // 更新动作
  const updateAction = (index: number, field: keyof ActionConfig, value: any) => {
    const newActions = [...actions];
    newActions[index] = { ...newActions[index], [field]: value };
    setActions(newActions);
    setTimeout(triggerChange, 0);
  };

  // 更新动作配置
  const updateActionConfig = (index: number, key: string, value: any) => {
    const newActions = [...actions];
    newActions[index] = {
      ...newActions[index],
      config: { ...newActions[index].config, [key]: value }
    };
    setActions(newActions);
    setTimeout(triggerChange, 0);
  };

  // 渲染告警配置
  const renderAlertConfig = (action: ActionConfig, index: number) => (
    <div>
      <Row gutter={16}>
        <Col span={12}>
          <Form.Item label="告警级别">
            <Select
              value={action.config.level || 'warning'}
              onChange={(value) => updateActionConfig(index, 'level', value)}
            >
              <Option value="info">信息</Option>
              <Option value="warning">警告</Option>
              <Option value="error">错误</Option>
              <Option value="critical">严重</Option>
            </Select>
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item label="限流时间">
            <Input
              placeholder="如 5m, 1h"
              value={action.config.throttle || ''}
              onChange={(e) => updateActionConfig(index, 'throttle', e.target.value)}
            />
          </Form.Item>
        </Col>
      </Row>
      <Form.Item label="告警消息">
        <TextArea
          rows={2}
          placeholder="支持变量: {{.DeviceID}}, {{.Key}}, {{.Value}}"
          value={action.config.message || ''}
          onChange={(e) => updateActionConfig(index, 'message', e.target.value)}
        />
      </Form.Item>
      <Form.Item label="通知渠道">
        <Select
          mode="multiple"
          placeholder="选择通知渠道"
          value={action.config.channels || []}
          onChange={(value) => updateActionConfig(index, 'channels', value)}
        >
          <Option value="console">控制台</Option>
          <Option value="webhook">Webhook</Option>
          <Option value="email">邮件</Option>
          <Option value="sms">短信</Option>
        </Select>
      </Form.Item>
    </div>
  );

  // 渲染转换配置
  const renderTransformConfig = (action: ActionConfig, index: number) => (
    <div>
      <Form.Item label="目标字段">
        <Input
          placeholder="要转换的字段名"
          value={action.config.field || ''}
          onChange={(e) => updateActionConfig(index, 'field', e.target.value)}
        />
      </Form.Item>
      
      <Form.Item label="转换操作">
        <Card size="small">
          <Form.Item label="缩放因子" style={{ marginBottom: 8 }}>
            <Row gutter={8}>
              <Col span={12}>
                <InputNumber
                  placeholder="乘数"
                  value={action.config.scale_factor}
                  onChange={(value) => updateActionConfig(index, 'scale_factor', value)}
                  style={{ width: '100%' }}
                />
              </Col>
              <Col span={12}>
                <InputNumber
                  placeholder="偏移量"
                  value={action.config.offset}
                  onChange={(value) => updateActionConfig(index, 'offset', value)}
                  style={{ width: '100%' }}
                />
              </Col>
            </Row>
          </Form.Item>
          
          <Form.Item label="精度" style={{ marginBottom: 8 }}>
            <InputNumber
              placeholder="小数位数"
              value={action.config.precision}
              onChange={(value) => updateActionConfig(index, 'precision', value)}
              min={0}
              max={10}
            />
          </Form.Item>
        </Card>
      </Form.Item>

      <Form.Item label="添加标签">
        <TextArea
          rows={2}
          placeholder='JSON格式，如: {"unit": "celsius", "source": "sensor"}'
          value={action.config.add_tags ? JSON.stringify(action.config.add_tags, null, 2) : ''}
          onChange={(e) => {
            try {
              const tags = JSON.parse(e.target.value || '{}');
              updateActionConfig(index, 'add_tags', tags);
            } catch {
              // 忽略JSON解析错误
            }
          }}
        />
      </Form.Item>
    </div>
  );

  // 渲染过滤配置
  const renderFilterConfig = (action: ActionConfig, index: number) => (
    <div>
      <Form.Item label="过滤类型">
        <Select
          value={action.config.type || 'range'}
          onChange={(value) => updateActionConfig(index, 'type', value)}
        >
          <Option value="range">范围过滤</Option>
          <Option value="dedup">去重过滤</Option>
          <Option value="rate_limit">速率限制</Option>
        </Select>
      </Form.Item>

      {action.config.type === 'range' && (
        <div>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item label="最小值">
                <InputNumber
                  value={action.config.min}
                  onChange={(value) => updateActionConfig(index, 'min', value)}
                  style={{ width: '100%' }}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="最大值">
                <InputNumber
                  value={action.config.max}
                  onChange={(value) => updateActionConfig(index, 'max', value)}
                  style={{ width: '100%' }}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item label="匹配时动作">
            <Select
              value={action.config.drop_on_match ? 'drop' : 'pass'}
              onChange={(value) => updateActionConfig(index, 'drop_on_match', value === 'drop')}
            >
              <Option value="pass">通过数据</Option>
              <Option value="drop">丢弃数据</Option>
            </Select>
          </Form.Item>
        </div>
      )}

      {action.config.type === 'rate_limit' && (
        <div>
          <Form.Item label="速率限制">
            <Row gutter={8}>
              <Col span={12}>
                <InputNumber
                  placeholder="数量"
                  value={action.config.rate}
                  onChange={(value) => updateActionConfig(index, 'rate', value)}
                  style={{ width: '100%' }}
                />
              </Col>
              <Col span={12}>
                <Input
                  placeholder="时间窗口，如 1m"
                  value={action.config.window}
                  onChange={(e) => updateActionConfig(index, 'window', e.target.value)}
                />
              </Col>
            </Row>
          </Form.Item>
        </div>
      )}
    </div>
  );

  // 渲染聚合配置
  const renderAggregateConfig = (action: ActionConfig, index: number) => (
    <div>
      <Form.Item label="窗口类型">
        <Select
          value={action.config.window_type || 'count'}
          onChange={(value) => updateActionConfig(index, 'window_type', value)}
        >
          <Option value="count">计数窗口</Option>
          <Option value="time">时间窗口</Option>
        </Select>
      </Form.Item>

      <Form.Item label="窗口大小">
        {action.config.window_type === 'time' ? (
          <Input
            placeholder="如 5m, 1h"
            value={action.config.size}
            onChange={(e) => updateActionConfig(index, 'size', e.target.value)}
          />
        ) : (
          <InputNumber
            placeholder="数据点数量"
            value={action.config.size}
            onChange={(value) => updateActionConfig(index, 'size', value)}
            min={1}
            style={{ width: '100%' }}
          />
        )}
      </Form.Item>

      <Form.Item label="聚合函数">
        <Select
          mode="multiple"
          placeholder="选择聚合函数"
          value={action.config.functions || []}
          onChange={(value) => updateActionConfig(index, 'functions', value)}
        >
          <Option value="avg">平均值</Option>
          <Option value="sum">求和</Option>
          <Option value="count">计数</Option>
          <Option value="max">最大值</Option>
          <Option value="min">最小值</Option>
          <Option value="stddev">标准差</Option>
        </Select>
      </Form.Item>

      <Form.Item label="分组字段">
        <Select
          mode="tags"
          placeholder="按字段分组"
          value={action.config.group_by || []}
          onChange={(value) => updateActionConfig(index, 'group_by', value)}
        >
          <Option value="device_id">设备ID</Option>
          <Option value="key">数据键</Option>
          <Option value="tags">标签</Option>
        </Select>
      </Form.Item>

      <Form.Item label="输出字段名">
        <Input
          placeholder="如 {{.Key}}_stats"
          value={action.config.output_key || ''}
          onChange={(e) => updateActionConfig(index, 'output_key', e.target.value)}
        />
      </Form.Item>

      <Form.Item label="转发结果">
        <Switch
          checked={action.config.forward}
          onChange={(checked) => updateActionConfig(index, 'forward', checked)}
          checkedChildren="是"
          unCheckedChildren="否"
        />
      </Form.Item>
    </div>
  );

  // 渲染转发配置
  const renderForwardConfig = (action: ActionConfig, index: number) => (
    <div>
      <Form.Item label="转发目标">
        <Select
          value={action.config.target_type || 'http'}
          onChange={(value) => updateActionConfig(index, 'target_type', value)}
        >
          <Option value="http">HTTP接口</Option>
          <Option value="file">文件</Option>
          <Option value="mqtt">MQTT</Option>
          <Option value="kafka">Kafka</Option>
        </Select>
      </Form.Item>

      {action.config.target_type === 'http' && (
        <div>
          <Form.Item label="URL地址">
            <Input
              placeholder="https://api.example.com/data"
              value={action.config.url}
              onChange={(e) => updateActionConfig(index, 'url', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="HTTP方法">
            <Select
              value={action.config.method || 'POST'}
              onChange={(value) => updateActionConfig(index, 'method', value)}
            >
              <Option value="POST">POST</Option>
              <Option value="PUT">PUT</Option>
              <Option value="PATCH">PATCH</Option>
            </Select>
          </Form.Item>
          <Form.Item label="请求头">
            <TextArea
              rows={2}
              placeholder='JSON格式，如: {"Content-Type": "application/json"}'
              value={action.config.headers ? JSON.stringify(action.config.headers, null, 2) : ''}
              onChange={(e) => {
                try {
                  const headers = JSON.parse(e.target.value || '{}');
                  updateActionConfig(index, 'headers', headers);
                } catch {
                  // 忽略JSON解析错误
                }
              }}
            />
          </Form.Item>
        </div>
      )}

      {action.config.target_type === 'file' && (
        <Form.Item label="文件路径">
          <Input
            placeholder="/var/log/iot_data.log"
            value={action.config.path}
            onChange={(e) => updateActionConfig(index, 'path', e.target.value)}
          />
        </Form.Item>
      )}

      {action.config.target_type === 'mqtt' && (
        <div>
          <Form.Item label="MQTT代理">
            <Input
              placeholder="tcp://localhost:1883"
              value={action.config.broker}
              onChange={(e) => updateActionConfig(index, 'broker', e.target.value)}
            />
          </Form.Item>
          <Form.Item label="主题模板">
            <Input
              placeholder="iot/{{.DeviceID}}/{{.Key}}"
              value={action.config.topic}
              onChange={(e) => updateActionConfig(index, 'topic', e.target.value)}
            />
          </Form.Item>
        </div>
      )}

      <Row gutter={16}>
        <Col span={12}>
          <Form.Item label="批处理大小">
            <InputNumber
              placeholder="1"
              value={action.config.batch_size || 1}
              onChange={(value) => updateActionConfig(index, 'batch_size', value)}
              min={1}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item label="超时时间">
            <Input
              placeholder="30s"
              value={action.config.timeout}
              onChange={(e) => updateActionConfig(index, 'timeout', e.target.value)}
            />
          </Form.Item>
        </Col>
      </Row>
    </div>
  );

  // 渲染动作配置表单
  const renderActionConfig = (action: ActionConfig, index: number) => {
    switch (action.type) {
      case 'alert':
        return renderAlertConfig(action, index);
      case 'transform':
        return renderTransformConfig(action, index);
      case 'filter':
        return renderFilterConfig(action, index);
      case 'aggregate':
        return renderAggregateConfig(action, index);
      case 'forward':
        return renderForwardConfig(action, index);
      default:
        return <div>请选择动作类型</div>;
    }
  };

  return (
    <Card title="执行动作配置" size="small">
      <Space direction="vertical" style={{ width: '100%' }}>
        {actions.map((action, index) => (
          <Card
            key={index}
            size="small"
            title={
              <Space>
                {actionTypes.find(t => t.value === action.type)?.icon}
                <span>动作 {index + 1}</span>
                <Tag color="blue">
                  {actionTypes.find(t => t.value === action.type)?.label || action.type}
                </Tag>
              </Space>
            }
            extra={
              actions.length > 1 && (
                <Button
                  type="text"
                  danger
                  size="small"
                  icon={<DeleteOutlined />}
                  onClick={() => removeAction(index)}
                />
              )
            }
          >
            <Form layout="vertical">
              <Form.Item label="动作类型">
                <Select
                  value={action.type}
                  onChange={(value) => {
                    updateAction(index, 'type', value);
                    updateAction(index, 'config', {}); // 重置配置
                  }}
                  style={{ width: '100%' }}
                >
                  {actionTypes.map(type => (
                    <Option key={type.value} value={type.value}>
                      <Space>
                        {type.icon}
                        {type.label}
                        <Text type="secondary">- {type.description}</Text>
                      </Space>
                    </Option>
                  ))}
                </Select>
              </Form.Item>

              {renderActionConfig(action, index)}

              <Divider size="small" />

              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item label={
                    <Space>
                      异步执行
                      <Tooltip title="是否异步执行此动作">
                        <QuestionCircleOutlined />
                      </Tooltip>
                    </Space>
                  }>
                    <Switch
                      checked={action.async}
                      onChange={(checked) => updateAction(index, 'async', checked)}
                      checkedChildren="是"
                      unCheckedChildren="否"
                    />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item label="超时时间">
                    <Input
                      placeholder="30s"
                      value={action.timeout}
                      onChange={(e) => updateAction(index, 'timeout', e.target.value)}
                    />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item label="重试次数">
                    <InputNumber
                      value={action.retry}
                      onChange={(value) => updateAction(index, 'retry', value || 0)}
                      min={0}
                      max={10}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
              </Row>
            </Form>
          </Card>
        ))}

        <Button
          type="dashed"
          icon={<PlusOutlined />}
          onClick={addAction}
          style={{ width: '100%' }}
        >
          添加动作
        </Button>
      </Space>
    </Card>
  );
};

export default ActionForm;