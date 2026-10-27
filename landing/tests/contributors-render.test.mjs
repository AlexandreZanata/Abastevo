import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

const compiled = (() => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-contribuidores-js-'));
  const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
  execFileSync(fs.existsSync(localTsc) ? localTsc : 'tsc', ['--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir], {
    cwd: root,
    stdio: 'pipe',
  });
  const modPath = path.join(tmpDir, 'contributors.js');
  assert.ok(fs.existsSync(modPath), 'contributors.js must compile');
  return { tmpDir, modPath };
})();

test.after(() => {
  fs.rmSync(compiled.tmpDir, { recursive: true, force: true });
});

async function contributorsModule() {
  return import(pathToFileURL(compiled.modPath).href);
}

function makeDoc() {
  const elements = [];
  const doc = {
    created: elements,
    createElement(tag) {
      const el = {
        tag,
        textContent: '',
        children: [],
        attrs: {},
        appendChild(child) { this.children.push(child); },
        setAttribute(name, value) { this.attrs[name] = value; },
      };
      elements.push(el);
      return el;
    },
    getElementById: () => null,
    querySelector: () => null,
  };
  const container = doc.createElement('div');
  return { doc, container };
}

function flatten(el) {
  const out = [];
  const visit = (node) => {
    out.push(node);
    for (const child of node.children || []) visit(child);
  };
  visit(el);
  return out;
}

test('xss payload names stay text, links pass the allowlist', async () => {
  const mod = await contributorsModule();
  const { doc, container } = makeDoc();
  const rendered = mod.renderContributors(doc, container, [
    {
      display_name: '<img src=x onerror=alert(1)>',
      links: [
        { platform: 'github', handle: 'octocat', url: 'https://github.com/octocat' },
        { platform: 'github', handle: 'evil', url: 'https://evil.com/evil' },
        { platform: 'myspace', handle: 'x', url: 'https://myspace.com/x' },
      ],
      github_login: 'octocat',
      contributor_since: '2026-03',
      roles: ['Suporte'],
    },
  ]);
  assert.equal(rendered, 1);
  const nodes = flatten(container);
  assert.ok(nodes.some((n) => n.textContent === '<img src=x onerror=alert(1)>'), 'payload kept as text');
  assert.ok(!nodes.some((n) => n.tag === 'script'), 'no script nodes');
  for (const node of nodes) {
    assert.ok(!/[<>]/.test(node.tag), `markup smuggled into tag: ${node.tag}`);
    for (const key of Object.keys(node.attrs)) {
      assert.ok(!/^on/i.test(key), `event handler attribute: ${key}`);
    }
  }
  const anchors = nodes.filter((n) => n.tag === 'a');
  assert.equal(anchors.length, 1, 'only the allowlisted link renders');
  assert.equal(anchors[0].attrs.href, 'https://github.com/octocat');
  assert.equal(anchors[0].attrs.rel, 'noopener noreferrer nofollow ugc');
  assert.equal(anchors[0].attrs.target, '_blank');
  for (const value of anchors.flatMap((a) => Object.values(a.attrs))) {
    assert.ok(!/^javascript:/i.test(value), `javascript url: ${value}`);
  }
});

test('malformed items are skipped and bad payloads render nothing', async () => {
  const mod = await contributorsModule();
  const { doc, container } = makeDoc();
  const rendered = mod.renderContributors(doc, container, [
    { display_name: '  ', links: [], roles: [] },
    null,
    'nope',
    { display_name: 'Bia', links: [], roles: [] },
  ]);
  assert.equal(rendered, 1);
  assert.equal(mod.renderContributors(doc, container, { items: [] }), 0);
});

test('disabled without meta and inert on bad payload', async () => {
  const mod = await contributorsModule();
  assert.equal(mod.apiOrigin({ querySelector: () => null }), '');
  let fetched = false;
  await mod.setupContributorsList({
    document: {
      createElement: () => { throw new Error('must stay inert'); },
      getElementById: () => null,
      querySelector: () => null,
    },
    fetchFn: async () => { fetched = true; throw new Error('must not fetch'); },
  });
  assert.equal(fetched, false);
});

test('compiled asset carries no unsafe DOM calls', async () => {
  const asset = fs.readFileSync(compiled.modPath, 'utf8');
  assert.doesNotMatch(asset, /innerHTML|outerHTML|insertAdjacentHTML|document\.write|eval\(/);
});
