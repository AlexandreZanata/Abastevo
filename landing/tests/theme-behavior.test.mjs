import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build } from '../scripts/build.mjs';

const root = path.resolve(import.meta.dirname, '..');

function emittedHtml(dir, rel = '') {
  return fs.readdirSync(path.join(dir, rel), { withFileTypes: true }).flatMap((entry) => {
    const child = path.join(rel, entry.name);
    if (entry.isDirectory()) return emittedHtml(dir, child);
    return entry.name.endsWith('.html') ? [child] : [];
  });
}

test('the site ships a single light theme: no toggle, no dark palette, no theme script', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-theme-'));
  try {
    build({ projectRoot: root, outDir });
    for (const rel of emittedHtml(outDir)) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
      assert.doesNotMatch(html, /data-theme-toggle|theme-toggle|Ativar tema/, `${rel} theme toggle`);
    }
    const assets = fs.readdirSync(path.join(outDir, 'assets'));
    const css = fs.readFileSync(path.join(outDir, 'assets', assets.find((f) => /^styles\.[a-f0-9]{8}\.css$/.test(f))), 'utf8');
    assert.match(css, /:root\s*\{[^}]*color-scheme:\s*light/);
    assert.doesNotMatch(css, /data-theme|prefers-color-scheme:\s*dark|color-scheme:\s*dark/);
    const js = fs.readFileSync(path.join(outDir, 'assets', assets.find((f) => /^main\.[a-f0-9]{8}\.js$/.test(f))), 'utf8');
    assert.doesNotMatch(js, /abastevo-theme|prefers-color-scheme|dataset\.theme/);
  } finally { fs.rmSync(outDir, { recursive: true, force: true }); }
});

test('the portal stylesheet has no dark palette', () => {
  const css = fs.readFileSync(path.join(root, '../backend/internal/modules/enterpriseexport/portalgen/assets/portal.css'), 'utf8');
  assert.match(css, /:root\s*\{[^}]*color-scheme:\s*light/);
  assert.doesNotMatch(css, /prefers-color-scheme:\s*dark|color-scheme:\s*dark|data-theme/);
});
