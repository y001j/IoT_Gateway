import React from 'react';
import {
  Form,
  Select,
  InputNumber,
  Input,
  Space,
  Card,
  Row,
  Col,
  Typography,
  Tooltip,
  Switch,
  DatePicker,
  TimePicker
} from 'antd';
import { 
  LineChartOutlined,
  BarChartOutlined,
  FunctionOutlined,
  AreaChartOutlined,
  RiseOutlined,
  ClockCircleOutlined,
  FilterOutlined,
  BorderOutlined
} from '@ant-design/icons';

const { Text, Title } = Typography;
const { Option } = Select;
const { RangePicker } = DatePicker;

interface TimeSeriesActionConfig {
  sub_type: 'trend_analysis' | 'seasonal_decompose' | 'moving_average' | 'diff' | 'resample' | 'anomaly_detection' | 'forecast' | 'correlation';
  window_size?: number;
  window_type?: 'fixed' | 'sliding' | 'expanding';
  trend_method?: 'linear' | 'polynomial' | 'exponential' | 'seasonal';
  seasonal_period?: number;
  decompose_model?: 'additive' | 'multiplicative';
  diff_order?: number;
  diff_seasonal?: boolean;
  resample_frequency?: 'minute' | 'hour' | 'day' | 'week' | 'month';
  resample_method?: 'mean' | 'sum' | 'max' | 'min' | 'first' | 'last';
  anomaly_method?: 'zscore' | 'iqr' | 'isolation_forest' | 'local_outlier';
  anomaly_threshold?: number;
  forecast_steps?: number;
  forecast_method?: 'arima' | 'exponential_smoothing' | 'linear' | 'seasonal_naive';
  correlation_lag?: number;
  output_key?: string;
}

interface TimeSeriesActionEditorProps {
  value?: TimeSeriesActionConfig;
  onChange?: (config: TimeSeriesActionConfig) => void;
}

