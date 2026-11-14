// In-house Markdown subset renderer for landing guides (D-SEO-05).
// Supported: ## to #### headings, paragraphs, **bold**, *italic*, `code`,
// [links](url), - / 1. lists (one level), pipe tables with a ": caption" line,
// > blockquotes, ![alt](/assets/...) images, ``` fences, --- rules and
// ::: resumo|nota|atencao callouts. Raw HTML is never passed through: every
// character of text is escaped, and unsafe constructs fail the build.

export class MarkdownError extends Error {}

const ESCAPES = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (ch) => ESCAPES[ch]);
}

// Slug rule shared with the portal: NFD, strip combining marks, lowercase,
// drop apostrophes, collapse non-alphanumerics into '-', trim dashes.
export function slugify(text) {
  return String(text)
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/['’]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

// ---------------------------------------------------------------------------
// Front matter: `key: value`, `key: [a, b]`, or `key:` followed by `  - item`.

const KEY_RE = /^[A-Za-z][A-Za-z0-9_]*$/;

function unquote(raw, where) {
  const value = raw.trim();
  if (value.startsWith('"')) {
    if (value.length < 2 || !value.endsWith('"')) throw new MarkdownError(`${where}: unterminated quoted value`);
    return value.slice(1, -1).replace(/\\(["\\])/g, '$1');
  }
  return value;
}

export function parseFrontMatter(source, file = 'content') {
  const text = String(source).replace(/\r\n?/g, '\n');
  if (!text.startsWith('---\n')) throw new MarkdownError(`${file}: missing front matter opening ---`);
  const end = text.indexOf('\n---\n', 3);
  const endAtEof = text.endsWith('\n---') ? text.length - 4 : -1;
  const close = end !== -1 ? end : endAtEof;
  if (close === -1) throw new MarkdownError(`${file}: missing front matter closing ---`);
  const header = text.slice(4, close);
  const body = text.slice(close + 5);
  const data = {};
  let listKey = null;
  for (const [index, line] of header.split('\n').entries()) {
    const where = `${file}: front matter line ${index + 2}`;
    if (line.trim() === '' || line.trimStart().startsWith('#')) continue;
    const item = /^\s+-\s+(.*)$/.exec(line);
    if (item) {
      if (listKey === null) throw new MarkdownError(`${where}: list item without a key`);
      data[listKey].push(unquote(item[1], where));
      continue;
    }
    const pair = /^([^:\s][^:]*):(.*)$/.exec(line);
    if (!pair) throw new MarkdownError(`${where}: expected "key: value"`);
    const key = pair[1].trim();
    if (!KEY_RE.test(key)) throw new MarkdownError(`${where}: invalid key "${key}"`);
    if (Object.hasOwn(data, key)) throw new MarkdownError(`${where}: duplicate key "${key}"`);
    const raw = pair[2].trim();
    listKey = null;
    if (raw === '') {
      data[key] = [];
      listKey = key;
    } else if (raw.startsWith('[')) {
      if (!raw.endsWith(']')) throw new MarkdownError(`${where}: unterminated inline list`);
      const inner = raw.slice(1, -1).trim();
      data[key] = inner === '' ? [] : inner.split(',').map((part) => unquote(part, where));
    } else {
      data[key] = unquote(raw, where);
    }
  }
  return { data, body };
}

// ---------------------------------------------------------------------------
// Links and images.

const INTERNAL_PATH_RE = /^\/(?!\/)[A-Za-z0-9\-._~/%]*(?:\?[A-Za-z0-9\-._~%=&+,]*)?(?:#[A-Za-z0-9\-_]*)?$/;
const IMAGE_SRC_RE = /^\/assets\/(?:[A-Za-z0-9_-]+\/)*[A-Za-z0-9_.-]+\.(?:webp|png|jpe?g|svg|avif)$/;

export function checkLinkUrl(raw) {
  const url = String(raw).trim();
  if (/[\s\u0000-\u001f\u007f"'<>`\\]/.test(url)) throw new MarkdownError(`Unsafe link URL: ${JSON.stringify(raw)}`);
  if (/^#[A-Za-z0-9\-_]+$/.test(url)) return { url, kind: 'anchor' };
  if (INTERNAL_PATH_RE.test(url) && !url.includes('..')) return { url, kind: 'internal' };
  if (url.startsWith('https://')) {
    let parsed;
    try {
      parsed = new URL(url);
    } catch {
      throw new MarkdownError(`Invalid link URL: ${url}`);
    }
    if (parsed.protocol === 'https:' && parsed.hostname !== '' && parsed.username === '' && parsed.password === '') {
      return { url, kind: 'external' };
    }
  }
  throw new MarkdownError(`Unsafe link URL: ${JSON.stringify(raw)} (allowed: https://, /internal paths, #anchors)`);
}

function checkImageSrc(raw) {
  const src = String(raw).trim();
  if (!IMAGE_SRC_RE.test(src) || src.includes('..')) {
    throw new MarkdownError(`Image source must be a local /assets/ image: ${JSON.stringify(raw)}`);
  }
  return src;
}

// ---------------------------------------------------------------------------
// Inline rendering. Code spans, links and backslash escapes become opaque
// placeholders; the remaining text is escaped and then emphasis is applied.

function renderInline(source, ctx, allowLinks = true) {
  if (/\u0000/.test(source)) throw new MarkdownError('NUL characters are not allowed');
  const slots = [];
  const hold = (html) => `\u0000${slots.push(html) - 1}\u0000`;
  let text = '';
  let i = 0;
  while (i < source.length) {
    const ch = source[i];
    if (ch === '\\' && i + 1 < source.length && /[\\`*_[\]()#+\-.!|>~]/.test(source[i + 1])) {
      text += hold(escapeHtml(source[i + 1]));
      i += 2;
      continue;
    }
    if (ch === '`') {
      const close = source.indexOf('`', i + 1);
      if (close === -1) throw new MarkdownError(`Unterminated code span in: ${source}`);
      text += hold(`<code>${escapeHtml(source.slice(i + 1, close))}</code>`);
      i = close + 1;
      continue;
    }
    if (ch === '!' && source[i + 1] === '[') {
      throw new MarkdownError('Images must stand alone on their own line');
    }
    if (ch === '[' && allowLinks) {
      const closeText = source.indexOf('](', i + 1);
      const closeUrl = closeText === -1 ? -1 : source.indexOf(')', closeText + 2);
      if (closeText !== -1 && closeUrl !== -1 && !source.slice(i + 1, closeText).includes('[')) {
        const label = source.slice(i + 1, closeText);
        const { url, kind } = checkLinkUrl(source.slice(closeText + 2, closeUrl));
        if (label.trim() === '') throw new MarkdownError(`Link without text: ${url}`);
        ctx.links.push({ url, kind, text: label });
        const rel = kind === 'external' ? ' rel="noopener"' : '';
        text += hold(`<a href="${escapeHtml(url)}"${rel}>${renderInline(label, ctx, false)}</a>`);
        i = closeUrl + 1;
        continue;
      }
    }
    text += ch;
    i += 1;
  }
  let html = escapeHtml(text);
  html = html.replace(/\*\*(?=\S)([\s\S]*?\S)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/(^|[^\w*])\*(?=\S)([^*]*?\S)\*(?![\w*])/g, '$1<em>$2</em>');
  html = html.replace(/(^|[^\w])_(?=\S)([^_]*?\S)_(?![\w])/g, '$1<em>$2</em>');
  if (/[*]{2}/.test(html.replace(/\u0000\d+\u0000/g, ''))) {
    throw new MarkdownError(`Unbalanced emphasis in: ${source}`);
  }
  return html.replace(/\u0000(\d+)\u0000/g, (_, n) => slots[Number(n)]);
}

// ---------------------------------------------------------------------------
// Block rendering.

const CALLOUTS = new Set(['resumo', 'nota', 'atencao']);
const UL_RE = /^([-*])\s+(.*)$/;
const OL_RE = /^(\d{1,3})[.)]\s+(.*)$/;
const TABLE_SEP_RE = /^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$/;

function splitRow(line) {
  let row = line.trim();
  if (row.startsWith('|')) row = row.slice(1);
  if (row.endsWith('|') && !row.endsWith('\\|')) row = row.slice(0, -1);
  const cells = [];
  let current = '';
  for (let i = 0; i < row.length; i++) {
    if (row[i] === '\\' && row[i + 1] === '|') {
      current += '\\|';
      i += 1;
    } else if (row[i] === '|') {
      cells.push(current.trim());
      current = '';
    } else {
      current += row[i];
    }
  }
  cells.push(current.trim());
  return cells;
}

function isBlockStart(line) {
  return /^#{1,6}\s/.test(line) || /^>/.test(line) || UL_RE.test(line) || OL_RE.test(line)
    || /^\|/.test(line) || /^!\[/.test(line) || /^```/.test(line) || /^:::/.test(line)
    || /^(-{3,}|\*{3,})\s*$/.test(line) || /^\s+([-*]|\d+[.)])\s/.test(line);
}

function renderBlocks(lines, ctx, nested) {
  const out = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      i += 1;
      continue;
    }

    const heading = /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line);
    if (heading) {
      if (nested) throw new MarkdownError('Headings are not allowed inside quotes or callouts');
      const level = heading[1].length;
      if (level === 1) throw new MarkdownError('Level-1 headings are reserved for the page title');
      if (level > 4) throw new MarkdownError(`Heading level ${level} is not supported`);
      const limit = ctx.lastLevel + 1;
      if (level > limit) throw new MarkdownError(`Heading level skipped: h${level} after h${ctx.lastLevel}`);
      ctx.lastLevel = level;
      const inner = renderInline(heading[2], ctx);
      const plain = inner.replace(/<[^>]+>/g, '');
      let id = slugify(plain.replace(/&[a-z#0-9]+;/g, '')) || 'secao';
      const base = id;
      for (let n = 2; ctx.ids.has(id); n++) id = `${base}-${n}`;
      ctx.ids.add(id);
      ctx.headings.push({ level, id, html: inner });
      out.push(`<h${level} id="${id}">${inner}</h${level}>`);
      i += 1;
      continue;
    }

    if (/^```/.test(line)) {
      const body = [];
      i += 1;
      while (i < lines.length && !/^```\s*$/.test(lines[i])) {
        body.push(lines[i]);
        i += 1;
      }
      if (i >= lines.length) throw new MarkdownError('Unterminated code fence');
      i += 1;
      out.push(`<pre class="article-code"><code>${escapeHtml(body.join('\n'))}</code></pre>`);
      continue;
    }

    const callout = /^:::\s*([a-z]+)\s*$/.exec(line);
    if (callout) {
      if (!CALLOUTS.has(callout[1])) throw new MarkdownError(`Unknown callout type: ${callout[1]}`);
      const body = [];
      i += 1;
      while (i < lines.length && !/^:::\s*$/.test(lines[i])) {
        body.push(lines[i]);
        i += 1;
      }
      if (i >= lines.length) throw new MarkdownError('Unterminated ::: callout');
      i += 1;
      out.push(`<div class="guide-callout callout-${callout[1]}">${renderBlocks(body, ctx, true)}</div>`);
      continue;
    }

    if (/^>/.test(line)) {
      const body = [];
      while (i < lines.length && /^>/.test(lines[i])) {
        body.push(lines[i].replace(/^>\s?/, ''));
        i += 1;
      }
      out.push(`<blockquote>${renderBlocks(body, ctx, true)}</blockquote>`);
      continue;
    }

    if (/^\s+([-*]|\d+[.)])\s/.test(line)) throw new MarkdownError('Nested lists are not supported');

    const ul = UL_RE.exec(line);
    const ol = OL_RE.exec(line);
    if ((ul && !/^(-{3,}|\*{3,})\s*$/.test(line)) || ol) {
      const ordered = Boolean(ol);
      const re = ordered ? OL_RE : UL_RE;
      const items = [];
      const start = ordered ? Number(ol[1]) : 1;
      while (i < lines.length) {
        const current = lines[i];
        const match = re.exec(current);
        if (match) {
          items.push(match[2]);
        } else if (/^\s+([-*]|\d+[.)])\s/.test(current)) {
          throw new MarkdownError('Nested lists are not supported');
        } else if (/^\s{2,}\S/.test(current) && items.length > 0) {
          items[items.length - 1] += ` ${current.trim()}`;
        } else {
          break;
        }
        i += 1;
      }
      const tag = ordered ? 'ol' : 'ul';
      const startAttr = ordered && start !== 1 ? ` start="${start}"` : '';
      out.push(`<${tag}${startAttr}>${items.map((item) => `<li>${renderInline(item, ctx)}</li>`).join('')}</${tag}>`);
      continue;
    }

    if (/^\|/.test(line)) {
      const header = splitRow(line);
      if (i + 1 >= lines.length || !TABLE_SEP_RE.test(lines[i + 1])) {
        throw new MarkdownError('Table header must be followed by a --- separator row');
      }
      i += 2;
      const rows = [];
      while (i < lines.length && /^\|/.test(lines[i])) {
        const cells = splitRow(lines[i]);
        if (cells.length !== header.length) throw new MarkdownError(`Table row has ${cells.length} cells, expected ${header.length}`);
        rows.push(cells);
        i += 1;
      }
      const caption = i < lines.length ? /^:\s+(.+)$/.exec(lines[i]) : null;
      if (!caption) throw new MarkdownError('Tables need a caption line (": Caption") right after the last row');
      i += 1;
      const captionHtml = renderInline(caption[1], ctx);
      const label = escapeHtml(caption[1].replace(/[*_`]/g, ''));
      const head = header.map((cell) => `<th scope="col">${renderInline(cell, ctx)}</th>`).join('');
      const body = rows.map((cells) => `<tr>${cells.map((cell) => `<td>${renderInline(cell, ctx)}</td>`).join('')}</tr>`).join('');
      out.push(`<div class="guide-table-wrap" role="region" aria-label="${label}" tabindex="0"><table class="guide-table"><caption>${captionHtml}</caption><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table></div>`);
      continue;
    }

    if (/^!\[/.test(line)) {
      const image = /^!\[([^\]]*)\]\(([^)\s]+)(?:\s+"([^"]*)")?\)\s*$/.exec(line);
      if (!image) throw new MarkdownError(`Malformed image: ${line}`);
      const alt = image[1].trim();
      if (alt === '') throw new MarkdownError(`Image needs alternative text: ${image[2]}`);
      const src = checkImageSrc(image[2]);
      const caption = image[3] ? `<figcaption>${escapeHtml(image[3])}</figcaption>` : '';
      out.push(`<figure class="article-figure"><img src="${escapeHtml(src)}" alt="${escapeHtml(alt)}" loading="lazy" decoding="async">${caption}</figure>`);
      i += 1;
      continue;
    }

    if (/^(-{3,}|\*{3,})\s*$/.test(line)) {
      out.push('<hr>');
      i += 1;
      continue;
    }

    const paragraph = [];
    while (i < lines.length && lines[i].trim() !== '' && (paragraph.length === 0 || !isBlockStart(lines[i]))) {
      paragraph.push(lines[i].trim());
      i += 1;
    }
    out.push(`<p>${renderInline(paragraph.join(' '), ctx)}</p>`);
  }
  return out.join('\n');
}

export function renderMarkdown(source, options = {}) {
  const text = String(source).replace(/\r\n?/g, '\n');
  if (/\u0000/.test(text)) throw new MarkdownError('NUL characters are not allowed');
  const ctx = { links: [], headings: [], ids: new Set(options.reservedIds || []), lastLevel: 1 };
  const html = renderBlocks(text.split('\n'), ctx, false);
  const plain = html.replace(/<[^>]+>/g, ' ').replace(/&[a-z#0-9]+;/g, ' ');
  const words = plain.split(/\s+/).filter((word) => /[\p{L}\p{N}]/u.test(word)).length;
  return { html, headings: ctx.headings, links: ctx.links, words };
}
