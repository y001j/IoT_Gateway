import React, { useEffect, useRef, useMemo } from 'react';
import { Card, Empty, Spin } from 'antd';
import * as echarts from 'echarts';

export interface DataPoint {
  timestamp: Date | string;
  value: number;
  [key: string]: any;
}

export interface ChartSeries {
  name: string;
  data: DataPoint[];
  color?: string;
  type?: 'line' | 'bar' | 'area';
  smooth?: boolean;
}

export interface RealTimeChartProps {
  title?: string;
  series: ChartSeries[];
  height?: number;
  width?: string;
  maxDataPoints?: number;
  refreshInterval?: number;
  loading?: boolean;
  showLegend?: boolean;
  showGrid?: boolean;
  xAxisLabel?: string;
  yAxisLabel?: string;
  timeFormat?: string;
  className?: string;
  style?: React.CSSProperties;
}

const defaultColors = [
  '#1890ff',
  '#52c41a',
  '#faad14',
  '#f5222d',
  '#722ed1',
  '#13c2c2',
  '#eb2f96',
  '#fa8c16',
];

export const RealTimeChart: React.FC<RealTimeChartProps> = ({
  title,
  series,
  height = 300,
  width = '100%',
  maxDataPoints = 50,
  loading = false,
  showLegend = true,
  showGrid = true,
  xAxisLabel = '时间',
  yAxisLabel = '数值',
  timeFormat = 'HH:mm:ss',
  className,
  style,
}) => {
  const chartRef = useRef<HTMLDivElement>(null);
  const chartInstance = useRef<echarts.ECharts | null>(null);

  // 准备图表数据
  const chartData = useMemo(() => {
    if (!series || series.length === 0) {
      return null;
    }

    return series.map((s, index) => {
      // 限制数据点数量并排序
      const limitedData = s.data
        .slice(-maxDataPoints)
        .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());

      return {
        name: s.name,
        type: s.type === 'area' ? 'line' : (s.type || 'line'),
        data: limitedData.map(point => [
          new Date(point.timestamp).getTime(),
          point.value
        ]),
        smooth: s.smooth !== false,
        lineStyle: {
          color: s.color || defaultColors[index % defaultColors.length],
          width: 2,
        },
        itemStyle: {
          color: s.color || defaultColors[index % defaultColors.length],
        },
        areaStyle: s.type === 'area' ? {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: s.color || defaultColors[index % defaultColors.length] },
            { offset: 1, color: 'transparent' }
          ]),
          opacity: 0.3,
        } : undefined,
        symbol: 'circle',
        symbolSize: 4,
        showSymbol: false,
        animation: true,
        animationDuration: 300,
      };
    });
  }, [series, maxDataPoints]);

  // 初始化图表
  useEffect(() => {
    if (!chartRef.current) return;

    // 创建图表实例
    chartInstance.current = echarts.init(chartRef.current);

    // 处理窗口大小变化
    const handleResize = () => {
      chartInstance.current?.resize();
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chartInstance.current?.dispose();
      chartInstance.current = null;
    };
  }, []);

  // 更新图表配置
  useEffect(() => {
    if (!chartInstance.current || !chartData) return;

    const option: echarts.EChartsOption = {
      title: title ? {
        text: title,
        left: 'center',
        textStyle: {
          fontSize: 14,
          fontWeight: 'normal',
        },
      } : undefined,
      tooltip: {
        trigger: 'axis',
        axisPointer: {
          type: 'cross',
          label: {
            backgroundColor: '#6a7985'
          }
        },
        formatter: function (params: any) {
          if (!Array.isArray(params)) params = [params];
          
          const time = new Date(params[0].axisValue).toLocaleTimeString();
          let content = `时间: ${time}<br/>`;
          
          params.forEach((param: any) => {
            const color = param.color;
            const value = typeof param.value[1] === 'number' 
              ? param.value[1].toFixed(2) 
              : param.value[1];
            content += `<span style="color:${color}">●</span> ${param.seriesName}: ${value}<br/>`;
          });
          
          return content;
        }
      },
      legend: showLegend ? {
        data: series.map(s => s.name),
        bottom: 0,
        textStyle: {
          fontSize: 12,
        },
      } : undefined,
      grid: {
        left: '3%',
        right: '4%',
        bottom: showLegend ? '15%' : '8%',
        containLabel: true,
        show: showGrid,
      },
      xAxis: {
        type: 'time',
        name: xAxisLabel,
        nameLocation: 'middle',
        nameGap: 25,
        axisLine: {
          show: true,
          lineStyle: {
            color: '#d9d9d9',
          },
        },
        axisTick: {
          show: true,
        },
        axisLabel: {
          formatter: function (value: number) {
            return echarts.format.formatTime(timeFormat, value);
          },
          fontSize: 10,
        },
        splitLine: {
          show: showGrid,
          lineStyle: {
            type: 'dashed',
            color: '#f0f0f0',
          },
        },
      },
      yAxis: {
        type: 'value',
        name: yAxisLabel,
        nameLocation: 'middle',
        nameGap: 40,
        scale: true,
        axisLine: {
          show: true,
          lineStyle: {
            color: '#d9d9d9',
          },
        },
        axisTick: {
          show: true,
        },
        axisLabel: {
          fontSize: 10,
          formatter: function (value: number) {
            if (Math.abs(value) >= 1000000) {
              return (value / 1000000).toFixed(1) + 'M';
            } else if (Math.abs(value) >= 1000) {
              return (value / 1000).toFixed(1) + 'K';
            }
            return value.toFixed(1);
          },
        },
        splitLine: {
          show: showGrid,
          lineStyle: {
            type: 'dashed',
            color: '#f0f0f0',
          },
        },
      },
      series: chartData,
      animation: true,
      animationDuration: 300,
      animationEasing: 'cubicOut',
    };

    chartInstance.current.setOption(option, true);
  }, [chartData, title, showLegend, showGrid, xAxisLabel, yAxisLabel, timeFormat, series]);

  // 处理空数据状态
  if (!series || series.length === 0) {
    return (
      <Card 
        title={title} 
        className={className} 
        style={{ height, width, ...style }}
      >
        <div style={{ 
          height: height - 100, 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'center' 
        }}>
          <Empty 
            description="暂无数据" 
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        </div>
      </Card>
    );
  }

  return (
    <Card 
      title={title} 
      className={className} 
      style={{ height, width, ...style }}
    >
      <Spin spinning={loading}>
        <div
          ref={chartRef}
          style={{
            height: title ? height - 100 : height - 50,
            width: '100%',
          }}
        />
      </Spin>
    </Card>
  );
};