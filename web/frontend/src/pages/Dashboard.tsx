import React, { useState, useEffect } from 'react';
import {
  Card,
  Row,
  Col,
  Typography,
  Spin,
  Tag,
  Button,
  Space,
  Badge,
  Statistic,
  Progress,
  Divider,
  List,
  Alert as AntAlert,
  Tabs
} from 'antd';
import {
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  MonitorOutlined,
  ApiOutlined,
  ThunderboltOutlined,
  WarningOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  LineChartOutlined
} from '@ant-design/icons';
import { useAuthStore } from '../store/authStore';
import { pluginService } from '../services/pluginService';
import { alertService } from '../services/alertService';
import { SystemMonitorChart } from '../components/charts/SystemMonitorChart';
import { IoTDataChart } from '../components/charts/IoTDataChart';
import { useRealTimeData } from '../hooks/useRealTimeData';
import type { Plugin } from '../types/plugin';
import type { Alert } from '../types/alert';
import { authService } from '../services/authService';

const { Title, Text } = Typography;

interface SystemStats {
  uptime: string;
  activeDevices: number;
  dataPoints: number;
  errorRate: number;
  cpuUsage: number;
  memoryUsage: number;
}


const Dashboard: React.FC = () => {
  console.log('Dashboard组件开始渲染');
  
  const { user } = useAuthStore();
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [stats, setStats] = useState<SystemStats>({
    uptime: '0h 0m',
    activeDevices: 0,
    dataPoints: 0,
    errorRate: 0,
    cpuUsage: 0,
    memoryUsage: 0
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [recentAlerts, setRecentAlerts] = useState<Alert[]>([]);
  
  // 使用实时数据Hook
  const { data: realtimeData, isConnected, connectionState, reconnect } = useRealTimeData();

  console.log('Dashboard状态:', {
    user,
    pluginsCount: plugins.length,
    recentAlertsCount: recentAlerts.length,
    loading,
    error,
    connectionState,
    isConnected,
    realtimeDataKeys: Object.keys(realtimeData || {}),
    systemStatus: realtimeData?.systemStatus,
    systemMetrics: realtimeData?.systemMetrics,
    systemMetricsHistory: realtimeData?.systemMetricsHistory?.length || 0,
    iotDataCount: realtimeData?.iotData?.length || 0,
    dataRecovery: {
      hasSystemStatus: !!realtimeData?.systemStatus,
      hasSystemMetrics: !!realtimeData?.systemMetrics,
      hasHistoryData: (realtimeData?.systemMetricsHistory?.length || 0) > 0,
      hasIoTData: (realtimeData?.iotData?.length || 0) > 0,
    }
  });

  const fetchPlugins = async () => {
    try {
      const response = await pluginService.getPlugins({ page: 1, page_size: 50 });
      setPlugins(response.data || []);
    } catch (error) {
      console.error('获取插件列表失败:', error);
    }
  };

  const fetchStats = async () => {
    // 从实时数据更新统计信息
    if (realtimeData?.systemStatus) {
      console.log('处理系统状态数据:', realtimeData.systemStatus);
      
      const systemStatus = realtimeData.systemStatus;
      let uptime = '0h 0m';
      
      // 处理uptime - 后端返回的可能是字符串格式
      if (systemStatus.uptime) {
        if (typeof systemStatus.uptime === 'string') {
          // 如果是字符串格式（如"2h30m15.5s"），直接使用
          uptime = systemStatus.uptime;
        } else if (typeof systemStatus.uptime === 'number') {
          // 如果是数字（秒数），转换为可读格式
          const uptimeSeconds = systemStatus.uptime;
          const days = Math.floor(uptimeSeconds / 86400);
          const hours = Math.floor((uptimeSeconds % 86400) / 3600);
          const minutes = Math.floor((uptimeSeconds % 3600) / 60);
          uptime = `${days}天 ${hours}小时 ${minutes}分钟`;
        }
      }

      setStats({
        uptime: uptime,
        activeDevices: systemStatus.active_connections || systemStatus.active_plugins || 0,
        dataPoints: realtimeData.iotData?.length || 0,
        errorRate: systemStatus.error_rate || 0,
        cpuUsage: systemStatus.cpu_usage || 0,
        memoryUsage: systemStatus.memory_usage || 0
      });
    } else {
      // 如果没有实时数据，尝试从API获取基本统计
      try {
        console.log('实时数据不可用，使用默认统计数据');
        // 这里可以调用API获取系统状态
      } catch (error) {
        console.error('获取系统统计失败:', error);
      }
    }
  };

  // 获取告警数据
  const fetchAlerts = async () => {
    try {
      const alertsResponse = await alertService.getAlerts({ page: 1, pageSize: 5 });
      setRecentAlerts(alertsResponse.alerts);
    } catch (error) {
      console.error('获取告警数据失败:', error);
    }
  };

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      await Promise.all([fetchPlugins(), fetchStats(), fetchAlerts()]);
      setLoading(false);
    };

    loadData();
  }, []);

  // 监听实时数据变化
  useEffect(() => {
    fetchStats();
  }, [realtimeData?.systemStatus, realtimeData?.iotData]);

  const getAlertIcon = (level: string) => {
    switch (level) {
      case 'critical':
      case 'error':
        return <CloseCircleOutlined style={{ color: '#f5222d' }} />;
      case 'warning':
        return <WarningOutlined style={{ color: '#faad14' }} />;
      default:
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
    }
  };

  const getAlertColor = (level: string) => {
    switch (level) {
      case 'critical':
      case 'error':
        return 'error';
      case 'warning':
        return 'warning';
      default:
        return 'info';
    }
  };

  const refreshData = async () => {
    setLoading(true);
    await Promise.all([fetchPlugins(), fetchStats(), fetchAlerts()]);
    if (!isConnected) {
      await reconnect();
    }
    setLoading(false);
  };

  const testWebSocketConnection = async () => {
    console.log('🔧 测试WebSocket连接...');
    console.log('当前认证token:', authService.getToken()?.substring(0, 20) + '...');
    
    try {
      // 测试API连接
      const response = await fetch('/api/v1/system/status');
      const data = await response.json();
      console.log('✅ API连接正常，系统状态:', data);
    } catch (error) {
      console.error('❌ API连接失败:', error);
    }
    
    // 手动重连WebSocket
    console.log('🔄 尝试重连WebSocket...');
    reconnect();
  };

  // 在组件加载时测试连接
  useEffect(() => {
    const timer = setTimeout(() => {
      if (!isConnected) {
        testWebSocketConnection();
      }
    }, 5000); // 5秒后如果还没连接就测试

    return () => clearTimeout(timer);
  }, [isConnected, reconnect]);

  const tabItems = [
    {
      key: 'overview',
      label: (
        <span>
          <DashboardOutlined />
          系统概览
        </span>
      ),
      children: (
        <Row gutter={[24, 24]}>
          {/* 系统统计卡片 */}
          <Col xs={24} sm={12} lg={6}>
            <Card>
              <Statistic
                title="系统运行时间"
                value={stats.uptime}
                prefix={<ThunderboltOutlined />}
                valueStyle={{ color: '#52c41a' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card>
              <Statistic
                title="活跃插件"
                value={stats.activeDevices}
                prefix={<DatabaseOutlined />}
                valueStyle={{ color: '#1890ff' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card>
              <Statistic
                title="实时数据点"
                value={stats.dataPoints}
                prefix={<LineChartOutlined />}
                valueStyle={{ color: '#722ed1' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card>
              <Statistic
                title="连接状态"
                value={isConnected ? "已连接" : "断开"}
                prefix={<ApiOutlined />}
                valueStyle={{ color: isConnected ? '#52c41a' : '#f5222d' }}
              />
            </Card>
          </Col>

          {/* 系统资源使用 */}
          <Col xs={24} md={12}>
            <Card title="系统资源" extra={<Button icon={<ReloadOutlined />} onClick={refreshData} loading={loading} />}>
              <Space direction="vertical" style={{ width: '100%' }}>
                <div>
                  <Text>CPU 使用率</Text>
                  <Progress
                    percent={Math.round(stats.cpuUsage)}
                    status={stats.cpuUsage > 80 ? 'exception' : 'active'}
                    strokeColor={stats.cpuUsage > 80 ? '#f5222d' : '#1890ff'}
                  />
                </div>
                <div>
                  <Text>内存使用率</Text>
                  <Progress
                    percent={Math.round(stats.memoryUsage)}
                    status={stats.memoryUsage > 85 ? 'exception' : 'active'}
                    strokeColor={stats.memoryUsage > 85 ? '#f5222d' : '#52c41a'}
                  />
                </div>
                <Divider />
                <Row gutter={16}>
                  <Col span={12}>
                    <Statistic
                      title="连接状态"
                      value={connectionState}
                      valueStyle={{ 
                        fontSize: '14px',
                        color: isConnected ? '#52c41a' : '#f5222d'
                      }}
                    />
                  </Col>
                  <Col span={12}>
                    <Statistic
                      title="最后更新"
                      value={new Date().toLocaleTimeString()}
                      valueStyle={{ fontSize: '14px' }}
                    />
                  </Col>
                </Row>
              </Space>
            </Card>
          </Col>

          {/* 最近告警 */}
          <Col xs={24} md={12}>
            <Card title="最近告警" extra={<Badge count={recentAlerts.filter(a => a.level === 'error' || a.level === 'critical').length} />}>
              <List
                dataSource={recentAlerts}
                renderItem={(alert) => (
                  <List.Item>
                    <List.Item.Meta
                      avatar={getAlertIcon(alert.level)}
                      title={
                        <Space>
                          <Tag color={getAlertColor(alert.level)}>{alert.level.toUpperCase()}</Tag>
                          <Text>{alert.title || alert.description}</Text>
                        </Space>
                      }
                      description={
                        <Space>
                          <Text type="secondary">{alert.source}</Text>
                          <Text type="secondary">•</Text>
                          <Text type="secondary">{new Date(alert.createdAt).toLocaleString()}</Text>
                        </Space>
                      }
                    />
                  </List.Item>
                )}
              />
            </Card>
          </Col>

          {/* 插件状态 */}
          <Col xs={24}>
            <Card title="插件状态" loading={loading}>
              <Row gutter={[16, 16]}>
                {plugins.slice(0, 8).map((plugin, index) => (
                  <Col xs={24} sm={12} md={8} lg={6} key={`plugin-${plugin.id || index}`}>
                    <Card size="small">
                      <Space direction="vertical" style={{ width: '100%' }}>
                        <Space>
                          <Badge
                            status={plugin.status === 'running' ? 'success' : 'error'}
                            text={plugin.name}
                          />
                        </Space>
                        <Text type="secondary" style={{ fontSize: '12px' }}>
                          {plugin.type} • v{plugin.version}
                        </Text>
                      </Space>
                    </Card>
                  </Col>
                ))}
              </Row>
            </Card>
          </Col>
        </Row>
      )
    },
    {
      key: 'monitoring',
      label: (
        <span>
          <MonitorOutlined />
          系统监控
        </span>
      ),
      children: <SystemMonitorChart height={350} showDetailedCharts={true} />
    },
    {
      key: 'iot-data',
      label: (
        <span>
          <LineChartOutlined />
          数据流监控
        </span>
      ),
      children: <IoTDataChart height={350} showRawData={true} maxChartPoints={100} />
    }
  ];

      if (loading) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '50vh' 
      }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div>
      <Row justify="space-between" align="middle" style={{ marginBottom: 24 }}>
        <Col>
          <Title level={2} style={{ margin: 0 }}>
            <DashboardOutlined /> 仪表板
          </Title>
          <Text type="secondary">欢迎回来, {user?.username}</Text>
        </Col>
        <Col>
          <Space>
            <Button type="primary" icon={<ReloadOutlined />} onClick={refreshData} loading={loading}>
              刷新数据
            </Button>
            <Button icon={<ApiOutlined />} onClick={testWebSocketConnection}>
              测试连接
            </Button>
            {!isConnected && (
              <Button type="primary" icon={<ApiOutlined />} onClick={reconnect}>
                重新连接
              </Button>
            )}
          </Space>
        </Col>
      </Row>

      {/* 连接状态提示 */}
      {!isConnected && (
        <AntAlert
          message="实时连接断开"
          description="WebSocket连接已断开，部分实时功能可能不可用。点击重新连接或刷新页面。"
          type="warning"
          showIcon
          closable
          style={{ marginBottom: 16 }}
        />
      )}

      <Tabs 
        defaultActiveKey="overview" 
        items={tabItems}
        size="large"
      />
    </div>
  );
};

export default Dashboard;