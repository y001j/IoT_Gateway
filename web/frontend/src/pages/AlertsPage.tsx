import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import {
  Table,
  Card,
  Button,
  Tag,
  Input,
  Select,

  Modal,
  Form,
  Typography,
  message,
  Popconfirm,
  Tooltip,
  Tabs,
  Row,
  Col,
  Space,
  Badge,
  Divider
} from 'antd';
import {
  AlertOutlined,
  PlusOutlined,
  ReloadOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  CloseCircleOutlined,
  BellOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { alertService } from '../services/alertService';
import { useRealTimeData } from '../hooks/useRealTimeData';
import { useSmartCache } from '../hooks/useSmartCache';
import { RealTimeAlertNotification } from '../components/alerts/RealTimeAlertNotification';
import { AlertStatsDashboard } from '../components/alerts/AlertStatsDashboard';
import { useAuthStore } from '../store/authStore';
import type { 
  Alert,
  AlertRule,
  AlertCreateRequest,
  AlertRuleCreateRequest,
  NotificationChannel,
  NotificationChannelCreateRequest,
  AlertListRequest
} from '../types/alert';

// Constants that were imported as types
const ALERT_LEVELS = [
  { value: 'info', label: '信息', color: 'blue' },
  { value: 'warning', label: '警告', color: 'orange' },
  { value: 'error', label: '错误', color: 'red' },
  { value: 'critical', label: '严重', color: 'purple' }
];

const ALERT_STATUSES = [
  { value: 'active', label: '活跃', color: 'red' },
  { value: 'acknowledged', label: '已确认', color: 'orange' },
  { value: 'resolved', label: '已解决', color: 'green' }
];

const NOTIFICATION_CHANNEL_TYPES = [
  { value: 'email', label: '邮件', icon: <BellOutlined /> },
  { value: 'webhook', label: 'Webhook', icon: <BellOutlined /> },
  { value: 'sms', label: '短信', icon: <BellOutlined /> },
  { value: 'slack', label: 'Slack', icon: <BellOutlined /> },
  { value: 'dingtalk', label: '钉钉', icon: <BellOutlined /> }
];

const { Text } = Typography;
const { Option } = Select;

export const AlertsPage: React.FC = () => {
  const [activeTab, setActiveTab] = useState('alerts');
  const [loading, setLoading] = useState(false);
  const { isAuthenticated, isInitialized, initialize } = useAuthStore();
  
  // 告警列表状态
  const [alertFilters, setAlertFilters] = useState<AlertListRequest>({
    page: 1,
    pageSize: 20,
  });
  
  // 稳定的缓存key
  const alertsCacheKey = useMemo(() => 
    `alerts-${JSON.stringify(alertFilters)}`, 
    [alertFilters]
  );
  
  // 稳定的fetch函数
  const fetchAlerts = useCallback(() => {
    console.log('📡 AlertsPage: 调用 alertService.getAlerts, filters:', alertFilters);
    return alertService.getAlerts(alertFilters);
  }, [alertFilters]);
  
  // 使用智能缓存获取告警列表
  const {
    data: alertsData,
    loading: alertsLoading,
    error: alertsError,
    isStale: alertsStale,
    refresh: refreshAlerts,
  } = useSmartCache(
    alertsCacheKey,
    fetchAlerts,
    { ttl: 2 * 60 * 1000, staleWhileRevalidate: 30 * 1000 } // 2分钟TTL，30秒过期可用
  );
  
  // 稳定的fetch函数
  const fetchAlertRules = useCallback(() => alertService.getAlertRules(), []);
  const fetchNotificationChannels = useCallback(() => alertService.getNotificationChannels(), []);
  const fetchAlertStats = useCallback(() => alertService.getAlertStats(), []);

  // 使用智能缓存获取告警规则
  const {
    data: alertRules,
    loading: rulesLoading,
    refresh: refreshRules,
  } = useSmartCache(
    'alert-rules',
    fetchAlertRules,
    { ttl: 10 * 60 * 1000 } // 10分钟TTL
  );
  
  // 使用智能缓存获取通知渠道
  const {
    data: notificationChannels,
    loading: channelsLoading,
    refresh: refreshChannels,
  } = useSmartCache(
    'notification-channels',
    fetchNotificationChannels,
    { ttl: 10 * 60 * 1000 } // 10分钟TTL
  );
  
  // 从缓存数据中提取告警列表
  const alerts = alertsData?.alerts || [];
  const alertTotal = alertsData?.total || 0;
  
  // 确保数据不为null，并提供空数组作为默认值
  const safeAlertRules = Array.isArray(alertRules) ? alertRules : [];
  const safeNotificationChannels = Array.isArray(notificationChannels) ? notificationChannels : [];
  
  // 获取告警统计
  const {
    data: alertStats,
    loading: statsLoading,
  } = useSmartCache(
    'alert-stats',
    fetchAlertStats,
    { ttl: 1 * 60 * 1000 } // 1分钟TTL
  );
  
  // 弹窗状态
  const [createAlertModalVisible, setCreateAlertModalVisible] = useState(false);
  const [createRuleModalVisible, setCreateRuleModalVisible] = useState(false);
  const [createChannelModalVisible, setCreateChannelModalVisible] = useState(false);
  
  // 表单
  const [createAlertForm] = Form.useForm();
  const [createRuleForm] = Form.useForm();
  const [createChannelForm] = Form.useForm();
  
  // 实时数据
  const { data: realTimeData } = useRealTimeData();
  
  // 防抖定时器
  const refreshTimerRef = useRef<NodeJS.Timeout | null>(null);


  // 初始化认证状态
  useEffect(() => {
    console.log('🔍 AlertsPage 认证状态:', { isInitialized, isAuthenticated });
    if (!isInitialized) {
      console.log('🚀 AlertsPage 初始化认证状态...');
      initialize();
    }
  }, [isInitialized, initialize, isAuthenticated]);


  // 根据当前标签设置loading状态
  useEffect(() => {
    if (activeTab === 'alerts') {
      setLoading(alertsLoading);
    } else if (activeTab === 'rules') {
      setLoading(rulesLoading);
    } else if (activeTab === 'channels') {
      setLoading(channelsLoading);
    }
  }, [activeTab, alertsLoading, rulesLoading, channelsLoading]);

  // 核心数据加载状态检查 - 将在JSX中处理，不在此处提前返回

  // 防抖刷新函数
  const debouncedRefresh = useCallback(() => {
    if (refreshTimerRef.current) {
      clearTimeout(refreshTimerRef.current);
    }
    
    refreshTimerRef.current = setTimeout(() => {
      if (activeTab === 'alerts') {
        refreshAlerts();
      }
    }, 1000); // 1秒防抖
  }, [activeTab, refreshAlerts]);

  // 处理实时告警数据
  useEffect(() => {
    if (realTimeData && realTimeData.alerts && realTimeData.alerts.length > 0 && activeTab === 'alerts') {
      // 使用防抖刷新，避免频繁请求
      debouncedRefresh();
    }
  }, [realTimeData?.alerts, activeTab, debouncedRefresh]);

  // 清理定时器
  useEffect(() => {
    return () => {
      if (refreshTimerRef.current) {
        clearTimeout(refreshTimerRef.current);
      }
    };
  }, []);

  // 显示错误消息
  useEffect(() => {
    if (alertsError) {
      message.error('获取告警列表失败: ' + alertsError.message);
    }
  }, [alertsError]);

  // 调试日志
  useEffect(() => {
    console.log('AlertsPage 数据状态:', {
      isInitialized,
      isAuthenticated,
      alertsLoading,
      alertsError: alertsError?.message,
      alertsCount: alertsData?.alerts?.length || 0,
      rulesCount: alertRules?.length || 0,
      channelsCount: notificationChannels?.length || 0,
      activeTab
    });
  }, [isInitialized, isAuthenticated, alertsData, alertRules, notificationChannels, alertsLoading, alertsError, activeTab]);

  // 确认告警
  const handleAcknowledgeAlert = async (alertId: string) => {
    try {
      await alertService.acknowledgeAlert(alertId, '手动确认');
      message.success('告警确认成功');
      refreshAlerts();
    } catch (error: any) {
      message.error('确认告警失败: ' + error.message);
    }
  };

  // 解决告警
  const handleResolveAlert = async (alertId: string) => {
    try {
      await alertService.resolveAlert(alertId, '手动解决');
      message.success('告警解决成功');
      refreshAlerts();
    } catch (error: any) {
      message.error('解决告警失败: ' + error.message);
    }
  };

  // 删除告警
  const handleDeleteAlert = async (alertId: string) => {
    try {
      await alertService.deleteAlert(alertId);
      message.success('告警删除成功');
      refreshAlerts();
    } catch (error: any) {
      message.error('删除告警失败: ' + error.message);
    }
  };

  // 创建告警
  const handleCreateAlert = async () => {
    try {
      const values = await createAlertForm.validateFields();
      const alertData: AlertCreateRequest = {
        title: values.title,
        description: values.description,
        level: values.level,
        source: values.source || 'manual',
        data: values.data ? JSON.parse(values.data) : {},
      };
      
      await alertService.createAlert(alertData);
      message.success('告警创建成功');
      setCreateAlertModalVisible(false);
      createAlertForm.resetFields();
      refreshAlerts();
    } catch (error: any) {
      message.error('创建告警失败: ' + error.message);
    }
  };

  // 创建告警规则
  const handleCreateAlertRule = async () => {
    try {
      const values = await createRuleForm.validateFields();
      const ruleData: AlertRuleCreateRequest = {
        name: values.name,
        description: values.description,
        enabled: values.enabled,
        level: values.level,
        condition: {
          type: values.conditionType,
          field: values.field,
          operator: values.operator,
          value: values.value,
        },
        notificationChannels: values.notificationChannels || [],
      };
      
      await alertService.createAlertRule(ruleData);
      message.success('告警规则创建成功');
      setCreateRuleModalVisible(false);
      createRuleForm.resetFields();
      refreshRules();
    } catch (error: any) {
      message.error('创建告警规则失败: ' + error.message);
    }
  };

  // 创建通知渠道
  const handleCreateNotificationChannel = async () => {
    try {
      const values = await createChannelForm.validateFields();
      const channelData: NotificationChannelCreateRequest = {
        name: values.name,
        type: values.type,
        enabled: values.enabled,
        config: values.config ? JSON.parse(values.config) : {},
      };
      
      await alertService.createNotificationChannel(channelData);
      message.success('通知渠道创建成功');
      setCreateChannelModalVisible(false);
      createChannelForm.resetFields();
      refreshChannels();
    } catch (error: any) {
      message.error('创建通知渠道失败: ' + error.message);
    }
  };

  // 测试通知渠道
  const handleTestChannel = async (channelId: string) => {
    try {
      await alertService.testNotificationChannel(channelId);
      message.success('测试通知发送成功');
    } catch (error: any) {
      message.error('测试通知失败: ' + error.message);
    }
  };

  // 告警表格列
  const alertColumns = [
    {
      title: '标题',
      dataIndex: 'title',
      key: 'title',
      ellipsis: true,
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      width: 80,
      render: (level: string) => (
        <Tag color={alertService.getAlertLevelColor(level)}>
          {alertService.getAlertLevelText(level)}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Tag 
          color={alertService.getAlertStatusColor(status)}
          icon={
            status === 'active' ? <ExclamationCircleOutlined /> :
            status === 'acknowledged' ? <CheckCircleOutlined /> :
            <CloseCircleOutlined />
          }
        >
          {alertService.getAlertStatusText(status)}
        </Tag>
      ),
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      width: 120,
      render: (source: string, record: Alert) => (
        <div>
          <div>{source}</div>
          {record.rule_name && (
            <div style={{ fontSize: '12px', color: '#666' }}>
              规则: {record.rule_name}
            </div>
          )}
        </div>
      ),
    },
    {
      title: '设备/键值',
      key: 'device_key',
      width: 150,
      render: (_: any, record: Alert) => (
        <div>
          {record.device_id && (
            <div style={{ fontSize: '12px', color: '#666' }}>
              设备: {record.device_id}
            </div>
          )}
          {record.key && (
            <div style={{ fontSize: '12px', color: '#666' }}>
              键: {record.key}
            </div>
          )}
          {record.value && (
            <div style={{ fontSize: '12px', color: '#666' }}>
              值: {JSON.stringify(record.value)}
            </div>
          )}
        </div>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 160,
      render: (time: Date | string) => {
        try {
          const date = time instanceof Date ? time : new Date(time);
          return isNaN(date.getTime()) ? '无效时间' : date.toLocaleString('zh-CN');
        } catch {
          return '无效时间';
        }
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_: any, record: Alert) => (
        <Space size="small">
          {record.status === 'active' && (
            <Tooltip title="确认告警">
              <Button
                size="small"
                icon={<CheckCircleOutlined />}
                onClick={() => handleAcknowledgeAlert(record.id)}
              >
                确认
              </Button>
            </Tooltip>
          )}
          {(record.status === 'active' || record.status === 'acknowledged') && (
            <Tooltip title="解决告警">
              <Button
                size="small"
                type="primary"
                icon={<CloseCircleOutlined />}
                onClick={() => handleResolveAlert(record.id)}
              >
                解决
              </Button>
            </Tooltip>
          )}
          <Popconfirm
            title="确定要删除这个告警吗？"
            onConfirm={() => handleDeleteAlert(record.id)}
          >
            <Button size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  // 告警规则表格列
  const ruleColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      width: 80,
      render: (level: string) => (
        <Tag color={alertService.getAlertLevelColor(level)}>
          {alertService.getAlertLevelText(level)}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      render: (enabled: boolean) => (
        <Badge status={enabled ? 'success' : 'default'} text={enabled ? '启用' : '禁用'} />
      ),
    },
    {
      title: '条件',
      key: 'condition',
      render: (_: any, record: AlertRule) => (
        <Text code>
          {record.condition.field} {record.condition.operator} {record.condition.value}
        </Text>
      ),
    },
    {
      title: '通知渠道',
      dataIndex: 'notificationChannels',
      key: 'notificationChannels',
      render: (channels: string[]) => (
        <span>{channels?.length || 0} 个渠道</span>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_: any) => (
        <Space size="small">
          <Button size="small">测试</Button>
          <Button size="small">编辑</Button>
          <Button size="small" danger>删除</Button>
        </Space>
      ),
    },
  ];

  // 通知渠道表格列
  const channelColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => {
        const channelType = NOTIFICATION_CHANNEL_TYPES.find(t => t.value === type);
        return (
          <span>
            {channelType?.icon} {channelType?.label || type}
          </span>
        );
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      render: (enabled: boolean) => (
        <Badge status={enabled ? 'success' : 'default'} text={enabled ? '启用' : '禁用'} />
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 160,
      render: (time: Date | string) => {
        try {
          const date = time instanceof Date ? time : new Date(time);
          return isNaN(date.getTime()) ? '无效时间' : date.toLocaleString('zh-CN');
        } catch {
          return '无效时间';
        }
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_: any, record: NotificationChannel) => (
        <Space size="small">
          <Button 
            size="small" 
            icon={<BellOutlined />}
            onClick={() => handleTestChannel(record.id)}
          >
            测试
          </Button>
          <Button size="small">编辑</Button>
          <Button size="small" danger>删除</Button>
        </Space>
      ),
    },
  ];

  const tabItems = [
    {
      key: 'alerts',
      label: (
        <span>
          <AlertOutlined />
          告警列表
          {alerts.filter(a => a.status === 'active').length > 0 && (
            <Badge count={alerts.filter(a => a.status === 'active').length} style={{ marginLeft: 8 }} />
          )}
        </span>
      ),
      children: (
        <div>
          {/* 告警统计仪表板 */}
          <AlertStatsDashboard stats={alertStats} loading={statsLoading} />
          
          {/* 过滤器 */}
          <Card size="small" style={{ marginBottom: 16 }}>
            <Row gutter={16}>
              <Col span={4}>
                <Select
                  placeholder="告警级别"
                  allowClear
                  value={alertFilters.level}
                  onChange={(value) => setAlertFilters({ ...alertFilters, level: value, page: 1 })}
                >
                  {ALERT_LEVELS.map(level => (
                    <Option key={level.value} value={level.value}>
                      <Tag color={level.color}>{level.label}</Tag>
                    </Option>
                  ))}
                </Select>
              </Col>
              <Col span={4}>
                <Select
                  placeholder="告警状态"
                  allowClear
                  value={alertFilters.status}
                  onChange={(value) => setAlertFilters({ ...alertFilters, status: value, page: 1 })}
                >
                  {ALERT_STATUSES.map(status => (
                    <Option key={status.value} value={status.value}>
                      <Tag color={status.color}>{status.label}</Tag>
                    </Option>
                  ))}
                </Select>
              </Col>
              <Col span={4}>
                <Input
                  placeholder="来源筛选"
                  allowClear
                  value={alertFilters.source}
                  onChange={(e) => setAlertFilters({ ...alertFilters, source: e.target.value, page: 1 })}
                />
              </Col>
              <Col span={6}>
                <Input.Search
                  placeholder="搜索告警"
                  allowClear
                  value={alertFilters.search}
                  onChange={(e) => setAlertFilters({ ...alertFilters, search: e.target.value, page: 1 })}
                  onSearch={() => refreshAlerts()}
                />
              </Col>
              <Col span={6}>
                <Space>
                  <Button 
                    icon={<ReloadOutlined />} 
                    onClick={refreshAlerts}
                    loading={alertsLoading}
                    type={alertsStale ? 'primary' : 'default'}
                  >
                    {alertsStale ? '数据过期，点击刷新' : '刷新'}
                  </Button>
                  <Button 
                    type="primary" 
                    icon={<PlusOutlined />}
                    onClick={() => setCreateAlertModalVisible(true)}
                  >
                    手动创建告警
                  </Button>
                </Space>
              </Col>
            </Row>
          </Card>

          {/* 告警表格 */}
          <Card>
            <Table
              columns={alertColumns}
              dataSource={alerts}
              rowKey="id"
              loading={loading}
              pagination={{
                current: alertFilters.page,
                pageSize: alertFilters.pageSize,
                total: alertTotal,
                onChange: (page, pageSize) => setAlertFilters({ ...alertFilters, page, pageSize }),
                showSizeChanger: true,
                showQuickJumper: true,
                showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`,
              }}
            />
          </Card>
        </div>
      ),
    },
    {
      key: 'rules',
      label: (
        <span>
          <SettingOutlined />
          告警规则
        </span>
      ),
      children: (
        <Card>
          <div style={{ marginBottom: 16 }}>
            <Space>
              <Button icon={<ReloadOutlined />} onClick={refreshRules}>
                刷新
              </Button>
              <Button 
                type="primary" 
                icon={<PlusOutlined />}
                onClick={() => setCreateRuleModalVisible(true)}
              >
                创建规则
              </Button>
            </Space>
          </div>
          
          <Table
            columns={ruleColumns}
            dataSource={safeAlertRules}
            rowKey="id"
            loading={loading}
            pagination={{ pageSize: 10 }}
          />
        </Card>
      ),
    },
    {
      key: 'channels',
      label: (
        <span>
          <BellOutlined />
          通知渠道
        </span>
      ),
      children: (
        <Card>
          <div style={{ marginBottom: 16 }}>
            <Space>
              <Button icon={<ReloadOutlined />} onClick={refreshChannels}>
                刷新
              </Button>
              <Button 
                type="primary" 
                icon={<PlusOutlined />}
                onClick={() => setCreateChannelModalVisible(true)}
              >
                创建渠道
              </Button>
            </Space>
          </div>
          
          <Table
            columns={channelColumns}
            dataSource={safeNotificationChannels}
            rowKey="id"
            loading={loading}
            pagination={{ pageSize: 10 }}
          />
        </Card>
      ),
    },
  ];

  // 如果认证状态未初始化，显示加载状态
  if (!isInitialized) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>初始化中...</div>;
  }

  // 如果未认证，显示登录提示
  if (!isAuthenticated) {
    return (
      <div style={{ padding: '20px', textAlign: 'center' }}>
        <p>请先登录系统</p>
        <Button type="primary" onClick={() => window.location.href = '/login'}>
          去登录
        </Button>
      </div>
    );
  }

  // 调试：打印当前状态
  console.log('🎯 AlertsPage 渲染状态:', {
    isInitialized,
    isAuthenticated,
    alertsLoading,
    hasAlertsData: !!alertsData,
    alertsError: alertsError?.message
  });

  // 如果初次加载且没有缓存数据，显示加载状态
  if (alertsLoading && !alertsData && !alertsError) {
    console.log('⏳ AlertsPage: 显示加载中状态');
    return <div style={{ padding: '20px', textAlign: 'center' }}>加载中...</div>;
  }

  // 如果有错误且没有缓存数据，显示错误状态
  if (alertsError && !alertsData) {
    const isAuthError = alertsError.message.includes('401') || 
                       alertsError.message.includes('unauthorized') ||
                       alertsError.message.includes('未提供认证令牌');
    
    return (
      <div style={{ padding: '20px', textAlign: 'center' }}>
        <p>加载失败: {alertsError.message}</p>
        {isAuthError ? (
          <div>
            <p>请先登录系统</p>
            <Button type="primary" onClick={() => window.location.href = '/login'}>
              去登录
            </Button>
          </div>
        ) : (
          <Button onClick={() => window.location.reload()}>重新加载</Button>
        )}
      </div>
    );
  }



  return (
    <div>
      {/* 实时告警通知 */}
      <RealTimeAlertNotification enabled={true} position="topRight" />
      
      
      <div style={{ marginBottom: 24 }}>
        <h2>告警管理</h2>
        <Text type="secondary">管理系统告警、告警规则和通知渠道</Text>
      </div>

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={tabItems}
      />

      {/* 创建告警弹窗 */}
      <Modal
        title="创建告警"
        open={createAlertModalVisible}
        onOk={handleCreateAlert}
        onCancel={() => setCreateAlertModalVisible(false)}
        width={600}
      >
        <Form form={createAlertForm} layout="vertical">
          <Form.Item name="title" label="告警标题" rules={[{ required: true }]}>
            <Input placeholder="请输入告警标题" />
          </Form.Item>
          <Form.Item name="description" label="告警描述" rules={[{ required: true }]}>
            <Input.TextArea rows={3} placeholder="请输入告警描述" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="level" label="告警级别" rules={[{ required: true }]}>
                <Select placeholder="选择告警级别">
                  {ALERT_LEVELS.map(level => (
                    <Option key={level.value} value={level.value}>
                      <Tag color={level.color}>{level.label}</Tag>
                    </Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="source" label="告警来源">
                <Input placeholder="请输入告警来源" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="data" label="附加数据">
            <Input.TextArea 
              rows={3} 
              placeholder="请输入JSON格式的附加数据（可选）" 
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* 创建告警规则弹窗 */}
      <Modal
        title="创建告警规则"
        open={createRuleModalVisible}
        onOk={handleCreateAlertRule}
        onCancel={() => setCreateRuleModalVisible(false)}
        width={700}
      >
        <Form form={createRuleForm} layout="vertical">
          <Form.Item name="name" label="规则名称" rules={[{ required: true }]}>
            <Input placeholder="请输入规则名称" />
          </Form.Item>
          <Form.Item name="description" label="规则描述">
            <Input.TextArea rows={2} placeholder="请输入规则描述" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="level" label="告警级别" rules={[{ required: true }]}>
                <Select placeholder="选择告警级别">
                  {ALERT_LEVELS.map(level => (
                    <Option key={level.value} value={level.value}>
                      <Tag color={level.color}>{level.label}</Tag>
                    </Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="enabled" label="启用状态" initialValue={true}>
                <Select>
                  <Option value={true}>启用</Option>
                  <Option value={false}>禁用</Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Divider>告警条件</Divider>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="field" label="字段名" rules={[{ required: true }]}>
                <Input placeholder="如: temperature" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="operator" label="操作符" rules={[{ required: true }]}>
                <Select placeholder="选择操作符">
                  <Option value="gt">大于 (&gt;)</Option>
                  <Option value="lt">小于 (&lt;)</Option>
                  <Option value="eq">等于 (=)</Option>
                  <Option value="gte">大于等于 (≥)</Option>
                  <Option value="lte">小于等于 (≤)</Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="value" label="阈值" rules={[{ required: true }]}>
                <Input placeholder="如: 80" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="notificationChannels" label="通知渠道">
            <Select mode="multiple" placeholder="选择通知渠道（可选）">
              {safeNotificationChannels.map(channel => (
                <Option key={channel.id} value={channel.id}>
                  {channel.name}
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* 创建通知渠道弹窗 */}
      <Modal
        title="创建通知渠道"
        open={createChannelModalVisible}
        onOk={handleCreateNotificationChannel}
        onCancel={() => setCreateChannelModalVisible(false)}
        width={600}
      >
        <Form form={createChannelForm} layout="vertical">
          <Form.Item name="name" label="渠道名称" rules={[{ required: true }]}>
            <Input placeholder="请输入渠道名称" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="type" label="渠道类型" rules={[{ required: true }]}>
                <Select placeholder="选择渠道类型">
                  {NOTIFICATION_CHANNEL_TYPES.map(type => (
                    <Option key={type.value} value={type.value}>
                      {type.icon} {type.label}
                    </Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="enabled" label="启用状态" initialValue={true}>
                <Select>
                  <Option value={true}>启用</Option>
                  <Option value={false}>禁用</Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="config" label="渠道配置" rules={[{ required: true }]}>
            <Input.TextArea 
              rows={6} 
              placeholder="请输入JSON格式的渠道配置，例如：&#10;{&#10;  &quot;url&quot;: &quot;https://hooks.slack.com/...&quot;,&#10;  &quot;channel&quot;: &quot;#alerts&quot;&#10;}" 
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};