import React, { useState, useEffect } from 'react';
import {
  Form,
  Input,
  InputNumber,
  Button,
  Card,
  Row,
  Col,
  message,
  Alert,
  Space,
  Switch,
  Typography,
  Slider
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  MonitorOutlined,
  DashboardOutlined,
  HeartOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { MonitoringConfig } from '../../types/settings';

const { Title, Text } = Typography;

const MonitoringSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<MonitoringConfig | null>(null);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const response = await settingsService.getMonitoringConfig();
      if (response.success) {
        setConfig(response.data);
        form.setFieldsValue(response.data);
      } else {
        message.error('加载监控配置失败');
      }
    } catch (error: any) {
      message.error('加载配置失败：' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 保存配置
  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      
      const response = await settingsService.updateMonitoringConfig(values);
      if (response.success) {
        message.success('监控配置保存成功');
        setConfig(values);
      } else {
        message.error('保存配置失败：' + (response.message || '未知错误'));
      }
    } catch (error: any) {
      if (error.errorFields) {
        message.error('请检查表单输入');
      } else {
        message.error('保存配置失败：' + (error.message || '未知错误'));
      }
    } finally {
      setLoading(false);
    }
  };

  // 重置配置
  const handleReset = () => {
    if (config) {
      form.setFieldsValue(config);
      message.info('已重置为已保存的配置');
    }
  };

  useEffect(() => {
    loadConfig();
  }, []);

  return (
    <div>
      <Alert
        message="监控配置说明"
        description="配置系统性能监控、指标收集和健康检查。监控数据可用于系统状态分析和告警触发。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      <Form
        form={form}
        layout="vertical"
        onValuesChange={() => {}}
      >
        {/* 指标收集配置 */}
        <Card 
          title={
            <Space>
              <DashboardOutlined />
              指标收集配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用指标收集"
            name={['metrics', 'enabled']}
            valuePropName="checked"
            tooltip="启用系统性能指标收集功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="收集间隔"
                name={['metrics', 'interval']}
                rules={[{ required: true, message: '请输入收集间隔' }]}
                tooltip="系统指标收集的时间间隔"
              >
                <Input placeholder="30s" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="数据保留时间"
                name={['metrics', 'retention']}
                rules={[{ required: true, message: '请输入数据保留时间' }]}
                tooltip="监控数据的保留时间"
              >
                <Input placeholder="7d" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="指标接口端点"
                name={['metrics', 'endpoint']}
                rules={[{ required: true, message: '请输入指标接口端点' }]}
                tooltip="Prometheus指标暴露端点"
              >
                <Input placeholder="/metrics" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 性能阈值配置 */}
        <Card 
          title={
            <Space>
              <MonitorOutlined />
              性能阈值配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="CPU使用率阈值 (%)"
                name={['performance', 'cpu_threshold']}
                rules={[
                  { required: true, message: '请设置CPU阈值' },
                  { type: 'number', min: 1, max: 100, message: '阈值范围1-100' }
                ]}
                tooltip="CPU使用率超过此值时触发告警"
              >
                <Slider
                  min={1}
                  max={100}
                  marks={{
                    50: '50%',
                    80: '80%',
                    90: '90%'
                  }}
                />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="内存使用率阈值 (%)"
                name={['performance', 'memory_threshold']}
                rules={[
                  { required: true, message: '请设置内存阈值' },
                  { type: 'number', min: 1, max: 100, message: '阈值范围1-100' }
                ]}
                tooltip="内存使用率超过此值时触发告警"
              >
                <Slider
                  min={1}
                  max={100}
                  marks={{
                    70: '70%',
                    85: '85%',
                    95: '95%'
                  }}
                />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="磁盘使用率阈值 (%)"
                name={['performance', 'disk_threshold']}
                rules={[
                  { required: true, message: '请设置磁盘阈值' },
                  { type: 'number', min: 1, max: 100, message: '阈值范围1-100' }
                ]}
                tooltip="磁盘使用率超过此值时触发告警"
              >
                <Slider
                  min={1}
                  max={100}
                  marks={{
                    80: '80%',
                    90: '90%',
                    95: '95%'
                  }}
                />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="连接数阈值"
                name={['performance', 'connection_threshold']}
                rules={[
                  { required: true, message: '请设置连接数阈值' },
                  { type: 'number', min: 1, message: '连接数必须大于0' }
                ]}
                tooltip="活跃连接数超过此值时触发告警"
              >
                <InputNumber 
                  placeholder="800" 
                  style={{ width: '100%' }}
                  min={1}
                  max={100000}
                />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 健康检查配置 */}
        <Card 
          title={
            <Space>
              <HeartOutlined />
              健康检查配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用健康检查"
            name={['health_check', 'enabled']}
            valuePropName="checked"
            tooltip="启用系统健康状态检查"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={12}>
              <Form.Item
                label="检查间隔"
                name={['health_check', 'interval']}
                rules={[{ required: true, message: '请输入检查间隔' }]}
                tooltip="健康检查的执行间隔"
              >
                <Input placeholder="60s" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                label="检查超时"
                name={['health_check', 'timeout']}
                rules={[{ required: true, message: '请输入检查超时时间' }]}
                tooltip="单次健康检查的超时时间"
              >
                <Input placeholder="10s" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 操作按钮 */}
        <Card size="small">
          <Space>
            <Button 
              type="primary" 
              icon={<SaveOutlined />}
              loading={loading}
              onClick={handleSave}
            >
              保存配置
            </Button>
            
            <Button 
              icon={<ReloadOutlined />}
              onClick={handleReset}
            >
              重置
            </Button>
            
            <Button 
              icon={<ReloadOutlined />}
              onClick={loadConfig}
              loading={loading}
            >
              重新加载
            </Button>
          </Space>
        </Card>
      </Form>
    </div>
  );
};

export default MonitoringSettings;