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
  Typography
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  TestOutlined,
  DatabaseOutlined,
  BackupOutlined,
  HistoryOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { DatabaseConfig } from '../../types/settings';

const { Title, Text } = Typography;

const DatabaseSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [config, setConfig] = useState<DatabaseConfig | null>(null);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const response = await settingsService.getDatabaseConfig();
      if (response.success) {
        setConfig(response.data);
        form.setFieldsValue(response.data);
      } else {
        message.error('加载数据库配置失败');
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
      
      const response = await settingsService.updateDatabaseConfig(values);
      if (response.success) {
        message.success('数据库配置保存成功');
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
      
      const response = await settingsService.testDatabaseConnection(values);
      if (response.success && response.data.success) {
        message.success('数据库连接测试成功');
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
      message.info('已重置为已保存的配置');
    }
  };

  useEffect(() => {
    loadConfig();
  }, []);

  return (
    <div>
      <Alert
        message="数据库配置说明"
        description="配置SQLite数据库设置，包括数据库文件路径、连接池参数和备份策略。数据库用于存储用户信息、规则配置和审计日志。"
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
              <DatabaseOutlined />
              数据库配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="SQLite数据库路径"
            name="sqlite_path"
            rules={[{ required: true, message: '请输入数据库文件路径' }]}
            tooltip="SQLite数据库文件的存储路径"
          >
            <Input placeholder="./data/gateway.db" />
          </Form.Item>
        </Card>

        {/* 连接池配置 */}
        <Card 
          title={
            <Space>
              <DatabaseOutlined />
              连接池配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="最大打开连接数"
                name={['connection_pool', 'max_open_conns']}
                rules={[
                  { required: true, message: '请输入最大打开连接数' },
                  { type: 'number', min: 1, message: '连接数必须大于0' }
                ]}
                tooltip="同时允许的最大数据库连接数"
              >
                <InputNumber 
                  placeholder="25" 
                  style={{ width: '100%' }}
                  min={1}
                  max={1000}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="最大空闲连接数"
                name={['connection_pool', 'max_idle_conns']}
                rules={[
                  { required: true, message: '请输入最大空闲连接数' },
                  { type: 'number', min: 1, message: '连接数必须大于0' }
                ]}
                tooltip="连接池中保持的最大空闲连接数"
              >
                <InputNumber 
                  placeholder="5" 
                  style={{ width: '100%' }}
                  min={1}
                  max={100}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="连接最大生存时间"
                name={['connection_pool', 'conn_max_lifetime']}
                rules={[{ required: true, message: '请输入连接最大生存时间' }]}
                tooltip="单个连接的最大生存时间"
              >
                <Input placeholder="1h" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 备份配置 */}
        <Card 
          title={
            <Space>
              <BackupOutlined />
              备份配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用自动备份"
            name={['backup', 'enabled']}
            valuePropName="checked"
            tooltip="启用数据库自动备份功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="备份间隔"
                name={['backup', 'interval']}
                rules={[{ required: true, message: '请输入备份间隔时间' }]}
                tooltip="自动备份的时间间隔"
              >
                <Input placeholder="24h" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="备份保留时间"
                name={['backup', 'retention']}
                rules={[{ required: true, message: '请输入备份保留时间' }]}
                tooltip="备份文件的保留时间"
              >
                <Input placeholder="30d" />
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

export default DatabaseSettings;