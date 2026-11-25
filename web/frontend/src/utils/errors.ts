// 统一的错误类型：catch 到的值是 unknown，这里规范成带 message 的错误对象，
// 并描述 axios 等 HTTP 客户端错误上常用的字段。

export interface HttpErrorResponse {
  status?: number;
  statusText?: string;
  data?: { message?: string; error?: string; [field: string]: unknown };
  headers?: Record<string, unknown>;
}

export interface HttpError extends Error {
  code?: string;
  response?: HttpErrorResponse;
  request?: { readyState?: number; status?: number; statusText?: string; responseURL?: string };
  config?: { url?: string; method?: string; baseURL?: string; timeout?: number; headers?: Record<string, unknown> };
}

/** 把任意抛出值转换为 HttpError（非 Error 值会被包装，保证 message 可用） */
export function asHttpError(caught: unknown): HttpError {
  if (caught instanceof Error) return caught as HttpError;
  if (caught && typeof caught === 'object') {
    const obj = caught as { message?: unknown };
    const err = new Error(typeof obj.message === 'string' ? obj.message : JSON.stringify(caught)) as HttpError;
    Object.assign(err, caught);
    return err;
  }
  return new Error(caught === undefined ? 'unknown error' : String(caught)) as HttpError;
}

/** 提取适合展示给用户的错误信息：优先使用后端返回的 message/error */
export function errorMessage(caught: unknown, fallback = '操作失败'): string {
  const err = asHttpError(caught);
  return err.response?.data?.message || err.response?.data?.error || err.message || fallback;
}
