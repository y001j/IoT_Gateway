import { test } from 'node:test';
import assert from 'node:assert/strict';
import { formatBytes, formatLatency, formatNumber, formatUptime } from '../.test-build/format.mjs';

test('formatBytes', () => {
  assert.equal(formatBytes(0), '0 B');
  assert.equal(formatBytes(512), '512 B');
  assert.equal(formatBytes(1024), '1 KB');
  assert.equal(formatBytes(1536), '1.5 KB');
  assert.equal(formatBytes(5 * 1024 ** 3), '5 GB');
  // 回归：小于 1 / 负数 / 非数字以前返回 "NaN undefined"
  assert.equal(formatBytes(0.5), '0.5 B');
  assert.equal(formatBytes(-10), '0 B');
  assert.equal(formatBytes(Number.NaN), '0 B');
  // 回归：超过 TB 的单位以前是 undefined
  assert.equal(formatBytes(2 * 1024 ** 5), '2 PB');
  assert.ok(!formatBytes(1024 ** 9).includes('undefined'));
});

test('formatUptime', () => {
  assert.equal(formatUptime(42.9), '42秒');
  assert.equal(formatUptime(125), '2分钟');
  assert.equal(formatUptime(3 * 3600 + 15 * 60), '3小时15分钟');
  assert.equal(formatUptime(2 * 86400 + 5 * 3600), '2天5小时');
  assert.equal(formatUptime(-1), '0秒');
});

test('formatNumber', () => {
  assert.equal(formatNumber(12.345), '12.3');
  assert.equal(formatNumber(1500), '1.5K');
  assert.equal(formatNumber(2500000, 2), '2.50M');
  assert.equal(formatNumber(-1500), '-1.5K');
  assert.equal(formatNumber(Number.POSITIVE_INFINITY), '0');
});

test('formatLatency', () => {
  assert.equal(formatLatency(0.25), '250μs');
  assert.equal(formatLatency(12.34), '12.3ms');
  assert.equal(formatLatency(2500), '2.50s');
  assert.equal(formatLatency(-5), '0μs');
});
