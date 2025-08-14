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
  Switch
} from 'antd';
import { 
  EnvironmentOutlined,
  SwapOutlined,
  AimOutlined,
  CompassOutlined
} from '@ant-design/icons';
import GpsFieldEditor from './GpsFieldEditor';

const { Text, Title } = Typography;
const { Option } = Select;

interface GpsActionConfig {
  sub_type: 'distance' | 'bearing' | 'coordinate_convert' | 'geofence';
  reference_point?: {
    latitude: number;
    longitude: number;
  };
  target_coordinate_system?: 'WGS84' | 'GCJ02' | 'BD09';
  source_coordinate_system?: 'WGS84' | 'GCJ02' | 'BD09';
  radius?: number;
  unit?: 'meters' | 'kilometers' | 'miles';
  output_key?: string;
}

interface GpsActionEditorProps {
  value?: GpsActionConfig;
  onChange?: (config: GpsActionConfig) => void;
}

export const GpsActionEditor: React.FC<GpsActionEditorProps> = ({
  value = {
    sub_type: 'distance',
    reference_point: { latitude: 0, longitude: 0 },
    unit: 'meters',
    output_key: 'gps_result'
  },
  onChange
}) => {
  const handleChange = (field: keyof GpsActionConfig, newValue: any) => {
    if (onChange) {
      onChange({
        ...value,
        [field]: newValue
      });
    }
  };

  const renderActionSpecificConfig = () => {
    switch (value.sub_type) {
      case 'distance':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><AimOutlined /> 距离计算配置</>}>
              <GpsFieldEditor
                label="参考点坐标"
                value={value.reference_point}
                onChange={(coord) => handleChange('reference_point', coord)}
                placeholder={{
                  latitude: "参考点纬度",
                  longitude: "参考点经度"
                }}
              />
              
              <Row gutter={[16, 8]} style={{ marginTop: 16 }}>
                <Col span={12}>
                  <Text strong>距离单位:</Text>
                  <Select
                    value={value.unit}
                    onChange={(unit) => handleChange('unit', unit)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="meters">米 (m)</Option>
                    <Option value="kilometers">千米 (km)</Option>
                    <Option value="miles">英里 (mile)</Option>
                  </Select>
                </Col>
                <Col span={12}>
                  <Text strong>输出字段名:</Text>
                  <Input
                    value={value.output_key}
                    onChange={(e) => handleChange('output_key', e.target.value)}
                    placeholder="distance_result"
                    style={{ marginTop: 4 }}
                  />
                </Col>
              </Row>
            </Card>
          </Space>
        );

      case 'bearing':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><CompassOutlined /> 方位角计算配置</>}>
              <GpsFieldEditor
                label="参考点坐标"
                value={value.reference_point}
                onChange={(coord) => handleChange('reference_point', coord)}
                placeholder={{
                  latitude: "参考点纬度",
                  longitude: "参考点经度"
                }}
              />
              
              <Row gutter={[16, 8]} style={{ marginTop: 16 }}>
                <Col span={12}>
                  <Text strong>输出字段名:</Text>
                  <Input
                    value={value.output_key}
                    onChange={(e) => handleChange('output_key', e.target.value)}
                    placeholder="bearing_result"
                    style={{ marginTop: 4 }}
                  />
                </Col>
              </Row>
              
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                方位角以度为单位，0°为北方，顺时针方向
              </Text>
            </Card>
          </Space>
        );

      case 'coordinate_convert':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><SwapOutlined /> 坐标系转换配置</>}>
              <Row gutter={[16, 8]}>
                <Col span={12}>
                  <Text strong>源坐标系:</Text>
                  <Select
                    value={value.source_coordinate_system}
                    onChange={(system) => handleChange('source_coordinate_system', system)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="WGS84">WGS84 (GPS标准)</Option>
                    <Option value="GCJ02">GCJ02 (国内常用)</Option>
                    <Option value="BD09">BD09 (百度坐标)</Option>
                  </Select>
                </Col>
                <Col span={12}>
                  <Text strong>目标坐标系:</Text>
                  <Select
                    value={value.target_coordinate_system}
                    onChange={(system) => handleChange('target_coordinate_system', system)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="WGS84">WGS84 (GPS标准)</Option>
                    <Option value="GCJ02">GCJ02 (国内常用)</Option>
                    <Option value="BD09">BD09 (百度坐标)</Option>
                  </Select>
                </Col>
              </Row>
              
              <Row gutter={[16, 8]} style={{ marginTop: 16 }}>
                <Col span={12}>
                  <Text strong>输出字段名:</Text>
                  <Input
                    value={value.output_key}
                    onChange={(e) => handleChange('output_key', e.target.value)}
                    placeholder="converted_coords"
                    style={{ marginTop: 4 }}
                  />
                </Col>
              </Row>
            </Card>
          </Space>
        );

      case 'geofence':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><EnvironmentOutlined /> 地理围栏配置</>}>
              <GpsFieldEditor
                label="围栏中心点"
                value={value.reference_point}
                onChange={(coord) => handleChange('reference_point', coord)}
                placeholder={{
                  latitude: "中心点纬度",
                  longitude: "中心点经度"
                }}
              />
              
              <Row gutter={[16, 8]} style={{ marginTop: 16 }}>
                <Col span={8}>
                  <Text strong>围栏半径:</Text>
                  <InputNumber
                    value={value.radius}
                    onChange={(radius) => handleChange('radius', radius)}
                    style={{ width: '100%', marginTop: 4 }}
                    min={1}
                    placeholder="100"
                  />
                </Col>
                <Col span={8}>
                  <Text strong>单位:</Text>
                  <Select
                    value={value.unit}
                    onChange={(unit) => handleChange('unit', unit)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="meters">米 (m)</Option>
                    <Option value="kilometers">千米 (km)</Option>
                  </Select>
                </Col>
                <Col span={8}>
                  <Text strong>输出字段名:</Text>
                  <Input
                    value={value.output_key}
                    onChange={(e) => handleChange('output_key', e.target.value)}
                    placeholder="in_geofence"
                    style={{ marginTop: 4 }}
                  />
                </Col>
              </Row>
              
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                输出布尔值：true表示在围栏内，false表示在围栏外
              </Text>
            </Card>
          </Space>
        );

      default:
        return null;
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Card size="small" title={<><EnvironmentOutlined /> GPS操作配置</>}>
        <Row gutter={[16, 8]}>
          <Col span={24}>
            <Text strong>GPS操作类型:</Text>
            <Select
              value={value.sub_type}
              onChange={(subType) => handleChange('sub_type', subType)}
              style={{ width: '100%', marginTop: 4 }}
            >
              <Option value="distance">
                <Space>
                  <AimOutlined />
                  距离计算
                </Space>
              </Option>
              <Option value="bearing">
                <Space>
                  <CompassOutlined />
                  方位角计算
                </Space>
              </Option>
              <Option value="coordinate_convert">
                <Space>
                  <SwapOutlined />
                  坐标系转换
                </Space>
              </Option>
              <Option value="geofence">
                <Space>
                  <EnvironmentOutlined />
                  地理围栏检查
                </Space>
              </Option>
            </Select>
          </Col>
        </Row>
      </Card>

      {renderActionSpecificConfig()}
    </Space>
  );
};

export default GpsActionEditor;