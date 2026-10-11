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

test('tutorial hub covers all 30 chapters with journeys and no dead links', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-hub-'));
  try {
    build({ projectRoot: root, outDir });
    const hubHtml = fs.readFileSync(path.join(outDir, 'como-usar', 'index.html'), 'utf8');

    // 1. All 30 chapter IDs present exactly once.
    for (let n = 1; n <= 30; n++) {
      const id = `TU${String(n).padStart(2, '0')}`;
      const occurrences = hubHtml.match(new RegExp(id, 'g')) || [];
      assert.equal(occurrences.length, 1, `${id} must appear exactly once`);
    }

    // 2. Journey entry points resolve to in-page anchors.
    for (const anchor of ['jornada-motorista', 'jornada-comunidade', 'jornada-representante', 'jornada-ajuda']) {
      assert.ok(hubHtml.includes(`href="#${anchor}"`), `journey link #${anchor} missing`);
      assert.ok(hubHtml.includes(`id="${anchor}"`), `journey target #${anchor} missing`);
    }

    // 3. No dead links: every internal href resolves to an emitted file or anchor.
    const hrefs = [...hubHtml.matchAll(/href="(\/[^"]*|#[^"]*)"/g)].map((m) => m[1]);
    assert.ok(hrefs.length > 0);
    for (const href of hrefs) {
      if (href.startsWith('#')) {
        const id = href.slice(1);
        assert.ok(hubHtml.includes(`id="${id}"`), `dead anchor ${href}`);
      } else if (href.startsWith('/')) {
        const rel = href === '/' ? 'index.html' : href.slice(1) + (href.endsWith('/') ? 'index.html' : '');
        assert.ok(fs.existsSync(path.join(outDir, rel)), `dead internal link ${href}`);
      }
    }

    // 4. Chapters without built pages carry status text, never a link.
    const chapterLinks = [...hubHtml.matchAll(/href="\/como-usar\/[\w-]+\/"/g)];
    for (const link of chapterLinks) {
      const rel = link[0].slice(7, -1) + 'index.html';
      assert.ok(fs.existsSync(path.join(outDir, rel)), `dead chapter link ${link[0]}`);
    }

    // 5. Availability vocabulary and destinations preserved.
    assert.match(hubHtml, /Em breve/);
    assert.match(hubHtml, /Planejado/);
    assert.match(hubHtml, /href="\/guias\/"/);
    assert.match(hubHtml, /href="\/privacidade\/"[^>]*>Privacidade<\/a>/);
    assert.match(hubHtml, /"@type": "CollectionPage"/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
