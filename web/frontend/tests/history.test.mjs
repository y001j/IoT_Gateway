import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  buildHistoryParams, toNumber, isNumericSeries, toChartData, formatPointValue, sinkStateInfo, formatLag, tierLabel,
} from '../.test-build/history.mjs';

test('buildHistoryParams 转义参数并使用 RFC3339 时间', () => {
  const start = new Date('2026-11-24T00:00:00Z');
  const qs = buildHistoryParams({ deviceId: 'plc.1 a&b', key: 'temp', start, end: null, limit: 500.7, agg: 'max' });
  const p = new URLSearchParams(qs);
  assert.equal(p.get('device_id'), 'plc.1 a&b');
  assert.equal(p.get('start'), '2026-11-24T00:00:00.000Z');
  assert.equal(p.get('end'), null);
  assert.equal(p.get('limit'), '500');
  assert.equal(p.get('agg'), 'max');
  assert.ok(!qs.includes('+00:00'));
  assert.equal(new URLSearchParams(buildHistoryParams({ deviceId: 'd', key: 'k', limit: 0 })).get('limit'), null);
  // auto 是默认值，不传；raw/rollup 原样传递
  assert.equal(new URLSearchParams(buildHistoryParams({ deviceId: 'd', key: 'k', tier: 'auto' })).get('tier'), null);
  assert.equal(new URLSearchParams(buildHistoryParams({ deviceId: 'd', key: 'k', tier: 'rollup' })).get('tier'), 'rollup');
});

test('tierLabel', () => {
  assert.equal(tierLabel('rollup'), '汇总层');
  assert.equal(tierLabel('raw'), '原始数据');
  assert.equal(tierLabel(''), '-');
  assert.equal(tierLabel('x'), 'x');
});

test('toNumber / isNumericSeries / toChartData', () => {
  assert.equal(toNumber(1.5), 1.5);
  assert.equal(toNumber(true), 1);
  assert.equal(toNumber(false), 0);
  assert.equal(toNumber('2.5'), 2.5);
  assert.equal(toNumber(''), null);
  assert.equal(toNumber('abc'), null);
  assert.equal(toNumber(Number.NaN), null);
  assert.equal(toNumber({ x: 1 }), null);
  assert.equal(isNumericSeries([{ t: '', v: 'on' }]), false);
  assert.equal(isNumericSeries([{ t: '', v: 'on' }, { t: '', v: 3 }]), true);
  const data = toChartData([{ t: '2026-11-24T00:00:00Z', v: 3 }, { t: '2026-11-24T00:00:01Z', v: 'x' }]);
  assert.deepEqual(data, [[Date.parse('2026-11-24T00:00:00Z'), 3], [Date.parse('2026-11-24T00:00:01Z'), null]]);
});

test('formatPointValue', () => {
  assert.equal(formatPointValue(null), '');
  assert.equal(formatPointValue(undefined), '');
  assert.equal(formatPointValue({ x: 1 }), '{"x":1}');
  assert.equal(formatPointValue(true), 'true');
  const cyclic = {};
  cyclic.self = cyclic;
  assert.equal(formatPointValue(cyclic), '[object Object]');
});

test('sinkStateInfo / formatLag', () => {
  assert.deepEqual(sinkStateInfo('buffering'), { text: '缓冲中', color: 'orange' });
  assert.equal(sinkStateInfo('realtime').text, '实时');
  assert.equal(sinkStateInfo('replaying').text, '补发中');
  assert.equal(sinkStateInfo('paused').text, '已暂停');
  assert.equal(sinkStateInfo('stopped').text, '已停止');
  assert.equal(sinkStateInfo('').text, '未知');
  assert.equal(sinkStateInfo('weird').text, 'weird');
  assert.equal(formatLag(0), '-');
  assert.equal(formatLag(Number.NaN), '-');
  assert.equal(formatLag(30), '30 秒');
  assert.equal(formatLag(600), '10 分钟');
  assert.equal(formatLag(7200), '2.0 小时');
  assert.equal(formatLag(172800), '2.0 天');
});
