import React, { useState, useCallback } from 'react';
import {
  Card,
  Space,
  Button,
  Tag,
  Form,
  Select,
  Input,
  InputNumber,
  Row,
  Col,
  Switch,
  Tabs,
  Tooltip,
  Divider,
  Alert,
  Typography
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  FireOutlined,
  SwapOutlined,
  FilterOutlined,
  FunctionOutlined,
  ForwardOutlined,
  EnvironmentOutlined,
  BorderOutlined,
  BgColorsOutlined,
  QuestionCircleOutlined
} from '@ant-design/icons';
import type { Action } from '../types/rule';
import type { DataTypeOption } from './DataTypeSelector';
import GpsActionEditor from './GpsActionEditor';
import VectorActionEditor from './VectorActionEditor';
import ColorActionEditor from './ColorActionEditor';
import ArrayActionEditor from './ArrayActionEditor';
import TimeSeriesActionEditor from './TimeSeriesActionEditor';

const { Option } = Select;
const { TextArea } = Input;
const { TabPane } = Tabs;
const { Text } = Typography;

interface ComplexActionFormProps {
  dataType: DataTypeOption;
  value?: Action[];
  onChange?: (actions: Action[]) => void;
}

interface ComplexActionConfig {
  type: string;
  sub_type?: string;
  config: Record<string, any>;
  async?: boolean;
  timeout?: string;
  retry?: number;
}

