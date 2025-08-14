import React, { useMemo, useState, useEffect } from 'react';
import { Card, Select, Checkbox, Space, Tag, Typography, Row, Col, Button } from 'antd';
import { SettingOutlined } from '@ant-design/icons';

const { Option } = Select;
const { Text } = Typography;

export interface CompositeDataComponent {
  key: string;
  label: string;
  value: number;
  unit?: string;
  color?: string;
}

export interface CompositeDataItem {
  deviceId: string;
  dataKey: string;
  dataType: string;
  timestamp: Date;
  components: CompositeDataComponent[];
  rawData: any;
}

export interface CompositeDataSelectorProps {
  data: CompositeDataItem[];
  onComponentSelectionChange: (selections: Record<string, string[]>) => void;
  onDataTypeFilter: (dataTypes: string[]) => void;
}

// 解析复合数据类型到分量
export const parseCompositeData = (item: any): CompositeDataItem | null => {
  if (!item.data) return null;

  const deviceId = item.data.device_id || 'unknown';
  const dataKey = item.data.key || 'unknown';
  const dataType = item.data.data_type || 'unknown';
  const rawData = item.data.value;

  if (!rawData || typeof rawData !== 'object') return null;

  let components: CompositeDataComponent[] = [];

  switch (dataType) {
    case 'location':
      if (rawData.latitude !== undefined) components.push({ key: 'latitude', label: '纬度', value: rawData.latitude, unit: '°', color: '#1890ff' });
      if (rawData.longitude !== undefined) components.push({ key: 'longitude', label: '经度', value: rawData.longitude, unit: '°', color: '#52c41a' });
      if (rawData.altitude !== undefined) components.push({ key: 'altitude', label: '高度', value: rawData.altitude, unit: 'm', color: '#faad14' });
      if (rawData.speed !== undefined) components.push({ key: 'speed', label: '速度', value: rawData.speed, unit: 'km/h', color: '#f5222d' });
      break;

    case 'vector3d':
      if (rawData.x !== undefined) components.push({ key: 'x', label: 'X轴', value: rawData.x, color: '#1890ff' });
      if (rawData.y !== undefined) components.push({ key: 'y', label: 'Y轴', value: rawData.y, color: '#52c41a' });
      if (rawData.z !== undefined) components.push({ key: 'z', label: 'Z轴', value: rawData.z, color: '#faad14' });
      if (rawData.magnitude !== undefined) components.push({ key: 'magnitude', label: '模长', value: rawData.magnitude, color: '#722ed1' });
      break;

    case 'color':
      if (rawData.r !== undefined) components.push({ key: 'r', label: '红色', value: rawData.r, unit: '', color: '#ff4d4f' });
      if (rawData.g !== undefined) components.push({ key: 'g', label: '绿色', value: rawData.g, unit: '', color: '#52c41a' });
      if (rawData.b !== undefined) components.push({ key: 'b', label: '蓝色', value: rawData.b, unit: '', color: '#1890ff' });
      if (rawData.hue !== undefined) components.push({ key: 'hue', label: '色相', value: rawData.hue, unit: '°', color: '#722ed1' });
      if (rawData.saturation !== undefined) components.push({ key: 'saturation', label: '饱和度', value: rawData.saturation, unit: '%', color: '#fa8c16' });
      if (rawData.lightness !== undefined) components.push({ key: 'lightness', label: '亮度', value: rawData.lightness, unit: '%', color: '#13c2c2' });
      break;

    case 'vector':
      if (rawData.values && Array.isArray(rawData.values)) {
        const labels = rawData.labels || [];
        const colors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#fa8c16', '#13c2c2', '#eb2f96'];
        rawData.values.forEach((value: number, index: number) => {
          const label = labels[index] || `分量${index + 1}`;
          components.push({
            key: `v${index}`,
            label,
            value,
            unit: rawData.unit === 'mixed' ? '' : rawData.unit,
            color: colors[index % colors.length]
          });
        });
      }
      break;

    case 'array':
      if (rawData.elements && Array.isArray(rawData.elements)) {
        const labels = rawData.labels || [];
        const colors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#fa8c16', '#13c2c2', '#eb2f96'];
        rawData.elements.forEach((value: number, index: number) => {
          const label = labels[index] || `元素${index + 1}`;
          components.push({
            key: `a${index}`,
            label,
            value,
            unit: rawData.unit === 'mixed' ? '' : rawData.unit,
            color: colors[index % colors.length]
          });
        });
      }
      break;

    default:
      return null;
  }

  if (components.length === 0) return null;

  return {
    deviceId,
    dataKey,
    dataType,
    timestamp: new Date(item.timestamp || Date.now()),
    components,
    rawData
  };
};

