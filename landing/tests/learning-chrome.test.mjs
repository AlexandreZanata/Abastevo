import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('every learning route shares the home header/footer and resolves home-section links', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-learning-'));
  try {
    build({ projectRoot: root, outDir });
    const home = fs.readFileSync(path.join(outDir, 'index.html'), 'utf8');
    for (const section of ['guias', 'como-usar']) {
      const files = fs.readdirSync(path.join(outDir, section), { recursive: true })
        .filter((file) => file.endsWith('index.html'));
      assert.ok(files.length >= 4, `${section} includes hub and detail routes`);
      for (const file of files) {
        const html = fs.readFileSync(path.join(outDir, section, file), 'utf8');
        for (const tag of ['header', 'footer']) {
          const pattern = new RegExp(`<${tag}\\b[\\s\\S]*?</${tag}>`);
          assert.equal(html.match(pattern)?.[0], home.match(pattern)?.[0].replaceAll('href="#', 'href="/#'), `${section}/${file}: ${tag} parity`);
        }
        assert.match(html, /class="landing-home learning-page/);
        assert.doesNotMatch(html, /LEARNING_HEADER|LEARNING_FOOTER|class="site-header"|class="site-footer"/);
        for (const anchor of [...html.matchAll(/href="\/#([^"]+)"/g)].map((m) => m[1])) {
          assert.ok(home.includes(`id="${anchor}"`), `${section}/${file}: home target ${anchor}`);
        }
      }
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
