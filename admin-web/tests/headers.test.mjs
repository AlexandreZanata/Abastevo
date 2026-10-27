import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('headers: admin _headers carries the ADR-023 CSP and no-store HTML', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-headers-'));
  try {
    build({ projectRoot: root, outDir });
    const headers = fs.readFileSync(path.join(outDir, '_headers'), 'utf8');
    assert.match(headers, /X-Content-Type-Options: nosniff/);
    assert.match(headers, /X-Robots-Tag: noindex, nofollow/);
    assert.match(headers, /Referrer-Policy: no-referrer/);
    assert.match(
      headers,
      /Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'/,
    );
    assert.match(headers, /\/admin\/\*\s*\n(\s+\S.*\n)*\s+Cache-Control: no-store/);
    assert.match(headers, /\/admin\/assets\/\*\s*\n(\s+\S.*\n)*\s+Cache-Control: public, max-age=31536000, immutable/);
    const robots = fs.readFileSync(path.join(outDir, 'robots.txt'), 'utf8');
    assert.match(robots, /Disallow: \//);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
