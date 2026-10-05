import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const DEFAULT_ROOT = path.resolve(__dirname, '..');

export function computeHash(content) {
  return crypto.createHash('sha256').update(content).digest('hex').slice(0, 8);
}

export function validateOrigin(origin) {
  if (!origin || typeof origin !== 'string') {
    throw new Error('Public origin is required and must be a string');
  }
  let url;
  try {
    url = new URL(origin);
  } catch (err) {
    throw new Error(`Invalid public origin URL: ${origin}`);
  }
  if (url.protocol !== 'https:') {
    throw new Error(`Insecure public origin: ${origin} (must use https:)`);
  }
  if (url.pathname !== '/' && url.pathname !== '') {
    throw new Error(`Public origin must not have path segments: ${origin}`);
  }
  if (url.search || url.hash) {
    throw new Error(`Public origin must not have query or hash: ${origin}`);
  }
  // Remove any trailing slash for consistent substitution
  return origin.replace(/\/$/, '');
}

export function validateStoreState(storeState, storeUrl) {
  if (storeState !== 'prelaunch' && storeState !== 'published') {
    throw new Error(`Invalid storeState: must be 'prelaunch' or 'published', got '${storeState}'`);
  }
  if (storeState === 'published') {
    if (!storeUrl || typeof storeUrl !== 'string') {
      throw new Error('storeUrl is required when storeState is "published"');
    }
    const storeRegex = /^https:\/\/play\.google\.com\/store\/apps\/details\?id=com\.anpfuel(&[a-zA-Z0-9_=-]+)*$/;
    if (!storeRegex.test(storeUrl)) {
      throw new Error(`Invalid store URL for published state: expected verified Google Play URL with package com.anpfuel, got '${storeUrl}'`);
    }
  }
}

