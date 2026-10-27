// Minimal DOM double for driving the compiled app.js in Node without a
// dependency. It models only what src/app.ts uses: elements by id from the
// shipped shell, createElement, a child tree, textContent, form/input
// properties, bubbling events, a tiny selector subset and window.history.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

// Selector subset: tag, [attr] or [attr="value"], e.g. 'input[name="override"]'.
function matches(node, selector) {
  const match = /^([a-z]*)(?:\[([a-z-]+)(?:="([^"]*)")?\])?$/.exec(selector);
  if (match === null) {
    throw new Error(`fake-dom: unsupported selector ${selector}`);
  }
  const [, tag, attr, value] = match;
  if (tag !== '' && node.tagName !== tag.toUpperCase()) return false;
  if (attr === undefined) return true;
  const actual = node.getAttribute(attr) ?? (typeof node[attr] === 'string' && node[attr] !== '' ? node[attr] : null);
  if (actual === null) return false;
  return value === undefined || actual === value;
}

export class FakeElement {
  constructor(tag) {
    this.tagName = tag.toUpperCase();
    this.children = [];
    this.parentNode = null;
    this.listeners = new Map();
    this.attributes = new Map();
    this.ownText = '';
    this.hidden = false;
    this.disabled = false;
    this.checked = false;
    this.value = '';
    this.type = '';
    this.name = '';
    this.className = '';
    this.placeholder = '';
  }

  get firstChild() {
    return this.children[0] ?? null;
  }

  appendChild(child) {
    if (child.parentNode !== null) child.parentNode.removeChild(child);
    child.parentNode = this;
    this.children.push(child);
    return child;
  }

  removeChild(child) {
    const index = this.children.indexOf(child);
    if (index < 0) throw new Error('fake-dom: not a child');
    this.children.splice(index, 1);
    child.parentNode = null;
    return child;
  }

  get textContent() {
    return this.ownText + this.children.map((child) => child.textContent).join('');
  }

  set textContent(value) {
    for (const child of this.children) child.parentNode = null;
    this.children = [];
    this.ownText = String(value);
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }

  getAttribute(name) {
    return this.attributes.has(name) ? this.attributes.get(name) : null;
  }

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type).push(listener);
  }

  dispatch(type) {
    const event = {
      type,
      target: this,
      defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; },
    };
    for (let node = this; node !== null; node = node.parentNode) {
      for (const listener of node.listeners.get(type) ?? []) listener(event);
    }
    return event;
  }

  // Browser semantics: a disabled control ignores user clicks.
  click() {
    if (this.disabled) return;
    this.dispatch('click');
    if (this.tagName === 'BUTTON' && this.type === 'submit') {
      this.closest('form')?.dispatch('submit');
    }
  }

  closest(selector) {
    for (let node = this; node !== null; node = node.parentNode) {
      if (matches(node, selector)) return node;
    }
    return null;
  }

  querySelectorAll(selector) {
    const found = [];
    const visit = (node) => {
      for (const child of node.children) {
        if (matches(child, selector)) found.push(child);
        visit(child);
      }
    };
    visit(this);
    return found;
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] ?? null;
  }
}

// Builds a document whose ids mirror the shipped shell, so a renamed or
// missing id in static/app/index.html fails the app tests too.
export function createDocument(shellHtml) {
  const byId = new Map();
  for (const match of shellHtml.matchAll(/<([a-z0-9]+)\b([^>]*)>/g)) {
    const [, tag, attrs] = match;
    const id = /\bid="([^"]+)"/.exec(attrs);
    if (id === null) continue;
    const node = new FakeElement(tag);
    node.setAttribute('id', id[1]);
    node.hidden = /\bhidden\b/.test(attrs);
    const type = /\btype="([^"]+)"/.exec(attrs);
    if (type !== null) node.type = type[1];
    const value = /\bvalue="([^"]*)"/.exec(attrs);
    if (value !== null) node.value = value[1];
    byId.set(id[1], node);
  }
  return {
    readyState: 'complete',
    getElementById: (id) => byId.get(id) ?? null,
    createElement: (tag) => new FakeElement(tag),
    addEventListener: () => {},
  };
}

export function createWindow(initialPath) {
  const popstate = [];
  const location = { pathname: initialPath };
  return {
    location,
    history: {
      pushState: (_state, _title, url) => { location.pathname = url; },
      replaceState: (_state, _title, url) => { location.pathname = url; },
    },
    addEventListener: (type, listener) => { if (type === 'popstate') popstate.push(listener); },
    confirm: () => true,
  };
}

export function jsonResponse(status, body, extraHeaders = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...extraHeaders },
  });
}

// Compiles src/ once per test file; each mount imports app.js with a unique
// query so module state (session, cursors) starts fresh.
export function compileApp(root) {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-app-'));
  const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
  execFileSync(fs.existsSync(localTsc) ? localTsc : 'tsc', [
    '--project', path.join(root, 'tsconfig.json'), '--outDir', outDir,
  ], { cwd: root, stdio: 'pipe' });
  let mounts = 0;
  return {
    outDir,
    async mount({ fetchHandler, path: initialPath = '/admin/login/' }) {
      const document = createDocument(fs.readFileSync(path.join(root, 'static', 'app', 'index.html'), 'utf8'));
      const window = createWindow(initialPath);
      const calls = [];
      globalThis.document = document;
      globalThis.window = window;
      globalThis.fetch = async (input, init = {}) => {
        calls.push({ input, method: init.method ?? 'GET', body: init.body === undefined ? undefined : JSON.parse(init.body) });
        return fetchHandler(input, init, calls);
      };
      mounts += 1;
      await import(`${pathToFileURL(path.join(outDir, 'app.js')).href}?mount=${mounts}`);
      return { document, window, calls, byId: (id) => document.getElementById(id) };
    },
    cleanup() {
      fs.rmSync(outDir, { recursive: true, force: true });
    },
  };
}

// Lets pending fetch promises and their continuations run.
export async function settle(rounds = 10) {
  for (let i = 0; i < rounds; i += 1) {
    await new Promise((resolve) => setImmediate(resolve));
  }
}
