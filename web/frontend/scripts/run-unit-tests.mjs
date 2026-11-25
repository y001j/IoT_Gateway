#!/usr/bin/env node
// 前端纯函数单元测试运行器（不依赖额外的测试框架）。
//
// 1. 把 TEST_SOURCES 中的 TypeScript 源文件转译为 .test-build/*.mjs
//    （优先使用项目的 typescript 依赖；未安装时使用 Node >= 22.13 内置的 stripTypeScriptTypes）
// 2. 用 node:test 运行 tests/*.test.mjs
//
// 兼容 CI 中的调用方式 `npm test -- --coverage --watchAll=false`：多余的参数会被忽略。
import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const buildDir = path.join(root, '.test-build');
const TEST_SOURCES = ['src/utils/format.ts', 'src/utils/history.ts', 'src/utils/errors.ts', 'src/utils/ruleData.ts'];

async function transpiler() {
  const require = createRequire(path.join(root, 'package.json'));
  try {
    const ts = require('typescript');
    return (code, file) =>
      ts.transpileModule(code, {
        fileName: file,
        compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 },
      }).outputText;
  } catch {
    const mod = await import('node:module');
    if (typeof mod.stripTypeScriptTypes === 'function') {
      return (code) => mod.stripTypeScriptTypes(code, { mode: 'transform' });
    }
    console.error('无法转译 TypeScript：请先执行 npm install（需要 typescript 依赖）');
    process.exit(1);
  }
}

const transpile = await transpiler();
rmSync(buildDir, { recursive: true, force: true });
mkdirSync(buildDir, { recursive: true });
for (const src of TEST_SOURCES) {
  const file = path.join(root, src);
  const out = path.join(buildDir, path.basename(src).replace(/\.ts$/, '.mjs'));
  writeFileSync(out, transpile(readFileSync(file, 'utf8'), file));
}

const testDir = path.join(root, 'tests');
const tests = readdirSync(testDir)
  .filter((f) => f.endsWith('.test.mjs'))
  .map((f) => path.join(testDir, f));
const result = spawnSync(process.execPath, ['--test', ...tests], { stdio: 'inherit' });
process.exit(result.status ?? 1);
