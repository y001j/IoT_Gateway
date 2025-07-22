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
  Select,
  Typography
} from 'antd';
import {
  SaveOutlined,
  ReloadOutlined,
  FileTextOutlined,
  CloudUploadOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { settingsService } from '../../services/settingsService';
import type { LogConfig } from '../../types/settings';

const { Option } = Select;
const { Title, Text } = Typography;

const LogSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<LogConfig | null>(null);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const response = await settingsService.getLogConfig();
      if (response.success) {
        setConfig(response.data);
        form.setFieldsValue(response.data);
      } else {
        message.error('加载日志配置失败');
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
      
      const response = await settingsService.updateLogConfig(values);
      if (response.success) {
        message.success('日志配置保存成功');
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
        message="日志配置说明"
        description="配置系统日志记录级别、轮转策略和远程日志发送。合理的日志配置有助于系统运维和问题排查。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      <Form
        form={form}
        layout="vertical"
        onValuesChange={() => {}}
      >
        {/* 基础日志配置 */}
        <Card 
          title={
            <Space>
              <FileTextOutlined />
              日志基础配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="日志级别"
                name="level"
                rules={[{ required: true, message: '请选择日志级别' }]}
                tooltip="设置系统日志的记录级别"
              >
                <Select placeholder="选择日志级别">
                  <Option value="debug">Debug</Option>
                  <Option value="info">Info</Option>
                  <Option value="warn">Warn</Option>
                  <Option value="error">Error</Option>
                  <Option value="fatal">Fatal</Option>
                </Select>
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="日志格式"
                name="format"
                rules={[{ required: true, message: '请选择日志格式' }]}
                tooltip="日志输出格式"
              >
                <Select placeholder="选择日志格式">
                  <Option value="json">JSON</Option>
                  <Option value="text">Text</Option>
                  <Option value="console">Console</Option>
                </Select>
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="日志文件路径"
                name="file_path"
                rules={[{ required: true, message: '请输入日志文件路径' }]}
                tooltip="日志文件存储路径"
              >
                <Input placeholder="./logs/gateway.log" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 日志轮转配置 */}
        <Card 
          title={
            <Space>
              <SettingOutlined />
              日志轮转配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用日志轮转"
            name={['rotation', 'enabled']}
            valuePropName="checked"
            tooltip="启用日志文件轮转功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="最大文件大小"
                name={['rotation', 'max_size']}
                rules={[{ required: true, message: '请输入最大文件大小' }]}
                tooltip="单个日志文件的最大大小"
              >
                <Input placeholder="100MB" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="最大文件数量"
                name={['rotation', 'max_files']}
                rules={[
                  { required: true, message: '请输入最大文件数量' },
                  { type: 'number', min: 1, message: '文件数量必须大于0' }
                ]}
                tooltip="保留的日志文件数量"
              >
                <InputNumber 
                  placeholder="10" 
                  style={{ width: '100%' }}
                  min={1}
                  max={100}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="压缩旧文件"
                name={['rotation', 'compress']}
                valuePropName="checked"
                tooltip="是否压缩轮转的日志文件"
              >
                <Switch 
                  checkedChildren="启用" 
                  unCheckedChildren="禁用"
                />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        {/* 远程日志配置 */}
        <Card 
          title={
            <Space>
              <CloudUploadOutlined />
              远程日志配置
            </Space>
          }
          size="small" 
          style={{ marginBottom: 16 }}
        >
          <Form.Item
            label="启用远程日志"
            name={['remote', 'enabled']}
            valuePropName="checked"
            tooltip="启用远程日志发送功能"
          >
            <Switch 
              checkedChildren="启用" 
              unCheckedChildren="禁用"
            />
          </Form.Item>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="远程日志类型"
                name={['remote', 'type']}
                rules={[{ required: true, message: '请选择远程日志类型' }]}
                tooltip="远程日志服务类型"
              >
                <Select placeholder="选择远程日志类型">
                  <Option value="elasticsearch">Elasticsearch</Option>
                  <Option value="syslog">Syslog</Option>
                  <Option value="fluentd">Fluentd</Option>
                  <Option value="loki">Loki</Option>
                </Select>
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="远程服务器地址"
                name={['remote', 'endpoint']}
                rules={[{ required: true, message: '请输入远程服务器地址' }]}
                tooltip="远程日志服务器地址"
              >
                <Input placeholder="http://elasticsearch:9200" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="索引/标签"
                name={['remote', 'index']}
                rules={[{ required: true, message: '请输入索引或标签' }]}
                tooltip="Elasticsearch索引或其他服务的标签"
              >
                <Input placeholder="iot-gateway-logs" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={24}>
            <Col span={8}>
              <Form.Item
                label="批量大小"
                name={['remote', 'batch_size']}
                rules={[
                  { required: true, message: '请输入批量大小' },
                  { type: 'number', min: 1, message: '批量大小必须大于0' }
                ]}
                tooltip="批量发送日志的数量"
              >
                <InputNumber 
                  placeholder="100" 
                  style={{ width: '100%' }}
                  min={1}
                  max={10000}
                />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="发送间隔"
                name={['remote', 'interval']}
                rules={[{ required: true, message: '请输入发送间隔' }]}
                tooltip="日志发送的时间间隔"
              >
                <Input placeholder="10s" />
              </Form.Item>
            </Col>

            <Col span={8}>
              <Form.Item
                label="超时时间"
                name={['remote', 'timeout']}
                rules={[{ required: true, message: '请输入超时时间' }]}
                tooltip="远程日志发送超时时间"
              >
                <Input placeholder="30s" />
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

export default LogSettings;