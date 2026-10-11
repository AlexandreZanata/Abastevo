import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, copyAppAssets } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('emitted app derivatives match provenance hashes with no master leakage', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-app-assets-'));
  try {
    const result = build({ projectRoot: root, outDir });
    const provenance = JSON.parse(
      fs.readFileSync(path.join(root, 'static', 'assets', 'app', 'provenance.json'), 'utf8'),
    );
    assert.equal(result.appAssets, provenance.outputs.length);
    const appOut = path.join(outDir, 'assets', 'app');
    const emitted = fs.readdirSync(appOut).sort();
    assert.ok(emitted.includes('provenance.json'));
    const webp = emitted.filter((f) => f.endsWith('.webp'));
    assert.equal(webp.length, provenance.outputs.length);
    assert.ok(!emitted.some((f) => f.endsWith('.png')), 'masters must never ship');
    const emittedProvenance = JSON.parse(fs.readFileSync(path.join(appOut, 'provenance.json'), 'utf8'));
    assert.equal(emittedProvenance.outputs.length, provenance.outputs.length);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

function stageAppDir(files) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-app-stage-'));
  const outputs = [];
  for (const [name, content] of Object.entries(files)) {
    fs.writeFileSync(path.join(dir, name), content);
  }
  return dir;
}

test('missing app derivative fails loudly', () => {
  const staticDir = stageAppDir({});
  fs.writeFileSync(
    path.join(staticDir, 'provenance.json'),
    JSON.stringify({ outputs: [{ file: 'ghost-360w.webp', sha256: '0'.repeat(64) }] }),
  );
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-app-out-'));
  try {
    assert.throws(() => copyAppAssets(staticDir, outDir), /App derivative missing: ghost-360w\.webp/);
  } finally {
    fs.rmSync(staticDir, { recursive: true, force: true });
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('traversal filename and hash mismatch are refused', () => {
  const staticDir = stageAppDir({ 'real-360w.webp': Buffer.from('RIFF') });
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-app-out-'));
  try {
    fs.writeFileSync(
      path.join(staticDir, 'provenance.json'),
      JSON.stringify({ outputs: [{ file: '../evil.webp', sha256: '0'.repeat(64) }] }),
    );
    assert.throws(() => copyAppAssets(staticDir, outDir), /possible traversal/);
    fs.writeFileSync(
      path.join(staticDir, 'provenance.json'),
      JSON.stringify({ outputs: [{ file: 'real-360w.webp', sha256: '1'.repeat(64) }] }),
    );
    assert.throws(() => copyAppAssets(staticDir, outDir), /hash mismatch/);
  } finally {
    fs.rmSync(staticDir, { recursive: true, force: true });
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('png master inside web assets refuses the build step', () => {
  const staticDir = stageAppDir({ 'master.png': Buffer.from('PNG') });
  fs.writeFileSync(
    path.join(staticDir, 'provenance.json'),
    JSON.stringify({ outputs: [] }),
  );
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-app-out-'));
  try {
    assert.throws(() => copyAppAssets(staticDir, outDir), /must not ship/);
  } finally {
    fs.rmSync(staticDir, { recursive: true, force: true });
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
