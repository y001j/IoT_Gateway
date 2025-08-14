import React, { useState, useEffect } from 'react';
import {
  Form,
  Input,
  InputNumber,
  Button,
  Card,
  Row,
  Col,
  message,
  Alert,
  Space,
  Switch,
  Typography,
  Table,
  Modal,
  Select,
  Popconfirm,
  Tag
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  BellOutlined,
  MailOutlined,
  PhoneOutlined,
  LinkOutlined,
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  ExperimentOutlined as TestOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { AlertConfig, AlertChannel } from '../../types/settings';

const { Option } = Select;
// const { Title } = Typography;
const { TextArea } = Input;

const AlertSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [channelForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<AlertConfig | null>(null);
  const [channels, setChannels] = useState<AlertChannel[]>([]);
  const [channelModalVisible, setChannelModalVisible] = useState(false);
  const [editingChannel, setEditingChannel] = useState<AlertChannel | null>(null);
  const [testing, setTesting] = useState<Record<string, boolean>>({});

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const [configResponse, channelsResponse] = await Promise.all([
        settingsService.getAlertConfig(),
        settingsService.getAlertChannels()
      ]);
      
      if (configResponse.success) {
        setConfig(configResponse.data);
        form.setFieldsValue(configResponse.data);
      }
      
      if (channelsResponse.success) {
        setChannels(channelsResponse.data);
      }
    } catch (error: any) {
      message.error('加载配置失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 保存配置
  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      
      const response = await settingsService.updateAlertConfig(values);
      if (response.success) {
        message.success('告警配置保存成功');
        setConfig(values);
      } else {
        message.error('保存配置失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      if (error.errorFields) {
        message.error('请检查表单输入');
      } else {
        message.error('保存配置失败：' + (error.message || '未知错误'));
      }
    } finally {
      setLoading(false);
    }
  };

  // 重置配置
  const handleReset = () => {
    if (config) {
      form.setFieldsValue(config);
      message.info('已重置为已保存的配置');
    }
  };

  // 创建/编辑告警通道
  const handleSaveChannel = async () => {
    try {
      const values = await channelForm.validateFields();
      
      if (editingChannel) {
        // 更新通道
        const response = await settingsService.updateAlertChannel(editingChannel.id, values);
        if (response.success) {
          message.success('告警通道更新成功');
          setChannels(channels.map(ch => ch.id === editingChannel.id ? { ...ch, ...values } : ch));
        } else {
          message.error('更新失败：' + (response.message || '未知错误'));
        }
      } else {
        // 创建新通道
        const response = await settingsService.createAlertChannel(values);
        if (response.success) {
          message.success('告警通道创建成功');
          setChannels([...channels, response.data]);
        } else {
          message.error('创建失败：' + (response.message || '未知错误'));
        }
      }
      
      setChannelModalVisible(false);
      setEditingChannel(null);
      channelForm.resetFields();
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 删除告警通道
  const handleDeleteChannel = async (id: string) => {
    try {
      const response = await settingsService.deleteAlertChannel(id);
      if (response.success) {
        message.success('告警通道删除成功');
        setChannels(channels.filter(ch => ch.id !== id));
      } else {
        message.error('删除失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('删除失败：' + (error.message || '未知错误'));
    }
  };

  // 测试告警通道
  const handleTestChannel = async (channel: AlertChannel) => {
    setTesting(prev => ({ ...prev, [channel.id]: true }));
    try {
      const response = await settingsService.testAlertChannel(channel.id);
      if (response.success) {
        message.success('测试消息发送成功');
      } else {
        message.error('测试失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('测试失败：' + (error.message || '未知错误'));
    } finally {
      setTesting(prev => ({ ...prev, [channel.id]: false }));
    }
  };

  // 打开编辑模态框
  const handleEditChannel = (channel: AlertChannel) => {
    setEditingChannel(channel);
    channelForm.setFieldsValue(channel);
    setChannelModalVisible(true);
  };

  // 获取通道类型图标
  const getChannelIcon = (type: string) => {
    switch (type) {
      case 'email':
        return <MailOutlined />;
      case 'sms':
        return <PhoneOutlined />;
      case 'webhook':
        return <LinkOutlined />;
      default:
        return <BellOutlined />;
    }
  };

  // 告警通道表格列
  const channelColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: AlertChannel) => (
        <Space>
          {getChannelIcon(record.type)}
          {name}
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => {
        const typeMap = {
          email: '邮件',
          sms: '短信',
          webhook: 'Webhook'
        };
        return <Tag color="blue">{typeMap[type as keyof typeof typeMap] || type}</Tag>;
      },
    },
    {
      title: '配置',
      dataIndex: 'config',
      key: 'config',
      render: (config: any, record: AlertChannel) => {
        switch (record.type) {
          case 'email':
            return `${config.to}`;
          case 'sms':
            return `${config.phone}`;
          case 'webhook':
            return `${config.url}`;
          default:
            return '-';
        }
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'green' : 'red'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: AlertChannel) => (
        <Space>
          <Button 
            type="text" 
            icon={<TestOutlined />}
            loading={testing[record.id]}
            onClick={() => handleTestChannel(record)}
          >
            测试
          </Button>
          <Button 
            type="text" 
            icon={<EditOutlined />}
            onClick={() => handleEditChannel(record)}
          >
            编辑
          </Button>
          <Popconfirm
            title="确定要删除这个告警通道吗？"
            onConfirm={() => handleDeleteChannel(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="text" icon={<DeleteOutlined />} danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  useEffect(() => {
    loadConfig();
  }, []);

  return (
    <div>
      <Alert
        message="告警配置说明"
        description="配置系统告警通道和告警策略。当系统监控指标超过阈值时，将通过配置的通道发送告警通知。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      <Form
        form={form}
        layout="vertical"
        onValuesChange={() => {}}
      >
        {/* 基础配置 */}
        <Card 
          title={
            <Space>
              <BellOutlined />
              告警基础配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用告警"
            name="enabled"
            valuePropName="checked"
            tooltip="启用系统告警功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="告警间隔"
                name="interval"
                rules={[{ required: true, message: '请输入告警间隔' }]}
                tooltip="相同告警的最小发送间隔"
              >
                <Input placeholder="5m" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="告警恢复通知"
                name="recovery_notification"
                valuePropName="checked"
                tooltip="当告警状态恢复时发送通知"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="最大重试次数"
                name="max_retries"
                rules={[
                  { required: true, message: '请输入最大重试次数' },
                  { type: 'number', min: 0, message: '重试次数不能小于0' }
                ]}
                tooltip="告警发送失败时的最大重试次数"
              >
                <InputNumber 
                  placeholder="3" 
                  style={{ width: '100%' }}
                  min={0}
                  max={10}
                />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 操作按钮 */}
        <Card size="small" style={{ marginBottom: 16 }}>
          <Space>
            <Button 
              type="primary" 
              icon={<SaveOutlined />}
              loading={loading}
              onClick={handleSave}
            >
              保存配置
            </Button>
            
            <Button 
              icon={<ReloadOutlined />}
              onClick={handleReset}
            >
              重置
            </Button>
            
            <Button 
              icon={<ReloadOutlined />}
              onClick={loadConfig}
              loading={loading}
            >
              重新加载
            </Button>
          </Space>
        </Card>
      </Form>

      {/* 告警通道管理 */}
      <Card 
        title={
          <Space>
            <BellOutlined />
            告警通道管理
            <Button 
              type="primary" 
              size="small"
              icon={<PlusOutlined />}
              onClick={() => {
                setEditingChannel(null);
                channelForm.resetFields();
                setChannelModalVisible(true);
              }}
            >
              添加通道
            </Button>
          </Space>
        }
      >
        <Table
          dataSource={channels}
          columns={channelColumns}
          rowKey="id"
          pagination={false}
          size="small"
        />
      </Card>

      {/* 创建/编辑告警通道模态框 */}
      <Modal
        title={editingChannel ? '编辑告警通道' : '创建告警通道'}
        open={channelModalVisible}
        onOk={handleSaveChannel}
        onCancel={() => {
          setChannelModalVisible(false);
          setEditingChannel(null);
          channelForm.resetFields();
        }}
        okText="保存"
        cancelText="取消"
        width={600}
      >
        <Form form={channelForm} layout="vertical">
          <Form.Item
            label="通道名称"
            name="name"
            rules={[{ required: true, message: '请输入通道名称' }]}
          >
            <Input placeholder="输入告警通道名称" />
          </Form.Item>

          <Form.Item
            label="通道类型"
            name="type"
            rules={[{ required: true, message: '请选择通道类型' }]}
          >
            <Select placeholder="选择通道类型">
              <Option value="email">邮件</Option>
              <Option value="sms">短信</Option>
              <Option value="webhook">Webhook</Option>
            </Select>
          </Form.Item>

          <Form.Item
            noStyle
            shouldUpdate={(prevValues, currentValues) => prevValues.type !== currentValues.type}
          >
            {({ getFieldValue }) => {
              const type = getFieldValue('type');
              
              if (type === 'email') {
                return (
                  <Row gutter={16}>
                    <Col span={12}>
                      <Form.Item
                        label="SMTP服务器"
                        name={['config', 'smtp_server']}
                        rules={[{ required: true, message: '请输入SMTP服务器' }]}
                      >
                        <Input placeholder="smtp.gmail.com" />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="SMTP端口"
                        name={['config', 'smtp_port']}
                        rules={[{ required: true, message: '请输入SMTP端口' }]}
                      >
                        <InputNumber placeholder="587" style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="发件人邮箱"
                        name={['config', 'from']}
                        rules={[{ required: true, message: '请输入发件人邮箱' }]}
                      >
                        <Input placeholder="alert@example.com" />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="收件人邮箱"
                        name={['config', 'to']}
                        rules={[{ required: true, message: '请输入收件人邮箱' }]}
                      >
                        <Input placeholder="admin@example.com" />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="用户名"
                        name={['config', 'username']}
                        rules={[{ required: true, message: '请输入用户名' }]}
                      >
                        <Input placeholder="用户名" />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="密码"
                        name={['config', 'password']}
                        rules={[{ required: true, message: '请输入密码' }]}
                      >
                        <Input.Password placeholder="密码或应用密码" />
                      </Form.Item>
                    </Col>
                  </Row>
                );
              } else if (type === 'sms') {
                return (
                  <Row gutter={16}>
                    <Col span={12}>
                      <Form.Item
                        label="短信服务商"
                        name={['config', 'provider']}
                        rules={[{ required: true, message: '请选择短信服务商' }]}
                      >
                        <Select placeholder="选择短信服务商">
                          <Option value="aliyun">阿里云</Option>
                          <Option value="tencent">腾讯云</Option>
                          <Option value="twilio">Twilio</Option>
                        </Select>
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="接收手机号"
                        name={['config', 'phone']}
                        rules={[{ required: true, message: '请输入接收手机号' }]}
                      >
                        <Input placeholder="+8613812345678" />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="Access Key"
                        name={['config', 'access_key']}
                        rules={[{ required: true, message: '请输入Access Key' }]}
                      >
                        <Input placeholder="Access Key" />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="Secret Key"
                        name={['config', 'secret_key']}
                        rules={[{ required: true, message: '请输入Secret Key' }]}
                      >
                        <Input.Password placeholder="Secret Key" />
                      </Form.Item>
                    </Col>
                  </Row>
                );
              } else if (type === 'webhook') {
                return (
                  <Row gutter={16}>
                    <Col span={24}>
                      <Form.Item
                        label="Webhook URL"
                        name={['config', 'url']}
                        rules={[{ required: true, message: '请输入Webhook URL' }]}
                      >
                        <Input placeholder="https://hooks.slack.com/services/..." />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="HTTP方法"
                        name={['config', 'method']}
                        rules={[{ required: true, message: '请选择HTTP方法' }]}
                      >
                        <Select placeholder="选择HTTP方法">
                          <Option value="POST">POST</Option>
                          <Option value="PUT">PUT</Option>
                        </Select>
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="Content-Type"
                        name={['config', 'content_type']}
                        rules={[{ required: true, message: '请选择Content-Type' }]}
                      >
                        <Select placeholder="选择Content-Type">
                          <Option value="application/json">application/json</Option>
                          <Option value="application/x-www-form-urlencoded">application/x-www-form-urlencoded</Option>
                        </Select>
                      </Form.Item>
                    </Col>
                    <Col span={24}>
                      <Form.Item
                        label="请求头"
                        name={['config', 'headers']}
                        tooltip="JSON格式的请求头，例如：{&quot;Authorization&quot;: &quot;Bearer token&quot;}"
                      >
                        <TextArea rows={3} placeholder='{"Authorization": "Bearer token"}' />
                      </Form.Item>
                    </Col>
                  </Row>
                );
              }
              return null;
            }}
          </Form.Item>

          <Form.Item
            label="启用通道"
            name="enabled"
            valuePropName="checked"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default AlertSettings;