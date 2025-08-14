import React, { useMemo, useState, useEffect, useCallback } from 'react';
import { Row, Col, Card, Statistic, Progress } from 'antd';
import { RealTimeChart, ChartSeries } from './RealTimeChart';
import { useRealTimeData } from '../../hooks/useRealTimeData';
import { lightweightMetricsService, type LightweightMetrics } from '../../services/lightweightMetricsService';
import { authService } from '../../services/authService';

export interface SystemMonitorChartProps {
  height?: number;
  showDetailedCharts?: boolean;
}

export const SystemMonitorChart: React.FC<SystemMonitorChartProps> = ({
  height = 300,
  showDetailedCharts = true,
}) => {
  const { data, isConnected } = useRealTimeData();
  const [metricsData, setMetricsData] = useState<LightweightMetrics | null>(null);
  const [systemMetrics, setSystemMetrics] = useState<any>(null); // 真正的系统指标
  const [metricsHistory, setMetricsHistory] = useState<any[]>([]);
  
  // 获取系统指标数据
  const fetchMetrics = useCallback(async () => {
    try {
      // 并行获取轻量级指标和系统指标
      const [metrics, systemStatus] = await Promise.all([
        lightweightMetricsService.getLightweightMetrics(),
        fetchSystemMetrics()
      ]);
      
      setMetricsData(metrics);
      setSystemMetrics(systemStatus);
      
      // 构建历史数据点 - 使用真正的系统内存使用率
      const historyPoint = {
        timestamp: new Date(),
        cpu: systemStatus?.cpu_usage || metrics.system.cpu_usage_percent,
        memory: systemStatus?.memory_usage || 0, // 使用真正的系统内存使用率
        disk: systemStatus?.disk_usage || metrics.system.disk_usage_percent || 0,
        cpu_percent: systemStatus?.cpu_usage || metrics.system.cpu_usage_percent,
        memory_percent: systemStatus?.memory_usage || 0,
        disk_percent: systemStatus?.disk_usage || metrics.system.disk_usage_percent || 0,
        memory_used: 0, // 系统API没有提供具体的内存使用量
        memory_total: 0, // 系统API没有提供总内存大小
        disk_used: 0,
        disk_total: 0,
        network_rx: systemStatus?.network_in_bytes || 0,
        network_tx: systemStatus?.network_out_bytes || 0,
        process_count: metrics.system.goroutine_count || 0,
        thread_count: 0,
        // Go程序特有指标
        heap_used: metrics.system.heap_in_use_bytes,
        heap_total: metrics.system.heap_size_bytes,
        heap_usage_percent: metrics.system.heap_size_bytes > 0 
          ? (metrics.system.heap_in_use_bytes / metrics.system.heap_size_bytes) * 100
          : 0,
      };
      
      setMetricsHistory(prev => {
        const newHistory = [...prev, historyPoint];
        if (newHistory.length > 50) {
          newHistory.splice(0, newHistory.length - 50);
        }
        return newHistory;
      });
      
      console.log('✅ SystemMonitor获取系统指标成功:', { metrics, systemStatus });
    } catch (error) {
      console.error('❌ SystemMonitor获取指标失败:', error);
    }
  }, []);
  
  // 获取真正的系统指标
  const fetchSystemMetrics = useCallback(async () => {
    try {
      const token = authService.getToken();
      if (!token) {
        throw new Error('No auth token available');
      }
      
      const response = await fetch('/api/v1/system/metrics', {
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }
      
      const result = await response.json();
      return result.data;
    } catch (error) {
      console.error('❌ 获取系统指标失败:', error);
      return null;
    }
  }, []);
  
  // 定期获取指标数据
  useEffect(() => {
    fetchMetrics(); // 初始获取
    const interval = setInterval(fetchMetrics, 10000); // 每10秒更新一次
    return () => clearInterval(interval);
  }, [fetchMetrics]);

  // 准备系统指标数据 - 优先使用API数据，WebSocket作为备选
  const systemMetricsHistory = useMemo(() => {
    // 优先使用轻量级指标API构建的历史数据
    if (metricsHistory && metricsHistory.length > 0) {
      return metricsHistory.map(metrics => ({
        timestamp: new Date(metrics.timestamp || Date.now()),
        cpu: metrics.cpu_percent || 0,
        memory: metrics.memory_percent || 0,
        disk: metrics.disk_percent || 0,
      }));
    }
    
    // 备选：使用WebSocket实时数据
    if (data.systemMetricsHistory && data.systemMetricsHistory.length > 0) {
      return data.systemMetricsHistory.map(metrics => ({
        timestamp: new Date(metrics.timestamp || Date.now()),
        cpu: metrics.cpu_percent || 0,
        memory: metrics.memory_percent || 0,
        disk: metrics.disk_percent || 0,
      }));
    }
    
    return [];
  }, [metricsHistory, data.systemMetricsHistory]);
  
  // 当前系统指标 - 使用真正的系统指标
  const currentSystemMetrics = useMemo(() => {
    if (systemMetrics && metricsData) {
      return {
        // 使用真正的系统资源使用率
        cpu_percent: systemMetrics.cpu_usage || metricsData.system.cpu_usage_percent,
        memory_percent: systemMetrics.memory_usage || 0, // 真正的系统内存使用率
        disk_percent: systemMetrics.disk_usage || metricsData.system.disk_usage_percent || 0,
        
        // 网络累计流量（字节）- 优先从轻量级指标获取，系统API作为备选
        network_rx: metricsData.system.network_in_bytes || systemMetrics.network_in_bytes || 0,
        network_tx: metricsData.system.network_out_bytes || systemMetrics.network_out_bytes || 0,
        
        // 网络实时速率（字节/秒）
        network_rx_rate: metricsData.system.network_in_bytes_per_sec || systemMetrics.network_in_bytes_per_sec || 0,
        network_tx_rate: metricsData.system.network_out_bytes_per_sec || systemMetrics.network_out_bytes_per_sec || 0,
        
        // Go程序指标
        process_count: metricsData.system.goroutine_count || 0,
        thread_count: 0,
        
        // 堆内存信息（独立显示）
        heap_used: metricsData.system.heap_in_use_bytes,
        heap_total: metricsData.system.heap_size_bytes,
        heap_usage_percent: metricsData.system.heap_size_bytes > 0 
          ? (metricsData.system.heap_in_use_bytes / metricsData.system.heap_size_bytes) * 100
          : 0,
          
        // 系统状态
        status: metricsData.gateway.status || 'unknown',
        
        // 用于显示的内存值（估算，因为API没提供具体值）
        memory_used: 0, // 系统API没有提供
        memory_total: 0, // 系统API没有提供
        disk_used: 0,
        disk_total: 0,
      };
    }
    
    // 备选：使用WebSocket数据
    return data.systemMetrics || null;
  }, [systemMetrics, metricsData, data.systemMetrics]);

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
              value={currentSystemMetrics?.status || data.systemStatus?.status || '--'}
              valueStyle={{ 
                color: (currentSystemMetrics?.status === 'running' || data.systemStatus?.status === 'running') ? '#52c41a' : '#faad14',
                fontSize: '16px'
              }}
            />
          </Card>
        </Col>
      </Row>

      {/* 当前系统指标 */}
      {currentSystemMetrics && (
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={24} md={8}>
            <Card title="CPU 使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(currentSystemMetrics.cpu_percent || 0)}
                status={currentSystemMetrics.cpu_percent > 80 ? 'exception' : 'active'}
                strokeColor={currentSystemMetrics.cpu_percent > 80 ? '#ff4d4f' : '#1890ff'}
                size={80}
              />
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card title="内存使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(currentSystemMetrics.memory_percent || 0)}
                status={currentSystemMetrics.memory_percent > 85 ? 'exception' : 'active'}
                strokeColor={currentSystemMetrics.memory_percent > 85 ? '#ff4d4f' : '#52c41a'}
                size={80}
              />
              <div style={{ textAlign: 'center', marginTop: 8, fontSize: '12px', color: '#666' }}>
                系统内存使用率: {currentSystemMetrics.memory_percent.toFixed(1)}%
              </div>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card title="磁盘使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(currentSystemMetrics.disk_percent || 0)}
                status={currentSystemMetrics.disk_percent > 90 ? 'exception' : 'active'}
                strokeColor={currentSystemMetrics.disk_percent > 90 ? '#ff4d4f' : '#faad14'}
                size={80}
              />
              <div style={{ textAlign: 'center', marginTop: 8, fontSize: '12px', color: '#666' }}>
                {formatBytes(currentSystemMetrics.disk_used || 0)} / {formatBytes(currentSystemMetrics.disk_total || 0)}
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
              loading={!isConnected && !metricsData}
            />
          </Col>
          <Col xs={24} lg={8}>
            <RealTimeChart
              title="CPU 使用率"
              series={cpuChartSeries}
              height={height - 50}
              yAxisLabel="使用率 (%)"
              maxDataPoints={30}
              loading={!isConnected && !metricsData}
            />
          </Col>
          <Col xs={24} lg={8}>
            <RealTimeChart
              title="内存使用率"
              series={memoryChartSeries}
              height={height - 50}
              yAxisLabel="使用率 (%)"
              maxDataPoints={30}
              loading={!isConnected && !metricsData}
            />
          </Col>
          <Col xs={24} lg={8}>
            <RealTimeChart
              title="磁盘使用率"
              series={diskChartSeries}
              height={height - 50}
              yAxisLabel="使用率 (%)"
              maxDataPoints={30}
              loading={!isConnected && !metricsData}
            />
          </Col>
        </Row>
      )}

      {/* Go程序堆内存监控 */}
      {currentSystemMetrics && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} md={8}>
            <Card title="Go堆内存使用率" size="small">
              <Progress
                type="circle"
                percent={Math.round(currentSystemMetrics.heap_usage_percent || 0)}
                status={currentSystemMetrics.heap_usage_percent > 80 ? 'exception' : 'active'}
                strokeColor={currentSystemMetrics.heap_usage_percent > 80 ? '#ff4d4f' : '#722ed1'}
                size={80}
              />
              <div style={{ textAlign: 'center', marginTop: 8, fontSize: '12px', color: '#666' }}>
                {formatBytes(currentSystemMetrics.heap_used || 0)} / {formatBytes(currentSystemMetrics.heap_total || 0)}
              </div>
            </Card>
          </Col>
          <Col xs={24} md={16}>
            <Card title="系统与应用指标" size="small">
              <Row gutter={[16, 16]}>
                <Col xs={24} sm={8} lg={4}>
                  <Statistic
                    title="网络接收(累计)"
                    value={currentSystemMetrics ? formatBytes(currentSystemMetrics.network_rx) : '--'}
                    valueStyle={{ fontSize: '12px', color: currentSystemMetrics.network_rx > 0 ? '#52c41a' : '#999' }}
                  />
                </Col>
                <Col xs={24} sm={8} lg={4}>
                  <Statistic
                    title="网络发送(累计)"
                    value={currentSystemMetrics ? formatBytes(currentSystemMetrics.network_tx) : '--'}
                    valueStyle={{ fontSize: '12px', color: currentSystemMetrics.network_tx > 0 ? '#52c41a' : '#999' }}
                  />
                </Col>
                <Col xs={24} sm={8} lg={4}>
                  <Statistic
                    title="接收速率"
                    value={currentSystemMetrics && currentSystemMetrics.network_rx_rate ? `${formatBytes(currentSystemMetrics.network_rx_rate)}/s` : '--'}
                    valueStyle={{ fontSize: '12px', color: currentSystemMetrics?.network_rx_rate > 0 ? '#1890ff' : '#999' }}
                  />
                </Col>
                <Col xs={24} sm={8} lg={4}>
                  <Statistic
                    title="发送速率"
                    value={currentSystemMetrics && currentSystemMetrics.network_tx_rate ? `${formatBytes(currentSystemMetrics.network_tx_rate)}/s` : '--'}
                    valueStyle={{ fontSize: '12px', color: currentSystemMetrics?.network_tx_rate > 0 ? '#1890ff' : '#999' }}
                  />
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Statistic
                    title="Goroutines"
                    value={currentSystemMetrics ? currentSystemMetrics.process_count || 0 : '--'}
                    valueStyle={{ fontSize: '14px', color: (currentSystemMetrics?.process_count || 0) > 0 ? '#1890ff' : '#999' }}
                  />
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Statistic
                    title="GC暂停时间"
                    value={metricsData && metricsData.system.gc_pause_ms >= 0 ? `${metricsData.system.gc_pause_ms.toFixed(2)}ms` : '--'}
                    valueStyle={{ 
                      fontSize: '14px',
                      color: metricsData && metricsData.system.gc_pause_ms > 10 ? '#faad14' : metricsData && metricsData.system.gc_pause_ms > 0 ? '#52c41a' : '#999'
                    }}
                  />
                </Col>
              </Row>
            </Card>
          </Col>
        </Row>
      )}
      
      {/* 系统版本信息 */}
      {metricsData && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="系统版本"
                value={metricsData.system.version || '--'}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="Go版本"
                value={metricsData.system.go_version || '--'}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="堆内存"
                value={formatBytes(metricsData.system.heap_in_use_bytes || 0)}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card size="small">
              <Statistic
                title="运行时间"
                value={lightweightMetricsService.formatUptime(metricsData.system.uptime_seconds)}
                valueStyle={{ fontSize: '14px' }}
              />
            </Card>
          </Col>
        </Row>
      )}
    </div>
  );
};