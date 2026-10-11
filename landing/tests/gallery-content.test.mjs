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

test('app gallery uses authentic captures with alt, dimensions and lazy loading', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-gallery-'));
  try {
    build({ projectRoot: root, outDir });
    const indexHtml = fs.readFileSync(path.join(outDir, 'index.html'), 'utf8');
    const gallery = indexHtml.match(/<div class="app-gallery"[\s\S]*?<\/div>\s*<div class="grid-2">/);
    assert.ok(gallery, 'app gallery section must exist before the status/FAQ grid');
    const figures = [...gallery[0].matchAll(/<figure class="app-shot"[\s\S]*?<\/figure>/g)];
    assert.equal(figures.length, 3);
    const referenced = new Set();
    for (const [figure] of figures) {
      const img = figure.match(/<img[^>]*>/)[0];
      assert.match(img, /loading="lazy"/);
      assert.match(img, /decoding="async"/);
      assert.match(img, /width="\d+" height="\d+"/);
      const alt = img.match(/alt="([^"]+)"/);
      assert.ok(alt && alt[1].length > 40, 'meaningful alt text required');
      assert.doesNotMatch(img, /\.png/);
      for (const src of [...img.matchAll(/\/assets\/app\/([a-z0-9-]+\.webp)/g)]) {
        referenced.add(src[1]);
      }
      assert.match(figure, /<figcaption>.*<\/figcaption>/s);
    }
    assert.ok(referenced.size >= 9, 'srcset widths must be referenced');
    // Every emitted reference resolves to a built asset (missing media fails loudly here).
    for (const file of referenced) {
      assert.ok(fs.existsSync(path.join(outDir, 'assets', 'app', file)), `missing emitted asset: ${file}`);
    }
    assert.doesNotMatch(gallery[0], /autoplay|carousel/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
