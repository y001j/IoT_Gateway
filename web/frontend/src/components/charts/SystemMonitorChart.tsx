import React, { useMemo } from 'react';
import { Row, Col, Card, Statistic, Progress } from 'antd';
import { RealTimeChart, ChartSeries } from './RealTimeChart';
import { useRealTimeData } from '../../hooks/useRealTimeData';

export interface SystemMonitorChartProps {
  height?: number;
  showDetailedCharts?: boolean;
}

export const SystemMonitorChart: React.FC<SystemMonitorChartProps> = ({
  height = 300,
  showDetailedCharts = true,
}) => {
  const { data, isConnected } = useRealTimeData();

  // 准备实时系统指标数据
  const systemMetricsHistory = useMemo(() => {
    // 使用历史数据而不是单一数据点
    if (!data.systemMetricsHistory || data.systemMetricsHistory.length === 0) {
      return [];
    }
    
    return data.systemMetricsHistory.map(metrics => ({
      timestamp: new Date(metrics.timestamp || Date.now()),
      cpu: metrics.cpu_percent || 0,
      memory: metrics.memory_percent || 0,
      disk: metrics.disk_percent || 0,
    }));
  }, [data.systemMetricsHistory]);

  // CPU 使用率图表数据
  const cpuChartSeries: ChartSeries[] = useMemo(() => [{
    name: 'CPU 使用率',
    data: systemMetricsHistory.map(item => ({
      timestamp: item.timestamp,
      value: item.cpu,
    })),
    color: '#1890ff',
    type: 'area',
  }], [systemMetricsHistory]);

  // 内存使用率图表数据
  const memoryChartSeries: ChartSeries[] = useMemo(() => [{
    name: '内存使用率',
    data: systemMetricsHistory.map(item => ({
      timestamp: item.timestamp,
      value: item.memory,
    })),
    color: '#52c41a',
    type: 'area',
  }], [systemMetricsHistory]);

  // 磁盘使用率图表数据
  const diskChartSeries: ChartSeries[] = useMemo(() => [{
    name: '磁盘使用率',
    data: systemMetricsHistory.map(item => ({
      timestamp: item.timestamp,
      value: item.disk,
    })),
    color: '#faad14',
    type: 'area',
  }], [systemMetricsHistory]);

  // 综合系统指标图表
  const combinedChartSeries: ChartSeries[] = useMemo(() => [
    {
      name: 'CPU',
      data: systemMetricsHistory.map(item => ({
        timestamp: item.timestamp,
        value: item.cpu,
      })),
      color: '#1890ff',
      type: 'line',
    },
    {
      name: '内存',
      data: systemMetricsHistory.map(item => ({
        timestamp: item.timestamp,
        value: item.memory,
      })),
      color: '#52c41a',
      type: 'line',
    },
    {
      name: '磁盘',
      data: systemMetricsHistory.map(item => ({
        timestamp: item.timestamp,
        value: item.disk,
      })),
      color: '#faad14',
      type: 'line',
    },
  ], [systemMetricsHistory]);

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
  };

  return (
    <div>
      {/* 系统状态概览 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title="连接状态"
              value={isConnected ? "已连接" : "未连接"}
              valueStyle={{ 
                color: isConnected ? '#52c41a' : '#f5222d',
                fontSize: '16px'
              }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title="最后更新"
              value={new Date().toLocaleTimeString()}
              valueStyle={{ fontSize: '16px' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title="系统状态"
              value={data.systemStatus?.status || '--'}
              valueStyle={{ 
                color: data.systemStatus?.status === 'running' ? '#52c41a' : '#faad14',
                fontSize: '16px'
              }}
            />
          </Card>
        </Col>
      </Row>

      {/* 当前系统指标 */}
      {data.systemMetrics && (
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={24} md={8}>
            <Card title="CPU 使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(data.systemMetrics.cpu_percent || 0)}
                status={data.systemMetrics.cpu_percent > 80 ? 'exception' : 'active'}
                strokeColor={data.systemMetrics.cpu_percent > 80 ? '#ff4d4f' : '#1890ff'}
                size={80}
              />
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card title="内存使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(data.systemMetrics.memory_percent || 0)}
                status={data.systemMetrics.memory_percent > 85 ? 'exception' : 'active'}
                strokeColor={data.systemMetrics.memory_percent > 85 ? '#ff4d4f' : '#52c41a'}
                size={80}
              />
              <div style={{ textAlign: 'center', marginTop: 8, fontSize: '12px', color: '#666' }}>
                {formatBytes(data.systemMetrics.memory_used || 0)} / {formatBytes(data.systemMetrics.memory_total || 0)}
              </div>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card title="磁盘使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(data.systemMetrics.disk_percent || 0)}
                status={data.systemMetrics.disk_percent > 90 ? 'exception' : 'active'}
                strokeColor={data.systemMetrics.disk_percent > 90 ? '#ff4d4f' : '#faad14'}
                size={80}
              />
              <div style={{ textAlign: 'center', marginTop: 8, fontSize: '12px', color: '#666' }}>
                {formatBytes(data.systemMetrics.disk_used || 0)} / {formatBytes(data.systemMetrics.disk_total || 0)}
              </div>
            </Card>
          </Col>
        </Row>
      )}

      {/* 实时图表 */}
      {showDetailedCharts && (
        <Row gutter={[16, 16]}>
          <Col xs={24}>
            <RealTimeChart
              title="系统资源使用趋势"
              series={combinedChartSeries}
              height={height}
              yAxisLabel="使用率 (%)"
              maxDataPoints={50}
              loading={!isConnected}
            />
          </Col>
          <Col xs={24} lg={8}>
            <RealTimeChart
              title="CPU 使用率"
              series={cpuChartSeries}
              height={height - 50}
              yAxisLabel="使用率 (%)"
              maxDataPoints={30}
              loading={!isConnected}
            />
          </Col>
          <Col xs={24} lg={8}>
            <RealTimeChart
              title="内存使用率"
              series={memoryChartSeries}
              height={height - 50}
              yAxisLabel="使用率 (%)"
              maxDataPoints={30}
              loading={!isConnected}
            />
          </Col>
          <Col xs={24} lg={8}>
            <RealTimeChart
              title="磁盘使用率"
              series={diskChartSeries}
              height={height - 50}
              yAxisLabel="使用率 (%)"
              maxDataPoints={30}
              loading={!isConnected}
            />
          </Col>
        </Row>
      )}

      {/* 网络和进程信息 */}
      {data.systemMetrics && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="网络接收"
                value={formatBytes(data.systemMetrics.network_rx || 0)}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="网络发送"
                value={formatBytes(data.systemMetrics.network_tx || 0)}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="进程数"
                value={data.systemMetrics.process_count || 0}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="线程数"
                value={data.systemMetrics.thread_count || 0}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
        </Row>
      )}
    </div>
  );
};