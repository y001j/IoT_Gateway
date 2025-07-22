import React, { useMemo, useState } from 'react';
import { Row, Col, Card, Select, Table, Tag, Button, Space, Statistic } from 'antd';
import { RealTimeChart, ChartSeries } from './RealTimeChart';
import { useRealTimeData } from '../../hooks/useRealTimeData';
import { ClearOutlined, PauseOutlined, PlayCircleOutlined } from '@ant-design/icons';

const { Option } = Select;

export interface IoTDataChartProps {
  height?: number;
  showRawData?: boolean;
  maxChartPoints?: number;
  maxTableRows?: number;
}

export const IoTDataChart: React.FC<IoTDataChartProps> = ({
  height = 350,
  showRawData = true,
  maxChartPoints = 100,
  maxTableRows = 50,
}) => {
  const { data, isConnected } = useRealTimeData();

  const [selectedDevice, setSelectedDevice] = useState<string>('all');
  const [selectedKey, setSelectedKey] = useState<string>('all');
  const [isPaused, setIsPaused] = useState(false);

  // 提取设备和数据字段
  const { devices, dataKeys } = useMemo(() => {
    const deviceSet = new Set<string>();
    const keySet = new Set<string>();

    data.iotData.forEach(item => {
      if (item.data?.device_id) {
        deviceSet.add(String(item.data.device_id));
      }
      if (item.data?.key) {
        keySet.add(String(item.data.key));
      }
    });

    return {
      devices: Array.from(deviceSet).sort(),
      dataKeys: Array.from(keySet).sort(),
    };
  }, [data.iotData]);

  // 过滤数据
  const filteredData = useMemo(() => {
    return data.iotData.filter(item => {
      const deviceMatch = selectedDevice === 'all' || item.data?.device_id === selectedDevice;
      const keyMatch = selectedKey === 'all' || item.data?.key === selectedKey;
      return deviceMatch && keyMatch;
    });
  }, [data.iotData, selectedDevice, selectedKey]);

  // 准备图表数据
  const chartSeries: ChartSeries[] = useMemo(() => {
    const seriesMap = new Map<string, { data: any[], color: string }>();
    const colors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#13c2c2', '#eb2f96', '#fa8c16'];
    let colorIndex = 0;

    filteredData.forEach(item => {
      if (typeof item.data?.value === 'number') {
        const seriesKey = `${item.data.device_id || 'unknown'}-${item.data.key || 'value'}`;
        
        if (!seriesMap.has(seriesKey)) {
          seriesMap.set(seriesKey, {
            data: [],
            color: colors[colorIndex % colors.length],
          });
          colorIndex++;
        }

        seriesMap.get(seriesKey)!.data.push({
          timestamp: item.timestamp,
          value: item.data.value,
        });
      }
    });

    return Array.from(seriesMap.entries()).map(([key, series]) => ({
      name: key,
      data: series.data.slice(-maxChartPoints),
      color: series.color,
      type: 'line' as const,
      smooth: true,
    }));
  }, [filteredData, maxChartPoints]);

  // 统计信息
  const statistics = useMemo(() => {
    const now = new Date();
    const lastMinute = new Date(now.getTime() - 60000);
    const recentData = filteredData.filter(item => new Date(item.timestamp) > lastMinute);
    
    return {
      totalPoints: filteredData.length,
      recentPoints: recentData.length,
      devicesCount: devices.length,
      dataKeysCount: dataKeys.length,
    };
  }, [filteredData, devices.length, dataKeys.length]);

  // 表格列定义
  const tableColumns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 120,
      render: (time: Date) => time.toLocaleTimeString(),
    },
    {
      title: '设备ID',
      dataIndex: ['data', 'device_id'],
      key: 'device_id',
      width: 120,
      render: (deviceId: string) => (
        <Tag color="blue">{deviceId || 'Unknown'}</Tag>
      ),
    },
    {
      title: '数据字段',
      dataIndex: ['data', 'key'],
      key: 'key',
      width: 100,
      render: (key: string) => (
        <Tag color="green">{key || 'value'}</Tag>
      ),
    },
    {
      title: '数值',
      dataIndex: ['data', 'value'],
      key: 'value',
      width: 100,
      render: (value: any) => (
        <span style={{ fontFamily: 'monospace' }}>
          {typeof value === 'number' ? value.toFixed(2) : String(value)}
        </span>
      ),
    },
    {
      title: '主题',
      dataIndex: ['data', 'subject'],
      key: 'subject',
      ellipsis: true,
      render: (subject: string) => (
        <span style={{ fontSize: '12px', color: '#666' }}>{subject}</span>
      ),
    },
  ];

  const handleClearData = () => {
    clearData('iotData');
  };

  return (
    <div>
      {/* 控制面板 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Row gutter={[16, 16]} align="middle">
          <Col xs={24} sm={12} md={6}>
            <span style={{ marginRight: 8 }}>设备:</span>
            <Select
              value={selectedDevice}
              onChange={setSelectedDevice}
              style={{ width: '100%' }}
              size="small"
            >
              <Option value="all">全部设备</Option>
              {devices.map(device => (
                <Option key={device} value={device}>{device}</Option>
              ))}
            </Select>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <span style={{ marginRight: 8 }}>字段:</span>
            <Select
              value={selectedKey}
              onChange={setSelectedKey}
              style={{ width: '100%' }}
              size="small"
            >
              <Option value="all">全部字段</Option>
              {dataKeys.map(key => (
                <Option key={key} value={key}>{key}</Option>
              ))}
            </Select>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Space>
              <Button
                size="small"
                icon={isPaused ? <PlayCircleOutlined /> : <PauseOutlined />}
                onClick={() => setIsPaused(!isPaused)}
              >
                {isPaused ? '恢复' : '暂停'}
              </Button>
              <Button
                size="small"
                icon={<ClearOutlined />}
                onClick={handleClearData}
              >
                清除
              </Button>
            </Space>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <span style={{ fontSize: '12px', color: isConnected ? '#52c41a' : '#f5222d' }}>
              ● {isConnected ? '实时连接' : '连接断开'}
            </span>
          </Col>
        </Row>
      </Card>

      {/* 统计信息 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic
              title="总数据点"
              value={statistics.totalPoints}
              valueStyle={{ fontSize: '16px' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic
              title="最近1分钟"
              value={statistics.recentPoints}
              valueStyle={{ fontSize: '16px', color: statistics.recentPoints > 0 ? '#52c41a' : '#666' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic
              title="设备数量"
              value={statistics.devicesCount}
              valueStyle={{ fontSize: '16px' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic
              title="数据字段"
              value={statistics.dataKeysCount}
              valueStyle={{ fontSize: '16px' }}
            />
          </Card>
        </Col>
      </Row>

      {/* 实时图表 */}
      <Row gutter={[16, 16]}>
        <Col xs={24}>
          <RealTimeChart
            title="IoT 数据流实时监控"
            series={chartSeries}
            height={height}
            yAxisLabel="数值"
            maxDataPoints={maxChartPoints}
            loading={!isConnected}
            timeFormat="HH:mm:ss"
          />
        </Col>
      </Row>

      {/* 原始数据表格 */}
      {showRawData && (
        <Card 
          title="原始数据流" 
          style={{ marginTop: 16 }}
          extra={
            <span style={{ fontSize: '12px', color: '#666' }}>
              显示最近 {Math.min(filteredData.length, maxTableRows)} 条记录
            </span>
          }
        >
          <Table
            columns={tableColumns}
            dataSource={filteredData.slice(-maxTableRows).reverse()}
            pagination={false}
            size="small"
            scroll={{ y: 300 }}
            rowKey={(record, index) => `${index}-${record.timestamp}`}
          />
        </Card>
      )}
    </div>
  );
};