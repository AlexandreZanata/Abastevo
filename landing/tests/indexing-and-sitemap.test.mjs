import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('sitemap: valid XML, exactly 4 indexable URLs, no 404 and no fabricated lastmod', () => {
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
    ];

    for (const url of expectedUrls) {
      assert.ok(sitemapContent.includes(`<loc>${url}</loc>`), `Sitemap must contain loc ${url}`);
    }

    // Negative assertions
    assert.ok(!sitemapContent.includes('404'), 'Sitemap must NOT contain 404 page');
    assert.ok(!sitemapContent.includes('<lastmod>'), 'Sitemap must NOT fabricate lastmod');
    assert.ok(!sitemapContent.includes('localhost'), 'Sitemap must NOT contain localhost');
    assert.ok(!sitemapContent.includes('{{CANONICAL_ORIGIN}}'), 'Sitemap must NOT have unreplaced origin placeholders');
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
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
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
    assert.ok(llmsContent.includes('licença MIT'), 'llms.txt must mention MIT license');
    assert.ok(llmsContent.includes('24 horas'), 'llms.txt must mention 24h photo retention');
    assert.ok(llmsContent.includes('https://abastevo.com.br/'), 'llms.txt must have resolved canonical origin');
    assert.ok(!llmsContent.includes('{{CANONICAL_ORIGIN}}'), 'llms.txt must not contain unresolved placeholders');

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
  } finally {
    delete process.env.PREVIEW;
    fs.rmSync(tmpProd, { recursive: true, force: true });
    fs.rmSync(tmpPrev, { recursive: true, force: true });
  }
});
