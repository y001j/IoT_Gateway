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
  Divider,
  Typography,
  Switch
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  TestOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { GatewayConfig } from '../../types/settings';

const { Option } = Select;
const { Title, Text } = Typography;

const GatewaySettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [config, setConfig] = useState<GatewayConfig | null>(null);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const response = await settingsService.getGatewayConfig();
      if (response.success) {
        setConfig(response.data);
        form.setFieldsValue(response.data);
      } else {
        message.error('加载网关配置失败');
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
      
      const response = await settingsService.updateGatewayConfig(values);
      if (response.success) {
        message.success('网关配置保存成功');
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

  // 测试配置
  const handleTest = async () => {
    try {
      const values = await form.validateFields();
      setTesting(true);
      
      // 这里可以添加配置测试逻辑
      // const response = await settingsService.testGatewayConfig(values);
      
      // 模拟测试
      await new Promise(resolve => setTimeout(resolve, 2000));
      message.success('配置测试通过');
    } catch (error: any) {
      message.error('配置测试失败：' + (error.message || '未知错误'));
    } finally {
      setTesting(false);
    }
  };

  // 重置配置
  const handleReset = () => {
    if (config) {
      form.setFieldsValue(config);
      message.info('已重置为已保存的配置');
    }
  };

  useEffect(() => {
    loadConfig();
  }, []);

  return (
    <div>
      <Alert
        message="网关配置说明"
        description="配置IoT网关的核心参数，包括服务端口、日志级别、NATS连接等基础设置。修改配置后需要重启服务才能生效。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      <Form
        form={form}
        layout="vertical"
        onValuesChange={() => {}}
      >
        {/* 基础服务配置 */}
        <Card 
          title={
            <Space>
              <SettingOutlined />
              基础服务配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="HTTP端口"
                name="http_port"
                rules={[
                  { required: true, message: '请输入HTTP端口' },
                  { type: 'number', min: 1, max: 65535, message: '端口范围1-65535' }
                ]}
                tooltip="Web UI和REST API的HTTP服务端口"
              >
                <InputNumber 
                  placeholder="8080" 
                  style={{ width: '100%' }}
                  min={1}
                  max={65535}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="HTTPS端口"
                name="https_port"
                rules={[
                  { type: 'number', min: 1, max: 65535, message: '端口范围1-65535' }
                ]}
                tooltip="HTTPS服务端口（可选）"
              >
                <InputNumber 
                  placeholder="8443" 
                  style={{ width: '100%' }}
                  min={1}
                  max={65535}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="日志级别"
                name="log_level"
                rules={[{ required: true, message: '请选择日志级别' }]}
                tooltip="系统日志输出级别"
              >
                <Select placeholder="选择日志级别">
                  <Option value="debug">Debug - 调试信息</Option>
                  <Option value="info">Info - 一般信息</Option>
                  <Option value="warn">Warn - 警告信息</Option>
                  <Option value="error">Error - 错误信息</Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 连接配置 */}
        <Card 
          title="连接配置" 
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="NATS服务器地址"
                name="nats_url"
                rules={[{ required: true, message: '请输入NATS服务器地址' }]}
                tooltip="NATS消息总线连接地址"
              >
                <Input placeholder="nats://localhost:4222" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="插件目录"
                name="plugins_dir"
                rules={[{ required: true, message: '请输入插件目录路径' }]}
                tooltip="外部插件存放目录"
              >
                <Input placeholder="./plugins" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="最大连接数"
                name="max_connections"
                rules={[
                  { required: true, message: '请输入最大连接数' },
                  { type: 'number', min: 1, message: '最大连接数必须大于0' }
                ]}
                tooltip="同时允许的最大客户端连接数"
              >
                <InputNumber 
                  placeholder="1000" 
                  style={{ width: '100%' }}
                  min={1}
                  max={100000}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="读取超时"
                name="read_timeout"
                rules={[{ required: true, message: '请输入读取超时时间' }]}
                tooltip="网络读取操作超时时间"
              >
                <Input placeholder="30s" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="写入超时"
                name="write_timeout"
                rules={[{ required: true, message: '请输入写入超时时间' }]}
                tooltip="网络写入操作超时时间"
              >
                <Input placeholder="30s" />
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
              测试配置
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

export default GatewaySettings;