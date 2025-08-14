import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
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
  LineChartOutlined,
  BugOutlined,
  FireOutlined
} from '@ant-design/icons';
import { useAuthStore } from '../store/authStore';
import { pluginService } from '../services/pluginService';
import { alertService } from '../services/alertService';
import { SystemMonitorChart } from '../components/charts/SystemMonitorChart';
import { IoTDataChart } from '../components/charts/IoTDataChart';
import { useRealTimeData } from '../hooks/useRealTimeData';
import { lightweightMetricsService, type LightweightMetrics } from '../services/lightweightMetricsService';
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
  const [metricsData, setMetricsData] = useState<LightweightMetrics | null>(null);
  const [metricsLoading, setMetricsLoading] = useState(true);
  
  // 防抖和缓存相关
  const metricsIntervalRef = useRef<NodeJS.Timeout | null>(null);
  const lastUpdateTime = useRef<string>('');
  const isFirstLoad = useRef(true);
  
  // 使用实时数据Hook
  const { data: realtimeData, isConnected, connectionState, reconnect } = useRealTimeData();


  const fetchPlugins = useCallback(async () => {
    try {
      const response = await pluginService.getPlugins({ page: 1, page_size: 50 });
      setPlugins(response.data || []);
    } catch (error) {
      console.error('❌ 获取插件列表失败:', error);
    }
  }, []);

  // 获取轻量级指标数据 - 优化版本，避免频繁loading状态切换
  const fetchLightweightMetrics = useCallback(async (showLoading: boolean = false) => {
    try {
      if (showLoading) {
        setMetricsLoading(true);
      }
      
      const data = await lightweightMetricsService.getLightweightMetrics();
      
      // 检查数据是否真的有变化，避免无意义的状态更新
      const newUpdateTime = data.last_updated;
      if (newUpdateTime !== lastUpdateTime.current || isFirstLoad.current) {
        setMetricsData(data);
        lastUpdateTime.current = newUpdateTime;
      }
      
      if (isFirstLoad.current) {
        isFirstLoad.current = false;
      }
    } catch (error) {
      console.error('❌ Dashboard获取轻量级指标失败:', error);
    } finally {
      if (showLoading) {
        setMetricsLoading(false);
      }
    }
  }, []);

  const fetchStats = useCallback(async () => {
    // 从实时数据更新统计信息
    if (realtimeData?.systemStatus) {
      
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

      setStats(prevStats => {
        const newStats = {
          uptime: uptime,
          activeDevices: systemStatus.active_connections || systemStatus.active_plugins || 0,
          dataPoints: realtimeData.iotData?.length || 0,
          errorRate: systemStatus.error_rate || 0,
          cpuUsage: systemStatus.cpu_usage || 0,
          memoryUsage: systemStatus.memory_usage || 0
        };
        
        // 只有数据真正改变时才更新状态
        if (JSON.stringify(prevStats) !== JSON.stringify(newStats)) {
          return newStats;
        }
        return prevStats;
      });
    } else {
      // 如果没有实时数据，尝试从API获取基本统计
      try {
        // 这里可以调用API获取系统状态
      } catch (error) {
        console.error('获取系统统计失败:', error);
      }
    }
  }, [realtimeData?.systemStatus, realtimeData?.iotData]);

  // 获取告警数据
  const fetchAlerts = useCallback(async () => {
    try {
      const alertsResponse = await alertService.getAlerts({ page: 1, pageSize: 5 });
      setRecentAlerts(alertsResponse.alerts);
    } catch (error) {
      console.error('❌ 获取告警数据失败:', error);
    }
  }, []);

  // 初始化数据加载
  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      await Promise.all([
        fetchPlugins(), 
        fetchStats(), 
        fetchAlerts(),
        fetchLightweightMetrics(true) // 第一次显示loading
      ]);
      setLoading(false);
    };

    loadData();
  }, []); // 只在组件挂载时执行一次

  // 监听实时数据变化 - 使用防抖
  useEffect(() => {
    if (realtimeData?.systemStatus || realtimeData?.iotData) {
      fetchStats();
    }
  }, [fetchStats]); // 依赖fetchStats而不是具体的实时数据

  // 定期更新轻量级指标 - 优化版本，避免频繁创建定时器
  useEffect(() => {
    // 清理之前的定时器
    if (metricsIntervalRef.current) {
      clearInterval(metricsIntervalRef.current);
    }

    // 创建新的定时器，但不显示loading状态
    metricsIntervalRef.current = setInterval(() => {
      fetchLightweightMetrics(false); // 后续更新不显示loading
    }, 8000); // 增加到8秒，减少更新频率

    return () => {
      if (metricsIntervalRef.current) {
        clearInterval(metricsIntervalRef.current);
        metricsIntervalRef.current = null;
      }
    };
  }, [fetchLightweightMetrics]);

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

  const refreshData = useCallback(async () => {
    setLoading(true);
    await Promise.all([
      fetchPlugins(), 
      fetchStats(), 
      fetchAlerts(),
      fetchLightweightMetrics(true) // 手动刷新时显示loading
    ]);
    if (!isConnected) {
      await reconnect();
    }
    setLoading(false);
  }, [fetchPlugins, fetchStats, fetchAlerts, fetchLightweightMetrics, isConnected, reconnect]);

  const testWebSocketConnection = async () => {
    
    try {
      // 测试API连接
      const response = await fetch('/api/v1/system/status');
      const data = await response.json();
    } catch (error) {
      console.error('❌ API连接失败:', error);
    }
    
    // 手动重连WebSocket
    reconnect();
  };

  const clearAuthAndReload = () => {
    const { logout } = useAuthStore.getState();
    logout();
    window.location.reload();
  };

  // 手动刷新认证token
  const refreshAuth = async () => {
    try {
      const newToken = await authService.refreshToken();
      
      // 更新WebSocket服务的token
      const { webSocketService } = await import('../services/websocketService');
      webSocketService.setToken(newToken);
      
      // 如果WebSocket未连接，尝试重连
      if (!isConnected) {
        reconnect();
      }
      
      // 重新加载数据
      await refreshData();
    } catch (error) {
      console.error('❌ 刷新认证token失败:', error);
      // 如果刷新失败，清除认证状态
      clearAuthAndReload();
    }
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

  // 使用useMemo缓存计算结果，避免每次渲染都重新计算
  const formattedMetrics = useMemo(() => {
    if (!metricsData) return null;
    
    return {
      uptime: lightweightMetricsService.formatUptime(metricsData.system.uptime_seconds),
      activeAdapters: `${metricsData.gateway.running_adapters}/${metricsData.gateway.total_adapters}`,
      dataPointsPerSecond: metricsData.data.data_points_per_second.toFixed(1),
      rulesEnabled: `${metricsData.rules.enabled_rules}/${metricsData.rules.total_rules}`,
      rulesMatched: lightweightMetricsService.formatNumber(metricsData.rules.rules_matched),
      actionsExecuted: lightweightMetricsService.formatNumber(metricsData.rules.actions_executed),
      successRate: metricsData.rules.actions_executed > 0 
        ? `${((metricsData.rules.actions_succeeded / metricsData.rules.actions_executed) * 100).toFixed(1)}%`
        : '0%',
      cpuUsage: metricsData.system.cpu_usage_percent,
      memoryUsagePercent: Math.round((metricsData.system.memory_usage_bytes / 1024 / 1024 / 1024) * 10),
      hasActionsFailed: metricsData.rules.actions_failed > 0
    };
  }, [metricsData]);

  // 缓存当前时间字符串，避免每秒都重新计算
  const currentTime = useMemo(() => new Date().toLocaleTimeString(), []);

  const tabItems = useMemo(() => [
    {
      key: 'overview',
      label: (
        <span>
          <DashboardOutlined />
          系统概览
        </span>
      ),
      children: (
        <Row gutter={[12, 12]}>
          {/* 系统统计卡片 - 紧凑布局 */}
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="系统运行时间"
                value={formattedMetrics?.uptime || stats.uptime}
                prefix={<ThunderboltOutlined />}
                valueStyle={{ color: '#52c41a', fontSize: '16px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="活跃适配器"
                value={formattedMetrics?.activeAdapters || stats.activeDevices}
                prefix={<DatabaseOutlined />}
                valueStyle={{ color: '#1890ff', fontSize: '16px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="数据点/秒"
                value={formattedMetrics?.dataPointsPerSecond || stats.dataPoints}
                prefix={<LineChartOutlined />}
                valueStyle={{ color: '#722ed1', fontSize: '16px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small">
              <Statistic
                title="连接状态"
                value={isConnected ? "已连接" : "断开"}
                prefix={<ApiOutlined />}
                valueStyle={{ color: isConnected ? '#52c41a' : '#f5222d', fontSize: '16px' }}
              />
            </Card>
          </Col>

          {/* 规则引擎统计卡片 - 紧凑布局 */}
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="启用规则"
                value={formattedMetrics?.rulesEnabled || '0/0'}
                prefix={<FireOutlined />}
                valueStyle={{ color: '#fa8c16', fontSize: '16px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="规则匹配次数"
                value={formattedMetrics?.rulesMatched || '0'}
                prefix={<CheckCircleOutlined />}
                valueStyle={{ color: '#52c41a', fontSize: '16px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="动作执行次数"
                value={formattedMetrics?.actionsExecuted || '0'}
                prefix={<ThunderboltOutlined />}
                valueStyle={{ color: '#722ed1', fontSize: '16px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8} lg={4} xl={3}>
            <Card size="small" loading={metricsLoading}>
              <Statistic
                title="执行成功率"
                value={formattedMetrics?.successRate || '0%'}
                prefix={<BugOutlined />}
                valueStyle={{ 
                  color: formattedMetrics?.hasActionsFailed ? '#f5222d' : '#52c41a',
                  fontSize: '16px'
                }}
              />
            </Card>
          </Col>

          {/* 系统资源使用 - 紧凑布局 */}
          <Col xs={24} xl={12}>
            <Card 
              title="系统资源" 
              size="small"
              extra={<Button size="small" icon={<ReloadOutlined />} onClick={refreshData} loading={loading} />}
            >
              <Row gutter={[8, 8]}>
                <Col xs={24} sm={12}>
                  <div style={{ marginBottom: 8 }}>
                    <Text style={{ fontSize: '12px' }}>CPU 使用率</Text>
                    <Progress
                      size="small"
                      percent={Math.round(formattedMetrics?.cpuUsage || stats.cpuUsage)}
                      status={stats.cpuUsage > 80 ? 'exception' : 'active'}
                      strokeColor={stats.cpuUsage > 80 ? '#f5222d' : '#1890ff'}
                      showInfo={true}
                    />
                  </div>
                </Col>
                <Col xs={24} sm={12}>
                  <div style={{ marginBottom: 8 }}>
                    <Text style={{ fontSize: '12px' }}>内存使用率</Text>
                    <Progress
                      size="small"
                      percent={formattedMetrics?.memoryUsagePercent || Math.round(stats.memoryUsage)}
                      status={stats.memoryUsage > 85 ? 'exception' : 'active'}
                      strokeColor={stats.memoryUsage > 85 ? '#f5222d' : '#52c41a'}
                      showInfo={true}
                    />
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Statistic
                    title="连接状态"
                    value={connectionState}
                    valueStyle={{ 
                      fontSize: '12px',
                      color: isConnected ? '#52c41a' : '#f5222d'
                    }}
                  />
                </Col>
                <Col xs={12} sm={6}>
                  <Statistic
                    title="最后更新"
                    value={currentTime}
                    valueStyle={{ fontSize: '12px' }}
                  />
                </Col>
              </Row>
            </Card>
          </Col>

          {/* 最近告警 - 紧凑布局 */}
          <Col xs={24} xl={12}>
            <Card 
              title="最近告警" 
              size="small"
              extra={<Badge count={recentAlerts.filter(a => a.level === 'error' || a.level === 'critical').length} />}
            >
              <List
                size="small"
                dataSource={recentAlerts.slice(0, 3)}
                renderItem={(alert) => (
                  <List.Item style={{ padding: '8px 0' }}>
                    <List.Item.Meta
                      avatar={getAlertIcon(alert.level)}
                      title={
                        <Space>
                          <Tag size="small" color={getAlertColor(alert.level)}>{alert.level.toUpperCase()}</Tag>
                          <Text style={{ fontSize: '13px' }}>{alert.title || alert.description}</Text>
                        </Space>
                      }
                      description={
                        <Space style={{ fontSize: '11px' }}>
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

          {/* 插件状态 - 分组显示 */}
          <Col xs={24}>
            <Card title="插件状态" size="small" loading={loading}>
              {(() => {
                const adapterPlugins = plugins.filter(p => p.type === 'adapter').sort((a, b) => a.name.localeCompare(b.name));
                const sinkPlugins = plugins.filter(p => p.type === 'sink').sort((a, b) => a.name.localeCompare(b.name));
                
                return (
                  <>
                    {/* Adapter 插件组 */}
                    {adapterPlugins.length > 0 && (
                      <div style={{ marginBottom: 16 }}>
                        <Text strong style={{ fontSize: '14px', color: '#1890ff' }}>
                          数据适配器 (Adapters) - {adapterPlugins.filter(p => p.status === 'running').length}/{adapterPlugins.length}
                        </Text>
                        <Row gutter={[8, 8]} style={{ marginTop: 8 }}>
                          {adapterPlugins.slice(0, 8).map((plugin, index) => (
                            <Col xs={24} sm={12} md={8} lg={6} xl={4} key={`adapter-${plugin.id || index}`}>
                              <div style={{ 
                                border: '1px solid #e6f7ff', 
                                borderRadius: '6px', 
                                padding: '8px',
                                backgroundColor: plugin.status === 'running' ? '#e6f7ff' : '#fff2f0',
                                minHeight: '50px'
                              }}>
                                <div style={{ display: 'flex', alignItems: 'center', marginBottom: '4px' }}>
                                  <Badge
                                    status={plugin.status === 'running' ? 'success' : 'error'}
                                    text={<Text style={{ fontSize: '12px', fontWeight: 500 }}>{plugin.name}</Text>}
                                  />
                                </div>
                                <Text type="secondary" style={{ fontSize: '11px' }}>
                                  {plugin.type} • v{plugin.version}
                                </Text>
                              </div>
                            </Col>
                          ))}
                        </Row>
                      </div>
                    )}

                    {/* Sink 插件组 */}
                    {sinkPlugins.length > 0 && (
                      <div>
                        <Text strong style={{ fontSize: '14px', color: '#52c41a' }}>
                          数据输出 (Sinks) - {sinkPlugins.filter(p => p.status === 'running').length}/{sinkPlugins.length}
                        </Text>
                        <Row gutter={[8, 8]} style={{ marginTop: 8 }}>
                          {sinkPlugins.slice(0, 8).map((plugin, index) => (
                            <Col xs={24} sm={12} md={8} lg={6} xl={4} key={`sink-${plugin.id || index}`}>
                              <div style={{ 
                                border: '1px solid #f6ffed', 
                                borderRadius: '6px', 
                                padding: '8px',
                                backgroundColor: plugin.status === 'running' ? '#f6ffed' : '#fff2f0',
                                minHeight: '50px'
                              }}>
                                <div style={{ display: 'flex', alignItems: 'center', marginBottom: '4px' }}>
                                  <Badge
                                    status={plugin.status === 'running' ? 'success' : 'error'}
                                    text={<Text style={{ fontSize: '12px', fontWeight: 500 }}>{plugin.name}</Text>}
                                  />
                                </div>
                                <Text type="secondary" style={{ fontSize: '11px' }}>
                                  {plugin.type} • v{plugin.version}
                                </Text>
                              </div>
                            </Col>
                          ))}
                        </Row>
                      </div>
                    )}

                    {/* 如果没有插件数据 */}
                    {plugins.length === 0 && (
                      <div style={{ textAlign: 'center', padding: '20px', color: '#999' }}>
                        暂无插件数据
                      </div>
                    )}
                  </>
                );
              })()}
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
      children: <SystemMonitorChart height={280} showDetailedCharts={true} />
    },
    {
      key: 'iot-data',
      label: (
        <span>
          <LineChartOutlined />
          数据流监控
        </span>
      ),
      children: <IoTDataChart 
        height={280} 
        showRawData={true} 
        maxChartPoints={100} 
        enableCompositeDataViewer={true}
        autoRefresh={true}
        refreshInterval={3000}
      />
    }
  ], [formattedMetrics, metricsLoading, stats, isConnected, connectionState, currentTime, plugins, loading, recentAlerts]);

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
      <Row justify="space-between" align="middle" style={{ marginBottom: 16 }}>
        <Col>
          <Title level={3} style={{ margin: 0, fontSize: '20px' }}>
            <DashboardOutlined /> 仪表板
          </Title>
          <Text type="secondary" style={{ fontSize: '13px' }}>欢迎回来, {user?.username}</Text>
        </Col>
        <Col>
          <Space size="small">
            <Button size="small" type="primary" icon={<ReloadOutlined />} onClick={refreshData} loading={loading}>
              刷新
            </Button>
            <Button size="small" icon={<ApiOutlined />} onClick={testWebSocketConnection}>
              测试连接
            </Button>
            {!isConnected && (
              <Button size="small" type="primary" icon={<ApiOutlined />} onClick={reconnect}>
                重连
              </Button>
            )}
            <Button size="small" danger onClick={clearAuthAndReload}>
              重新登录
            </Button>
          </Space>
        </Col>
      </Row>

      {/* 连接状态提示 - 紧凑样式 */}
      {!isConnected && (
        <AntAlert
          message="实时连接断开"
          description="WebSocket连接已断开，部分实时功能可能不可用。"
          type="warning"
          showIcon
          closable
          size="small"
          style={{ marginBottom: 12 }}
        />
      )}

      {/* 调试信息面板 - 紧凑样式 */}
      {import.meta.env.DEV && (
        <AntAlert
          message="开发调试信息"
          description={
            <div>
              <div style={{ fontSize: '11px', fontFamily: 'monospace', marginBottom: '8px' }}>
                <div>🌐 API: {import.meta.env.VITE_API_BASE_URL || 'default'} | 🔐 Token: {authService.getToken() ? '✓' : '❌'} | 📡 状态: {connectionState}</div>
              </div>
              <Space size="small">
                <Button size="small" onClick={testWebSocketConnection}>测试连接</Button>
                <Button size="small" onClick={refreshAuth}>刷新Token</Button>
                <Button danger size="small" onClick={clearAuthAndReload}>重新登录</Button>
              </Space>
            </div>
          }
          type="info"
          showIcon
          closable
          style={{ marginBottom: 12 }}
        />
      )}

      <Tabs 
        defaultActiveKey="overview" 
        items={tabItems}
        size="small"
        style={{ marginTop: 8 }}
      />
    </div>
  );
};

export default Dashboard;