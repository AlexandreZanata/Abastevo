import test from 'node:test';
import assert from 'node:assert/strict';
import { renderMarkdown, parseFrontMatter, escapeHtml, slugify, checkLinkUrl, MarkdownError } from '../scripts/content/markdown.mjs';

const render = (source) => renderMarkdown(source).html;

test('escapeHtml escapes every HTML-significant character', () => {
  assert.equal(escapeHtml(`<a href="x" onclick='y'>&</a>`), '&lt;a href=&quot;x&quot; onclick=&#39;y&#39;&gt;&amp;&lt;/a&gt;');
});

test('raw HTML in text is escaped, never passed through (XSS)', () => {
  const html = render('Olá <script>alert(1)</script> e <img src=x onerror=alert(1)>');
  assert.equal(html, '<p>Olá &lt;script&gt;alert(1)&lt;/script&gt; e &lt;img src=x onerror=alert(1)&gt;</p>');
  assert.doesNotMatch(html, /<script|<img/);
});

test('HTML inside headings, lists, tables, quotes and code stays escaped', () => {
  const html = render([
    '## Título <b>x</b>',
    '',
    '- item <i>y</i>',
    '',
    '| A <u> | B |',
    '| --- | --- |',
    '| <script>x</script> | "q" |',
    ': Legenda <em>z</em>',
    '',
    '> citação <iframe>',
    '',
    '```',
    '<script>alert(1)</script>',
    '```',
    '',
    'código `<svg onload=x>`',
  ].join('\n'));
  for (const tag of ['<b>', '<i>', '<u>', '<script>', '<iframe>', '<svg', '<em>z']) {
    assert.ok(!html.includes(tag), `raw ${tag} must not survive`);
  }
  assert.match(html, /&lt;script&gt;alert\(1\)&lt;\/script&gt;/);
  assert.match(html, /<code>&lt;svg onload=x&gt;<\/code>/);
});

test('links: https, internal paths and anchors render; text and URL are escaped', () => {
  const { html, links } = renderMarkdown('Veja [a "ANP" <b>](https://www.gov.br/anp/?a=1&b=2) e [preços](/precos/sp/) e [topo](#topo).\n\n## Topo');
  assert.match(html, /<a href="https:\/\/www\.gov\.br\/anp\/\?a=1&amp;b=2" rel="noopener">a &quot;ANP&quot; &lt;b&gt;<\/a>/);
  assert.match(html, /<a href="\/precos\/sp\/">preços<\/a>/);
  assert.match(html, /<a href="#topo">topo<\/a>/);
  assert.deepEqual(links.map((l) => l.kind), ['external', 'internal', 'anchor']);
});

test('unsafe link schemes and protocol-relative URLs fail the build', () => {
  for (const url of [
    'javascript:alert(1)',
    'JAVASCRIPT:alert(1)',
    ' javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)',
    '//evil.example/',
    'http://insecure.example/',
    'https://user:pass@example.com/',
    '/a/../../etc/passwd',
    '/x" onmouseover="alert(1)',
  ]) {
    assert.throws(() => checkLinkUrl(url), MarkdownError, url);
  }
  assert.throws(() => render('[clique](javascript:alert(1))'), MarkdownError);
  assert.throws(() => render('[](/precos/)'), /Link without text/);
});

test('emphasis, bold and backslash escapes', () => {
  assert.equal(render('**forte** e *ênfase* e _outra_ e \\*literal\\*'), '<p><strong>forte</strong> e <em>ênfase</em> e <em>outra</em> e *literal*</p>');
  assert.equal(render('5 * 3 = 15 e snake_case_name'), '<p>5 * 3 = 15 e snake_case_name</p>');
  assert.throws(() => render('**sem fechar'), /Unbalanced emphasis/);
});

test('headings: ids, no h1, no skipped levels, unique ids', () => {
  const { html, headings } = renderMarkdown('## Preço à vista\n\n### Detalhe\n\n## Preço à vista', { reservedIds: ['conteudo'] });
  assert.match(html, /<h2 id="preco-a-vista">Preço à vista<\/h2>/);
  assert.match(html, /<h3 id="detalhe">Detalhe<\/h3>/);
  assert.match(html, /<h2 id="preco-a-vista-2">/);
  assert.equal(headings.length, 3);
  assert.throws(() => render('# Título'), /Level-1 headings are reserved/);
  assert.throws(() => render('### Pulou'), /Heading level skipped/);
  assert.equal(renderMarkdown('## Conteudo', { reservedIds: ['conteudo'] }).headings[0].id, 'conteudo-2');
});

test('lists: unordered, ordered with start, continuation lines; nested lists rejected', () => {
  assert.equal(render('- um\n- dois\n  continua'), '<ul><li>um</li><li>dois continua</li></ul>');
  assert.equal(render('3. três\n4. quatro'), '<ol start="3"><li>três</li><li>quatro</li></ol>');
  assert.throws(() => render('- um\n  - aninhado'), /Nested lists/);
});

