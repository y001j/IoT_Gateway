import React, { useEffect } from 'react';
import { Navigate, Outlet } from 'react-router-dom';
import { useAuthStore } from '../../store/authStore';

interface ProtectedRouteProps {
  children?: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const { isAuthenticated, isInitialized, accessToken, user } = useAuthStore();

  useEffect(() => {
    console.log('ProtectedRoute状态更新:', {
      isInitialized,
      isAuthenticated,
      hasToken: !!accessToken,
      hasUser: !!user,
      hasChildren: !!children,
      timestamp: new Date().toISOString()
    });
  }, [isInitialized, isAuthenticated, accessToken, user, children]);

  // 如果还在初始化，不做任何跳转（App.tsx会处理初始化加载状态）
  if (!isInitialized) {
    console.log('认证状态还在初始化中...');
    return null;
  }

  if (!isAuthenticated) {
    console.log('用户未认证，跳转到登录页');
    return <Navigate to="/login" replace />;
  }

  console.log('用户已认证，渲染受保护的内容');
  return children ? children : <Outlet />;
};

export default ProtectedRoute; 