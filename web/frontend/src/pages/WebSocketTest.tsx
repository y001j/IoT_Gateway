import React, { useState, useEffect } from 'react';
import { Card, Button, Typography, Space, Alert, Badge } from 'antd';
import { webSocketService } from '../services/websocketService';
import { useAuthStore } from '../store/authStore';

const { Title, Text, Paragraph } = Typography;

export const WebSocketTest: React.FC = () => {
  const [connectionState, setConnectionState] = useState('DISCONNECTED');
  const [lastMessage, setLastMessage] = useState<any>(null);
  const [disconnectReason, setDisconnectReason] = useState('');
  const [reconnectInfo, setReconnectInfo] = useState({ attempts: 0, maxAttempts: 0, interval: 0 });
  const [messageCount, setMessageCount] = useState(0);
  const token = useAuthStore((state) => state.accessToken);

  useEffect(() => {
    const updateStatus = () => {
      setConnectionState(webSocketService.getConnectionState());
      setDisconnectReason(webSocketService.getLastDisconnectReason());
      setReconnectInfo(webSocketService.getReconnectInfo());
    };

    // 设置回调
    webSocketService.setCallbacks({
      onConnect: () => {
        console.log('测试页面: WebSocket已连接');
        updateStatus();
      },
      onDisconnect: () => {
        console.log('测试页面: WebSocket已断开');
        updateStatus();
      },
      onError: (error) => {
        console.error('测试页面: WebSocket错误', error);
        updateStatus();
      },
      onMessage: (message) => {
        console.log('测试页面: 收到消息', message);
        setLastMessage(message);
        setMessageCount(prev => prev + 1);
      }
    });

    // 定期更新状态
    const interval = setInterval(updateStatus, 1000);

    return () => clearInterval(interval);
  }, []);

  const handleConnect = async () => {
    if (!token) {
      alert('请先登录获取认证令牌');
      return;
    }
    
    try {
      webSocketService.setToken(token);
      await webSocketService.connect();
    } catch (error) {
      console.error('连接失败:', error);
    }
  };

  const handleDisconnect = () => {
    webSocketService.disconnect();
  };

  const handleReconnect = () => {
    webSocketService.resetReconnectAttempts();
    handleDisconnect();
    setTimeout(handleConnect, 1000);
  };

  const getStatusColor = (state: string) => {
    switch (state) {
      case 'CONNECTED': return 'success';
      case 'CONNECTING': return 'processing';
      case 'DISCONNECTED': return 'default';
      case 'ERROR': return 'error';
      default: return 'warning';
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Title level={2}>WebSocket 连接测试</Title>
      
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Card title="连接状态">
          <Space direction="vertical" size="middle">
            <div>
              <Text strong>状态: </Text>
              <Badge status={getStatusColor(connectionState)} text={connectionState} />
            </div>
            
            <div>
              <Text strong>认证令牌: </Text>
              <Text code>{token ? `${token.substring(0, 20)}...` : '未设置'}</Text>
            </div>
            
            <div>
              <Text strong>最后断开原因: </Text>
              <Text type={disconnectReason === '无断开记录' ? 'secondary' : 'danger'}>
                {disconnectReason}
              </Text>
            </div>
            
            <div>
              <Text strong>重连信息: </Text>
              <Text>
                {reconnectInfo.attempts}/{reconnectInfo.maxAttempts} 次，间隔 {reconnectInfo.interval/1000}秒
              </Text>
            </div>
            
            <div>
              <Text strong>接收消息数: </Text>
              <Badge count={messageCount} showZero />
            </div>
          </Space>
        </Card>

        <Card title="控制操作">
          <Space>
            <Button 
              type="primary" 
              onClick={handleConnect}
              disabled={connectionState === 'CONNECTED' || connectionState === 'CONNECTING'}
            >
              连接
            </Button>
            <Button 
              onClick={handleDisconnect}
              disabled={connectionState === 'DISCONNECTED'}
            >
              断开
            </Button>
            <Button 
              onClick={handleReconnect}
              type="default"
            >
              重连
            </Button>
          </Space>
        </Card>

        {lastMessage && (
          <Card title="最后接收的消息">
            <Paragraph>
              <Text strong>类型: </Text>{lastMessage.type}<br/>
              <Text strong>时间戳: </Text>{lastMessage.timestamp}<br/>
              <Text strong>数据: </Text>
              <pre style={{ background: '#f5f5f5', padding: '8px', marginTop: '8px' }}>
                {JSON.stringify(lastMessage.data, null, 2)}
              </pre>
            </Paragraph>
          </Card>
        )}

        <Alert
          message="调试说明"
          description="这个页面用于调试WebSocket连接问题。请检查浏览器控制台获取详细日志信息。"
          type="info"
          showIcon
        />
      </Space>
    </div>
  );
}; 