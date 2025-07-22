import React, { useState, useEffect } from 'react';
import {
  Card,
  Row,
  Col,
  Table,
  Tag,
  Button,
  Tabs,
  Progress,
  Typography,
  Tooltip,
  Modal,
  Select,
  Spin,
  Empty,
  message,
  Divider,
  Space,
  Alert,
  Statistic,
  Badge,
} from 'antd';
import {
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  MonitorOutlined,
  ApiOutlined,
  ThunderboltOutlined,
  DatabaseOutlined,
  LineChartOutlined,
  RestOutlined,
  BugOutlined,
  ForkOutlined,
  WarningOutlined
} from '@ant-design/icons';
import { monitoringService } from '../services/monitoringService';
import { lightweightMetricsService } from '../services/lightweightMetricsService';
import DataFlowChart from '../components/charts/DataFlowChart';
import RealTimeMetrics from '../components/metrics/RealTimeMetrics';
import SystemMetricsChart from '../components/charts/SystemMetricsChart';
import type {
  AdapterStatus,
  SinkStatus,
  ConnectionOverview,
  DataFlowMetrics,
  AdapterDiagnostics,
} from '../types/monitoring';
import { TIME_RANGE_OPTIONS } from '../types/monitoring';

const { Title, Text } = Typography;
const { Option } = Select;

