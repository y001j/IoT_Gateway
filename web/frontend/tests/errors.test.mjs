import { test } from 'node:test';
import assert from 'node:assert/strict';
import { asHttpError, errorMessage } from '../.test-build/errors.mjs';

test('asHttpError 规范各种抛出值', () => {
  const e = new Error('boom');
  assert.equal(asHttpError(e), e);
  assert.equal(asHttpError('text').message, 'text');
  assert.equal(asHttpError(undefined).message, 'unknown error');
  assert.equal(asHttpError(42).message, '42');
  const obj = asHttpError({ message: 'm', code: 'E1', response: { status: 500 } });
  assert.equal(obj.message, 'm');
  assert.equal(obj.code, 'E1');
  assert.equal(obj.response.status, 500);
  assert.equal(asHttpError({ a: 1 }).message, '{"a":1}');
});

test('errorMessage 优先使用后端返回的信息', () => {
  const axiosLike = Object.assign(new Error('Request failed with status code 400'), { response: { data: { message: '名称不能为空' } } });
  assert.equal(errorMessage(axiosLike), '名称不能为空');
  const withError = Object.assign(new Error('x'), { response: { data: { error: '未授权' } } });
  assert.equal(errorMessage(withError), '未授权');
  assert.equal(errorMessage(new Error('plain')), 'plain');
  assert.equal(errorMessage(Object.assign(new Error(''), {}), '默认'), '默认');
});
