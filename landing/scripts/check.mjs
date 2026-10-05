import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const DEFAULT_ROOT = path.resolve(__dirname, '..');

export function checkDist(options = {}) {
  const root = options.projectRoot || DEFAULT_ROOT;
  const distDir = options.distDir || path.join(root, 'dist');
  const errors = [];

  if (!fs.existsSync(distDir)) {
    throw new Error(`dist directory does not exist at ${distDir}. Run build first.`);
  }

  // 1. Required files inventory
  const requiredFiles = [
    'index.html',
    'privacidade/index.html',
    'excluir-conta/index.html',
    'termos/index.html',
    '404.html',
    'robots.txt',
    'sitemap.xml',
    'site.webmanifest',
    '_headers',
    '_redirects',
  ];

  for (const file of requiredFiles) {
    const full = path.join(distDir, file);
    if (!fs.existsSync(full)) {
      errors.push(`Missing required file in dist: ${file}`);
    }
  }

  // 2. Locate fingerprinted CSS and JS
  const assetsDir = path.join(distDir, 'assets');
  let cssFile = null;
  let jsFile = null;

  if (fs.existsSync(assetsDir)) {
    const assetFiles = fs.readdirSync(assetsDir);
    cssFile = assetFiles.find((f) => /^styles\.[a-f0-9]{8}\.css$/.test(f));
    jsFile = assetFiles.find((f) => /^main\.[a-f0-9]{8}\.js$/.test(f));
  }

  if (!cssFile) {
    errors.push('Missing fingerprinted CSS (expected styles.[hash].css in dist/assets/)');
  }
  if (!jsFile) {
    errors.push('Missing fingerprinted JS (expected main.[hash].js in dist/assets/)');
  }

  // 3. Inspect HTML files for link integrity and required content
  const htmlPages = [
    'index.html',
    'privacidade/index.html',
    'excluir-conta/index.html',
    'termos/index.html',
    '404.html',
  ];

  for (const rel of htmlPages) {
    const full = path.join(distDir, rel);
    if (!fs.existsSync(full)) continue;
    const content = fs.readFileSync(full, 'utf8');

    // Check CSS and JS references if applicable
    if (rel !== '404.html') {
      if (cssFile && !content.includes(`/assets/${cssFile}`)) {
        errors.push(`${rel} does not reference fingerprinted CSS (/assets/${cssFile})`);
      }
      if (jsFile && !content.includes(`/assets/${jsFile}`)) {
        errors.push(`${rel} does not reference fingerprinted JS (/assets/${jsFile})`);
      }
    }

    // Check internal local links / resources
    const srcMatches = Array.from(content.matchAll(/(?:src|href)="(\/[^"#?]+)"/g));
    for (const match of srcMatches) {
      const resourcePath = match[1];
      // Skip root "/" or directories that have index.html
      let diskTarget;
      if (resourcePath === '/') {
        diskTarget = path.join(distDir, 'index.html');
      } else if (resourcePath.endsWith('/')) {
        diskTarget = path.join(distDir, resourcePath, 'index.html');
      } else {
        diskTarget = path.join(distDir, resourcePath);
      }
      if (!fs.existsSync(diskTarget)) {
        errors.push(`${rel} references nonexistent local resource: ${resourcePath}`);
      }
    }
  }

  // 4. Check essential content in index.html (works without JS)
  const indexHtmlPath = path.join(distDir, 'index.html');
  if (fs.existsSync(indexHtmlPath)) {
    const indexContent = fs.readFileSync(indexHtmlPath, 'utf8');
    if (!indexContent.includes('abastevo: preços de combustíveis com a comunidade')) {
      errors.push('index.html is missing main H1 text');
    }
    if (!indexContent.includes('https://github.com/AlexandreZanata/abastevo')) {
      errors.push('index.html is missing repository link');
    }
    if (!indexContent.includes('/privacidade/')) {
      errors.push('index.html is missing /privacidade/ link');
    }
    if (!indexContent.includes('/excluir-conta/')) {
      errors.push('index.html is missing /excluir-conta/ link');
    }
    if (!indexContent.includes('/termos/')) {
      errors.push('index.html is missing /termos/ link');
    }
  }

  // 5. Header baseline checks
  const headersPath = path.join(distDir, '_headers');
  if (fs.existsSync(headersPath)) {
    const headersContent = fs.readFileSync(headersPath, 'utf8');
    const requiredHeaderDirectives = [
      'X-Content-Type-Options: nosniff',
      'Referrer-Policy: strict-origin-when-cross-origin',
      'Permissions-Policy: camera=(), microphone=(), geolocation=()',
      'Content-Security-Policy:',
      'Strict-Transport-Security:',
      'Cache-Control: public, max-age=31536000, immutable',
    ];
    for (const directive of requiredHeaderDirectives) {
      if (!headersContent.includes(directive)) {
        errors.push(`_headers is missing required directive: ${directive}`);
      }
    }
  }

  // 6. Performance budget checks
  if (cssFile) {
    const cssContent = fs.readFileSync(path.join(assetsDir, cssFile));
    const gzippedCss = zlib.gzipSync(cssContent);
    const cssBudget = 25 * 1024; // 25 KiB
    if (gzippedCss.length > cssBudget) {
      errors.push(`CSS gzipped size (${gzippedCss.length} bytes) exceeds budget of ${cssBudget} bytes`);
    }
  }

  if (jsFile) {
    const jsContent = fs.readFileSync(path.join(assetsDir, jsFile));
    const gzippedJs = zlib.gzipSync(jsContent);
    const jsBudget = 10 * 1024; // 10 KiB
    if (gzippedJs.length > jsBudget) {
      errors.push(`JS gzipped size (${gzippedJs.length} bytes) exceeds budget of ${jsBudget} bytes`);
    }
  }

  // 7. No source leaks (.ts, .map)
  function assertNoLeaks(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        assertNoLeaks(full);
      } else {
        if (entry.name.endsWith('.ts') || entry.name.endsWith('.map')) {
          errors.push(`Forbidden leaked file in dist: ${full}`);
        }
      }
    }
  }
  assertNoLeaks(distDir);

  if (errors.length > 0) {
    throw new Error(`Landing check failed with ${errors.length} error(s):\n  - ${errors.join('\n  - ')}`);
  }

  return { ok: true, cssFile, jsFile };
}

// CLI execution
if (process.argv[1] === __filename) {
  const result = checkDist();
  console.log(`Landing check PASSED: dist is valid (CSS: ${result.cssFile}, JS: ${result.jsFile})`);
}
