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
    assert.equal(figures.length, 6);
    const referenced = new Set();
    for (const [figure] of figures) {
      const img = figure.match(/<img[^>]*>/)[0];
      assert.match(img, /loading="lazy"/);
      assert.match(img, /decoding="async"/);
      // 9:16 captures (1080 x 1920 masters): the frame reserves the exact box.
      assert.match(img, /width="360" height="640"/);
      const alt = img.match(/alt="([^"]+)"/);
      assert.ok(alt && alt[1].length > 40, 'meaningful alt text required');
      assert.doesNotMatch(img, /\.png/);
      for (const src of [...img.matchAll(/\/assets\/app\/([a-z0-9-]+\.webp)/g)]) {
        referenced.add(src[1]);
      }
      assert.match(figure, /<figcaption>.*<\/figcaption>/s);
    }
    // Three widths per capture, minus the one map capture whose 1080w file
    // would exceed the per-file budget and is not shipped.
    assert.ok(referenced.size >= 17, 'srcset widths must be referenced');
    // Every emitted reference resolves to a built asset (missing media fails loudly here).
    for (const file of referenced) {
      assert.ok(fs.existsSync(path.join(outDir, 'assets', 'app', file)), `missing emitted asset: ${file}`);
    }
    assert.doesNotMatch(gallery[0], /autoplay|carousel/);
    // The section head is the title alone: no intro line and no app version.
    const section = indexHtml.match(/<section id="aplicativo"[\s\S]*?<\/section>/)[0];
    assert.match(section, /<div class="block-head">\s*<h2 class="h2">O aplicativo em telas reais<\/h2>\s*<\/div>/);
    assert.doesNotMatch(section, /3\.1\.0/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
