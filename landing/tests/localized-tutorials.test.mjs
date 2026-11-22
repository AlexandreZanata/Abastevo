import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build } from '../scripts/build.mjs';
import { loadSite } from '../scripts/content/site.mjs';

const root = path.resolve(import.meta.dirname, '..');
const origin = 'https://abastevo.com.br';
const sourceRoot = '/como-usar/';

test('both tutorial hubs and all steps are available in their languages without JavaScript', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-tutorials-'));
  try {
    const site = loadSite(root);
    build({ projectRoot: root, outDir, origin });
    const sourceChapters = [...site.registry.values()].filter((entry) =>
      entry.kind === 'static' && entry.route.startsWith(sourceRoot) && entry.route !== sourceRoot);
    const readPage = (route) => fs.readFileSync(path.join(outDir, site.registry.get(route).outputRel), 'utf8');
    const sitemap = fs.readFileSync(path.join(outDir, 'sitemap.xml'), 'utf8');

    for (const [code, hub, screenshot] of [
      ['en', '/en/how-to/', 'View original screenshot (Portuguese)'],
      ['es', '/es/como-usar/', 'Ver captura original (en portugués)'],
    ]) {
      const index = readPage(hub);
      assert.ok(index.includes(`<html lang="${code}">`));
      assert.equal([...index.matchAll(/class="lesson-step chapter-card"/g)].length, 30);
      assert.equal([...index.matchAll(/ data-pending>/g)].length, 30 - sourceChapters.length);
      assert.ok(sitemap.includes(`<loc>${origin}${hub}</loc>`));
      let count = 0;
      for (const source of sourceChapters) {
        const route = site.localizedRoute(source.route, code);
        assert.ok(route.startsWith(hub), `${source.route} maps to the localized tutorial hub`);
        assert.ok(index.includes(`href="${route}"`), `hub includes each published chapter: ${route}`);
        assert.ok(sitemap.includes(`<loc>${origin}${route}</loc>`), `sitemap includes ${route}`);
        const html = readPage(route);
        assert.ok(html.includes(`<html lang="${code}">`));
        assert.ok(html.includes(`hreflang="pt-BR" href="${origin}${source.route}"`));
        assert.ok(readPage(source.route).includes(`hreflang="${code}" href="${origin}${route}"`));
        const lesson = JSON.parse(fs.readFileSync(path.join(root, 'static', path.dirname(source.source), 'lesson.json'), 'utf8'));
        const steps = [...html.matchAll(/<li class="lesson-step" id="([^"]+)"( data-text-only)?>([\s\S]*?)<\/li>/g)];
        assert.deepEqual(steps.map(([, id]) => id), lesson.steps.map(({ stepId }) => stepId));
        assert.doesNotMatch(html, /<img\b[^>]+\/assets\/app\//, `no Portuguese app screenshots embedded in ${route}`);
        for (const [i, [, id, marker, markup]] of steps.entries()) {
          assert.match(markup, /<h2>[^<]+<\/h2>/);
          assert.equal([...markup.matchAll(/<p><strong>/g)].length, 2, `${id} keeps outcome and recovery instructions`);
          assert.equal(Boolean(marker), Boolean(lesson.steps[i].textOnly));
          if (marker) {
            assert.match(markup, /class="lesson-text-only"/);
            assert.ok(!markup.includes(screenshot), `${id} must not link to an unavailable capture`);
          } else {
            assert.ok(markup.includes(screenshot));
            assert.ok(markup.includes(`href="${source.route}#${id}" hreflang="pt-BR"`));
            assert.ok(readPage(source.route).includes(`id="${id}"`), 'original screenshot anchor exists');
          }
        }
        const navigation = html.match(/<nav class="lesson-nav"[\s\S]*?<\/nav>/)[0];
        assert.ok([...navigation.matchAll(/href="([^"]+)"/g)].every(([, target]) => target.startsWith(hub)));
        count += steps.length;
      }
      assert.equal(sourceChapters.length, 16);
      assert.equal(count, 90);
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
