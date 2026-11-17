import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('app captures enlarge in a script-built dialog and stay plain images without JavaScript', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-zoom-'));
  try {
    const result = build({ projectRoot: root, outDir });
    const js = fs.readFileSync(path.join(outDir, 'assets', result.fingerprintedJs), 'utf8');
    const css = fs.readFileSync(path.join(outDir, 'assets', result.fingerprintedCss), 'utf8');

    // 1. One enhancement covers the gallery, the tutorial steps and the guide strips.
    assert.match(js, /\.app-gallery img, \.tutorial-frame img, \.app-capture img/);
    // 2. Native modal dialog: Escape and focus trapping come from the browser;
    // the script adds the close button, the outside click and the focus return.
    assert.match(js, /showModal\(\)/);
    assert.match(js, /Fechar captura ampliada/);
    assert.match(js, /event\.target === dialog/);
    assert.match(js, /opener\?\.focus\(\)/);
    // 3. Keyboard users can open it: the capture becomes a focusable button.
    assert.match(js, /tabIndex = 0/);
    assert.match(js, /'Enter'/);
    // 4. Untrusted text never becomes markup.
    assert.doesNotMatch(js, /innerHTML|insertAdjacentHTML|document\.write/);
    // 5. The dialog is styled and the page behind it is blurred.
    assert.match(css, /\.capture-zoom::backdrop \{[^}]*backdrop-filter: blur\(/);
    assert.match(css, /\.capture-zoom-close \{/);

    // 6. Without JavaScript nothing changes: no dialog and no dead control in the HTML.
    for (const rel of ['index.html', 'como-usar/postos/index.html', 'guias/como-economizar-combustivel/index.html']) {
      const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
      assert.doesNotMatch(html, /<dialog|capture-zoom/, `${rel} must not ship dialog markup`);
      assert.match(html, /<img[^>]*\/assets\/app\//, `${rel} keeps its captures as images`);
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
