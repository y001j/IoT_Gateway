import React, { useState } from 'react';
import {
  Layout,
  Menu,
  Card,
  Typography,
  Spin
} from 'antd';
import {
  SettingOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  SafetyOutlined,
  MonitorOutlined,
  BellOutlined,
  FileTextOutlined,
  UserOutlined,
  TeamOutlined,
  KeyOutlined,
  AuditOutlined
} from '@ant-design/icons';

// 导入设置组件
import GatewaySettings from '../components/settings/GatewaySettings';
import NatsSettings from '../components/settings/NatsSettings';
import DatabaseSettings from '../components/settings/DatabaseSettings';
import SecuritySettings from '../components/settings/SecuritySettings';
import MonitoringSettings from '../components/settings/MonitoringSettings';
import AlertSettings from '../components/settings/AlertSettings';
import LogSettings from '../components/settings/LogSettings';
import UserManagement from '../components/settings/UserManagement';
import RoleManagement from '../components/settings/RoleManagement';
import ApiPermissions from '../components/settings/ApiPermissions';
import AuditSettings from '../components/settings/AuditSettings';

const { Title } = Typography;
const { Sider, Content } = Layout;

type SettingsTab = 
  | 'gateway'
  | 'nats' 
  | 'database'
  | 'security'
  | 'monitoring'
  | 'alerts'
  | 'logs'
  | 'users'
  | 'roles'
  | 'api-permissions'
  | 'audit';

const SettingsPage: React.FC = () => {
  const [activeTab, setActiveTab] = useState<SettingsTab>('gateway');
  const [, setLoading] = useState(false);

  const menuItems = [
    {
      key: 'core',
      label: '核心配置',
      type: 'group' as const,
      children: [
        {
          key: 'gateway',
          icon: <SettingOutlined />,
          label: '网关配置'
        },
        {
          key: 'nats',
          icon: <CloudServerOutlined />,
          label: 'NATS配置'
        },
        {
          key: 'database',
          icon: <DatabaseOutlined />,
          label: '数据库配置'
        },
        {
          key: 'security',
          icon: <SafetyOutlined />,
          label: '安全配置'
        }
      ]
    },
    {
      key: 'monitoring',
      label: '监控告警',
      type: 'group' as const,
      children: [
        {
          key: 'monitoring',
          icon: <MonitorOutlined />,
          label: '系统监控'
        },
        {
          key: 'alerts',
          icon: <BellOutlined />,
          label: '告警配置'
        },
        {
          key: 'logs',
          icon: <FileTextOutlined />,
          label: '日志配置'
        }
      ]
    },
    {
      key: 'access',
      label: '用户权限',
      type: 'group' as const,
      children: [
        {
          key: 'users',
          icon: <UserOutlined />,
          label: '用户管理'
        },
        {
          key: 'roles',
          icon: <TeamOutlined />,
          label: '角色管理'
        },
        {
          key: 'api-permissions',
          icon: <KeyOutlined />,
          label: 'API权限'
        },
        {
          key: 'audit',
          icon: <AuditOutlined />,
          label: '审计日志'
        }
      ]
    }
  ];

  const renderContent = () => {
    switch (activeTab) {
      case 'gateway':
        return <GatewaySettings />;
      case 'nats':
        return <NatsSettings />;
      case 'database':
        return <DatabaseSettings />;
      case 'security':
        return <SecuritySettings />;
      case 'monitoring':
        return <MonitoringSettings />;
      case 'alerts':
        return <AlertSettings />;
      case 'logs':
        return <LogSettings />;
      case 'users':
        return <UserManagement />;
      case 'roles':
        return <RoleManagement />;
      case 'api-permissions':
        return <ApiPermissions />;
      case 'audit':
        return <AuditSettings />;
      default:
        return <div>选择一个配置项</div>;
    }
  };

  const getPageTitle = () => {
    const item = menuItems
      .flatMap(group => group.children || [])
      .find(item => item.key === activeTab);
    return item?.label || '系统设置';
  };

  return (
    <div>
      <Title level={2}>系统设置</Title>
      
      <Layout style={{ minHeight: '70vh', background: '#fff' }}>
        <Sider width={250} style={{ background: '#fafafa' }}>
          <Menu
            mode="inline"
            selectedKeys={[activeTab]}
            items={menuItems}
            onClick={({ key }) => setActiveTab(key as SettingsTab)}
            style={{ border: 'none', background: 'transparent' }}
          />
        </Sider>
        
        <Content style={{ padding: '24px', background: '#fff' }}>
          <Card 
            title={getPageTitle()}
            style={{ minHeight: '60vh' }}
            styles={{ body: { padding: '24px' } }}
          >
            <Spin spinning={loading}>
              {renderContent()}
            </Spin>
          </Card>
        </Content>
      </Layout>
    </div>
  );
};

export default SettingsPage;