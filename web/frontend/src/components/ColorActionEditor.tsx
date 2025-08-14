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
  ColorPicker
} from 'antd';
import { 
  BgColorsOutlined,
  EyeOutlined,
  SwapOutlined,
  FunctionOutlined,
  SkinOutlined,
  BorderOutlined
} from '@ant-design/icons';

const { Text, Title } = Typography;
const { Option } = Select;

interface ColorInput {
  r: number;
  g: number;
  b: number;
  a?: number;
}

interface ColorActionConfig {
  sub_type: 'convert' | 'similarity' | 'extract_dominant' | 'brightness_adjust' | 'saturation_adjust' | 'hue_shift';
  target_color_space?: 'RGB' | 'HSL' | 'HSV' | 'CMYK' | 'LAB';
  source_color_space?: 'RGB' | 'HSL' | 'HSV' | 'CMYK' | 'LAB';
  reference_color?: ColorInput;
  similarity_threshold?: number;
  brightness_adjustment?: number;
  saturation_adjustment?: number;
  hue_shift_degrees?: number;
  output_key?: string;
}

interface ColorActionEditorProps {
  value?: ColorActionConfig;
  onChange?: (config: ColorActionConfig) => void;
}

export const ColorActionEditor: React.FC<ColorActionEditorProps> = ({
  value = {
    sub_type: 'convert',
    output_key: 'color_result'
  },
  onChange
}) => {
  const handleChange = (field: keyof ColorActionConfig, newValue: any) => {
    if (onChange) {
      onChange({
        ...value,
        [field]: newValue
      });
    }
  };

  const handleColorChange = (color: any) => {
    const colorValues = color.toRgb();
    handleChange('reference_color', {
      r: Math.round(colorValues.r),
      g: Math.round(colorValues.g),
      b: Math.round(colorValues.b),
      a: colorValues.a
    });
  };

  const renderColorInput = (
    label: string,
    colorValue: ColorInput = { r: 255, g: 255, b: 255 }
  ) => {
    const hexColor = `#${colorValue.r.toString(16).padStart(2, '0')}${colorValue.g.toString(16).padStart(2, '0')}${colorValue.b.toString(16).padStart(2, '0')}`;
    
    return (
      <Card size="small" title={label} style={{ marginTop: 16 }}>
        <Row gutter={[16, 8]}>
          <Col span={12}>
            <Text strong>颜色选择:</Text>
            <ColorPicker
              value={hexColor}
              onChange={handleColorChange}
              showText
              style={{ width: '100%', marginTop: 4 }}
            />
          </Col>
          <Col span={12}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <Row gutter={8}>
                <Col span={8}>
                  <Text strong>R:</Text>
                  <InputNumber
                    value={colorValue.r}
                    onChange={(val) => handleChange('reference_color', { ...colorValue, r: val || 0 })}
                    min={0}
                    max={255}
                    style={{ width: '100%' }}
                  />
                </Col>
                <Col span={8}>
                  <Text strong>G:</Text>
                  <InputNumber
                    value={colorValue.g}
                    onChange={(val) => handleChange('reference_color', { ...colorValue, g: val || 0 })}
                    min={0}
                    max={255}
                    style={{ width: '100%' }}
                  />
                </Col>
                <Col span={8}>
                  <Text strong>B:</Text>
                  <InputNumber
                    value={colorValue.b}
                    onChange={(val) => handleChange('reference_color', { ...colorValue, b: val || 0 })}
                    min={0}
                    max={255}
                    style={{ width: '100%' }}
                  />
                </Col>
              </Row>
            </Space>
          </Col>
        </Row>
      </Card>
    );
  };

  const renderActionSpecificConfig = () => {
    switch (value.sub_type) {
      case 'convert':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><SwapOutlined /> 颜色空间转换</>}>
              <Row gutter={[16, 8]}>
                <Col span={12}>
                  <Text strong>源颜色空间:</Text>
                  <Select
                    value={value.source_color_space || 'RGB'}
                    onChange={(space) => handleChange('source_color_space', space)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="RGB">RGB (红绿蓝)</Option>
                    <Option value="HSL">HSL (色相饱和度亮度)</Option>
                    <Option value="HSV">HSV (色相饱和度明度)</Option>
                    <Option value="CMYK">CMYK (印刷四色)</Option>
                    <Option value="LAB">LAB (感知均匀)</Option>
                  </Select>
                </Col>
                <Col span={12}>
                  <Text strong>目标颜色空间:</Text>
                  <Select
                    value={value.target_color_space || 'HSL'}
                    onChange={(space) => handleChange('target_color_space', space)}
                    style={{ width: '100%', marginTop: 4 }}
                  >
                    <Option value="RGB">RGB (红绿蓝)</Option>
                    <Option value="HSL">HSL (色相饱和度亮度)</Option>
                    <Option value="HSV">HSV (色相饱和度明度)</Option>
                    <Option value="CMYK">CMYK (印刷四色)</Option>
                    <Option value="LAB">LAB (感知均匀)</Option>
                  </Select>
                </Col>
              </Row>
              <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                将颜色从一个颜色空间转换到另一个颜色空间
              </Text>
            </Card>
          </Space>
        );

      case 'similarity':
        return (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Card size="small" title={<><EyeOutlined /> 颜色相似度计算</>}>
              {renderColorInput('参考颜色', value.reference_color)}
              
              <Card size="small" title="相似度阈值" style={{ marginTop: 16 }}>
                <Row gutter={[16, 8]}>
                  <Col span={24}>
                    <Text strong>相似度阈值 (0-1):</Text>
                    <Tooltip title="0表示完全不同，1表示完全相同">
                      <InputNumber
                        value={value.similarity_threshold || 0.8}
                        onChange={(val) => handleChange('similarity_threshold', val)}
                        style={{ width: '100%', marginTop: 4 }}
                        min={0}
                        max={1}
                        step={0.01}
                        precision={2}
                        placeholder="0.80"
                      />
                    </Tooltip>
                  </Col>
                </Row>
                <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
                  计算输入颜色与参考颜色的相似度，结果为0-1之间的数值
                </Text>
              </Card>
            </Card>
          </Space>
        );

      case 'extract_dominant':
        return (
          <Card size="small" title={<><SkinOutlined /> 主色调提取</>}>
            <Text type="secondary">
              从颜色数据中提取主要的颜色成分，输出最具代表性的颜色值
            </Text>
          </Card>
        );

      case 'brightness_adjust':
        return (
          <Card size="small" title={<><BgColorsOutlined /> 亮度调整</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>亮度调整量 (-1 到 1):</Text>
                <Tooltip title="正数增加亮度，负数降低亮度">
                  <InputNumber
                    value={value.brightness_adjustment || 0}
                    onChange={(val) => handleChange('brightness_adjustment', val)}
                    style={{ width: '100%', marginTop: 4 }}
                    min={-1}
                    max={1}
                    step={0.1}
                    precision={2}
                    placeholder="0.20"
                  />
                </Tooltip>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              调整颜色的亮度，0.2表示增加20%亮度，-0.3表示降低30%亮度
            </Text>
          </Card>
        );

      case 'saturation_adjust':
        return (
          <Card size="small" title={<><BgColorsOutlined /> 饱和度调整</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>饱和度调整量 (-1 到 1):</Text>
                <Tooltip title="正数增加饱和度，负数降低饱和度">
                  <InputNumber
                    value={value.saturation_adjustment || 0}
                    onChange={(val) => handleChange('saturation_adjustment', val)}
                    style={{ width: '100%', marginTop: 4 }}
                    min={-1}
                    max={1}
                    step={0.1}
                    precision={2}
                    placeholder="0.15"
                  />
                </Tooltip>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              调整颜色的饱和度，0.15表示增加15%饱和度，-0.2表示降低20%饱和度
            </Text>
          </Card>
        );

      case 'hue_shift':
        return (
          <Card size="small" title={<><FunctionOutlined /> 色相偏移</>}>
            <Row gutter={[16, 8]}>
              <Col span={24}>
                <Text strong>色相偏移角度 (-180 到 180):</Text>
                <Tooltip title="调整颜色的色相，以度为单位">
                  <InputNumber
                    value={value.hue_shift_degrees || 0}
                    onChange={(val) => handleChange('hue_shift_degrees', val)}
                    style={{ width: '100%', marginTop: 4 }}
                    min={-180}
                    max={180}
                    step={1}
                    placeholder="30"
                    addonAfter="度"
                  />
                </Tooltip>
              </Col>
            </Row>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 8 }}>
              在色相环上偏移指定角度，30度可能将红色偏向橙色
            </Text>
          </Card>
        );

      default:
        return null;
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Card size="small" title={<><BorderOutlined /> 颜色操作配置</>}>
        <Row gutter={[16, 8]}>
          <Col span={24}>
            <Text strong>颜色操作类型:</Text>
            <Select
              value={value.sub_type}
              onChange={(subType) => handleChange('sub_type', subType)}
              style={{ width: '100%', marginTop: 4 }}
            >
              <Option value="convert">
                <Space>
                  <SwapOutlined />
                  颜色空间转换
                </Space>
              </Option>
              <Option value="similarity">
                <Space>
                  <EyeOutlined />
                  颜色相似度
                </Space>
              </Option>
              <Option value="extract_dominant">
                <Space>
                  <SkinOutlined />
                  主色调提取
                </Space>
              </Option>
              <Option value="brightness_adjust">
                <Space>
                  <BgColorsOutlined />
                  亮度调整
                </Space>
              </Option>
              <Option value="saturation_adjust">
                <Space>
                  <BgColorsOutlined />
                  饱和度调整
                </Space>
              </Option>
              <Option value="hue_shift">
                <Space>
                  <FunctionOutlined />
                  色相偏移
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
              placeholder="color_result"
              style={{ marginTop: 4 }}
            />
          </Col>
        </Row>
      </Card>
    </Space>
  );
};

export default ColorActionEditor;