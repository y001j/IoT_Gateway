import React, { useState, useEffect } from 'react';
import {
  Form,
  Input,
  Button,
  Card,
  Row,
  Col,
  message,
  Alert,
  Space,
  Switch,
  Typography,
  Table,
  Modal,
  Popconfirm,
  Tag,
  Checkbox,
  Divider
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  TeamOutlined,
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  SafetyOutlined,
  KeyOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { UserRole, Permission } from '../../types/settings';

const { Title, Text } = Typography;
const { TextArea } = Input;

const RoleManagement: React.FC = () => {
  const [roleForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [roles, setRoles] = useState<UserRole[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [roleModalVisible, setRoleModalVisible] = useState(false);
  const [editingRole, setEditingRole] = useState<UserRole | null>(null);

  // 加载角色和权限数据
  const loadData = async () => {
    setLoading(true);
    try {
      const [rolesResponse, permissionsResponse] = await Promise.all([
        settingsService.getRoles(),
        settingsService.getPermissions()
      ]);
      
      if (rolesResponse.success) {
        setRoles(rolesResponse.data);
      }
      
      if (permissionsResponse.success) {
        setPermissions(permissionsResponse.data);
      }
    } catch (error: any) {
      message.error('加载数据失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 创建/编辑角色
  const handleSaveRole = async () => {
    try {
      const values = await roleForm.validateFields();
      
      if (editingRole) {
        // 更新角色
        const response = await settingsService.updateRole(editingRole.id, values);
        if (response.success) {
          message.success('角色更新成功');
          setRoles(roles.map(role => role.id === editingRole.id ? { ...role, ...values } : role));
        } else {
          message.error('更新失败：' + (response.message || '未知错误'));
        }
      } else {
        // 创建新角色
        const response = await settingsService.createRole(values);
        if (response.success) {
          message.success('角色创建成功');
          setRoles([...roles, response.data]);
        } else {
          message.error('创建失败：' + (response.message || '未知错误'));
        }
      }
      
      setRoleModalVisible(false);
      setEditingRole(null);
      roleForm.resetFields();
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 删除角色
  const handleDeleteRole = async (id: string) => {
    try {
      const response = await settingsService.deleteRole(id);
      if (response.success) {
        message.success('角色删除成功');
        setRoles(roles.filter(role => role.id !== id));
      } else {
        message.error('删除失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('删除失败：' + (error.message || '未知错误'));
    }
  };

  // 打开编辑模态框
  const handleEditRole = (role: UserRole) => {
    setEditingRole(role);
    roleForm.setFieldsValue({
      name: role.name,
      description: role.description,
      permissions: role.permissions,
      enabled: role.enabled
    });
    setRoleModalVisible(true);
  };

  // 获取权限名称
  const getPermissionName = (permissionId: string) => {
    const permission = permissions.find(p => p.id === permissionId);
    return permission ? permission.name : permissionId;
  };

  // 按分类分组权限
  const groupPermissionsByCategory = () => {
    const grouped: { [key: string]: Permission[] } = {};
    permissions.forEach(permission => {
      if (!grouped[permission.category]) {
        grouped[permission.category] = [];
      }
      grouped[permission.category].push(permission);
    });
    return grouped;
  };

  // 角色表格列
  const roleColumns = [
    {
      title: '角色名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <Space>
          <TeamOutlined />
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
    },
    {
      title: '权限数量',
      dataIndex: 'permissions',
      key: 'permissions_count',
      render: (permissions: string[]) => (
        <Tag color="blue">{permissions ? permissions.length : 0} 个权限</Tag>
      ),
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
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: UserRole) => (
        <Space>
          <Button 
            type="text" 
            icon={<EditOutlined />}
            onClick={() => handleEditRole(record)}
          >
            编辑
          </Button>
          
          <Popconfirm
            title="确定要删除这个角色吗？"
            onConfirm={() => handleDeleteRole(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="text" icon={<DeleteOutlined />} danger>
              删除
            </Button>
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
        message="角色管理说明"
        description="管理系统角色和权限分配。角色定义了用户能够执行的操作，权限控制了对特定功能的访问。请谨慎修改管理员角色的权限。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      {/* 角色列表 */}
      <Card 
        title={
          <Space>
            <TeamOutlined />
            角色管理
            <Button 
              type="primary" 
              size="small"
              icon={<PlusOutlined />}
              onClick={() => {
                setEditingRole(null);
                roleForm.resetFields();
                setRoleModalVisible(true);
              }}
            >
              添加角色
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
          dataSource={roles}
          columns={roleColumns}
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

      {/* 创建/编辑角色模态框 */}
      <Modal
        title={editingRole ? '编辑角色' : '创建角色'}
        open={roleModalVisible}
        onOk={handleSaveRole}
        onCancel={() => {
          setRoleModalVisible(false);
          setEditingRole(null);
          roleForm.resetFields();
        }}
        okText="保存"
        cancelText="取消"
        width={800}
      >
        <Form form={roleForm} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                label="角色名称"
                name="name"
                rules={[
                  { required: true, message: '请输入角色名称' },
                  { min: 2, message: '角色名称至少2个字符' },
                  { max: 50, message: '角色名称最多50个字符' }
                ]}
              >
                <Input placeholder="输入角色名称" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="启用角色"
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

          <Form.Item
            label="角色描述"
            name="description"
            rules={[{ required: true, message: '请输入角色描述' }]}
          >
            <TextArea rows={3} placeholder="输入角色描述" />
          </Form.Item>

          <Form.Item
            label="权限分配"
            name="permissions"
            rules={[{ required: true, message: '请选择角色权限' }]}
          >
            <Checkbox.Group style={{ width: '100%' }}>
              {Object.entries(groupPermissionsByCategory()).map(([category, perms]) => (
                <div key={category} style={{ marginBottom: 16 }}>
                  <Title level={5} style={{ marginBottom: 8 }}>
                    <SafetyOutlined /> {category}
                  </Title>
                  <Row>
                    {perms.map(permission => (
                      <Col span={8} key={permission.id} style={{ marginBottom: 8 }}>
                        <Checkbox value={permission.id}>
                          <Space>
                            <KeyOutlined style={{ fontSize: '12px' }} />
                            <div>
                              <div style={{ fontWeight: 500 }}>{permission.name}</div>
                              <div style={{ fontSize: '12px', color: '#666' }}>
                                {permission.description}
                              </div>
                            </div>
                          </Space>
                        </Checkbox>
                      </Col>
                    ))}
                  </Row>
                  <Divider />
                </div>
              ))}
            </Checkbox.Group>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default RoleManagement;