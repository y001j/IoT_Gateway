/**
 * 纯函数形式的格式化工具（不依赖 React / antd，可在 Node 中直接单元测试）。
 * 测试：npm test（见 scripts/run-unit-tests.mjs 与 tests/*.test.mjs）
 */

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB'];

/** 格式化运行时间（秒） */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    seconds = 0;
  }
  if (seconds < 60) {
    return `${Math.floor(seconds)}秒`;
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}分钟`;
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return `${hours}小时${minutes}分钟`;
  }
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  return `${days}天${hours}小时`;
}

/**
 * 格式化字节数。
 * 以前小于 1 的值会得到负数下标（返回 "NaN undefined"），超过 TB 的值单位为 undefined。
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }
  const k = 1024;
  let i = Math.floor(Math.log(bytes) / Math.log(k));
  i = Math.max(0, Math.min(i, BYTE_UNITS.length - 1));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${BYTE_UNITS[i]}`;
}

/** 格式化数字（K / M 缩写） */
export function formatNumber(num: number, decimals: number = 1): string {
  if (!Number.isFinite(num)) {
    return '0';
  }
  const abs = Math.abs(num);
  if (abs >= 1000000) {
    return `${(num / 1000000).toFixed(decimals)}M`;
  }
  if (abs >= 1000) {
    return `${(num / 1000).toFixed(decimals)}K`;
  }
  return num.toFixed(decimals);
}

/** 格式化延迟（毫秒） */
export function formatLatency(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) {
    ms = 0;
  }
  if (ms < 1) {
    return `${(ms * 1000).toFixed(0)}μs`;
  }
  if (ms < 1000) {
    return `${ms.toFixed(1)}ms`;
  }
  return `${(ms / 1000).toFixed(2)}s`;
}