export const CompositeDataSelector: React.FC<CompositeDataSelectorProps> = ({
  data,
  onComponentSelectionChange,
  onDataTypeFilter,
}) => {
  const [selectedDataTypes, setSelectedDataTypes] = useState<string[]>([]);
  const [componentSelections, setComponentSelections] = useState<Record<string, string[]>>({});
  const [showAdvanced, setShowAdvanced] = useState(false);

  // 提取可用的数据类型和设备组合
  const { dataTypes, deviceDataKeys, totalComponents } = useMemo(() => {
    const typeSet = new Set<string>();
    const deviceKeyMap = new Map<string, Set<string>>();
    let componentCount = 0;

    data.forEach(item => {
      typeSet.add(item.dataType);
      const deviceKey = `${item.deviceId}-${item.dataKey}`;
      
      if (!deviceKeyMap.has(deviceKey)) {
        deviceKeyMap.set(deviceKey, new Set());
      }
      
      item.components.forEach(comp => {
        deviceKeyMap.get(deviceKey)!.add(`${comp.key}:${comp.label}`);
        componentCount++;
      });
    });

    return {
      dataTypes: Array.from(typeSet).sort(),
      deviceDataKeys: Array.from(deviceKeyMap.entries()).map(([deviceKey, components]) => ({
        deviceKey,
        components: Array.from(components)
      })),
      totalComponents: componentCount
    };
  }, [data]);

  // 初始化选择状态
  useEffect(() => {
    if (dataTypes.length > 0 && selectedDataTypes.length === 0) {
      setSelectedDataTypes([...dataTypes]);
    }
  }, [dataTypes]);

  useEffect(() => {
    onDataTypeFilter(selectedDataTypes);
  }, [selectedDataTypes, onDataTypeFilter]);

  // 处理组件选择变化
  const handleComponentSelection = (deviceKey: string, componentKey: string, checked: boolean) => {
    setComponentSelections(prev => {
      const newSelections = { ...prev };
      if (!newSelections[deviceKey]) {
        newSelections[deviceKey] = [];
      }
      
      if (checked) {
        if (!newSelections[deviceKey].includes(componentKey)) {
          newSelections[deviceKey] = [...newSelections[deviceKey], componentKey];
        }
      } else {
        newSelections[deviceKey] = newSelections[deviceKey].filter(key => key !== componentKey);
      }
      
      onComponentSelectionChange(newSelections);
      return newSelections;
    });
  };

  // 全选/取消全选某个设备的所有组件
  const toggleDeviceComponents = (deviceKey: string, components: string[]) => {
    const allSelected = components.every(comp => 
      componentSelections[deviceKey]?.includes(comp.split(':')[0]) || false
    );
    
    setComponentSelections(prev => {
      const newSelections = { ...prev };
      if (allSelected) {
        newSelections[deviceKey] = [];
      } else {
        newSelections[deviceKey] = components.map(comp => comp.split(':')[0]);
      }
      onComponentSelectionChange(newSelections);
      return newSelections;
    });
  };

  const getDataTypeColor = (dataType: string): string => {
    const colors = {
      'location': '#722ed1',
      'vector3d': '#1890ff', 
      'color': '#f5222d',
      'vector': '#52c41a',
      'array': '#faad14',
    };
    return colors[dataType as keyof typeof colors] || '#666';
  };

  const getDataTypeIcon = (dataType: string): string => {
    const icons = {
      'location': '🌍',
      'vector3d': '📐', 
      'color': '🎨',
      'vector': '📊',
      'array': '📋',
    };
    return icons[dataType as keyof typeof icons] || '📦';
  };

  return (
    <Card
      title={
        <Space>
          <span>复合数据类型分量选择器</span>
          <Tag color="blue">{data.length} 条复合数据</Tag>
          <Tag color="green">{totalComponents} 个分量</Tag>
        </Space>
      }
      size="small"
      extra={
        <Button 
          size="small" 
          icon={<SettingOutlined />}
          onClick={() => setShowAdvanced(!showAdvanced)}
        >
          {showAdvanced ? '简化' : '高级'}
        </Button>
      }
    >
      {/* 数据类型过滤器 */}
      <div style={{ marginBottom: 12 }}>
        <Text strong>数据类型过滤：</Text>
        <div style={{ marginTop: 4 }}>
          <Space wrap>
            {dataTypes.map(type => (
              <Checkbox
                key={type}
                checked={selectedDataTypes.includes(type)}
                onChange={(e) => {
                  if (e.target.checked) {
                    setSelectedDataTypes(prev => [...prev, type]);
                  } else {
                    setSelectedDataTypes(prev => prev.filter(t => t !== type));
                  }
                }}
              >
                <Tag color={getDataTypeColor(type)} style={{ margin: 0 }}>
                  {getDataTypeIcon(type)} {type}
                </Tag>
              </Checkbox>
            ))}
          </Space>
        </div>
      </div>

      {/* 组件选择器 */}
      {showAdvanced && (
        <div>
          <Text strong>分量选择：</Text>
          <div style={{ marginTop: 8, maxHeight: 200, overflowY: 'auto' }}>
            {deviceDataKeys.map(({ deviceKey, components }) => {
              const [deviceId, dataKey] = deviceKey.split('-');
              const selectedCount = componentSelections[deviceKey]?.length || 0;
              
              return (
                <Card key={deviceKey} size="small" style={{ marginBottom: 8 }}>
                  <div style={{ marginBottom: 8 }}>
                    <Space>
                      <Text strong>{deviceId}</Text>
                      <Text type="secondary">→</Text>
                      <Text code>{dataKey}</Text>
                      <Button
                        size="small"
                        type="text"
                        onClick={() => toggleDeviceComponents(deviceKey, components)}
                      >
                        {selectedCount === components.length ? '取消全选' : '全选'} 
                        ({selectedCount}/{components.length})
                      </Button>
                    </Space>
                  </div>
                  <Row gutter={[8, 4]}>
                    {components.map(comp => {
                      const [compKey, compLabel] = comp.split(':');
                      const isSelected = componentSelections[deviceKey]?.includes(compKey) || false;
                      
                      return (
                        <Col key={compKey}>
                          <Checkbox
                            checked={isSelected}
                            onChange={(e) => handleComponentSelection(deviceKey, compKey, e.target.checked)}
                          >
                            <Text style={{ fontSize: '12px' }}>{compLabel}</Text>
                          </Checkbox>
                        </Col>
                      );
                    })}
                  </Row>
                </Card>
              );
            })}
          </div>
        </div>
      )}

      {/* 快速统计信息 */}
      <div style={{ marginTop: 8, padding: '8px', backgroundColor: '#fafafa', borderRadius: '4px' }}>
        <Space split={<span style={{ color: '#d9d9d9' }}>|</span>}>
          <Text style={{ fontSize: '12px' }}>
            数据类型: {selectedDataTypes.length}/{dataTypes.length}
          </Text>
          <Text style={{ fontSize: '12px' }}>
            设备组合: {deviceDataKeys.length}
          </Text>
          <Text style={{ fontSize: '12px' }}>
            已选分量: {Object.values(componentSelections).reduce((sum, arr) => sum + arr.length, 0)}
          </Text>
        </Space>
      </div>
    </Card>
  );
};