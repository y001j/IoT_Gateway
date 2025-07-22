import React, { useState } from 'react';
import { Layout, Menu, type MenuProps } from 'antd';
import { Outlet, Link, useLocation } from 'react-router-dom';
import {
  DashboardOutlined,
  DeploymentUnitOutlined,
  ApartmentOutlined,
  AlertOutlined,
  MonitorOutlined,
  SettingOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { authService } from '../../services/authService';

const { Header, Content, Sider } = Layout;

type MenuItem = Required<MenuProps>['items'][number];

const MainLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false);
  const location = useLocation();

  const handleLogout = () => {
    authService.logout();
    // The protected route will handle the redirect
  };

  // 根据当前路径获取选中的菜单项
  const getSelectedKeys = () => {
    const path = location.pathname;
    if (path === '/' || path.startsWith('/dashboard')) return ['1'];
    if (path.startsWith('/plugins')) return ['2'];
    if (path.startsWith('/rules')) return ['3'];
    if (path.startsWith('/alerts')) return ['4'];
    if (path.startsWith('/monitoring')) return ['5'];
    if (path.startsWith('/system')) return ['6'];
    return ['1'];
  };

  // 侧边栏菜单项
  const sideMenuItems: MenuItem[] = [
    {
      key: '1',
      icon: <DashboardOutlined />,
      label: <Link to="/">仪表盘</Link>,
    },
    {
      key: '2',
      icon: <DeploymentUnitOutlined />,
      label: <Link to="/plugins">插件管理</Link>,
    },
    {
      key: '3',
      icon: <ApartmentOutlined />,
      label: <Link to="/rules">规则管理</Link>,
    },
    {
      key: '4',
      icon: <AlertOutlined />,
      label: <Link to="/alerts">告警管理</Link>,
    },
    {
      key: '5',
      icon: <MonitorOutlined />,
      label: <Link to="/monitoring">连接监控</Link>,
    },
    {
      key: '6',
      icon: <SettingOutlined />,
      label: <Link to="/system">系统设置</Link>,
    },
  ];

  // 顶部菜单项
  const topMenuItems: MenuItem[] = [
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      onClick: handleLogout,
    },
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider 
        collapsible 
        collapsed={collapsed} 
        onCollapse={setCollapsed}
        width={256}
      >
        <div 
          className="logo" 
          style={{ 
            height: '32px', 
            margin: '16px', 
            background: 'rgba(255, 255, 255, 0.2)',
            borderRadius: '4px'
          }} 
        />
        <Menu 
          theme="dark" 
          selectedKeys={getSelectedKeys()}
          mode="inline"
          items={sideMenuItems}
        />
      </Sider>
      <Layout className="site-layout">
        <Header 
          className="site-layout-background" 
          style={{ 
            padding: '0 16px', 
            background: '#fff', 
            display: 'flex', 
            justifyContent: 'flex-end', 
            alignItems: 'center',
            boxShadow: '0 1px 4px rgba(0,21,41,0.08)'
          }}
        >
          <Menu 
            mode="horizontal" 
            selectable={false}
            items={topMenuItems}
            style={{ border: 'none' }}
          />
        </Header>
        <Content style={{ margin: '16px' }}>
          <div 
            className="site-layout-background" 
            style={{ 
              padding: 24, 
              minHeight: 360, 
              background: '#fff',
              borderRadius: '6px',
              boxShadow: '0 1px 2px rgba(0,21,41,0.08)'
            }}
          >
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  );
};

export default MainLayout; 