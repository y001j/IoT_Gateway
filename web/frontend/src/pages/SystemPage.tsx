import React, { useState, useEffect } from 'react';
import {
  Card,
  Row,
  Col,
  Button,
  Typography,
  Form,
  Input,
  InputNumber,
  Modal,
  Tag,
  Space,
  Tabs,
  Switch,
  Select,
  Divider,
  Table,
  DatePicker,
  Popconfirm,
  App
} from 'antd';
import {
  ReloadOutlined,
  SettingOutlined,
  SaveOutlined,
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  KeyOutlined,
  SecurityScanOutlined,
  DatabaseOutlined,
  CloudServerOutlined
} from '@ant-design/icons';
import { systemService } from '../services/systemService';
import { useAuthStore } from '../store/authStore';
import type {
  SystemStatus,
  SystemMetrics,
} from '../types/system';

const { Title } = Typography;

const SystemPage: React.FC = () => {
  const { message } = App.useApp();
  const [loading, setLoading] = useState(false);
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [systemMetrics, setSystemMetrics] = useState<SystemMetrics | null>(null);
  const [systemConfig, setSystemConfig] = useState<any>(null);
  // 移除未使用的状态变量
  const [configForm] = Form.useForm();
  const [activeTab, setActiveTab] = useState('status');

  // 获取默认配置
  const getDefaultConfig = () => ({
    gateway: {
      id: 'iot-gateway',
      http_port: 8080,
      log_level: 'info',
      nats_url: 'nats://localhost:4222',
      plugins_dir: './plugins',
      metrics: {
        enabled: true,
        port: 9090
      }
    },
    nats: {
      enabled: true,
      embedded: true,
      host: 'localhost',
      port: 4222,
      cluster_port: 6222,
      monitor_port: 8222,
      jetstream: {
        enabled: true,
        store_dir: './data/jetstream',
        max_memory: 1073741824,
        max_file: 10737418240
      },
      cluster: {
        enabled: false,
        name: 'iot-cluster',
        routes: []
      },
      tls: {
        enabled: false,
        cert_file: '',
        key_file: '',
        ca_file: ''
      }
    },
    web_ui: {
      enabled: true,
      port: 3000,
      auth: {
        jwt_secret: 'your-secret-key',
        token_duration: '24h',
        refresh_duration: '72h',
        max_login_attempts: 5,
        lockout_duration: '15m',
        enable_two_factor: false,
        session_timeout: '30m',
        password_min_length: 8,
        bcrypt_cost: 12
      }
    },
    database: {
      sqlite: {
        path: './data/iot-gateway.db',
        max_open_conns: 25,
        max_idle_conns: 5,
        conn_max_lifetime: '5m',
        conn_max_idle_time: '1m'
      }
    },
    security: {
      api_keys: {
        enabled: false,
        keys: []
      },
      https: {
        enabled: false,
        cert_file: '',
        key_file: '',
        redirect_http: false
      },
      cors: {
        enabled: true,
        allowed_origins: ['*'],
        allowed_methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
        allowed_headers: ['*'],
        credentials: false
      }
    },
    rules: {
      dir: './rules'
    }
  });

  // 获取系统状态
  const fetchSystemStatus = async () => {
    try {
      const status = await systemService.getStatus();
      setSystemStatus(status);
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : '未知错误';
      message.error('获取系统状态失败：' + errorMessage);
    }
  };

  // 获取系统指标
  const fetchSystemMetrics = async () => {
    try {
      const metrics = await systemService.getMetrics();
      setSystemMetrics(metrics);
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : '未知错误';
      message.error('获取系统指标失败：' + errorMessage);
    }
  };

  // 获取健康检查
  const fetchHealthCheck = async () => {
    try {
      const health = await systemService.getHealth();
      // 健康检查数据获取成功，但暂时不使用
      console.log('健康检查数据:', health);
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : '未知错误';
      message.error('获取健康检查失败：' + errorMessage);
    }
  };

  // 获取系统配置
  const fetchSystemConfig = async () => {
    try {
      const config = await systemService.getConfig();
      // 系统配置获取成功
      if (config) {
        setSystemConfig(config);
        configForm.setFieldsValue(config);
      }
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : '未知错误';
      message.error('获取系统配置失败：' + errorMessage);
      // 如果获取失败，设置默认值以避免警告
      const defaultConfig = getDefaultConfig();
      setSystemConfig(defaultConfig);
      configForm.setFieldsValue(defaultConfig);
    }
  };

  // 刷新所有数据
  const refreshAll = async () => {
    setLoading(true);
    try {
      await Promise.all([
        fetchSystemStatus(),
        fetchSystemMetrics(),
        fetchHealthCheck(),
        fetchSystemConfig()
      ]);
    } finally {
      setLoading(false);
    }
  };

  // 保存配置
  const handleSaveConfig = async () => {
    try {
      const values = await configForm.validateFields();
      await systemService.updateConfig(values);
      message.success('配置保存成功');
      await fetchSystemConfig();
    } catch (error: unknown) {
      const errorMessage = error instanceof Error ? error.message : '未知错误';
      message.error('保存配置失败：' + errorMessage);
    }
  };

  // 重启系统
  const handleRestart = () => {
    Modal.confirm({
      title: '确认重启系统',
      content: '重启系统将会中断所有正在运行的连接和任务，确定要继续吗？',
      okText: '确认重启',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await systemService.restart();
          message.success('系统重启中...');
        } catch (error: unknown) {
          const errorMessage = error instanceof Error ? error.message : '未知错误';
          message.error('重启失败：' + errorMessage);
        }
      },
    });
  };

  // 格式化运行时间
  const formatUptime = (seconds: number) => {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return `${days}天 ${hours}小时 ${minutes}分钟`;
  };

  // 格式化文件大小
  const formatFileSize = (bytes: number) => {
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let size = bytes;
    let unitIndex = 0;
    while (size >= 1024 && unitIndex < units.length - 1) {
      size /= 1024;
      unitIndex++;
    }
    return `${size.toFixed(2)} ${units[unitIndex]}`;
  };

  // 获取状态颜色
  const getStatusColor = (status: string): 'success' | 'warning' | 'error' | 'default' => {
    switch (status.toLowerCase()) {
      case 'running':
      case 'healthy':
      case 'connected':
        return 'success';
      case 'warning':
      case 'degraded':
        return 'warning';
      case 'error':
      case 'failed':
      case 'disconnected':
        return 'error';
      default:
        return 'default';
    }
  };

  useEffect(() => {
    // 初始化表单默认值
    configForm.setFieldsValue(getDefaultConfig());
    
    refreshAll();
  }, [configForm]);

  const tabItems = [
    {
      key: 'status',
      label: '系统状态',
      children: (
        <div>
          <Row gutter={[16, 16]}>
            <Col span={12}>
              <Card title="基本信息" size="small">
                {systemStatus && (
                  <div>
                    <p><strong>状态:</strong> <Tag color={getStatusColor(systemStatus.status)}>{systemStatus.status}</Tag></p>
                    <p><strong>版本:</strong> {systemStatus.version}</p>
                    <p><strong>运行时间:</strong> {formatUptime(systemStatus.uptime)}</p>
                    <p><strong>Go版本:</strong> {systemStatus.go_version}</p>
                  </div>
                )}
              </Card>
            </Col>
            <Col span={12}>
              <Card title="资源使用" size="small">
                {systemMetrics && (
                  <div>
                    <p><strong>CPU:</strong> {systemMetrics.cpu_percent ? systemMetrics.cpu_percent.toFixed(1) : '0.0'}%</p>
                    <p><strong>内存:</strong> {formatFileSize(systemMetrics.memory_used || 0)} / {formatFileSize(systemMetrics.memory_total || 0)}</p>
                    <p><strong>磁盘:</strong> {formatFileSize(systemMetrics.disk_used || 0)} / {formatFileSize(systemMetrics.disk_total || 0)}</p>
                  </div>
                )}
              </Card>
            </Col>
          </Row>
        </div>
      )
    },
    {
      key: 'config',
      label: '系统配置',
      children: (
        <div>
          <div style={{ marginBottom: '20px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Title level={3}>系统配置</Title>
            <Space>
              <Button icon={<SaveOutlined />} onClick={handleSaveConfig} type="primary">
                保存配置
              </Button>
            </Space>
          </div>
          
          <Form form={configForm} layout="vertical">
            <Tabs
              items={[
                {
                  key: 'gateway',
                  label: '网关配置',
                  icon: <SettingOutlined />,
                  children: (
                    <Card title="网关基础配置（配置文件）" size="small">
                      <div style={{ marginBottom: 16, padding: '8px 12px', backgroundColor: '#f6f8fa', borderRadius: 4, fontSize: 12, color: '#666' }}>
                        ℹ️ 网关配置来自配置文件，只能查看，不能修改。如需修改请编辑配置文件并重启服务。
                      </div>
                      <Row gutter={16}>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>网关ID</div>
                            <Input value={systemConfig?.gateway?.id || '-'} readOnly />
                          </div>
                        </Col>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>HTTP端口</div>
                            <Input value={systemConfig?.gateway?.http_port || '-'} readOnly />
                          </div>
                        </Col>
                      </Row>
                      <Row gutter={16}>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>日志级别</div>
                            <Input value={systemConfig?.gateway?.log_level || '-'} readOnly />
                          </div>
                        </Col>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>NATS连接地址</div>
                            <Input value={systemConfig?.gateway?.nats_url || '-'} readOnly />
                          </div>
                        </Col>
                      </Row>
                      <Row gutter={16}>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>插件目录</div>
                            <Input value={systemConfig?.gateway?.plugins_dir || '-'} readOnly />
                          </div>
                        </Col>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>指标端口</div>
                            <Input value={systemConfig?.gateway?.metrics?.port || '-'} readOnly />
                          </div>
                        </Col>
                      </Row>
                      <Row gutter={16}>
                        <Col span={12}>
                          <div style={{ marginBottom: 16 }}>
                            <div style={{ marginBottom: 4, fontWeight: 500 }}>启用指标</div>
                            <Tag color={systemConfig?.gateway?.metrics?.enabled ? 'green' : 'red'}>
                              {systemConfig?.gateway?.metrics?.enabled ? '已启用' : '已禁用'}
                            </Tag>
                          </div>
                        </Col>
                      </Row>
                    </Card>
                  )
                },
                {
                  key: 'nats',
                  label: 'NATS配置',
                  icon: <CloudServerOutlined />,
                  children: (
                    <div>
                      <Card title="NATS服务器配置" size="small" style={{ marginBottom: 16 }}>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="启用NATS" name={['nats', 'enabled']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="内嵌模式" name={['nats', 'embedded']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                          </Col>
                        </Row>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="主机地址" name={['nats', 'host']}>
                              <Input placeholder="localhost" />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="端口" name={['nats', 'port']}>
                              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                            </Form.Item>
                          </Col>
                        </Row>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="集群端口" name={['nats', 'cluster_port']}>
                              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="监控端口" name={['nats', 'monitor_port']}>
                              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                            </Form.Item>
                          </Col>
                        </Row>
                      </Card>
                      
                      <Card title="JetStream配置" size="small" style={{ marginBottom: 16 }}>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="启用JetStream" name={['nats', 'jetstream', 'enabled']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="存储目录" name={['nats', 'jetstream', 'store_dir']}>
                              <Input placeholder="./data/jetstream" />
                            </Form.Item>
                          </Col>
                        </Row>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="最大内存(字节)" name={['nats', 'jetstream', 'max_memory']}>
                              <InputNumber min={0} style={{ width: '100%' }} />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="最大文件大小(字节)" name={['nats', 'jetstream', 'max_file']}>
                              <InputNumber min={0} style={{ width: '100%' }} />
                            </Form.Item>
                          </Col>
                        </Row>
                      </Card>
                    </div>
                  )
                },
                {
                  key: 'database',
                  label: '数据库配置',
                  icon: <DatabaseOutlined />,
                  children: (
                    <Card title="SQLite数据库配置" size="small">
                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="数据库路径" name={['database', 'sqlite', 'path']}>
                            <Input placeholder="./data/iot-gateway.db" />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="最大连接数" name={['database', 'sqlite', 'max_open_conns']}>
                            <InputNumber min={1} max={100} style={{ width: '100%' }} />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="最大空闲连接数" name={['database', 'sqlite', 'max_idle_conns']}>
                            <InputNumber min={1} max={50} style={{ width: '100%' }} />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="连接最大生存时间" name={['database', 'sqlite', 'conn_max_lifetime']}>
                            <Input placeholder="5m" />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="连接最大空闲时间" name={['database', 'sqlite', 'conn_max_idle_time']}>
                            <Input placeholder="1m" />
                          </Form.Item>
                        </Col>
                      </Row>
                    </Card>
                  )
                },
                {
                  key: 'security',
                  label: '安全配置',
                  icon: <SecurityScanOutlined />,
                  children: (
                    <div>
                      <Card title="HTTPS配置" size="small" style={{ marginBottom: 16 }}>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="启用HTTPS" name={['security', 'https', 'enabled']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="重定向HTTP" name={['security', 'https', 'redirect_http']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                          </Col>
                        </Row>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="证书文件" name={['security', 'https', 'cert_file']}>
                              <Input placeholder="./certs/server.crt" />
                            </Form.Item>
                          </Col>
                          <Col span={12}>
                            <Form.Item label="私钥文件" name={['security', 'https', 'key_file']}>
                              <Input placeholder="./certs/server.key" />
                            </Form.Item>
                          </Col>
                        </Row>
                      </Card>
                      
                      <Card title="API密钥管理" size="small" style={{ marginBottom: 16 }}>
                        <Row gutter={16}>
                          <Col span={12}>
                            <Form.Item label="启用API密钥" name={['security', 'api_keys', 'enabled']} valuePropName="checked">
                              <Switch />
                            </Form.Item>
                          </Col>
                        </Row>
                        <Divider>API密钥列表</Divider>
                        <Form.List name={['security', 'api_keys', 'keys']}>
                          {(fields, { add, remove }) => (
                            <>
                              {fields.map(({ key, name, ...restField }) => (
                                <div key={key} style={{ display: 'flex', marginBottom: 8, alignItems: 'center' }}>
                                  <Form.Item
                                    {...restField}
                                    name={[name, 'name']}
                                    style={{ flex: 1, marginRight: 8 }}
                                  >
                                    <Input placeholder="密钥名称" />
                                  </Form.Item>
                                  <Form.Item
                                    {...restField}
                                    name={[name, 'key']}
                                    style={{ flex: 2, marginRight: 8 }}
                                  >
                                    <Input placeholder="密钥值" />
                                  </Form.Item>
                                  <Form.Item
                                    {...restField}
                                    name={[name, 'enabled']}
                                    valuePropName="checked"
                                    style={{ marginRight: 8 }}
                                  >
                                    <Switch size="small" />
                                  </Form.Item>
                                  <Button
                                    type="link"
                                    danger
                                    icon={<DeleteOutlined />}
                                    onClick={() => remove(name)}
                                  />
                                </div>
                              ))}
                              <Form.Item>
                                <Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
                                  添加API密钥
                                </Button>
                              </Form.Item>
                            </>
                          )}
                        </Form.List>
                      </Card>
                    </div>
                  )
                }
              ]}
            />
          </Form>
        </div>
      )
    }
  ];

  return (
    <div style={{ padding: '20px' }}>
      <div style={{ marginBottom: '20px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Title level={2}>系统管理</Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={refreshAll} loading={loading}>
            刷新
          </Button>
          <Button danger icon={<SettingOutlined />} onClick={handleRestart}>
            重启系统
          </Button>
        </Space>
      </div>

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={tabItems}
      />
    </div>
  );
};

export default SystemPage; 