import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function definedTokens(css) {
  const names = new Set();
  for (const match of css.matchAll(/--([a-zA-Z0-9-]+)\s*:/g)) {
    names.add(match[1]);
  }
  return names;
}

function referencedTokens(css) {
  const names = new Set();
  for (const match of css.matchAll(/var\(\s*--([a-zA-Z0-9-]+)/g)) {
    names.add(match[1]);
  }
  return names;
}

test('admin.css: only existing landing tokens, no new custom properties', () => {
  const landingCss = fs.readFileSync(path.join(root, '..', 'landing', 'static', 'assets', 'styles.css'), 'utf8');
  const adminCss = fs.readFileSync(path.join(root, 'static', 'assets', 'admin.css'), 'utf8');
  const landingTokens = definedTokens(landingCss);
  assert.ok(landingTokens.size > 0, 'landing tokens found');
  const adminDefined = [...definedTokens(adminCss)];
  assert.deepEqual(adminDefined, [], `admin.css must not define new tokens: ${adminDefined.join(', ')}`);
  const unknown = [...referencedTokens(adminCss)].filter((name) => !landingTokens.has(name));
  assert.deepEqual(unknown, [], `admin.css references unknown tokens: ${unknown.join(', ')}`);
});