export const TimeSeriesActionEditor: React.FC<TimeSeriesActionEditorProps> = ({
  value = {
    sub_type: 'trend_analysis',
    output_key: 'timeseries_result'
  },
  onChange
}) => {
  const handleChange = (field: keyof TimeSeriesActionConfig, newValue: any) => {
    if (onChange) {
      onChange({
        ...value,
        [field]: newValue
      });
    }
  };

  const renderActionSpecificConfig = () => {
    switch (value.sub_type) {
      case 'trend_analysis':
        return (
          <Card size="small" title={<><RiseOutlined /> 趋势分析</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>趋势分析方法:</Text>
                <Select
                  value={value.trend_method || 'linear'}
                  onChange={(method) => handleChange('trend_method', method)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="linear">线性趋势</Option>
                  <Option value="polynomial">多项式趋势</Option>
                  <Option value="exponential">指数趋势</Option>
                  <Option value="seasonal">季节性趋势</Option>
                </Select>
              </Col>
              <Col span={12}>
                <Text strong>分析窗口大小:</Text>
                <InputNumber
                  value={value.window_size || 10}
                  onChange={(val) => handleChange('window_size', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={3}
                  placeholder="10"
                />
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              分析时间序列数据的趋势方向和强度，输出趋势系数和预测值
            </Text>
          </Card>
        );

      case 'seasonal_decompose':
        return (
          <Card size="small" title={<><AreaChartOutlined /> 季节性分解</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>季节周期:</Text>
                <InputNumber
                  value={value.seasonal_period || 12}
                  onChange={(val) => handleChange('seasonal_period', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={2}
                  placeholder="12"
                  addonAfter="点"
                />
              </Col>
              <Col span={12}>
                <Text strong>分解模型:</Text>
                <Select
                  value={value.decompose_model || 'additive'}
                  onChange={(model) => handleChange('decompose_model', model)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="additive">加法模型</Option>
                  <Option value="multiplicative">乘法模型</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              将时间序列分解为趋势、季节性和随机成分
            </Text>
          </Card>
        );

      case 'moving_average':
        return (
          <Card size="small" title={<><LineChartOutlined /> 移动平均</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>窗口大小:</Text>
                <InputNumber
                  value={value.window_size || 5}
                  onChange={(val) => handleChange('window_size', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={2}
                  placeholder="5"
                />
              </Col>
              <Col span={12}>
                <Text strong>窗口类型:</Text>
                <Select
                  value={value.window_type || 'sliding'}
                  onChange={(type) => handleChange('window_type', type)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="fixed">固定窗口</Option>
                  <Option value="sliding">滑动窗口</Option>
                  <Option value="expanding">扩展窗口</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              计算时间序列的移动平均值，平滑短期波动
            </Text>
          </Card>
        );

      case 'diff':
        return (
          <Card size="small" title={<><FunctionOutlined /> 差分运算</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>差分阶数:</Text>
                <InputNumber
                  value={value.diff_order || 1}
                  onChange={(val) => handleChange('diff_order', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={1}
                  max={3}
                  placeholder="1"
                />
              </Col>
              <Col span={12}>
                <Text strong>季节性差分:</Text>
                <Switch
                  checked={value.diff_seasonal || false}
                  onChange={(checked) => handleChange('diff_seasonal', checked)}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                />
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              计算时间序列的差分，消除趋势和季节性影响
            </Text>
          </Card>
        );

      case 'resample':
        return (
          <Card size="small" title={<><ClockCircleOutlined /> 重采样</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>重采样频率:</Text>
                <Select
                  value={value.resample_frequency || 'hour'}
                  onChange={(freq) => handleChange('resample_frequency', freq)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="minute">分钟</Option>
                  <Option value="hour">小时</Option>
                  <Option value="day">天</Option>
                  <Option value="week">周</Option>
                  <Option value="month">月</Option>
                </Select>
              </Col>
              <Col span={12}>
                <Text strong>聚合方法:</Text>
                <Select
                  value={value.resample_method || 'mean'}
                  onChange={(method) => handleChange('resample_method', method)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="mean">平均值</Option>
                  <Option value="sum">求和</Option>
                  <Option value="max">最大值</Option>
                  <Option value="min">最小值</Option>
                  <Option value="first">首个值</Option>
                  <Option value="last">末个值</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              改变时间序列的采样频率，上采样或下采样数据
            </Text>
          </Card>
        );

      case 'anomaly_detection':
        return (
          <Card size="small" title={<><FilterOutlined /> 异常检测</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>检测方法:</Text>
                <Select
                  value={value.anomaly_method || 'zscore'}
                  onChange={(method) => handleChange('anomaly_method', method)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="zscore">Z-Score方法</Option>
                  <Option value="iqr">四分位距方法</Option>
                  <Option value="isolation_forest">孤立森林</Option>
                  <Option value="local_outlier">局部异常因子</Option>
                </Select>
              </Col>
              <Col span={12}>
                <Text strong>异常阈值:</Text>
                <InputNumber
                  value={value.anomaly_threshold || 2.5}
                  onChange={(val) => handleChange('anomaly_threshold', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={1}
                  max={5}
                  step={0.1}
                  precision={1}
                  placeholder="2.5"
                />
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              检测时间序列中的异常点或异常模式
            </Text>
          </Card>
        );

      case 'forecast':
        return (
          <Card size="small" title={<><RiseOutlined /> 时间序列预测</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>预测方法:</Text>
                <Select
                  value={value.forecast_method || 'linear'}
                  onChange={(method) => handleChange('forecast_method', method)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="arima">ARIMA模型</Option>
                  <Option value="exponential_smoothing">指数平滑</Option>
                  <Option value="linear">线性预测</Option>
                  <Option value="seasonal_naive">季节朴素法</Option>
                </Select>
              </Col>
              <Col span={12}>
                <Text strong>预测步数:</Text>
                <InputNumber
                  value={value.forecast_steps || 5}
                  onChange={(val) => handleChange('forecast_steps', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={1}
                  max={100}
                  placeholder="5"
                  addonAfter="步"
                />
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              基于历史数据预测未来时间点的数值
            </Text>
          </Card>
        );

      case 'correlation':
        return (
          <Card size="small" title={<><BarChartOutlined /> 相关性分析</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>滞后期数:</Text>
                <Tooltip title="分析时间序列与其滞后版本的相关性">
                  <InputNumber
                    value={value.correlation_lag || 1}
                    onChange={(val) => handleChange('correlation_lag', val)}
                    style={{ width: '100%', marginTop: 4 }}
                    min={0}
                    max={50}
                    placeholder="1"
                    addonAfter="期"
                  />
                </Tooltip>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              计算时间序列的自相关或与其他序列的互相关
            </Text>
          </Card>
        );

      default:
        return null;
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Card size="small" title={<><BorderOutlined /> 时间序列操作配置</>}>
        <Row gutter={[16, 8]}>
          <Col span={24}>
            <Text strong>时序操作类型:</Text>
            <Select
              value={value.sub_type}
              onChange={(subType) => handleChange('sub_type', subType)}
              style={{ width: '100%', marginTop: 4 }}
            >
              <Option value="trend_analysis">
                <Space>
                  <RiseOutlined />
                  趋势分析
                </Space>
              </Option>
              <Option value="seasonal_decompose">
                <Space>
                  <AreaChartOutlined />
                  季节性分解
                </Space>
              </Option>
              <Option value="moving_average">
                <Space>
                  <LineChartOutlined />
                  移动平均
                </Space>
              </Option>
              <Option value="diff">
                <Space>
                  <FunctionOutlined />
                  差分运算
                </Space>
              </Option>
              <Option value="resample">
                <Space>
                  <ClockCircleOutlined />
                  重采样
                </Space>
              </Option>
              <Option value="anomaly_detection">
                <Space>
                  <FilterOutlined />
                  异常检测
                </Space>
              </Option>
              <Option value="forecast">
                <Space>
                  <RiseOutlined />
                  时序预测
                </Space>
              </Option>
              <Option value="correlation">
                <Space>
                  <BarChartOutlined />
                  相关性分析
                </Space>
              </Option>
            </Select>
          </Col>
        </Row>
      </Card>

      {renderActionSpecificConfig()}

      <Card size="small" title="输出配置">
        <Row gutter={[16, 8]}>
          <Col span={24}>
            <Text strong>输出字段名:</Text>
            <Input
              value={value.output_key}
              onChange={(e) => handleChange('output_key', e.target.value)}
              placeholder="timeseries_result"
              style={{ marginTop: 4 }}
            />
          </Col>
        </Row>
      </Card>
    </Space>
  );
};

export default TimeSeriesActionEditor;