import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  Row,
  Col,
  Statistic,
  Progress,
  Typography,
  Space,
  Button,
  Spin,
  Alert,
  Badge,
  Tag,
  Tooltip,
  Divider,
} from 'antd';
import {
  ReloadOutlined,
  ClockCircleOutlined,
  HddOutlined,
  ThunderboltOutlined,
  DatabaseOutlined,
  ApiOutlined,
  BugOutlined,
  LineChartOutlined,
  CheckCircleOutlined,
  WarningOutlined,
  CloseCircleOutlined,
  CloudServerOutlined,
} from '@ant-design/icons';
import { lightweightMetricsService, type LightweightMetrics } from '../../services/lightweightMetricsService';

const { Title, Text } = Typography;

interface RealTimeMetricsProps {
  autoRefresh?: boolean;
  refreshInterval?: number;
  compact?: boolean;
}

const RealTimeMetrics: React.FC<RealTimeMetricsProps> = ({
  autoRefresh = true,
  refreshInterval = 5000,
  compact = false,
}) => {
  const [metrics, setMetrics] = useState<LightweightMetrics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [isOnline, setIsOnline] = useState(true);

  const fetchMetrics = useCallback(async () => {
    try {
      setError(null);
      const data = await lightweightMetricsService.getLightweightMetrics();
      console.log('Received metrics data:', data);
      console.log('Gateway info:', data.gateway);
      setMetrics(data);
      setLastUpdated(new Date());
      setIsOnline(true);
    } catch (err: any) {
      setError(err.message || '获取指标失败');
      setIsOnline(false);
      console.error('Failed to fetch metrics:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  const handleRefresh = () => {
    setLoading(true);
    fetchMetrics();
  };

  useEffect(() => {
    fetchMetrics();
  }, [fetchMetrics]);

  useEffect(() => {
    if (!autoRefresh) return;

    const interval = setInterval(fetchMetrics, refreshInterval);
    return () => clearInterval(interval);
  }, [autoRefresh, refreshInterval, fetchMetrics]);

  const getHealthIcon = (status: string) => {
    switch (status) {
      case 'running':
      case 'healthy':
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
      case 'degraded':
        return <WarningOutlined style={{ color: '#faad14' }} />;
      case 'error':
      case 'unhealthy':
        return <CloseCircleOutlined style={{ color: '#f5222d' }} />;
      default:
        return <CloudServerOutlined style={{ color: '#d9d9d9' }} />;
    }
  };

  const getMemoryUsagePercent = () => {
    if (!metrics?.system) return 0;
    const { memory_usage_bytes, heap_size_bytes } = metrics.system;
    return heap_size_bytes > 0 ? (memory_usage_bytes / heap_size_bytes) * 100 : 0;
  };

  const getErrorRateColor = (rate: number) => {
    if (rate > 0.05) return '#f5222d'; // 红色：>5%
    if (rate > 0.01) return '#faad14'; // 黄色：>1%
    return '#52c41a'; // 绿色：<=1%
  };

  if (loading && !metrics) {
    return (
      <Card>
        <div style={{ textAlign: 'center', padding: '40px 0' }}>
          <Spin size="large" />
          <div style={{ marginTop: 16 }}>
            <Text>加载实时指标中...</Text>
          </div>
        </div>
      </Card>
    );
  }

  if (error && !metrics) {
    return (
      <Card>
        <Alert
          message="无法获取实时指标"
          description={error}
          type="error"
          showIcon
          action={
            <Button size="small" onClick={handleRefresh} loading={loading}>
              重试
            </Button>
          }
        />
      </Card>
    );
  }

  if (!metrics) {
    return null;
  }

  const systemCards = [
    {
      title: '系统运行时间',
      value: lightweightMetricsService.formatUptime(metrics.system.uptime_seconds),
      icon: <ClockCircleOutlined />,
      color: '#1890ff',
    },
    {
      title: '内存使用率',
      value: `${getMemoryUsagePercent().toFixed(1)}%`,
      icon: <HddOutlined />,
      color: getMemoryUsagePercent() > 80 ? '#f5222d' : '#52c41a',
      extra: (
        <Progress
          percent={getMemoryUsagePercent()}
          size="small"
          strokeColor={getMemoryUsagePercent() > 80 ? '#f5222d' : '#52c41a'}
          showInfo={false}
        />
      ),
    },
    {
      title: 'CPU使用率',
      value: `${metrics.system.cpu_usage_percent.toFixed(1)}%`,
      icon: <ThunderboltOutlined />,
      color: metrics.system.cpu_usage_percent > 80 ? '#f5222d' : '#52c41a',
      extra: (
        <Progress
          percent={metrics.system.cpu_usage_percent}
          size="small"
          strokeColor={metrics.system.cpu_usage_percent > 80 ? '#f5222d' : '#52c41a'}
          showInfo={false}
        />
      ),
    },
    {
      title: 'Goroutines',
      value: metrics.system.goroutine_count,
      icon: <ApiOutlined />,
      color: '#722ed1',
    },
  ];

  const gatewayCards = [
    {
      title: '网关状态',
      value: metrics.gateway.status,
      icon: getHealthIcon(metrics.gateway.status),
      color: lightweightMetricsService.getStatusColor(metrics.gateway.status),
      valueRender: (value: any) => (
        <Tag color={lightweightMetricsService.getStatusColor(String(value))}>
          {String(value).toUpperCase()}
        </Tag>
      ),
    },
    {
      title: '活跃适配器',
      value: `${metrics.gateway.running_adapters}/${metrics.gateway.total_adapters}`,
      icon: <DatabaseOutlined />,
      color: '#1890ff',
      extra: (
        <Progress
          percent={metrics.gateway.total_adapters > 0 ? (metrics.gateway.running_adapters / metrics.gateway.total_adapters) * 100 : 0}
          size="small"
          strokeColor="#1890ff"
          showInfo={false}
        />
      ),
    },
    {
      title: '活跃连接器',
      value: `${metrics.gateway.running_sinks}/${metrics.gateway.total_sinks}`,
      icon: <ThunderboltOutlined />,
      color: '#52c41a',
      extra: (
        <Progress
          percent={metrics.gateway.total_sinks > 0 ? (metrics.gateway.running_sinks / metrics.gateway.total_sinks) * 100 : 0}
          size="small"
          strokeColor="#52c41a"
          showInfo={false}
        />
      ),
    },
    {
      title: 'NATS连接',
      value: metrics.gateway.nats_connected ? '已连接' : '未连接',
      icon: <ApiOutlined />,
      color: metrics.gateway.nats_connected ? '#52c41a' : '#f5222d',
      valueRender: (value: any) => (
        <Badge
          status={metrics.gateway.nats_connected ? 'success' : 'error'}
          text={String(value)}
        />
      ),
    },
  ];

  const dataCards = [
    {
      title: '数据点/秒',
      value: metrics.data.data_points_per_second.toFixed(1),
      icon: <LineChartOutlined />,
      color: '#722ed1',
    },
    {
      title: '字节/秒',
      value: lightweightMetricsService.formatBytes(metrics.data.bytes_per_second),
      icon: <DatabaseOutlined />,
      color: '#13c2c2',
    },
    {
      title: '平均延迟',
      value: lightweightMetricsService.formatLatency(metrics.data.average_latency_ms),
      icon: <ClockCircleOutlined />,
      color: metrics.data.average_latency_ms > 100 ? '#faad14' : '#52c41a',
    },
    {
      title: '错误率',
      value: `${(metrics.errors.error_rate * 100).toFixed(2)}%`,
      icon: <BugOutlined />,
      color: getErrorRateColor(metrics.errors.error_rate),
    },
  ];

  const ruleCards = [
    {
      title: '启用规则',
      value: `${metrics.rules.enabled_rules}/${metrics.rules.total_rules}`,
      icon: <ApiOutlined />,
      color: '#1890ff',
    },
    {
      title: '规则匹配',
      value: lightweightMetricsService.formatNumber(metrics.rules.rules_matched),
      icon: <CheckCircleOutlined />,
      color: '#52c41a',
    },
    {
      title: '动作执行',
      value: lightweightMetricsService.formatNumber(metrics.rules.actions_executed),
      icon: <ThunderboltOutlined />,
      color: '#722ed1',
    },
    {
      title: '执行成功率',
      value: metrics.rules.actions_executed > 0 
        ? `${((metrics.rules.actions_succeeded / metrics.rules.actions_executed) * 100).toFixed(1)}%`
        : '0%',
      icon: <CheckCircleOutlined />,
      color: '#52c41a',
    },
  ];

  const renderMetricCard = (card: any) => (
    <Card size="small" key={card.title}>
      <Statistic
        title={card.title}
        value={card.value}
        prefix={card.icon}
        valueStyle={{ color: card.color }}
        valueRender={card.valueRender}
      />
      {card.extra && <div style={{ marginTop: 8 }}>{card.extra}</div>}
    </Card>
  );

  const renderSection = (title: string, cards: any[], icon: React.ReactNode) => (
    <Card 
      title={
        <Space>
          {icon}
          <span>{title}</span>
        </Space>
      }
      size="small"
      style={{ marginBottom: 16 }}
    >
      <Row gutter={[16, 16]}>
        {cards.map((card, index) => (
          <Col xs={24} sm={12} lg={compact ? 12 : 6} key={index}>
            {renderMetricCard(card)}
          </Col>
        ))}
      </Row>
    </Card>
  );

  return (
    <div>
      {/* 头部信息 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Row justify="space-between" align="middle">
          <Col>
            <Space>
              <Title level={4} style={{ margin: 0 }}>
                <LineChartOutlined /> 实时系统指标
              </Title>
              <Badge
                status={isOnline ? 'success' : 'error'}
                text={isOnline ? '在线' : '离线'}
              />
            </Space>
          </Col>
          <Col>
            <Space>
              {lastUpdated && (
                <Text type="secondary">
                  更新时间: {lastUpdated.toLocaleTimeString()}
                </Text>
              )}
              <Button
                icon={<ReloadOutlined />}
                onClick={handleRefresh}
                loading={loading}
                size="small"
              >
                刷新
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 错误提示 */}
      {error && (
        <Alert
          message="数据更新失败"
          description={error}
          type="warning"
          showIcon
          closable
          style={{ marginBottom: 16 }}
        />
      )}

      {/* 系统指标 */}
      {renderSection('系统资源', systemCards, <HddOutlined />)}

      {/* 网关指标 */}
      {renderSection('网关状态', gatewayCards, <CloudServerOutlined />)}

      {/* 数据处理指标 */}
      {renderSection('数据处理', dataCards, <LineChartOutlined />)}

      {/* 规则引擎指标 */}
      {renderSection('规则引擎', ruleCards, <ThunderboltOutlined />)}

      {/* 详细信息 */}
      {!compact && (
        <Card 
          title={
            <Space>
              <BugOutlined />
              <span>详细信息</span>
            </Space>
          }
          size="small"
        >
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={8}>
              <Card size="small" title="系统信息">
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div>
                    <Text strong>版本: </Text>
                    <Text>{metrics.system.version}</Text>
                  </div>
                  <div>
                    <Text strong>Go版本: </Text>
                    <Text>{metrics.system.go_version}</Text>
                  </div>
                  <div>
                    <Text strong>配置文件: </Text>
                    <Text code>{metrics.gateway.config_file}</Text>
                  </div>
                  <div>
                    <Text strong>插件目录: </Text>
                    <Text code>{metrics.gateway.plugins_directory}</Text>
                  </div>
                </Space>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={8}>
              <Card size="small" title="连接统计">
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div>
                    <Text strong>活跃连接: </Text>
                    <Text>{metrics.connections.active_connections}</Text>
                  </div>
                  <div>
                    <Text strong>总连接数: </Text>
                    <Text>{lightweightMetricsService.formatNumber(metrics.connections.total_connections)}</Text>
                  </div>
                  <div>
                    <Text strong>失败连接: </Text>
                    <Text type="danger">{lightweightMetricsService.formatNumber(metrics.connections.failed_connections)}</Text>
                  </div>
                  <div>
                    <Text strong>重连次数: </Text>
                    <Text>{lightweightMetricsService.formatNumber(metrics.connections.reconnection_count)}</Text>
                  </div>
                </Space>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={8}>
              <Card size="small" title="错误统计">
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div>
                    <Text strong>总错误数: </Text>
                    <Text type="danger">{lightweightMetricsService.formatNumber(metrics.errors.total_errors)}</Text>
                  </div>
                  <div>
                    <Text strong>错误/秒: </Text>
                    <Text>{metrics.errors.errors_per_second.toFixed(2)}</Text>
                  </div>
                  <div>
                    <Text strong>恢复次数: </Text>
                    <Text type="success">{lightweightMetricsService.formatNumber(metrics.errors.recovery_count)}</Text>
                  </div>
                  {metrics.errors.last_error && (
                    <div>
                      <Text strong>最后错误: </Text>
                      <Tooltip title={metrics.errors.last_error}>
                        <Text type="danger" ellipsis>
                          {metrics.errors.last_error.substring(0, 30)}...
                        </Text>
                      </Tooltip>
                    </div>
                  )}
                </Space>
              </Card>
            </Col>
          </Row>
        </Card>
      )}
    </div>
  );
};

export default RealTimeMetrics;