import React, { useState, useEffect } from 'react';
import {
  Form,
  Input,
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
  Table,
  Modal,
  Upload,
  Tag,
  Popconfirm
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  SafetyOutlined,
  KeyOutlined,
  UploadOutlined,
  PlusOutlined,
  DeleteOutlined,
  EyeInvisibleOutlined,
  EyeOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { SecurityConfig, ApiKey } from '../../types/settings';

const { Option } = Select;
const { Title, Text } = Typography;
const { TextArea } = Input;

const SecuritySettings: React.FC = () => {
  const [form] = Form.useForm();
  const [apiKeyForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<SecurityConfig | null>(null);
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([]);
  const [apiKeyModalVisible, setApiKeyModalVisible] = useState(false);
  const [showApiKeys, setShowApiKeys] = useState<Record<string, boolean>>({});

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const [configResponse, apiKeysResponse] = await Promise.all([
        settingsService.getSecurityConfig(),
        settingsService.getApiKeys()
      ]);
      
      if (configResponse.success) {
        setConfig(configResponse.data);
        form.setFieldsValue(configResponse.data);
      }
      
      if (apiKeysResponse.success) {
        setApiKeys(apiKeysResponse.data);
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
      
      const response = await settingsService.updateSecurityConfig(values);
      if (response.success) {
        message.success('安全配置保存成功');
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

  // 创建API密钥
  const handleCreateApiKey = async () => {
    try {
      const values = await apiKeyForm.validateFields();
      const response = await settingsService.createApiKey(values);
      
      if (response.success) {
        message.success('API密钥创建成功');
        setApiKeyModalVisible(false);
        apiKeyForm.resetFields();
        loadConfig(); // 重新加载API密钥列表
      } else {
        message.error('创建失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('创建失败：' + (error.message || '未知错误'));
    }
  };

  // 删除API密钥
  const handleDeleteApiKey = async (id: string) => {
    try {
      const response = await settingsService.deleteApiKey(id);
      if (response.success) {
        message.success('API密钥删除成功');
        setApiKeys(apiKeys.filter(key => key.id !== id));
      } else {
        message.error('删除失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('删除失败：' + (error.message || '未知错误'));
    }
  };

  // 切换API密钥显示
  const toggleApiKeyVisibility = (id: string) => {
    setShowApiKeys(prev => ({
      ...prev,
      [id]: !prev[id]
    }));
  };

  // API密钥表格列
  const apiKeyColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'API密钥',
      dataIndex: 'key',
      key: 'key',
      render: (key: string, record: ApiKey) => (
        <Space>
          <Text code copyable={{ text: key }}>
            {showApiKeys[record.id] ? key : '••••••••••••••••'}
          </Text>
          <Button
            type="text"
            size="small"
            icon={showApiKeys[record.id] ? <EyeInvisibleOutlined /> : <EyeOutlined />}
            onClick={() => toggleApiKeyVisibility(record.id)}
          />
        </Space>
      ),
    },
    {
      title: '权限',
      dataIndex: 'permissions',
      key: 'permissions',
      render: (permissions: string[]) => (
        <Space>
          {permissions.map(permission => (
            <Tag key={permission} color="blue">{permission}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '过期时间',
      dataIndex: 'expires_at',
      key: 'expires_at',
      render: (date?: string) => date ? new Date(date).toLocaleString() : '永不过期',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: ApiKey) => (
        <Popconfirm
          title="确定要删除这个API密钥吗？"
          onConfirm={() => handleDeleteApiKey(record.id)}
          okText="确定"
          cancelText="取消"
        >
          <Button type="text" icon={<DeleteOutlined />} danger>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  useEffect(() => {
    loadConfig();
  }, []);

  return (
    <div>
      <Alert
        message="安全配置说明"
        description="配置系统的认证方式、API密钥管理、HTTPS证书和CORS策略。这些设置直接影响系统的安全性，请谨慎操作。"
        type="warning"
        showIcon
        style={{ marginBottom: 24 }}
      />

      <Form
        form={form}
        layout="vertical"
        onValuesChange={() => {}}
      >
        {/* 认证配置 */}
        <Card 
          title={
            <Space>
              <SafetyOutlined />
              认证配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用认证"
            name={['authentication', 'enabled']}
            valuePropName="checked"
            tooltip="是否启用用户认证功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="认证方式"
                name={['authentication', 'method']}
                rules={[{ required: true, message: '请选择认证方式' }]}
                tooltip="选择用户身份验证的方法"
              >
                <Select placeholder="选择认证方式">
                  <Option value="jwt">JWT Token</Option>
                  <Option value="basic">Basic Auth</Option>
                  <Option value="oauth">OAuth 2.0</Option>
                </Select>
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="JWT密钥"
                name={['authentication', 'jwt_secret']}
                rules={[{ required: true, message: '请输入JWT密钥' }]}
                tooltip="JWT Token签名密钥，请使用强密码"
              >
                <Input.Password placeholder="请输入JWT密钥" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="Token过期时间"
                name={['authentication', 'token_expire']}
                rules={[{ required: true, message: '请输入Token过期时间' }]}
                tooltip="JWT Token的有效期"
              >
                <Input placeholder="24h" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* HTTPS配置 */}
        <Card 
          title={
            <Space>
              <SafetyOutlined />
              HTTPS配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用HTTPS"
            name={['https', 'enabled']}
            valuePropName="checked"
            tooltip="启用HTTPS加密传输"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="证书文件"
                name={['https', 'cert_file']}
                tooltip="HTTPS证书文件路径"
              >
                <Input placeholder="./certs/server.crt" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="私钥文件"
                name={['https', 'key_file']}
                tooltip="HTTPS私钥文件路径"
              >
                <Input placeholder="./certs/server.key" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="自动证书"
                name={['https', 'auto_cert']}
                valuePropName="checked"
                tooltip="使用Let's Encrypt自动获取证书"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* CORS配置 */}
        <Card 
          title={
            <Space>
              <SafetyOutlined />
              CORS配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用CORS"
            name={['cors', 'enabled']}
            valuePropName="checked"
            tooltip="启用跨域资源共享"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="允许的域名"
                name={['cors', 'allowed_origins']}
                tooltip="允许跨域访问的域名列表，每行一个"
              >
                <TextArea 
                  rows={3}
                  placeholder="https://example.com
https://app.example.com
*"
                />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="允许的HTTP方法"
                name={['cors', 'allowed_methods']}
                tooltip="允许的HTTP请求方法"
              >
                <Select
                  mode="multiple"
                  placeholder="选择允许的HTTP方法"
                  options={[
                    { label: 'GET', value: 'GET' },
                    { label: 'POST', value: 'POST' },
                    { label: 'PUT', value: 'PUT' },
                    { label: 'DELETE', value: 'DELETE' },
                    { label: 'PATCH', value: 'PATCH' },
                    { label: 'OPTIONS', value: 'OPTIONS' }
                  ]}
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

      {/* API密钥管理 */}
      <Card 
        title={
          <Space>
            <KeyOutlined />
            API密钥管理
            <Button 
              type="primary" 
              size="small"
              icon={<PlusOutlined />}
              onClick={() => setApiKeyModalVisible(true)}
            >
              创建密钥
            </Button>
          </Space>
        }
        style={{ marginTop: 16 }}
      >
        <Table
          dataSource={apiKeys}
          columns={apiKeyColumns}
          rowKey="id"
          pagination={false}
          size="small"
        />
      </Card>

      {/* 创建API密钥模态框 */}
      <Modal
        title="创建API密钥"
        open={apiKeyModalVisible}
        onOk={handleCreateApiKey}
        onCancel={() => {
          setApiKeyModalVisible(false);
          apiKeyForm.resetFields();
        }}
        okText="创建"
        cancelText="取消"
      >
        <Form form={apiKeyForm} layout="vertical">
          <Form.Item
            label="密钥名称"
            name="name"
            rules={[{ required: true, message: '请输入密钥名称' }]}
          >
            <Input placeholder="输入API密钥名称" />
          </Form.Item>

          <Form.Item
            label="权限"
            name="permissions"
            rules={[{ required: true, message: '请选择权限' }]}
          >
            <Select
              mode="multiple"
              placeholder="选择API权限"
              options={[
                { label: '读取数据', value: 'read' },
                { label: '写入数据', value: 'write' },
                { label: '管理规则', value: 'rules' },
                { label: '管理用户', value: 'users' },
                { label: '系统配置', value: 'admin' }
              ]}
            />
          </Form.Item>

          <Form.Item
            label="过期时间"
            name="expires_at"
            tooltip="留空表示永不过期"
          >
            <Input type="datetime-local" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default SecuritySettings;