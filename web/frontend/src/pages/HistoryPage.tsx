import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert, App, Button, Card, Col, DatePicker, Empty, InputNumber, Popconfirm, Progress, Row, Select, Space,
  Statistic, Table, Tag, Tooltip, Typography,
} from 'antd';
import { DownloadOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import dayjs, { type Dayjs } from 'dayjs';
import { dataService, type BufferedSinkStats, type LatestPoint, type SeriesInfo, type SeriesResult, type SinkAction, type StorageStatus } from '../services/dataService';
import { formatLag, formatPointValue, isNumericSeries, sinkStateInfo, tierLabel, toChartData, type Agg, type Tier } from '../utils/history';
import { formatBytes } from '../utils/format';
import { useAuthStore } from '../store/authStore';

const { RangePicker } = DatePicker;
const { Text } = Typography;

const AGG_OPTIONS: { value: Agg; label: string }[] = [
  { value: 'avg', label: '平均值' },
  { value: 'min', label: '最小值' },
  { value: 'max', label: '最大值' },
  { value: 'last', label: '最后值' },
  { value: 'lttb', label: '保形 (LTTB)' },
];

const TIER_OPTIONS: { value: Tier; label: string }[] = [
  { value: 'auto', label: '自动选择数据层' },
  { value: 'raw', label: '原始数据' },
  { value: 'rollup', label: '汇总层' },
];

const RANGE_PRESETS: { label: string; value: [Dayjs, Dayjs] }[] = [
  { label: '最近 15 分钟', value: [dayjs().subtract(15, 'minute'), dayjs()] },
  { label: '最近 1 小时', value: [dayjs().subtract(1, 'hour'), dayjs()] },
  { label: '最近 6 小时', value: [dayjs().subtract(6, 'hour'), dayjs()] },
  { label: '最近 24 小时', value: [dayjs().subtract(24, 'hour'), dayjs()] },
  { label: '最近 7 天', value: [dayjs().subtract(7, 'day'), dayjs()] },
  { label: '最近 30 天', value: [dayjs().subtract(30, 'day'), dayjs()] },
];

function errorMessage(err: unknown): string {
  const e = err as { response?: { data?: { message?: string } }; message?: string };
  return e?.response?.data?.message ?? e?.message ?? '请求失败';
}

const HistoryPage: React.FC = () => {
  const { message } = App.useApp();
  const isAdmin = useAuthStore((s) => s.user?.role === 'admin');

  const [status, setStatus] = useState<StorageStatus | null>(null);
  const [series, setSeries] = useState<SeriesInfo[]>([]);
  const [device, setDevice] = useState<string>();
  const [key, setKey] = useState<string>();
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(1, 'hour'), dayjs()]);
  const [agg, setAgg] = useState<Agg>('avg');
  const [limit, setLimit] = useState<number>(1000);
  const [tier, setTier] = useState<Tier>('auto');
  const [result, setResult] = useState<SeriesResult | null>(null);
  const [latest, setLatest] = useState<LatestPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);

  const loadStatus = useCallback(async () => {
    try {
      const st = await dataService.storageStatus();
      setStatus(st);
      if (st.enabled) {
        const res = await dataService.listSeries();
        setSeries(res.series ?? []);
      }
    } catch (err) {
      message.error(`读取本地存储状态失败：${errorMessage(err)}`);
    }
  }, [message]);

  useEffect(() => {
    loadStatus();
    const timer = setInterval(() => {
      dataService.storageStatus().then(setStatus).catch(() => undefined);
    }, 5000);
    return () => clearInterval(timer);
  }, [loadStatus]);

  const devices = useMemo(() => Array.from(new Set(series.map((s) => s.device_id))), [series]);
  const keys = useMemo(() => series.filter((s) => s.device_id === device).map((s) => s.key), [series, device]);

  useEffect(() => {
    if (!device) {
      setLatest([]);
      return;
    }
    dataService.latest(device).then(setLatest).catch(() => setLatest([]));
  }, [device]);

  const query = async () => {
    if (!device || !key) {
      message.warning('请选择设备和点位');
      return;
    }
    setLoading(true);
    try {
      const res = await dataService.history({ deviceId: device, key, start: range[0].toDate(), end: range[1].toDate(), limit, agg, tier });
      setResult(res);
    } catch (err) {
      message.error(`查询失败：${errorMessage(err)}`);
    } finally {
      setLoading(false);
    }
  };

  const exportCsv = async () => {
    if (!device || !key) return;
    setExporting(true);
    try {
      const { truncated } = await dataService.exportCsv({ deviceId: device, key, start: range[0].toDate(), end: range[1].toDate() });
      if (truncated) message.warning('数据量超过单次扫描上限，导出的数据不完整，请缩小时间范围');
    } catch (err) {
      message.error(`导出失败：${errorMessage(err)}`);
    } finally {
      setExporting(false);
    }
  };

  const control = async (name: string, action: SinkAction, from?: Date) => {
    try {
      await dataService.controlSink(name, action, from);
      message.success('操作已执行');
      setStatus(await dataService.storageStatus());
    } catch (err) {
      message.error(`操作失败：${errorMessage(err)}`);
    }
  };

  const numeric = result ? isNumericSeries(result.points) : false;
  const chartOption = useMemo(() => {
    if (!result || !numeric) return null;
    return {
      tooltip: { trigger: 'axis' },
      grid: { left: 56, right: 24, top: 24, bottom: 64 },
      xAxis: { type: 'time' },
      yAxis: { type: 'value', scale: true },
      dataZoom: [{ type: 'inside' }, { type: 'slider' }],
      series: [{
        name: `${result.device_id} / ${result.key}`,
        type: 'line',
        showSymbol: result.points.length < 100,
        connectNulls: false,
        data: toChartData(result.points),
      }],
    };
  }, [result, numeric]);

  if (status && !status.enabled) {
    return (
      <Card title="历史数据">
        <Alert
          type="info"
          showIcon
          message="本地暂存未开启"
          description="在 config.yaml 中设置 storage.enabled: true 后重启网关，即可在本地保存近期采集数据并在此查询。"
        />
      </Card>
    );
  }

  const stream = status?.stream;
  const sinkColumns = [
    { title: '连接器', dataIndex: 'name' },
    {
      title: '状态', dataIndex: 'state',
      render: (s: string, r: BufferedSinkStats) => {
        const info = sinkStateInfo(s);
        return <Tooltip title={r.last_error}><Tag color={info.color}>{info.text}</Tag></Tooltip>;
      },
    },
    { title: '积压', dataIndex: 'pending', render: (n: number) => n.toLocaleString() },
    { title: '积压时长', dataIndex: 'lag_seconds', render: (n: number) => formatLag(n) },
    { title: '已写入', dataIndex: 'delivered_points', render: (n: number) => n.toLocaleString() },
    {
      title: '因淘汰丢失', dataIndex: 'lost',
      render: (n: number) => (n > 0 ? <Text type="danger">{n.toLocaleString()}</Text> : 0),
    },
    ...(isAdmin ? [{
      title: '操作', key: 'ops',
      render: (_: unknown, r: BufferedSinkStats) => (
        <Space size="small" wrap>
          {r.state === 'paused'
            ? <Button size="small" onClick={() => control(r.name, 'resume')}>恢复</Button>
            : <Button size="small" onClick={() => control(r.name, 'pause')}>暂停</Button>}
          <Popconfirm title="丢弃全部积压数据？这些数据将不会写入目标。" onConfirm={() => control(r.name, 'skip')}>
            <Button size="small" danger>跳过积压</Button>
          </Popconfirm>
          <Popconfirm
            title="从所选时间范围的起点重新投递？目标中会出现重复数据。"
            onConfirm={() => control(r.name, 'replay', range[0].toDate())}
          >
            <Button size="small">从 {range[0].format('MM-DD HH:mm')} 重放</Button>
          </Popconfirm>
        </Space>
      ),
    }] : []),
  ];

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card
        title="本地存储"
        extra={<Button icon={<ReloadOutlined />} onClick={loadStatus}>刷新</Button>}
      >
        {status?.error && <Alert type="error" showIcon message={status.error} style={{ marginBottom: 16 }} />}
        {status?.alerts?.map((a) => (
          <Alert
            key={`${a.kind}-${a.sink ?? ''}`}
            type={a.level === 'critical' ? 'error' : 'warning'}
            showIcon
            message={a.message}
            style={{ marginBottom: 8 }}
          />
        ))}
        <Row gutter={[16, 16]}>
          <Col xs={12} md={6}><Statistic title="消息数" value={stream?.messages ?? 0} /></Col>
          <Col xs={12} md={6}><Statistic title="占用" value={formatBytes(stream?.bytes ?? 0)} /></Col>
          <Col xs={12} md={6}>
            <div style={{ color: 'rgba(0,0,0,0.45)', marginBottom: 4 }}>容量使用</div>
            <Progress percent={Math.round((stream?.usage_ratio ?? 0) * 100)} status={(stream?.usage_ratio ?? 0) > 0.9 ? 'exception' : 'normal'} />
          </Col>
          <Col xs={12} md={6}>
            <Statistic title="最早数据" value={stream?.first_time && stream.messages > 0 ? dayjs(stream.first_time).format('MM-DD HH:mm:ss') : '-'} />
          </Col>
        </Row>
        {status?.rollup && (
          <Space wrap style={{ marginTop: 16 }}>
            <Text type="secondary">汇总层（每 {status.rollup.interval}）：</Text>
            <Tag>记录 {status.rollup.records.toLocaleString()}</Tag>
            <Tag>占用 {formatBytes(status.rollup.bytes)}</Tag>
            <Tag>未关闭窗口 {status.rollup.open_windows}</Tag>
            {status.rollup.late_points > 0 && <Tag color="orange">迟到点 {status.rollup.late_points.toLocaleString()}</Tag>}
            {status.rollup.last_error && <Tag color="red">{status.rollup.last_error}</Tag>}
          </Space>
        )}
        {(status?.sinks?.length ?? 0) > 0 && (
          <Table
            style={{ marginTop: 16 }}
            size="small"
            rowKey="name"
            pagination={false}
            columns={sinkColumns}
            dataSource={status?.sinks}
            scroll={{ x: true }}
          />
        )}
      </Card>

      <Card title="历史数据">
        <Space wrap style={{ marginBottom: 16 }}>
          <Select
            showSearch
            placeholder="设备"
            style={{ width: 200 }}
            value={device}
            onChange={(v) => { setDevice(v); setKey(undefined); setResult(null); }}
            options={devices.map((d) => ({ value: d, label: d }))}
          />
          <Select
            showSearch
            placeholder="点位"
            style={{ width: 180 }}
            value={key}
            onChange={setKey}
            disabled={!device}
            options={keys.map((k) => ({ value: k, label: k }))}
          />
          <RangePicker
            showTime
            value={range}
            presets={RANGE_PRESETS}
            onChange={(v) => { if (v?.[0] && v?.[1]) setRange([v[0], v[1]]); }}
          />
          <Select value={agg} onChange={setAgg} style={{ width: 130 }} options={AGG_OPTIONS} />
          <Select
            value={tier}
            onChange={setTier}
            style={{ width: 150 }}
            options={TIER_OPTIONS.map((o) => ({ ...o, disabled: o.value === 'rollup' && !status?.rollup }))}
          />
          <Tooltip title="最多返回的点数，超出时在网关端降采样">
            <InputNumber min={10} max={10000} step={100} value={limit} onChange={(v) => setLimit(v ?? 1000)} addonBefore="点数" style={{ width: 150 }} />
          </Tooltip>
          <Button type="primary" icon={<SearchOutlined />} loading={loading} onClick={query}>查询</Button>
          <Button icon={<DownloadOutlined />} loading={exporting} disabled={!device || !key} onClick={exportCsv}>导出 CSV</Button>
        </Space>

        {result && (
          <Space wrap style={{ marginBottom: 12 }}>
            <Tag color={result.tier === 'rollup' ? 'purple' : 'default'}>{tierLabel(result.tier)}</Tag>
            <Tag>原始点 {result.matched.toLocaleString()}</Tag>
            <Tag>返回 {result.points.length.toLocaleString()}</Tag>
            {result.downsampled && <Tag color="blue">已降采样（{result.agg}）</Tag>}
            {result.truncated && <Tag color="orange">结果不完整：超过扫描上限或超时，请缩小时间范围</Tag>}
          </Space>
        )}

        {!result && <Empty description="选择设备、点位和时间范围后查询" />}
        {result && result.points.length === 0 && <Empty description="该时间范围内没有数据" />}
        {result && result.points.length > 0 && chartOption && (
          <ReactECharts option={chartOption} style={{ height: 380 }} notMerge />
        )}
        {result && result.points.length > 0 && !numeric && (
          <Table
            size="small"
            rowKey={(r) => r.t}
            dataSource={result.points}
            pagination={{ pageSize: 20 }}
            columns={[
              { title: '时间', dataIndex: 't', render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm:ss.SSS') },
              { title: '值', dataIndex: 'v', render: (v: unknown) => formatPointValue(v) },
              { title: '质量码', dataIndex: 'q', render: (q?: number) => q ?? 0 },
            ]}
          />
        )}
      </Card>

      {device && latest.length > 0 && (
        <Card title={`${device} 的最新值`}>
          <Table
            size="small"
            rowKey="key"
            pagination={false}
            dataSource={latest}
            onRow={(r) => ({ onClick: () => setKey(r.key), style: { cursor: 'pointer' } })}
            columns={[
              { title: '点位', dataIndex: 'key' },
              { title: '值', dataIndex: 'value', render: (v: unknown) => formatPointValue(v) },
              { title: '采集时间', dataIndex: 'timestamp', render: (t: string) => dayjs(t).format('YYYY-MM-DD HH:mm:ss') },
              { title: '质量码', dataIndex: 'quality' },
            ]}
          />
        </Card>
      )}
    </Space>
  );
};

export default HistoryPage;
