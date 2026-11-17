import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
test('illustrated scenes ship responsive bounded derivatives with distinct category accents', () => {
  const art = JSON.parse(fs.readFileSync(path.join(root, 'static/assets/art/provenance.json'), 'utf8'));
  const html = fs.readFileSync(path.join(root, 'static/index.html'), 'utf8');
  const scenes = art.sources.filter(source => source.accent);
  assert.equal(scenes.length, 7); // LH09 adds the owner-supplied open-data scene.
  assert.equal(new Set(scenes.map(source => source.accent)).size, 7);
  for (const scene of scenes) {
    const outputs = art.outputs.filter(output => output.role === scene.name);
    assert.deepEqual(outputs.map(output => output.width), [320, 640]);
    for (const output of outputs) {
      assert.ok(output.bytes <= 36 * 1024, `${output.file} exceeds scene budget`);
      assert.ok(html.includes(output.file), `missing responsive scene ${output.file}`);
    }
  }
  assert.equal(art.outputs.filter(output => output.role === 'hero-background').length, 0);
  assert.doesNotMatch(html, /hero-landscape|hero-background-|class="product-map"/);
  const master = fs.readFileSync(path.resolve(root, '..', art.export4k.master));
  assert.equal(master.readUInt32BE(16), 3840);
  assert.equal(master.readUInt32BE(20), 2160);
  // The home hero is one hand-authored SVG built around the approved brand
  // mark; the earlier road/car/skyline/trees rasters no longer ship.
  for (const role of ['trees', 'skyline', 'road', 'car']) {
    assert.equal(art.outputs.filter(output => output.role === role).length, 0);
  }
  const hero = art.outputs.filter(output => output.role === 'hero-mark');
  assert.equal(hero.length, 1);
  assert.match(hero[0].file, /^hero-mark\.[a-f0-9]{8}\.svg$/);
  assert.ok(hero[0].bytes <= 40 * 1024, 'hero art stays small');
  assert.ok(html.includes(`/assets/art/${hero[0].file}`), 'hero references the shipped art');
  const heroSvg = fs.readFileSync(path.join(root, 'static/assets/art', hero[0].file), 'utf8');
  assert.doesNotMatch(heroSvg, /<text\b|<image\b|<script\b|href=/, 'hero art is local vector geometry without text');
  const heroSource = art.sources.find(source => source.name === 'hero-mark');
  assert.deepEqual(fs.readFileSync(path.resolve(root, '..', heroSource.master)), fs.readFileSync(path.join(root, 'static/assets/art', hero[0].file)));
  assert.equal((html.match(/class="card-art card-art-/g) || []).length, 6);
  assert.doesNotMatch(html, /class="hero-stats"/); // Preserve the owner's OpenCode removal.
  assert.match(html, /Conheça o aplicativo/);
  assert.match(html, /assets\/app\/fuel-prices-01-anp-average-cards-720w\.webp/); // LH09: real captures live in the gallery.
});
