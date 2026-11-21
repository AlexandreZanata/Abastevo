import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';
import { loadSite } from '../scripts/content/site.mjs';
import { listHtmlPages, LLMS_DATA_ROUTES } from '../scripts/check.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('sitemap: valid XML, required indexable URLs present, no 404 and content-dated lastmod', () => {
  const tmpOut = path.join(root, '.test-sitemap-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });

    const sitemapContent = fs.readFileSync(path.join(tmpOut, 'sitemap.xml'), 'utf8');
    assert.ok(sitemapContent.startsWith('<?xml version="1.0" encoding="UTF-8"?>'), 'Must start with XML declaration');
    assert.ok(sitemapContent.includes('<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'), 'Must declare urlset namespace');

    const expectedUrls = [
      'https://abastevo.com.br/',
      'https://abastevo.com.br/privacidade/',
      'https://abastevo.com.br/excluir-conta/',
      'https://abastevo.com.br/termos/',
      'https://abastevo.com.br/como-usar/',
    ];

    for (const url of expectedUrls) {
      assert.ok(sitemapContent.includes(`<loc>${url}</loc>`), `Sitemap must contain loc ${url}`);
    }

    // Negative assertions
    assert.ok(!sitemapContent.includes('404'), 'Sitemap must NOT contain 404 page');
    // SEO-08: every URL carries the date its content last changed, taken from
    // the content manifest or front matter. Build time is never used.
    const urlBlocks = [...sitemapContent.matchAll(/<url>\s*<loc>([^<]+)<\/loc>\s*<lastmod>([^<]+)<\/lastmod>\s*<\/url>/g)];
    assert.equal(urlBlocks.length, (sitemapContent.match(/<url>/g) || []).length, 'Every sitemap URL needs exactly loc + lastmod');
    const expected = new Map([...loadSite(root).registry.values()]
      .filter((entry) => entry.indexable)
      .map((entry) => [`https://abastevo.com.br${entry.route}`, entry.lastmod]));
    assert.equal(urlBlocks.length, expected.size, 'Sitemap lists exactly the indexable manifest pages');
    for (const [, loc, lastmod] of urlBlocks) {
      assert.match(lastmod, /^\d{4}-\d{2}-\d{2}$/, `${loc} lastmod format`);
      assert.equal(lastmod, expected.get(loc), `${loc} lastmod must equal its manifest/front-matter date`);
    }
    assert.match(sitemapContent, /<loc>https:\/\/abastevo\.com\.br\/como-usar\/<\/loc>\s*<lastmod>2026-11-17<\/lastmod>/, 'static pages keep their fixed manifest date');
    assert.match(sitemapContent, /<loc>https:\/\/abastevo\.com\.br\/ferramentas\/consumo-medio\/<\/loc>\s*<lastmod>2026-11-17<\/lastmod>/, 'Markdown pages use their front matter dateModified');
    assert.ok(!/<(priority|changefreq)>/.test(sitemapContent), 'Sitemap must not declare priority or changefreq');
    assert.ok(!sitemapContent.includes('localhost'), 'Sitemap must NOT contain localhost');
    assert.ok(!sitemapContent.includes('{{CANONICAL_ORIGIN}}'), 'Sitemap must NOT have unreplaced origin placeholders');

    // LR38 parity: every emitted indexable page is listed, and every listed URL resolves.
    const emittedPages = [];
    function collectHtml(rel) {
      const abs = path.join(tmpOut, rel);
      for (const entry of fs.readdirSync(abs, { withFileTypes: true })) {
        const entryRel = path.join(rel, entry.name);
        if (entry.isDirectory()) {
          collectHtml(entryRel);
        } else if (entry.name === 'index.html' && entryRel !== path.join('index.html')) {
          emittedPages.push(entryRel);
        }
      }
    }
    collectHtml('');
    assert.ok(emittedPages.length >= 20, `expected emitted subpages, got ${emittedPages.length}`);
    for (const rel of emittedPages) {
      const urlPath = `/${rel.slice(0, -'/index.html'.length)}/`;
      assert.ok(
        sitemapContent.includes(`<loc>https://abastevo.com.br${urlPath}</loc>`),
        `Emitted page missing from sitemap: ${urlPath}`
      );
    }
    for (const loc of [...sitemapContent.matchAll(/<loc>(https:\/\/abastevo\.com\.br[^<]*)<\/loc>/g)].map((m) => m[1])) {
      const rel = loc.replace('https://abastevo.com.br/', '');
      const file = rel === '' ? 'index.html' : `${rel}index.html`;
      assert.ok(fs.existsSync(path.join(tmpOut, file)), `Sitemap URL has no emitted page: ${loc}`);
    }
    // SEO-21: every sitemap URL is indexable and declares the rich directive.
    for (const loc of [...sitemapContent.matchAll(/<loc>([^<]+)<\/loc>/g)].map((m) => m[1])) {
      const rel = loc.replace('https://abastevo.com.br/', '');
      const file = rel === '' ? 'index.html' : `${rel}index.html`;
      const html = fs.readFileSync(path.join(tmpOut, file), 'utf8');
      const robots = [...html.matchAll(/<meta\s+name="robots"\s+content="([^"]*)"/g)];
      assert.equal(robots.length, 1, `${loc} must declare exactly one robots meta`);
      assert.equal(robots[0][1], 'index,follow,max-image-preview:large,max-snippet:-1', `${loc} robots directive`);
    }
    const notFound = fs.readFileSync(path.join(tmpOut, '404.html'), 'utf8');
    assert.ok(notFound.includes('<meta name="robots" content="noindex, nofollow">'), '404.html stays noindex');
    assert.ok(!notFound.includes('max-image-preview:large'), '404.html must not carry the indexable directive');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('robots: allows crawling and references canonical sitemap', () => {
  const tmpOut = path.join(root, '.test-robots-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });

    const robotsContent = fs.readFileSync(path.join(tmpOut, 'robots.txt'), 'utf8');
    assert.ok(robotsContent.includes('User-agent: *'), 'Robots must declare User-agent: *');
    assert.ok(robotsContent.includes('Allow: /'), 'Robots must allow /');
    assert.ok(
      robotsContent.includes('Sitemap: https://abastevo.com.br/sitemap.xml'),
      'Robots must point to canonical sitemap'
    );
    assert.ok(!robotsContent.includes('{{CANONICAL_ORIGIN}}'), 'Robots must not contain unreplaced placeholders');

    assert.doesNotMatch(robotsContent, /["\\{}]/, 'Rendered robots must be safe in a Caddy quoted string');
    const caddy = fs.readFileSync(path.join(root, '../infra/caddy/staging-web.Caddyfile'), 'utf8');
    const caddyRobots = caddy.match(/handle \/robots\.txt \{\s*respond "([^"]*)" 200/);
    assert.ok(caddyRobots, 'Caddyfile must serve a robots response');
    assert.equal(caddyRobots[1], robotsContent, 'Caddy response must match the built robots file');

    const groups = [...robotsContent.matchAll(/^User-agent: (.+)\n((?:(?:Allow|Disallow): .+\n)+)/gm)];
    assert.deepEqual(groups.map(([, agent]) => agent),
      ['*', 'OAI-SearchBot', 'GPTBot', 'ClaudeBot', 'ChatGPT-User', 'Perplexity-User']);
    for (const [, agent, rules] of groups) {
      const trainingCrawler = agent === 'GPTBot' || agent === 'ClaudeBot';
      const expected = trainingCrawler ? 'Disallow: /\n'
        : 'Allow: /\nDisallow: /admin/\nDisallow: /empresas/\nDisallow: /dados/busca/\n';
      assert.equal(rules, expected, agent + ' must follow the crawler policy');
    }

    // Crawler separation (LR28): search allowed, training not granted, fetchers transparent.
    for (const token of ['OAI-SearchBot', 'ChatGPT-User', 'Perplexity-User']) {
      assert.ok(robotsContent.includes(`User-agent: ${token}`), `Robots must name ${token}`);
    }
    for (const token of ['GPTBot', 'ClaudeBot']) {
      const section = robotsContent.match(new RegExp(`User-agent: ${token}\\s*\\nDisallow: /`));
      assert.ok(section, `Robots must disallow training crawler ${token}`);
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('IndexNow public key is copied verbatim and excluded from the sitemap', t => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-indexnow-dist-'));
  t.after(() => fs.rmSync(outDir, { recursive: true, force: true }));
  buildSite({ projectRoot: root, outDir });

  const files = fs.readdirSync(outDir).filter(name => /^[a-f0-9]{32}\.txt$/.test(name));
  assert.equal(files.length, 1);
  assert.equal(fs.readFileSync(path.join(outDir, files[0]), 'utf8'), files[0].slice(0, -4));
  assert.ok(!fs.readFileSync(path.join(outDir, 'sitemap.xml'), 'utf8').includes(files[0]));
});

test('IndexNow build rejects zero keys, multiple keys and content drift', t => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-indexnow-source-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const projectRoot = path.join(temp, 'landing');
  const staticDir = path.join(projectRoot, 'static');
  const sourceStatic = path.join(root, 'static');
  fs.cpSync(sourceStatic, staticDir, {
    recursive: true,
    filter: source => !/^[a-f0-9]{32}\.txt$/.test(path.basename(source)),
  });
  fs.symlinkSync(path.join(root, '../contracts'), path.join(temp, 'contracts'), 'dir');
  for (const name of ['content', 'src', 'node_modules']) {
    fs.symlinkSync(path.join(root, name), path.join(projectRoot, name), 'dir');
  }
  fs.copyFileSync(path.join(root, 'tsconfig.json'), path.join(projectRoot, 'tsconfig.json'));
  const first = 'a'.repeat(32);
  const second = 'b'.repeat(32);
  assert.throws(() => buildSite({ projectRoot }), /IndexNow.*exactly one/i);

  fs.writeFileSync(path.join(staticDir, first + '.txt'), first + '\n');
  assert.throws(() => buildSite({ projectRoot }), /IndexNow.*content/i);

  fs.writeFileSync(path.join(staticDir, first + '.txt'), first);
  fs.writeFileSync(path.join(staticDir, second + '.txt'), second);
  assert.throws(() => buildSite({ projectRoot }), /IndexNow.*exactly one/i);
});

test('discovery files: llms.txt and .well-known/security.txt exist and are populated', () => {
  const tmpOut = path.join(root, '.test-discovery-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });

    // llms.txt
    const llmsPath = path.join(tmpOut, 'llms.txt');
    assert.ok(fs.existsSync(llmsPath), 'llms.txt must exist');
    const llmsContent = fs.readFileSync(llmsPath, 'utf8');
    assert.ok(llmsContent.includes('# abastevo'), 'llms.txt must have title');
    assert.ok(llmsContent.includes('licença de atribuição visível Abastevo'), 'llms.txt must identify the current Abastevo license');
    assert.ok(llmsContent.includes('24 horas'), 'llms.txt must mention 24h photo retention');
    assert.ok(llmsContent.includes('https://abastevo.com.br/'), 'llms.txt must have resolved canonical origin');
    assert.ok(!llmsContent.includes('{{CANONICAL_ORIGIN}}'), 'llms.txt must not contain unresolved placeholders');
    const llmsLines = llmsContent.split('\n');
    for (const route of LLMS_DATA_ROUTES) {
      assert.ok(llmsLines.some((line) => line.endsWith(': https://abastevo.com.br' + route)), 'llms.txt must list ' + route);
    }

    // security.txt
    const secPath = path.join(tmpOut, '.well-known', 'security.txt');
    assert.ok(fs.existsSync(secPath), '.well-known/security.txt must exist');
    const secContent = fs.readFileSync(secPath, 'utf8');
    assert.ok(secContent.includes('Contact: https://github.com/AlexandreZanata/abastevo/security/advisories'), 'security.txt must have Contact');
    assert.ok(secContent.includes('Preferred-Languages: pt-BR, en'), 'security.txt must have Preferred-Languages');
    assert.ok(secContent.includes('Canonical: https://abastevo.com.br/.well-known/security.txt'), 'security.txt must have Canonical');
    assert.ok(secContent.includes('Policy:'), 'security.txt must have Policy');
    assert.ok(secContent.includes('Expires:'), 'security.txt must have Expires');
    assert.ok(!secContent.includes('{{CANONICAL_ORIGIN}}'), 'security.txt must not contain unresolved placeholders');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('preview mode: PREVIEW=1 injects noindex, production build does not leak noindex', () => {
  const tmpProd = path.join(root, '.test-preview-prod-' + Math.random().toString(36).slice(2, 8));
  const tmpPrev = path.join(root, '.test-preview-prev-' + Math.random().toString(36).slice(2, 8));
  try {
    // Production build
    delete process.env.PREVIEW;
    buildSite({
      projectRoot: root,
      outDir: tmpProd,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });
    const prodIndex = fs.readFileSync(path.join(tmpProd, 'index.html'), 'utf8');
    assert.ok(!prodIndex.includes('noindex'), 'Production index.html must NOT have noindex');

    // Preview build
    process.env.PREVIEW = '1';
    buildSite({
      projectRoot: root,
      outDir: tmpPrev,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });
    const prevIndex = fs.readFileSync(path.join(tmpPrev, 'index.html'), 'utf8');
    assert.ok(prevIndex.includes('<meta name="robots" content="noindex, nofollow">'), 'Preview index.html must have noindex robots');
    for (const rel of listHtmlPages(tmpPrev)) {
      const html = fs.readFileSync(path.join(tmpPrev, rel), 'utf8');
      const robots = [...html.matchAll(/<meta\s+name="robots"\s+content="([^"]*)"/g)];
      assert.equal(robots.length, 1, `${rel} must declare exactly one robots meta in preview`);
      assert.equal(robots[0][1], 'noindex, nofollow', `${rel} robots directive in preview`);
    }
  } finally {
    delete process.env.PREVIEW;
    fs.rmSync(tmpProd, { recursive: true, force: true });
    fs.rmSync(tmpPrev, { recursive: true, force: true });
  }
});
