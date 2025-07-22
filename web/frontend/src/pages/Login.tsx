import React, { useState } from 'react';
import { Form, Input, Button, Card, Typography, Alert } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { authService } from '../services/authService';
import { useAuthStore } from '../store/authStore';

const { Title } = Typography;

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const navigate = useNavigate();
  const { setTokens, setUser, isAuthenticated, isInitialized } = useAuthStore();

  const onFinish = async (values: any) => {
    console.log('开始登录，用户名:', values.username);
    setLoading(true);
    setError(null);
    setSuccess(null);
    
    try {
      console.log('发送登录请求...');
      const data = await authService.login(values);
      console.log('登录请求成功，收到数据:', data);
      
      setTokens(data.token, data.refresh_token);
      setUser(data.user);
      
      console.log('设置token和用户信息完成');
      console.log('当前认证状态:', useAuthStore.getState().isAuthenticated);
      
      setSuccess('登录成功！正在跳转...');
      
      // 短暂延迟以显示成功消息
      setTimeout(() => {
        console.log('准备跳转到首页...');
        navigate('/', { replace: true });
      }, 1000);
      
    } catch (err: any) {
      console.error('登录失败:', err);
      console.error('错误详情:', err.response?.data);
      setError(err.response?.data?.message || err.message || '登录失败，请检查您的用户名和密码。');
    } finally {
      setLoading(false);
    }
  };

  // 如果已经认证并且初始化完成，直接跳转
  React.useEffect(() => {
    // 只有在初始化完成且已认证的情况下才跳转
    if (isInitialized && isAuthenticated) {
      console.log('用户已认证且初始化完成，跳转到首页');
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, isInitialized, navigate]);

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
      <Card style={{ width: 400 }}>
        <div style={{ textAlign: 'center', marginBottom: '24px' }}>
          <Title level={2}>IoT Gateway 登录</Title>
        </div>
        {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 24 }} />}
        {success && <Alert message={success} type="success" showIcon style={{ marginBottom: 24 }} />}
        <Form
          name="login"
          initialValues={{ username: 'admin', password: 'admin123' }}
          onFinish={onFinish}
        >
          <Form.Item
            name="username"
            rules={[{ required: true, message: '请输入用户名!' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="用户名" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码!' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} style={{ width: '100%' }}>
              登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default Login; 