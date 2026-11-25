import React, { useMemo, useState, useEffect } from 'react';
import { Card, Checkbox, Space, Tag, Typography, Row, Col, Button } from 'antd';
import { SettingOutlined } from '@ant-design/icons';
import { type CompositeDataItem } from '../../utils/compositeData';
const { Text } = Typography;

export type { CompositeDataComponent, CompositeDataItem } from '../../utils/compositeData';

export interface CompositeDataSelectorProps {
  data: CompositeDataItem[];
  onComponentSelectionChange: (selections: Record<string, string[]>) => void;
  onDataTypeFilter: (dataTypes: string[]) => void;
}

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
    // 只在数据类型变化时初始化；用户取消全部选择后不会被自动重新全选
    if (dataTypes.length > 0) {
      setSelectedDataTypes(prev => (prev.length === 0 ? [...dataTypes] : prev));
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