const ComplexActionForm: React.FC<ComplexActionFormProps> = ({
  dataType,
  value = [],
  onChange
}) => {
  const [actions, setActions] = useState<ComplexActionConfig[]>(
    value.length > 0 ? value.map(action => ({
      type: action.type,
      sub_type: action.config.sub_type,
      config: action.config,
      async: action.async,
      timeout: action.timeout,
      retry: action.retry
    })) : []
  );

  // 根据数据类型获取可用的动作类型
  const getActionTypesForDataType = (dataType: DataTypeOption) => {
    const baseActions = [
      {
        type: 'alert',
        name: '告警通知',
        icon: <FireOutlined style={{ color: '#ff4d4f' }} />,
        description: '发送告警消息',
        available: true
      },
      {
        type: 'forward',
        name: '数据转发',
        icon: <ForwardOutlined style={{ color: '#fa8c16' }} />,
        description: '转发到外部系统',
        available: true
      }
    ];

    switch (dataType.category) {
      case 'geospatial':
        return [
          ...baseActions,
          {
            type: 'geo_transform',
            name: 'GPS数据变换',
            icon: <EnvironmentOutlined style={{ color: '#52c41a' }} />,
            description: '距离计算、坐标转换、地理围栏等',
            available: true
          },
          {
            type: 'geo_aggregate',
            name: 'GPS数据聚合',
            icon: <FunctionOutlined style={{ color: '#1890ff' }} />,
            description: '轨迹分析、位置聚类等',
            available: true
          },
          {
            type: 'geo_filter',
            name: 'GPS数据过滤',
            icon: <FilterOutlined style={{ color: '#722ed1' }} />,
            description: '地理围栏过滤、位置阈值等',
            available: true
          }
        ];

      case 'vector':
        return [
          ...baseActions,
          {
            type: 'vector_transform',
            name: '向量数据变换',
            icon: <BorderOutlined style={{ color: '#13c2c2' }} />,
            description: '向量运算、归一化、投影等',
            available: true
          },
          {
            type: 'vector_aggregate',
            name: '向量数据聚合',
            icon: <FunctionOutlined style={{ color: '#1890ff' }} />,
            description: '向量统计、主成分分析等',
            available: true
          },
          {
            type: 'vector_filter',
            name: '向量数据过滤',
            icon: <FilterOutlined style={{ color: '#722ed1' }} />,
            description: '向量阈值过滤、方向过滤等',
            available: true
          }
        ];

      case 'visual':
        return [
          ...baseActions,
          {
            type: 'color_transform',
            name: '颜色数据变换',
            icon: <BgColorsOutlined style={{ color: '#eb2f96' }} />,
            description: '颜色空间转换、颜色分析等',
            available: true
          },
          {
            type: 'color_aggregate',
            name: '颜色数据聚合',
            icon: <FunctionOutlined style={{ color: '#1890ff' }} />,
            description: '颜色统计、主色调提取等',
            available: true
          },
          {
            type: 'color_filter',
            name: '颜色数据过滤',
            icon: <FilterOutlined style={{ color: '#722ed1' }} />,
            description: '颜色相似度过滤等',
            available: true
          }
        ];

      case 'array':
      case 'matrix':
        return [
          ...baseActions,
          {
            type: 'array_transform',
            name: '数组数据变换',
            icon: <SwapOutlined style={{ color: '#2f54eb' }} />,
            description: '数组运算、统计变换等',
            available: true
          },
          {
            type: 'array_aggregate',
            name: '数组数据聚合',
            icon: <FunctionOutlined style={{ color: '#1890ff' }} />,
            description: '数组统计、模式识别等',
            available: true
          },
          {
            type: 'array_filter',
            name: '数组数据过滤',
            icon: <FilterOutlined style={{ color: '#722ed1' }} />,
            description: '数组模式过滤、异常检测等',
            available: true
          }
        ];

      case 'timeseries':
        return [
          ...baseActions,
          {
            type: 'timeseries_transform',
            name: '时序数据变换',
            icon: <SwapOutlined style={{ color: '#f5222d' }} />,
            description: '趋势分析、滤波处理等',
            available: true
          },
          {
            type: 'timeseries_aggregate',
            name: '时序数据聚合',
            icon: <FunctionOutlined style={{ color: '#1890ff' }} />,
            description: '时间窗口聚合、周期分析等',
            available: true
          },
          {
            type: 'timeseries_filter',
            name: '时序数据过滤',
            icon: <FilterOutlined style={{ color: '#722ed1' }} />,
            description: '异常检测、噪声过滤等',
            available: true
          }
        ];

      default:
        return baseActions;
    }
  };

  // 根据动作类型获取子类型选项
  const getSubTypesForAction = (actionType: string) => {
    switch (actionType) {
      case 'geo_transform':
        return [
          { value: 'distance', label: '距离计算', description: '计算到参考点的距离' },
          { value: 'bearing', label: '方位角计算', description: '计算相对于参考点的方位角' },
          { value: 'geofence', label: '地理围栏检查', description: '检查是否在指定区域内' },
          { value: 'coordinate_convert', label: '坐标系转换', description: 'WGS84、GCJ02等坐标系转换' },
          { value: 'clustering', label: '位置聚类', description: '地理位置聚类分析' }
        ];

      case 'vector_transform':
        return [
          { value: 'normalize', label: '向量归一化', description: '将向量归一化到单位长度' },
          { value: 'projection', label: '向量投影', description: '向量在指定方向上的投影' },
          { value: 'cross_product', label: '向量叉积', description: '计算向量叉积' },
          { value: 'dot_product', label: '向量点积', description: '计算向量点积' },
          { value: 'rotation', label: '向量旋转', description: '旋转向量到指定角度' }
        ];

      case 'color_transform':
        return [
          { value: 'rgb_to_hsl', label: 'RGB转HSL', description: '将RGB颜色转换为HSL' },
          { value: 'hsl_to_rgb', label: 'HSL转RGB', description: '将HSL颜色转换为RGB' },
          { value: 'brightness', label: '亮度调整', description: '调整颜色亮度' },
          { value: 'saturation', label: '饱和度调整', description: '调整颜色饱和度' },
          { value: 'color_distance', label: '颜色距离', description: '计算与参考颜色的距离' }
        ];

      case 'array_transform':
        return [
          { value: 'statistics', label: '统计计算', description: '计算数组统计信息' },
          { value: 'normalize', label: '数组归一化', description: '将数组归一化' },
          { value: 'filter_outliers', label: '异常值过滤', description: '过滤数组中的异常值' },
          { value: 'smooth', label: '数据平滑', description: '对数组数据进行平滑处理' },
          { value: 'fft', label: '频域变换', description: '快速傅里叶变换' }
        ];

      case 'timeseries_transform':
        return [
          { value: 'trend_analysis', label: '趋势分析', description: '分析时间序列趋势' },
          { value: 'seasonal_decompose', label: '季节分解', description: '分解季节性成分' },
          { value: 'moving_average', label: '移动平均', description: '计算移动平均' },
          { value: 'diff', label: '差分运算', description: '计算时间序列差分' },
          { value: 'resample', label: '重采样', description: '改变时间序列采样频率' }
        ];

      default:
        return [];
    }
  };

  // 渲染动作配置表单
  const renderActionConfig = (action: ComplexActionConfig, index: number) => {
    switch (action.type) {
      case 'geo_transform':
        return renderGeoTransformConfig(action, index);
      case 'vector_transform':
        return renderVectorTransformConfig(action, index);
      case 'color_transform':
        return renderColorTransformConfig(action, index);
      case 'array_transform':
        return renderArrayTransformConfig(action, index);
      case 'timeseries_transform':
        return renderTimeSeriesTransformConfig(action, index);
      case 'alert':
        return renderAlertConfig(action, index);
      case 'forward':
        return renderForwardConfig(action, index);
      default:
        return <div>请选择动作类型</div>;
    }
  };

  // 渲染GPS变换配置
  const renderGeoTransformConfig = (action: ComplexActionConfig, index: number) => {
    // 将当前action配置转换为GPS编辑器格式
    const gpsConfig = {
      sub_type: action.sub_type as any,
      reference_point: {
        latitude: action.config.reference_lat || action.config.center_lat || 0,
        longitude: action.config.reference_lng || action.config.center_lng || 0
      },
      target_coordinate_system: action.config.target_coordinate_system || 'WGS84',
      source_coordinate_system: action.config.source_coordinate_system || 'WGS84',
      radius: action.config.radius,
      unit: action.config.unit || 'meters',
      output_key: action.config.output_key || 'gps_result'
    };

    const handleGpsConfigChange = (newConfig: any) => {
      // 更新action配置
      const updatedConfig = {
        ...action.config,
        // 根据GPS编辑器的配置更新action
        reference_lat: newConfig.reference_point?.latitude,
        reference_lng: newConfig.reference_point?.longitude,
        center_lat: newConfig.reference_point?.latitude, // 兼容geofence
        center_lng: newConfig.reference_point?.longitude, // 兼容geofence
        target_coordinate_system: newConfig.target_coordinate_system,
        source_coordinate_system: newConfig.source_coordinate_system,
        radius: newConfig.radius,
        unit: newConfig.unit,
        output_key: newConfig.output_key
      };

      // 更新sub_type
      if (newConfig.sub_type !== action.sub_type) {
        updateActionSubType(index, newConfig.sub_type);
      }

      // 更新配置
      updateAction(index, {
        ...action,
        sub_type: newConfig.sub_type,
        config: updatedConfig
      });
    };

    return (
      <GpsActionEditor
        value={gpsConfig}
        onChange={handleGpsConfigChange}
      />
    );
  };

  // 渲染向量变换配置
  const renderVectorTransformConfig = (action: ComplexActionConfig, index: number) => {
    // 将当前action配置转换为向量编辑器格式
    const vectorConfig = {
      sub_type: action.sub_type as any,
      reference_vector: {
        x: action.config.reference_x || 0,
        y: action.config.reference_y || 0,
        z: action.config.reference_z || 0
      },
      rotation_axis: action.config.rotation_axis || 'z',
      rotation_angle: action.config.rotation_angle || 0,
      custom_axis: {
        x: action.config.custom_axis_x || 0,
        y: action.config.custom_axis_y || 0,
        z: action.config.custom_axis_z || 1
      },
      normalize_magnitude: action.config.normalize_magnitude || 1.0,
      output_key: action.config.output_key || 'vector_result'
    };

    const handleVectorConfigChange = (newConfig: any) => {
      // 更新action配置
      const updatedConfig = {
        ...action.config,
        // 根据向量编辑器的配置更新action
        reference_x: newConfig.reference_vector?.x,
        reference_y: newConfig.reference_vector?.y,
        reference_z: newConfig.reference_vector?.z,
        rotation_axis: newConfig.rotation_axis,
        rotation_angle: newConfig.rotation_angle,
        custom_axis_x: newConfig.custom_axis?.x,
        custom_axis_y: newConfig.custom_axis?.y,
        custom_axis_z: newConfig.custom_axis?.z,
        normalize_magnitude: newConfig.normalize_magnitude,
        output_key: newConfig.output_key
      };

      // 更新sub_type
      if (newConfig.sub_type !== action.sub_type) {
        updateActionSubType(index, newConfig.sub_type);
      }

      // 更新配置
      updateAction(index, {
        ...action,
        sub_type: newConfig.sub_type,
        config: updatedConfig
      });
    };

    return (
      <VectorActionEditor
        value={vectorConfig}
        onChange={handleVectorConfigChange}
      />
    );
  };

  // 渲染颜色变换配置
  const renderColorTransformConfig = (action: ComplexActionConfig, index: number) => {
    // 将当前action配置转换为颜色编辑器格式
    const colorConfig = {
      sub_type: action.sub_type as any,
      target_color_space: action.config.target_color_space || 'HSL',
      source_color_space: action.config.source_color_space || 'RGB',
      reference_color: {
        r: action.config.reference_r || 255,
        g: action.config.reference_g || 255,
        b: action.config.reference_b || 255,
        a: action.config.reference_a || 1
      },
      similarity_threshold: action.config.similarity_threshold || 0.8,
      brightness_adjustment: action.config.brightness_adjustment || 0,
      saturation_adjustment: action.config.saturation_adjustment || 0,
      hue_shift_degrees: action.config.hue_shift_degrees || 0,
      output_key: action.config.output_key || 'color_result'
    };

    const handleColorConfigChange = (newConfig: any) => {
      // 更新action配置
      const updatedConfig = {
        ...action.config,
        target_color_space: newConfig.target_color_space,
        source_color_space: newConfig.source_color_space,
        reference_r: newConfig.reference_color?.r,
        reference_g: newConfig.reference_color?.g,
        reference_b: newConfig.reference_color?.b,
        reference_a: newConfig.reference_color?.a,
        similarity_threshold: newConfig.similarity_threshold,
        brightness_adjustment: newConfig.brightness_adjustment,
        saturation_adjustment: newConfig.saturation_adjustment,
        hue_shift_degrees: newConfig.hue_shift_degrees,
        output_key: newConfig.output_key
      };

      // 更新sub_type
      if (newConfig.sub_type !== action.sub_type) {
        updateActionSubType(index, newConfig.sub_type);
      }

      // 更新配置
      updateAction(index, {
        ...action,
        sub_type: newConfig.sub_type,
        config: updatedConfig
      });
    };

    return (
      <ColorActionEditor
        value={colorConfig}
        onChange={handleColorConfigChange}
      />
    );
  };

  // 渲染数组变换配置  
  const renderArrayTransformConfig = (action: ComplexActionConfig, index: number) => {
    // 将当前action配置转换为数组编辑器格式
    const arrayConfig = {
      sub_type: action.sub_type as any,
      operation: action.config.operation || 'mean',
      filter_condition: action.config.filter_condition,
      filter_type: action.config.filter_type || 'value_range',
      min_value: action.config.min_value,
      max_value: action.config.max_value,
      outlier_method: action.config.outlier_method || 'zscore',
      outlier_threshold: action.config.outlier_threshold || 3,
      sort_order: action.config.sort_order || 'asc',
      sort_by: action.config.sort_by || 'value',
      slice_start: action.config.slice_start || 0,
      slice_end: action.config.slice_end,
      slice_step: action.config.slice_step || 1,
      smooth_window: action.config.smooth_window || 5,
      smooth_method: action.config.smooth_method || 'moving_average',
      normalize_method: action.config.normalize_method || 'minmax',
      fft_type: action.config.fft_type || 'magnitude',
      output_key: action.config.output_key || 'array_result'
    };

    const handleArrayConfigChange = (newConfig: any) => {
      // 更新action配置
      const updatedConfig = {
        ...action.config,
        ...newConfig  // 直接复制所有属性
      };

      // 更新sub_type
      if (newConfig.sub_type !== action.sub_type) {
        updateActionSubType(index, newConfig.sub_type);
      }

      // 更新配置
      updateAction(index, {
        ...action,
        sub_type: newConfig.sub_type,
        config: updatedConfig
      });
    };

    return (
      <ArrayActionEditor
        value={arrayConfig}
        onChange={handleArrayConfigChange}
      />
    );
  };

  // 渲染时序变换配置
  const renderTimeSeriesTransformConfig = (action: ComplexActionConfig, index: number) => {
    // 将当前action配置转换为时间序列编辑器格式
    const timeseriesConfig = {
      sub_type: action.sub_type as any,
      window_size: action.config.window_size || 10,
      window_type: action.config.window_type || 'sliding',
      trend_method: action.config.trend_method || 'linear',
      seasonal_period: action.config.seasonal_period || 12,
      decompose_model: action.config.decompose_model || 'additive',
      diff_order: action.config.diff_order || 1,
      diff_seasonal: action.config.diff_seasonal || false,
      resample_frequency: action.config.resample_frequency || 'hour',
      resample_method: action.config.resample_method || 'mean',
      anomaly_method: action.config.anomaly_method || 'zscore',
      anomaly_threshold: action.config.anomaly_threshold || 2.5,
      forecast_steps: action.config.forecast_steps || 5,
      forecast_method: action.config.forecast_method || 'linear',
      correlation_lag: action.config.correlation_lag || 1,
      output_key: action.config.output_key || 'timeseries_result'
    };

    const handleTimeSeriesConfigChange = (newConfig: any) => {
      // 更新action配置
      const updatedConfig = {
        ...action.config,
        ...newConfig  // 直接复制所有属性
      };

      // 更新sub_type
      if (newConfig.sub_type !== action.sub_type) {
        updateActionSubType(index, newConfig.sub_type);
      }

      // 更新配置
      updateAction(index, {
        ...action,
        sub_type: newConfig.sub_type,
        config: updatedConfig
      });
    };

    return (
      <TimeSeriesActionEditor
        value={timeseriesConfig}
        onChange={handleTimeSeriesConfigChange}
      />
    );
  };

  // 渲染告警配置
  const renderAlertConfig = (action: ComplexActionConfig, index: number) => (
    <div>
      <Row gutter={16}>
        <Col span={12}>
          <Form.Item label="告警级别">
            <Select
              value={action.config.level || 'warning'}
              onChange={(value) => updateActionConfig(index, 'level', value)}
            >
              <Option value="info">信息</Option>
              <Option value="warning">警告</Option>
              <Option value="error">错误</Option>
              <Option value="critical">严重</Option>
            </Select>
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item label="限流时间">
            <Input
              value={action.config.throttle || ''}
              onChange={(e) => updateActionConfig(index, 'throttle', e.target.value)}
              placeholder="如 5m, 1h"
            />
          </Form.Item>
        </Col>
      </Row>
      
      <Form.Item label="告警消息">
        <TextArea
          rows={2}
          value={action.config.message || ''}
          onChange={(e) => updateActionConfig(index, 'message', e.target.value)}
          placeholder={`${dataType.name}数据告警: {{.field_name}}`}
        />
      </Form.Item>
    </div>
  );

  // 渲染转发配置
  const renderForwardConfig = (action: ComplexActionConfig, index: number) => (
    <div>
      <Form.Item label="转发目标">
        <Select
          value={action.config.target_type || 'http'}
          onChange={(value) => updateActionConfig(index, 'target_type', value)}
        >
          <Option value="http">HTTP接口</Option>
          <Option value="mqtt">MQTT</Option>
          <Option value="kafka">Kafka</Option>
        </Select>
      </Form.Item>

      {action.config.target_type === 'http' && (
        <Form.Item label="URL地址">
          <Input
            value={action.config.url}
            onChange={(e) => updateActionConfig(index, 'url', e.target.value)}
            placeholder="https://api.example.com/complex-data"
          />
        </Form.Item>
      )}
    </div>
  );

  // 更新动作类型
  const updateActionType = (index: number, actionType: string) => {
    const newActions = [...actions];
    newActions[index] = {
      ...newActions[index],
      type: actionType,
      sub_type: '',
      config: {}
    };
    setActions(newActions);
    emitChange(newActions);
  };

  // 更新动作子类型
  const updateActionSubType = (index: number, subType: string) => {
    const newActions = [...actions];
    newActions[index] = {
      ...newActions[index],
      sub_type: subType,
      config: { ...newActions[index].config, sub_type: subType }
    };
    setActions(newActions);
    emitChange(newActions);
  };

  // 更新动作配置
  const updateActionConfig = (index: number, key: string, value: any) => {
    const newActions = [...actions];
    newActions[index] = {
      ...newActions[index],
      config: { ...newActions[index].config, [key]: value }
    };
    setActions(newActions);
    emitChange(newActions);
  };

  // 更新整个动作
  const updateAction = (index: number, action: ComplexActionConfig) => {
    const newActions = [...actions];
    newActions[index] = action;
    setActions(newActions);
    emitChange(newActions);
  };

  // 添加动作
  const addAction = (actionType: string) => {
    const newAction: ComplexActionConfig = {
      type: actionType,
      config: {},
      async: false,
      timeout: '30s',
      retry: 0
    };
    const newActions = [...actions, newAction];
    setActions(newActions);
    emitChange(newActions);
  };

  // 删除动作
  const removeAction = (index: number) => {
    const newActions = actions.filter((_, i) => i !== index);
    setActions(newActions);
    emitChange(newActions);
  };

  // 触发onChange事件
  const emitChange = useCallback((newActions: ComplexActionConfig[]) => {
    const formattedActions: Action[] = newActions.map(action => ({
      type: action.type,
      config: action.config,
      async: action.async,
      timeout: action.timeout,
      retry: action.retry
    }));
    onChange?.(formattedActions);
  }, [onChange]);

  const availableActionTypes = getActionTypesForDataType(dataType);

  return (
    <Card
      title={
        <Space>
          {dataType.icon}
          {dataType.name} 执行动作配置
        </Space>
      }
      size="small"
    >
      <Space direction="vertical" style={{ width: '100%' }}>
        {actions.map((action, index) => (
          <Card
            key={index}
            size="small"
            title={
              <Space>
                {availableActionTypes.find(t => t.type === action.type)?.icon}
                <span>动作 {index + 1}</span>
                <Tag color="blue">
                  {availableActionTypes.find(t => t.type === action.type)?.name || action.type}
                </Tag>
                {action.sub_type && (
                  <Tag color="green" size="small">{action.sub_type}</Tag>
                )}
              </Space>
            }
            extra={
              actions.length > 1 && (
                <Button
                  type="text"
                  danger
                  size="small"
                  icon={<DeleteOutlined />}
                  onClick={() => removeAction(index)}
                />
              )
            }
          >
            <div>
              <Form.Item label="动作类型">
                <Select
                  value={action.type}
                  onChange={(value) => updateActionType(index, value)}
                  style={{ width: '100%' }}
                >
                  {availableActionTypes.map(type => (
                    <Option key={type.type} value={type.type}>
                      <Space>
                        {type.icon}
                        {type.name}
                        <Text type="secondary">- {type.description}</Text>
                      </Space>
                    </Option>
                  ))}
                </Select>
              </Form.Item>

              {renderActionConfig(action, index)}

              <Divider size="small" />

              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item label="异步执行">
                    <Switch
                      checked={action.async}
                      onChange={(checked) => {
                        const newActions = [...actions];
                        newActions[index] = { ...newActions[index], async: checked };
                        setActions(newActions);
                        emitChange(newActions);
                      }}
                      checkedChildren="是"
                      unCheckedChildren="否"
                    />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item label="超时时间">
                    <Input
                      value={action.timeout}
                      onChange={(e) => {
                        const newActions = [...actions];
                        newActions[index] = { ...newActions[index], timeout: e.target.value };
                        setActions(newActions);
                        emitChange(newActions);
                      }}
                      placeholder="30s"
                    />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item label="重试次数">
                    <InputNumber
                      value={action.retry}
                      onChange={(value) => {
                        const newActions = [...actions];
                        newActions[index] = { ...newActions[index], retry: value || 0 };
                        setActions(newActions);
                        emitChange(newActions);
                      }}
                      min={0}
                      max={10}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
              </Row>
            </div>
          </Card>
        ))}

        <Card size="small">
          <Alert
            message="选择适合的动作类型"
            description={`为${dataType.name}数据选择合适的处理动作`}
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
          />
          
          <Space wrap>
            {availableActionTypes.map(actionType => (
              <Button
                key={actionType.type}
                type="dashed"
                icon={actionType.icon}
                onClick={() => addAction(actionType.type)}
              >
                {actionType.name}
              </Button>
            ))}
          </Space>
        </Card>
      </Space>
    </Card>
  );
};

export default ComplexActionForm;