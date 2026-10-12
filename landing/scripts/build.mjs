import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import zlib from 'node:zlib';
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

export function copyAppAssets(staticAppDir, outAppDir) {
  const provenancePath = path.join(staticAppDir, 'provenance.json');
  if (!fs.existsSync(provenancePath)) {
    throw new Error(`App asset provenance missing: ${provenancePath}`);
  }
  const provenance = JSON.parse(fs.readFileSync(provenancePath, 'utf8'));
  // Masters (lossless PNG) must never ship: only derived WebP + provenance land in dist.
  for (const item of fs.readdirSync(staticAppDir)) {
    if (item.endsWith('.png')) {
      throw new Error(`App master must not ship in web assets: ${item}`);
    }
  }
  if (!Array.isArray(provenance.outputs) || provenance.outputs.length === 0) {
    throw new Error('App asset provenance declares no outputs');
  }
  fs.mkdirSync(outAppDir, { recursive: true });
  let copied = 0;
  for (const output of provenance.outputs) {
    if (!/^[a-z0-9-]+\.webp$/.test(output.file)) {
      throw new Error(`Invalid app derivative filename (possible traversal): ${output.file}`);
    }
    const src = path.join(staticAppDir, output.file);
    if (!fs.existsSync(src)) {
      throw new Error(`App derivative missing: ${output.file}`);
    }
    const content = fs.readFileSync(src);
    const sha256 = crypto.createHash('sha256').update(content).digest('hex');
    if (sha256 !== output.sha256) {
      throw new Error(`App derivative hash mismatch: ${output.file}`);
    }
    fs.writeFileSync(path.join(outAppDir, output.file), content);
    copied += 1;
  }
  fs.copyFileSync(provenancePath, path.join(outAppDir, 'provenance.json'));
  return copied;
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

    // Self-hosted fonts use versioned content-hash names for immutable caching.
    // Copy only declared public font assets and their required license notices.
    const fontsStaticDir = path.join(staticDir, 'assets', 'fonts');
    const fontsOutDir = path.join(outDir, 'assets', 'fonts');
    const fontManifest = JSON.parse(fs.readFileSync(path.join(fontsStaticDir, 'provenance.json'), 'utf8'));
    fs.mkdirSync(fontsOutDir, { recursive: true });
    for (const font of fontManifest.fonts) {
      if (!/^[a-z0-9-]+\.[a-f0-9]{8}\.woff2$/.test(font.file)) {
        throw new Error(`Invalid public font filename: ${font.file}`);
      }
      const content = fs.readFileSync(path.join(fontsStaticDir, font.file));
      const sha256 = crypto.createHash('sha256').update(content).digest('hex');
      if (sha256 !== font.sha256 || !font.file.includes(`.${sha256.slice(0, 8)}.`)) {
        throw new Error(`Font provenance/hash mismatch: ${font.file}`);
      }
      fs.writeFileSync(path.join(fontsOutDir, font.file), content);
    }
    for (const license of ['OFL-Manrope.txt', 'OFL-Source-Sans-3.txt']) {
      fs.copyFileSync(path.join(fontsStaticDir, license), path.join(fontsOutDir, license));
    }

    // Public app derivatives (LR08) gated by their provenance manifest (LR09):
    // hash-verified WebP only; masters and undeclared files never ship.
    const appAssetsCopied = copyAppAssets(
      path.join(staticDir, 'assets', 'app'),
      path.join(outDir, 'assets', 'app'),
    );

    // LH01 generated decoration: declared, immutable WebP and traced SVG derivatives only.
    const artDir = path.join(staticDir, 'assets', 'art');
    const artManifest = JSON.parse(fs.readFileSync(path.join(artDir, 'provenance.json'), 'utf8'));
    const artOut = path.join(outDir, 'assets', 'art');
    fs.mkdirSync(artOut, { recursive: true });
    for (const output of artManifest.outputs) {
      if (!/^[a-z0-9-]+\.[a-f0-9]{8}\.(webp|svg)$/.test(output.file)) throw new Error('Invalid decorative art filename');
      const bytes = fs.readFileSync(path.join(artDir, output.file));
      const digest = crypto.createHash('sha256').update(bytes).digest('hex');
      if (digest !== output.sha256 || !output.file.includes(`.${digest.slice(0, 8)}.`)) throw new Error('Decorative art hash mismatch');
      const limit = output.file.endsWith('.svg') ? 1024 * 1024 : 120 * 1024;
      if (bytes.length > limit) throw new Error('Decorative art exceeds its transfer budget');
      if (output.file.endsWith('.svg') && zlib.gzipSync(bytes).length > 80 * 1024) throw new Error('Decorative SVG exceeds 80 KiB gzip budget');
      if (output.file.endsWith('.svg') && /<(?:script|image|foreignObject)\b|\bon\w+\s*=|\bhref\s*=/i.test(bytes.toString('utf8'))) throw new Error('Decorative SVG must contain only local vector geometry');
      fs.writeFileSync(path.join(artOut, output.file), bytes);
    }
    fs.copyFileSync(path.join(artDir, 'provenance.json'), path.join(artOut, 'provenance.json'));

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
      { srcRel: 'como-usar/index.html', destRel: 'como-usar/index.html' },
      { srcRel: 'como-usar/escolher-cidade/index.html', destRel: 'como-usar/escolher-cidade/index.html' },
      { srcRel: 'como-usar/precos-do-posto/index.html', destRel: 'como-usar/precos-do-posto/index.html' },
      { srcRel: 'como-usar/comunidade/index.html', destRel: 'como-usar/comunidade/index.html' },
      { srcRel: 'como-usar/conta-e-privacidade/index.html', destRel: 'como-usar/conta-e-privacidade/index.html' },
      { srcRel: 'como-usar/fotografar-preco/index.html', destRel: 'como-usar/fotografar-preco/index.html' },
      { srcRel: 'como-usar/denunciar-abuso/index.html', destRel: 'como-usar/denunciar-abuso/index.html' },
      { srcRel: 'como-usar/veiculos/index.html', destRel: 'como-usar/veiculos/index.html' },
      { srcRel: 'como-usar/offline-e-recuperacao/index.html', destRel: 'como-usar/offline-e-recuperacao/index.html' },
      { srcRel: 'como-usar/ajuda-e-regras/index.html', destRel: 'como-usar/ajuda-e-regras/index.html' },
      { srcRel: 'como-usar/precos-anp/index.html', destRel: 'como-usar/precos-anp/index.html' },
      { srcRel: 'como-usar/semanas-anp/index.html', destRel: 'como-usar/semanas-anp/index.html' },
      { srcRel: 'como-usar/historico/index.html', destRel: 'como-usar/historico/index.html' },
      { srcRel: 'como-usar/postos/index.html', destRel: 'como-usar/postos/index.html' },
      { srcRel: 'como-usar/minhas-contribuicoes/index.html', destRel: 'como-usar/minhas-contribuicoes/index.html' },
      { srcRel: 'como-usar/perfil-do-posto/index.html', destRel: 'como-usar/perfil-do-posto/index.html' },
      { srcRel: 'como-usar/configuracoes/index.html', destRel: 'como-usar/configuracoes/index.html' },
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
      const playButton = `<a href="${storeUrl}" class="store-btn store-pill" target="_blank" rel="noopener"><span class="store-btn-icon" aria-hidden="true"><svg fill="currentColor" role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M22.018 13.298l-3.919 2.218-3.515-3.493 3.543-3.521 3.891 2.202a1.49 1.49 0 0 1 0 2.594zM1.337.924a1.486 1.486 0 0 0-.112.568v21.017c0 .217.045.419.124.6l11.155-11.087L1.337.924zm12.207 10.065l3.258-3.238L3.45.195a1.466 1.466 0 0 0-.946-.179l11.04 10.973zm0 2.067l-11 10.933c.298.036.612-.016.906-.183l13.324-7.54-3.23-3.21z"/></svg></span><span class="store-btn-text"><strong>Ver na Google Play</strong><small>Baixar o aplicativo</small></span></a>`;
      if (srcRel === 'index.html') {
        // Prelaunch keeps the static honest pill; published swaps in the verified link.
        if (storeState === 'published') {
          const slotRegex = /<!-- STORE_STATE_SLOT -->[\s\S]*?<!-- \/STORE_STATE_SLOT -->/;
          html = html.replace(slotRegex, playButton);
        }

        if (storeState === 'published') {
          html = html.replaceAll('{{STORE_SAME_AS_EXTRA}}', `,\n          "${storeUrl}"`);
          const appNode = `,\n      {\n        "@type": "MobileApplication",\n        "@id": "${origin}/#mobileapp",\n        "name": "abastevo",\n        "operatingSystem": "Android",\n        "applicationCategory": "UtilitiesApplication",\n        "installUrl": "${storeUrl}",\n        "offers": {\n          "@type": "Offer",\n          "price": "0",\n          "priceCurrency": "BRL"\n        }\n      }`;
          html = html.replaceAll('{{STORE_APP_SCHEMA_NODE}}', appNode);
        } else {
          html = html.replaceAll('{{STORE_SAME_AS_EXTRA}}', '');
          html = html.replaceAll('{{STORE_APP_SCHEMA_NODE}}', '');
        }
      }

      // Header store pill: prelaunch keeps the static honest pill on every page.
      if (storeState === 'published') {
        const headerSlotRegex = /<!-- HEADER_STORE_SLOT -->[\s\S]*?<!-- \/HEADER_STORE_SLOT -->/;
        if (headerSlotRegex.test(html)) {
          html = html.replace(headerSlotRegex, playButton);
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
      appAssets: appAssetsCopied,
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
