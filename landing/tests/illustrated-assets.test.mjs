import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
test('illustrated scenes ship responsive bounded derivatives with distinct category accents', () => {
  const art = JSON.parse(fs.readFileSync(path.join(root, 'static/assets/art/provenance.json'), 'utf8'));
  const html = fs.readFileSync(path.join(root, 'static/index.html'), 'utf8');
  const scenes = art.sources.filter(source => source.accent);
  assert.equal(scenes.length, 6);
  assert.equal(new Set(scenes.map(source => source.accent)).size, 6);
  for (const scene of scenes) {
    const outputs = art.outputs.filter(output => output.role === scene.name);
    assert.deepEqual(outputs.map(output => output.width), [320, 640]);
    for (const output of outputs) {
      assert.ok(output.bytes <= 36 * 1024, `${output.file} exceeds scene budget`);
      assert.ok(html.includes(output.file), `missing responsive scene ${output.file}`);
    }
  }
  for (const output of art.outputs.filter(output => output.role === 'hero-background')) {
    assert.ok(output.bytes <= 96 * 1024, 'hero landscape exceeds budget');
    assert.ok(html.includes(output.file));
  }
  assert.equal(art.outputs.filter(output => output.role === 'hero-background').length, 4);
  assert.match(html, /<section class="hero">\s*<picture class="hero-landscape"/);
  assert.doesNotMatch(html, /class="product-map"/);
  const master = fs.readFileSync(path.resolve(root, '..', art.export4k.master));
  assert.equal(master.readUInt32BE(16), 3840);
  assert.equal(master.readUInt32BE(20), 2160);
  for (const role of ['trees', 'skyline', 'road', 'car']) {
    const outputs = art.outputs.filter(output => output.role === role);
    assert.deepEqual(outputs.map(output => output.width), [320, 640]);
    assert.ok(outputs.every(output => output.bytes <= 36 * 1024));
  }
  assert.equal((html.match(/class="card-art card-art-/g) || []).length, 6);
  assert.doesNotMatch(html, /class="hero-stats"/); // Preserve the owner's OpenCode removal.
  assert.match(html, /Conheça o aplicativo/);
  assert.match(html, /assets\/app\/explore-light-720w\.webp/);
});
