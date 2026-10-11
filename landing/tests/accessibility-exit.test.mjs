import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

function shippedCss(outDir) {
  const cssFile = fs.readdirSync(path.join(outDir, 'assets')).find((f) => /^styles\.[a-f0-9]{8}\.css$/.test(f));
  assert.ok(cssFile, 'fingerprinted stylesheet must ship');
  return fs.readFileSync(path.join(outDir, 'assets', cssFile), 'utf8');
}

test('accessibility exit: touch targets meet 44px and motion stays optional', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-a11y-'));
  try {
    build({ projectRoot: root, outDir });
    const css = shippedCss(outDir);

    // 1. Primary controls declare a minimum 44px hit area.
    for (const selector of ['.site-nav a', '.btn-primary', '.btn-secondary', '.store-link', '.brand-link']) {
      const rule = css.match(new RegExp(`${selector.replace(/[./]/g, (c) => `\\${c}`)}\\s*\\{[^}]*\\}`));
      assert.ok(rule, `${selector} rule must ship`);
      const minHeight = rule[0].match(/min-height:\s*(\d+)px/);
      assert.ok(minHeight && Number(minHeight[1]) >= 44, `${selector} hit area must be >= 44px`);
    }

    // 2. Smooth scrolling yields to reduced-motion preferences.
    assert.match(css, /@media\s*\(prefers-reduced-motion:\s*reduce\)/);
    assert.match(css, /scroll-behavior:\s*auto/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
