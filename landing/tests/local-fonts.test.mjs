import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('emitted typography loads genuine local WOFF2 with licenses and no remote font service', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-fonts-'));
  try {
    const result = build({ projectRoot: root, outDir });
    const css = fs.readFileSync(path.join(outDir, 'assets', result.fingerprintedCss), 'utf8');
    const references = [...css.matchAll(/url\(['"]?(\/assets\/fonts\/[^)'"\s]+\.woff2)['"]?\)/g)];
    assert.equal(references.length, 4, 'both variable fonts need Latin and Latin Extended files');
    for (const [, reference] of references) {
      const emitted = fs.readFileSync(path.join(outDir, reference));
      assert.equal(emitted.subarray(0, 4).toString(), 'wOF2', `${reference} must be a WOFF2 font`);
    }
    for (const license of ['OFL-Manrope.txt', 'OFL-Source-Sans-3.txt']) {
      assert.match(fs.readFileSync(path.join(outDir, 'assets/fonts', license), 'utf8'), /SIL Open Font License/);
    }
    assert.match(css, /font-family:\s*'Manrope'/);
    assert.match(css, /font-family:\s*'Source Sans 3'/);
    assert.match(css, /font-display:\s*swap/);
    assert.doesNotMatch(css, /https?:\/\/(?:fonts\.googleapis|fonts\.gstatic)/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
