import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('progressive enhancement: optional interaction elements exist with HTML fallbacks', () => {
  const tmpOut = path.join(root, '.test-dist-pe-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut });
    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');

    // 1. Copy button has data-copy-target pointing to real anchor #btn-github
    assert.match(indexHtml, /<button[^>]*data-copy-target="#btn-github"[^>]*>Copiar link<\/button>/);
    assert.match(indexHtml, /id="btn-github"/);

    // 2. Back to top button has data-back-to-top and hidden attribute (graceful fallback: doesn't show without JS)
    assert.match(indexHtml, /<button[^>]*data-back-to-top[^>]*hidden/);

    // 3. Skip link for keyboard navigation exists at the very top of body
    assert.match(indexHtml, /<a href="#conteudo" class="skip-link">Pular para o conteúdo<\/a>/);

    // 4. FAQ uses native <details> and <summary> so it opens without JS
    assert.match(indexHtml, /<details class="faq-item">[\s\S]*?<summary class="faq-summary">/);

    // 5. Section anchors match in-page nav targets
    const navAnchors = Array.from(indexHtml.matchAll(/<nav class="site-nav"[^>]*>([\s\S]*?)<\/nav>/g))[0][1];
    const targetIds = Array.from(navAnchors.matchAll(/href="#([^"]+)"/g)).map((m) => m[1]);
    assert.ok(targetIds.length >= 3, 'Expected at least 3 nav anchors');
    for (const id of targetIds) {
      assert.match(indexHtml, new RegExp(`id="${id}"`), `Nav anchor #${id} must exist in document`);
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('visual tokens: contrast ratios meet WCAG AA standards', () => {
  // WCAG relative luminance formula
  function luminance(hex) {
    const rgb = [
      parseInt(hex.slice(1, 3), 16) / 255,
      parseInt(hex.slice(3, 5), 16) / 255,
      parseInt(hex.slice(5, 7), 16) / 255,
    ].map((c) => (c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)));
    return 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
  }

  function contrast(hex1, hex2) {
    const l1 = luminance(hex1);
    const l2 = luminance(hex2);
    const brightest = Math.max(l1, l2);
    const darkest = Math.min(l1, l2);
    return (brightest + 0.05) / (darkest + 0.05);
  }

  const white = '#ffffff';
  const surface = '#f5f8fc';
  const footerBg = '#eef2f9';
  const ink = '#0b1849';
  const textMuted = '#44506b';
  const link = '#0042c5';
  const greenText = '#03664b';

  // Headings/text on white and surface (AA requires >= 4.5 for normal text, 3.0 for large)
  assert.ok(contrast(ink, white) >= 4.5, 'ink on white contrast');
  assert.ok(contrast(ink, surface) >= 4.5, 'ink on surface contrast');
  assert.ok(contrast(textMuted, white) >= 4.5, 'textMuted on white contrast');
  assert.ok(contrast(link, white) >= 4.5, 'link on white contrast');
  assert.ok(contrast(link, footerBg) >= 4.5, 'link on footerBg contrast');
  assert.ok(contrast(greenText, white) >= 4.5, 'greenText on white contrast');
});
