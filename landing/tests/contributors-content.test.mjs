import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function buildTmp(env = {}) {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-contribuidores-'));
  const previous = process.env.ABASTEVO_API_ORIGIN;
  try {
    if (env.apiOrigin === undefined) {
      delete process.env.ABASTEVO_API_ORIGIN;
    } else {
      process.env.ABASTEVO_API_ORIGIN = env.apiOrigin;
    }
    build({ projectRoot: root, outDir });
    return outDir;
  } finally {
    if (previous === undefined) {
      delete process.env.ABASTEVO_API_ORIGIN;
    } else {
      process.env.ABASTEVO_API_ORIGIN = previous;
    }
  }
}

test('contribuidores page works without JS: explainer, repo and app links', () => {
  const outDir = buildTmp();
  try {
    const html = fs.readFileSync(path.join(outDir, 'contribuidores/index.html'), 'utf8');
    assert.match(html, /<h1>Contribuidores do projeto<\/h1>/);
    assert.ok(html.includes('https://github.com/AlexandreZanata/abastevo'), 'repo link');
    assert.ok(html.includes('play.google.com/store/apps/details?id=com.anpfuel'), 'app link');
    assert.ok(html.includes('Sem conta para ler') || html.includes('sem conta'), 'no-account note');
    // API origin disabled by default: meta left empty, enhancement stays inert.
    assert.match(html, /<meta name="abastevo-api" content="">/);
    // Fingerprinted enhancement script referenced.
    const js = [...html.matchAll(/\/assets\/(contributors\.[a-f0-9]{8}\.js)/g)];
    assert.equal(js.length, 1, 'one fingerprinted contributors.js');
    assert.ok(fs.existsSync(path.join(outDir, 'assets', js[0][1])));
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('contribuidores page carries the configured API origin', () => {
  const outDir = buildTmp({ apiOrigin: 'https://api.example.invalid' });
  try {
    const html = fs.readFileSync(path.join(outDir, 'contribuidores/index.html'), 'utf8');
    assert.match(html, /<meta name="abastevo-api" content="https:\/\/api\.example\.invalid">/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