export function build(options = {}) {
  const root = options.projectRoot || DEFAULT_ROOT;
  const origin = validateOrigin(options.origin || options.canonicalOrigin || 'https://abastevo.com.br');
  const storeState = options.storeState || 'prelaunch';
  const storeUrl = options.storeUrl || null;
  validateStoreState(storeState, storeUrl);

  const outDir = options.outDir || path.join(root, 'dist');
  const staticDir = path.join(root, 'static');
  const srcDir = path.join(root, 'src');

  // 1. Compile TypeScript to temp directory
  const tmpDir = path.join(root, '.build-tmp-' + crypto.randomBytes(4).toString('hex'));
  fs.mkdirSync(tmpDir, { recursive: true });

  try {
    const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
    const tscBin = fs.existsSync(localTsc) ? localTsc : 'tsc';
    execFileSync(tscBin, ['--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir], {
      cwd: root,
      stdio: 'pipe',
    });

    const compiledJsPath = path.join(tmpDir, 'main.js');
    if (!fs.existsSync(compiledJsPath)) {
      throw new Error('TypeScript compilation failed: compiled main.js not found');
    }
    const jsContent = fs.readFileSync(compiledJsPath);
    const jsHash = computeHash(jsContent);
    const fingerprintedJsName = `main.${jsHash}.js`;

    // 2. Read and fingerprint CSS
    const cssPath = path.join(staticDir, 'assets', 'styles.css');
    if (!fs.existsSync(cssPath)) {
      throw new Error('static/assets/styles.css not found');
    }
    const cssContent = fs.readFileSync(cssPath);
    const cssHash = computeHash(cssContent);
    const fingerprintedCssName = `styles.${cssHash}.css`;

    // 3. Clean and prepare outDir
    if (fs.existsSync(outDir)) {
      fs.rmSync(outDir, { recursive: true, force: true });
    }
    fs.mkdirSync(path.join(outDir, 'assets', 'brand'), { recursive: true });

    // 4. Write fingerprinted JS and CSS
    fs.writeFileSync(path.join(outDir, 'assets', fingerprintedJsName), jsContent);
    fs.writeFileSync(path.join(outDir, 'assets', fingerprintedCssName), cssContent);

    // 5. Copy brand assets
    const brandStaticDir = path.join(staticDir, 'assets', 'brand');
    if (fs.existsSync(brandStaticDir)) {
      for (const item of fs.readdirSync(brandStaticDir)) {
        const src = path.join(brandStaticDir, item);
        const dest = path.join(outDir, 'assets', 'brand', item);
        fs.copyFileSync(src, dest);
      }
    }

    // 6. Copy and substitute static configuration files
    const configFiles = ['robots.txt', 'sitemap.xml', 'site.webmanifest', '_headers', '_redirects', 'llms.txt'];
    for (const file of configFiles) {
      const src = path.join(staticDir, file);
      if (fs.existsSync(src)) {
        let text = fs.readFileSync(src, 'utf8');
        text = text.replaceAll('{{CANONICAL_ORIGIN}}', origin);
        fs.writeFileSync(path.join(outDir, file), text, 'utf8');
      }
    }

    // Copy root favicon.ico if present
    const rootFavicon = path.join(staticDir, 'favicon.ico');
    if (fs.existsSync(rootFavicon)) {
      fs.copyFileSync(rootFavicon, path.join(outDir, 'favicon.ico'));
    }

    // Process .well-known directory
    const wellKnownStatic = path.join(staticDir, '.well-known');
    if (fs.existsSync(wellKnownStatic)) {
      const wellKnownOut = path.join(outDir, '.well-known');
      fs.mkdirSync(wellKnownOut, { recursive: true });
      for (const item of fs.readdirSync(wellKnownStatic)) {
        const itemSrc = path.join(wellKnownStatic, item);
        let text = fs.readFileSync(itemSrc, 'utf8');
        text = text.replaceAll('{{CANONICAL_ORIGIN}}', origin);
        fs.writeFileSync(path.join(wellKnownOut, item), text, 'utf8');
      }
    }

    // 7. Process HTML files
    const htmlFiles = [
      { srcRel: 'index.html', destRel: 'index.html' },
      { srcRel: 'privacidade/index.html', destRel: 'privacidade/index.html' },
      { srcRel: 'excluir-conta/index.html', destRel: 'excluir-conta/index.html' },
      { srcRel: 'termos/index.html', destRel: 'termos/index.html' },
      { srcRel: 'guias/index.html', destRel: 'guias/index.html' },
      { srcRel: 'guias/pesquisa-anp/index.html', destRel: 'guias/pesquisa-anp/index.html' },
      { srcRel: 'guias/etanol-ou-gasolina/index.html', destRel: 'guias/etanol-ou-gasolina/index.html' },
      { srcRel: 'guias/como-ler-precos/index.html', destRel: 'guias/como-ler-precos/index.html' },
      { srcRel: '404.html', destRel: '404.html' },
    ];

    const isPreview = process.env.PREVIEW === '1';

    for (const { srcRel, destRel } of htmlFiles) {
      const srcPath = path.join(staticDir, srcRel);
      if (!fs.existsSync(srcPath)) {
        throw new Error(`Required HTML file missing: ${srcRel}`);
      }
      let html = fs.readFileSync(srcPath, 'utf8');

      // Asset reference rewrites
      html = html.replaceAll('/assets/styles.css', `/assets/${fingerprintedCssName}`);
      html = html.replaceAll('/assets/main.js', `/assets/${fingerprintedJsName}`);
      html = html.replaceAll('{{CANONICAL_ORIGIN}}', origin);

      // Store state replacement
      if (srcRel === 'index.html') {
        const slotRegex = /<!-- STORE_STATE_SLOT -->[\s\S]*?<!-- \/STORE_STATE_SLOT -->/;
        let replacement = '';
        if (storeState === 'published') {
          replacement = `<a href="${storeUrl}" class="store-link" rel="noopener noreferrer">Ver na Google Play</a>`;
        } else {
          replacement = `<span class="store-notice">Em breve na Google Play</span>`;
        }
        html = html.replace(slotRegex, replacement);

        if (storeState === 'published') {
          html = html.replaceAll('{{STORE_SAME_AS_EXTRA}}', `,\n          "${storeUrl}"`);
          const appNode = `,\n      {\n        "@type": "MobileApplication",\n        "@id": "${origin}/#mobileapp",\n        "name": "abastevo",\n        "operatingSystem": "Android",\n        "applicationCategory": "UtilitiesApplication",\n        "installUrl": "${storeUrl}",\n        "offers": {\n          "@type": "Offer",\n          "price": "0",\n          "priceCurrency": "BRL"\n        }\n      }`;
          html = html.replaceAll('{{STORE_APP_SCHEMA_NODE}}', appNode);
        } else {
          html = html.replaceAll('{{STORE_SAME_AS_EXTRA}}', '');
          html = html.replaceAll('{{STORE_APP_SCHEMA_NODE}}', '');
        }
      }

      // Preview mode indexing guard
      if (isPreview && srcRel !== '404.html') {
        if (!html.includes('robots')) {
          html = html.replace('<head>', '<head>\n  <meta name="robots" content="noindex, nofollow">');
        }
      }

      const destPath = path.join(outDir, destRel);
      fs.mkdirSync(path.dirname(destPath), { recursive: true });
      fs.writeFileSync(destPath, html, 'utf8');
    }

    // 8. Sanity check: Ensure no forbidden files leaked into outDir
    const leaked = [];
    function scanLeaks(dir) {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          scanLeaks(full);
        } else {
          if (entry.name.endsWith('.ts') || entry.name.endsWith('.map') || entry.name.startsWith('.env') || entry.name === 'package.json') {
            leaked.push(full);
          }
        }
      }
    }
    scanLeaks(outDir);
    if (leaked.length > 0) {
      throw new Error(`Leaked files detected in dist: ${leaked.join(', ')}`);
    }

    return {
      outDir,
      origin,
      storeState,
      storeUrl,
      fingerprintedCss: fingerprintedCssName,
      fingerprintedJs: fingerprintedJsName,
      cssHash,
      jsHash,
    };
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

// CLI execution
if (process.argv[1] === __filename) {
  const args = process.argv.slice(2);
  const options = {};
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--origin' && args[i + 1]) {
      options.origin = args[++i];
    } else if (args[i] === '--store-state' && args[i + 1]) {
      options.storeState = args[++i];
    } else if (args[i] === '--store-url' && args[i + 1]) {
      options.storeUrl = args[++i];
    } else if (args[i] === '--out-dir' && args[i + 1]) {
      options.outDir = path.resolve(args[++i]);
    }
  }
  const result = build(options);
  console.log(`Build complete: dist created at ${result.outDir}`);
  console.log(`  CSS: /assets/${result.fingerprintedCss} (hash ${result.cssHash})`);
  console.log(`  JS:  /assets/${result.fingerprintedJs} (hash ${result.jsHash})`);
  console.log(`  Store state: ${result.storeState}`);
}

export const buildSite = build;
