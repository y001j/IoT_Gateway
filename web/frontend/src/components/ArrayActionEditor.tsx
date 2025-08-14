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
  Slider
} from 'antd';
import { 
  OrderedListOutlined,
  BarChartOutlined,
  FilterOutlined,
  SortAscendingOutlined,
  ScissorOutlined,
  FunctionOutlined,
  LineChartOutlined,
  BorderOutlined
} from '@ant-design/icons';

const { Text, Title } = Typography;
const { Option } = Select;
const { TextArea } = Input;

interface ArrayActionConfig {
  sub_type: 'aggregate' | 'transform' | 'filter' | 'sort' | 'slice' | 'smooth' | 'normalize' | 'fft';
  operation?: 'sum' | 'mean' | 'max' | 'min' | 'std' | 'median' | 'count' | 'p90' | 'p95' | 'p99';
  filter_condition?: string;
  filter_type?: 'value_range' | 'outliers' | 'expression';
  min_value?: number;
  max_value?: number;
  outlier_method?: 'zscore' | 'iqr' | 'percentile';
  outlier_threshold?: number;
  sort_order?: 'asc' | 'desc';
  sort_by?: 'value' | 'index' | 'abs_value';
  slice_start?: number;
  slice_end?: number;
  slice_step?: number;
  smooth_window?: number;
  smooth_method?: 'moving_average' | 'gaussian' | 'savgol';
  normalize_method?: 'minmax' | 'zscore' | 'robust';
  fft_type?: 'magnitude' | 'phase' | 'power' | 'complex';
  output_key?: string;
}

interface ArrayActionEditorProps {
  value?: ArrayActionConfig;
  onChange?: (config: ArrayActionConfig) => void;
}

