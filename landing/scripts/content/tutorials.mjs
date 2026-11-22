import fs from 'node:fs';
import path from 'node:path';
import { escapeHtml as esc } from './markdown.mjs';

const SOURCE_ROOT = '/como-usar/';
const CHAPTERS = {
  TU02: 'escolher-cidade',
  TU05: 'precos-anp',
  TU06: 'semanas-anp',
  TU07: 'historico',
  TU08: 'postos',
  TU09: 'precos-do-posto',
  TU10: 'comunidade',
  TU13: 'conta-e-privacidade',
  TU14: 'fotografar-preco',
  TU17: 'minhas-contribuicoes',
  TU21: 'denunciar-abuso',
  TU23: 'perfil-do-posto',
  TU26: 'veiculos',
  TU28: 'configuracoes',
  TU29: 'offline-e-recuperacao',
  TU30: 'ajuda-e-regras',
};
const JOURNEYS = [
  { id: 'jornada-motorista', start: 1, end: 10 },
  { id: 'jornada-comunidade', start: 11, end: 21 },
  { id: 'jornada-representante', start: 22, end: 28 },
  { id: 'jornada-ajuda', start: 29, end: 30 },
];
const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const STEP_FIELDS = ['title', 'instruction', 'expected', 'recovery', 'visual'];
const ARROW = '<span class="topic-arrow" aria-hidden="true">→</span>';

