import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const DEFAULT_ROOT = path.resolve(__dirname, '..');

// dist is served at /admin/* with the prefix stripped (Caddy handle_path), so
// every root-absolute reference must start with /admin/ and resolve to a dist
// file once the prefix is removed.
const MOUNT_PATH = '/admin';

function resolveMounted(distDir, owner, resourcePath, errors) {
  if (!resourcePath.startsWith(`${MOUNT_PATH}/`)) {
    errors.push(`${owner} references ${resourcePath} outside ${MOUNT_PATH}/`);
    return;
  }
  const stripped = resourcePath.slice(MOUNT_PATH.length);
  const diskTarget = stripped.endsWith('/')
    ? path.join(distDir, stripped, 'index.html')
    : path.join(distDir, stripped);
  if (!fs.existsSync(diskTarget)) {
    errors.push(`${owner} references nonexistent local resource: ${resourcePath}`);
  }
}

export function checkDist(options = {}) {
  const root = options.projectRoot || DEFAULT_ROOT;
  const distDir = options.distDir || path.join(root, 'dist');
  const errors = [];

  if (!fs.existsSync(distDir)) {
    throw new Error(`dist directory does not exist at ${distDir}. Run build first.`);
  }

  // 1. Required files inventory
  for (const file of ['login/index.html', 'accounts/index.html', 'contributors/index.html', 'audit/index.html', '404.html', '_headers', 'robots.txt']) {
    if (!fs.existsSync(path.join(distDir, file))) {
      errors.push(`Missing required file in dist: ${file}`);
    }
  }

  // 2. Locate fingerprinted assets
  const assetsDir = path.join(distDir, 'assets');
  let cssFile = null;
  let adminCssFile = null;
  const jsFiles = [];
  if (fs.existsSync(assetsDir)) {
    for (const f of fs.readdirSync(assetsDir)) {
      if (/^styles\.[a-f0-9]{8}\.css$/.test(f)) cssFile = f;
      else if (/^admin\.[a-f0-9]{8}\.css$/.test(f)) adminCssFile = f;
      else if (/^[a-z]+\.[a-f0-9]{8}\.js$/.test(f)) jsFiles.push(f);
    }
  }
  if (!cssFile) errors.push('Missing fingerprinted CSS (styles.[hash].css)');
  if (!adminCssFile) errors.push('Missing fingerprinted admin CSS (admin.[hash].css)');
  for (const expected of ['app', 'api', 'validators']) {
    if (!jsFiles.some((f) => f.startsWith(`${expected}.`))) {
      errors.push(`Missing fingerprinted JS (expected ${expected}.[hash].js)`);
    }
  }

  // 3. HTML pages: references, robots, titles
  for (const rel of ['login/index.html', 'accounts/index.html', 'contributors/index.html', 'audit/index.html', '404.html']) {
    const full = path.join(distDir, rel);
    if (!fs.existsSync(full)) continue;
    const content = fs.readFileSync(full, 'utf8');
    if (!content.match(/<meta name="robots" content="noindex, nofollow">/)) {
      errors.push(`${rel} is missing robots noindex meta`);
    }
    if (!content.match(/<title>[^<]+<\/title>/)) {
      errors.push(`${rel} is missing <title>`);
    }
    for (const match of content.matchAll(/(?:src|href)="(\/[^"#?]*)"/g)) {
      resolveMounted(distDir, rel, match[1], errors);
    }
    if (rel !== '404.html') {
      if (cssFile && !content.includes(`"${MOUNT_PATH}/assets/${cssFile}"`)) {
        errors.push(`${rel} does not reference fingerprinted CSS`);
      }
      if (adminCssFile && !content.includes(`"${MOUNT_PATH}/assets/${adminCssFile}"`)) {
        errors.push(`${rel} does not reference fingerprinted admin CSS`);
      }
      const appJs = jsFiles.find((f) => f.startsWith('app.'));
      if (appJs && !content.includes(`"${MOUNT_PATH}/assets/${appJs}"`)) {
        errors.push(`${rel} does not reference fingerprinted app JS`);
      }
    }
  }

  // Login page keeps the password discipline in markup
  const loginHtml = path.join(distDir, 'login/index.html');
  if (fs.existsSync(loginHtml)) {
    const content = fs.readFileSync(loginHtml, 'utf8');
    if (!content.includes('type="password"') || !content.includes('autocomplete="off"')) {
      errors.push('login page must use a password input with autocomplete off');
    }
  }

  // 4. Header baseline checks
  const headersPath = path.join(distDir, '_headers');
  if (fs.existsSync(headersPath)) {
    const headersContent = fs.readFileSync(headersPath, 'utf8');
    for (const directive of [
      'X-Content-Type-Options: nosniff',
      'X-Robots-Tag: noindex, nofollow',
      'Referrer-Policy: no-referrer',
      "Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'",
      'Cache-Control: no-store',
      'Cache-Control: public, max-age=31536000, immutable',
    ]) {
      if (!headersContent.includes(directive)) {
        errors.push(`_headers is missing required directive: ${directive}`);
      }
    }
  }

  // robots.txt must disallow everything
  const robotsPath = path.join(distDir, 'robots.txt');
  if (fs.existsSync(robotsPath)) {
    const robotsContent = fs.readFileSync(robotsPath, 'utf8');
    if (!robotsContent.includes('Disallow: /')) {
      errors.push('robots.txt must disallow everything');
    }
  }

  // 5. Budgets: CSS/JS gzip, no remote fonts
  if (cssFile) {
    const gzipped = zlib.gzipSync(fs.readFileSync(path.join(assetsDir, cssFile)));
    if (gzipped.length > 25 * 1024) {
      errors.push(`CSS gzipped size (${gzipped.length} bytes) exceeds 25 KiB budget`);
    }
    const cssText = fs.readFileSync(path.join(assetsDir, cssFile), 'utf8');
    for (const match of cssText.matchAll(/url\(\s*['"]?(\/[^)'"\s]+)['"]?\s*\)/g)) {
      resolveMounted(distDir, `assets/${cssFile}`, match[1], errors);
    }
    if (/https?:\/\/(?:fonts\.googleapis|fonts\.gstatic)/.test(cssText)) {
      errors.push('CSS references a remote font service');
    }
  }
  for (const js of jsFiles) {
    const gzipped = zlib.gzipSync(fs.readFileSync(path.join(assetsDir, js)));
    if (gzipped.length > 10 * 1024) {
      errors.push(`JS ${js} gzipped size (${gzipped.length} bytes) exceeds 10 KiB budget`);
    }
  }

  // 6. No source leaks (.ts, .map)
  function assertNoLeaks(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        assertNoLeaks(full);
      } else if (entry.name.endsWith('.ts') || entry.name.endsWith('.map')) {
        errors.push(`Forbidden leaked file in dist: ${full}`);
      }
    }
  }
  assertNoLeaks(distDir);

  if (errors.length > 0) {
    throw new Error(`Admin-web check failed with ${errors.length} error(s):\n  - ${errors.join('\n  - ')}`);
  }

  return { ok: true, cssFile, adminCssFile, jsFiles };
}

// CLI execution
if (process.argv[1] === __filename) {
  const result = checkDist();
  console.log(`Admin-web check PASSED: dist is valid (CSS: ${result.cssFile}, JS: ${result.jsFiles.join(', ')})`);
}
