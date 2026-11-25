import React, { useMemo, useState, useEffect, useCallback } from 'react';
import { Row, Col, Card, Select, Table, Tag, Button, Space, Statistic, message, Alert, Switch } from 'antd';
import { RealTimeChart, ChartSeries, type DataPoint } from './RealTimeChart';
import { CompositeDataSelector } from './CompositeDataSelector';
import { parseCompositeData, type CompositeDataItem } from '../../utils/compositeData';
import { useRealTimeData } from '../../hooks/useRealTimeData';
import { monitoringService } from '../../services/monitoringService';
import { ClearOutlined, PauseOutlined, PlayCircleOutlined, DownloadOutlined, ReloadOutlined, ApiOutlined, WifiOutlined  } from '@ant-design/icons';
import type { DataFlowMetrics } from '../../types/monitoring';

import type { IoTDataPoint } from '../../types/realtime';

/** 图表/表格中的一条数据（复合数据展开后会带上 _expanded_id） */
type ChartItem = IoTDataPoint & { _expanded_id?: string };
const { Option } = Select;

export interface IoTDataChartProps {
  height?: number;
  showRawData?: boolean;
  maxChartPoints?: number;
  maxTableRows?: number;
  autoRefresh?: boolean;
  refreshInterval?: number;
  preferWebSocket?: boolean;
  enableCompositeDataViewer?: boolean;
}

