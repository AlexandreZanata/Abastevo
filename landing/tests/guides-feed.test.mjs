import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';
import { loadSite, renderGuidesFeed } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const FEED_LINK = '<link rel="alternate" type="application/atom+xml" title="Guias do abastevo" href="/guias/feed.xml">';

test('guides feed: newest first, content-dated, discoverable and linked to sitemap pages', () => {
  const tmpOut = path.join(root, '.test-feed-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({ projectRoot: root, outDir: tmpOut, canonicalOrigin: 'https://abastevo.com.br', storeState: 'prelaunch' });
    const feed = fs.readFileSync(path.join(tmpOut, 'guias', 'feed.xml'), 'utf8');
    assert.ok(feed.startsWith('<?xml version="1.0" encoding="utf-8"?>'));
    assert.ok(feed.trimEnd().endsWith('</feed>'));
    assert.ok(feed.includes('<title>Guias do abastevo</title>'));
    assert.ok(feed.includes('<name>Equipe abastevo</name>'));

    const entries = [...feed.matchAll(/<entry>([\s\S]*?)<\/entry>/g)].map((match) => match[1]);
    assert.ok(entries.length > 0 && entries.length <= 20, `feed entry count ${entries.length}`);
    const guides = loadSite(root).guides;
    assert.equal(entries.length, Math.min(20, guides.length));

    const expected = [...guides]
      .sort((a, b) => (b.dateModified || b.lastmod).localeCompare(a.dateModified || a.lastmod) || a.route.localeCompare(b.route))
      .slice(0, 20);
    let previous = null;
    entries.forEach((body, index) => {
      const link = body.match(/<link rel="alternate" type="text\/html" href="([^"]+)"\/>/)[1];
      const updated = body.match(/<updated>([^<]+)<\/updated>/)[1];
      assert.equal(body.match(/<id>([^<]+)<\/id>/)[1], link, 'entry id equals its link');
      assert.equal(link, `https://abastevo.com.br${expected[index].route}`);
      assert.equal(updated, `${expected[index].dateModified || expected[index].lastmod}T00:00:00Z`);
      if (previous !== null) assert.ok(updated <= previous, 'newest entries come first');
      previous = updated;
      const rel = link.replace('https://abastevo.com.br/', '');
      assert.ok(fs.existsSync(path.join(tmpOut, rel, 'index.html')), `entry link resolves: ${link}`);
      assert.ok(fs.readFileSync(path.join(tmpOut, 'sitemap.xml'), 'utf8').includes(`<loc>${link}</loc>`), `entry is in the sitemap: ${link}`);
    });

    const newest = `${expected[0].dateModified || expected[0].lastmod}T00:00:00Z`;
    assert.ok(feed.split('<entry>')[0].includes(`<updated>${newest}</updated>`), 'feed updated is the newest content date');
    assert.ok(feed.includes('https://abastevo.com.br/guias/etanol-ou-gasolina/'), 'authored HTML guide is in the feed');

    for (const route of ['/guias/', '/guias/etanol-ou-gasolina/', '/guias/icms-sobre-combustiveis/']) {
      const rel = route === '/guias/' ? 'guias/index.html' : `${route.slice(1)}index.html`;
      assert.ok(fs.readFileSync(path.join(tmpOut, rel), 'utf8').includes(FEED_LINK), `${route} links the guide feed`);
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('guides feed: XML escaping and a deterministic feed date', () => {
  const site = {
    guides: [
      { route: '/guias/antigo/', title: 'Guia antigo', description: 'Texto antigo.', dateModified: '2026-11-10', lastmod: '2026-11-10' },
      { route: '/guias/novo/', title: 'Gasolina & etanol <b>', description: 'R$ 6,29 & "aspas" <forte>', dateModified: '2026-11-17', lastmod: '2026-11-17' },
    ],
  };
  const first = renderGuidesFeed(site, 'https://abastevo.com.br');
  const second = renderGuidesFeed(site, 'https://abastevo.com.br');
  assert.equal(first, second, 'feed bytes do not depend on the clock');
  assert.ok(first.includes('<title>Gasolina &amp; etanol &lt;b&gt;</title>'));
  assert.ok(first.includes('R$ 6,29 &amp; &quot;aspas&quot; &lt;forte&gt;'));
  assert.ok(!first.includes('<b>') && !first.includes('<forte>'), 'no raw markup from content');
  assert.ok(first.indexOf('/guias/novo/') < first.indexOf('/guias/antigo/'), 'newest entry first');
  assert.ok(first.split('<entry>')[0].includes('<updated>2026-11-17T00:00:00Z</updated>'));
});