test('tables need a header separator and a caption; headers are scoped', () => {
  const html = render('| Preço | Paridade |\n| --- | ---: |\n| R$ 5,00 | 70% |\n: Exemplo ilustrativo');
  assert.match(html, /<div class="guide-table-wrap" role="region" aria-label="Exemplo ilustrativo" tabindex="0"><table class="guide-table"><caption>Exemplo ilustrativo<\/caption>/);
  assert.match(html, /<th scope="col">Preço<\/th><th scope="col">Paridade<\/th>/);
  assert.match(html, /<td>R\$ 5,00<\/td><td>70%<\/td>/);
  assert.throws(() => render('| A | B |\n| --- | --- |\n| 1 | 2 |'), /caption/);
  assert.throws(() => render('| A | B |\n| 1 | 2 |'), /separator/);
  assert.throws(() => render('| A | B |\n| --- | --- |\n| 1 |\n: c'), /expected 2/);
});

test('blockquotes and callouts render nested blocks but no headings', () => {
  assert.equal(render('> linha um\n> linha dois'), '<blockquote><p>linha um linha dois</p></blockquote>');
  assert.equal(render('::: resumo\n**Resposta:** sim.\n:::'), '<div class="guide-callout callout-resumo"><p><strong>Resposta:</strong> sim.</p></div>');
  assert.throws(() => render('> ## título'), /Headings are not allowed/);
  assert.throws(() => render('::: script\nx\n:::'), /Unknown callout/);
  assert.throws(() => render('::: nota\nsem fim'), /Unterminated/);
});

test('images require alt text and a local /assets/ source', () => {
  const html = render('![Bomba de combustível com visor zerado](/assets/art/pump.webp "Legenda <x>")');
  assert.equal(html, '<figure class="article-figure"><img src="/assets/art/pump.webp" alt="Bomba de combustível com visor zerado" loading="lazy" decoding="async"><figcaption>Legenda &lt;x&gt;</figcaption></figure>');
  assert.throws(() => render('![](/assets/art/pump.webp)'), /alternative text/);
  assert.throws(() => render('![alt](https://evil.example/x.png)'), /local \/assets\//);
  assert.throws(() => render('![alt](/assets/../secret.png)'), /local \/assets\//);
  assert.equal(
    render('![alt" onerror="x](/assets/a.png)'),
    '<figure class="article-figure"><img src="/assets/a.png" alt="alt&quot; onerror=&quot;x" loading="lazy" decoding="async"></figure>',
    'quotes in alt text are escaped, so no attribute injection'
  );
  assert.throws(() => render('texto ![alt](/assets/a.png) no meio'), /stand alone/);
});

test('NUL characters are rejected and word counts ignore markup', () => {
  assert.throws(() => render('a\u0000b'), /NUL/);
  assert.equal(renderMarkdown('## Dois títulos\n\nTrês palavras aqui. R$ 5,00').words, 7);
});

test('slugify follows the shared portal rule', () => {
  assert.equal(slugify("ALTA FLORESTA D'OESTE"), 'alta-floresta-doeste');
  assert.equal(slugify('São Paulo'), 'sao-paulo');
  assert.equal(slugify('Cuiabá'), 'cuiaba');
  assert.equal(slugify('  --Preço à vista!-- '), 'preco-a-vista');
});

test('front matter: scalars, quoted values, inline and block lists', () => {
  const { data, body } = parseFrontMatter([
    '---',
    'title: Etanol: qual compensa',
    'quoted: "com \\"aspas\\" e: dois pontos"',
    'tags: [etanol, "gasolina"]',
    'related:',
    '  - /precos/',
    '  - /guias/',
    '# comentário',
    '---',
    'Corpo',
  ].join('\n'), 'x.md');
  assert.deepEqual(data, {
    title: 'Etanol: qual compensa',
    quoted: 'com "aspas" e: dois pontos',
    tags: ['etanol', 'gasolina'],
    related: ['/precos/', '/guias/'],
  });
  assert.equal(body, 'Corpo');
});

test('front matter errors: missing delimiters, duplicate keys, malformed lines', () => {
  assert.throws(() => parseFrontMatter('title: x\n', 'a.md'), /missing front matter opening/);
  assert.throws(() => parseFrontMatter('---\ntitle: x\n', 'a.md'), /missing front matter closing/);
  assert.throws(() => parseFrontMatter('---\ntitle: a\ntitle: b\n---\n', 'a.md'), /duplicate key "title"/);
  assert.throws(() => parseFrontMatter('---\n  - solto\n---\n', 'a.md'), /list item without a key/);
  assert.throws(() => parseFrontMatter('---\nsem dois pontos\n---\n', 'a.md'), /expected "key: value"/);
  assert.throws(() => parseFrontMatter('---\ntags: [a, b\n---\n', 'a.md'), /unterminated inline list/);
  assert.throws(() => parseFrontMatter('---\nt: "aberto\n---\n', 'a.md'), /unterminated quoted value/);
});
