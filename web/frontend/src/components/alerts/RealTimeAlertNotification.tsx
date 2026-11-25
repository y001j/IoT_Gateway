import React, { useEffect, useState } from 'react';
import { notification, Badge } from 'antd';
import { AlertOutlined, CheckCircleOutlined, ExclamationCircleOutlined, WarningOutlined } from '@ant-design/icons';
import { useRealTimeData } from '../../hooks/useRealTimeData';

import type { RealtimeAlert } from '../../types/realtime';
interface RealTimeAlertNotificationProps {
  enabled?: boolean;
  position?: 'topLeft' | 'topRight' | 'bottomLeft' | 'bottomRight';
}

export const RealTimeAlertNotification: React.FC<RealTimeAlertNotificationProps> = ({
  enabled = true,
  position = 'topRight'
}) => {
  const { data: realTimeData } = useRealTimeData();
  const [processedAlerts, setProcessedAlerts] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!enabled || !realTimeData || !realTimeData.alerts) return;

    // 处理新的告警
    realTimeData.alerts.forEach((alertEvent: RealtimeAlert) => {
      const eventKey = String(alertEvent.id);
      if (processedAlerts.has(eventKey)) return;

      // 标记为已处理
      setProcessedAlerts(prev => new Set(prev).add(eventKey));

      if (alertEvent.type === 'alert_created') {
        const alert = (alertEvent.data ?? {}) as { id?: string; title?: string; description?: string; level?: string; source?: string };
        
        // 根据告警级别选择图标和颜色
        const getAlertIcon = (level: string) => {
          switch (level) {
            case 'critical':
              return <AlertOutlined style={{ color: '#ff4d4f' }} />;
            case 'error':
              return <ExclamationCircleOutlined style={{ color: '#ff7a45' }} />;
            case 'warning':
              return <WarningOutlined style={{ color: '#fa8c16' }} />;
            default:
              return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
          }
        };

        const getLevelText = (level: string) => {
          switch (level) {
            case 'critical': return '严重';
            case 'error': return '错误';
            case 'warning': return '警告';
            case 'info': return '信息';
            default: return level;
          }
        };

        // 显示通知
        notification.open({
          message: `新告警：${alert.title}`,
          description: (
            <div>
              <p>{alert.description}</p>
              <div style={{ marginTop: 8 }}>
                <Badge color={getAlertLevel(alert.level)} text={`级别: ${getLevelText(alert.level)}`} />
                <span style={{ marginLeft: 16 }}>来源: {alert.source}</span>
              </div>
            </div>
          ),
          icon: getAlertIcon(alert.level),
          placement: position,
          duration: alert.level === 'critical' ? 0 : 6, // 严重告警不自动关闭
          key: alert.id,
          onClick: () => {
            // 点击通知跳转到告警页面
            window.location.hash = '#/alerts';
          },
        });
      } else if (alertEvent.type === 'alert_resolved') {
        // 告警解决通知
        notification.success({
          message: '告警已解决',
          description: `告警 ${String(alertEvent.data?.alert_id ?? alertEvent.id)} 已被解决`,
          placement: position,
          duration: 3,
          key: `resolved-${String(alertEvent.data?.alert_id ?? alertEvent.id)}`,
        });
      }
    });
  }, [realTimeData, enabled, position, processedAlerts]);

  // 清理过期的已处理告警ID
  useEffect(() => {
    const cleanup = setInterval(() => {
      setProcessedAlerts(prev => {
        const now = Date.now();
        const filtered = new Set<string>();
        
        // 只保留最近5分钟的告警ID
        prev.forEach(id => {
          const timestamp = parseInt(id.split('-')[0]) || 0;
          if (now - timestamp < 5 * 60 * 1000) {
            filtered.add(id);
          }
        });
        
        return filtered;
      });
    }, 60000); // 每分钟清理一次

    return () => clearInterval(cleanup);
  }, []);

  return null; // 这是一个功能组件，不渲染任何内容
};

// 辅助函数：获取告警级别颜色
const getAlertLevel = (level: string): string => {
  switch (level) {
    case 'critical':
      return '#ff4d4f';
    case 'error':
      return '#ff7a45';
    case 'warning':
      return '#fa8c16';
    case 'info':
      return '#1890ff';
    default:
      return '#d9d9d9';
  }
};

export default RealTimeAlertNotification;