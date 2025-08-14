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
        width={180}
        style={{
          background: '#0f3820 !important',
          backgroundColor: '#0f3820 !important',
          boxShadow: '4px 0 20px rgba(20, 83, 45, 0.5)',
          position: 'relative',
          overflow: 'hidden'
        }}
      >
        
        <div 
          className="logo" 
          style={{ 
            height: collapsed ? '40px' : '50px',
            margin: collapsed ? '12px 16px' : '12px 10px',
            padding: collapsed ? '8px' : '8px',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            background: 'rgba(255, 255, 255, 0.1)',
            border: '1px solid rgba(255, 255, 255, 0.2)',
            borderRadius: '8px',
            boxShadow: '0 2px 8px rgba(0, 0, 0, 0.15)',
            transition: 'all 0.3s ease',
            position: 'relative',
            zIndex: 1
          }} 
        >
          {/* 中集安瑞科官方标志 */}
          <img
            src="/logo.png"
            alt="CIMC ENRIC"
            width={collapsed ? 78 : 108}
            height={collapsed ? 78 : 108}
            style={{
              flexShrink: 0,
              objectFit: 'contain',
              filter: 'brightness(1.1) contrast(1.2) drop-shadow(0 2px 4px rgba(0,0,0,0.3))' // 适配红色侧边栏
            }}
          />
          
        </div>
        <Menu 
          theme="dark" 
          selectedKeys={getSelectedKeys()}
          mode="inline"
          items={sideMenuItems}
          style={{
            background: 'transparent',
            border: 'none',
            position: 'relative',
            zIndex: 1
          }}
          className="custom-sidebar-menu"
        />
        
        <style jsx global>{`
          .custom-sidebar-menu .ant-menu-item {
            background: transparent !important;
            border-radius: 8px !important;
            margin: 4px 8px !important;
            transition: all 0.3s ease !important;
          }
          
          .custom-sidebar-menu .ant-menu-item:hover {
            background: rgba(255, 255, 255, 0.15) !important;
            border-radius: 6px !important;
            transform: translateX(2px) !important;
          }
          
          .custom-sidebar-menu .ant-menu-item-selected {
            background: rgba(255, 255, 255, 0.25) !important;
            border-radius: 6px !important;
            transform: translateX(4px) !important;
          }
          
          .custom-sidebar-menu .ant-menu-item-selected::after {
            display: none !important;
          }
          
          .custom-sidebar-menu .ant-menu-item .anticon {
            color: #bbf7d0 !important;
          }
          
          .custom-sidebar-menu .ant-menu-item-selected .anticon {
            color: #ffffff !important;
          }
          
          .custom-sidebar-menu .ant-menu-item:hover .anticon {
            color: #ffffff !important;
          }
          
          /* 菜单项文字颜色 */
          .custom-sidebar-menu .ant-menu-item,
          .custom-sidebar-menu .ant-menu-item a {
            color: #f1f5f9 !important;
            font-weight: 500 !important;
          }
          
          .custom-sidebar-menu .ant-menu-item-selected,
          .custom-sidebar-menu .ant-menu-item-selected a {
            color: #ffffff !important;
            font-weight: 600 !important;
          }
          
          .custom-sidebar-menu .ant-menu-item:hover,
          .custom-sidebar-menu .ant-menu-item:hover a {
            color: #ffffff !important;
            font-weight: 500 !important;
          }
          
          /* 顶部菜单样式 */
          .modern-top-menu .ant-menu-item {
            border-radius: 8px !important;
            margin: 0 4px !important;
            transition: all 0.3s ease !important;
          }
          
          .modern-top-menu .ant-menu-item:hover {
            background: linear-gradient(135deg, rgba(22, 163, 74, 0.08) 0%, rgba(187, 247, 208, 0.12) 100%) !important;
            border-radius: 8px !important;
            transform: translateY(-1px) !important;
            box-shadow: 0 4px 12px rgba(22, 163, 74, 0.15) !important;
          }
          
          .modern-top-menu .ant-menu-item .anticon {
            color: #64748b !important;
            transition: all 0.3s ease !important;
          }
          
          .modern-top-menu .ant-menu-item:hover .anticon {
            color: #16a34a !important;
          }
          
          /* 滚动条美化 */
          ::-webkit-scrollbar {
            width: 6px;
            height: 6px;
          }
          
          ::-webkit-scrollbar-track {
            background: rgba(226, 232, 240, 0.3);
            border-radius: 3px;
          }
          
          ::-webkit-scrollbar-thumb {
            background: linear-gradient(135deg, rgba(59, 130, 246, 0.4) 0%, rgba(168, 85, 247, 0.3) 100%);
            border-radius: 3px;
          }
          
          ::-webkit-scrollbar-thumb:hover {
            background: linear-gradient(135deg, rgba(59, 130, 246, 0.6) 0%, rgba(168, 85, 247, 0.5) 100%);
          }
          
          /* 全局紧凑样式 */
          .ant-table {
            font-size: 13px !important;
          }
          
          .ant-table-thead > tr > th,
          .ant-table-tbody > tr > td {
            padding: 8px 12px !important;
          }
          
          .ant-form-item {
            margin-bottom: 16px !important;
          }
          
          .ant-card {
            margin-bottom: 12px !important;
          }
          
          .ant-card .ant-card-head {
            padding: 0 16px !important;
            min-height: 44px !important;
          }
          
          .ant-card .ant-card-body {
            padding: 16px !important;
          }
          
          .ant-btn {
            height: 32px !important;
            padding: 4px 12px !important;
            font-size: 13px !important;
          }
          
          .ant-btn-sm {
            height: 28px !important;
            padding: 2px 8px !important;
            font-size: 12px !important;
          }
          
          .ant-input {
            padding: 4px 8px !important;
            font-size: 13px !important;
          }
          
          .ant-select {
            font-size: 13px !important;
          }
          
          .ant-select-single:not(.ant-select-customize-input) .ant-select-selector {
            height: 32px !important;
            padding: 0 8px !important;
          }
          
          .ant-modal-header {
            padding: 16px 20px 12px !important;
          }
          
          .ant-modal-body {
            padding: 16px 20px !important;
          }
          
          .ant-modal-footer {
            padding: 12px 20px 16px !important;
          }
          
          .ant-tabs-tab {
            padding: 8px 12px !important;
            font-size: 13px !important;
          }
          
          .ant-space {
            gap: 8px !important;
          }
          
          /* 页面内容区域优化 */
          .modern-content-card > div:first-child {
            height: 100% !important;
            display: flex !important;
            flex-direction: column !important;
          }
          
          /* 表格容器优化 */
          .ant-table-wrapper {
            margin: 0 !important;
          }
          
          .ant-table-container {
            border-radius: 8px !important;
          }
          
          /* 强制覆盖 Ant Design 默认 Sider 背景 */
          .ant-layout-sider,
          .ant-layout-sider-dark {
            background: #0f3820 !important;
            backgroundColor: #0f3820 !important;
          }
          
          .ant-menu-dark,
          .ant-menu-dark .ant-menu-sub {
            background: transparent !important;
          }
          
          .ant-layout-sider-trigger {
            background: rgba(255, 255, 255, 0.1) !important;
            border-top: 1px solid rgba(255, 255, 255, 0.2) !important;
            color: #f8fafc !important;
            font-weight: 500 !important;
          }
          
          .ant-layout-sider-trigger:hover {
            background: rgba(255, 255, 255, 0.2) !important;
            color: #ffffff !important;
            font-weight: 500 !important;
          }
        `}</style>
      </Sider>
      <Layout className="site-layout" style={{ 
        background: 'linear-gradient(135deg, #f0fdf4 0%, #bbf7d0 30%, #f1f5f9 70%, #e2e8f0 100%)',
        position: 'relative',
        overflow: 'hidden'
      }}>
        {/* 主界面背景装饰 */}
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: `
            radial-gradient(circle at 85% 15%, rgba(22, 163, 74, 0.06) 0%, transparent 40%),
            radial-gradient(circle at 15% 85%, rgba(21, 128, 61, 0.04) 0%, transparent 40%),
            radial-gradient(circle at 60% 40%, rgba(20, 83, 45, 0.03) 0%, transparent 30%),
            radial-gradient(circle at 30% 70%, rgba(187, 247, 208, 0.05) 0%, transparent 35%)
          `,
          pointerEvents: 'none',
          zIndex: 0
        }} />
        
        <Header 
          className="site-layout-background" 
          style={{ 
            padding: '0 24px', 
            background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.85) 0%, rgba(240, 253, 244, 0.9) 30%, rgba(248, 250, 252, 0.8) 100%)',
            backdropFilter: 'blur(12px)',
            display: 'flex', 
            justifyContent: 'space-between', 
            alignItems: 'center',
            boxShadow: '0 4px 20px rgba(0, 0, 0, 0.08), 0 1px 0 rgba(255, 255, 255, 0.4)',
            border: '1px solid rgba(255, 255, 255, 0.2)',
            borderBottom: '1px solid rgba(226, 232, 240, 0.6)',
            position: 'relative',
            zIndex: 2
          }}
        >
          {/* 左侧公司标志 */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>

            
            {/* 应用标题 */}
            <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.2 }}>
              <span style={{ 
                fontSize: '14px', 
                fontWeight: '500', 
                color: '#333'
              }}>
                Maien IoT Gateway
              </span>
              <span style={{ 
                fontSize: '11px', 
                color: '#999'
              }}>
                麦恩物联网管理平台
              </span>
            </div>
          </div>

          {/* 右侧菜单 */}
          <Menu 
            mode="horizontal" 
            selectable={false}
            items={topMenuItems}
            style={{ 
              border: 'none',
              background: 'transparent',
              boxShadow: 'none'
            }}
            className="modern-top-menu"
          />
        </Header>
        <Content style={{ 
          margin: '12px', 
          position: 'relative',
          zIndex: 1,
          display: 'flex',
          flexDirection: 'column'
        }}>
          <div 
            className="site-layout-background modern-content-card" 
            style={{ 
              padding: '16px 20px', 
              height: 'calc(100vh - 88px)', 
              background: 'linear-gradient(145deg, rgba(255, 255, 255, 0.92) 0%, rgba(240, 253, 244, 0.9) 40%, rgba(248, 250, 252, 0.88) 100%)',
              backdropFilter: 'blur(16px)',
              borderRadius: '12px',
              boxShadow: `
                0 6px 24px rgba(0, 0, 0, 0.05),
                0 1px 4px rgba(0, 0, 0, 0.03),
                inset 0 1px 0 rgba(255, 255, 255, 0.8)
              `,
              border: '1px solid rgba(255, 255, 255, 0.3)',
              position: 'relative',
              overflow: 'hidden',
              display: 'flex',
              flexDirection: 'column'
            }}
          >
            {/* 内容区域装饰效果 */}
            <div style={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              background: `
                radial-gradient(circle at 90% 10%, rgba(22, 163, 74, 0.02) 0%, transparent 30%),
                radial-gradient(circle at 10% 90%, rgba(21, 128, 61, 0.015) 0%, transparent 30%),
                radial-gradient(circle at 50% 50%, rgba(187, 247, 208, 0.025) 0%, transparent 25%)
              `,
              pointerEvents: 'none',
              zIndex: 0
            }} />
            
            {/* 内容区域 */}
            <div style={{ 
              position: 'relative', 
              zIndex: 1, 
              flex: 1,
              overflow: 'auto'
            }}>
              <Outlet />
            </div>
          </div>
        </Content>
      </Layout>
    </Layout>
  );
};

export default MainLayout; 