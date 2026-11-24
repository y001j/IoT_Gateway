import React, { useState, useEffect } from 'react';
import { Form, Input, Button, Card, Typography, Alert, Space, Divider } from 'antd';
import { UserOutlined, LockOutlined, WifiOutlined, CheckCircleOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { authService } from '../services/authService';
import { useAuthStore } from '../store/authStore';

const { Title, Text } = Typography;

interface LoginCredentials {
  username: string;
  password: string;
}

const Login: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [backendStatus, setBackendStatus] = useState<'checking' | 'online' | 'offline'>('checking');
  const navigate = useNavigate();
  const { isAuthenticated, isInitialized, logout } = useAuthStore();

  // 检查后端服务状态
  const checkBackendStatus = async () => {
    setBackendStatus('checking');
    try {
      // 使用一个简单的GET请求来检查后端是否在线
      // 这会触发CORS preflight，如果服务离线会立即失败
      const response = await fetch('/api/v1/plugins?page=1&page_size=1', {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json'
        }
      });
      
      // 不管返回什么状态码，只要有响应就说明服务在线
      setBackendStatus('online');
      console.log('✅ 后端服务在线，状态码:', response.status);
    } catch (error) {
      setBackendStatus('offline');
      console.log('❌ 后端服务离线:', error);
    }
  };

  // 组件挂载时检查后端状态
  useEffect(() => {
    checkBackendStatus();
    // 每30秒检查一次后端状态
    const interval = setInterval(checkBackendStatus, 30000);
    return () => clearInterval(interval);
  }, []);

  // 如果已经认证，跳转到首页
  useEffect(() => {
    if (isInitialized && isAuthenticated) {
      console.log('✅ 用户已认证，跳转到首页');
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, isInitialized, navigate]);

  const onFinish = async (values: LoginCredentials) => {
    console.log('🚀 开始登录流程，用户名:', values.username);
    
    setLoading(true);
    setError(null);
    setSuccess(null);

    // 先检查后端状态
    if (backendStatus === 'offline') {
      setError('后端服务不可用，请确认服务已启动。');
      setLoading(false);
      return;
    }

    try {
      console.log('📤 发送登录请求...');
      
      // 清理旧的认证信息
      logout();
      
      // 等待一下让状态清理完成
      await new Promise(resolve => setTimeout(resolve, 100));
      
      // 发送登录请求
      const result = await authService.login(values);
      console.log('✅ 登录成功:', result);

      // 验证登录结果
      const currentState = useAuthStore.getState();
      console.log('📋 当前认证状态:', {
        hasAccessToken: !!currentState.accessToken,
        hasRefreshToken: !!currentState.refreshToken,
        hasUser: !!currentState.user,
        isAuthenticated: currentState.isAuthenticated,
        isInitialized: currentState.isInitialized
      });

      // 检查localStorage存储
      console.log('💾 检查存储状态...');

      if (currentState.isAuthenticated && currentState.accessToken) {
        setSuccess('登录成功！正在跳转到首页...');
        
        // 延迟跳转，让用户看到成功消息
        setTimeout(() => {
          console.log('🔄 跳转到首页');
          navigate('/', { replace: true });
        }, 1500);
      } else {
        throw new Error('登录响应正常，但认证状态未正确设置');
      }

    } catch (err: any) {
      console.error('❌ 登录失败:', err);
      
      let errorMessage = '登录失败，请重试。';
      
      if (err.name === 'NetworkError') {
        errorMessage = '网络连接失败，请检查后端服务是否运行。';
        setBackendStatus('offline');
      } else if (err.name === 'AuthExpiredError') {
        errorMessage = '认证已过期，请重新登录。';
      } else if (err.response?.status === 401) {
        errorMessage = '用户名或密码错误，请检查后重试。';
      } else if (err.response?.status === 400) {
        errorMessage = err.response.data?.message || '请求参数错误，请检查输入。';
      } else if (err.response?.status === 500) {
        errorMessage = '服务器内部错误，请稍后重试。';
      } else if (err.message) {
        errorMessage = err.message;
      }
      
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const renderBackendStatus = () => {
    switch (backendStatus) {
      case 'checking':
        return (
          <Alert
            message="检查后端服务状态中..."
            type="info"
            showIcon
            icon={<WifiOutlined spin />}
            style={{ marginBottom: 16 }}
          />
        );
      case 'online':
        return (
          <Alert
            message="后端服务正常"
            type="success"
            showIcon
            icon={<CheckCircleOutlined />}
            style={{ marginBottom: 16 }}
          />
        );
      case 'offline':
        return (
          <Alert
            message="后端服务不可用"
            description="请确认IoT Gateway服务已启动，并运行在端口8081上。"
            type="error"
            showIcon
            icon={<ExclamationCircleOutlined />}
            style={{ marginBottom: 16 }}
            action={
              <Button size="small" onClick={checkBackendStatus}>
                重新检查
              </Button>
            }
          />
        );
    }
  };

  return (
    <div style={{ 
      display: 'flex', 
      justifyContent: 'center', 
      alignItems: 'center', 
      minHeight: '100vh', 
      background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      padding: '20px'
    }}>
      <Card style={{ 
        width: 450, 
        boxShadow: '0 10px 30px rgba(0,0,0,0.1)',
        borderRadius: '12px'
      }}>
        {/* 标题 */}
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title level={2} style={{ color: '#1890ff', marginBottom: 8 }}>
            IoT Gateway
          </Title>
          <Text type="secondary">物联网网关管理系统</Text>
        </div>

        {/* 后端状态 */}
        {renderBackendStatus()}

        {/* 错误和成功消息 */}
        {error && (
          <Alert 
            message={error} 
            type="error" 
            showIcon 
            closable
            onClose={() => setError(null)}
            style={{ marginBottom: 16 }} 
          />
        )}
        
        {success && (
          <Alert 
            message={success} 
            type="success" 
            showIcon 
            style={{ marginBottom: 16 }} 
          />
        )}

        <Divider />

        {/* 登录表单 */}
        <Form
          form={form}
          name="login"
          initialValues={{ 
            username: 'admin'
          }}
          onFinish={onFinish}
          size="large"
        >
          <Form.Item
            name="username"
            rules={[
              { required: true, message: '请输入用户名' },
              { min: 3, message: '用户名至少3个字符' }
            ]}
          >
            <Input 
              prefix={<UserOutlined />} 
              placeholder="用户名"
              autoComplete="username"
            />
          </Form.Item>

          <Form.Item
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 6, message: '密码至少6个字符' }
            ]}
          >
            <Input.Password 
              prefix={<LockOutlined />} 
              placeholder="密码"
              autoComplete="current-password"
            />
          </Form.Item>

          <Form.Item>
            <Button 
              type="primary" 
              htmlType="submit" 
              loading={loading}
              disabled={backendStatus === 'offline'}
              style={{ width: '100%', height: '45px' }}
            >
              {loading ? '登录中...' : '登录'}
            </Button>
          </Form.Item>
        </Form>

        <Divider />

        {/* 调试工具 */}
        {process.env.NODE_ENV === 'development' && (
          <Space direction="vertical" style={{ width: '100%' }} size="small">
            <Text type="secondary" style={{ fontSize: '12px' }}>
              开发模式调试工具
            </Text>
            <Space>
              <Button 
                size="small" 
                onClick={() => console.log('Storage:', localStorage.getItem('accessToken'))}
              >
                检查存储
              </Button>
              <Button 
                size="small" 
                onClick={() => console.log('Auth Store:', useAuthStore.getState())}
              >
                调试认证
              </Button>
              <Button 
                size="small" 
                onClick={checkBackendStatus}
              >
                检查后端
              </Button>
            </Space>
          </Space>
        )}
      </Card>
    </div>
  );
};

export default Login;