import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Tag,
  Space,
  Modal,
  Form,
  Input,
  Select,
  Switch,
  message,
  Popconfirm,
  Tooltip,
  Row,
  Col,
  Typography,
  Drawer,
  Tabs,
  Descriptions,
  InputNumber,
  Segmented,
  Alert
} from 'antd';
import type { ColumnsType, TablePaginationConfig } from 'antd/es/table';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  EyeOutlined,
  SettingOutlined,
  CopyOutlined,
  CheckCircleOutlined,
  QuestionCircleOutlined,
  BookOutlined,
  CodeOutlined,
  FormOutlined
} from '@ant-design/icons';
import { ruleService } from '../services/ruleService';
import type {
  Rule,
  RuleListRequest,
  Action,
  Condition
} from '../types/rule';
import RuleHelp from '../components/RuleHelp';
import ConditionForm from '../components/ConditionForm';
import ActionForm from '../components/ActionForm';
import RuleTemplates from '../components/RuleTemplates';

const { Title, Text } = Typography;
const { Search } = Input;
const { Option } = Select;
const { TextArea } = Input;

const RulesPage: React.FC = () => {
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [filterEnabled, setFilterEnabled] = useState<boolean | undefined>();
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0
  });

  // 详情抽屉状态
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false);
  const [selectedRule, setSelectedRule] = useState<Rule | null>(null);

  // 编辑模态框状态
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editForm] = Form.useForm();

  // 帮助抽屉状态
  const [helpVisible, setHelpVisible] = useState(false);

  // 模板选择状态
  const [templatesVisible, setTemplatesVisible] = useState(false);

  // 表单模式状态
  const [formMode, setFormMode] = useState<'visual' | 'json'>('visual');

  // 结构化表单状态
  const [currentCondition, setCurrentCondition] = useState<Condition | undefined>();
  const [currentActions, setCurrentActions] = useState<Action[]>([]);

  // JSON预览状态
  const [jsonPreview, setJsonPreview] = useState<string>('');
  const [validationErrors, setValidationErrors] = useState<string[]>([]);

  // 获取规则列表
  const fetchRules = async () => {
    setLoading(true);
    try {
      const params: RuleListRequest = {
        page: pagination.current,
        page_size: pagination.pageSize,
        search: searchText || undefined,
        enabled: filterEnabled
      };

      const response = await ruleService.getRules(params);
      setRules(response.data);
      setPagination(prev => ({
        ...prev,
        total: response.pagination.total
      }));
    } catch (error: any) {
      message.error('获取规则列表失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 切换规则状态
  const toggleRuleStatus = async (rule: Rule) => {
    try {
      if (rule.enabled) {
        await ruleService.disableRule(rule.id);
        message.success('规则已禁用');
      } else {
        await ruleService.enableRule(rule.id);
        message.success('规则已启用');
      }
      await fetchRules();
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 删除规则
  const handleDeleteRule = async (rule: Rule) => {
    try {
      await ruleService.deleteRule(rule.id);
      message.success('删除规则成功');
      await fetchRules();
    } catch (error: any) {
      message.error('删除规则失败：' + (error.message || '未知错误'));
    }
  };

  // 显示规则详情
  const showRuleDetails = (rule: Rule) => {
    setSelectedRule(rule);
    setDetailDrawerVisible(true);
  };

  // 显示编辑模态框
  const showEditModal = (rule?: Rule) => {
    setIsEditing(!!rule);
    if (rule) {
      setSelectedRule(rule);
      // 设置表单基本字段
      editForm.setFieldsValue({
        name: rule.name,
        description: rule.description,
        enabled: rule.enabled,
        priority: rule.priority,
        tags: rule.tags ? Object.entries(rule.tags).map(([key, value]) => ({ key, value })) : []
      });
      // 设置结构化表单数据
      setCurrentCondition(rule.conditions);
      setCurrentActions(rule.actions || []);
      // 设置JSON模式数据
      editForm.setFieldsValue({
        conditions: JSON.stringify(rule.conditions, null, 2),
        actions: JSON.stringify(rule.actions, null, 2)
      });
    } else {
      editForm.resetFields();
      const defaultCondition: Condition = {
        type: 'simple',
        field: 'key',
        operator: 'eq',
        value: 'temperature'
      };
      const defaultActions: Action[] = [{
        type: 'alert',
        config: {
          level: 'warning',
          message: '告警信息'
        }
      }];
      
      editForm.setFieldsValue({
        enabled: true,
        priority: 100,
        conditions: JSON.stringify(defaultCondition, null, 2),
        actions: JSON.stringify(defaultActions, null, 2)
      });
      setCurrentCondition(defaultCondition);
      setCurrentActions(defaultActions);
    }
    setFormMode('visual'); // 默认使用可视化模式
    setValidationErrors([]);
    updateJsonPreview();
    setEditModalVisible(true);
  };

  // 更新JSON预览
  const updateJsonPreview = () => {
    try {
      const values = editForm.getFieldsValue();
      let conditions, actions;
      
      if (formMode === 'visual') {
        conditions = currentCondition;
        actions = currentActions;
      } else {
        conditions = values.conditions ? JSON.parse(values.conditions) : undefined;
        actions = values.actions ? JSON.parse(values.actions) : [];
      }
      
      const preview = {
        name: values.name || '',
        description: values.description || '',
        enabled: values.enabled ?? true,
        priority: values.priority || 100,
        conditions,
        actions,
        tags: values.tags?.reduce((acc: any, item: any) => {
          if (item.key && item.value) {
            acc[item.key] = item.value;
          }
          return acc;
        }, {}) || {}
      };
      
      setJsonPreview(JSON.stringify(preview, null, 2));
      setValidationErrors([]);
    } catch (error: any) {
      setValidationErrors([error.message || 'JSON格式错误']);
    }
  };

  // 处理表单字段变更
  const handleFormChange = () => {
    setTimeout(updateJsonPreview, 0);
  };

  // 处理条件变更
  const handleConditionChange = (condition: Condition) => {
    setCurrentCondition(condition);
    if (formMode === 'visual') {
      editForm.setFieldsValue({
        conditions: JSON.stringify(condition, null, 2)
      });
    }
    setTimeout(updateJsonPreview, 0);
  };

  // 处理动作变更
  const handleActionsChange = (actions: Action[]) => {
    setCurrentActions(actions);
    if (formMode === 'visual') {
      editForm.setFieldsValue({
        actions: JSON.stringify(actions, null, 2)
      });
    }
    setTimeout(updateJsonPreview, 0);
  };

  // 处理模式切换
  const handleModeChange = (mode: 'visual' | 'json') => {
    if (mode === 'json' && formMode === 'visual') {
      // 从可视化模式切换到JSON模式，同步数据
      editForm.setFieldsValue({
        conditions: JSON.stringify(currentCondition, null, 2),
        actions: JSON.stringify(currentActions, null, 2)
      });
    } else if (mode === 'visual' && formMode === 'json') {
      // 从JSON模式切换到可视化模式，解析JSON数据
      try {
        const values = editForm.getFieldsValue();
        if (values.conditions) {
          const conditions = JSON.parse(values.conditions);
          setCurrentCondition(conditions);
        }
        if (values.actions) {
          const actions = JSON.parse(values.actions);
          setCurrentActions(actions);
        }
        setValidationErrors([]);
      } catch (error: any) {
        message.error('JSON格式错误，无法切换到可视化模式');
        return;
      }
    }
    setFormMode(mode);
    setTimeout(updateJsonPreview, 0);
  };

  // 保存规则
  const handleSaveRule = async () => {
    try {
      const values = await editForm.validateFields();
      
      let conditions, actions;
      if (formMode === 'visual') {
        conditions = currentCondition;
        actions = currentActions;
      } else {
        conditions = JSON.parse(values.conditions);
        actions = JSON.parse(values.actions);
      }
      
      const tags = values.tags?.reduce((acc: any, item: any) => {
        if (item.key && item.value) {
          acc[item.key] = item.value;
        }
        return acc;
      }, {});

      const ruleData = {
        name: values.name,
        description: values.description,
        enabled: values.enabled,
        priority: values.priority,
        conditions,
        actions,
        tags
      };

      if (isEditing && selectedRule) {
        await ruleService.updateRule(selectedRule.id, {
          ...ruleData,
          version: selectedRule.version
        });
        message.success('规则更新成功');
      } else {
        await ruleService.createRule(ruleData);
        message.success('规则创建成功');
      }

      setEditModalVisible(false);
      await fetchRules();
    } catch (error: any) {
      if (error instanceof SyntaxError) {
        message.error('JSON 格式错误，请检查条件和动作配置');
      } else {
        message.error('保存规则失败：' + (error.message || '未知错误'));
      }
    }
  };

  // 复制规则
  const copyRule = (rule: Rule) => {
    const newRule = {
      ...rule,
      name: `${rule.name} (副本)`,
      id: '' // 新规则使用空字符串ID
    };
    showEditModal(newRule);
  };

  // 处理模板选择
  const handleTemplateSelect = (template: any) => {
    const templateRule = template.rule;
    
    // 设置表单基本字段
    editForm.setFieldsValue({
      name: templateRule.name,
      description: templateRule.description,
      enabled: templateRule.enabled ?? true,
      priority: templateRule.priority || 100,
      tags: templateRule.tags ? Object.entries(templateRule.tags).map(([key, value]) => ({ key, value })) : []
    });
    
    // 设置结构化表单数据
    setCurrentCondition(templateRule.conditions);
    setCurrentActions(templateRule.actions || []);
    
    // 设置JSON模式数据
    editForm.setFieldsValue({
      conditions: JSON.stringify(templateRule.conditions, null, 2),
      actions: JSON.stringify(templateRule.actions, null, 2)
    });
    
    setFormMode('visual'); // 使用可视化模式
    setValidationErrors([]);
    setIsEditing(false);
    setSelectedRule(null);
    
    setTimeout(updateJsonPreview, 0);
    setEditModalVisible(true);
    
    message.success(`已加载模板：${template.name}`);
  };

  // 工具函数
  const getPriorityColor = (priority: number) => {
    if (priority >= 150) return 'red';
    if (priority >= 100) return 'orange';
    if (priority >= 50) return 'blue';
    return 'green';
  };

  const getActionTypeTag = (type: string) => {
    const colors: Record<string, string> = {
      alert: 'red',
      transform: 'blue',
      filter: 'purple',
      aggregate: 'cyan',
      forward: 'green'
    };
    return <Tag color={colors[type] || 'default'}>{type}</Tag>;
  };

  // 表格列定义
  const columns: ColumnsType<Rule> = [
    {
      title: '规则名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: Rule) => (
        <div>
          <div><strong>{name}</strong></div>
          <Text type="secondary" style={{ fontSize: 12 }}>{record.description}</Text>
        </div>
      )
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 100,
      render: (priority: number) => (
        <Tag color={getPriorityColor(priority)}>{priority}</Tag>
      ),
      sorter: (a, b) => a.priority - b.priority
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 100,
      render: (enabled: boolean, record: Rule) => (
        <Switch
          checked={enabled}
          onChange={() => toggleRuleStatus(record)}
          checkedChildren="启用"
          unCheckedChildren="禁用"
        />
      ),
      filters: [
        { text: '已启用', value: true },
        { text: '已禁用', value: false }
      ]
    },
    {
      title: '动作类型',
      dataIndex: 'actions',
      key: 'actions',
      width: 150,
      render: (actions: Action[]) => (
        <Space wrap>
          {actions && actions.length > 0 ? (
            <>
              {actions.slice(0, 2).map((action, index) => (
                <Tooltip key={index} title={JSON.stringify(action.config, null, 2)}>
                  {getActionTypeTag(action.type)}
                </Tooltip>
              ))}
              {actions.length > 2 && (
                <Tag>+{actions.length - 2}</Tag>
              )}
            </>
          ) : (
            <Tag color="default">无动作</Tag>
          )}
        </Space>
      )
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
      render: (time: string) => new Date(time).toLocaleString()
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, record: Rule) => (
        <Space>
          <Tooltip title="查看详情">
            <Button
              type="link"
              icon={<EyeOutlined />}
              onClick={() => showRuleDetails(record)}
            />
          </Tooltip>
          
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => showEditModal(record)}
            />
          </Tooltip>
          
          <Tooltip title="复制">
            <Button
              type="link"
              icon={<CopyOutlined />}
              onClick={() => copyRule(record)}
            />
          </Tooltip>
          
          <Popconfirm
            title="确定要删除这个规则吗？"
            description="删除后无法恢复"
            onConfirm={() => handleDeleteRule(record)}
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
    
    if (filters.enabled !== undefined) {
      setFilterEnabled(filters.enabled?.[0]);
    }
  };

  // 初始化和依赖更新
  useEffect(() => {
    fetchRules();
  }, [pagination.current, pagination.pageSize, searchText, filterEnabled]);

  return (
    <div>
      <Title level={2}>规则管理</Title>
      
      {/* 操作栏 */}
      <Card style={{ marginBottom: 16 }}>
        <Row gutter={16} align="middle">
          <Col flex="auto">
            <Space>
              <Search
                placeholder="搜索规则名称或描述"
                allowClear
                style={{ width: 300 }}
                onSearch={setSearchText}
                onClear={() => setSearchText('')}
              />
              <Select
                placeholder="状态筛选"
                allowClear
                style={{ width: 120 }}
                value={filterEnabled}
                onChange={setFilterEnabled}
              >
                <Option value={true}>已启用</Option>
                <Option value={false}>已禁用</Option>
              </Select>
            </Space>
          </Col>
          <Col>
            <Space>
              <Button 
                icon={<QuestionCircleOutlined />} 
                onClick={() => setHelpVisible(true)}
              >
                帮助文档
              </Button>
              <Button icon={<PlayCircleOutlined />}>
                规则测试
              </Button>
              <Button 
                icon={<SettingOutlined />}
                onClick={() => setTemplatesVisible(true)}
              >
                模板库
              </Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => showEditModal()}>
                创建规则
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 规则列表 */}
      <Card>
        <Table
          columns={columns}
          dataSource={rules}
          rowKey="id"
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

      {/* 规则详情抽屉 */}
      <Drawer
        title="规则详情"
        width={800}
        open={detailDrawerVisible}
        onClose={() => setDetailDrawerVisible(false)}
      >
        {selectedRule && (
          <Tabs
            defaultActiveKey="1"
            items={[
              {
                key: "1",
                label: "基本信息",
                children: (
                  <Descriptions title="基本信息" bordered>
                    <Descriptions.Item label="规则名称">{selectedRule.name}</Descriptions.Item>
                    <Descriptions.Item label="描述" span={2}>{selectedRule.description}</Descriptions.Item>
                    <Descriptions.Item label="状态">
                      {selectedRule.enabled ? 
                        <Tag color="green" icon={<CheckCircleOutlined />}>已启用</Tag> : 
                        <Tag color="red" icon={<PauseCircleOutlined />}>已禁用</Tag>
                      }
                    </Descriptions.Item>
                    <Descriptions.Item label="优先级">
                      <Tag color={getPriorityColor(selectedRule.priority)}>{selectedRule.priority}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="版本">v{selectedRule.version}</Descriptions.Item>
                    <Descriptions.Item label="创建时间">{new Date(selectedRule.created_at).toLocaleString()}</Descriptions.Item>
                    <Descriptions.Item label="更新时间">{new Date(selectedRule.updated_at).toLocaleString()}</Descriptions.Item>
                    <Descriptions.Item label="标签" span={2}>
                      {selectedRule.tags && Object.entries(selectedRule.tags).map(([key, value]) => (
                        <Tag key={key}>{key}: {value}</Tag>
                      ))}
                    </Descriptions.Item>
                  </Descriptions>
                )
              },
              {
                key: "2",
                label: "条件配置",
                children: (
                  <div>
                    <Title level={4}>触发条件</Title>
                    <pre style={{ background: '#f5f5f5', padding: 16, borderRadius: 4 }}>
                      {JSON.stringify(selectedRule.conditions, null, 2)}
                    </pre>
                  </div>
                )
              },
              {
                key: "3",
                label: "动作配置",
                children: (
                  <div>
                    <Title level={4}>执行动作</Title>
                    {selectedRule.actions && selectedRule.actions.length > 0 ? (
                      selectedRule.actions.map((action, index) => (
                        <Card key={index} size="small" style={{ marginBottom: 8 }}>
                          <Space>
                            {getActionTypeTag(action.type)}
                            <Text strong>动作 {index + 1}</Text>
                          </Space>
                          <pre style={{ background: '#f5f5f5', padding: 8, marginTop: 8, borderRadius: 4 }}>
                            {JSON.stringify(action.config, null, 2)}
                          </pre>
                        </Card>
                      ))
                    ) : (
                      <Text type="secondary">该规则暂无配置动作</Text>
                    )}
                  </div>
                )
              }
            ]}
          />
        )}
      </Drawer>

      {/* 编辑/创建规则模态框 */}
      <Modal
        title={
          <Space>
            {isEditing ? '编辑规则' : '创建规则'}
            <Button 
              type="link" 
              icon={<BookOutlined />} 
              onClick={() => setHelpVisible(true)}
              size="small"
            >
              帮助
            </Button>
          </Space>
        }
        open={editModalVisible}
        onOk={handleSaveRule}
        onCancel={() => setEditModalVisible(false)}
        width={1200}
        okText="保存"
        cancelText="取消"
        bodyStyle={{ maxHeight: '70vh', overflowY: 'auto' }}
      >
        <Row gutter={16}>
          <Col span={formMode === 'visual' ? 24 : 14}>
            {/* 模式切换 */}
            <Card size="small" style={{ marginBottom: 16 }}>
              <Row justify="space-between" align="middle">
                <Col>
                  <Segmented
                    value={formMode}
                    onChange={handleModeChange}
                    options={[
                      {
                        label: (
                          <Space>
                            <FormOutlined />
                            可视化编辑
                          </Space>
                        ),
                        value: 'visual'
                      },
                      {
                        label: (
                          <Space>
                            <CodeOutlined />
                            JSON编辑
                          </Space>
                        ),
                        value: 'json'
                      }
                    ]}
                  />
                </Col>
                <Col>
                  {validationErrors.length > 0 && (
                    <Alert
                      message="配置错误"
                      description={validationErrors.join(', ')}
                      type="error"
                      showIcon
                    />
                  )}
                </Col>
              </Row>
            </Card>

            <Form form={editForm} layout="vertical" onValuesChange={handleFormChange}>
              {/* 基本信息 */}
              <Card title="基本信息" size="small" style={{ marginBottom: 16 }}>
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
                      label="优先级"
                      name="priority"
                      rules={[{ required: true, message: '请输入优先级' }]}
                    >
                      <InputNumber 
                        placeholder="数值越大优先级越高" 
                        style={{ width: '100%' }}
                        min={1}
                        max={999}
                      />
                    </Form.Item>
                  </Col>
                </Row>
                
                <Form.Item
                  label="描述"
                  name="description"
                  rules={[{ required: true, message: '请输入规则描述' }]}
                >
                  <TextArea rows={2} placeholder="输入规则描述" />
                </Form.Item>
                
                <Form.Item label="启用状态" name="enabled" valuePropName="checked">
                  <Switch checkedChildren="启用" unCheckedChildren="禁用" />
                </Form.Item>
              </Card>

              {/* 条件和动作配置 */}
              {formMode === 'visual' ? (
                <>
                  <ConditionForm 
                    value={currentCondition} 
                    onChange={handleConditionChange}
                  />
                  <div style={{ marginBottom: 16 }} />
                  <ActionForm 
                    value={currentActions} 
                    onChange={handleActionsChange}
                  />
                </>
              ) : (
                <>
                  <Card title="JSON配置" size="small">
                    <Form.Item
                      label="触发条件 (JSON 格式)"
                      name="conditions"
                      rules={[
                        { required: true, message: '请输入触发条件' },
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
                      <TextArea
                        rows={8}
                        placeholder='{"type": "simple", "field": "key", "operator": "eq", "value": "temperature"}'
                        style={{ fontFamily: 'monospace' }}
                      />
                    </Form.Item>
                    
                    <Form.Item
                      label="执行动作 (JSON 格式)"
                      name="actions"
                      rules={[
                        { required: true, message: '请输入执行动作' },
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
                      <TextArea
                        rows={8}
                        placeholder='[{"type": "alert", "config": {"level": "warning", "message": "告警信息"}}]'
                        style={{ fontFamily: 'monospace' }}
                      />
                    </Form.Item>
                  </Card>
                </>
              )}
            </Form>
          </Col>
          
          {formMode === 'json' && (
            <Col span={10}>
              <Card title="实时预览" size="small" style={{ position: 'sticky', top: 0 }}>
                <pre style={{
                  background: '#f5f5f5',
                  padding: 12,
                  borderRadius: 4,
                  fontSize: 12,
                  lineHeight: 1.4,
                  maxHeight: '60vh',
                  overflow: 'auto'
                }}>
                  {jsonPreview}
                </pre>
              </Card>
            </Col>
          )}
        </Row>
      </Modal>

      {/* 帮助抽屉 */}
      <RuleHelp 
        visible={helpVisible} 
        onClose={() => setHelpVisible(false)} 
      />

      {/* 规则模板选择 */}
      <RuleTemplates
        visible={templatesVisible}
        onClose={() => setTemplatesVisible(false)}
        onSelect={handleTemplateSelect}
      />
    </div>
  );
};

export default RulesPage; 