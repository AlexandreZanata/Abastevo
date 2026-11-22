import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { portalRouteLabel } from '../scripts/content/data.mjs';

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
        // Shared chrome also links the separately generated same-origin portal.
        if (['/consultar/', '/fiscalizacao/', '/metodologia/', '/precos/'].includes(href)) {
          assert.ok(fs.existsSync(path.join(root, 'prototype', href, 'index.html')), `missing portal route ${href}`);
          continue;
        }
        // State hubs are portal routes too (SEO-15 footer): only known lowercase UFs.
        if (/^\/precos\/[a-z]{2}\/$/.test(href)) {
          assert.ok(portalRouteLabel(href), `unknown portal state route ${href}`);
          continue;
        }
        // The editorial forum is a portal hub with no page in the prototype.
        if (href === '/forum/') {
          assert.equal(portalRouteLabel(href), 'Fórum abastevo');
          continue;
        }
        const [url, anchor] = href.split('#');
        const [pathname] = url.split('?');
        const rel = pathname === '/' ? 'index.html' : pathname.slice(1) + (pathname.endsWith('/') ? 'index.html' : '');
        assert.ok(fs.existsSync(path.join(outDir, rel)), `dead internal link ${href}`);
        if (anchor) assert.ok(fs.readFileSync(path.join(outDir, rel), 'utf8').includes(`id="${anchor}"`), `dead home anchor ${href}`);
      }
    }

    // 4. Chapters without built pages carry status text, never a link.
    const chapterLinks = [...hubHtml.matchAll(/href="\/como-usar\/[\w-]+\/"/g)];
    for (const link of chapterLinks) {
      const rel = link[0].slice(7, -1) + 'index.html';
      assert.ok(fs.existsSync(path.join(outDir, rel)), `dead chapter link ${link[0]}`);
    }

    // 4b. A published chapter is one whole-card link; an unpublished one has no
    // link and says why it is not there yet.
    const cards = [...hubHtml.matchAll(/<li class="lesson-step chapter-card"( data-pending)?>([\s\S]*?)<\/li>/g)];
    assert.equal(cards.length, 30, 'every chapter is a card');
    for (const [, pending, body] of cards) {
      const links = body.match(/<a /g) || [];
      assert.match(body, /<span class="chapter-id">TU\d\d<\/span>/);
      if (pending) {
        assert.equal(links.length, 0, 'an unpublished chapter must not be a link');
        assert.match(body, /<span class="lesson-availability">(Em breve|Planejado)<\/span>/);
      } else {
        assert.equal(links.length, 1, 'a published chapter has exactly one link');
        assert.match(body, /<span class="lesson-availability">Disponível<\/span><\/p><h3><a href="\/como-usar\/[a-z-]+\/">/);
      }
    }
    assert.equal(cards.filter(([, pending]) => !pending).length, 16);

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
