import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('learning and legal routes share the home header/footer and resolve home-section links', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-learning-'));
  try {
    build({ projectRoot: root, outDir });
    const home = fs.readFileSync(path.join(outDir, 'index.html'), 'utf8');
    for (const section of ['guias', 'como-usar', 'privacidade', 'excluir-conta', 'termos']) {
      const files = fs.readdirSync(path.join(outDir, section), { recursive: true })
        .filter((file) => file.endsWith('index.html'));
      const learning = section === 'guias' || section === 'como-usar';
      assert.ok(files.length >= (learning ? 4 : 1), `${section} includes its public routes`);
      for (const file of files) {
        const html = fs.readFileSync(path.join(outDir, section, file), 'utf8');
        for (const tag of ['header', 'footer']) {
          const pattern = new RegExp(`<${tag}\\b[\\s\\S]*?</${tag}>`);
          assert.equal(html.match(pattern)?.[0], home.match(pattern)?.[0].replaceAll('href="#', 'href="/#'), `${section}/${file}: ${tag} parity`);
        }
        assert.match(html, learning ? /class="landing-home learning-page/ : /class="landing-home legal-document"/);
        assert.doesNotMatch(html, /(?:LEARNING|SITE)_(?:HEADER|FOOTER)|class="site-header"|class="site-footer"/);
        for (const anchor of [...html.matchAll(/href="\/#([^"]+)"/g)].map((m) => m[1])) {
          assert.ok(home.includes(`id="${anchor}"`), `${section}/${file}: home target ${anchor}`);
        }
      }
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
