import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('build: dist has pages, fingerprinted assets, noindex and fonts', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-build-'));
  try {
    const result = build({ projectRoot: root, outDir });
    assert.ok(result.cssName.match(/^styles\.[a-f0-9]{8}\.css$/));
    assert.ok(result.adminCssName.match(/^admin\.[a-f0-9]{8}\.css$/));
    for (const name of ['app', 'api', 'validators']) {
      assert.match(result.fingerprinted[`${name}.js`], new RegExp(`^${name}\\.[a-f0-9]{8}\\.js$`));
    }
    for (const rel of ['login/index.html', 'accounts/index.html', 'contributors/index.html', 'audit/index.html', '404.html', '_headers', 'robots.txt']) {
      assert.ok(fs.existsSync(path.join(outDir, rel)), `missing ${rel}`);
    }
    for (const rel of ['login/index.html', 'accounts/index.html', 'contributors/index.html', 'audit/index.html', '404.html']) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
      assert.match(html, /<meta name="robots" content="noindex, nofollow">/, `${rel} noindex`);
      assert.match(html, new RegExp(`/assets/${result.cssName.replace('.', '\\.')}`), `${rel} css ref`);
      assert.match(html, new RegExp(`/assets/${result.adminCssName.replace('.', '\\.')}`), `${rel} admin css ref`);
    }
    for (const rel of ['login/index.html', 'accounts/index.html', 'contributors/index.html', 'audit/index.html']) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
      const appJs = result.fingerprinted['app.js'];
      assert.ok(html.includes(`/assets/${appJs}`), `${rel} app js ref`);
      assert.doesNotMatch(html, /innerHTML|outerHTML|insertAdjacentHTML/);
    }
    for (const [rel, id] of [
      ['contributors/index.html', 'view-contributors'],
      ['audit/index.html', 'view-audit'],
      ['accounts/index.html', 'role-form'],
      ['audit/index.html', 'audit-filter-form'],
    ]) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
      assert.ok(html.includes(`id="${id}"`), `${rel} has #${id}`);
    }
    // Fonts copied from landing, hash-verified
    const css = fs.readFileSync(path.join(outDir, 'assets', result.cssName), 'utf8');
    const references = [...css.matchAll(/url\(['"]?\/admin(\/assets\/fonts\/[^)'"\s]+\.woff2)['"]?\)/g)];
    assert.ok(references.length >= 4, 'expected font references under /admin/assets/fonts in CSS');
    for (const [, reference] of references) {
      const emitted = fs.readFileSync(path.join(outDir, reference));
      assert.equal(emitted.subarray(0, 4).toString(), 'wOF2', `${reference} must be WOFF2`);
    }
    for (const license of ['OFL-Manrope.txt', 'OFL-Source-Sans-3.txt']) {
      assert.match(fs.readFileSync(path.join(outDir, 'assets/fonts', license), 'utf8'), /SIL Open Font License/);
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

// Pages are mounted under /admin/ and B3 serves dist with Caddy handle_path,
// which strips the /admin prefix before the file lookup. A root-absolute
// /assets/... reference would therefore resolve against the API origin root.
const MOUNT = '/admin';

function walk(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else out.push(full);
  }
  return out;
}

test('build: every asset reference is mounted under /admin and resolves after the prefix is stripped', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-mount-'));
  try {
    build({ projectRoot: root, outDir });
    const files = walk(outDir).filter((f) => f.endsWith('.html') || f.endsWith('.css'));
    assert.ok(files.some((f) => f.endsWith('.css')), 'expected CSS in dist');
    let checked = 0;
    for (const file of files) {
      const rel = path.relative(outDir, file);
      const content = fs.readFileSync(file, 'utf8');
      assert.doesNotMatch(content, /(?<![\w/])\/assets\//, `${rel} has a root-absolute /assets/ reference`);
      const refs = [
        ...content.matchAll(/(?:src|href)="(\/[^"#?]*)"/g),
        ...content.matchAll(/url\(\s*['"]?(\/[^)'"\s]+)['"]?\s*\)/g),
      ].map((match) => match[1]);
      for (const ref of refs) {
        assert.ok(ref.startsWith(`${MOUNT}/`), `${rel} references ${ref} outside ${MOUNT}/`);
        const stripped = ref.slice(MOUNT.length);
        const target = stripped.endsWith('/')
          ? path.join(outDir, stripped, 'index.html')
          : path.join(outDir, stripped);
        assert.ok(fs.existsSync(target), `${rel} references ${ref}, missing ${stripped} in dist`);
        checked += 1;
      }
    }
    assert.ok(checked >= 10, `expected asset references to be checked, got ${checked}`);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
