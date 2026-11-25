import { useState, useEffect, useRef, useCallback } from 'react';
import { WebSocketService, WebSocketMessage } from '../services/websocketService';
import { authService } from '../services/authService';


import type { EnhancedIoTData, IoTDataPoint, RealtimeAlert, RealtimeSystemMetrics, RealtimeSystemStatus } from '../types/realtime';
// 全局 WebSocket 服务实例（单例模式）
let globalWebSocketService: WebSocketService | null = null;

// 全局数据存储（单例模式）
let globalRealTimeData: RealTimeData = {
  iotData: [],
  systemStatus: null,
  systemMetrics: null,
  systemMetricsHistory: [],
  alerts: [],
  connectionState: 'DISCONNECTED',
  reconnectInfo: { attempts: 0, maxAttempts: 3, interval: 15000 },
  lastDisconnectReason: '',
  connectionInfo: { id: '', state: 'DISCONNECTED', attempts: 0 },
};

// 数据更新监听器
const dataUpdateListeners = new Set<(data: RealTimeData) => void>();

// 更新全局数据并通知所有监听器
const updateGlobalData = (updater: (prevData: RealTimeData) => RealTimeData) => {
  globalRealTimeData = updater(globalRealTimeData);
  dataUpdateListeners.forEach(listener => listener(globalRealTimeData));
};

interface RealTimeData {
  iotData: IoTDataPoint[];
  systemStatus: RealtimeSystemStatus | null;
  systemMetrics: RealtimeSystemMetrics | null;
  systemMetricsHistory: RealtimeSystemMetrics[];
  alerts: RealtimeAlert[];
  connectionState: string;
  reconnectInfo: {
    attempts: number;
    maxAttempts: number;
    interval: number;
  };
  lastDisconnectReason: string;
  connectionInfo: {
    id: string;
    state: string;
    attempts: number;
  };
}

