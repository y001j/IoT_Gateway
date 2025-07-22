import React, { useState, useEffect } from 'react';
import {
  Modal,
  Tabs,
  Card,
  Descriptions,
  Button,
  Tag,
  Table,
  Form,
  Input,
  Select,
  message,
  Spin,
  Row,
  Col,
  Progress,
  Typography,
  Divider,
  Space,
  Statistic
} from 'antd';
import {
  PauseCircleOutlined,
  ReloadOutlined,
  EditOutlined,
  SaveOutlined,
  FileTextOutlined,
  BarChartOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  CloseCircleOutlined,
  PlayCircleOutlined,
  SettingOutlined
} from '@ant-design/icons';
import Editor from '@monaco-editor/react';
import { pluginService } from '../services/pluginService';
import type { Plugin } from '../types/plugin';

const { Option } = Select;
const { Text } = Typography;

interface PluginDetailModalProps {
  visible: boolean;
  plugin: Plugin | null;
  onClose: () => void;
  onUpdate: () => void;
}

interface PluginLog {
  id: string;
  level: string;
  message: string;
  timestamp: Date;
  source: string;
}

interface PluginStats {
  name: string;
  status: string;
  uptime: number;
  dataPointsCount: number;
  errorCount: number;
  lastActivity: Date;
  memoryUsage: number;
  cpuUsage: number;
  networkRx: number;
  networkTx: number;
  customMetrics: Record<string, any>;
}