export const ArrayActionEditor: React.FC<ArrayActionEditorProps> = ({
  value = {
    sub_type: 'aggregate',
    output_key: 'array_result'
  },
  onChange
}) => {
  const handleChange = (field: keyof ArrayActionConfig, newValue: any) => {
    if (onChange) {
      onChange({
        ...value,
        [field]: newValue
      });
    }
  };

  const renderActionSpecificConfig = () => {
    switch (value.sub_type) {
      case 'aggregate':
        return (
          <Card size="small" title={<><BarChartOutlined /> 数组聚合操作</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>聚合函数:</Text>
                <Select
                  value={value.operation || 'mean'}
                  onChange={(operation) => handleChange('operation', operation)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="sum">求和 (Sum)</Option>
                  <Option value="mean">平均值 (Mean)</Option>
                  <Option value="median">中位数 (Median)</Option>
                  <Option value="max">最大值 (Max)</Option>
                  <Option value="min">最小值 (Min)</Option>
                  <Option value="std">标准差 (Std Dev)</Option>
                  <Option value="count">计数 (Count)</Option>
                  <Option value="p90">90分位数</Option>
                  <Option value="p95">95分位数</Option>
                  <Option value="p99">99分位数</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              对数组中的数值执行统计聚合操作，输出单个聚合结果
            </Text>
          </Card>
        );

      case 'transform':
        return (
          <Card size="small" title={<><FunctionOutlined /> 数组变换</>}>
            <Text type="secondary">
              对数组元素进行数学变换，如对数、指数、平方根等操作
            </Text>
          </Card>
        );

      case 'filter':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><FilterOutlined /> 数组过滤</>}>
              <Row gutter={[16, 8]}>
                <Col span={24}>
                  <Text strong>过滤类型:</Text>
                  <Select
                    value={value.filter_type || 'value_range'}
                    onChange={(type) => handleChange('filter_type', type)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="value_range">数值范围过滤</Option>
                    <Option value="outliers">异常值过滤</Option>
                    <Option value="expression">条件表达式过滤</Option>
                  </Select>
                </Col>
              </Row>

              {value.filter_type === 'value_range' && (
                <Card size="small" title="数值范围" style={{ marginTop: 16 }}>
                  <Row gutter={[16, 8]}>
                    <Col span={12}>
                      <Text strong>最小值:</Text>
                      <InputNumber
                        value={value.min_value}
                        onChange={(val) => handleChange('min_value', val)}
                        style={{ width: '100%', marginTop: 4 }}
                        placeholder="如 0"
                      />
                    </Col>
                    <Col span={12}>
                      <Text strong>最大值:</Text>
                      <InputNumber
                        value={value.max_value}
                        onChange={(val) => handleChange('max_value', val)}
                        style={{ width: '100%', marginTop: 4 }}
                        placeholder="如 100"
                      />
                    </Col>
                  </Row>
                </Card>
              )}

              {value.filter_type === 'outliers' && (
                <Card size="small" title="异常值检测" style={{ marginTop: 16 }}>
                  <Row gutter={[16, 8]}>
                    <Col span={12}>
                      <Text strong>检测方法:</Text>
                      <Select
                        value={value.outlier_method || 'zscore'}
                        onChange={(method) => handleChange('outlier_method', method)}
                        style={{ width: '100%', marginTop: 4 }}
                      >
                        <Option value="zscore">Z-Score标准化</Option>
                        <Option value="iqr">四分位距方法</Option>
                        <Option value="percentile">百分位数方法</Option>
                      </Select>
                    </Col>
                    <Col span={12}>
                      <Text strong>阈值:</Text>
                      <InputNumber
                        value={value.outlier_threshold || 3}
                        onChange={(val) => handleChange('outlier_threshold', val)}
                        style={{ width: '100%', marginTop: 4 }}
                        min={1}
                        max={5}
                        step={0.1}
                        placeholder="3.0"
                      />
                    </Col>
                  </Row>
                </Card>
              )}

              {value.filter_type === 'expression' && (
                <Card size="small" title="过滤表达式" style={{ marginTop: 16 }}>
                  <Text strong>条件表达式:</Text>
                  <TextArea
                    value={value.filter_condition}
                    onChange={(e) => handleChange('filter_condition', e.target.value)}
                    style={{ marginTop: 4 }}
                    rows={2}
                    placeholder="如: value > 0 && value < 100"
                  />
                  <Text type="secondary" style={{ fontSize: 12, marginTop: 4 }}>
                    使用 value 表示数组元素值，支持 &gt;、&lt;、==、&amp;&amp;、|| 等运算符
                  </Text>
                </Card>
              )}
            </Card>
          </Space>
        );

      case 'sort':
        return (
          <Card size="small" title={<><SortAscendingOutlined /> 数组排序</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>排序依据:</Text>
                <Select
                  value={value.sort_by || 'value'}
                  onChange={(sortBy) => handleChange('sort_by', sortBy)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="value">按数值大小</Option>
                  <Option value="abs_value">按绝对值大小</Option>
                  <Option value="index">按索引位置</Option>
                </Select>
              </Col>
              <Col span={12}>
                <Text strong>排序顺序:</Text>
                <Select
                  value={value.sort_order || 'asc'}
                  onChange={(order) => handleChange('sort_order', order)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="asc">升序 (小到大)</Option>
                  <Option value="desc">降序 (大到小)</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              对数组元素进行排序操作，输出排序后的数组
            </Text>
          </Card>
        );

      case 'slice':
        return (
          <Card size="small" title={<><ScissorOutlined /> 数组切片</>}>
            <Row gutter={[16, 8]}>
              <Col span={8}>
                <Text strong>起始位置:</Text>
                <InputNumber
                  value={value.slice_start || 0}
                  onChange={(val) => handleChange('slice_start', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={0}
                  placeholder="0"
                />
              </Col>
              <Col span={8}>
                <Text strong>结束位置:</Text>
                <InputNumber
                  value={value.slice_end}
                  onChange={(val) => handleChange('slice_end', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={0}
                  placeholder="10"
                />
              </Col>
              <Col span={8}>
                <Text strong>步长:</Text>
                <InputNumber
                  value={value.slice_step || 1}
                  onChange={(val) => handleChange('slice_step', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={1}
                  placeholder="1"
                />
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              提取数组的指定片段，支持步长跳跃采样
            </Text>
          </Card>
        );

      case 'smooth':
        return (
          <Card size="small" title={<><LineChartOutlined /> 数据平滑</>}>
            <Row gutter={[16, 8]}>
              <Col span={12}>
                <Text strong>平滑方法:</Text>
                <Select
                  value={value.smooth_method || 'moving_average'}
                  onChange={(method) => handleChange('smooth_method', method)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="moving_average">移动平均</Option>
                  <Option value="gaussian">高斯滤波</Option>
                  <Option value="savgol">Savitzky-Golay滤波</Option>
                </Select>
              </Col>
              <Col span={12}>
                <Text strong>窗口大小:</Text>
                <InputNumber
                  value={value.smooth_window || 5}
                  onChange={(val) => handleChange('smooth_window', val)}
                  style={{ width: '100%', marginTop: 4 }}
                  min={3}
                  max={51}
                  step={2}
                  placeholder="5"
                />
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              对数组数据进行平滑处理，减少噪声和波动
            </Text>
          </Card>
        );

      case 'normalize':
        return (
          <Card size="small" title={<><BarChartOutlined /> 数据归一化</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>归一化方法:</Text>
                <Select
                  value={value.normalize_method || 'minmax'}
                  onChange={(method) => handleChange('normalize_method', method)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="minmax">最小最大值归一化 (0-1)</Option>
                  <Option value="zscore">Z-Score标准化 (均值0，标准差1)</Option>
                  <Option value="robust">鲁棒归一化 (基于中位数)</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              将数组数据缩放到特定范围，便于不同尺度数据的比较
            </Text>
          </Card>
        );

      case 'fft':
        return (
          <Card size="small" title={<><FunctionOutlined /> 快速傅里叶变换</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>输出类型:</Text>
                <Select
                  value={value.fft_type || 'magnitude'}
                  onChange={(type) => handleChange('fft_type', type)}
                  style={{ width: '100%', marginTop: 4 }}
                >
                  <Option value="magnitude">幅度谱</Option>
                  <Option value="phase">相位谱</Option>
                  <Option value="power">功率谱</Option>
                  <Option value="complex">复数形式</Option>
                </Select>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              将时域信号转换到频域，分析信号的频率成分
            </Text>
          </Card>
        );

      default:
        return null;
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Card size="small" title={<><BorderOutlined /> 数组操作配置</>}>
        <Row gutter={[16, 8]}>
          <Col span={24}>
            <Text strong>数组操作类型:</Text>
            <Select
              value={value.sub_type}
              onChange={(subType) => handleChange('sub_type', subType)}
              style={{ width: '100%', marginTop: 4 }}
            >
              <Option value="aggregate">
                <Space>
                  <BarChartOutlined />
                  数组聚合
                </Space>
              </Option>
              <Option value="transform">
                <Space>
                  <FunctionOutlined />
                  数组变换
                </Space>
              </Option>
              <Option value="filter">
                <Space>
                  <FilterOutlined />
                  数组过滤
                </Space>
              </Option>
              <Option value="sort">
                <Space>
                  <SortAscendingOutlined />
                  数组排序
                </Space>
              </Option>
              <Option value="slice">
                <Space>
                  <ScissorOutlined />
                  数组切片
                </Space>
              </Option>
              <Option value="smooth">
                <Space>
                  <LineChartOutlined />
                  数据平滑
                </Space>
              </Option>
              <Option value="normalize">
                <Space>
                  <BarChartOutlined />
                  数据归一化
                </Space>
              </Option>
              <Option value="fft">
                <Space>
                  <FunctionOutlined />
                  傅里叶变换
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
              placeholder="array_result"
              style={{ marginTop: 4 }}
            />
          </Col>
        </Row>
      </Card>
    </Space>
  );
};

export default ArrayActionEditor;