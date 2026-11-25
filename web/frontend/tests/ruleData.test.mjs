import { test } from 'node:test';
import assert from 'node:assert/strict';
import { dataTypeName, cfgNumber, cfgString, cfgEnum, cfgObject, cfgOptNumber } from '../.test-build/ruleData.mjs';

test('dataTypeName', () => {
  assert.equal(dataTypeName('location'), 'location');
  assert.equal(dataTypeName({ type: 'vector3d' }), 'vector3d');
  assert.equal(dataTypeName({ name: 'color' }), 'color');
  assert.equal(dataTypeName({ type: 1 }), '');
  assert.equal(dataTypeName(undefined), '');
  assert.equal(dataTypeName(null), '');
});

test('cfgNumber / cfgString / cfgEnum / cfgObject', () => {
  assert.equal(cfgNumber(3), 3);
  assert.equal(cfgNumber('2.5'), 2.5);
  assert.equal(cfgNumber('x', 7), 7);
  assert.equal(cfgNumber('', 1), 1);
  assert.equal(cfgNumber(Number.NaN, 4), 4);
  assert.equal(cfgString('a'), 'a');
  assert.equal(cfgString(1, 'd'), 'd');
  assert.equal(cfgEnum('x', ['x', 'y'], 'y'), 'x');
  assert.equal(cfgEnum('z', ['x', 'y'], 'y'), 'y');
  assert.deepEqual(cfgObject({ a: 1 }), { a: 1 });
  assert.deepEqual(cfgObject([1]), {});
  assert.deepEqual(cfgObject(null), {});
});

test('cfgOptNumber', () => {
  assert.equal(cfgOptNumber(1.5), 1.5);
  assert.equal(cfgOptNumber('3'), 3);
  assert.equal(cfgOptNumber(''), undefined);
  assert.equal(cfgOptNumber('x'), undefined);
  assert.equal(cfgOptNumber(undefined), undefined);
});