const MonitoringPage: React.FC = () => {
  // 状态管理
  const [loading, setLoading] = useState(true);
  const [adapters, setAdapters] = useState<AdapterStatus[]>([]);
  const [sinks, setSinks] = useState<SinkStatus[]>([]);
  const [overview, setOverview] = useState<ConnectionOverview | null>(null);
  const [dataFlow, setDataFlow] = useState<DataFlowMetrics[]>([]);
  const [timeRange, setTimeRange] = useState('5m');
  const [selectedAdapter, setSelectedAdapter] = useState<string | null>(null);
  const [diagnostics, setDiagnostics] = useState<AdapterDiagnostics | null>(null);
  const [diagnosticsVisible, setDiagnosticsVisible] = useState(false);
  const [activeTab, setActiveTab] = useState('overview');

  // 实时数据连接状态
  const isConnected = true; // 临时设置，待实际实现WebSocket连接

  // 从轻量级指标服务获取概览数据
  const loadOverviewFromMetrics = async () => {
    try {
      const metrics = await lightweightMetricsService.getLightweightMetrics();
      
      // 基于轻量级指标构建概览数据
      const overview: ConnectionOverview = {
        system_health: metrics.gateway.status === 'running' ? 'healthy' : 'degraded',
        active_connections: metrics.connections.active_connections,
        total_adapters: metrics.gateway.total_adapters,
        running_adapters: metrics.gateway.running_adapters,
        healthy_adapters: metrics.gateway.running_adapters, // 假设运行中的都是健康的
        total_sinks: metrics.gateway.total_sinks,
        running_sinks: metrics.gateway.running_sinks,
        healthy_sinks: metrics.gateway.running_sinks, // 假设运行中的都是健康的
        total_data_points_per_sec: metrics.data.data_points_per_second,
        total_errors_per_sec: metrics.errors.errors_per_second,
      };
      
      setOverview(overview);
      console.log('从轻量级指标获取概览数据:', overview);
    } catch (error) {
      console.warn('轻量级指标不可用，使用默认概览数据:', error);
      setOverview({
        system_health: 'unknown',
        active_connections: 0,
        total_adapters: 0,
        running_adapters: 0,
        healthy_adapters: 0,
        total_sinks: 0,
        running_sinks: 0,
        healthy_sinks: 0,
        total_data_points_per_sec: 0,
        total_errors_per_sec: 0,
      });
    }
  };

  // 加载数据
  const loadData = async () => {
    console.log('🚀 MonitoringPage loadData 开始');
    try {
      setLoading(true);
      
      // 优先从轻量级指标服务获取概览数据
      console.log('📊 加载轻量级指标数据...');
      await loadOverviewFromMetrics();
      
      // 直接从插件API获取适配器和连接器数据
      try {
        console.log('🔍 开始加载插件数据...');
        const pluginData = await monitoringService.getPlugins();
        console.log('✅ 插件数据加载完成:', pluginData);
        
        // 按名称排序适配器和连接器
        const sortedAdapters = [...pluginData.adapters].sort((a, b) => a.name.localeCompare(b.name));
        const sortedSinks = [...pluginData.sinks].sort((a, b) => a.name.localeCompare(b.name));
        
        setAdapters(sortedAdapters);
        setSinks(sortedSinks);
        
        console.log('📊 设置状态:', {
          adapters: pluginData.adapters.length,
          sinks: pluginData.sinks.length
        });
        
        // 数据流指标从监控API获取真实数据
        try {
          const flowData = await monitoringService.getDataFlowMetrics({ time_range: timeRange });
          if (flowData.metrics && flowData.metrics.length > 0) {
            setDataFlow(flowData.metrics);
            console.log('数据流指标获取成功:', flowData.metrics.length, '个数据流');
            console.log('数据流详情:', flowData.metrics);
          } else {
            // 如果没有数据流数据，创建空数组
            setDataFlow([]);
            console.log('当前没有数据流数据，API返回:', flowData);
          }
        } catch (flowError) {
          console.warn('数据流指标获取失败:', flowError);
          setDataFlow([]);
        }
      } catch (apiError) {
        console.error('❌ 插件API调用失败:', apiError);
        message.error('获取插件数据失败: ' + apiError.message);
        setAdapters([]);
        setSinks([]);
        setDataFlow([]);
      }
    } catch (error: any) {
      console.error('❌ 加载监控数据失败:', error);
      message.error('加载监控数据失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  // 刷新数据
  const refreshData = async () => {
    await loadData();
    message.success('数据已刷新');
  };

  // 测试连接
  const testConnection = async (adapterName: string) => {
    try {
      const result = await monitoringService.testAdapterConnection(adapterName);
      if (result.success) {
        message.success(`${adapterName} 连接测试成功`);
      } else {
        message.error(`${adapterName} 连接测试失败: ${result.error}`);
      }
    } catch (error: any) {
      message.error('连接测试失败: ' + error.message);
    }
  };

  // 重启适配器
  const restartAdapter = async (adapterName: string) => {
    Modal.confirm({
      title: '确认重启',
      content: `确定要重启适配器 "${adapterName}" 吗？`,
      onOk: async () => {
        try {
          await monitoringService.restartAdapter(adapterName);
          message.success('重启请求已提交');
          setTimeout(loadData, 2000); // 2秒后刷新数据
        } catch (error: any) {
          message.error('重启失败: ' + error.message);
        }
      },
    });
  };

  // 查看诊断信息
  const viewDiagnostics = async (adapterName: string) => {
    try {
      setSelectedAdapter(adapterName);
      const result = await monitoringService.getAdapterDiagnostics(adapterName);
      setDiagnostics(result);
      setDiagnosticsVisible(true);
    } catch (error: any) {
      message.error('获取诊断信息失败: ' + error.message);
    }
  };

  // 初始加载
  useEffect(() => {
    loadData();
  }, []);

  // 时间范围变化时重新加载数据流
  useEffect(() => {
    if (timeRange) {
      // 直接从监控API获取真实数据流指标
      monitoringService.getDataFlowMetrics({ time_range: timeRange })
        .then(data => {
          setDataFlow(data.metrics);
          console.log('数据流指标已更新:', data.metrics);
        })
        .catch(error => {
          console.error('获取数据流指标失败:', error);
          // 如果监控API失败，尝试从轻量级指标获取基础数据
          lightweightMetricsService.getLightweightMetrics()
            .then(metrics => {
              // 基于轻量级指标创建基础数据流数据
              const now = new Date();
              const fallbackDataFlowMetrics: DataFlowMetrics[] = [
                {
                  adapter_name: 'system_metrics',
                  device_id: 'gateway',
                  key: 'data_throughput',
                  data_points_per_sec: metrics.data.data_points_per_second,
                  bytes_per_sec: metrics.data.bytes_per_second,
                  latency_ms: metrics.data.average_latency_ms,
                  error_rate: metrics.errors.error_rate,
                  last_value: metrics.data.data_points_per_second,
                  last_timestamp: now.toISOString()
                }
              ];
              setDataFlow(fallbackDataFlowMetrics);
              console.log('使用轻量级指标作为数据流后备数据');
            })
            .catch(fallbackError => {
              console.error('轻量级指标也失败:', fallbackError);
              setDataFlow([]);
            });
        });
    }
  }, [timeRange]);

  // 实时数据更新
  useEffect(() => {
    if (isConnected) {
      // 这里可以处理实时数据更新
      // 比如更新某些实时指标
    }
  }, [isConnected]);

  // 获取系统健康状态图标
  const getSystemHealthIcon = (health: string) => {
    switch (health) {
      case 'healthy':
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
      case 'degraded':
        return <WarningOutlined style={{ color: '#faad14' }} />;
      case 'unhealthy':
        return <CloseCircleOutlined style={{ color: '#f5222d' }} />;
      default:
        return <MonitorOutlined style={{ color: '#d9d9d9' }} />;
    }
  };

  // 适配器表格列定义
  const adapterColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: AdapterStatus) => (
        <Space>
          <span style={{ fontSize: '16px' }}>
            {monitoringService.getAdapterIcon(record.type)}
          </span>
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => <Tag>{type.toUpperCase()}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Badge
          status={monitoringService.getStatusColor(status) as any}
          text={monitoringService.getStatusText(status)}
        />
      ),
    },
    {
      title: '健康状态',
      dataIndex: 'health',
      key: 'health',
      render: (health: string, record: AdapterStatus) => (
        <Tooltip title={record.health_message}>
          <Tag color={monitoringService.getHealthColor(health)}>
            {monitoringService.getHealthText(health)}
          </Tag>
        </Tooltip>
      ),
    },
    {
      title: '运行时间',
      dataIndex: 'connection_uptime',
      key: 'connection_uptime',
      render: (uptime: number) => monitoringService.formatUptime(uptime),
    },
    {
      title: '数据点',
      dataIndex: 'data_points_count',
      key: 'data_points_count',
      render: (count: number) => monitoringService.formatNumber(count),
    },
    {
      title: '错误数',
      dataIndex: 'errors_count',
      key: 'errors_count',
      render: (count: number, record: AdapterStatus) => {
        const errorRate = record.data_points_count > 0 
          ? (count / record.data_points_count * 100).toFixed(2)
          : '0';
        return (
          <Tooltip title={`错误率: ${errorRate}%`}>
            <Text type={count > 0 ? 'danger' : 'secondary'}>{count}</Text>
          </Tooltip>
        );
      },
    },
    {
      title: '响应时间',
      dataIndex: 'response_time_ms',
      key: 'response_time_ms',
      render: (time: number) => monitoringService.formatLatency(time),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_: any, record: AdapterStatus) => (
        <Space size="small">
          <Tooltip title="测试连接">
            <Button
              size="small"
              icon={<ForkOutlined />}
              onClick={() => testConnection(record.name)}
              disabled={record.status !== 'running'}
            />
          </Tooltip>
          <Tooltip title="诊断">
            <Button
              size="small"
              icon={<BugOutlined />}
              onClick={() => viewDiagnostics(record.name)}
            />
          </Tooltip>
          <Tooltip title="重启">
            <Button
              size="small"
              icon={<RestOutlined />}
              onClick={() => restartAdapter(record.name)}
              disabled={record.status === 'stopped'}
              danger
            />
          </Tooltip>
        </Space>
      ),
    },
  ];

  // 连接器表格列定义
  const sinkColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: SinkStatus) => (
        <Space>
          <span style={{ fontSize: '16px' }}>
            {monitoringService.getSinkIcon(record.type)}
          </span>
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => <Tag>{type.toUpperCase()}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Badge
          status={monitoringService.getStatusColor(status) as any}
          text={monitoringService.getStatusText(status)}
        />
      ),
    },
    {
      title: '健康状态',
      dataIndex: 'health',
      key: 'health',
      render: (health: string, record: SinkStatus) => (
        <Tooltip title={record.health_message}>
          <Tag color={monitoringService.getHealthColor(health)}>
            {monitoringService.getHealthText(health)}
          </Tag>
        </Tooltip>
      ),
    },
    {
      title: '运行时间',
      dataIndex: 'connection_uptime',
      key: 'connection_uptime',
      render: (uptime: number) => monitoringService.formatUptime(uptime),
    },
    {
      title: '消息发布',
      dataIndex: 'messages_published',
      key: 'messages_published',
      render: (count: number) => monitoringService.formatNumber(count),
    },
    {
      title: '错误数',
      dataIndex: 'errors_count',
      key: 'errors_count',
      render: (count: number) => (
        <Text type={count > 0 ? 'danger' : 'secondary'}>{count}</Text>
      ),
    },
    {
      title: '响应时间',
      dataIndex: 'response_time_ms',
      key: 'response_time_ms',
      render: (time: number) => monitoringService.formatLatency(time),
    },
  ];

  // 数据流表格列定义
  const dataFlowColumns = [
    {
      title: '适配器',
      dataIndex: 'adapter_name',
      key: 'adapter_name',
      render: (name: string) => {
        const adapter = adapters.find(a => a.name === name);
        return (
          <Space>
            <span style={{ fontSize: '16px' }}>
              {adapter ? monitoringService.getAdapterIcon(adapter.type) : '📱'}
            </span>
            <Text strong>{name}</Text>
          </Space>
        );
      },
    },
    {
      title: '设备ID',
      dataIndex: 'device_id',
      key: 'device_id',
      render: (deviceId: string) => <Tag color="blue">{deviceId}</Tag>,
    },
    {
      title: '数据键',
      dataIndex: 'key',
      key: 'key',
      render: (key: string) => <Text code>{key}</Text>,
    },
    {
      title: '数据点/秒',
      dataIndex: 'data_points_per_sec',
      key: 'data_points_per_sec',
      render: (rate: number) => (
        <Statistic 
          value={rate.toFixed(1)} 
          valueStyle={{ fontSize: '14px', color: rate > 0 ? '#1890ff' : '#999' }}
          suffix="点/秒"
        />
      ),
    },
    {
      title: '字节/秒',
      dataIndex: 'bytes_per_sec',
      key: 'bytes_per_sec',
      render: (rate: number) => (
        <Text style={{ color: rate > 0 ? '#52c41a' : '#999' }}>
          {monitoringService.formatBytes(rate)}
        </Text>
      ),
    },
    {
      title: '延迟',
      dataIndex: 'latency_ms',
      key: 'latency_ms',
      render: (latency: number) => (
        <Tag color={latency > 100 ? 'red' : latency > 50 ? 'orange' : 'green'}>
          {monitoringService.formatLatency(latency)}
        </Tag>
      ),
    },
    {
      title: '错误率',
      dataIndex: 'error_rate',
      key: 'error_rate',
      render: (rate: number) => (
        <Text type={rate > 0.05 ? 'danger' : rate > 0.01 ? 'warning' : 'success'}>
          {(rate * 100).toFixed(2)}%
        </Text>
      ),
    },
    {
      title: '最后数值',
      dataIndex: 'last_value',
      key: 'last_value',
      render: (value: any, record: DataFlowMetrics) => {
        if (!value) return <Text type="secondary">-</Text>;
        
        const displayValue = typeof value === 'number' ? 
          value.toFixed(2) : 
          JSON.stringify(value);
        
        return (
          <Tooltip title={`时间: ${new Date(record.last_timestamp).toLocaleString()}`}>
            <Text code style={{ fontSize: '12px' }}>
              {displayValue}
            </Text>
          </Tooltip>
        );
      },
    },
  ];

  // Tab项目
  const tabItems = [
    {
      key: 'overview',
      label: (
        <span>
          <MonitorOutlined />
          系统概览
        </span>
      ),
      children: (
        <div>
          {/* 实时指标监控 */}
          <RealTimeMetrics autoRefresh={true} refreshInterval={5000} />
          
          {/* 系统概览统计 */}
          {overview && (
            <Row gutter={[24, 24]} style={{ marginBottom: 24 }}>
              <Col xs={24} sm={12} lg={6}>
                <Card>
                  <Statistic
                    title="系统健康"
                    value={overview.system_health}
                    prefix={getSystemHealthIcon(overview.system_health)}
                    valueStyle={{ 
                      color: overview.system_health === 'healthy' ? '#52c41a' : 
                             overview.system_health === 'degraded' ? '#faad14' : '#f5222d'
                    }}
                  />
                </Card>
              </Col>
              <Col xs={24} sm={12} lg={6}>
                <Card>
                  <Statistic
                    title="活跃连接"
                    value={overview.active_connections}
                    prefix={<ApiOutlined />}
                    suffix={`/ ${overview.total_adapters + overview.total_sinks}`}
                    valueStyle={{ color: '#1890ff' }}
                  />
                </Card>
              </Col>
              <Col xs={24} sm={12} lg={6}>
                <Card>
                  <Statistic
                    title="数据点/秒"
                    value={overview.total_data_points_per_sec.toFixed(1)}
                    prefix={<LineChartOutlined />}
                    valueStyle={{ color: '#722ed1' }}
                  />
                </Card>
              </Col>
              <Col xs={24} sm={12} lg={6}>
                <Card>
                  <Statistic
                    title="错误/秒"
                    value={overview.total_errors_per_sec.toFixed(2)}
                    prefix={<WarningOutlined />}
                    valueStyle={{ 
                      color: overview.total_errors_per_sec > 0 ? '#f5222d' : '#52c41a' 
                    }}
                  />
                </Card>
              </Col>
            </Row>
          )}

          {/* 适配器和连接器状态概览 */}
          {overview && (
            <Row gutter={[24, 24]}>
              <Col xs={24} lg={12}>
                <Card title="适配器状态" size="small">
                  <Row gutter={16}>
                    <Col span={8}>
                      <Statistic
                        title="总数"
                        value={overview.total_adapters}
                        prefix={<DatabaseOutlined />}
                      />
                    </Col>
                    <Col span={8}>
                      <Statistic
                        title="运行中"
                        value={overview.running_adapters}
                        valueStyle={{ color: '#1890ff' }}
                      />
                    </Col>
                    <Col span={8}>
                      <Statistic
                        title="健康"
                        value={overview.healthy_adapters}
                        valueStyle={{ color: '#52c41a' }}
                      />
                    </Col>
                  </Row>
                  <Divider />
                  <Progress
                    percent={overview.total_adapters > 0 ? (overview.healthy_adapters / overview.total_adapters) * 100 : 0}
                    strokeColor="#52c41a"
                    format={() => `${overview.healthy_adapters}/${overview.total_adapters} 健康`}
                  />
                </Card>
              </Col>
              <Col xs={24} lg={12}>
                <Card title="连接器状态" size="small">
                  <Row gutter={16}>
                    <Col span={8}>
                      <Statistic
                        title="总数"
                        value={overview.total_sinks}
                        prefix={<ThunderboltOutlined />}
                      />
                    </Col>
                    <Col span={8}>
                      <Statistic
                        title="运行中"
                        value={overview.running_sinks}
                        valueStyle={{ color: '#1890ff' }}
                      />
                    </Col>
                    <Col span={8}>
                      <Statistic
                        title="健康"
                        value={overview.healthy_sinks}
                        valueStyle={{ color: '#52c41a' }}
                      />
                    </Col>
                  </Row>
                  <Divider />
                  <Progress
                    percent={overview.total_sinks > 0 ? (overview.healthy_sinks / overview.total_sinks) * 100 : 0}
                    strokeColor="#52c41a"
                    format={() => `${overview.healthy_sinks}/${overview.total_sinks} 健康`}
                  />
                </Card>
              </Col>
            </Row>
          )}
        </div>
      ),
    },
    {
      key: 'adapters',
      label: (
        <span>
          <DatabaseOutlined />
          适配器监控
          {adapters.filter(a => a.status === 'error').length > 0 && (
            <Badge count={adapters.filter(a => a.status === 'error').length} style={{ marginLeft: 8 }} />
          )}
        </span>
      ),
      children: (
        <Card>
          <Table
            columns={adapterColumns}
            dataSource={adapters}
            rowKey="name"
            loading={loading}
            pagination={{ pageSize: 10 }}
            scroll={{ x: 1200 }}
          />
        </Card>
      ),
    },
    {
      key: 'sinks',
      label: (
        <span>
          <ThunderboltOutlined />
          连接器监控
          {sinks.filter(s => s.status === 'error').length > 0 && (
            <Badge count={sinks.filter(s => s.status === 'error').length} style={{ marginLeft: 8 }} />
          )}
        </span>
      ),
      children: (
        <Card>
          <Table
            columns={sinkColumns}
            dataSource={sinks}
            rowKey="name"
            loading={loading}
            pagination={{ pageSize: 10 }}
            scroll={{ x: 1000 }}
          />
        </Card>
      ),
    },
    {
      key: 'metrics',
      label: (
        <span>
          <LineChartOutlined />
          系统指标
        </span>
      ),
      children: (
        <div>
          {/* 系统指标图表 */}
          <SystemMetricsChart height={400} autoRefresh={true} refreshInterval={5000} />
          
          {/* 紧凑版实时指标 */}
          <div style={{ marginTop: 16 }}>
            <RealTimeMetrics autoRefresh={true} refreshInterval={5000} compact={true} />
          </div>
        </div>
      ),
    },
    {
      key: 'dataflow',
      label: (
        <span>
          <LineChartOutlined />
          数据流监控
        </span>
      ),
      children: (
        <div>
          {/* 数据流图表 */}
          <DataFlowChart height={400} autoRefresh={true} refreshInterval={10000} />
          
          {/* 数据流详细表格 */}
          <Card title="数据流详情" style={{ marginTop: 16 }}>
            <div style={{ marginBottom: 16 }}>
              <Space>
                <Text>时间范围:</Text>
                <Select
                  value={timeRange}
                  onChange={setTimeRange}
                  style={{ width: 120 }}
                >
                  {TIME_RANGE_OPTIONS.map(option => (
                    <Option key={option.value} value={option.value}>
                      {option.label}
                    </Option>
                  ))}
                </Select>
                <Button icon={<ReloadOutlined />} onClick={refreshData}>
                  刷新
                </Button>
              </Space>
            </div>
            {dataFlow.length > 0 ? (
              <>
                <Alert
                  message="数据流说明"
                  description="每行显示一个数据点的指标。如果一个适配器配置了多个数据点（如温度、湿度），每个数据点都会单独显示。数据点/秒表示该特定数据点的生成频率。"
                  type="info"
                  style={{ marginBottom: 16 }}
                  showIcon
                />
                {/* 适配器聚合统计 */}
                <Card title="适配器聚合统计" size="small" style={{ marginBottom: 16 }}>
                  <Row gutter={16}>
                    {(() => {
                      const adapterStats = dataFlow.reduce((acc, record) => {
                        const adapterName = record.adapter_name;
                        if (!acc[adapterName]) {
                          acc[adapterName] = {
                            name: adapterName,
                            device_id: record.device_id,
                            totalPoints: 0,
                            totalBytes: 0,
                            dataPointCount: 0,
                            avgLatency: 0,
                            avgErrorRate: 0,
                            keys: []
                          };
                        }
                        acc[adapterName].totalPoints += record.data_points_per_sec;
                        acc[adapterName].totalBytes += record.bytes_per_sec;
                        acc[adapterName].dataPointCount++;
                        acc[adapterName].avgLatency += record.latency_ms;
                        acc[adapterName].avgErrorRate += record.error_rate;
                        acc[adapterName].keys.push(record.key);
                        return acc;
                      }, {} as any);

                      return Object.values(adapterStats).map((stat: any) => (
                        <Col xs={24} sm={12} lg={8} key={stat.name}>
                          <Card size="small" style={{ marginBottom: 8 }}>
                            <Space>
                              <span style={{ fontSize: '16px' }}>
                                {adapters.find(a => a.name === stat.name)?.type === 'mock' ? '🎭' : '📱'}
                              </span>
                              <Text strong>{stat.name}</Text>
                            </Space>
                            <div style={{ marginTop: 8, fontSize: '12px' }}>
                              <div>设备: <Text code>{stat.device_id}</Text></div>
                              <div>数据点: <Text code>{stat.keys.join(', ')}</Text></div>
                              <div>总频率: <Text type="success">{stat.totalPoints.toFixed(1)} 点/秒</Text></div>
                              <div>平均延迟: <Text>{(stat.avgLatency / stat.dataPointCount).toFixed(1)} ms</Text></div>
                            </div>
                          </Card>
                        </Col>
                      ));
                    })()}
                  </Row>
                </Card>
                <Table
                  columns={dataFlowColumns}
                  dataSource={dataFlow}
                  rowKey={record => `${record.adapter_name}-${record.device_id}-${record.key}`}
                  pagination={{ pageSize: 10 }}
                  scroll={{ x: 1000 }}
                  expandable={{
                    expandedRowRender: (record) => (
                      <div style={{ padding: '16px', background: '#fafafa' }}>
                        <Row gutter={16}>
                          <Col span={12}>
                            <strong>数据点详情：</strong>
                            <ul style={{ marginTop: 8 }}>
                              <li>适配器: {record.adapter_name}</li>
                              <li>设备ID: {record.device_id}</li>
                              <li>数据键: {record.key}</li>
                              <li>最后更新: {new Date(record.last_timestamp).toLocaleString()}</li>
                            </ul>
                          </Col>
                          <Col span={12}>
                            <strong>性能指标：</strong>
                            <ul style={{ marginTop: 8 }}>
                              <li>数据点频率: {record.data_points_per_sec.toFixed(2)} 点/秒</li>
                              <li>数据传输: {monitoringService.formatBytes(record.bytes_per_sec)}/秒</li>
                              <li>网络延迟: {record.latency_ms.toFixed(1)} ms</li>
                              <li>错误率: {(record.error_rate * 100).toFixed(2)}%</li>
                            </ul>
                          </Col>
                        </Row>
                      </div>
                    ),
                    expandRowByClick: true,
                  }}
                />
              </>
            ) : (
              <Empty description="暂无数据流数据" />
            )}
          </Card>
        </div>
      ),
    },
  ];

  // 如果正在初始加载，显示加载状态
  if (loading && !adapters.length && !sinks.length && !overview) {
    return (
      <div style={{ textAlign: 'center', padding: '50px' }}>
        <Spin size="large" tip="加载监控数据中..." />
      </div>
    );
  }

  return (
    <div>
      <Row justify="space-between" align="middle" style={{ marginBottom: 24 }}>
        <Col>
          <Title level={2} style={{ margin: 0 }}>
            <MonitorOutlined /> 连接监控
          </Title>
          <Text type="secondary">实时监控适配器和连接器状态</Text>
        </Col>
        <Col>
          <Space>
            <Badge
              status={isConnected ? 'success' : 'error'}
              text={isConnected ? 'WebSocket已连接' : 'WebSocket断开'}
            />
            <Button icon={<ReloadOutlined />} onClick={refreshData} loading={loading}>
              刷新数据
            </Button>
          </Space>
        </Col>
      </Row>

      {/* 连接状态提示 */}
      {!isConnected && (
        <Alert
          message="实时连接断开"
          description="WebSocket连接已断开，部分实时功能可能不可用。"
          type="warning"
          showIcon
          closable
          style={{ marginBottom: 16 }}
        />
      )}

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={tabItems}
        size="large"
      />

      {/* 诊断信息弹窗 */}
      <Modal
        title={`适配器诊断 - ${selectedAdapter}`}
        open={diagnosticsVisible}
        onCancel={() => setDiagnosticsVisible(false)}
        footer={[
          <Button key="close" onClick={() => setDiagnosticsVisible(false)}>
            关闭
          </Button>,
        ]}
        width={800}
      >
        {diagnostics ? (
          <div>
            {/* 连接测试结果 */}
            {diagnostics.connection_test && (
              <Card title="连接测试" size="small" style={{ marginBottom: 16 }}>
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div>
                    <Text strong>状态: </Text>
                    <Tag color={diagnostics.connection_test.success ? 'success' : 'error'}>
                      {diagnostics.connection_test.success ? '成功' : '失败'}
                    </Tag>
                  </div>
                  <div>
                    <Text strong>响应时间: </Text>
                    <Text>{diagnostics.connection_test.response_time}ms</Text>
                  </div>
                  {diagnostics.connection_test.error && (
                    <div>
                      <Text strong>错误: </Text>
                      <Text type="danger">{diagnostics.connection_test.error}</Text>
                    </div>
                  )}
                </Space>
              </Card>
            )}

            {/* 健康检查结果 */}
            {diagnostics.health_checks.length > 0 && (
              <Card title="健康检查" size="small" style={{ marginBottom: 16 }}>
                <Space direction="vertical" style={{ width: '100%' }}>
                  {diagnostics.health_checks.map((check, index) => (
                    <div key={index}>
                      <Space>
                        <Tag color={check.status === 'pass' ? 'success' : check.status === 'warn' ? 'warning' : 'error'}>
                          {check.check_name}
                        </Tag>
                        <Text>{check.message}</Text>
                        <Text type="secondary">({check.duration}ms)</Text>
                      </Space>
                    </div>
                  ))}
                </Space>
              </Card>
            )}

            {/* 性能测试结果 */}
            {diagnostics.performance_test && (
              <Card title="性能测试" size="small" style={{ marginBottom: 16 }}>
                <Row gutter={16}>
                  <Col span={12}>
                    <Statistic
                      title="吞吐量"
                      value={diagnostics.performance_test.throughput_per_sec}
                      suffix="ops/sec"
                    />
                  </Col>
                  <Col span={12}>
                    <Statistic
                      title="平均延迟"
                      value={diagnostics.performance_test.avg_latency}
                      suffix="ns"
                    />
                  </Col>
                </Row>
              </Card>
            )}

            {/* 优化建议 */}
            {diagnostics.recommendations.length > 0 && (
              <Card title="优化建议" size="small">
                <ul>
                  {diagnostics.recommendations.map((rec, index) => (
                    <li key={index}>{rec}</li>
                  ))}
                </ul>
              </Card>
            )}
          </div>
        ) : (
          <Spin />
        )}
      </Modal>
    </div>
  );
};

export default MonitoringPage;