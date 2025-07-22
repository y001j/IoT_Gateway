import React, { useState } from 'react';
import { Button, Card, Space, Typography } from 'antd';
import { alertService } from '../../services/alertService';

const { Text, Paragraph } = Typography;

export const APITestComponent: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<any>(null);
  const [error, setError] = useState<string | null>(null);

  const testAPI = async () => {
    setLoading(true);
    setResult(null);
    setError(null);
    
    try {
      console.log('🧪 开始API测试...');
      const startTime = Date.now();
      
      const alerts = await alertService.getAlerts({
        page: 1,
        pageSize: 20
      });
      
      const duration = Date.now() - startTime;
      console.log('🧪 API测试完成，耗时:', duration, 'ms');
      
      setResult({
        duration,
        data: alerts,
        success: true
      });
    } catch (err: any) {
      console.error('🧪 API测试失败:', err);
      setError(err.message || '未知错误');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card title="API连接测试" style={{ margin: '16px' }}>
      <Space direction="vertical" style={{ width: '100%' }}>
        <Button 
          type="primary" 
          onClick={testAPI} 
          loading={loading}
        >
          测试告警API
        </Button>
        
        {result && (
          <div>
            <Text type="success">✅ API调用成功</Text>
            <Paragraph>
              <Text strong>耗时: </Text>{result.duration}ms<br/>
              <Text strong>数据: </Text>
              <pre style={{ fontSize: '12px', backgroundColor: '#f5f5f5', padding: '8px' }}>
                {JSON.stringify(result.data, null, 2)}
              </pre>
            </Paragraph>
          </div>
        )}
        
        {error && (
          <div>
            <Text type="danger">❌ API调用失败</Text>
            <Paragraph>
              <Text code>{error}</Text>
            </Paragraph>
          </div>
        )}
      </Space>
    </Card>
  );
};