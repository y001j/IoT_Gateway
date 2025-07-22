import React from 'react';
import { Card, Row, Col, Statistic, Progress, Badge, Space } from 'antd';
import { AlertOutlined, CheckCircleOutlined, ExclamationCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import type { AlertStats } from '../../types/alert';

interface AlertStatsDashboardProps {
  stats: AlertStats | null;
  loading?: boolean;
}

export const AlertStatsDashboard: React.FC<AlertStatsDashboardProps> = ({ stats, loading = false }) => {
  if (!stats) return null;

  const activePercentage = stats.total > 0 ? (stats.active / stats.total) * 100 : 0;
  const resolvedPercentage = stats.total > 0 ? (stats.resolved / stats.total) * 100 : 0;
  
  // 安全地访问byLevel属性
  const byLevel = stats.byLevel || {};
  const bySource = stats.bySource || {};

  return (
    <Card title="告警统计" style={{ marginBottom: 16 }} loading={loading}>
      <Row gutter={16}>
        {/* 总览统计 */}
        <Col span={6}>
          <Statistic
            title="总告警数"
            value={stats.total}
            prefix={<AlertOutlined />}
            valueStyle={{ color: '#1890ff' }}
          />
        </Col>
        <Col span={6}>
          <Statistic
            title="活跃告警"
            value={stats.active}
            prefix={<ExclamationCircleOutlined />}
            valueStyle={{ color: '#ff4d4f' }}
            suffix={
              <Progress 
                percent={activePercentage} 
                size="small" 
                strokeColor="#ff4d4f"
                showInfo={false}
                style={{ marginLeft: 8, width: 60 }}
              />
            }
          />
        </Col>
        <Col span={6}>
          <Statistic
            title="已确认"
            value={stats.acknowledged}
            prefix={<CheckCircleOutlined />}
            valueStyle={{ color: '#fa8c16' }}
          />
        </Col>
        <Col span={6}>
          <Statistic
            title="已解决"
            value={stats.resolved}
            prefix={<CloseCircleOutlined />}
            valueStyle={{ color: '#52c41a' }}
            suffix={
              <Progress 
                percent={resolvedPercentage} 
                size="small" 
                strokeColor="#52c41a"
                showInfo={false}
                style={{ marginLeft: 8, width: 60 }}
              />
            }
          />
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        {/* 按级别统计 */}
        <Col span={12}>
          <Card size="small" title="按级别统计" style={{ height: 200 }}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Badge color="#ff4d4f" text="严重" />
                <span>{byLevel.critical || 0}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Badge color="#fa8c16" text="错误" />
                <span>{byLevel.error || 0}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Badge color="#faad14" text="警告" />
                <span>{byLevel.warning || 0}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Badge color="#1890ff" text="信息" />
                <span>{byLevel.info || 0}</span>
              </div>
            </Space>
          </Card>
        </Col>

        {/* 按来源统计 */}
        <Col span={12}>
          <Card size="small" title="按来源统计" style={{ height: 200 }}>
            <Space direction="vertical" style={{ width: '100%' }}>
              {Object.entries(bySource).map(([source, count]) => (
                <div key={source} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span>{source}</span>
                  <Badge count={count} showZero color="#1890ff" />
                </div>
              ))}
              {Object.keys(bySource).length === 0 && (
                <div style={{ textAlign: 'center', color: '#999' }}>暂无数据</div>
              )}
            </Space>
          </Card>
        </Col>
      </Row>
    </Card>
  );
};

export default AlertStatsDashboard;