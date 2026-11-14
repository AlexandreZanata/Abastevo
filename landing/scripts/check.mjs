import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { isPortalRoute } from './content/data.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const DEFAULT_ROOT = path.resolve(__dirname, '..');
const HTML_PAGE_BUDGET = 200 * 1024;
// Bundles copied into dist by the release workflow after this check runs.
const FOREIGN_BUNDLES = new Set(['admin', 'empresas']);

export function listHtmlPages(distDir) {
  const pages = [];
  const walk = (rel) => {
    const entries = fs.readdirSync(path.join(distDir, rel), { withFileTypes: true })
      .sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      const child = rel === '' ? entry.name : `${rel}/${entry.name}`;
      if (entry.isDirectory()) {
        if (rel === '' && (FOREIGN_BUNDLES.has(entry.name) || entry.name === 'assets')) continue;
        walk(child);
      } else if (entry.name.endsWith('.html')) {
        pages.push(child);
      }
    }
  };
  walk('');
  return pages;
}

export function routeForHtml(rel) {
  if (rel === 'index.html') return '/';
  if (rel.endsWith('/index.html')) return `/${rel.slice(0, -'index.html'.length)}`;
  return `/${rel}`;
}

function directorySize(dir) {
  let total = 0;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    total += entry.isDirectory() ? directorySize(full) : fs.statSync(full).size;
  }
  return total;
}

