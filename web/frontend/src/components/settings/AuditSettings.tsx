import React, { useState, useEffect } from 'react';
import {
  Form,
  Input,
  InputNumber,
  Button,
  Card,
  Row,
  Col,
  message,
  Alert,
  Space,
  Switch,
  Select,
  Typography,
  Table,
  DatePicker,
  Tag,
  Tooltip
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  AuditOutlined,
  EyeOutlined,
  SearchOutlined,
  DownloadOutlined,
  DeleteOutlined,
  FilterOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { AuditConfig, AuditLog } from '../../types/settings';

const { Option } = Select;
const { Title } = Typography;
const { RangePicker } = DatePicker;

const AuditSettings: React.FC = () => {
  const [configForm] = Form.useForm();
  const [searchForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<AuditConfig | null>(null);
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);
  const [totalLogs, setTotalLogs] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const response = await settingsService.getAuditConfig();
      if (response.success) {
        setConfig(response.data);
        configForm.setFieldsValue(response.data);
      } else {
        message.error('加载审计配置失败');
      }
    } catch (error: any) {
      message.error('加载配置失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 加载审计日志
  const loadAuditLogs = async (page = 1, size = 10, filters = {}) => {
    setLoading(true);
    try {
      const response = await settingsService.getAuditLogs({ page, size, ...filters });
      if (response.success) {
        setAuditLogs(response.data.data);
        setTotalLogs(response.data.total);
        setCurrentPage(page);
        setPageSize(size);
      } else {
        message.error('加载审计日志失败');
      }
    } catch (error: any) {
      message.error('加载日志失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 保存配置
  const handleSaveConfig = async () => {
    try {
      const values = await configForm.validateFields();
      setLoading(true);
      
      const response = await settingsService.updateAuditConfig(values);
      if (response.success) {
        message.success('审计配置保存成功');
        setConfig(values);
      } else {
        message.error('保存配置失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      if (error.errorFields) {
        message.error('请检查表单输入');
      } else {
        message.error('保存配置失败：' + (error.message || '未知错误'));
      }
    } finally {
      setLoading(false);
    }
  };

  // 重置配置
  const handleResetConfig = () => {
    if (config) {
      configForm.setFieldsValue(config);
      message.info('已重置为已保存的配置');
    }
  };

  // 搜索审计日志
  const handleSearch = async () => {
    try {
      const values = await searchForm.validateFields();
      const filters: any = {};
      
      if (values.user) filters.user = values.user;
      if (values.action) filters.action = values.action;
      if (values.resource) filters.resource = values.resource;
      if (values.date_range) {
        filters.start_time = values.date_range[0].format('YYYY-MM-DD HH:mm:ss');
        filters.end_time = values.date_range[1].format('YYYY-MM-DD HH:mm:ss');
      }
      
      await loadAuditLogs(1, pageSize, filters);
    } catch (error: any) {
      message.error('搜索失败：' + (error.message || '未知错误'));
    }
  };

  // 清空搜索
  const handleClearSearch = () => {
    searchForm.resetFields();
    loadAuditLogs(1, pageSize);
  };

  // 导出审计日志
  const handleExportLogs = async () => {
    try {
      const response = await settingsService.exportAuditLogs();
      if (response.success) {
        message.success('审计日志导出成功');
        // 处理文件下载
        const blob = new Blob([response.data], { type: 'application/json' });
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `audit-logs-${new Date().toISOString().split('T')[0]}.json`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        window.URL.revokeObjectURL(url);
      } else {
        message.error('导出失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('导出失败：' + (error.message || '未知错误'));
    }
  };

  // 清理旧日志
  const handleCleanupLogs = async () => {
    try {
      const response = await settingsService.cleanupAuditLogs();
      if (response.success) {
        message.success('旧日志清理成功');
        loadAuditLogs(currentPage, pageSize);
      } else {
        message.error('清理失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('清理失败：' + (error.message || '未知错误'));
    }
  };

  // 获取操作类型颜色
  const getActionColor = (action: string) => {
    switch (action) {
      case 'CREATE':
        return 'green';
      case 'UPDATE':
        return 'blue';
      case 'DELETE':
        return 'red';
      case 'READ':
        return 'default';
      case 'LOGIN':
        return 'purple';
      case 'LOGOUT':
        return 'orange';
      default:
        return 'default';
    }
  };

  // 审计日志表格列
  const logColumns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 180,
      render: (timestamp: string) => new Date(timestamp).toLocaleString(),
      sorter: (a: AuditLog, b: AuditLog) => 
        new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 120,
      render: (user: string) => <Text strong>{user}</Text>,
    },
    {
      title: '操作',
      dataIndex: 'action',
      key: 'action',
      width: 100,
      render: (action: string) => (
        <Tag color={getActionColor(action)}>{action}</Tag>
      ),
    },
    {
      title: '资源',
      dataIndex: 'resource',
      key: 'resource',
      width: 150,
      render: (resource: string) => <Text code>{resource}</Text>,
    },
    {
      title: 'IP地址',
      dataIndex: 'ip_address',
      key: 'ip_address',
      width: 120,
    },
    {
      title: '用户代理',
      dataIndex: 'user_agent',
      key: 'user_agent',
      width: 200,
      render: (userAgent: string) => (
        <Tooltip title={userAgent}>
          <Text ellipsis style={{ maxWidth: 180 }}>{userAgent}</Text>
        </Tooltip>
      ),
    },
    {
      title: '结果',
      dataIndex: 'result',
      key: 'result',
      width: 80,
      render: (result: string) => (
        <Tag color={result === 'success' ? 'green' : 'red'}>
          {result === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: '详情',
      dataIndex: 'details',
      key: 'details',
      width: 200,
      render: (details: any) => (
        <Tooltip title={JSON.stringify(details, null, 2)}>
          <Button 
            type="text" 
            icon={<EyeOutlined />}
            size="small"
          >
            查看
          </Button>
        </Tooltip>
      ),
    },
  ];

  useEffect(() => {
    loadConfig();
    loadAuditLogs();
  }, []);

  return (
    <div>
      <Alert
        message="审计日志说明"
        description="记录系统中所有用户操作和访问日志，用于安全审计和问题追踪。审计日志包含用户身份、操作时间、资源访问和操作结果等信息。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      {/* 审计配置 */}
      <Card 
        title={
          <Space>
            <AuditOutlined />
            审计配置
          </Space>
        }
        size="small" 
        style={{ marginBottom: 16 }}
      >
        <Form
          form={configForm}
          layout="vertical"
          onValuesChange={() => {}}
        >
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="启用审计日志"
                name="enabled"
                valuePropName="checked"
                tooltip="启用系统审计日志记录功能"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="日志保留时间"
                name="retention_days"
                rules={[
                  { required: true, message: '请输入日志保留时间' },
                  { type: 'number', min: 1, message: '保留时间必须大于0' }
                ]}
                tooltip="审计日志的保留天数"
              >
                <InputNumber 
                  placeholder="90" 
                  style={{ width: '100%' }}
                  min={1}
                  max={3650}
                  addonAfter="天"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="详细程度"
                name="detail_level"
                rules={[{ required: true, message: '请选择详细程度' }]}
                tooltip="审计日志的详细程度"
              >
                <Select placeholder="选择详细程度">
                  <Option value="minimal">最少</Option>
                  <Option value="standard">标准</Option>
                  <Option value="detailed">详细</Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="记录成功操作"
                name="log_success"
                valuePropName="checked"
                tooltip="是否记录成功的操作"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="记录失败操作"
                name="log_failure"
                valuePropName="checked"
                tooltip="是否记录失败的操作"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="自动清理"
                name="auto_cleanup"
                valuePropName="checked"
                tooltip="是否自动清理过期的审计日志"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>
          </Row>

          <Space>
            <Button 
              type="primary" 
              icon={<SaveOutlined />}
              loading={loading}
              onClick={handleSaveConfig}
            >
              保存配置
            </Button>
            
            <Button 
              icon={<ReloadOutlined />}
              onClick={handleResetConfig}
            >
              重置
            </Button>
          </Space>
        </Form>
      </Card>

      {/* 审计日志查询 */}
      <Card 
        title={
          <Space>
            <SearchOutlined />
            审计日志查询
          </Space>
        }
        size="small" 
        style={{ marginBottom: 16 }}
      >
        <Form
          form={searchForm}
          layout="inline"
          onFinish={handleSearch}
        >
          <Form.Item name="user" label="用户">
            <Input placeholder="用户名" style={{ width: 120 }} />
          </Form.Item>

          <Form.Item name="action" label="操作">
            <Select placeholder="选择操作" style={{ width: 120 }}>
              <Option value="CREATE">创建</Option>
              <Option value="UPDATE">更新</Option>
              <Option value="DELETE">删除</Option>
              <Option value="READ">读取</Option>
              <Option value="LOGIN">登录</Option>
              <Option value="LOGOUT">登出</Option>
            </Select>
          </Form.Item>

          <Form.Item name="resource" label="资源">
            <Input placeholder="资源路径" style={{ width: 150 }} />
          </Form.Item>

          <Form.Item name="date_range" label="时间范围">
            <RangePicker 
              showTime 
              format="YYYY-MM-DD HH:mm:ss"
              style={{ width: 300 }}
            />
          </Form.Item>

          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>
                搜索
              </Button>
              <Button onClick={handleClearSearch} icon={<FilterOutlined />}>
                清空
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      {/* 审计日志列表 */}
      <Card 
        title={
          <Space>
            <AuditOutlined />
            审计日志
          </Space>
        }
        extra={
          <Space>
            <Button 
              icon={<DownloadOutlined />}
              onClick={handleExportLogs}
            >
              导出
            </Button>
            <Button 
              icon={<DeleteOutlined />}
              onClick={handleCleanupLogs}
              danger
            >
              清理旧日志
            </Button>
            <Button 
              icon={<ReloadOutlined />}
              onClick={() => loadAuditLogs(currentPage, pageSize)}
              loading={loading}
            >
              刷新
            </Button>
          </Space>
        }
      >
        <Table
          dataSource={auditLogs}
          columns={logColumns}
          rowKey="id"
          pagination={{
            current: currentPage,
            pageSize: pageSize,
            total: totalLogs,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`,
            onChange: (page, size) => loadAuditLogs(page, size),
            onShowSizeChange: (current, size) => loadAuditLogs(current, size),
          }}
          loading={loading}
          scroll={{ x: 1200 }}
          size="small"
        />
      </Card>
    </div>
  );
};

export default AuditSettings;