import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('seo & metadata: title, description, open graph, and icon links across all pages', () => {
  const tmpOut = path.join(root, '.test-seo-dist-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });

    const indexablePages = [
      { rel: 'index.html', canonicalPath: '/' },
      { rel: 'privacidade/index.html', canonicalPath: '/privacidade/' },
      { rel: 'excluir-conta/index.html', canonicalPath: '/excluir-conta/' },
      { rel: 'termos/index.html', canonicalPath: '/termos/' },
    ];

    for (const page of indexablePages) {
      const htmlPath = path.join(tmpOut, page.rel);
      assert.ok(fs.existsSync(htmlPath), `Page must exist: ${page.rel}`);
      const html = fs.readFileSync(htmlPath, 'utf8');

      // Title tag length (target bounds [25, 65])
      const titleMatch = html.match(/<title>([^<]+)<\/title>/);
      assert.ok(titleMatch, `${page.rel} must have a <title> tag`);
      const title = titleMatch[1].trim();
      assert.ok(
        title.length >= 25 && title.length <= 65,
        `${page.rel} title length (${title.length}) must be between 25 and 65: "${title}"`
      );

      // Meta description (target bounds [110, 165])
      const descMatch = html.match(/<meta\s+name="description"\s+content="([^"]+)"/);
      assert.ok(descMatch, `${page.rel} must have meta description`);
      const desc = descMatch[1].trim();
      assert.ok(
        desc.length >= 110 && desc.length <= 165,
        `${page.rel} description length (${desc.length}) must be between 110 and 165: "${desc}"`
      );

      // Canonical link
      const expectedCanonical = `https://abastevo.com.br${page.canonicalPath}`;
      assert.ok(
        html.includes(`<link rel="canonical" href="${expectedCanonical}">`),
        `${page.rel} must contain exact canonical link to ${expectedCanonical}`
      );

      // Open Graph essentials
      assert.ok(html.includes('<meta property="og:site_name" content="abastevo">'), `${page.rel} must have og:site_name`);
      assert.ok(html.includes('<meta property="og:locale" content="pt_BR">'), `${page.rel} must have og:locale pt_BR`);
      assert.ok(html.includes(`content="https://abastevo.com.br/assets/brand/og-image-v1.png"`), `${page.rel} must have og-image-v1.png`);
      assert.ok(html.includes('<meta property="og:image:width" content="1200">'), `${page.rel} must have og:image:width 1200`);
      assert.ok(html.includes('<meta property="og:image:height" content="630">'), `${page.rel} must have og:image:height 630`);

      // Twitter card
      assert.ok(html.includes('<meta name="twitter:card" content="summary_large_image">'), `${page.rel} must have summary_large_image`);

      // Icons
      assert.ok(html.includes('href="/assets/brand/favicon.svg"'), `${page.rel} must link favicon.svg`);
      assert.ok(html.includes('href="/assets/brand/icon-192.png"'), `${page.rel} must link icon-192.png`);
      assert.ok(html.includes('href="/favicon.ico"'), `${page.rel} must link root favicon.ico`);
      assert.ok(html.includes('href="/assets/brand/apple-touch-icon.png"'), `${page.rel} must link apple-touch-icon.png`);
      assert.ok(html.includes('href="/site.webmanifest"'), `${page.rel} must link site.webmanifest`);
      assert.ok(html.includes('<meta name="theme-color" content="#ffffff">'), `${page.rel} must define theme-color`);
    }

    // 404 page checks
    const notFoundHtml = fs.readFileSync(path.join(tmpOut, '404.html'), 'utf8');
    assert.ok(notFoundHtml.includes('<meta name="robots" content="noindex, nofollow">'), '404.html must have noindex robots');
    assert.ok(!notFoundHtml.includes('rel="canonical"'), '404.html must not declare a canonical URL');

    // Referenced static assets exist on disk
    const requiredAssets = [
      'assets/brand/favicon.svg',
      'assets/brand/apple-touch-icon.png',
      'assets/brand/icon-192.png',
      'assets/brand/icon-512.png',
      'assets/brand/og-image-v1.png',
      'favicon.ico',
    ];
    for (const asset of requiredAssets) {
      assert.ok(fs.existsSync(path.join(tmpOut, asset)), `Referenced asset must exist in output: ${asset}`);
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