function hasTextFields(value, keys) {
  return value && keys.every((key) => typeof value[key] === 'string' && value[key].trim());
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

export function loadTutorials(root, locale) {
  const file = path.join(root, 'content', 'tutorials', `${locale.code}.json`);
  if (!fs.existsSync(file)) return [];
  const { hub, chapters, common, lastmod, pending } = readJson(file);
  const hubRoute = `${locale.prefix}/${locale.segments['como-usar']}/`;
  const entryBase = { locale: locale.code, lastmod, chrome: 'locale', indexable: true };
  const fail = (message) => { throw new Error(`Tutorial ${locale.code}: ${message}`); };
  if (!hasTextFields(hub, ['title', 'description', 'intro', 'statuses', 'start', 'sourcesTitle', 'sources'])) fail('incomplete tutorial hub copy');
  if (!hasTextFields(common, ['available', 'planned', 'soon', 'chapters', 'all', 'step', 'expected', 'recovery', 'noCapture', 'originalCapture', 'previous', 'next', 'index', 'navigation', 'explore', 'guides', 'tools', 'home', 'github', 'context', 'stepCount', 'describedCount', 'portuguese', 'metaLabel'])) fail('incomplete common tutorial copy');
  if (!/^\d{4}-\d{2}-\d{2}$/.test(lastmod)) fail('missing publication date');
  if (!Array.isArray(hub.journeys) || hub.journeys.length !== JOURNEYS.length ||
      hub.journeys.some((journey) => !hasTextFields(journey, ['title', 'description']))) fail('incomplete tutorial journeys');
  if (Object.keys(chapters).length !== Object.keys(CHAPTERS).length) fail('unexpected chapter inventory');
  const lessons = Object.entries(CHAPTERS).map(([id, sourceSlug]) => {
    const chapter = chapters[id];
    if (!hasTextFields(chapter, ['slug', 'title', 'description', 'summary']) || !SLUG.test(chapter.slug)) fail(`${id}: incomplete metadata`);
    const source = readJson(path.join(root, 'static', 'como-usar', sourceSlug, 'lesson.json'));
    const stepIds = Object.keys(chapter.steps || {});
    if (stepIds.length !== source.steps.length || source.steps.some((step) => !hasTextFields(chapter.steps?.[step.stepId], STEP_FIELDS))) {
      fail(`${id}: all ${source.steps.length} steps require localized titles, directions, outcomes, recovery and screenshot descriptions or reasons`);
    }
    const html = fs.readFileSync(path.join(root, 'static', 'como-usar', sourceSlug, 'index.html'), 'utf8');
    const relatedList = html.match(/<ul class="link-list">([\s\S]*?)<\/ul>/)?.[1];
    const related = [...(relatedList || '').matchAll(/<a href="(\/[^"]+)">/g)].map((match) => match[1]);
    if (related.some((route) => !hasTextFields(chapter.related, [route]))) fail(`${id}: incomplete related link copy`);
    return {
      ...entryBase,
      kind: 'tutorial',
      route: `${hubRoute}${chapter.slug}/`,
      translationOf: `${SOURCE_ROOT}${sourceSlug}/`,
      title: chapter.title,
      description: chapter.description,
      tutorial: { id, chapter, common, related, sourceSteps: source.steps, hubRoute },
    };
  });
  const published = new Map(lessons.map((entry) => [entry.tutorial.id, entry]));
  const journeys = JOURNEYS.map(({ id, start, end }, index) => ({
    ...hub.journeys[index],
    id,
    cards: Array.from({ length: end - start + 1 }, (_, offset) => {
      const lessonId = `TU${String(start + offset).padStart(2, '0')}`;
      const entry = published.get(lessonId);
      if (entry) {
        const { title, summary } = entry.tutorial.chapter;
        return { lessonId, status: 'available', route: entry.route, title, summary };
      }
      const card = pending[lessonId];
      if (!hasTextFields(card, ['title', 'summary'])) fail(`${lessonId}: incomplete planned chapter copy`);
      return { lessonId, status: lessonId === 'TU11' ? 'soon' : 'planned', ...card };
    }),
  }));
  const hubEntry = {
    ...entryBase,
    kind: 'tutorial-hub',
    route: hubRoute,
    translationOf: SOURCE_ROOT,
    title: hub.title,
    description: hub.description,
    tutorial: { common, hub, journeys },
  };
  const navTarget = ({ kind, route, title }) => ({ kind, route, title });
  lessons.forEach(({ tutorial }, index) => {
    tutorial.previous = navTarget(lessons[index - 1] || hubEntry);
    tutorial.next = navTarget(lessons[index + 1] || hubEntry);
  });
  return [hubEntry, ...lessons];
}

function message(pattern, values) {
  return pattern.replace(/\{(\w+)\}/g, (_, key) => values[key]);
}

function renderHub(entry, navigation) {
  const { common, hub, journeys } = entry.tutorial;
  const top = journeys.map((journey) => `<a class="step-card" href="#${journey.id}">
          <h3 class="step-title">${esc(journey.title)}</h3>
          <p class="step-desc">${esc(journey.description)}</p>
          <span class="text-link">${esc(common.chapters)}</span>
        </a>`).join('\n        ');
  const groups = journeys.map((journey) => {
    const cards = journey.cards.map((card) => {
      const heading = card.route ? `<a href="${esc(card.route)}">${esc(card.title)}</a>` : esc(card.title);
      return `<li class="lesson-step chapter-card"${card.route ? '' : ' data-pending'}>
            <p class="chapter-top"><span class="chapter-id">${card.lessonId}</span><span class="lesson-availability">${esc(common[card.status])}</span></p>
            <h3>${heading}</h3><p class="lesson-meta">${esc(card.summary)}</p>${card.route ? ARROW : ''}
          </li>`;
    }).join('\n          ');
    return `<h2 id="${journey.id}">${esc(journey.title)}</h2>
        <ol class="lesson-steps">
          ${cards}
        </ol>`;
  }).join('\n        ');
  return `<article class="legal-article">
        <h1>${esc(entry.title)}</h1>
        <p class="article-lead">${esc(hub.intro)}</p>
        <p>${esc(hub.statuses)}</p>
        <h2>${esc(hub.start)}</h2>
        <div class="grid-3 journey-grid">
        ${top}
        </div>
        ${groups}
        <h2>${esc(hub.sourcesTitle)}</h2>
        <p>${esc(hub.sources)}</p>
        <p>${navigation('/guias/', common.guides)} · ${navigation('/ferramentas/', common.tools)}</p>
        <p>${navigation('/', common.home)} · <a href="https://github.com/AlexandreZanata/abastevo">${esc(common.github)}</a></p>
      </article>`;
}

function renderLesson(entry, navigation) {
  const { chapter, common, sourceSteps, id, previous, next, related } = entry.tutorial;
  const count = message(common.stepCount, { total: sourceSteps.length });
  const textOnly = sourceSteps.filter((step) => step.textOnly).length;
  const context = [`${common.context} ${id} · ${count}`, textOnly ? message(common.describedCount, { total: textOnly }) : '']
    .filter(Boolean).join(' · ');
  const steps = sourceSteps.map(({ stepId, textOnly }, index) => {
    const copy = chapter.steps[stepId];
    const visual = textOnly
      ? `<p class="lesson-text-only"><strong>${esc(common.noCapture)}</strong> ${esc(copy.visual)}</p>`
      : `<div class="tutorial-visual">
              <p class="lesson-meta">${esc(copy.visual)}</p>
              <a class="text-link" href="${esc(entry.translationOf)}#${esc(stepId)}" hreflang="pt-BR" aria-label="${esc(common.originalCapture)} · ${esc(common.step)} ${index + 1}">${esc(common.originalCapture)}</a>
            </div>`;
    return `<li class="lesson-step" id="${esc(stepId)}"${textOnly ? ' data-text-only' : ''}>
            <h2>${esc(common.step)} ${index + 1} · ${esc(copy.title)}</h2>
            <p>${esc(copy.instruction)}</p>
            ${visual}
            <p><strong>${esc(common.expected)}</strong> ${esc(copy.expected)}</p>
            <p><strong>${esc(common.recovery)}</strong> ${esc(copy.recovery)}</p>
          </li>`;
  });
  const navigationButton = (target, direction) => {
    const hub = target.kind === 'tutorial-hub';
    const back = direction === 'previous';
    const arrow = back ? '←' : '→';
    const caption = hub ? common.index : common[direction];
    return `<a href="${esc(target.route)}" class="btn-${back ? 'secondary' : 'primary'}"><span class="lesson-nav-label">${esc(caption)} <span aria-hidden="true">${arrow}</span></span><span class="lesson-nav-title">${esc(hub ? common.all : target.title)}</span></a>`;
  };
  const links = related.map((route) => `<li>${navigation(route, chapter.related[route])}</li>`);
  return `<article class="legal-article lesson-article">
        <p><span class="lesson-availability">${esc(common.available)}</span></p>
        <h1>${esc(entry.title)}</h1>
        <p class="article-lead">${esc(entry.description)}</p>
        <p class="lesson-context">${esc(context)}. ${esc(common.portuguese)}</p>
        <ol class="lesson-steps">
          ${steps.join('\n          ')}
        </ol>
        <nav class="lesson-nav" aria-label="${esc(common.navigation)}">
          ${navigationButton(previous, 'previous')}
          ${navigationButton(next, 'next')}
        </nav>
        <h2 id="continue-explorando">${esc(common.explore)}</h2>
        <ul class="link-list">${links.join('')}</ul>
      </article>`;
}

export function renderTutorialContent(entry, navigation) {
  return entry.kind === 'tutorial-hub' ? renderHub(entry, navigation) : renderLesson(entry, navigation);
}