export const useRealTimeData = () => {
  const [data, setData] = useState<RealTimeData>(globalRealTimeData);
  const [isConnected, setIsConnected] = useState(false);
  const dataUpdateTimer = useRef<NodeJS.Timeout | null>(null);
  const mountedRef = useRef(true);
  const lastDataUpdate = useRef<number>(0);

  // 确保组件卸载时清理
  useEffect(() => {
    mountedRef.current = true;
    
    // 添加数据更新监听器
    const listener = (newData: RealTimeData) => {
      if (mountedRef.current) {
        setData(newData);
      }
    };
    dataUpdateListeners.add(listener);
    
    return () => {
      mountedRef.current = false;
      dataUpdateListeners.delete(listener);
    };
  }, []);

  // 获取或创建 WebSocket 服务实例（单例）
  const getWebSocketService = useCallback(() => {
    if (!globalWebSocketService) {
      console.log('🏗️ 创建全局 WebSocket 服务实例');
      globalWebSocketService = new WebSocketService();
    }
    return globalWebSocketService;
  }, []);

  // 处理 WebSocket 消息
  const handleMessage = useCallback((message: WebSocketMessage) => {
    if (!mountedRef.current) return;

    console.log('📨 处理实时数据消息:', message.type, message.data);
    console.log('📨 完整消息对象:', message);

    updateGlobalData(prevData => {
      const newData = { ...prevData };

      switch (message.type) {
        case 'iot_data': {
          // 后端发送的数据格式：{ subject: "...", data: enhancedIoTData }
          // enhancedIoTData 包含: device_id, key, value, data_type, derived_values 等
          const enhancedData = (message.data?.data ?? {}) as EnhancedIoTData;
          if (!enhancedData.device_id || !enhancedData.key) {
            console.warn('⚠️ 数据缺少必要字段:', message.data);
          }

          const iotDataPoint = {
            timestamp: new Date(message.timestamp || Date.now()),
            subject: typeof message.data?.subject === 'string' ? message.data.subject : '',
            data: {
              // 保留原始数据结构
              ...enhancedData,
              device_id: enhancedData.device_id,
              key: enhancedData.key,
              value: enhancedData.value,
              data_type: enhancedData.data_type || 'unknown',
              derived_values: enhancedData.derived_values || {},
            },
            // 为了向后兼容，保留原始消息
            raw_message: message.data
          };

          // 限制 IoT 数据数组大小
          const newIotData = [...prevData.iotData, iotDataPoint];
          if (newIotData.length > 100) {
            newIotData.splice(0, newIotData.length - 100); // 保留最后100条
          }
          newData.iotData = newIotData;
          break;
        }

        case 'system_status':
        case 'system_status_update':
          console.log('📊 更新系统状态:', message.data);
          newData.systemStatus = message.data;
          break;

        case 'system_metrics':
        case 'system_metrics_update': {
          console.log('📈 更新系统指标:', message.data);
          
          // 字段映射：将后端字段名转换为前端期望的字段名
          const raw = message.data as Partial<RealtimeSystemMetrics>;
          const mappedMetrics: RealtimeSystemMetrics = {
            ...raw,
            cpu_percent: raw.cpu_usage || raw.cpu_percent || 0,
            memory_percent: raw.memory_usage || raw.memory_percent || 0,
            disk_percent: raw.disk_usage || raw.disk_percent || 0,
            timestamp: new Date(),
          };
          
          // 将metrics数据存储到systemMetrics字段
          newData.systemMetrics = mappedMetrics;
          
          // 维护历史记录
          const newHistory = [...prevData.systemMetricsHistory, mappedMetrics];
          if (newHistory.length > 50) { // 保留最近50条记录
            newHistory.splice(0, newHistory.length - 50);
          }
          newData.systemMetricsHistory = newHistory;
          
          // 同时为了向后兼容，也合并到systemStatus中
          if (newData.systemStatus) {
            newData.systemStatus = {
              ...newData.systemStatus,
              ...mappedMetrics
            } as RealtimeSystemStatus;
          } else {
            newData.systemStatus = { ...mappedMetrics } as RealtimeSystemStatus;
          }
          break;
        }

        case 'rule_event':
          // 处理规则引擎事件（可能包含告警）
          if (message.data && typeof message.data === 'object') {
            const eventData = message.data as { subject?: string; data?: Record<string, unknown> };
            if (eventData.subject && eventData.subject.includes('alert')) {
              const newAlerts = [...prevData.alerts, {
                id: Date.now(),
                timestamp: new Date().toISOString(),
                ...eventData.data
              }];
              // 限制告警数组大小
              if (newAlerts.length > 50) {
                newAlerts.splice(0, newAlerts.length - 50);
              }
              newData.alerts = newAlerts;
            }
          }
          break;

        case 'alert_created':
          // 处理新创建的告警
          if (message.data && typeof message.data === 'object') {
            const alertData = message.data as { alert_id: string; timestamp: string; alert?: Record<string, unknown> };
            console.log('📢 收到新告警:', alertData.alert);
            
            // 添加到告警列表
            const newAlerts = [...prevData.alerts, {
              id: alertData.alert_id,
              timestamp: alertData.timestamp,
              type: 'alert_created',
              data: alertData.alert
            }];
            
            // 限制告警数组大小
            if (newAlerts.length > 50) {
              newAlerts.splice(0, newAlerts.length - 50);
            }
            newData.alerts = newAlerts;
          }
          break;

        case 'alert_resolved':
          // 处理告警解决
          if (message.data && typeof message.data === 'object') {
            const alertData = message.data as { alert_id: string; timestamp: string; [field: string]: unknown };
            console.log('✅ 告警已解决:', alertData.alert_id);
            
            // 添加到告警列表
            const newAlerts = [...prevData.alerts, {
              id: alertData.alert_id,
              timestamp: alertData.timestamp,
              type: 'alert_resolved',
              data: alertData
            }];
            
            // 限制告警数组大小
            if (newAlerts.length > 50) {
              newAlerts.splice(0, newAlerts.length - 50);
            }
            newData.alerts = newAlerts;
          }
          break;

        case 'system_event':
          // 处理系统事件
          console.log('系统事件:', message.data);
          break;

        default:
          console.log('未处理的消息类型:', message.type);
          break;
      }

      return newData;
    });
  }, []);

  // 更新连接信息
  const updateConnectionInfo = useCallback(() => {
    if (!mountedRef.current) return;
    
    const service = getWebSocketService();
    updateGlobalData(prevData => ({
      ...prevData,
      connectionState: service.getConnectionState(),
      reconnectInfo: service.getReconnectInfo(),
      lastDisconnectReason: service.getLastDisconnectReason(),
      connectionInfo: service.getConnectionInfo(),
    }));
  }, [getWebSocketService]);

  // 设置WebSocket回调
  useEffect(() => {
    const service = getWebSocketService();
    
    // 每次都重新设置回调，确保当前组件能正确接收事件
    service.setCallbacks({
      onConnect: () => {
        console.log('🔗 WebSocket 全局连接回调');
        // 广播给所有挂载的组件
        if (mountedRef.current) {
          setIsConnected(true);
          updateConnectionInfo();
        }
      },
      onDisconnect: () => {
        console.log('🔌 WebSocket 全局断开回调');
        // 广播给所有挂载的组件
        if (mountedRef.current) {
          setIsConnected(false);
          updateConnectionInfo();
        }
      },
      onMessage: (message: WebSocketMessage) => {
        if (!mountedRef.current) return;
        
        // 限制数据更新频率
        const now = Date.now();
        if (now - lastDataUpdate.current < 100) { // 100ms 最小间隔
          return;
        }
        lastDataUpdate.current = now;
        
        handleMessage(message);
      },
      onError: (error: Event) => {
        console.error('🚨 WebSocket 全局错误回调:', error);
        if (mountedRef.current) {
          setIsConnected(false);
          updateConnectionInfo();
        }
      },
    });
  }, [getWebSocketService, handleMessage, updateConnectionInfo]);

  // 初始化 WebSocket 连接
  useEffect(() => {
    const service = getWebSocketService();
    
    // 首先更新当前连接状态
    updateConnectionInfo();
    
    // 同步isConnected状态
    const currentlyConnected = service.isConnected();
    setIsConnected(currentlyConnected);
    
    // 尝试建立连接的函数
    const tryConnect = () => {
      const token = authService.getToken();
      if (token) {
        console.log('🔑 设置 WebSocket 认证令牌');
        service.setToken(token);
        
        // 检查当前连接状态，如果未连接才尝试连接
        if (!service.isConnected()) {
          console.log('📡 WebSocket未连接，尝试建立连接...');
          service.connect().catch((error: unknown) => {
            console.error('❌ WebSocket 连接失败:', error);
          });
        } else {
          console.log('✅ WebSocket已连接，无需重新连接');
          setIsConnected(true);
        }
      } else {
        console.warn('⚠️ 没有找到认证令牌，无法建立 WebSocket 连接');
      }
    };

    // 立即尝试连接
    tryConnect();

    // 监听认证状态变化，当token可用时自动重连
    const checkTokenInterval = setInterval(() => {
      const token = authService.getToken();
      if (token && !service.isConnected()) {
        console.log('🔄 检测到认证令牌已可用，尝试重新连接WebSocket');
        tryConnect();
      }
    }, 2000); // 每2秒检查一次token状态

    // 定期更新连接状态 (每3秒)
    const statusTimer = setInterval(() => {
      if (mountedRef.current) {
        updateConnectionInfo();
        // 同时同步isConnected状态
        const currentlyConnected = service.isConnected();
        setIsConnected(currentlyConnected);
      }
    }, 3000);

    // 清理定时器
    return () => {
      clearInterval(statusTimer);
      clearInterval(checkTokenInterval);
    };
  }, [getWebSocketService, updateConnectionInfo]);

  // 手动重连
  const reconnect = useCallback(() => {
    console.log('🔄 手动重连 WebSocket');
    const service = getWebSocketService();
    
    if (service.isConnected()) {
      console.log('✅ WebSocket 已连接，无需重连');
      return;
    }

    // 重置重连计数器
    service.resetReconnectAttempts();
    
         // 尝试连接
     service.connect().catch((error: unknown) => {
       console.error('❌ 手动重连失败:', error);
     });
  }, [getWebSocketService]);

  // 断开连接
  const disconnect = useCallback(() => {
    console.log('🔌 手动断开 WebSocket');
    const service = getWebSocketService();
    service.disconnect();
  }, [getWebSocketService]);

  // 发送消息
  const sendMessage = useCallback((type: string, data: Record<string, unknown>) => {
    const service = getWebSocketService();
    service.send(type, data);
  }, [getWebSocketService]);

  // 组件卸载时的清理
  useEffect(() => {
    // 复制 ref 对象本身（不是 .current），清理时读取的是最新的定时器
    const timerRef = dataUpdateTimer;
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
      }
      // 注意：我们不在这里断开全局 WebSocket 连接
      // 因为其他组件可能还在使用它
    };
  }, []);

  return {
    data,
    isConnected,
    reconnect,
    disconnect,
    sendMessage,
    connectionState: data.connectionState,
    reconnectInfo: data.reconnectInfo,
    lastDisconnectReason: data.lastDisconnectReason,
    connectionInfo: data.connectionInfo,
  };
};