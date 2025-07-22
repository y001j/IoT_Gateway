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
  Select,
  Typography,
  Table,
  Modal,
  Popconfirm,
  Tag,
  Avatar,
  Tooltip
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  UserOutlined,
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  LockOutlined,
  UnlockOutlined,
  EyeOutlined,
  UserAddOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { User, UserRole } from '../../types/settings';

const { Option } = Select;
const { Title, Text } = Typography;

const UserManagement: React.FC = () => {
  const [userForm] = Form.useForm();
  const [passwordForm] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [users, setUsers] = useState<User[]>([]);
  const [roles, setRoles] = useState<UserRole[]>([]);
  const [userModalVisible, setUserModalVisible] = useState(false);
  const [passwordModalVisible, setPasswordModalVisible] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);

  // 加载用户和角色数据
  const loadData = async () => {
    setLoading(true);
    try {
      const [usersResponse, rolesResponse] = await Promise.all([
        settingsService.getUsers(),
        settingsService.getRoles()
      ]);
      
      if (usersResponse.success) {
        setUsers(usersResponse.data);
      }
      
      if (rolesResponse.success) {
        setRoles(rolesResponse.data);
      }
    } catch (error: any) {
      message.error('加载数据失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 创建/编辑用户
  const handleSaveUser = async () => {
    try {
      const values = await userForm.validateFields();
      
      if (editingUser) {
        // 更新用户
        const response = await settingsService.updateUser(editingUser.id, values);
        if (response.success) {
          message.success('用户更新成功');
          setUsers(users.map(user => user.id === editingUser.id ? { ...user, ...values } : user));
        } else {
          message.error('更新失败：' + (response.message || '未知错误'));
        }
      } else {
        // 创建新用户
        const response = await settingsService.createUser(values);
        if (response.success) {
          message.success('用户创建成功');
          setUsers([...users, response.data]);
        } else {
          message.error('创建失败：' + (response.message || '未知错误'));
        }
      }
      
      setUserModalVisible(false);
      setEditingUser(null);
      userForm.resetFields();
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 删除用户
  const handleDeleteUser = async (id: string) => {
    try {
      const response = await settingsService.deleteUser(id);
      if (response.success) {
        message.success('用户删除成功');
        setUsers(users.filter(user => user.id !== id));
      } else {
        message.error('删除失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('删除失败：' + (error.message || '未知错误'));
    }
  };

  // 重置密码
  const handleResetPassword = async () => {
    try {
      const values = await passwordForm.validateFields();
      
      if (selectedUser) {
        const response = await settingsService.resetUserPassword(selectedUser.id, values.password);
        if (response.success) {
          message.success('密码重置成功');
          setPasswordModalVisible(false);
          setSelectedUser(null);
          passwordForm.resetFields();
        } else {
          message.error('重置失败：' + (response.message || '未知错误'));
        }
      }
    } catch (error: any) {
      message.error('重置失败：' + (error.message || '未知错误'));
    }
  };

  // 启用/禁用用户
  const handleToggleUser = async (user: User) => {
    try {
      const response = await settingsService.updateUser(user.id, { enabled: !user.enabled });
      if (response.success) {
        message.success(`用户${user.enabled ? '禁用' : '启用'}成功`);
        setUsers(users.map(u => u.id === user.id ? { ...u, enabled: !u.enabled } : u));
      } else {
        message.error('操作失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      message.error('操作失败：' + (error.message || '未知错误'));
    }
  };

  // 打开编辑模态框
  const handleEditUser = (user: User) => {
    setEditingUser(user);
    userForm.setFieldsValue({
      username: user.username,
      email: user.email,
      role_id: user.role_id,
      enabled: user.enabled
    });
    setUserModalVisible(true);
  };

  // 打开密码重置模态框
  const handleOpenPasswordModal = (user: User) => {
    setSelectedUser(user);
    passwordForm.resetFields();
    setPasswordModalVisible(true);
  };

  // 获取角色名称
  const getRoleName = (roleId: string) => {
    const role = roles.find(r => r.id === roleId);
    return role ? role.name : '未知角色';
  };

  // 获取角色颜色
  const getRoleColor = (roleId: string) => {
    const role = roles.find(r => r.id === roleId);
    if (!role) return 'default';
    
    switch (role.name) {
      case 'admin':
        return 'red';
      case 'operator':
        return 'orange';
      case 'viewer':
        return 'green';
      default:
        return 'blue';
    }
  };

  // 用户表格列
  const userColumns = [
    {
      title: '用户',
      key: 'user',
      render: (_: any, record: User) => (
        <Space>
          <Avatar icon={<UserOutlined />} size="small" />
          <div>
            <div style={{ fontWeight: 500 }}>{record.username}</div>
            <div style={{ fontSize: '12px', color: '#666' }}>{record.email}</div>
          </div>
        </Space>
      ),
    },
    {
      title: '角色',
      dataIndex: 'role_id',
      key: 'role',
      render: (roleId: string) => (
        <Tag color={getRoleColor(roleId)}>
          {getRoleName(roleId)}
        </Tag>
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
      title: '最后登录',
      dataIndex: 'last_login',
      key: 'last_login',
      render: (date?: string) => date ? new Date(date).toLocaleString() : '从未登录',
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
      render: (_: any, record: User) => (
        <Space>
          <Tooltip title="编辑用户">
            <Button 
              type="text" 
              icon={<EditOutlined />}
              onClick={() => handleEditUser(record)}
            />
          </Tooltip>
          
          <Tooltip title="重置密码">
            <Button 
              type="text" 
              icon={<LockOutlined />}
              onClick={() => handleOpenPasswordModal(record)}
            />
          </Tooltip>
          
          <Tooltip title={record.enabled ? '禁用用户' : '启用用户'}>
            <Button 
              type="text" 
              icon={record.enabled ? <LockOutlined /> : <UnlockOutlined />}
              onClick={() => handleToggleUser(record)}
            />
          </Tooltip>
          
          <Popconfirm
            title="确定要删除这个用户吗？"
            onConfirm={() => handleDeleteUser(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Tooltip title="删除用户">
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
        message="用户管理说明"
        description="管理系统用户账户，包括用户创建、角色分配、密码重置和账户启用/禁用。请谨慎操作管理员账户。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      {/* 用户列表 */}
      <Card 
        title={
          <Space>
            <UserOutlined />
            用户管理
            <Button 
              type="primary" 
              size="small"
              icon={<UserAddOutlined />}
              onClick={() => {
                setEditingUser(null);
                userForm.resetFields();
                setUserModalVisible(true);
              }}
            >
              添加用户
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
          dataSource={users}
          columns={userColumns}
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

      {/* 创建/编辑用户模态框 */}
      <Modal
        title={editingUser ? '编辑用户' : '创建用户'}
        open={userModalVisible}
        onOk={handleSaveUser}
        onCancel={() => {
          setUserModalVisible(false);
          setEditingUser(null);
          userForm.resetFields();
        }}
        okText="保存"
        cancelText="取消"
      >
        <Form form={userForm} layout="vertical">
          <Form.Item
            label="用户名"
            name="username"
            rules={[
              { required: true, message: '请输入用户名' },
              { min: 3, message: '用户名至少3个字符' },
              { max: 50, message: '用户名最多50个字符' }
            ]}
          >
            <Input placeholder="输入用户名" />
          </Form.Item>

          <Form.Item
            label="邮箱"
            name="email"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '请输入有效的邮箱地址' }
            ]}
          >
            <Input placeholder="输入邮箱地址" />
          </Form.Item>

          <Form.Item
            label="角色"
            name="role_id"
            rules={[{ required: true, message: '请选择用户角色' }]}
          >
            <Select placeholder="选择用户角色">
              {roles.map(role => (
                <Option key={role.id} value={role.id}>
                  {role.name} - {role.description}
                </Option>
              ))}
            </Select>
          </Form.Item>

          {!editingUser && (
            <Form.Item
              label="密码"
              name="password"
              rules={[
                { required: true, message: '请输入密码' },
                { min: 6, message: '密码至少6个字符' }
              ]}
            >
              <Input.Password placeholder="输入密码" />
            </Form.Item>
          )}

          <Form.Item
            label="启用用户"
            name="enabled"
            valuePropName="checked"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* 重置密码模态框 */}
      <Modal
        title="重置密码"
        open={passwordModalVisible}
        onOk={handleResetPassword}
        onCancel={() => {
          setPasswordModalVisible(false);
          setSelectedUser(null);
          passwordForm.resetFields();
        }}
        okText="重置"
        cancelText="取消"
      >
        <Form form={passwordForm} layout="vertical">
          <Alert
            message={`正在为用户 "${selectedUser?.username}" 重置密码`}
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
          
          <Form.Item
            label="新密码"
            name="password"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 6, message: '密码至少6个字符' }
            ]}
          >
            <Input.Password placeholder="输入新密码" />
          </Form.Item>

          <Form.Item
            label="确认密码"
            name="confirm_password"
            dependencies={['password']}
            rules={[
              { required: true, message: '请确认密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) {
                    return Promise.resolve();
                  }
                  return Promise.reject(new Error('两次密码输入不一致'));
                },
              }),
            ]}
          >
            <Input.Password placeholder="确认新密码" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default UserManagement;