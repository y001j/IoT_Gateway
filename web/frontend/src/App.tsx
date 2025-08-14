import React, { useEffect, Suspense, useState } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { ConfigProvider, theme, Spin, App as AntdApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { useAuthStore } from './store/authStore';
import ProtectedRoute from './components/router/ProtectedRoute';
import MainLayout from './components/layout/MainLayout';

// 使用React.lazy进行路由级别的代码分割
const Login = React.lazy(() => import('./pages/Login'));
const Dashboard = React.lazy(() => import('./pages/Dashboard'));
const PluginsPage = React.lazy(() => import('./pages/PluginsPage'));
const RulesPage = React.lazy(() => import('./pages/RulesPage'));
const SystemPage = React.lazy(() => import('./pages/SystemPage'));
const AlertsPage = React.lazy(() => import('./pages/AlertsPage').then(module => ({ default: module.AlertsPage })));
const MonitoringPage = React.lazy(() => import('./pages/MonitoringPage'));

import './App.css';

// 全局加载组件
const PageLoader = () => (
  <div style={{ 
    display: 'flex', 
    flexDirection: 'column',
    justifyContent: 'center', 
    alignItems: 'center', 
    height: '200px' 
  }}>
    <Spin size="large" />
    <div style={{ marginTop: '12px', color: '#666' }}>页面加载中...</div>
  </div>
);

// 应用初始化加载组件
const AppLoader = () => (
  <div style={{ 
    display: 'flex', 
    flexDirection: 'column',
    justifyContent: 'center', 
    alignItems: 'center', 
    height: '100vh',
    background: '#f0f2f5'
  }}>
    <Spin size="large" />
    <div style={{ marginTop: '16px', color: '#666', fontSize: '16px' }}>
      正在初始化应用...
    </div>
    <div style={{ marginTop: '8px', color: '#999', fontSize: '14px' }}>
      正在验证登录状态...
    </div>
  </div>
);

function App() {
  const { initialize, isInitialized } = useAuthStore();
  const [initError, setInitError] = useState<string | null>(null);

  useEffect(() => {
    const initAuth = async () => {
      try {
        console.log('🚀 开始初始化认证状态...');
        await initialize();
        console.log('✅ 认证状态初始化完成');
      } catch (error) {
        console.error('❌ 认证初始化失败:', error);
        setInitError('认证初始化失败，请刷新页面重试');
      }
    };

    initAuth();
  }, [initialize]);

  // 如果还在初始化中，显示加载页面
  if (!isInitialized) {
    if (initError) {
      return (
        <div style={{ 
          display: 'flex', 
          flexDirection: 'column',
          justifyContent: 'center', 
          alignItems: 'center', 
          height: '100vh',
          background: '#f0f2f5'
        }}>
          <div style={{ color: '#ff4d4f', marginBottom: '16px' }}>
            {initError}
          </div>
          <button 
            onClick={() => window.location.reload()} 
            style={{ padding: '8px 16px', cursor: 'pointer' }}
          >
            刷新页面
          </button>
        </div>
      );
    }
    return <AppLoader />;
  }

  return (
    <ConfigProvider 
      locale={zhCN}
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#16a34a', // 绿色主色调
          colorSuccess: '#16a34a',
          colorInfo: '#16a34a',
          colorLink: '#16a34a',
          borderRadius: 6,
        },
        components: {
          Button: {
            colorPrimary: '#16a34a',
            colorPrimaryHover: '#15803d',
            colorPrimaryActive: '#166534',
          },
          Menu: {
            colorPrimary: '#16a34a',
            colorPrimaryBorder: '#16a34a',
            colorPrimaryHover: '#15803d',
          },
          Tabs: {
            colorPrimary: '#16a34a',
            colorPrimaryHover: '#15803d',
            colorPrimaryActive: '#166534',
          },
          Select: {
            colorPrimary: '#16a34a',
            colorPrimaryHover: '#15803d',
          },
          Input: {
            colorPrimary: '#16a34a',
            colorPrimaryHover: '#15803d',
          },
          Switch: {
            colorPrimary: '#16a34a',
          },
          Progress: {
            colorPrimary: '#16a34a',
          },
        }
      }}
    >
      <AntdApp>
        <Router>
          <Suspense fallback={<PageLoader />}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route 
                path="/"
                element={
                  <ProtectedRoute>
                    <MainLayout />
                  </ProtectedRoute>
                }
              >
                <Route index element={
                  <Suspense fallback={<PageLoader />}>
                    <Dashboard />
                  </Suspense>
                } />
                <Route path="plugins" element={
                  <Suspense fallback={<PageLoader />}>
                    <PluginsPage />
                  </Suspense>
                } />
                <Route path="rules" element={
                  <Suspense fallback={<PageLoader />}>
                    <RulesPage />
                  </Suspense>
                } />
                <Route path="alerts" element={
                  <Suspense fallback={<PageLoader />}>
                    <AlertsPage />
                  </Suspense>
                } />
                <Route path="monitoring" element={
                  <Suspense fallback={<PageLoader />}>
                    <MonitoringPage />
                  </Suspense>
                } />
                <Route path="system" element={
                  <Suspense fallback={<PageLoader />}>
                    <SystemPage />
                  </Suspense>
                } />
              </Route>
            </Routes>
          </Suspense>
        </Router>
      </AntdApp>
    </ConfigProvider>
  );
}

export default App; 