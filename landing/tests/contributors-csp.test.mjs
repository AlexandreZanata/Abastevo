import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function buildTmp(apiOrigin) {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-contribuidores-csp-'));
  const previous = process.env.ABASTEVO_API_ORIGIN;
  try {
    if (apiOrigin === undefined) {
      delete process.env.ABASTEVO_API_ORIGIN;
    } else {
      process.env.ABASTEVO_API_ORIGIN = apiOrigin;
    }
    build({ projectRoot: root, outDir });
    return outDir;
  } finally {
    if (previous === undefined) {
      delete process.env.ABASTEVO_API_ORIGIN;
    } else {
      process.env.ABASTEVO_API_ORIGIN = previous;
    }
  }
}

function blockFor(headers, route) {
  const lines = headers.split('\n');
  const start = lines.findIndex((line) => line.trim() === route);
  assert.ok(start >= 0, `missing ${route} block`);
  const block = [];
  for (let i = start + 1; i < lines.length; i++) {
    if (/^\S/.test(lines[i]) && lines[i].trim() !== '') break;
    block.push(lines[i]);
  }
  return block.join('\n');
}

test('csp: API origin appears only on the contribuidores path', () => {
  const outDir = buildTmp('https://api.example.invalid');
  try {
    const headers = fs.readFileSync(path.join(outDir, '_headers'), 'utf8');
    const scoped = blockFor(headers, '/contribuidores/*');
    assert.match(scoped, /connect-src 'self' https:\/\/api\.example\.invalid;/);
    let inScopedBlock = false;
    for (const line of headers.split('\n')) {
      if (line.trim() === '/contribuidores/*') {
        inScopedBlock = true;
        continue;
      }
      if (/^\S/.test(line)) {
        inScopedBlock = false;
      }
      if (inScopedBlock || line.startsWith('/') || line.trim() === '' || line.startsWith('#')) continue;
      assert.doesNotMatch(line, /api\.example\.invalid/, `leaked origin: ${line}`);
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('csp: disabled build keeps a self-only connect-src on that path', () => {
  const outDir = buildTmp(undefined);
  try {
    const headers = fs.readFileSync(path.join(outDir, '_headers'), 'utf8');
    const scoped = blockFor(headers, '/contribuidores/*');
    assert.match(scoped, /connect-src 'self';/);
    assert.doesNotMatch(headers, /example\.invalid/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