export const IoTDataChart: React.FC<IoTDataChartProps> = ({
  height = 350,
  showRawData = true,
  maxChartPoints = 100,
  maxTableRows = 50,
  autoRefresh = true,
  refreshInterval = 3000,
  preferWebSocket: _preferWebSocket = false,
  enableCompositeDataViewer = true,
}) => {
  const { data: wsData, isConnected } = useRealTimeData();

  // 状态管理
  const [selectedDevice, setSelectedDevice] = useState<string>('all');
  const [selectedKey, setSelectedKey] = useState<string>('all');
  const [isPaused, setIsPaused] = useState(false);
  const [apiData, setApiData] = useState<DataFlowMetrics[]>([]);
  const [apiLoading, setApiLoading] = useState(false);
  const [dataSource, setDataSource] = useState<'websocket' | 'api' | 'mixed'>('api');
  const [lastApiUpdate, setLastApiUpdate] = useState<Date | null>(null);
  const [apiError, setApiError] = useState<string | null>(null);
  
  // 复合数据类型状态
  const [showCompositeViewer, setShowCompositeViewer] = useState(false);
  const [selectedCompositeDataTypes, setSelectedCompositeDataTypes] = useState<string[]>([]);
  const [componentSelections, setComponentSelections] = useState<Record<string, string[]>>({});
  const [compositeViewMode, setCompositeViewMode] = useState<'combined' | 'separated'>('combined');

  // API数据获取
  const fetchApiData = useCallback(async () => {
    try {
      setApiLoading(true);
      setApiError(null);
      const metrics = await monitoringService.getDataFlowMetrics({
        time_range: '5m',
        limit: 100
      });
      setApiData(metrics.metrics || []);
      setLastApiUpdate(new Date());
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : '未知错误';
      setApiError(errorMsg);
      console.warn('❌ API数据获取失败:', errorMsg);
    } finally {
      setApiLoading(false);
    }
  }, []);

  // 自动刷新API数据
  useEffect(() => {
    if (!autoRefresh) return;
    
    fetchApiData();
    const interval = setInterval(fetchApiData, refreshInterval);
    return () => clearInterval(interval);
  }, [autoRefresh, refreshInterval, fetchApiData]);

  // 转换API数据为WebSocket格式 - 增强版本，支持复合数据类型检测
  const convertApiToWebSocketFormat = useCallback((metrics: DataFlowMetrics[]) => {
    return metrics.map((metric, _index) => {
      // 智能检测数据类型
      let detectedDataType = 'unknown';
      let parsedValue = metric.last_value;

      // 尝试解析JSON字符串格式的复合数据
      if (typeof metric.last_value === 'string') {
        try {
          const parsed = JSON.parse(metric.last_value);
          if (parsed && typeof parsed === 'object') {
            parsedValue = parsed;
            // 根据字段特征检测复合数据类型
            if (parsed.latitude !== undefined && parsed.longitude !== undefined) {
              detectedDataType = 'location';
            } else if (parsed.x !== undefined && parsed.y !== undefined && parsed.z !== undefined) {
              detectedDataType = 'vector3d';
            } else if (parsed.r !== undefined && parsed.g !== undefined && parsed.b !== undefined) {
              detectedDataType = 'color';
            } else if (parsed.values && Array.isArray(parsed.values)) {
              detectedDataType = 'vector';
            } else if (parsed.elements && Array.isArray(parsed.elements)) {
              detectedDataType = 'array';
            } else {
              detectedDataType = 'object';
            }
          } else {
            detectedDataType = 'string';
          }
        } catch (e) {
          // 不是JSON，保持为字符串
          detectedDataType = 'string';
        }
      } else if (typeof metric.last_value === 'number') {
        detectedDataType = 'float';
      } else if (typeof metric.last_value === 'object' && metric.last_value !== null) {
        const obj = metric.last_value as Record<string, unknown>;
        parsedValue = obj;
        // 检测已经是对象格式的复合数据
        if (obj.latitude !== undefined && obj.longitude !== undefined) {
          detectedDataType = 'location';
        } else if (obj.x !== undefined && obj.y !== undefined && obj.z !== undefined) {
          detectedDataType = 'vector3d';
        } else if (obj.r !== undefined && obj.g !== undefined && obj.b !== undefined) {
          detectedDataType = 'color';
        } else if (obj.values && Array.isArray(obj.values)) {
          detectedDataType = 'vector';
        } else if (obj.elements && Array.isArray(obj.elements)) {
          detectedDataType = 'array';
        } else {
          detectedDataType = 'object';
        }
      }

      return {
        timestamp: new Date(metric.last_timestamp),
        subject: `iot.data.${metric.adapter_name}.${metric.device_id}.${metric.key}`,
        data: {
          device_id: metric.device_id,
          key: metric.key,
          value: parsedValue,
          data_type: detectedDataType,
          adapter_name: metric.adapter_name,
          bytes_per_sec: metric.bytes_per_sec,
          data_points_per_sec: metric.data_points_per_sec,
          latency_ms: metric.latency_ms,
          error_rate: metric.error_rate,
          derived_values: {
            display_value: typeof parsedValue === 'object' && parsedValue !== null 
              ? `${detectedDataType} 对象` 
              : parsedValue,
            summary: `${metric.data_points_per_sec.toFixed(2)} 点/秒, ${metric.latency_ms.toFixed(1)}ms 延迟, ${(metric.bytes_per_sec/1024).toFixed(1)}KB/s`
          }
        }
      };
    });
  }, []);

  // 智能数据源选择 - 修复版本：优先使用API数据
  const effectiveData = useMemo(() => {
    const wsDataCount = wsData.iotData.length;
    const apiDataCount = apiData.length;
    
    
    // 强制优先使用API数据源（因为WebSocket推送不稳定）
    if (dataSource === 'api' && apiDataCount > 0) {
      const convertedData = convertApiToWebSocketFormat(apiData);
      return convertedData;
    } else if (dataSource === 'websocket' && wsDataCount > 0 && isConnected) {
      return wsData.iotData;
    } else if (dataSource === 'mixed') {
      // 混合模式：优先API，WebSocket作为补充
      if (apiDataCount > 0) {
        const convertedData = convertApiToWebSocketFormat(apiData);
        return convertedData;
      } else if (wsDataCount > 0 && isConnected) {
        return wsData.iotData;
      }
    }
    
    // 自动降级：如果选择的数据源无数据，尝试其他数据源
    if (apiDataCount > 0) {
      const convertedData = convertApiToWebSocketFormat(apiData);
      return convertedData;
    } else if (wsDataCount > 0 && isConnected) {
      return wsData.iotData;
    }
    
    return [];
  }, [wsData.iotData, apiData, dataSource, isConnected, convertApiToWebSocketFormat]);

  // 解析复合数据类型
  const compositeData = useMemo(() => {



    const parsed = effectiveData
      .map((item, _index) => {
        const result = parseCompositeData(item);
        return result;
      })
      .filter(Boolean) as CompositeDataItem[];
    

    return parsed;
  }, [effectiveData]);

  // 过滤复合数据
  const filteredCompositeData = useMemo(() => {
    return compositeData.filter(item => {
      if (selectedCompositeDataTypes.length > 0 && !selectedCompositeDataTypes.includes(item.dataType)) {
        return false;
      }
      return true;
    });
  }, [compositeData, selectedCompositeDataTypes]);

  // 提取设备和数据字段
  const { devices, dataKeys } = useMemo(() => {
    const deviceSet = new Set<string>();
    const keySet = new Set<string>();

    effectiveData.forEach(item => {
      if (item.data?.device_id) {
        deviceSet.add(String(item.data.device_id));
      }
      // 只有在选择了特定设备时，才添加该设备的字段
      // 如果选择了'all'，则添加所有字段
      if (item.data?.key && (selectedDevice === 'all' || item.data.device_id === selectedDevice)) {
        keySet.add(String(item.data.key));
      }
    });

    return {
      devices: Array.from(deviceSet).sort(),
      dataKeys: Array.from(keySet).sort(),
    };
  }, [effectiveData, selectedDevice]);

  // 过滤数据 - 添加暂停功能支持
  const [frozenData, setFrozenData] = useState<ChartItem[]>([]);
  
  const filteredData = useMemo(() => {
    const sourceData = isPaused ? frozenData : effectiveData;
    return sourceData.filter(item => {
      const deviceMatch = selectedDevice === 'all' || item.data?.device_id === selectedDevice;
      const keyMatch = selectedKey === 'all' || item.data?.key === selectedKey;
      return deviceMatch && keyMatch;
    });
  }, [effectiveData, frozenData, isPaused, selectedDevice, selectedKey]);

  // 扩展数据 - 包含复合数据分量的展开数据
  const expandedData = useMemo(() => {
    const expanded: ChartItem[] = [];
    
    filteredData.forEach(item => {
      const dataType = item.data?.data_type;
      
      // 如果是复合数据类型且启用了复合数据查看器
      if (['location', 'vector3d', 'color', 'vector', 'array'].includes(dataType) && showCompositeViewer) {
        const compositeItem = parseCompositeData(item);
        if (compositeItem && compositeItem.components.length > 0) {
          // 为每个分量创建一个数据行
          compositeItem.components.forEach(component => {
            expanded.push({
              ...item,
              data: {
                ...item.data,
                key: `${item.data.key}-${component.key}`,
                value: component.value,
                data_type: `${dataType}_component`,
                derived_values: {
                  display_value: component.value,
                  summary: `${component.label}: ${component.value}${component.unit || ''}`
                },
                component_info: {
                  parent_key: item.data.key,
                  component_key: component.key,
                  component_label: component.label,
                  component_unit: component.unit || '',
                  component_color: component.color
                }
              },
              // 添加唯一标识符
              _expanded_id: `${item.data?.device_id}-${item.data?.key}-${component.key}-${item.timestamp}`
            });
          });
        } else {
          // 如果解析失败，仍然保留原始数据
          expanded.push(item);
        }
      } else {
        // 非复合数据或未启用复合数据查看器，直接添加
        expanded.push(item);
      }
    });
    
    
    return expanded;
  }, [filteredData, showCompositeViewer]);

  // 当暂停/恢复时更新冻结的数据
  React.useEffect(() => {
    if (isPaused && frozenData.length === 0) {
      // 暂停时保存当前数据
      setFrozenData([...effectiveData]);
    } else if (!isPaused) {
      // 恢复时清除冻结数据
      setFrozenData([]);
    }
  }, [isPaused, effectiveData, frozenData.length]);

  // 当设备选择变化时，重置字段选择为'all'
  React.useEffect(() => {
    setSelectedKey('all');
  }, [selectedDevice]);

  // 准备图表数据 - 增强支持复合数据类型分量
  const chartSeries: ChartSeries[] = useMemo(() => {
    const seriesMap = new Map<string, { data: DataPoint[], color: string }>();
    const colors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#13c2c2', '#eb2f96', '#fa8c16'];
    let colorIndex = 0;

    // 处理普通数据
    filteredData.forEach(item => {
      const dataType = item.data?.data_type || 'unknown';
      let plotValue: number | undefined;
      let seriesName = `${item.data?.device_id || 'unknown'}-${item.data?.key || 'value'}`;

      // 根据数据类型确定绘图值
      if (typeof item.data?.value === 'number') {
        // 简单数值类型
        plotValue = item.data.value;
      } else if (item.data?.derived_values?.display_value !== undefined) {
        // 复合数据类型，使用派生的显示值
        const displayValue = item.data.derived_values.display_value;
        if (typeof displayValue === 'number') {
          plotValue = displayValue;
          // 为复合数据类型添加类型标识到系列名称
          seriesName = `${seriesName} (${dataType})`;
        }
      }

      if (plotValue !== undefined) {
        if (!seriesMap.has(seriesName)) {
          seriesMap.set(seriesName, {
            data: [],
            color: colors[colorIndex % colors.length],
          });
          colorIndex++;
        }

        seriesMap.get(seriesName)!.data.push({
          timestamp: item.timestamp,
          value: plotValue,
        });
      }
    });

    // 处理复合数据类型的分量
    if (showCompositeViewer && filteredCompositeData.length > 0) {
      filteredCompositeData.forEach(compositeItem => {
        const deviceKey = `${compositeItem.deviceId}-${compositeItem.dataKey}`;
        const selectedComponents = componentSelections[deviceKey] || [];
        
        compositeItem.components.forEach(component => {
          if (selectedComponents.length === 0 || selectedComponents.includes(component.key)) {
            const componentSeriesName = compositeViewMode === 'separated' 
              ? `${compositeItem.deviceId}-${component.label} (${compositeItem.dataType})`
              : `${compositeItem.deviceId}-${compositeItem.dataKey}-${component.label}`;
            
            if (!seriesMap.has(componentSeriesName)) {
              seriesMap.set(componentSeriesName, {
                data: [],
                color: component.color || colors[colorIndex % colors.length],
              });
              colorIndex++;
            }

            seriesMap.get(componentSeriesName)!.data.push({
              timestamp: compositeItem.timestamp,
              value: component.value,
            });
          }
        });
      });
    }

    return Array.from(seriesMap.entries()).map(([key, series]) => ({
      name: key,
      data: series.data.slice(-maxChartPoints),
      color: series.color,
      type: 'line' as const,
      smooth: true,
    }));
  }, [filteredData, maxChartPoints, showCompositeViewer, filteredCompositeData, componentSelections, compositeViewMode]);

  // 统计信息 - 基于扩展数据
  const statistics = useMemo(() => {
    const now = new Date();
    const lastMinute = new Date(now.getTime() - 60000);
    const last5Minutes = new Date(now.getTime() - 300000);
    
    // 使用扩展数据进行统计
    const baseData = showCompositeViewer ? expandedData : filteredData;
    
    const recentData = baseData.filter(item => {
      const timestamp = new Date(item.timestamp || Date.now());
      return timestamp > lastMinute;
    });
    
    const recent5MinData = baseData.filter(item => {
      const timestamp = new Date(item.timestamp || Date.now());
      return timestamp > last5Minutes;
    });

    // 计算数值型数据的统计 - 包含复合数据分量
    const numericData = baseData.filter(item => {
      return typeof item.data?.value === 'number' || 
             (typeof item.data?.derived_values?.display_value === 'number');
    });
    
    const numericValues = numericData.map(item => {
      if (typeof item.data?.value === 'number') {
        return item.data.value;
      } else if (typeof item.data?.derived_values?.display_value === 'number') {
        return item.data.derived_values.display_value;
      }
      return 0;
    });
    
    const avgValue = numericValues.length > 0 
      ? numericValues.reduce((sum, val) => sum + val, 0) / numericValues.length 
      : 0;

    // 统计数据类型分布 - 基于显示的数据
    const dataTypeStats = new Map<string, number>();
    baseData.forEach(item => {
      let dataType = item.data?.data_type || 'unknown';
      // 将组件类型简化显示
      if (dataType.endsWith('_component')) {
        dataType = dataType.replace('_component', '分量');
      }
      dataTypeStats.set(dataType, (dataTypeStats.get(dataType) || 0) + 1);
    });
    
    // 复合数据统计
    const compositeStats = {
      totalCompositeItems: compositeData.length,
      compositeDataTypes: [...new Set(compositeData.map(item => item.dataType))],
      totalComponents: compositeData.reduce((sum, item) => sum + item.components.length, 0),
      selectedComponents: Object.values(componentSelections).reduce((sum, arr) => sum + arr.length, 0),
      expandedItems: showCompositeViewer ? expandedData.length - filteredData.length : 0,
    };

    return {
      totalPoints: baseData.length,
      recentPoints: recentData.length,
      recent5MinPoints: recent5MinData.length,
      devicesCount: devices.length,
      dataKeysCount: dataKeys.length,
      avgValue: avgValue,
      numericPointsCount: numericData.length,
      dataTypeStats: Object.fromEntries(dataTypeStats),
      composite: compositeStats,
    };
  }, [filteredData, expandedData, showCompositeViewer, devices.length, dataKeys.length, compositeData, componentSelections]);

  // 表格列定义
  const tableColumns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 100,
      render: (time: Date | string) => {
        if (!time) return '--';
        try {
          const date = typeof time === 'string' ? new Date(time) : time;
          return date.toLocaleTimeString();
        } catch (error) {
          console.warn('Invalid timestamp:', time);
          return '--';
        }
      },
    },
    {
      title: '设备ID',
      dataIndex: ['data', 'device_id'],
      key: 'device_id',
      width: 100,
      render: (deviceId: string) => (
        <Tag color="blue">{deviceId || 'Unknown'}</Tag>
      ),
    },
    {
      title: '数据字段',
      dataIndex: ['data', 'key'],
      key: 'key',
      width: 120,
      render: (key: string, record: ChartItem) => {
        const isComponent = record.data?.data_type?.endsWith('_component');
        const componentInfo = record.data?.component_info;
        
        if (isComponent && componentInfo) {
          return (
            <div>
              <Tag color="green">{componentInfo.parent_key}</Tag>
              <Tag color="cyan">{componentInfo.component_label}</Tag>
            </div>
          );
        }
        
        return <Tag color="green">{key || 'value'}</Tag>;
      },
    },
    {
      title: '数据类型',
      dataIndex: ['data', 'data_type'],
      key: 'data_type',
      width: 100,
      render: (dataType: string, _record: ChartItem) => {
        const typeColors = {
          'float': 'blue',
          'int': 'cyan', 
          'string': 'green',
          'bool': 'orange',
          'location': 'purple',
          'vector3d': 'magenta',
          'color': 'red',
          'vector': 'volcano',
          'array': 'geekblue',
          'matrix': 'gold',
          'timeseries': 'lime',
          'location_component': 'purple',
          'vector3d_component': 'magenta',
          'color_component': 'red',
          'vector_component': 'volcano',
          'array_component': 'geekblue',
          'unknown': 'default'
        };
        
        const isComponent = dataType?.endsWith('_component');
        const displayType = isComponent ? dataType.replace('_component', '') + '分量' : dataType;
        
        return (
          <Tag color={typeColors[dataType as keyof typeof typeColors] || 'default'}>
            {displayType || 'unknown'}
          </Tag>
        );
      },
    },
    {
      title: '数值',
      dataIndex: ['data'],
      key: 'value',
      width: 150,
      render: (data: ChartItem['data'], _record: ChartItem) => {
        const dataType = data?.data_type || 'unknown';
        const derivedValues = data?.derived_values;
        const originalValue = data?.value;
        const isComponent = dataType?.endsWith('_component');
        const componentInfo = data?.component_info;

        // 对于分量数据，显示分量值和单位
        if (isComponent && componentInfo) {
          return (
            <div style={{ fontFamily: 'monospace' }}>
              <div style={{ fontSize: '14px', fontWeight: 'bold', color: componentInfo.component_color }}>
                {typeof originalValue === 'number' ? originalValue.toFixed(3) : String(originalValue)}
                {componentInfo.component_unit && <span style={{ fontSize: '12px', color: '#999', marginLeft: '2px' }}>{componentInfo.component_unit}</span>}
              </div>
              <div style={{ fontSize: '10px', color: '#666' }}>
                {componentInfo.component_label}
              </div>
            </div>
          );
        }

        // 原有的显示逻辑
        if (derivedValues?.summary) {
          return (
            <div style={{ fontFamily: 'monospace' }}>
              <div style={{ fontSize: '13px', fontWeight: 'bold' }}>
                {derivedValues.summary}
              </div>
              {derivedValues.display_value !== undefined && (
                <div style={{ fontSize: '11px', color: '#666' }}>
                  值: {typeof derivedValues.display_value === 'number' 
                    ? derivedValues.display_value.toFixed(2) 
                    : String(derivedValues.display_value)}
                </div>
              )}
            </div>
          );
        } else if (typeof originalValue === 'number') {
          return (
            <span style={{ fontFamily: 'monospace' }}>
              {originalValue.toFixed(2)}
            </span>
          );
        } else if (typeof originalValue === 'object' && originalValue !== null) {
          return (
            <span style={{ fontFamily: 'monospace', fontSize: '11px', color: '#666' }}>
              {dataType} 对象
            </span>
          );
        } else {
          return (
            <span style={{ fontFamily: 'monospace' }}>
              {String(originalValue)}
            </span>
          );
        }
      },
    },
    {
      title: '主题',
      dataIndex: ['data', 'subject'],
      key: 'subject',
      ellipsis: true,
      render: (subject: string) => (
        <span style={{ fontSize: '12px', color: '#666' }}>{subject}</span>
      ),
    },
  ];

  const handleClearData = () => {
    message.info('数据清除功能需要后端支持，当前仅为演示');
  };

  const handleExportData = () => {
    try {
      let csvContent = '';
      let totalRows = 0;

      // 导出普通数据
      const normalDataRows = filteredData.map(item => {
        const dataType = item.data?.data_type || 'unknown';
        const originalValue = item.data?.value;
        const derivedValues = item.data?.derived_values;
        
        // 处理原始值的显示
        let originalValueStr = '';
        if (typeof originalValue === 'object' && originalValue !== null) {
          originalValueStr = JSON.stringify(originalValue);
        } else {
          originalValueStr = String(originalValue || '');
        }
        
        // 处理显示值
        const displayValue = derivedValues?.display_value !== undefined 
          ? String(derivedValues.display_value) 
          : '';
        
        // 处理摘要
        const summary = derivedValues?.summary || '';

        return [
          new Date(item.timestamp || Date.now()).toISOString(),
          item.data?.device_id || '',
          item.data?.key || '',
          dataType,
          '普通数据',
          `"${originalValueStr.replace(/"/g, '""')}"`, // 转义CSV中的引号
          displayValue,
          `"${summary.replace(/"/g, '""')}"`, // 转义CSV中的引号
          // subject 在数据项上（以前读 item.data.subject，导出的主题列始终为空）
          item.subject || ''
        ].join(',');
      });

      // 导出复合数据分量
      const compositeDataRows: string[] = [];
      if (showCompositeViewer && filteredCompositeData.length > 0) {
        filteredCompositeData.forEach(compositeItem => {
          const deviceKey = `${compositeItem.deviceId}-${compositeItem.dataKey}`;
          const selectedComponents = componentSelections[deviceKey] || [];
          
          compositeItem.components.forEach(component => {
            if (selectedComponents.length === 0 || selectedComponents.includes(component.key)) {
              compositeDataRows.push([
                compositeItem.timestamp.toISOString(),
                compositeItem.deviceId,
                `${compositeItem.dataKey}-${component.label}`,
                compositeItem.dataType,
                '复合数据分量',
                String(component.value),
                String(component.value),
                `分量: ${component.label}${component.unit ? ' (' + component.unit + ')' : ''}`,
                `${compositeItem.deviceId}.${compositeItem.dataKey}.${component.key}`
              ].join(','));
            }
          });
        });
      }

      // 合并所有数据
      const allRows = [...normalDataRows, ...compositeDataRows];
      totalRows = allRows.length;

      csvContent = [
        ['时间', '设备ID', '数据字段', '数据类型', '数据来源', '原始值', '显示值', '摘要', '主题'].join(','),
        ...allRows
      ].join('\n');

      const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
      const link = document.createElement('a');
      if (link.download !== undefined) {
        const url = URL.createObjectURL(blob);
        link.setAttribute('href', url);
        link.setAttribute('download', `iot-data-enhanced-${new Date().toISOString().slice(0, 10)}.csv`);
        link.style.visibility = 'hidden';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        message.success(`已导出 ${totalRows} 条增强数据（包含 ${compositeDataRows.length} 个复合数据分量）`);
      }
    } catch (error) {
      console.error('Export failed:', error);
      message.error('导出失败');
    }
  };

  return (
    <div>
      {/* 控制面板 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Row gutter={[16, 16]} align="middle">
          <Col xs={24} sm={12} md={6}>
            <span style={{ marginRight: 8 }}>设备:</span>
            <Select
              value={selectedDevice}
              onChange={setSelectedDevice}
              style={{ width: '100%' }}
              size="small"
            >
              <Option value="all">全部设备</Option>
              {devices.map(device => (
                <Option key={device} value={device}>{device}</Option>
              ))}
            </Select>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <span style={{ marginRight: 8 }}>字段:</span>
            <Select
              value={selectedKey}
              onChange={setSelectedKey}
              style={{ width: '100%' }}
              size="small"
            >
              <Option value="all">全部字段</Option>
              {dataKeys.map(key => (
                <Option key={key} value={key}>{key}</Option>
              ))}
            </Select>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <span style={{ marginRight: 8 }}>数据源:</span>
            <Select
              value={dataSource}
              onChange={setDataSource}
              style={{ width: '100%' }}
              size="small"
            >
              <Option value="api">
                <ApiOutlined /> API轮询 (推荐)
              </Option>
              <Option value="mixed">
                <WifiOutlined /> 智能混合
              </Option>
              <Option value="websocket">
                <WifiOutlined /> WebSocket
              </Option>
            </Select>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Space>
              <Button
                size="small"
                icon={isPaused ? <PlayCircleOutlined /> : <PauseOutlined />}
                onClick={() => setIsPaused(!isPaused)}
              >
                {isPaused ? '恢复' : '暂停'}
              </Button>
              <Button
                size="small"
                icon={<ReloadOutlined />}
                onClick={fetchApiData}
                loading={apiLoading}
              >
                刷新
              </Button>
            </Space>
          </Col>
          {enableCompositeDataViewer && compositeData.length > 0 && (
            <Col xs={24} sm={12} md={6}>
              <Space>
                <span style={{ fontSize: '12px' }}>复合数据:</span>
                <Switch
                  size="small"
                  checked={showCompositeViewer}
                  onChange={setShowCompositeViewer}
                  checkedChildren="分量"
                  unCheckedChildren="隐藏"
                />
                {showCompositeViewer && (
                  <Select
                    size="small"
                    value={compositeViewMode}
                    onChange={setCompositeViewMode}
                    style={{ width: 80 }}
                  >
                    <Option value="combined">合并</Option>
                    <Option value="separated">分离</Option>
                  </Select>
                )}
              </Space>
            </Col>
          )}
          
          <Col xs={24} sm={12} md={6}>
            <Space>
              <Button
                size="small"
                icon={<DownloadOutlined />}
                onClick={handleExportData}
                disabled={filteredData.length === 0}
              >
                导出
              </Button>
              <Button
                size="small"
                icon={<ClearOutlined />}
                onClick={handleClearData}
                danger
              >
                清除
              </Button>
            </Space>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '12px', marginBottom: '2px' }}>
                <span style={{ color: isConnected ? '#52c41a' : '#f5222d' }}>
                  ● WS: {isConnected ? '已连接' : '断开'}
                </span>
                <span style={{ marginLeft: '8px', color: apiError ? '#f5222d' : '#52c41a' }}>
                  ● API: {apiError ? '错误' : '正常'}
                </span>
              </div>
              <div style={{ fontSize: '11px', color: '#999' }}>
                {filteredData.length > 0 ? `显示 ${filteredData.length} 条记录` : '暂无数据'}
              </div>
              {lastApiUpdate && (
                <div style={{ fontSize: '10px', color: '#ccc' }}>
                  API更新: {lastApiUpdate.toLocaleTimeString()}
                </div>
              )}
            </div>
          </Col>
        </Row>
      </Card>

      {/* 数据源状态提示 */}
      {apiError && (
        <Alert
          message="API数据获取错误"
          description={`无法从监控API获取数据: ${apiError}${isConnected ? '，当前使用WebSocket数据。' : '，建议检查网络连接。'}`}
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={fetchApiData} loading={apiLoading}>
              重试
            </Button>
          }
        />
      )}
      
      {!isConnected && apiData.length === 0 && (
        <Alert
          message="数据源不可用"
          description="WebSocket连接断开且API数据获取失败。请检查网络连接或联系系统管理员。"
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}
      
      {effectiveData.length === 0 && !apiLoading && (
        <Alert
          message="暂无IoT数据"
          description={`当前${dataSource === 'websocket' ? 'WebSocket' : dataSource === 'api' ? 'API' : '所有数据源'}都没有可用数据。请检查设备连接状态和数据采集配置。`}
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          action={
            <Space>
              <Button size="small" onClick={fetchApiData} loading={apiLoading}>
                刷新API
              </Button>
              <Button size="small" onClick={() => setDataSource('mixed')}>
                切换到混合模式
              </Button>
            </Space>
          }
        />
      )}

      {/* 复合数据类型选择器 */}
      {enableCompositeDataViewer && showCompositeViewer && compositeData.length > 0 && (
        <CompositeDataSelector
          data={filteredCompositeData}
          onComponentSelectionChange={setComponentSelections}
          onDataTypeFilter={setSelectedCompositeDataTypes}
        />
      )}

      {/* 统计信息 */}
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title="总数据点"
              value={statistics.totalPoints}
              valueStyle={{ fontSize: '15px' }}
              suffix={
                <span style={{ fontSize: '10px', color: '#999' }}>
                  ({dataSource === 'websocket' ? 'WS' : dataSource === 'api' ? 'API' : '混合'})
                </span>
              }
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title="最近1分钟"
              value={statistics.recentPoints}
              valueStyle={{ fontSize: '15px', color: statistics.recentPoints > 0 ? '#52c41a' : '#666' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title="最近5分钟"
              value={statistics.recent5MinPoints}
              valueStyle={{ fontSize: '15px', color: statistics.recent5MinPoints > 0 ? '#1890ff' : '#666' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6} md={3}>
          <Card size="small">
            <Statistic
              title="设备数量"
              value={statistics.devicesCount}
              valueStyle={{ fontSize: '15px' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6} md={3}>
          <Card size="small">
            <Statistic
              title="数据字段"
              value={statistics.dataKeysCount}
              valueStyle={{ fontSize: '15px' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card size="small">
            <Statistic
              title="数值平均值"
              value={statistics.avgValue.toFixed(2)}
              valueStyle={{ fontSize: '15px', color: statistics.numericPointsCount > 0 ? '#722ed1' : '#666' }}
              suffix={statistics.numericPointsCount > 0 ? `(${statistics.numericPointsCount}个数值)` : '(无数值)'}
            />
          </Card>
        </Col>
        {/* 复合数据统计 */}
        {enableCompositeDataViewer && statistics.composite.totalCompositeItems > 0 && (
          <>
            <Col xs={12} sm={8} md={4}>
              <Card size="small">
                <Statistic
                  title="复合数据项"
                  value={statistics.composite.totalCompositeItems}
                  valueStyle={{ fontSize: '15px', color: '#13c2c2' }}
                  suffix={`(${statistics.composite.compositeDataTypes.length}种类型)`}
                />
              </Card>
            </Col>
            <Col xs={12} sm={8} md={4}>
              <Card size="small">
                <Statistic
                  title="数据分量"
                  value={`${statistics.composite.selectedComponents}/${statistics.composite.totalComponents}`}
                  valueStyle={{ fontSize: '15px', color: showCompositeViewer ? '#52c41a' : '#999' }}
                  suffix={showCompositeViewer ? '(已选择)' : '(隐藏)'}
                />
              </Card>
            </Col>
          </>
        )}
      </Row>

      {/* 数据类型分布统计 */}
      {Object.keys(statistics.dataTypeStats).length > 1 && (
        <Card 
          title="数据类型分布" 
          size="small" 
          style={{ marginBottom: 16 }}
          bodyStyle={{ padding: '8px 16px' }}
        >
          <Row gutter={[8, 8]}>
            {Object.entries(statistics.dataTypeStats).map(([type, count]) => {
              const typeColors = {
                'float': 'blue', 'int': 'cyan', 'string': 'green', 'bool': 'orange',
                'location': 'purple', 'vector3d': 'magenta', 'color': 'red',
                'vector': 'volcano', 'array': 'geekblue', 'matrix': 'gold',
                'timeseries': 'lime', 'unknown': 'default'
              };
              return (
                <Col key={type}>
                  <Tag 
                    color={typeColors[type as keyof typeof typeColors] || 'default'} 
                    style={{ margin: '2px' }}
                  >
                    {type}: {count}
                  </Tag>
                </Col>
              );
            })}
          </Row>
        </Card>
      )}

      {/* 实时图表 */}
      <Row gutter={[16, 16]}>
        <Col xs={24}>
          {chartSeries.length > 0 ? (
            <RealTimeChart
              title={`IoT 数据流实时监控 ${isPaused ? '(已暂停)' : ''} - ${dataSource === 'websocket' ? 'WebSocket实时' : dataSource === 'api' ? 'API轮询' : '智能混合'}${showCompositeViewer ? ' + 复合数据分量' : ''}`}
              series={chartSeries}
              height={height}
              yAxisLabel="数值"
              maxDataPoints={maxChartPoints}
              loading={apiLoading && !isConnected}
              timeFormat="HH:mm:ss"
            />
          ) : (
            <Card 
              title={`IoT 数据流实时监控 ${isPaused ? '(已暂停)' : ''} - ${dataSource === 'websocket' ? 'WebSocket实时' : dataSource === 'api' ? 'API轮询' : '智能混合'}${showCompositeViewer ? ' + 复合数据分量' : ''}`}
              style={{ height }}
            >
              <div style={{ 
                height: height - 100, 
                display: 'flex', 
                flexDirection: 'column',
                alignItems: 'center', 
                justifyContent: 'center',
                color: '#999'
              }}>
                <div style={{ fontSize: '16px', marginBottom: '8px' }}>
                  📊 暂无可视化数据
                </div>
                <div style={{ fontSize: '14px', textAlign: 'center' }}>
                  {filteredData.length === 0 
                    ? '当前筛选条件下没有数据' 
                    : '当前数据中没有数值型字段，无法生成图表'
                  }
                </div>
                {!isConnected && !apiData.length && (
                  <div style={{ fontSize: '12px', marginTop: '8px', color: '#f5222d' }}>
                    ● WebSocket连接断开，API数据也不可用
                  </div>
                )}
                {!isConnected && apiData.length > 0 && (
                  <div style={{ fontSize: '12px', marginTop: '8px', color: '#faad14' }}>
                    ● WebSocket断开，正在使用API数据
                  </div>
                )}
                {isConnected && dataSource === 'api' && (
                  <div style={{ fontSize: '12px', marginTop: '8px', color: '#1890ff' }}>
                    ● 已切换到API轮询模式
                  </div>
                )}
              </div>
            </Card>
          )}
        </Col>
      </Row>

      {/* 原始数据表格 */}
      {showRawData && (
        <Card 
          title={`原始数据流${showCompositeViewer ? ' (含复合数据分量)' : ''}`}
          style={{ marginTop: 16 }}
          extra={
            <span style={{ fontSize: '12px', color: '#666' }}>
              显示最近 {Math.min(expandedData.length, maxTableRows)} 条记录
              {showCompositeViewer && expandedData.length > filteredData.length && 
                ` (${expandedData.length - filteredData.length} 个分量扩展)`
              }
            </span>
          }
        >
          <Table
            columns={tableColumns}
            dataSource={expandedData.slice(-maxTableRows).reverse()}
            pagination={false}
            size="small"
            scroll={{ y: 300 }}
            rowKey={(record, index) => record._expanded_id || `${index}-${record.timestamp || Date.now()}`}
          />
        </Card>
      )}
    </div>
  );
};