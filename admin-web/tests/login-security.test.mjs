import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('login: password discipline in markup and submit handler', () => {
  const html = fs.readFileSync(path.join(root, 'static', 'app', 'index.html'), 'utf8');
  assert.match(html, /<input[^>]*id="admin-key"[^>]*type="password"[^>]*>/);
  assert.match(html, /<input[^>]*id="admin-key"[^>]*autocomplete="off"[^>]*>/);
  const app = fs.readFileSync(path.join(root, 'src', 'app.ts'), 'utf8');
  assert.match(app, /keyInput\.value = ''/, 'password cleared right after submit');
  assert.doesNotMatch(app, /localStorage|sessionStorage/);
});
