import React, { useState, useEffect } from 'react';
import {
  Table,
  Card,
  Select,
  DatePicker,
  Tag,
  Typography,
  Row,
  Col,
  Button,
  message
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';
import { pluginService } from '../../services/pluginService';
import type { PluginLog, PluginLogRequest } from '../../types/plugin';
import dayjs from 'dayjs';

const { Option } = Select;
const { RangePicker } = DatePicker;
const { Text } = Typography;

interface PluginLogViewerProps {
  pluginName: string;
}

const PluginLogViewer: React.FC<PluginLogViewerProps> = ({ pluginName }) => {
  const [logs, setLogs] = useState<PluginLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 20,
    total: 0
  });
  const [filters, setFilters] = useState({
    level: '',
    source: '',
    dateRange: null as [dayjs.Dayjs, dayjs.Dayjs] | null
  });

  // 获取日志列表
  const fetchLogs = async () => {
    setLoading(true);
    try {
      const params: PluginLogRequest = {
        page: pagination.current,
        page_size: pagination.pageSize,
        level: filters.level || undefined,
        source: filters.source || undefined,
        from: filters.dateRange?.[0]?.toISOString(),
        to: filters.dateRange?.[1]?.toISOString()
      };

      const response = await pluginService.getPluginLogs(pluginName, params);
      setLogs(response.data);
      setPagination(prev => ({
        ...prev,
        total: response.total
      }));
    } catch (error: any) {
      message.error('获取日志失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 获取日志级别标签
  const getLevelTag = (level: string) => {
    switch (level.toLowerCase()) {
      case 'error':
        return <Tag color="red">{level.toUpperCase()}</Tag>;
      case 'warn':
      case 'warning':
        return <Tag color="orange">{level.toUpperCase()}</Tag>;
      case 'info':
        return <Tag color="blue">{level.toUpperCase()}</Tag>;
      case 'debug':
        return <Tag color="default">{level.toUpperCase()}</Tag>;
      default:
        return <Tag>{level.toUpperCase()}</Tag>;
    }
  };

  // 表格列定义
  const columns: ColumnsType<PluginLog> = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 180,
      render: (timestamp: string) => (
        <Text style={{ fontSize: 12, fontFamily: 'monospace' }}>
          {dayjs(timestamp).format('YYYY-MM-DD HH:mm:ss')}
        </Text>
      )
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      width: 80,
      render: (level: string) => getLevelTag(level)
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      width: 120,
      render: (source: string) => (
        <Tag color="geekblue">{source}</Tag>
      )
    },
    {
      title: '消息',
      dataIndex: 'message',
      key: 'message',
      render: (message: string) => (
        <Text
          style={{
            fontFamily: 'monospace',
            fontSize: 12,
            wordBreak: 'break-all'
          }}
        >
          {message}
        </Text>
      )
    }
  ];

  // 处理分页变化
  const handleTableChange = (paginationConfig: any) => {
    setPagination(prev => ({
      ...prev,
      current: paginationConfig.current || 1,
      pageSize: paginationConfig.pageSize || 20
    }));
  };

  // 处理筛选变化
  const handleFilterChange = (key: string, value: any) => {
    setFilters(prev => ({
      ...prev,
      [key]: value
    }));
    setPagination(prev => ({
      ...prev,
      current: 1 // 重置到第一页
    }));
  };

  // 初始化和依赖更新
  useEffect(() => {
    fetchLogs();
  }, [pagination.current, pagination.pageSize, filters]);

  return (
    <Card title={`插件日志 - ${pluginName}`}>
      {/* 筛选栏 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={4}>
          <Select
            placeholder="日志级别"
            allowClear
            style={{ width: '100%' }}
            value={filters.level}
            onChange={(value) => handleFilterChange('level', value)}
          >
            <Option value="error">ERROR</Option>
            <Option value="warn">WARN</Option>
            <Option value="info">INFO</Option>
            <Option value="debug">DEBUG</Option>
          </Select>
        </Col>
        <Col span={4}>
          <Select
            placeholder="日志来源"
            allowClear
            style={{ width: '100%' }}
            value={filters.source}
            onChange={(value) => handleFilterChange('source', value)}
          >
            <Option value="main">主程序</Option>
            <Option value="plugin">插件</Option>
            <Option value="system">系统</Option>
          </Select>
        </Col>
        <Col span={8}>
          <RangePicker
            style={{ width: '100%' }}
            value={filters.dateRange}
            onChange={(dates) => handleFilterChange('dateRange', dates)}
            showTime
            format="YYYY-MM-DD HH:mm:ss"
          />
        </Col>
        <Col span={4}>
          <Button
            icon={<ReloadOutlined />}
            onClick={fetchLogs}
            style={{ width: '100%' }}
          >
            刷新
          </Button>
        </Col>
      </Row>

      {/* 日志表格 */}
      <Table
        columns={columns}
        dataSource={logs}
        rowKey="id"
        loading={loading}
        size="small"
        pagination={{
          current: pagination.current,
          pageSize: pagination.pageSize,
          total: pagination.total,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`,
          pageSizeOptions: ['20', '50', '100']
        }}
        onChange={handleTableChange}
        scroll={{ y: 400 }}
      />
    </Card>
  );
};

export default PluginLogViewer; 