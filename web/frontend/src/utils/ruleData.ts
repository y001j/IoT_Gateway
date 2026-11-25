// 规则配置相关的纯函数：规则和动作的配置是自由格式 JSON（类型为 unknown），读取时在这里统一转换。

/** 规则 data_type 字段可以是字符串，也可以是 {type/name: ...} 形式的详细定义 */
export function dataTypeName(dt: unknown): string {
  if (typeof dt === 'string') return dt;
  if (dt && typeof dt === 'object') {
    const def = dt as { type?: unknown; name?: unknown };
    if (typeof def.type === 'string') return def.type;
    if (typeof def.name === 'string') return def.name;
  }
  return '';
}

/** 把配置值转换为数字，无法转换时返回默认值 */
export function cfgNumber(v: unknown, fallback = 0): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '' && Number.isFinite(Number(v))) return Number(v);
  return fallback;
}

/** 把配置值转换为字符串，非字符串返回默认值 */
export function cfgString(v: unknown, fallback = ''): string {
  return typeof v === 'string' ? v : fallback;
}

/** 配置值是否为给定的枚举成员之一 */
export function cfgEnum<T extends string>(v: unknown, allowed: readonly T[], fallback: T): T {
  return typeof v === 'string' && (allowed as readonly string[]).includes(v) ? (v as T) : fallback;
}

/** 把配置值转换为对象，非对象返回空对象 */
export function cfgObject(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

/** 数字输入框的值：是数字（或数字字符串）时返回数字，否则返回 undefined（显示为空） */
export function cfgOptNumber(v: unknown): number | undefined {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '' && Number.isFinite(Number(v))) return Number(v);
  return undefined;
}
