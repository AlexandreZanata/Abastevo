import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('headers: _headers contains essential security and caching policies', () => {
  const tmpOut = path.join(root, '.test-dist-headers-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut });
    const headersContent = fs.readFileSync(path.join(tmpOut, '_headers'), 'utf8');

    // Security headers
    assert.match(headersContent, /X-Content-Type-Options: nosniff/);
    assert.match(headersContent, /Referrer-Policy: strict-origin-when-cross-origin/);
    assert.match(headersContent, /Permissions-Policy: camera=\(\), microphone=\(\), geolocation=\(\)/);
    assert.match(headersContent, /Content-Security-Policy:.*default-src 'self'/);
    assert.match(headersContent, /Strict-Transport-Security: max-age=31536000; includeSubDomains/);

    // Caching policies
    assert.match(headersContent, /\/assets\/\*\s+Cache-Control: public, max-age=31536000, immutable/);
    assert.match(headersContent, /Cache-Control: public, max-age=3600, must-revalidate/);
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('404: noindex plain-HTML fallback with recovery links and no scripts', () => {
  const tmpOut = path.join(root, '.test-dist-404-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut });
    const notFound = fs.readFileSync(path.join(tmpOut, '404.html'), 'utf8');
    assert.match(notFound, /<meta name="robots" content="noindex, nofollow">/);
    assert.doesNotMatch(notFound, /<script/);
    assert.ok(notFound.includes('href="/"'), '404 recovers to home');
    assert.ok(notFound.includes('href="/como-usar/"'), '404 recovers to tutorial hub');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('redirects: _redirects specifies valid HTTP redirect status codes', () => {
  const tmpOut = path.join(root, '.test-dist-redirects-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut });
    const redirectsContent = fs.readFileSync(path.join(tmpOut, '_redirects'), 'utf8');

    const lines = redirectsContent.split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#'));
    assert.ok(lines.length > 0, '_redirects should contain at least one redirect rule');

    for (const line of lines) {
      const parts = line.split(/\s+/);
      assert.ok(parts.length >= 3, `Line should have source target status: ${line}`);
      const status = Number(parts[parts.length - 1]);
      assert.ok([301, 302, 307, 308].includes(status), `Invalid HTTP status code in line: ${line}`);
      const target = parts[1];
      assert.ok(
        target.startsWith('https://github.com/AlexandreZanata/'),
        `Redirect target must stay inside the project repository: ${line}`
      );
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
