import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

function getDirectoryTree(dir) {
  const tree = {};
  function walk(currentRel) {
    const currentAbs = path.join(dir, currentRel);
    const entries = fs.readdirSync(currentAbs, { withFileTypes: true });
    entries.sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      const rel = path.join(currentRel, entry.name);
      const full = path.join(dir, rel);
      if (entry.isDirectory()) {
        walk(rel);
      } else {
        const content = fs.readFileSync(full);
        const hash = crypto.createHash('sha256').update(content).digest('hex');
        tree[rel] = { size: content.length, hash };
      }
    }
  }
  walk('');
  return tree;
}

test('build reproducibility: consecutive builds yield byte-identical output', () => {
  const tmpOut1 = path.join(root, '.test-dist-repro-1-' + Date.now());
  const tmpOut2 = path.join(root, '.test-dist-repro-2-' + Date.now());

  try {
    const res1 = build({ projectRoot: root, outDir: tmpOut1 });
    const res2 = build({ projectRoot: root, outDir: tmpOut2 });

    assert.equal(res1.fingerprintedCss, res2.fingerprintedCss);
    assert.equal(res1.fingerprintedJs, res2.fingerprintedJs);
    assert.equal(res1.cssHash, res2.cssHash);
    assert.equal(res1.jsHash, res2.jsHash);

    const tree1 = getDirectoryTree(tmpOut1);
    const tree2 = getDirectoryTree(tmpOut2);

    assert.deepEqual(Object.keys(tree1).sort(), Object.keys(tree2).sort());

    for (const file of Object.keys(tree1)) {
      assert.equal(tree1[file].hash, tree2[file].hash, `Hash mismatch for file: ${file}`);
      assert.equal(tree1[file].size, tree2[file].size, `Size mismatch for file: ${file}`);
    }
  } finally {
    fs.rmSync(tmpOut1, { recursive: true, force: true });
    fs.rmSync(tmpOut2, { recursive: true, force: true });
  }
});

test('no TypeScript sources, sourcemaps or private configs leak into dist', () => {
  const tmpOut = path.join(root, '.test-dist-no-leaks-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut });
    const tree = getDirectoryTree(tmpOut);
    for (const file of Object.keys(tree)) {
      assert.ok(!file.endsWith('.ts'), `Found leaked TypeScript file: ${file}`);
      assert.ok(!file.endsWith('.map'), `Found leaked sourcemap file: ${file}`);
      assert.ok(!file.includes('.env'), `Found leaked env file: ${file}`);
      assert.ok(!file.endsWith('package.json'), `Found leaked package.json: ${file}`);
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
