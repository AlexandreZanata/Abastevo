import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { listHtmlPages } from '../scripts/check.mjs';
import { loadSite, loadAppCaptures } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repo = path.resolve(root, '..');
const provenance = JSON.parse(fs.readFileSync(path.join(root, 'static', 'assets', 'app', 'provenance.json'), 'utf8'));
const captures = JSON.parse(fs.readFileSync(path.join(repo, 'docs', 'assets', 'app-tutorial', 'manifest.json'), 'utf8'));

// feature/NN-slug.png -> manifest step, for every tutorial capture.
const steps = new Map();
for (const feature of captures.features) {
  for (const step of feature.steps) steps.set(`docs/assets/app-tutorial/${feature.id}/${step.file}`, { feature: feature.id, ...step });
}

function builtPages(run) {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-captures-'));
  try {
    build({ projectRoot: root, outDir });
    const pages = new Map(listHtmlPages(outDir).map((rel) => [rel, fs.readFileSync(path.join(outDir, rel), 'utf8')]));
    return run(pages, outDir);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
}

test('every derivative keeps its capture stem and comes from an allowed 9:16 master', () => {
  assert.deepEqual(provenance.params, { format: 'WEBP', quality: 85, method: 6, filter: 'LANCZOS', widths: [360, 720, 1080] });
  const byStem = new Map();
  for (const output of provenance.outputs) {
    const step = steps.get(output.master);
    assert.ok(step, `${output.file}: master ${output.master} is not a tutorial capture`);
    // Contract rule C3: <feature>-<NN-slug>-<width>w.webp.
    assert.equal(output.file, `${step.feature}-${step.file.slice(0, -4)}-${output.width}w.webp`);
    assert.equal(output.height * 9, output.width * 16, `${output.file} must stay 9:16`);
    assert.ok(output.bytes <= 120 * 1024, `${output.file} exceeds the per-file budget`);
    // Captures of the emulator camera scene or of a known app defect stay off the site.
    assert.ok(!step.context?.emulatorArtifact, `${output.file} shows the emulator camera scene`);
    assert.ok(!step.context?.knownIssue, `${output.file} shows a known app defect`);
    const stem = output.file.replace(/-\d+w\.webp$/, '');
    byStem.set(stem, [...(byStem.get(stem) || []), output.width]);
  }
  for (const [stem, widths] of byStem) {
    assert.ok(widths.includes(360) && widths.includes(720), `${stem} needs the 360w and 720w derivatives`);
    if (!widths.includes(1080)) assert.ok(provenance.skipped1080w.includes(stem), `${stem} misses 1080w without a recorded reason`);
  }
});

test('pages reference only shipped derivatives, with 9:16 boxes, and none is left unused', () => builtPages((pages) => {
  const shipped = new Set(provenance.outputs.map((output) => output.file));
  const used = new Set();
  for (const [rel, html] of pages) {
    for (const img of html.matchAll(/<img\b[^>]*\/assets\/app\/[^>]*>/g)) {
      const tag = img[0];
      assert.match(tag, /width="360" height="640"/, `${rel}: app capture without the 9:16 box`);
      assert.match(tag, /alt="[^"]{40,}"/, `${rel}: app capture needs descriptive alt text`);
      assert.match(tag, /loading="lazy"/, `${rel}: app captures load lazily`);
      const files = [...tag.matchAll(/\/assets\/app\/([a-z0-9-]+\.webp)/g)].map((m) => m[1]);
      assert.ok(files.length >= 3, `${rel}: src plus a srcset with at least two widths`);
      for (const file of files) {
        assert.ok(shipped.has(file), `${rel} references ${file}, which is not in the provenance`);
        used.add(file);
      }
    }
    // The retired 1220 x 2712 captures and their frame are gone from every page.
    assert.doesNotMatch(html, /height="800"|viewBox="0 0 1220 2712"/, `${rel} still uses the old capture frame`);
    assert.doesNotMatch(html, /explore-dark|station-sources|community-feed-\d|stations-list-\d/, `${rel} references a retired capture`);
  }
  const unused = [...shipped].filter((file) => !used.has(file));
  assert.deepEqual(unused, [], 'derivatives no page uses must be removed with their provenance entry');
}));

test('every guide and calculator explains how to do it in the app, with real captures', () => builtPages((pages) => {
  const site = loadSite(root);
  const guides = site.guides.map((guide) => [guide.outputRel, 'Como fazer no aplicativo']);
  const tools = site.tools.map((tool) => [tool.outputRel, 'No aplicativo']);
  assert.equal(guides.length, 13);
  assert.equal(tools.length, 5);
  for (const [rel, heading] of [...guides, ...tools]) {
    const html = pages.get(rel);
    const start = html.search(new RegExp(`<h2[^>]*>(?:\\d+\\. )?${heading}</h2>`));
    assert.ok(start !== -1, `${rel} is missing the "${heading}" section`);
    const section = html.slice(start).split(/<h2[ >]/)[1];
    const items = section.match(/<ol>[\s\S]*?<\/ol>/)?.[0].match(/<li>/g) || [];
    assert.ok(items.length >= 2 && items.length <= 5, `${rel} needs 2 to 5 numbered steps, has ${items.length}`);
    const strip = section.match(/<div class="article-figures" role="group" aria-label="Capturas de tela do aplicativo" tabindex="0">[\s\S]*?<\/div>/);
    assert.ok(strip, `${rel} needs a strip of app captures`);
    const figures = strip[0].match(/<figure class="article-figure app-capture">/g) || [];
    assert.ok(figures.length >= 2 && figures.length <= 4, `${rel} strip has ${figures.length} captures`);
    assert.equal((strip[0].match(/<figcaption>Passo \d/g) || []).length, figures.length, `${rel} captions name their step`);
    // Honest framing: development status, capture date and no store promise.
    assert.match(section, /em desenvolvimento/, `${rel} states the app is in development`);
    assert.match(section, /17 de novembro de 2026/, `${rel} dates its captures`);
    assert.match(section, /href="\/como-usar\/[a-z-]+\/"/, `${rel} links the full tutorial`);
    assert.doesNotMatch(section, /baixe o app|baixe já|disponível na Google Play|melhor|nº 1|garantid/i, `${rel} makes a forbidden claim`);
  }
}));

test('the Markdown capture lookup only resolves the 720w derivative of a shipped capture', () => {
  const info = loadAppCaptures(root);
  const route = info('/assets/app/map-05-route-preview-720w.webp');
  assert.deepEqual(route, {
    width: 360,
    height: 640,
    candidates: [
      { src: '/assets/app/map-05-route-preview-360w.webp', width: 360 },
      { src: '/assets/app/map-05-route-preview-720w.webp', width: 720 },
    ],
  });
  assert.equal(info('/assets/app/fuel-prices-01-anp-average-cards-720w.webp').candidates.length, 3);
  assert.equal(info('/assets/app/fuel-prices-01-anp-average-cards-360w.webp'), null);
  assert.equal(info('/assets/app/contribute-price-05-phone-camera-720w.webp'), null);
  assert.equal(info('/assets/art/car-640w.97ea8555.webp'), null);
  // A root without provenance resolves nothing instead of throwing.
  assert.equal(loadAppCaptures(os.tmpdir())('/assets/app/map-05-route-preview-720w.webp'), null);
});
