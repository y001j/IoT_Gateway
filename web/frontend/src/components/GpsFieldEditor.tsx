import React from 'react';
import {
  Form,
  InputNumber,
  Input,
  Select,
  Row,
  Col,
  Space,
  Typography,
  Tooltip,
  Card
} from 'antd';
import { 
  EnvironmentOutlined,
  InfoCircleOutlined 
} from '@ant-design/icons';

const { Text } = Typography;
const { Option } = Select;

interface GpsCoordinate {
  latitude: number;
  longitude: number;
  altitude?: number;
}

interface GpsFieldEditorProps {
  label?: string;
  value?: GpsCoordinate;
  onChange?: (value: GpsCoordinate) => void;
  placeholder?: {
    latitude?: string;
    longitude?: string;
    altitude?: string;
  };
  showAltitude?: boolean;
  coordinateSystem?: 'WGS84' | 'GCJ02' | 'BD09';
  onCoordinateSystemChange?: (system: string) => void;
}

export const GpsFieldEditor: React.FC<GpsFieldEditorProps> = ({
  label = "GPS坐标",
  value = { latitude: 0, longitude: 0 },
  onChange,
  placeholder = {
    latitude: "例: 39.9042",
    longitude: "例: 116.4074", 
    altitude: "例: 100"
  },
  showAltitude = false,
  coordinateSystem = 'WGS84',
  onCoordinateSystemChange
}) => {
  const handleFieldChange = (field: keyof GpsCoordinate, newValue: number | undefined) => {
    if (onChange && newValue !== undefined) {
      onChange({
        ...value,
        [field]: newValue
      });
    }
  };

  const validateLatitude = (lat: number): boolean => {
    return lat >= -90 && lat <= 90;
  };

  const validateLongitude = (lng: number): boolean => {
    return lng >= -180 && lng <= 180;
  };

  const getCoordinateStatus = () => {
    const { latitude, longitude } = value;
    const latValid = validateLatitude(latitude);
    const lngValid = validateLongitude(longitude);
    
    if (latValid && lngValid) {
      return { status: 'success' as const, text: '坐标有效' };
    } else if (!latValid && !lngValid) {
      return { status: 'error' as const, text: '纬度和经度都无效' };
    } else if (!latValid) {
      return { status: 'error' as const, text: '纬度无效 (应在-90到90之间)' };
    } else {
      return { status: 'error' as const, text: '经度无效 (应在-180到180之间)' };
    }
  };

  const coordinateStatus = getCoordinateStatus();

  return (
    <Card size="small" title={
      <Space>
        <EnvironmentOutlined />
        {label}
        <Text type={coordinateStatus.status === 'success' ? 'success' : 'danger'}>
          {coordinateStatus.text}
        </Text>
      </Space>
    }>
      <Space direction="vertical" style={{ width: '100%' }}>
        {/* 坐标系选择 */}
        <Row gutter={[16, 8]}>
          <Col span={12}>
            <Text strong>坐标系:</Text>
            <Select
              value={coordinateSystem}
              onChange={onCoordinateSystemChange}
              style={{ width: '100%', marginTop: 4 }}
            >
              <Option value="WGS84">
                WGS84 
                <Tooltip title="GPS标准坐标系">
                  <InfoCircleOutlined style={{ marginLeft: 4, color: '#1890ff' }} />
                </Tooltip>
              </Option>
              <Option value="GCJ02">
                GCJ02 
                <Tooltip title="国内常用坐标系(高德、腾讯)">
                  <InfoCircleOutlined style={{ marginLeft: 4, color: '#1890ff' }} />
                </Tooltip>
              </Option>
              <Option value="BD09">
                BD09 
                <Tooltip title="百度坐标系">
                  <InfoCircleOutlined style={{ marginLeft: 4, color: '#1890ff' }} />
                </Tooltip>
              </Option>
            </Select>
          </Col>
        </Row>

        {/* 坐标输入 */}
        <Row gutter={[16, 8]}>
          <Col span={12}>
            <Text strong>纬度 (Latitude):</Text>
            <InputNumber
              value={value.latitude}
              onChange={(newValue) => handleFieldChange('latitude', newValue)}
              placeholder={placeholder.latitude}
              style={{ width: '100%', marginTop: 4 }}
              step={0.000001}
              precision={6}
              min={-90}
              max={90}
              status={validateLatitude(value.latitude) ? undefined : 'error'}
            />
          </Col>
          <Col span={12}>
            <Text strong>经度 (Longitude):</Text>
            <InputNumber
              value={value.longitude}
              onChange={(newValue) => handleFieldChange('longitude', newValue)}
              placeholder={placeholder.longitude}
              style={{ width: '100%', marginTop: 4 }}
              step={0.000001}
              precision={6}
              min={-180}
              max={180}
              status={validateLongitude(value.longitude) ? undefined : 'error'}
            />
          </Col>
        </Row>

        {/* 可选的海拔 */}
        {showAltitude && (
          <Row gutter={[16, 8]}>
            <Col span={12}>
              <Text strong>海拔 (米):</Text>
              <InputNumber
                value={value.altitude}
                onChange={(newValue) => handleFieldChange('altitude', newValue)}
                placeholder={placeholder.altitude}
                style={{ width: '100%', marginTop: 4 }}
                step={1}
                precision={1}
              />
            </Col>
          </Row>
        )}

        {/* 常用坐标快速输入 */}
        <Row>
          <Col span={24}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              常用示例: 北京天安门(39.9042, 116.4074) | 上海外滩(31.2304, 121.4737) | 深圳市政府(22.5431, 114.0579)
            </Text>
          </Col>
        </Row>
      </Space>
    </Card>
  );
};

export default GpsFieldEditor;