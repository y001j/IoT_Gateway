import React, { useState, useEffect } from 'react';
import {
  Form,
  Input,
  InputNumber,
  Select,
  Button,
  Card,
  Row,
  Col,
  message,
  Alert,
  Space,
  Switch,
  Typography,
  Divider,
  Tag
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  TestOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  DatabaseOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { NatsConfig } from '../../types/settings';

const { Option } = Select;
const { Title, Text } = Typography;
const { TextArea } = Input;

const NatsSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [config, setConfig] = useState<NatsConfig | null>(null);
  const [mode, setMode] = useState<'embedded' | 'external'>('embedded');

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const response = await settingsService.getNatsConfig();
      if (response.success) {
        setConfig(response.data);
        setMode(response.data.mode);
        form.setFieldsValue(response.data);
      } else {
        message.error('加载NATS配置失败');
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
      
      const response = await settingsService.updateNatsConfig(values);
      if (response.success) {
        message.success('NATS配置保存成功');
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

  // 测试连接
  const handleTest = async () => {
    try {
      const values = await form.validateFields();
      setTesting(true);
      
      const response = await settingsService.testNatsConnection(values);
      if (response.success && response.data.success) {
        message.success('NATS连接测试成功');
      } else {
        message.error('连接测试失败：' + (response.data?.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('连接测试失败：' + (error.message || '未知错误'));
    } finally {
      setTesting(false);
    }
  };

  // 重置配置
  const handleReset = () => {
    if (config) {
      form.setFieldsValue(config);
      setMode(config.mode);
      message.info('已重置为已保存的配置');
    }
  };

  // 处理模式切换
  const handleModeChange = (value: 'embedded' | 'external') => {
    setMode(value);
    form.setFieldValue('mode', value);
  };

  useEffect(() => {
    loadConfig();
  }, []);

  return (
    <div>
      <Alert
        message="NATS消息总线配置"
        description="NATS是IoT网关的核心消息总线，负责所有组件间的通信。可以使用内嵌模式或连接到外部NATS服务器。"
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
              <CloudServerOutlined />
              NATS服务器配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="运行模式"
            name="mode"
            rules={[{ required: true, message: '请选择运行模式' }]}
            tooltip="选择使用内嵌NATS服务器还是连接外部服务器"
          >
            <Select 
              placeholder="选择运行模式"
              onChange={handleModeChange}
            >
              <Option value="embedded">
                <Space>
                  <Tag color="green">内嵌模式</Tag>
                  启动内置NATS服务器
                </Space>
              </Option>
              <Option value="external">
                <Space>
                  <Tag color="blue">外部模式</Tag>
                  连接到外部NATS服务器
                </Space>
              </Option>
            </Select>
          </Form.Item>

          {mode === 'external' ? (
            <Form.Item
              label="外部服务器地址"
              name="external_url"
              rules={[
                { required: true, message: '请输入外部NATS服务器地址' }
              ]}
              tooltip="外部NATS服务器的连接地址"
            >
              <Input placeholder="nats://external-nats:4222" />
            </Form.Item>
          ) : (
            <Form.Item
              label="内嵌服务器端口"
              name="embedded_port"
              rules={[
                { required: true, message: '请输入内嵌服务器端口' },
                { type: 'number', min: 1, max: 65535, message: '端口范围1-65535' }
              ]}
              tooltip="内嵌NATS服务器监听端口"
            >
              <InputNumber 
                placeholder="4222" 
                style={{ width: '100%' }}
                min={1}
                max={65535}
              />
            </Form.Item>
          )}
        </Card>

        {/* JetStream配置 */}
        <Card 
          title={
            <Space>
              <DatabaseOutlined />
              JetStream配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用JetStream"
            name={['jetstream', 'enabled']}
            valuePropName="checked"
            tooltip="JetStream提供持久化消息存储和流处理功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="存储目录"
                name={['jetstream', 'store_dir']}
                rules={[{ required: true, message: '请输入存储目录' }]}
                tooltip="JetStream数据存储目录"
              >
                <Input placeholder="./data/jetstream" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="内存限制"
                name={['jetstream', 'max_memory']}
                rules={[{ required: true, message: '请输入内存限制' }]}
                tooltip="JetStream最大内存使用量"
              >
                <Input placeholder="1GB" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="文件存储限制"
                name={['jetstream', 'max_file']}
                rules={[{ required: true, message: '请输入文件存储限制' }]}
                tooltip="JetStream最大文件存储大小"
              >
                <Input placeholder="10GB" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 集群配置 */}
        <Card 
          title={
            <Space>
              <ClusterOutlined />
              集群配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用集群"
            name={['cluster', 'enabled']}
            valuePropName="checked"
            tooltip="启用NATS集群模式以实现高可用性"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="集群端口"
                name={['cluster', 'port']}
                rules={[
                  { type: 'number', min: 1, max: 65535, message: '端口范围1-65535' }
                ]}
                tooltip="集群通信端口"
              >
                <InputNumber 
                  placeholder="6222" 
                  style={{ width: '100%' }}
                  min={1}
                  max={65535}
                />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="集群路由"
                name={['cluster', 'routes']}
                tooltip="其他NATS集群节点的地址列表，每行一个"
              >
                <TextArea 
                  rows={3}
                  placeholder={`nats://node1:6222\nnats://node2:6222`}
                />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 操作按钮 */}
        <Card size="small">
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
              icon={<TestOutlined />}
              loading={testing}
              onClick={handleTest}
            >
              测试连接
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
    </div>
  );
};

export default NatsSettings;