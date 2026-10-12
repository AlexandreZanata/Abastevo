import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import vm from 'node:vm';
import { build } from '../scripts/build.mjs';

const root = path.resolve(import.meta.dirname, '..');

test('emitted theme follows system until explicit choice, survives blocked storage and synchronizes tabs', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-theme-'));
  try {
    build({ projectRoot: root, outDir });
    const jsFile = fs.readdirSync(path.join(outDir, 'assets')).find(f => /^main\.[a-f0-9]{8}\.js$/.test(f));
    const script = fs.readFileSync(path.join(outDir, 'assets', jsFile), 'utf8');
    function launch({ saved = null, blocked = false, systemDark = false } = {}) {
      const buttonEvents = {};
      const windowEvents = {};
      const mediaEvents = {};
      const dataset = {};
      const attributes = {};
      const meta = {};
      const stored = [];
      const icons = ['sun', 'moon'].map(name => ({ name, hidden: false, classList: { contains: value => value === `theme-icon-${name}` }, toggleAttribute(k, v) { this[k] = v; } }));
      const button = { hidden: true, querySelectorAll: () => icons, setAttribute: (k, v) => { attributes[k] = v; }, addEventListener: (k, v) => { buttonEvents[k] = v; } };
      const media = { matches: systemDark, addEventListener: (k, v) => { mediaEvents[k] = v; } };
      vm.runInNewContext(script, {
        document: {
          documentElement: { dataset }, readyState: 'loading', addEventListener() {},
          querySelectorAll: s => s === '[data-theme-toggle]' ? [button] : [],
          querySelector: () => ({ setAttribute: (k, v) => { meta[k] = v; } }),
        },
        window: { matchMedia: () => media, addEventListener: (k, v) => { windowEvents[k] = v; } },
        localStorage: {
          getItem() { if (blocked) throw new Error('blocked'); return saved; },
          setItem(k, v) { if (blocked) throw new Error('blocked'); stored.push([k, v]); },
        },
      });
      return { dataset, button, icons, attributes, meta, stored, buttonEvents, mediaEvents, windowEvents };
    }
    const auto = launch();
    assert.equal(auto.dataset.theme, 'light');
    assert.equal(auto.button.hidden, false);
    assert.deepEqual(auto.icons.map(i => i.hidden), [true, false]);
    auto.mediaEvents.change({ matches: true });
    assert.equal(auto.dataset.theme, 'dark');
    assert.equal(auto.attributes['aria-label'], 'Ativar tema claro');
    assert.equal(auto.meta.content, '#090909');
    assert.deepEqual(auto.icons.map(i => i.hidden), [false, true]);
    auto.buttonEvents.click();
    assert.equal(auto.dataset.theme, 'light');
    assert.deepEqual(auto.icons.map(i => i.hidden), [true, false]);
    assert.deepEqual(auto.stored, [['abastevo-theme', 'light']]);
    auto.mediaEvents.change({ matches: true });
    assert.equal(auto.dataset.theme, 'light', 'system cannot override explicit choice');
    auto.windowEvents.storage({ key: 'unrelated', newValue: 'dark' });
    assert.equal(auto.dataset.theme, 'light');
    auto.windowEvents.storage({ key: 'abastevo-theme', newValue: 'dark' });
    assert.equal(auto.dataset.theme, 'dark');
    auto.windowEvents.storage({ key: null, newValue: null });
    assert.equal(auto.dataset.theme, 'light', 'cleared preference returns to system');
    assert.equal(launch({ saved: 'dark' }).dataset.theme, 'dark');
    assert.equal(launch({ saved: 'invalid', systemDark: true }).dataset.theme, 'dark');
    const denied = launch({ blocked: true, systemDark: true });
    denied.buttonEvents.click();
    assert.equal(denied.dataset.theme, 'light');
    assert.equal(denied.attributes['aria-label'], 'Ativar tema escuro');
    assert.equal(denied.meta.content, '#ffffff');
  } finally { fs.rmSync(outDir, { recursive: true, force: true }); }
});

test('actual dark palette text, links and primary controls meet AA on every declared surface', () => {
  const css = fs.readFileSync(path.join(root, 'static/assets/styles.css'), 'utf8');
  const dark = css.match(/html\[data-theme="dark"\]\s*\{([^}]+)\}/)[1];
  const vars = Object.fromEntries([...dark.matchAll(/--([\w-]+):\s*(#[\da-f]{6})/g)].map(m => [m[1], m[2]]));
  for (const [name, hex] of Object.entries(vars)) {
    const channels = hex.slice(1).match(/../g);
    assert.equal(new Set(channels).size, 1, `${name} must be grayscale`);
  }
  function luminance(hex) {
    const rgb = hex.slice(1).match(/../g).map(x => parseInt(x, 16) / 255).map(x => x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4);
    return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722;
  }
  function check(fg, bg) {
    const [a, b] = [luminance(vars[fg]), luminance(vars[bg])].sort((a, b) => b - a);
    assert.ok((a + .05) / (b + .05) >= 4.5, `${fg}/${bg}`);
  }
  for (const surface of ['page', 'card', 'surface', 'footer-bg', 'tint-blue', 'tint-green']) {
    for (const foreground of ['ink', 'text-muted', 'link', 'green-text']) check(foreground, surface);
  }
  check('on-button', 'button-bg');
  check('on-button', 'button-hover');
  check('badge-text', 'tint-amber');
});
