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

export function landingRoot(projectRoot) {
  return path.resolve(projectRoot, '..', 'landing');
}

export function build(options = {}) {
  const root = options.projectRoot || DEFAULT_ROOT;
  const outDir = options.outDir || path.join(root, 'dist');
  const staticDir = path.join(root, 'static');
  const srcDir = path.join(root, 'src');
  const landing = options.landingRoot || landingRoot(root);

  // 1. Compile TypeScript to temp directory
  const tmpDir = path.join(root, '.build-tmp-' + crypto.randomBytes(4).toString('hex'));
  fs.mkdirSync(tmpDir, { recursive: true });

  let emitted = [];
  try {
    const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
    const tscBin = fs.existsSync(localTsc) ? localTsc : 'tsc';
    execFileSync(tscBin, ['--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir], {
      cwd: root,
      stdio: 'pipe',
    });
    emitted = fs.readdirSync(tmpDir).filter((f) => f.endsWith('.js')).sort();
    if (emitted.length === 0) {
      throw new Error('TypeScript compilation failed: no emitted JS found');
    }

    // 2. Clean and prepare outDir (deterministic builds, no stale assets)
    if (fs.existsSync(outDir)) {
      fs.rmSync(outDir, { recursive: true, force: true });
    }
    fs.mkdirSync(path.join(outDir, 'assets'), { recursive: true });

    // 3. Fingerprint emitted JS and rewrite relative specifiers
    const renamed = new Map();
    for (const file of emitted) {
      const base = file.slice(0, -3);
      renamed.set(`./${base}.js`, `./${base}.${computeHash(fs.readFileSync(path.join(tmpDir, file)))}.js`);
    }
    const fingerprinted = new Map();
    for (const file of emitted) {
      let content = fs.readFileSync(path.join(tmpDir, file), 'utf8');
      for (const [from, to] of renamed) {
        content = content.split(`from '${from}'`).join(`from '${to}'`);
        content = content.split(`from "${from}"`).join(`from "${to}"`);
      }
      const base = file.slice(0, -3);
      const fingerprintedName = `${base}.${computeHash(fs.readFileSync(path.join(tmpDir, file)))}.js`;
      fingerprinted.set(file, fingerprintedName);
      fs.mkdirSync(path.join(outDir, 'assets'), { recursive: true });
      fs.writeFileSync(path.join(outDir, 'assets', fingerprintedName), content, 'utf8');
    }

    // 3. Copy landing stylesheet (read-only input) with fingerprint
    const landingCssPath = path.join(landing, 'static', 'assets', 'styles.css');
    if (!fs.existsSync(landingCssPath)) {
      throw new Error(`landing stylesheet not found: ${landingCssPath}`);
    }
    const cssContent = fs.readFileSync(landingCssPath);
    const cssName = `styles.${computeHash(cssContent)}.css`;
    fs.writeFileSync(path.join(outDir, 'assets', cssName), cssContent);

    // 4. Fingerprint admin-only stylesheet (existing tokens only; enforced by tests)
    const adminCssPath = path.join(staticDir, 'assets', 'admin.css');
    if (!fs.existsSync(adminCssPath)) {
      throw new Error('static/assets/admin.css not found');
    }
    const adminCssContent = fs.readFileSync(adminCssPath);
    const adminCssName = `admin.${computeHash(adminCssContent)}.css`;
    fs.writeFileSync(path.join(outDir, 'assets', adminCssName), adminCssContent);

    // 5. Self-hosted fonts from landing, hash-verified like the landing build
    const fontsStaticDir = path.join(landing, 'static', 'assets', 'fonts');
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

    // 6. Process HTML files and rewrite asset references. The single app
    // shell is served at both admin routes; the JS router picks the view.
    const htmlFiles = [
      { srcRel: 'app/index.html', destRel: 'login/index.html' },
      { srcRel: 'app/index.html', destRel: 'accounts/index.html' },
      { srcRel: 'app/index.html', destRel: 'contributors/index.html' },
      { srcRel: 'app/index.html', destRel: 'audit/index.html' },
      { srcRel: '404.html', destRel: '404.html' },
    ];
    for (const { srcRel, destRel } of htmlFiles) {
      const srcPath = path.join(staticDir, srcRel);
      if (!fs.existsSync(srcPath)) {
        throw new Error(`Required HTML file missing: ${srcRel}`);
      }
      let html = fs.readFileSync(srcPath, 'utf8');
      html = html.replaceAll('/assets/styles.css', `/assets/${cssName}`);
      html = html.replaceAll('/assets/admin.css', `/assets/${adminCssName}`);
      for (const [file, fingerprintedName] of fingerprinted) {
        html = html.replaceAll(`/assets/${file}`, `/assets/${fingerprintedName}`);
      }
      const destPath = path.join(outDir, destRel);
      fs.mkdirSync(path.dirname(destPath), { recursive: true });
      fs.writeFileSync(destPath, html, 'utf8');
    }

    // 7. Copy static configuration files (no origin substitution: no absolute URLs)
    for (const file of ['_headers', 'robots.txt']) {
      const src = path.join(staticDir, file);
      if (!fs.existsSync(src)) {
        throw new Error(`Required static file missing: ${file}`);
      }
      const text = fs.readFileSync(src, 'utf8');
      if (text.includes('{{')) {
        throw new Error(`Unsubstituted placeholder in static/${file}`);
      }
      fs.writeFileSync(path.join(outDir, file), text, 'utf8');
    }

    // 8. Sanity check: no source leaks
    const leaked = [];
    function scanLeaks(dir) {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          scanLeaks(full);
        } else if (
          entry.name.endsWith('.ts') ||
          entry.name.endsWith('.map') ||
          entry.name.startsWith('.env') ||
          entry.name === 'package.json' ||
          entry.name === 'package-lock.json'
        ) {
          leaked.push(full);
        }
      }
    }
    scanLeaks(outDir);
    if (leaked.length > 0) {
      throw new Error(`Leaked files detected in dist: ${leaked.join(', ')}`);
    }

    return { outDir, cssName, adminCssName, fingerprinted: Object.fromEntries(fingerprinted) };
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

// CLI execution
if (process.argv[1] === __filename) {
  const args = process.argv.slice(2);
  const options = {};
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--out-dir' && args[i + 1]) {
      options.outDir = path.resolve(args[++i]);
    }
  }
  const result = build(options);
  console.log(`Build complete: dist created at ${result.outDir}`);
  console.log(`  CSS: /assets/${result.cssName}`);
  console.log(`  Admin CSS: /assets/${result.adminCssName}`);
  for (const [from, to] of Object.entries(result.fingerprinted)) {
    console.log(`  JS ${from} -> /assets/${to}`);
  }
}

export const buildSite = build;
