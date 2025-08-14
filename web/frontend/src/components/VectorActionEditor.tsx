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
  Tooltip
} from 'antd';
import { 
  BorderOutlined,
  SwapOutlined,
  AimOutlined,
  RotateRightOutlined,
  FunctionOutlined
} from '@ant-design/icons';

const { Text, Title } = Typography;
const { Option } = Select;

interface Vector3DInput {
  x: number;
  y: number;
  z: number;
}

interface VectorActionConfig {
  sub_type: 'magnitude' | 'normalize' | 'projection' | 'rotation' | 'cross_product' | 'dot_product';
  reference_vector?: Vector3DInput;
  rotation_axis?: 'x' | 'y' | 'z' | 'custom';
  rotation_angle?: number;
  custom_axis?: Vector3DInput;
  normalize_magnitude?: number;
  output_key?: string;
}

interface VectorActionEditorProps {
  value?: VectorActionConfig;
  onChange?: (config: VectorActionConfig) => void;
}

export const VectorActionEditor: React.FC<VectorActionEditorProps> = ({
  value = {
    sub_type: 'magnitude',
    output_key: 'vector_result'
  },
  onChange
}) => {
  const handleChange = (field: keyof VectorActionConfig, newValue: any) => {
    if (onChange) {
      onChange({
        ...value,
        [field]: newValue
      });
    }
  };

  const handleVectorChange = (field: 'reference_vector' | 'custom_axis', axis: 'x' | 'y' | 'z', newValue: number) => {
    const currentVector = value[field] || { x: 0, y: 0, z: 0 };
    handleChange(field, {
      ...currentVector,
      [axis]: newValue
    });
  };

  const renderVectorInput = (
    label: string,
    vectorField: 'reference_vector' | 'custom_axis',
    placeholder = { x: '1.0', y: '0.0', z: '0.0' }
  ) => {
    const vector = value[vectorField] || { x: 0, y: 0, z: 0 };
    
    return (
      <Card size="small" title={label} style={{ marginTop: 16 }}>
        <Row gutter={[8, 8]}>
          <Col span={8}>
            <Text strong>X轴:</Text>
            <InputNumber
              value={vector.x}
              onChange={(val) => handleVectorChange(vectorField, 'x', val || 0)}
              placeholder={placeholder.x}
              style={{ width: '100%', marginTop: 4 }}
              step={0.1}
              precision={3}
            />
          </Col>
          <Col span={8}>
            <Text strong>Y轴:</Text>
            <InputNumber
              value={vector.y}
              onChange={(val) => handleVectorChange(vectorField, 'y', val || 0)}
              placeholder={placeholder.y}
              style={{ width: '100%', marginTop: 4 }}
              step={0.1}
              precision={3}
            />
          </Col>
          <Col span={8}>
            <Text strong>Z轴:</Text>
            <InputNumber
              value={vector.z}
              onChange={(val) => handleVectorChange(vectorField, 'z', val || 0)}
              placeholder={placeholder.z}
              style={{ width: '100%', marginTop: 4 }}
              step={0.1}
              precision={3}
            />
          </Col>
        </Row>
      </Card>
    );
  };

  const renderActionSpecificConfig = () => {
    switch (value.sub_type) {
      case 'magnitude':
        return (
          <Card size="small" title={<><AimOutlined /> 向量模长计算</>}>
            <Text type="secondary">
              计算向量的模长：|v| = √(x² + y² + z²)
            </Text>
          </Card>
        );

      case 'normalize':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><SwapOutlined /> 向量归一化</>}>
              <Row gutter={[16, 8]}>
                <Col span={12}>
                  <Text strong>目标模长:</Text>
                  <Tooltip title="归一化后向量的模长，默认为1.0">
                    <InputNumber
                      value={value.normalize_magnitude || 1.0}
                      onChange={(val) => handleChange('normalize_magnitude', val)}
                      style={{ width: '100%', marginTop: 4 }}
                      min={0.001}
                      step={0.1}
                      precision={3}
                      placeholder="1.0"
                    />
                  </Tooltip>
                </Col>
              </Row>
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                将向量缩放到指定模长，保持方向不变
              </Text>
            </Card>
          </Space>
        );

      case 'projection':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><AimOutlined /> 向量投影</>}>
              {renderVectorInput('参考向量', 'reference_vector', { x: '1.0', y: '0.0', z: '0.0' })}
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                计算向量在参考向量方向上的投影: proj = (v·u / |u|²) × u
              </Text>
            </Card>
          </Space>
        );

      case 'rotation':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><RotateRightOutlined /> 向量旋转</>}>
              <Row gutter={[16, 8]}>
                <Col span={12}>
                  <Text strong>旋转轴:</Text>
                  <Select
                    value={value.rotation_axis || 'z'}
                    onChange={(axis) => handleChange('rotation_axis', axis)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="x">X轴</Option>
                    <Option value="y">Y轴</Option>
                    <Option value="z">Z轴</Option>
                    <Option value="custom">自定义轴</Option>
                  </Select>
                </Col>
                <Col span={12}>
                  <Text strong>旋转角度 (度):</Text>
                  <InputNumber
                    value={value.rotation_angle || 0}
                    onChange={(angle) => handleChange('rotation_angle', angle)}
                    style={{ width: '100%', marginTop: 4 }}
                    min={-360}
                    max={360}
                    step={1}
                    placeholder="45"
                  />
                </Col>
              </Row>
              
              {value.rotation_axis === 'custom' && 
                renderVectorInput('自定义旋转轴', 'custom_axis', { x: '0.0', y: '0.0', z: '1.0' })
              }
              
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                绕指定轴旋转向量指定角度
              </Text>
            </Card>
          </Space>
        );

      case 'cross_product':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><FunctionOutlined /> 向量叉积</>}>
              {renderVectorInput('参考向量', 'reference_vector', { x: '0.0', y: '1.0', z: '0.0' })}
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                计算向量叉积: v × u，结果是垂直于两个向量的新向量
              </Text>
            </Card>
          </Space>
        );

      case 'dot_product':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><FunctionOutlined /> 向量点积</>}>
              {renderVectorInput('参考向量', 'reference_vector', { x: '1.0', y: '1.0', z: '1.0' })}
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                计算向量点积: v·u = vx×ux + vy×uy + vz×uz，结果是标量
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
      <Card size="small" title={<><BorderOutlined /> 向量操作配置</>}>
        <Row gutter={[16, 8]}>
          <Col span={24}>
            <Text strong>向量操作类型:</Text>
            <Select
              value={value.sub_type}
              onChange={(subType) => handleChange('sub_type', subType)}
              style={{ width: '100%', marginTop: 4 }}
            >
              <Option value="magnitude">
                <Space>
                  <AimOutlined />
                  模长计算
                </Space>
              </Option>
              <Option value="normalize">
                <Space>
                  <SwapOutlined />
                  向量归一化
                </Space>
              </Option>
              <Option value="projection">
                <Space>
                  <AimOutlined />
                  向量投影
                </Space>
              </Option>
              <Option value="rotation">
                <Space>
                  <RotateRightOutlined />
                  向量旋转
                </Space>
              </Option>
              <Option value="cross_product">
                <Space>
                  <FunctionOutlined />
                  向量叉积
                </Space>
              </Option>
              <Option value="dot_product">
                <Space>
                  <FunctionOutlined />
                  向量点积
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
              placeholder="vector_result"
              style={{ marginTop: 4 }}
            />
          </Col>
        </Row>
      </Card>
    </Space>
  );
};

export default VectorActionEditor;