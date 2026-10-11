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

function emittedHtml(outDir) {
  const pages = [];
  function walk(rel) {
    const abs = path.join(outDir, rel);
    for (const entry of fs.readdirSync(abs, { withFileTypes: true })) {
      const entryRel = path.join(rel, entry.name);
      if (entry.isDirectory()) {
        walk(entryRel);
      } else if (entry.name.endsWith('.html') && entry.name !== '404.html') {
        pages.push(entryRel);
      }
    }
  }
  walk('');
  return pages;
}

test('plain anchors, headings and keyboard affordances hold on every page', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-nav-'));
  try {
    build({ projectRoot: root, outDir });
    const pages = emittedHtml(outDir);
    assert.ok(pages.length >= 20, `expected all emitted pages, got ${pages.length}`);

    for (const rel of pages) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');

      // 1. Skip link targets the main landmark.
      assert.match(html, /href="#conteudo"/, `${rel} skip link`);
      assert.match(html, /<main id="conteudo"/, `${rel} main landmark`);

      // 2. Every in-page anchor resolves (keyboard/screen-reader traversal intact).
      for (const href of [...html.matchAll(/href="#([^"]+)"/g)].map((m) => m[1])) {
        assert.ok(html.includes(`id="${href}"`), `${rel} dead anchor #${href}`);
      }

      // 3. Exactly one h1; no skipped heading levels.
      assert.equal((html.match(/<h1[\s>]/g) || []).length, 1, `${rel} single h1`);
      const levels = [...html.matchAll(/<h([1-6])[\s>]/g)].map((m) => Number(m[1]));
      for (let i = 1; i < levels.length; i++) {
        assert.ok(levels[i] - levels[i - 1] <= 1, `${rel} skipped heading level`);
      }

      // 4. Interactive disclosure stays native (no JS-only widgets).
      assert.doesNotMatch(html, /role="button"/, `${rel} div-button`);
    }

    // 5. Visible focus ring survives in the shipped stylesheet.
    const cssFile = fs.readdirSync(path.join(outDir, 'assets')).find((f) => /^styles\.[a-f0-9]{8}\.css$/.test(f));
    const css = fs.readFileSync(path.join(outDir, 'assets', cssFile), 'utf8');
    assert.match(css, /:focus-visible/);
    assert.match(css, /outline:\s*3px/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
