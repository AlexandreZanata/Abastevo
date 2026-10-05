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
    'llms.txt',
    '.well-known/security.txt',
    'favicon.ico',
    'assets/brand/favicon.svg',
    'assets/brand/apple-touch-icon.png',
    'assets/brand/icon-192.png',
    'assets/brand/icon-512.png',
    'assets/brand/og-image-v1.png',
  ];

  for (const file of requiredFiles) {
    const full = path.join(distDir, file);
    if (!fs.existsSync(full)) {
      errors.push(`Missing required file in dist: ${file}`);
    }
  }

  // Check OG image budget (<= 300 KiB)
  const ogImgPath = path.join(distDir, 'assets', 'brand', 'og-image-v1.png');
  if (fs.existsSync(ogImgPath)) {
    const stat = fs.statSync(ogImgPath);
    if (stat.size > 300 * 1024) {
      errors.push(`og-image-v1.png (${stat.size} bytes) exceeds 300 KiB budget`);
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

  // 3. Inspect HTML files for link integrity, metadata bounds, and JSON-LD
  const htmlPages = [
    { rel: 'index.html', isIndexable: true },
    { rel: 'privacidade/index.html', isIndexable: true },
    { rel: 'excluir-conta/index.html', isIndexable: true },
    { rel: 'termos/index.html', isIndexable: true },
    { rel: '404.html', isIndexable: false },
  ];

  for (const { rel, isIndexable } of htmlPages) {
    const full = path.join(distDir, rel);
    if (!fs.existsSync(full)) continue;
    const content = fs.readFileSync(full, 'utf8');

    // Title length check (30 - 65 chars)
    const titleMatch = content.match(/<title>([^<]+)<\/title>/);
    if (!titleMatch) {
      errors.push(`${rel} is missing <title> tag`);
    } else {
      const titleLen = titleMatch[1].trim().length;
      if (titleLen < 25 || titleLen > 65) {
        errors.push(`${rel} <title> length (${titleLen}) outside target bounds [25, 65]`);
      }
    }

    // Description length check (110 - 165 chars for indexable pages)
    if (isIndexable) {
      const descMatch = content.match(/<meta\s+name="description"\s+content="([^"]+)"/);
      if (!descMatch) {
        errors.push(`${rel} is missing meta description`);
      } else {
        const descLen = descMatch[1].trim().length;
        if (descLen < 110 || descLen > 165) {
          errors.push(`${rel} meta description length (${descLen}) outside target bounds [110, 165]`);
        }
      }
    }

    // Check JSON-LD validity
    const jsonLdMatches = Array.from(content.matchAll(/<script\s+type="application\/ld\+json">([\s\S]*?)<\/script>/g));
    for (const jsonLdMatch of jsonLdMatches) {
      const rawJson = jsonLdMatch[1].trim();
      try {
        const parsed = JSON.parse(rawJson);
        if (!parsed['@context']) {
          errors.push(`${rel} JSON-LD missing @context`);
        }
        // Graph ID uniqueness
        if (parsed['@graph'] && Array.isArray(parsed['@graph'])) {
          const ids = new Set();
          for (const node of parsed['@graph']) {
            if (node['@id']) {
              if (ids.has(node['@id'])) {
                errors.push(`${rel} JSON-LD contains duplicate @id: ${node['@id']}`);
              }
              ids.add(node['@id']);
            }
          }
        }
      } catch (err) {
        errors.push(`${rel} has malformed JSON-LD: ${err.message}`);
      }
    }

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

  // 4. Sitemap and Robots validation
  const sitemapPath = path.join(distDir, 'sitemap.xml');
  if (fs.existsSync(sitemapPath)) {
    const sitemapContent = fs.readFileSync(sitemapPath, 'utf8');
    if (!sitemapContent.startsWith('<?xml') || !sitemapContent.includes('<urlset')) {
      errors.push('sitemap.xml is not valid XML');
    }
    if (sitemapContent.includes('404.html')) {
      errors.push('sitemap.xml must not include 404 page');
    }
    if (sitemapContent.includes('<lastmod>')) {
      errors.push('sitemap.xml contains fabricated lastmod');
    }
    for (const p of ['/', '/privacidade/', '/excluir-conta/', '/termos/']) {
      if (!sitemapContent.includes(`<loc>https://`)) {
        errors.push('sitemap.xml loc entries must be absolute HTTPS URLs');
      }
    }
  }

  const robotsPath = path.join(distDir, 'robots.txt');
  if (fs.existsSync(robotsPath)) {
    const robotsContent = fs.readFileSync(robotsPath, 'utf8');
    if (!robotsContent.includes('Sitemap:')) {
      errors.push('robots.txt missing Sitemap directive');
    }
    if (!robotsContent.includes('User-agent:')) {
      errors.push('robots.txt missing User-agent directive');
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
