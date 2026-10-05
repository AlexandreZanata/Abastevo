import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, validateOrigin } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('origin validation: accepts valid HTTPS origins', () => {
  assert.equal(validateOrigin('https://abastevo.com.br'), 'https://abastevo.com.br');
  assert.equal(validateOrigin('https://abastevo.com.br/'), 'https://abastevo.com.br');
  assert.equal(validateOrigin('https://sub.abastevo.com.br'), 'https://sub.abastevo.com.br');
});

test('origin validation: rejects missing, insecure or malformed origins', () => {
  assert.throws(() => validateOrigin(''), /Public origin is required/);
  assert.throws(() => validateOrigin(null), /Public origin is required/);
  assert.throws(() => validateOrigin('http://abastevo.com.br'), /Insecure public origin.*must use https:/);
  assert.throws(() => validateOrigin('not-a-url'), /Invalid public origin URL/);
  assert.throws(() => validateOrigin('https://abastevo.com.br/path/segment'), /Public origin must not have path segments/);
  assert.throws(() => validateOrigin('https://abastevo.com.br?query=1'), /Public origin must not have query or hash/);
});

test('origin is correctly injected into canonical, sitemap and robots', () => {
  const tmpOut = path.join(root, '.test-dist-origin-' + Date.now());
  const origin = 'https://custom.abastevo.com.br';
  try {
    build({ projectRoot: root, outDir: tmpOut, origin });

    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');
    assert.match(indexHtml, /<link rel="canonical" href="https:\/\/custom\.abastevo\.com\.br\/">/);
    assert.match(indexHtml, /"url": "https:\/\/custom\.abastevo\.com\.br\/"/);

    const sitemapXml = fs.readFileSync(path.join(tmpOut, 'sitemap.xml'), 'utf8');
    assert.match(sitemapXml, /<loc>https:\/\/custom\.abastevo\.com\.br\/<\/loc>/);
    assert.match(sitemapXml, /<loc>https:\/\/custom\.abastevo\.com\.br\/privacidade\/<\/loc>/);

    const robotsTxt = fs.readFileSync(path.join(tmpOut, 'robots.txt'), 'utf8');
    assert.match(robotsTxt, /Sitemap: https:\/\/custom\.abastevo\.com\.br\/sitemap\.xml/);
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
