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
  Modal,
  Popconfirm,
  Tag,
  Tooltip
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  ApiOutlined,
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  LockOutlined,
  UnlockOutlined,
  ThunderboltOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { ApiPermissionRule, ApiEndpoint } from '../../types/settings';

const { Option } = Select;
const { Text } = Typography;

const ApiPermissions: React.FC = () => {
  const [ruleForm] = Form.useForm();
  const [globalForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [globalConfig, setGlobalConfig] = useState<any>(null);
  const [rules, setRules] = useState<ApiPermissionRule[]>([]);
  const [, setEndpoints] = useState<ApiEndpoint[]>([]);
  const [ruleModalVisible, setRuleModalVisible] = useState(false);
  const [editingRule, setEditingRule] = useState<ApiPermissionRule | null>(null);

  // 加载配置和规则数据
  const loadData = async () => {
    setLoading(true);
    try {
      const [configResponse, rulesResponse, endpointsResponse] = await Promise.all([
        settingsService.getApiPermissionsConfig(),
        settingsService.getApiPermissionRules(),
        settingsService.getApiEndpoints()
      ]);
      
      if (configResponse.success) {
        setGlobalConfig(configResponse.data);
        globalForm.setFieldsValue(configResponse.data);
      }
      
      if (rulesResponse.success) {
        setRules(rulesResponse.data);
      }
      
      if (endpointsResponse.success) {
        setEndpoints(endpointsResponse.data);
      }
    } catch (error: any) {
      message.error('加载数据失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 保存全局配置
  const handleSaveGlobalConfig = async () => {
    try {
      const values = await globalForm.validateFields();
      setLoading(true);
      
      const response = await settingsService.updateApiPermissionsConfig(values);
      if (response.success) {
        message.success('API权限配置保存成功');
        setGlobalConfig(values);
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

  // 创建/编辑规则
  const handleSaveRule = async () => {
    try {
      const values = await ruleForm.validateFields();
      
      if (editingRule) {
        // 更新规则
        const response = await settingsService.updateApiPermissionRule(editingRule.id, values);
        if (response.success) {
          message.success('规则更新成功');
          setRules(rules.map(rule => rule.id === editingRule.id ? { ...rule, ...values } : rule));
        } else {
          message.error('更新失败：' + (response.message || '未知错误'));
        }
      } else {
        // 创建新规则
        const response = await settingsService.createApiPermissionRule(values);
        if (response.success) {
          message.success('规则创建成功');
          setRules([...rules, response.data]);
        } else {
          message.error('创建失败：' + (response.message || '未知错误'));
        }
      }
      
      setRuleModalVisible(false);
      setEditingRule(null);
      ruleForm.resetFields();
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 删除规则
  const handleDeleteRule = async (id: string) => {
    try {
      const response = await settingsService.deleteApiPermissionRule(id);
      if (response.success) {
        message.success('规则删除成功');
        setRules(rules.filter(rule => rule.id !== id));
      } else {
        message.error('删除失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('删除失败：' + (error.message || '未知错误'));
    }
  };

  // 启用/禁用规则
  const handleToggleRule = async (rule: ApiPermissionRule) => {
    try {
      const response = await settingsService.updateApiPermissionRule(rule.id, { enabled: !rule.enabled });
      if (response.success) {
        message.success(`规则${rule.enabled ? '禁用' : '启用'}成功`);
        setRules(rules.map(r => r.id === rule.id ? { ...r, enabled: !r.enabled } : r));
      } else {
        message.error('操作失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 打开编辑模态框
  const handleEditRule = (rule: ApiPermissionRule) => {
    setEditingRule(rule);
    ruleForm.setFieldsValue(rule);
    setRuleModalVisible(true);
  };

  // 重置全局配置
  const handleResetGlobalConfig = () => {
    if (globalConfig) {
      globalForm.setFieldsValue(globalConfig);
      message.info('已重置为已保存的配置');
    }
  };

  // 获取HTTP方法颜色
  const getMethodColor = (method: string) => {
    switch (method) {
      case 'GET':
        return 'green';
      case 'POST':
        return 'blue';
      case 'PUT':
        return 'orange';
      case 'DELETE':
        return 'red';
      default:
        return 'default';
    }
  };

  // 规则表格列
  const ruleColumns = [
    {
      title: '规则名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <Space>
          <ApiOutlined />
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: '路径',
      dataIndex: 'path',
      key: 'path',
      render: (path: string) => <Text code>{path}</Text>,
    },
    {
      title: 'HTTP方法',
      dataIndex: 'method',
      key: 'method',
      render: (method: string) => (
        <Tag color={getMethodColor(method)}>{method}</Tag>
      ),
    },
    {
      title: '访问控制',
      dataIndex: 'access_type',
      key: 'access_type',
      render: (type: string) => {
        const typeMap = {
          allow: { color: 'green', text: '允许' },
          deny: { color: 'red', text: '拒绝' },
          rate_limit: { color: 'orange', text: '限流' }
        };
        const config = typeMap[type as keyof typeof typeMap] || { color: 'default', text: type };
        return <Tag color={config.color}>{config.text}</Tag>;
      },
    },
    {
      title: '限制参数',
      dataIndex: 'rate_limit',
      key: 'rate_limit',
      render: (rateLimit: any, record: ApiPermissionRule) => {
        if (record.access_type === 'rate_limit' && rateLimit) {
          return `${rateLimit.requests}/${rateLimit.window}`;
        }
        return '-';
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'green' : 'red'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: ApiPermissionRule) => (
        <Space>
          <Tooltip title="编辑规则">
            <Button 
              type="text" 
              icon={<EditOutlined />}
              onClick={() => handleEditRule(record)}
            />
          </Tooltip>
          
          <Tooltip title={record.enabled ? '禁用规则' : '启用规则'}>
            <Button 
              type="text" 
              icon={record.enabled ? <LockOutlined /> : <UnlockOutlined />}
              onClick={() => handleToggleRule(record)}
            />
          </Tooltip>
          
          <Popconfirm
            title="确定要删除这个规则吗？"
            onConfirm={() => handleDeleteRule(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Tooltip title="删除规则">
              <Button type="text" icon={<DeleteOutlined />} danger />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  useEffect(() => {
    loadData();
  }, []);

  return (
    <div>
      <Alert
        message="API权限配置说明"
        description="配置REST API的访问控制和速率限制。可以设置全局默认策略，也可以为特定API端点创建自定义规则。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      {/* 全局配置 */}
      <Card 
        title={
          <Space>
            <SettingOutlined />
            全局API配置
          </Space>
        }
        size="small" 
        style={{ marginBottom: 16 }}
      >
        <Form
          form={globalForm}
          layout="vertical"
          onValuesChange={() => {}}
        >
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="启用API访问控制"
                name="enabled"
                valuePropName="checked"
                tooltip="启用REST API访问控制功能"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="默认访问策略"
                name="default_policy"
                rules={[{ required: true, message: '请选择默认策略' }]}
                tooltip="未匹配任何规则时的默认处理方式"
              >
                <Select placeholder="选择默认策略">
                  <Option value="allow">允许访问</Option>
                  <Option value="deny">拒绝访问</Option>
                  <Option value="rate_limit">应用速率限制</Option>
                </Select>
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="全局速率限制"
                name="global_rate_limit"
                rules={[{ required: true, message: '请输入速率限制' }]}
                tooltip="全局API请求速率限制"
              >
                <Input placeholder="1000/hour" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="IP白名单"
                name="ip_whitelist"
                tooltip="允许访问的IP地址列表，每行一个"
              >
                <Input.TextArea 
                  rows={3}
                  placeholder="192.168.1.0/24
10.0.0.0/8"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="IP黑名单"
                name="ip_blacklist"
                tooltip="禁止访问的IP地址列表，每行一个"
              >
                <Input.TextArea 
                  rows={3}
                  placeholder="192.168.1.100
10.0.0.50"
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="CORS配置"
                name="cors_origins"
                tooltip="允许跨域访问的域名列表，每行一个"
              >
                <Input.TextArea 
                  rows={3}
                  placeholder="https://example.com
https://app.example.com"
                />
              </Form.Item>
            </Col>
          </Row>

          <Space>
            <Button 
              type="primary" 
              icon={<SaveOutlined />}
              loading={loading}
              onClick={handleSaveGlobalConfig}
            >
              保存配置
            </Button>
            
            <Button 
              icon={<ReloadOutlined />}
              onClick={handleResetGlobalConfig}
            >
              重置
            </Button>
          </Space>
        </Form>
      </Card>

      {/* 权限规则管理 */}
      <Card 
        title={
          <Space>
            <ThunderboltOutlined />
            API权限规则
            <Button 
              type="primary" 
              size="small"
              icon={<PlusOutlined />}
              onClick={() => {
                setEditingRule(null);
                ruleForm.resetFields();
                setRuleModalVisible(true);
              }}
            >
              添加规则
            </Button>
          </Space>
        }
        extra={
          <Button 
            icon={<ReloadOutlined />}
            onClick={loadData}
            loading={loading}
          >
            刷新
          </Button>
        }
      >
        <Table
          dataSource={rules}
          columns={ruleColumns}
          rowKey="id"
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`
          }}
          loading={loading}
        />
      </Card>

      {/* 创建/编辑规则模态框 */}
      <Modal
        title={editingRule ? '编辑API权限规则' : '创建API权限规则'}
        open={ruleModalVisible}
        onOk={handleSaveRule}
        onCancel={() => {
          setRuleModalVisible(false);
          setEditingRule(null);
          ruleForm.resetFields();
        }}
        okText="保存"
        cancelText="取消"
        width={700}
      >
        <Form form={ruleForm} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                label="规则名称"
                name="name"
                rules={[{ required: true, message: '请输入规则名称' }]}
              >
                <Input placeholder="输入规则名称" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="启用规则"
                name="enabled"
                valuePropName="checked"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                label="API路径"
                name="path"
                rules={[{ required: true, message: '请输入API路径' }]}
                tooltip="支持通配符，如 /api/v1/devices/*"
              >
                <Input placeholder="/api/v1/devices" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="HTTP方法"
                name="method"
                rules={[{ required: true, message: '请选择HTTP方法' }]}
              >
                <Select placeholder="选择HTTP方法">
                  <Option value="GET">GET</Option>
                  <Option value="POST">POST</Option>
                  <Option value="PUT">PUT</Option>
                  <Option value="DELETE">DELETE</Option>
                  <Option value="*">全部</Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>

          <Form.Item
            label="访问控制类型"
            name="access_type"
            rules={[{ required: true, message: '请选择访问控制类型' }]}
          >
            <Select placeholder="选择访问控制类型">
              <Option value="allow">允许访问</Option>
              <Option value="deny">拒绝访问</Option>
              <Option value="rate_limit">速率限制</Option>
            </Select>
          </Form.Item>

          <Form.Item
            noStyle
            shouldUpdate={(prevValues, currentValues) => prevValues.access_type !== currentValues.access_type}
          >
            {({ getFieldValue }) => {
              const accessType = getFieldValue('access_type');
              
              if (accessType === 'rate_limit') {
                return (
                  <Row gutter={16}>
                    <Col span={12}>
                      <Form.Item
                        label="请求数量"
                        name={['rate_limit', 'requests']}
                        rules={[{ required: true, message: '请输入请求数量' }]}
                      >
                        <InputNumber 
                          placeholder="100" 
                          style={{ width: '100%' }}
                          min={1}
                        />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        label="时间窗口"
                        name={['rate_limit', 'window']}
                        rules={[{ required: true, message: '请输入时间窗口' }]}
                      >
                        <Select placeholder="选择时间窗口">
                          <Option value="minute">每分钟</Option>
                          <Option value="hour">每小时</Option>
                          <Option value="day">每天</Option>
                        </Select>
                      </Form.Item>
                    </Col>
                  </Row>
                );
              }
              return null;
            }}
          </Form.Item>

          <Form.Item
            label="应用到角色"
            name="roles"
            tooltip="选择此规则适用的用户角色，留空表示适用于所有用户"
          >
            <Select
              mode="multiple"
              placeholder="选择角色（可选）"
              allowClear
            >
              <Option value="admin">管理员</Option>
              <Option value="operator">操作员</Option>
              <Option value="viewer">查看者</Option>
            </Select>
          </Form.Item>

          <Form.Item
            label="描述"
            name="description"
          >
            <Input.TextArea rows={3} placeholder="输入规则描述" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default ApiPermissions;