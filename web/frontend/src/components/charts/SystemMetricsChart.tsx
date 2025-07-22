import React, { useState, useEffect, useRef } from 'react';
import { Card, Select, Space, Typography, Button, Spin, Alert } from 'antd';
import { LineChartOutlined, ReloadOutlined } from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import { lightweightMetricsService, type LightweightMetrics } from '../../services/lightweightMetricsService';

const { Option } = Select;
const { Text } = Typography;

interface SystemMetricsChartProps {
  height?: number;
  autoRefresh?: boolean;
  refreshInterval?: number;
}

interface MetricDataPoint {
  timestamp: number;
  value: number;
}

interface ChartData {
  cpu: MetricDataPoint[];
  memory: MetricDataPoint[];
  goroutines: MetricDataPoint[];
  dataPoints: MetricDataPoint[];
  errors: MetricDataPoint[];
}

const SystemMetricsChart: React.FC<SystemMetricsChartProps> = ({
  height = 300,
  autoRefresh = true,
  refreshInterval = 5000,
}) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedMetric, setSelectedMetric] = useState<string>('cpu');
  const [chartData, setChartData] = useState<ChartData>({
    cpu: [],
    memory: [],
    goroutines: [],
    dataPoints: [],
    errors: [],
  });
  const [currentMetrics, setCurrentMetrics] = useState<LightweightMetrics | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  const fetchMetrics = async () => {
    try {
      setError(null);
      const metrics = await lightweightMetricsService.getLightweightMetrics();
      setCurrentMetrics(metrics);
      
      const now = Date.now();
      const memoryUsagePercent = metrics.system.heap_size_bytes > 0 
        ? (metrics.system.memory_usage_bytes / metrics.system.heap_size_bytes) * 100 
        : 0;
      
      setChartData(prev => {
        const maxDataPoints = 50; // 保留最近50个数据点
        
        const addDataPoint = (data: MetricDataPoint[], value: number) => {
          const newData = [...data, { timestamp: now, value }];
          return newData.slice(-maxDataPoints);
        };
        
        return {
          cpu: addDataPoint(prev.cpu, metrics.system.cpu_usage_percent),
          memory: addDataPoint(prev.memory, memoryUsagePercent),
          goroutines: addDataPoint(prev.goroutines, metrics.system.goroutine_count),
          dataPoints: addDataPoint(prev.dataPoints, metrics.data.data_points_per_second),
          errors: addDataPoint(prev.errors, metrics.errors.errors_per_second),
        };
      });
    } catch (err: any) {
      setError(err.message || '获取指标失败');
      console.error('Failed to fetch metrics:', err);
    }
  };

  const handleRefresh = async () => {
    setLoading(true);
    await fetchMetrics();
    setLoading(false);
  };

  useEffect(() => {
    fetchMetrics();
  }, []);

  useEffect(() => {
    if (!autoRefresh) return;

    intervalRef.current = setInterval(fetchMetrics, refreshInterval);
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [autoRefresh, refreshInterval]);

  const getChartOption = () => {
    const data = chartData[selectedMetric as keyof ChartData] || [];
    
    const getMetricConfig = (metric: string) => {
      switch (metric) {
        case 'cpu':
          return {
            title: 'CPU使用率 (%)',
            color: '#1890ff',
            yMax: 100,
            formatter: (value: number) => `${value.toFixed(1)}%`,
          };
        case 'memory':
          return {
            title: '内存使用率 (%)',
            color: '#52c41a',
            yMax: 100,
            formatter: (value: number) => `${value.toFixed(1)}%`,
          };
        case 'goroutines':
          return {
            title: 'Goroutines数量',
            color: '#722ed1',
            yMax: null,
            formatter: (value: number) => `${Math.round(value)}`,
          };
        case 'dataPoints':
          return {
            title: '数据点/秒',
            color: '#13c2c2',
            yMax: null,
            formatter: (value: number) => `${value.toFixed(1)}/s`,
          };
        case 'errors':
          return {
            title: '错误/秒',
            color: '#f5222d',
            yMax: null,
            formatter: (value: number) => `${value.toFixed(2)}/s`,
          };
        default:
          return {
            title: '未知指标',
            color: '#d9d9d9',
            yMax: null,
            formatter: (value: number) => `${value}`,
          };
      }
    };

    const config = getMetricConfig(selectedMetric);
    
    return {
      title: {
        text: config.title,
        left: 'center',
        textStyle: {
          fontSize: 14,
          fontWeight: 'normal',
        },
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: {
          type: 'cross',
          label: {
            backgroundColor: '#6a7985',
          },
        },
        formatter: (params: any) => {
          const point = params[0];
          const time = new Date(point.axisValue).toLocaleTimeString();
          const value = config.formatter(point.value);
          return `${time}<br/>${config.title}: ${value}`;
        },
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '3%',
        containLabel: true,
      },
      xAxis: {
        type: 'time',
        boundaryGap: false,
        axisLabel: {
          formatter: (value: number) => new Date(value).toLocaleTimeString(),
        },
        splitLine: {
          show: false,
        },
      },
      yAxis: {
        type: 'value',
        max: config.yMax,
        axisLabel: {
          formatter: config.formatter,
        },
        splitLine: {
          lineStyle: {
            type: 'dashed',
          },
        },
      },
      series: [
        {
          name: config.title,
          type: 'line',
          data: data.map(point => [point.timestamp, point.value]),
          smooth: true,
          lineStyle: {
            color: config.color,
          },
          itemStyle: {
            color: config.color,
          },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                {
                  offset: 0,
                  color: config.color + '40',
                },
                {
                  offset: 1,
                  color: config.color + '10',
                },
              ],
            },
          },
          symbol: 'circle',
          symbolSize: 4,
          showSymbol: false,
        },
      ],
    };
  };

  const metricOptions = [
    { value: 'cpu', label: 'CPU使用率' },
    { value: 'memory', label: '内存使用率' },
    { value: 'goroutines', label: 'Goroutines' },
    { value: 'dataPoints', label: '数据点/秒' },
    { value: 'errors', label: '错误/秒' },
  ];

  const getCurrentValue = () => {
    if (!currentMetrics) return 'N/A';
    
    switch (selectedMetric) {
      case 'cpu':
        return `${currentMetrics.system.cpu_usage_percent.toFixed(1)}%`;
      case 'memory':
        return currentMetrics.system.heap_size_bytes > 0
          ? `${((currentMetrics.system.memory_usage_bytes / currentMetrics.system.heap_size_bytes) * 100).toFixed(1)}%`
          : 'N/A';
      case 'goroutines':
        return `${currentMetrics.system.goroutine_count}`;
      case 'dataPoints':
        return `${currentMetrics.data.data_points_per_second.toFixed(1)}/s`;
      case 'errors':
        return `${currentMetrics.errors.errors_per_second.toFixed(2)}/s`;
      default:
        return 'N/A';
    }
  };

  return (
    <Card
      title={
        <Space>
          <LineChartOutlined />
          <span>系统指标趋势</span>
        </Space>
      }
      extra={
        <Space>
          <Select
            value={selectedMetric}
            onChange={setSelectedMetric}
            style={{ width: 120 }}
            size="small"
          >
            {metricOptions.map(option => (
              <Option key={option.value} value={option.value}>
                {option.label}
              </Option>
            ))}
          </Select>
          <Button
            icon={<ReloadOutlined />}
            onClick={handleRefresh}
            loading={loading}
            size="small"
          >
            刷新
          </Button>
        </Space>
      }
      size="small"
    >
      {error && (
        <Alert
          message="数据加载失败"
          description={error}
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}
      
      <div style={{ marginBottom: 16 }}>
        <Space>
          <Text strong>当前值:</Text>
          <Text>{getCurrentValue()}</Text>
          <Text type="secondary">
            ({chartData[selectedMetric as keyof ChartData]?.length || 0} 个数据点)
          </Text>
        </Space>
      </div>
      
      {loading && chartData[selectedMetric as keyof ChartData]?.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '40px 0' }}>
          <Spin size="large" />
          <div style={{ marginTop: 16 }}>
            <Text>加载指标数据中...</Text>
          </div>
        </div>
      ) : (
        <ReactECharts
          option={getChartOption()}
          style={{ height: height }}
          notMerge={true}
          lazyUpdate={true}
        />
      )}
    </Card>
  );
};

export default SystemMetricsChart;