import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { checkDist } from '../scripts/check.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('check: fresh build passes the dist gate', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-check-'));
  try {
    build({ projectRoot: root, outDir });
    const result = checkDist({ projectRoot: root, distDir: outDir });
    assert.equal(result.ok, true);
    assert.ok(result.cssFile.startsWith('styles.'));
    assert.ok(result.jsFiles.some((f) => f.startsWith('app.')));
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
