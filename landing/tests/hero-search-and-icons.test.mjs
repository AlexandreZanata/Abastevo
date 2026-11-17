import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { listHtmlPages } from '../scripts/check.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('browser tab icons: bare mark for light, monochrome mark for dark, on every page', () => {
  const brand = path.join(root, 'static', 'assets', 'brand');
  const light = fs.readFileSync(path.join(brand, 'favicon.svg'), 'utf8');
  const dark = fs.readFileSync(path.join(brand, 'favicon-dark.svg'), 'utf8');
  for (const svg of [light, dark]) {
    assert.doesNotMatch(svg, /<rect\b/, 'no background square');
    assert.doesNotMatch(svg, /<style\b|<script\b|href=/, 'no inline style (CSP) or external reference');
    // Cropped to the mark so it renders larger, with a margin so nothing is cut.
    assert.match(svg, /viewBox="122 107 1020 1020"/);
  }
  // Same geometry, different paint.
  const paths = (svg) => [...svg.matchAll(/ d="([^"]+)"/g)].map((m) => m[1]).join('|');
  assert.equal(paths(dark), paths(light));
  assert.match(light, /#0052e5/i);
  for (const colour of dark.match(/#[0-9a-f]{6}/gi)) {
    const [r, g, b] = [1, 3, 5].map((i) => parseInt(colour.slice(i, i + 2), 16));
    assert.ok(r === g && g === b && r >= 0x99, `dark icon colour ${colour} must be white or light grey`);
  }

  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-icons-'));
  try {
    build({ projectRoot: root, outDir });
    for (const rel of listHtmlPages(outDir)) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
      // Dark first and light last: a browser that ignores `media` keeps the coloured mark.
      assert.match(html, /<link rel="icon" type="image\/svg\+xml" href="\/assets\/brand\/favicon-dark\.svg" media="\(prefers-color-scheme: dark\)">\s*<link rel="icon" type="image\/svg\+xml" href="\/assets\/brand\/favicon\.svg" media="\(prefers-color-scheme: light\)">/, rel);
    }
    assert.ok(fs.existsSync(path.join(outDir, 'assets', 'brand', 'favicon-dark.svg')));
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('the state picker enhances the native select and never replaces it in the HTML', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-picker-'));
  try {
    const result = build({ projectRoot: root, outDir });
    const html = fs.readFileSync(path.join(outDir, 'index.html'), 'utf8');
    const js = fs.readFileSync(path.join(outDir, 'assets', result.fingerprintedJs), 'utf8');
    // Without JavaScript the form is the plain, required native select with 27 states.
    const select = html.match(/<select name="uf" required>[\s\S]*?<\/select>/)[0];
    assert.equal((select.match(/<option value="[A-Z]{2}">/g) || []).length, 27);
    assert.doesNotMatch(html, /uf-button|uf-list|role="listbox"/);
    // The enhancement is a labelled listbox with keyboard support, built from DOM nodes.
    for (const needle of ["'listbox'", "'option'", 'aria-activedescendant', 'aria-expanded', "'ArrowDown'", "'Escape'", "'Home'", 'select.value']) {
      assert.ok(js.includes(needle), `state picker is missing ${needle}`);
    }
    assert.doesNotMatch(js, /innerHTML|insertAdjacentHTML/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
