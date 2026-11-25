// 复合数据（位置、向量、颜色、数组等）的分量解析
import type { EnhancedIoTData } from '../types/realtime';

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
  rawData: unknown;
}


// 解析复合数据类型到分量
export const parseCompositeData = (item: { data?: EnhancedIoTData; timestamp?: Date | string | number }): CompositeDataItem | null => {
  if (!item.data) return null;

  const deviceId = item.data.device_id || 'unknown';
  const dataKey = item.data.key || 'unknown';
  const dataType = item.data.data_type || 'unknown';
  const rawData = item.data.value;

  if (!rawData || typeof rawData !== 'object') return null;
  const raw = rawData as Record<string, unknown>;
  const unitOf = (u: unknown) => (typeof u === 'string' && u !== 'mixed' ? u : '');

  const components: CompositeDataComponent[] = [];

  switch (dataType) {
    case 'location':
      if (raw.latitude !== undefined) components.push({ key: 'latitude', label: '纬度', value: Number(raw.latitude), unit: '°', color: '#1890ff' });
      if (raw.longitude !== undefined) components.push({ key: 'longitude', label: '经度', value: Number(raw.longitude), unit: '°', color: '#52c41a' });
      if (raw.altitude !== undefined) components.push({ key: 'altitude', label: '高度', value: Number(raw.altitude), unit: 'm', color: '#faad14' });
      if (raw.speed !== undefined) components.push({ key: 'speed', label: '速度', value: Number(raw.speed), unit: 'km/h', color: '#f5222d' });
      break;

    case 'vector3d':
      if (raw.x !== undefined) components.push({ key: 'x', label: 'X轴', value: Number(raw.x), color: '#1890ff' });
      if (raw.y !== undefined) components.push({ key: 'y', label: 'Y轴', value: Number(raw.y), color: '#52c41a' });
      if (raw.z !== undefined) components.push({ key: 'z', label: 'Z轴', value: Number(raw.z), color: '#faad14' });
      if (raw.magnitude !== undefined) components.push({ key: 'magnitude', label: '模长', value: Number(raw.magnitude), color: '#722ed1' });
      break;

    case 'color':
      if (raw.r !== undefined) components.push({ key: 'r', label: '红色', value: Number(raw.r), unit: '', color: '#ff4d4f' });
      if (raw.g !== undefined) components.push({ key: 'g', label: '绿色', value: Number(raw.g), unit: '', color: '#52c41a' });
      if (raw.b !== undefined) components.push({ key: 'b', label: '蓝色', value: Number(raw.b), unit: '', color: '#1890ff' });
      if (raw.hue !== undefined) components.push({ key: 'hue', label: '色相', value: Number(raw.hue), unit: '°', color: '#722ed1' });
      if (raw.saturation !== undefined) components.push({ key: 'saturation', label: '饱和度', value: Number(raw.saturation), unit: '%', color: '#fa8c16' });
      if (raw.lightness !== undefined) components.push({ key: 'lightness', label: '亮度', value: Number(raw.lightness), unit: '%', color: '#13c2c2' });
      break;

    case 'vector':
      if (Array.isArray(raw.values)) {
        const labels = Array.isArray(raw.labels) ? (raw.labels as string[]) : [];
        const colors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#fa8c16', '#13c2c2', '#eb2f96'];
        (raw.values as unknown[]).forEach((v, index) => {
          const value = Number(v);
          const label = labels[index] || `分量${index + 1}`;
          components.push({
            key: `v${index}`,
            label,
            value,
            unit: unitOf(raw.unit),
            color: colors[index % colors.length]
          });
        });
      }
      break;

    case 'array':
      // 后端 ArrayData 的字段是 values（以前只读 elements，数组分量始终为空）
      if (Array.isArray(raw.values) || Array.isArray(raw.elements)) {
        const labels = Array.isArray(raw.labels) ? (raw.labels as string[]) : [];
        const colors = ['#1890ff', '#52c41a', '#faad14', '#f5222d', '#722ed1', '#fa8c16', '#13c2c2', '#eb2f96'];
        ((Array.isArray(raw.values) ? raw.values : raw.elements) as unknown[]).forEach((v, index) => {
          const value = Number(v);
          const label = labels[index] || `元素${index + 1}`;
          components.push({
            key: `a${index}`,
            label,
            value,
            unit: unitOf(raw.unit),
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
