import React, { useState, useEffect } from 'react';
import {
  Table,
  Card,
  Button,
  Tag,
  Input,
  Select,
  Modal,
  Drawer,
  Descriptions,
  Row,
  Col,
  Typography,
  message,
  Popconfirm,
  Tooltip,
  Avatar,
  Form,
  Switch,
  Tabs,
  Space,
  Badge,
  Statistic
} from 'antd';
import type { ColumnsType, TablePaginationConfig } from 'antd/es/table';
import {
  PauseCircleOutlined,
  ReloadOutlined,
  DeleteOutlined,
  PlusOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  PlayCircleOutlined,
  EyeOutlined,
  SettingOutlined,
  WarningOutlined
} from '@ant-design/icons';
import { pluginService } from '../services/pluginService';
import PluginLogViewer from '../components/plugins/PluginLogViewer';
import type {
  Plugin,
  PluginListRequest,
  PluginAction,
  PluginStatus,
  PluginType,
  PluginStats
} from '../types/plugin';

const { Title, Text } = Typography;
const { Search } = Input;
const { Option } = Select;

const PluginsPage: React.FC = () => {
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [filterType, setFilterType] = useState<string>('');
  const [filterStatus, setFilterStatus] = useState<string>('');
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0
  });

  // 详情抽屉状态
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false);
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null);
  const [pluginStats, setPluginStats] = useState<PluginStats | null>(null);

  // 配置模态框状态
  const [configModalVisible, setConfigModalVisible] = useState(false);
  const [configForm] = Form.useForm();

  // 获取插件列表
  const fetchPlugins = async () => {
    setLoading(true);
    try {
      const params: PluginListRequest = {
        page: pagination.current,
        page_size: pagination.pageSize,
        search: searchText || undefined,
        type: filterType || undefined,
        status: filterStatus || undefined
      };

      const response = await pluginService.getPlugins(params);
      
      // 调试：输出插件数量和名称
      console.log(`📊 获取到 ${response.data?.length || 0} 个插件:`, 
        response.data?.map(p => `${p.name}(${p.type})`).join(', '));
      
      // 排序逻辑：运行中的插件排在最上面，然后按名称排序
      const sortedPlugins = [...(response.data || [])].sort((a, b) => {
        // 首先按状态排序：运行中的排在最上面
        if (a.status === 'running' && b.status !== 'running') return -1;
        if (a.status !== 'running' && b.status === 'running') return 1;
        
        // 状态相同时按名称排序
        return a.name.localeCompare(b.name);
      });
      
      setPlugins(sortedPlugins);
      setPagination(prev => ({
        ...prev,
        total: response.pagination.total
      }));
    } catch (error: any) {
      message.error('获取插件列表失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 获取插件统计
  const fetchPluginStats = async (pluginName: string) => {
    try {
      console.log('正在获取插件统计:', pluginName);
      const stats = await pluginService.getPluginStats(pluginName);
      console.log('插件统计数据:', stats);
      setPluginStats(stats);
    } catch (error: any) {
      console.error('获取插件统计失败:', error);
      message.error('获取插件统计失败：' + (error.message || '未知错误'));
      // 设置默认统计数据以避免显示错误
      setPluginStats({
        plugin_id: 0,
        data_points_total: 0,
        data_points_hour: 0,
        errors_total: 0,
        errors_hour: 0,
        uptime_seconds: 0,
        average_latency: 0,
        memory_usage: 0,
        cpu_usage: 0,
        last_update: new Date().toISOString()
      });
    }
  };

  // 执行插件操作
  const handlePluginAction = async (plugin: Plugin, action: PluginAction) => {
    try {
      const result = await pluginService.executePluginAction(plugin.name, action);
      if (result.success) {
        message.success(`${getActionText(action)}成功`);
        await fetchPlugins(); // 刷新列表
      } else {
        message.error(result.message || `${getActionText(action)}失败`);
      }
    } catch (error: any) {
      message.error(`${getActionText(action)}失败：` + (error.message || '未知错误'));
    }
  };

  // 删除插件
  const handleDeletePlugin = async (plugin: Plugin) => {
    try {
      await pluginService.deletePlugin(plugin.name);
      message.success('删除插件成功');
      await fetchPlugins();
    } catch (error: any) {
      message.error('删除插件失败：' + (error.message || '未知错误'));
    }
  };

  // 显示插件详情
  const showPluginDetails = async (plugin: Plugin) => {
    setSelectedPlugin(plugin);
    setDetailDrawerVisible(true);
    await fetchPluginStats(plugin.name);
  };

  // 显示配置模态框
  const showConfigModal = async (plugin: Plugin) => {
    try {
      const config = await pluginService.getPluginConfig(plugin.name);
      setSelectedPlugin(plugin);
      configForm.setFieldsValue({ config: JSON.stringify(config, null, 2) });
      setConfigModalVisible(true);
    } catch (error: any) {
      message.error('获取插件配置失败：' + (error.message || '未知错误'));
    }
  };

  // 保存配置
  const handleSaveConfig = async () => {
    if (!selectedPlugin) return;

    try {
      const values = await configForm.validateFields();
      const config = JSON.parse(values.config);
      
      await pluginService.updatePluginConfig(selectedPlugin.name, config);
      message.success('配置保存成功');
      setConfigModalVisible(false);
      await fetchPlugins();
    } catch (error: any) {
      if (error instanceof SyntaxError) {
        message.error('配置格式错误，请检查 JSON 格式');
      } else {
        message.error('保存配置失败：' + (error.message || '未知错误'));
      }
    }
  };

  // 工具函数
  const getActionText = (action: PluginAction): string => {
    switch (action) {
      case 'start': return '启动';
      case 'stop': return '停止';
      case 'restart': return '重启';
      default: return '操作';
    }
  };

  const getStatusTag = (status: PluginStatus) => {
    switch (status) {
      case 'running':
        return <Tag color="green" icon={<CheckCircleOutlined />}>运行中</Tag>;
      case 'stopped':
        return <Tag color="default" icon={<PauseCircleOutlined />}>已停止</Tag>;
      case 'error':
        return <Tag color="red" icon={<ExclamationCircleOutlined />}>错误</Tag>;
      default:
        return <Tag color="default">{status}</Tag>;
    }
  };

  const getTypeTag = (type: PluginType) => {
    switch (type) {
      case 'adapter':
        return <Tag color="blue">适配器</Tag>;
      case 'sink':
        return <Tag color="purple">连接器</Tag>;
      default:
        return <Tag>{type}</Tag>;
    }
  };

  // 表格列定义
  const columns: ColumnsType<Plugin> = [
    {
      title: '插件名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: Plugin) => (
        <Space>
          <Avatar shape="square" style={{ backgroundColor: record.type === 'adapter' ? '#1890ff' : '#722ed1' }}>
            {name.charAt(0).toUpperCase()}
          </Avatar>
          <div>
            <div><strong>{name}</strong></div>
            <Text type="secondary" style={{ fontSize: 12 }}>{record.description || '无描述'}</Text>
          </div>
        </Space>
      )
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: PluginType) => getTypeTag(type),
      filters: [
        { text: '适配器', value: 'adapter' },
        { text: '连接器', value: 'sink' }
      ]
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version'
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: PluginStatus, record: Plugin) => (
        <Space>
          {getStatusTag(status)}
          {record.error_count && record.error_count > 0 && (
            <Tooltip title={`错误次数: ${record.error_count}`}>
              <Badge count={record.error_count} size="small">
                <WarningOutlined style={{ color: '#ff4d4f' }} />
              </Badge>
            </Tooltip>
          )}
        </Space>
      ),
      filters: [
        { text: '运行中', value: 'running' },
        { text: '已停止', value: 'stopped' },
        { text: '错误', value: 'error' }
      ]
    },
    {
      title: '启用状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Switch checked={enabled} disabled size="small" />
      )
    },
    {
      title: '端口',
      dataIndex: 'port',
      key: 'port',
      render: (port: number) => port || '-'
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, record: Plugin) => (
        <Space>
          <Tooltip title="查看详情">
            <Button
              type="link"
              icon={<EyeOutlined />}
              onClick={() => showPluginDetails(record)}
            />
          </Tooltip>
          
          {record.status === 'running' ? (
            <Tooltip title="停止">
              <Button
                type="link"
                icon={<PauseCircleOutlined />}
                onClick={() => handlePluginAction(record, 'stop')}
              />
            </Tooltip>
          ) : (
            <Tooltip title="启动">
              <Button
                type="link"
                icon={<PlayCircleOutlined />}
                onClick={() => handlePluginAction(record, 'start')}
              />
            </Tooltip>
          )}
          
          <Tooltip title="重启">
            <Button
              type="link"
              icon={<ReloadOutlined />}
              onClick={() => handlePluginAction(record, 'restart')}
            />
          </Tooltip>
          
          <Tooltip title="配置">
            <Button
              type="link"
              icon={<SettingOutlined />}
              onClick={() => showConfigModal(record)}
            />
          </Tooltip>
          
          <Popconfirm
            title="确定要删除这个插件吗？"
            description="删除后无法恢复"
            onConfirm={() => handleDeletePlugin(record)}
            okText="确定"
            cancelText="取消"
          >
            <Tooltip title="删除">
              <Button
                type="link"
                danger
                icon={<DeleteOutlined />}
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      )
    }
  ];

  // 处理表格变化
  const handleTableChange = (paginationConfig: TablePaginationConfig, filters: any) => {
    setPagination(prev => ({
      ...prev,
      current: paginationConfig.current || 1,
      pageSize: paginationConfig.pageSize || 10
    }));
    
    // 处理筛选
    if (filters.type) {
      setFilterType(filters.type[0] || '');
    }
    if (filters.status) {
      setFilterStatus(filters.status[0] || '');
    }
  };

  // 初始化和依赖更新
  useEffect(() => {
    fetchPlugins();
  }, [pagination.current, pagination.pageSize, searchText, filterType, filterStatus]);

  return (
    <div>
      <Title level={2}>插件管理</Title>
      
      {/* 操作栏 */}
      <Card style={{ marginBottom: 16 }}>
        <Row gutter={16} align="middle">
          <Col flex="auto">
            <Space>
              <Search
                placeholder="搜索插件名称或描述"
                allowClear
                style={{ width: 300 }}
                onSearch={setSearchText}
                onClear={() => setSearchText('')}
              />
              <Select
                placeholder="插件类型"
                allowClear
                style={{ width: 120 }}
                value={filterType}
                onChange={setFilterType}
              >
                <Option value="adapter">适配器</Option>
                <Option value="sink">连接器</Option>
              </Select>
              <Select
                placeholder="状态"
                allowClear
                style={{ width: 120 }}
                value={filterStatus}
                onChange={setFilterStatus}
              >
                <Option value="running">运行中</Option>
                <Option value="stopped">已停止</Option>
                <Option value="error">错误</Option>
              </Select>
            </Space>
          </Col>
          <Col>
            <Space>
              <Button icon={<ReloadOutlined />} onClick={fetchPlugins}>
                刷新
              </Button>
              <Button type="primary" icon={<PlusOutlined />}>
                添加插件
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 插件列表 */}
      <Card>
        <Table
          columns={columns}
          dataSource={plugins}
          rowKey="name"
          loading={loading}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`
          }}
          onChange={handleTableChange}
        />
      </Card>

      {/* 插件详情抽屉 */}
      <Drawer
        title="插件详情"
        width={600}
        open={detailDrawerVisible}
        onClose={() => setDetailDrawerVisible(false)}
      >
        {selectedPlugin && (
          <div>
            <Tabs 
              defaultActiveKey="1"
              items={[
                {
                  key: "1",
                  label: "基本信息",
                  children: (
                    <Descriptions title="基本信息" bordered>
                      <Descriptions.Item label="名称">{selectedPlugin.name}</Descriptions.Item>
                      <Descriptions.Item label="类型">{getTypeTag(selectedPlugin.type as PluginType)}</Descriptions.Item>
                      <Descriptions.Item label="版本">{selectedPlugin.version}</Descriptions.Item>
                      <Descriptions.Item label="状态">{getStatusTag(selectedPlugin.status as PluginStatus)}</Descriptions.Item>
                      <Descriptions.Item label="端口">{selectedPlugin.port || '无'}</Descriptions.Item>
                      <Descriptions.Item label="作者">{selectedPlugin.author || '未知'}</Descriptions.Item>
                      <Descriptions.Item label="路径" span={3}>{selectedPlugin.path}</Descriptions.Item>
                      <Descriptions.Item label="描述" span={3}>{selectedPlugin.description || '无描述'}</Descriptions.Item>
                    </Descriptions>
                  )
                },
                {
                  key: "2",
                  label: "运行统计",
                  children: pluginStats ? (
                    <div style={{ marginTop: 24 }}>
                      <Title level={4}>运行统计</Title>
                      <Row gutter={[16, 16]}>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="总数据点" 
                              value={pluginStats.data_points_total} 
                              valueStyle={{ color: '#1890ff' }}
                              formatter={(value) => Number(value).toLocaleString()}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="小时数据点" 
                              value={pluginStats.data_points_hour}
                              valueStyle={{ color: '#52c41a' }}
                              formatter={(value) => Number(value).toLocaleString()}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="运行时间" 
                              value={Math.floor(pluginStats.uptime_seconds / 3600)} 
                              suffix="小时"
                              valueStyle={{ color: '#722ed1' }}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="总错误" 
                              value={pluginStats.errors_total} 
                              valueStyle={{ 
                                color: pluginStats.errors_total > 0 ? '#f5222d' : '#52c41a' 
                              }}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="小时错误" 
                              value={pluginStats.errors_hour}
                              valueStyle={{ 
                                color: pluginStats.errors_hour > 0 ? '#f5222d' : '#52c41a' 
                              }}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="平均延迟" 
                              value={pluginStats.average_latency} 
                              precision={2} 
                              suffix="ms"
                              valueStyle={{ 
                                color: pluginStats.average_latency > 100 ? '#f5222d' : 
                                       pluginStats.average_latency > 50 ? '#faad14' : '#52c41a' 
                              }}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="内存使用" 
                              value={(pluginStats.memory_usage / 1024 / 1024)} 
                              precision={1}
                              suffix="MB"
                              valueStyle={{ color: '#13c2c2' }}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="CPU使用率" 
                              value={pluginStats.cpu_usage} 
                              precision={1} 
                              suffix="%"
                              valueStyle={{ 
                                color: pluginStats.cpu_usage > 80 ? '#f5222d' : 
                                       pluginStats.cpu_usage > 50 ? '#faad14' : '#52c41a' 
                              }}
                            />
                          </Card>
                        </Col>
                        <Col xs={24} sm={12} lg={8}>
                          <Card size="small">
                            <Statistic 
                              title="最后更新" 
                              value={new Date(pluginStats.last_update).toLocaleString()}
                              valueStyle={{ color: '#666', fontSize: '14px' }}
                            />
                          </Card>
                        </Col>
                      </Row>
                    </div>
                  ) : (
                    <div style={{ 
                      padding: '24px', 
                      textAlign: 'center',
                      color: '#999'
                    }}>
                      <Text type="secondary">暂无统计数据</Text>
                    </div>
                  )
                },
                {
                  key: "3",
                  label: "插件日志",
                  children: <PluginLogViewer pluginName={selectedPlugin.name} />
                }
              ]}
            />
          </div>
        )}
      </Drawer>

      {/* 配置编辑模态框 */}
      <Modal
        title="插件配置"
        open={configModalVisible}
        onOk={handleSaveConfig}
        onCancel={() => setConfigModalVisible(false)}
        width={800}
        okText="保存"
        cancelText="取消"
      >
        {selectedPlugin && (
          <Form form={configForm} layout="vertical">
            <Form.Item
              label="配置 (JSON 格式)"
              name="config"
              rules={[
                { required: true, message: '请输入配置' },
                {
                  validator: (_, value) => {
                    try {
                      JSON.parse(value);
                      return Promise.resolve();
                    } catch {
                      return Promise.reject(new Error('JSON 格式错误'));
                    }
                  }
                }
              ]}
            >
              <Input.TextArea
                rows={20}
                placeholder='{"key": "value"}'
                style={{ fontFamily: 'monospace' }}
              />
            </Form.Item>
          </Form>
        )}
      </Modal>
    </div>
  );
};

export default PluginsPage; 