export function checkDist(options = {}) {
  const root = options.projectRoot || DEFAULT_ROOT;
  const distDir = options.distDir || path.join(root, 'dist');
  const origin = (options.origin || 'https://abastevo.com.br').replace(/\/$/, '');
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
    'como-usar/index.html',
    'como-usar/escolher-cidade/index.html',
    'como-usar/precos-do-posto/index.html',
    'como-usar/comunidade/index.html',
    'como-usar/conta-e-privacidade/index.html',
    'como-usar/fotografar-preco/index.html',
    'como-usar/denunciar-abuso/index.html',
    'como-usar/veiculos/index.html',
    'como-usar/offline-e-recuperacao/index.html',
    'como-usar/ajuda-e-regras/index.html',
    'como-usar/precos-anp/index.html',
    'como-usar/semanas-anp/index.html',
    'como-usar/historico/index.html',
    'como-usar/postos/index.html',
    'como-usar/minhas-contribuicoes/index.html',
    'como-usar/perfil-do-posto/index.html',
    'como-usar/configuracoes/index.html',
    'contribuidores/index.html',
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

  // 3. Inspect every emitted landing page: metadata bounds, unique titles and
  // descriptions, self-canonical on the configured origin, JSON-LD validity,
  // asset references and internal links. Portal routes are owned by the
  // separately generated portal artifact (backend portalgen) and merged at the
  // static edge, so links into them are allowlisted as external to the landing.
  const htmlPages = listHtmlPages(distDir);
  const seenTitles = new Map();
  const seenDescriptions = new Map();
  const indexableRoutes = new Set();
  const homeHtml = fs.existsSync(path.join(distDir, 'index.html')) ? fs.readFileSync(path.join(distDir, 'index.html'), 'utf8') : '';
  let toolsFile = null;

  for (const rel of htmlPages) {
    const full = path.join(distDir, rel);
    const content = fs.readFileSync(full, 'utf8');
    const route = routeForHtml(rel);
    const isIndexable = !/<meta\s+name="robots"\s+content="[^"]*noindex/i.test(content);
    if (isIndexable) indexableRoutes.add(route);

    if (content.length > HTML_PAGE_BUDGET) {
      errors.push(`${rel} (${content.length} bytes) exceeds the ${HTML_PAGE_BUDGET}-byte HTML page budget`);
    }

    // Title length check (25 - 65 chars)
    const titleMatch = content.match(/<title>([^<]+)<\/title>/);
    if (!titleMatch) {
      errors.push(`${rel} is missing <title> tag`);
    } else {
      const title = titleMatch[1].trim();
      if (title.length < 25 || title.length > 65) {
        errors.push(`${rel} <title> length (${title.length}) outside target bounds [25, 65]`);
      }
      if (isIndexable) {
        if (seenTitles.has(title)) errors.push(`${rel} duplicates the <title> of ${seenTitles.get(title)}: "${title}"`);
        seenTitles.set(title, rel);
      }
    }

    if (isIndexable) {
      // Description length check (110 - 165 chars for indexable pages)
      const descMatch = content.match(/<meta\s+name="description"\s+content="([^"]+)"/);
      if (!descMatch) {
        errors.push(`${rel} is missing meta description`);
      } else {
        const desc = descMatch[1].trim();
        if (desc.length < 110 || desc.length > 165) {
          errors.push(`${rel} meta description length (${desc.length}) outside target bounds [110, 165]`);
        }
        if (seenDescriptions.has(desc)) errors.push(`${rel} duplicates the meta description of ${seenDescriptions.get(desc)}`);
        seenDescriptions.set(desc, rel);
      }

      // Self-canonical on the configured origin, mirrored by og:url.
      const canonicals = [...content.matchAll(/<link\s+rel="canonical"\s+href="([^"]+)"/g)].map((m) => m[1]);
      const expected = `${origin}${route}`;
      if (canonicals.length !== 1) {
        errors.push(`${rel} must declare exactly one canonical link (found ${canonicals.length})`);
      } else if (canonicals[0] !== expected) {
        errors.push(`${rel} canonical ${canonicals[0]} is not the self URL ${expected}`);
      }
      const ogUrl = content.match(/<meta\s+property="og:url"\s+content="([^"]+)"/);
      if (ogUrl && ogUrl[1] !== expected) {
        errors.push(`${rel} og:url ${ogUrl[1]} does not match ${expected}`);
      }
    } else if (/rel="canonical"/.test(content)) {
      errors.push(`${rel} is noindex and must not declare a canonical URL`);
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
            if (!node['@type']) errors.push(`${rel} JSON-LD graph node without @type`);
            if (node['@id']) {
              if (ids.has(node['@id'])) {
                errors.push(`${rel} JSON-LD contains duplicate @id: ${node['@id']}`);
              }
              ids.add(node['@id']);
            }
          }
        } else if (!parsed['@type']) {
          errors.push(`${rel} JSON-LD without @type`);
        }
      } catch (err) {
        errors.push(`${rel} has malformed JSON-LD: ${err.message}`);
      }
    }

    // No inline executable scripts (CSP script-src 'self'); JSON-LD is data.
    for (const script of content.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/g)) {
      const attrs = script[1];
      if (/type="application\/ld\+json"/.test(attrs)) continue;
      if (!/\bsrc="\/assets\/[a-z-]+\.[a-f0-9]{8}\.js"/.test(attrs) || script[2].trim() !== '') {
        errors.push(`${rel} has an inline or non-fingerprinted script`);
      }
    }
    if (/\son[a-z]+\s*=\s*"/i.test(content)) {
      errors.push(`${rel} has an inline event handler attribute`);
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
    const toolsRef = content.match(/\/assets\/(tools\.[a-f0-9]{8}\.js)/);
    if (toolsRef) toolsFile = toolsRef[1];
    if (/data-tool="/.test(content) && !toolsRef) {
      errors.push(`${rel} has a calculator form but does not load the fingerprinted tools module`);
    }

    // In-page anchors resolve.
    for (const anchor of content.matchAll(/href="#([^"]+)"/g)) {
      if (!content.includes(`id="${anchor[1]}"`)) errors.push(`${rel} has a dead in-page anchor #${anchor[1]}`);
    }

    // Internal links and resources resolve to emitted files or portal routes.
    const srcMatches = Array.from(content.matchAll(/(?:src|href)="(\/(?!\/)[^"]*)"/g));
    for (const match of srcMatches) {
      const [pathAndQuery, hash] = match[1].split('#');
      const resourcePath = pathAndQuery.split('?')[0];
      if (resourcePath === '/' && hash !== undefined && hash !== '') {
        if (!homeHtml.includes(`id="${hash}"`)) errors.push(`${rel} links to a missing home anchor /#${hash}`);
      }
      if (isPortalRoute(resourcePath)) continue;
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

  if (!toolsFile) {
    errors.push('Missing fingerprinted calculators module (expected tools.[hash].js referenced by tool pages)');
  }

  // 4. Sitemap and Robots validation
  const sitemapPath = path.join(distDir, 'sitemap.xml');
  let sitemapUrls = 0;
  if (fs.existsSync(sitemapPath)) {
    const sitemapContent = fs.readFileSync(sitemapPath, 'utf8');
    if (!sitemapContent.startsWith('<?xml') || !sitemapContent.includes('<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">')) {
      errors.push('sitemap.xml is not a valid urlset');
    }
    if (sitemapContent.includes('404.html')) {
      errors.push('sitemap.xml must not include 404 page');
    }
    if (/<(priority|changefreq)>/.test(sitemapContent)) {
      errors.push('sitemap.xml must not declare priority or changefreq');
    }
    const entries = [...sitemapContent.matchAll(/<url>\s*<loc>([^<]+)<\/loc>\s*(?:<lastmod>([^<]+)<\/lastmod>\s*)?<\/url>/g)];
    const blocks = (sitemapContent.match(/<url>/g) || []).length;
    if (entries.length !== blocks) errors.push('sitemap.xml has malformed <url> entries');
    sitemapUrls = entries.length;
    const listed = new Set();
    for (const [, loc, lastmod] of entries) {
      if (!loc.startsWith(`${origin}/`)) {
        errors.push(`sitemap.xml loc ${loc} is not on the configured origin ${origin}`);
        continue;
      }
      const route = loc.slice(origin.length);
      if (listed.has(route)) errors.push(`sitemap.xml lists ${route} twice`);
      listed.add(route);
      if (!indexableRoutes.has(route)) errors.push(`sitemap.xml lists ${route}, which is not an emitted indexable landing page`);
      if (!lastmod || !/^\d{4}-\d{2}-\d{2}$/.test(lastmod) || Number.isNaN(Date.parse(`${lastmod}T00:00:00Z`))) {
        errors.push(`sitemap.xml entry ${route} needs a YYYY-MM-DD lastmod`);
      }
    }
    for (const route of indexableRoutes) {
      if (!listed.has(route)) errors.push(`indexable page ${route} is missing from sitemap.xml`);
    }
  }

  const robotsPath = path.join(distDir, 'robots.txt');
  if (fs.existsSync(robotsPath)) {
    const robotsContent = fs.readFileSync(robotsPath, 'utf8');
    if (!robotsContent.includes(`Sitemap: ${origin}/sitemap.xml`)) {
      errors.push(`robots.txt missing Sitemap directive for ${origin}`);
    }
    if (!robotsContent.includes('User-agent:')) {
      errors.push('robots.txt missing User-agent directive');
    }
  }

  // llms.txt lists the content hubs and every generated page.
  const llmsPath = path.join(distDir, 'llms.txt');
  if (fs.existsSync(llmsPath)) {
    const llms = fs.readFileSync(llmsPath, 'utf8');
    for (const route of ['/guias/', '/ferramentas/', '/sobre/', '/politica-editorial/']) {
      if (!llms.includes(`${origin}${route}`)) errors.push(`llms.txt does not list ${route}`);
    }
    if (llms.includes('{{')) errors.push('llms.txt has unresolved placeholders');
  }

  // 4. Check essential content in index.html (works without JS)
  const indexHtmlPath = path.join(distDir, 'index.html');
  if (fs.existsSync(indexHtmlPath)) {
    const indexContent = fs.readFileSync(indexHtmlPath, 'utf8');
    if (!indexContent.includes('Mais informação<br>para quem vive<br><span>na estrada.</span>')) {
      errors.push('index.html is missing main H1 text');
    }
    if (!indexContent.includes('class="store-btn btn-github"') || !indexContent.includes('Código com atribuição visível')) {
      errors.push('index.html is missing primary GitHub action');
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

  if (toolsFile && fs.existsSync(path.join(assetsDir, toolsFile))) {
    const gzippedTools = zlib.gzipSync(fs.readFileSync(path.join(assetsDir, toolsFile)));
    const toolsBudget = 10 * 1024; // 10 KiB, loaded only by calculator pages
    if (gzippedTools.length > toolsBudget) {
      errors.push(`Calculators JS gzipped size (${gzippedTools.length} bytes) exceeds budget of ${toolsBudget} bytes`);
    }
  }

  // Self-hosted font payload (LR29: measured 128,916 bytes on 2026-10-11).
  const fontsDir = path.join(assetsDir, 'fonts');
  if (fs.existsSync(fontsDir)) {
    let fontBytes = 0;
    for (const item of fs.readdirSync(fontsDir)) {
      if (item.endsWith('.woff2')) {
        fontBytes += fs.statSync(path.join(fontsDir, item)).size;
      }
    }
    const fontBudget = 150 * 1024; // 150 KiB
    if (fontBytes > fontBudget) {
      errors.push(`Font payload (${fontBytes} bytes) exceeds budget of ${fontBudget} bytes`);
    }
  }

  // Real media derivatives only: downscaled WebP, never masters (LR29).
  const appDir = path.join(assetsDir, 'app');
  if (fs.existsSync(appDir)) {
    const derivativeBudget = 120 * 1024; // 120 KiB per file; largest measured 97,168 bytes
    for (const item of fs.readdirSync(appDir)) {
      const full = path.join(appDir, item);
      if (item.endsWith('.png')) {
        errors.push(`App master must not ship: ${item}`);
      }
      if (item.endsWith('.webp') && fs.statSync(full).size > derivativeBudget) {
        errors.push(`App derivative ${item} exceeds budget of ${derivativeBudget} bytes`);
      }
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

  return { ok: true, cssFile, jsFile, toolsFile, pages: htmlPages.length, sitemapUrls, totalBytes: directorySize(distDir) };
}

// CLI execution
if (process.argv[1] === __filename) {
  const args = process.argv.slice(2);
  const options = {};
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--origin' && args[i + 1]) options.origin = args[++i];
    else if (args[i] === '--dist-dir' && args[i + 1]) options.distDir = path.resolve(args[++i]);
  }
  const result = checkDist(options);
  console.log(`Landing check PASSED: dist is valid (CSS: ${result.cssFile}, JS: ${result.jsFile}, tools: ${result.toolsFile})`);
  console.log(`  ${result.pages} HTML pages, ${result.sitemapUrls} sitemap URLs, ${result.totalBytes} bytes in dist`);
}