export const PluginDetailModal: React.FC<PluginDetailModalProps> = ({
  visible,
  plugin,
  onClose,
  onUpdate,
}) => {
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState('info');
  const [config, setConfig] = useState<any>(null);
  const [configEditing, setConfigEditing] = useState(false);
  const [logs, setLogs] = useState<PluginLog[]>([]);
  const [stats, setStats] = useState<PluginStats | null>(null);
  const [configForm] = Form.useForm();

  // 获取插件配置
  const fetchConfig = async () => {
    if (!plugin) return;
    try {
      const configData = await pluginService.getPluginConfig(String(plugin.id));
      setConfig(configData);
      configForm.setFieldsValue(configData);
    } catch (error: any) {
      message.error('获取插件配置失败: ' + error.message);
    }
  };

  // 获取插件日志
  const fetchLogs = async () => {
    if (!plugin) return;
    try {
      const response = await pluginService.getPluginLogs(String(plugin.id), {
        page: 1,
        page_size: 50,
        level: '',
      });
      // 转换日志数据类型
      const convertedLogs = response.data.map((log: any) => ({
        ...log,
        id: String(log.id),
      }));
      setLogs(convertedLogs);
    } catch (error: any) {
      message.error('获取插件日志失败: ' + error.message);
    }
  };

  // 获取插件统计
  const fetchStats = async () => {
    if (!plugin) return;
    try {
      const statsData = await pluginService.getPluginStats(String(plugin.id));
      // 类型转换以确保兼容性
      setStats(statsData as any);
    } catch (error: any) {
      message.error('获取插件统计失败: ' + error.message);
    }
  };

  // 启动插件
  const handleStart = async () => {
    if (!plugin) return;
    try {
      setLoading(true);
      await pluginService.startPlugin(String(plugin.id));
      message.success('插件启动成功');
      onUpdate();
      await fetchStats();
    } catch (error: any) {
      message.error('启动插件失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  // 停止插件
  const handleStop = async () => {
    if (!plugin) return;
    try {
      setLoading(true);
      await pluginService.stopPlugin(String(plugin.id));
      message.success('插件停止成功');
      onUpdate();
      await fetchStats();
    } catch (error: any) {
      message.error('停止插件失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  // 重启插件
  const handleRestart = async () => {
    if (!plugin) return;
    try {
      setLoading(true);
      await pluginService.restartPlugin(String(plugin.id));
      message.success('插件重启成功');
      onUpdate();
      await fetchStats();
    } catch (error: any) {
      message.error('重启插件失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  // 保存配置
  const handleSaveConfig = async () => {
    if (!plugin) return;
    try {
      const values = await configForm.validateFields();
      setLoading(true);
      
      // 验证配置
      const validation = await pluginService.validatePluginConfig(String(plugin.id), values);
      if (!validation.valid) {
        message.error('配置验证失败: ' + (validation.errors || []).join(', '));
        return;
      }

      // 保存配置
      await pluginService.updatePluginConfig(String(plugin.id), values);
      message.success('配置保存成功');
      setConfigEditing(false);
      setConfig(values);
    } catch (error: any) {
      message.error('保存配置失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  // 格式化时间
  const formatDuration = (seconds: number) => {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return `${hours}小时 ${minutes}分钟`;
  };

  // 格式化文件大小
  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
  };

  // 获取状态颜色
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running':
        return 'success';
      case 'stopped':
        return 'default';
      case 'error':
        return 'error';
      default:
        return 'warning';
    }
  };

  // 获取日志级别颜色
  const getLogLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'error':
        return 'red';
      case 'warn':
      case 'warning':
        return 'orange';
      case 'info':
        return 'blue';
      case 'debug':
        return 'gray';
      default:
        return 'default';
    }
  };

  // 日志表格列
  const logColumns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 150,
      render: (time: Date) => new Date(time).toLocaleString(),
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      width: 80,
      render: (level: string) => (
        <Tag color={getLogLevelColor(level)}>{level.toUpperCase()}</Tag>
      ),
    },
    {
      title: '消息',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
    },
  ];

  // 初始化数据
  useEffect(() => {
    if (visible && plugin) {
      fetchConfig();
      fetchLogs();
      fetchStats();
    }
  }, [visible, plugin]);

  if (!plugin) return null;

  const tabItems = [
    {
      key: 'info',
      label: (
        <span>
          <SettingOutlined />
          基本信息
        </span>
      ),
      children: (
        <Card>
          <Descriptions column={2} bordered>
            <Descriptions.Item label="插件名称">{plugin.name}</Descriptions.Item>
            <Descriptions.Item label="版本">{plugin.version}</Descriptions.Item>
            <Descriptions.Item label="类型">
              <Tag color={plugin.type === 'adapter' ? 'blue' : 'green'}>
                {plugin.type}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={getStatusColor(plugin.status)} icon={
                plugin.status === 'running' ? <CheckCircleOutlined /> :
                plugin.status === 'error' ? <CloseCircleOutlined /> :
                <ExclamationCircleOutlined />
              }>
                {plugin.status}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="描述" span={2}>
              {plugin.description || '暂无描述'}
            </Descriptions.Item>
          </Descriptions>

          <Divider />

          <Space>
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              onClick={handleStart}
              loading={loading}
              disabled={plugin.status === 'running'}
            >
              启动
            </Button>
            <Button
              icon={<PauseCircleOutlined />}
              onClick={handleStop}
              loading={loading}
              disabled={plugin.status !== 'running'}
            >
              停止
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={handleRestart}
              loading={loading}
            >
              重启
            </Button>
          </Space>
        </Card>
      ),
    },
    {
      key: 'config',
      label: (
        <span>
          <EditOutlined />
          配置管理
        </span>
      ),
      children: (
        <Card
          title="插件配置"
          extra={
            <Space>
              {configEditing ? (
                <>
                  <Button onClick={() => setConfigEditing(false)}>取消</Button>
                  <Button
                    type="primary"
                    icon={<SaveOutlined />}
                    onClick={handleSaveConfig}
                    loading={loading}
                  >
                    保存
                  </Button>
                </>
              ) : (
                <Button
                  icon={<EditOutlined />}
                  onClick={() => setConfigEditing(true)}
                >
                  编辑
                </Button>
              )}
            </Space>
          }
        >
          {config ? (
            configEditing ? (
              <Form form={configForm} layout="vertical">
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item label="插件名称" name="name">
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="版本" name="version">
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="类型" name="type">
                      <Select>
                        <Option value="adapter">Adapter</Option>
                        <Option value="sink">Sink</Option>
                      </Select>
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="模式" name="mode">
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col span={24}>
                    <Form.Item label="入口点" name="entry">
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col span={24}>
                    <Form.Item label="描述" name="description">
                      <Input.TextArea rows={3} />
                    </Form.Item>
                  </Col>
                </Row>
              </Form>
            ) : (
              <Editor
                height="400px"
                language="json"
                value={JSON.stringify(config, null, 2)}
                options={{
                  readOnly: true,
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                }}
              />
            )
          ) : (
            <Spin />
          )}
        </Card>
      ),
    },
    {
      key: 'logs',
      label: (
        <span>
          <FileTextOutlined />
          运行日志
        </span>
      ),
      children: (
        <Card
          title="运行日志"
          extra={
            <Button icon={<ReloadOutlined />} onClick={fetchLogs}>
              刷新
            </Button>
          }
        >
          <Table
            columns={logColumns}
            dataSource={logs}
            pagination={{ pageSize: 10 }}
            size="small"
            rowKey="id"
          />
        </Card>
      ),
    },
    {
      key: 'stats',
      label: (
        <span>
          <BarChartOutlined />
          性能统计
        </span>
      ),
      children: (
        <div>
          {stats ? (
            <>
              <Row gutter={[16, 16]}>
                <Col xs={24} sm={12} lg={6}>
                  <Card size="small">
                    <Statistic
                      title="运行时间"
                      value={formatDuration(stats.uptime / 1000)}
                      valueStyle={{ fontSize: '16px' }}
                    />
                  </Card>
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Card size="small">
                    <Statistic
                      title="处理数据点"
                      value={stats.dataPointsCount}
                      valueStyle={{ fontSize: '16px' }}
                    />
                  </Card>
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Card size="small">
                    <Statistic
                      title="错误次数"
                      value={stats.errorCount}
                      valueStyle={{ 
                        fontSize: '16px',
                        color: stats.errorCount > 0 ? '#f5222d' : '#52c41a'
                      }}
                    />
                  </Card>
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Card size="small">
                    <Statistic
                      title="最后活动"
                      value={new Date(stats.lastActivity).toLocaleTimeString()}
                      valueStyle={{ fontSize: '16px' }}
                    />
                  </Card>
                </Col>
              </Row>

              <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
                <Col xs={24} md={12}>
                  <Card title="资源使用" size="small">
                    <div style={{ marginBottom: 16 }}>
                      <Text>CPU 使用率: {stats.cpuUsage.toFixed(1)}%</Text>
                      <Progress
                        percent={Math.round(stats.cpuUsage)}
                        status={stats.cpuUsage > 80 ? 'exception' : 'active'}
                        size="small"
                      />
                    </div>
                    <div>
                      <Text>内存使用: {stats.memoryUsage.toFixed(1)} MB</Text>
                      <Progress
                        percent={Math.min(Math.round(stats.memoryUsage), 100)}
                        status={stats.memoryUsage > 100 ? 'exception' : 'active'}
                        size="small"
                      />
                    </div>
                  </Card>
                </Col>
                <Col xs={24} md={12}>
                  <Card title="网络流量" size="small">
                    <Row gutter={16}>
                      <Col span={12}>
                        <Statistic
                          title="接收"
                          value={formatBytes(stats.networkRx)}
                          valueStyle={{ fontSize: '14px' }}
                        />
                      </Col>
                      <Col span={12}>
                        <Statistic
                          title="发送"
                          value={formatBytes(stats.networkTx)}
                          valueStyle={{ fontSize: '14px' }}
                        />
                      </Col>
                    </Row>
                  </Card>
                </Col>
              </Row>

              {Object.keys(stats.customMetrics).length > 0 && (
                <Card title="自定义指标" size="small" style={{ marginTop: 16 }}>
                  <Row gutter={16}>
                    {Object.entries(stats.customMetrics).map(([key, value]) => (
                      <Col xs={24} sm={12} lg={6} key={key}>
                        <Statistic
                          title={key}
                          value={typeof value === 'number' ? value.toFixed(2) : String(value)}
                          valueStyle={{ fontSize: '14px' }}
                        />
                      </Col>
                    ))}
                  </Row>
                </Card>
              )}
            </>
          ) : (
            <Spin />
          )}
        </div>
      ),
    },
  ];

  return (
    <Modal
      title={`插件详情 - ${plugin.name}`}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={800}
      style={{ top: 20 }}
    >
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={tabItems}
      />
    </Modal>
  );
};