import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');
const CHAPTERS = ['escolher-cidade', 'precos-do-posto', 'comunidade', 'conta-e-privacidade', 'fotografar-preco', 'denunciar-abuso', 'veiculos', 'precos-anp', 'semanas-anp', 'historico', 'postos', 'minhas-contribuicoes', 'perfil-do-posto', 'configuracoes', 'offline-e-recuperacao'];

test('driver chapters render full text, true overlays and working prev/next', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-chapters-'));
  try {
    build({ projectRoot: root, outDir });
    for (const slug of CHAPTERS) {
      const lesson = JSON.parse(
        fs.readFileSync(path.join(root, 'static', 'como-usar', slug, 'lesson.json'), 'utf8'),
      );
      const page = fs.readFileSync(path.join(outDir, 'como-usar', slug, 'index.html'), 'utf8');

      // 1. Full text searchable without JS: every instruction/expected/recovery present.
      for (const step of lesson.steps) {
        assert.ok(page.includes(step.instructionPtBr), `${slug}/${step.stepId} instruction missing`);
        assert.ok(page.includes(step.expectedPtBr), `${slug}/${step.stepId} expected missing`);
        assert.ok(page.includes(step.recoveryPtBr), `${slug}/${step.stepId} recovery missing`);
        assert.ok(page.includes(step.altPtBr), `${slug}/${step.stepId} alt missing`);
      }

      // 2. Overlays use the native viewBox with true control rectangles.
      const svgs = [...page.matchAll(/<svg class="tutorial-overlay" viewBox="0 0 (\d+) (\d+)"[\s\S]*?<\/svg>/g)];
      const withTarget = lesson.steps.filter((s) => s.target);
      assert.equal(svgs.length, withTarget.length, `${slug} overlay count`);
      for (const [svg, step] of svgs.map((m, i) => [m, withTarget[i]])) {
        assert.equal(Number(svg[1]), step.nativeWidth);
        assert.equal(Number(svg[2]), step.nativeHeight);
        assert.ok(svg[0].includes(`<rect x="${step.target.x}" y="${step.target.y}"`), `${slug}/${step.stepId} rect`);
      }

      // 3. No app-write semantics: demonstration clicks stay links, never forms.
      assert.doesNotMatch(page, /<form/);
      assert.doesNotMatch(page, /submit|api\/|fetch\(/);

      // 4. Prev/next and hub links resolve to emitted pages.
      const nav = page.match(/<nav class="lesson-nav"[\s\S]*?<\/nav>/)[0];
      for (const href of [...nav.matchAll(/href="(\/[^"]*)"/g)].map((m) => m[1])) {
        const rel = href === '/' ? 'index.html' : href.slice(1) + 'index.html';
        assert.ok(fs.existsSync(path.join(outDir, rel)), `${slug} dead nav ${href}`);
      }
    }

    // 5. Sitemap lists the shipped chapters with stable canonical URLs.
    const sitemap = fs.readFileSync(path.join(outDir, 'sitemap.xml'), 'utf8');
    for (const slug of CHAPTERS) {
      assert.ok(sitemap.includes(`<loc>https://abastevo.com.br/como-usar/${slug}/</loc>`), slug);
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
