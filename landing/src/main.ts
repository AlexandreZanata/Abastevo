/**
 * abastevo landing page progressive enhancement.
 * Essential content and navigation work completely without JavaScript.
 */

function setupNavigationHighlight(): void {
  if (typeof window === 'undefined' || !('IntersectionObserver' in window)) {
    return;
  }

  const navLinks = document.querySelectorAll<HTMLAnchorElement>('.site-nav a[href^="#"]');
  if (navLinks.length === 0) {
    return;
  }

  const sectionIds = Array.from(navLinks)
    .map((link) => link.getAttribute('href')?.replace('#', ''))
    .filter((id): id is string => Boolean(id));

  const sections = sectionIds
    .map((id) => document.getElementById(id))
    .filter((el): el is HTMLElement => el !== null);

  if (sections.length === 0) {
    return;
  }

  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          const id = entry.target.id;
          for (const link of navLinks) {
            const isCurrent = link.getAttribute('href') === `#${id}`;
            if (isCurrent) {
              link.setAttribute('aria-current', 'location');
              link.classList.add('active');
            } else {
              link.removeAttribute('aria-current');
              link.classList.remove('active');
            }
          }
        }
      }
    },
    {
      rootMargin: '-20% 0px -70% 0px',
      threshold: 0,
    }
  );

  for (const section of sections) {
    observer.observe(section);
  }
}

function setupCopyHelper(): void {
  const copyButtons = document.querySelectorAll<HTMLButtonElement>('[data-copy-target]');
  for (const btn of copyButtons) {
    btn.addEventListener('click', async () => {
      const targetSelector = btn.getAttribute('data-copy-target');
      if (!targetSelector) return;
      const targetEl = document.querySelector<HTMLAnchorElement | HTMLElement>(targetSelector);
      const textToCopy = targetEl instanceof HTMLAnchorElement ? targetEl.href : targetEl?.textContent?.trim();
      if (!textToCopy) return;

      try {
        await navigator.clipboard.writeText(textToCopy);
        const originalText = btn.textContent;
        btn.textContent = btn.dataset['copySuccess'] || 'Copiado!';
        btn.setAttribute('aria-live', 'polite');
        setTimeout(() => {
          btn.textContent = originalText;
        }, 2000);
      } catch {
        // Fallback: clipboard permission denied or not supported; silent no-op.
      }
    });
  }
}

function setupBackToTop(): void {
  const backToTopBtn = document.querySelector<HTMLButtonElement>('[data-back-to-top]');
  if (!backToTopBtn) return;

  const prefersReduced = typeof window !== 'undefined' &&
    window.matchMedia &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  window.addEventListener(
    'scroll',
    () => {
      if (window.scrollY > 400) {
        backToTopBtn.removeAttribute('hidden');
        backToTopBtn.setAttribute('aria-hidden', 'false');
      } else {
        backToTopBtn.setAttribute('hidden', '');
        backToTopBtn.setAttribute('aria-hidden', 'true');
      }
    },
    { passive: true }
  );

  backToTopBtn.addEventListener('click', (e) => {
    e.preventDefault();
    window.scrollTo({
      top: 0,
      behavior: prefersReduced ? 'auto' : 'smooth',
    });
  });
}

function setupMobileNav(): void {
  const toggle = document.querySelector<HTMLDetailsElement>('.nav-toggle');
  if (!toggle) return;
  toggle.querySelectorAll('a').forEach((link) => {
    link.addEventListener('click', () => { toggle.removeAttribute('open'); });
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') toggle.removeAttribute('open');
  });
}

/**
 * App captures open enlarged in a native modal dialog. Without JavaScript the
 * captures stay as plain images. The dialog closes with its button, a click on
 * the blurred backdrop or Escape, and focus returns to the capture.
 */
function setupCaptureZoom(): void {
  const captures = document.querySelectorAll<HTMLImageElement>(
    '.app-gallery img, .tutorial-frame img, .app-capture img',
  );
  if (captures.length === 0 || typeof HTMLDialogElement === 'undefined') return;

  const dialog = document.createElement('dialog');
  dialog.className = 'capture-zoom';
  dialog.setAttribute('aria-label', 'Captura do aplicativo ampliada');
  const close = document.createElement('button');
  close.type = 'button';
  close.className = 'capture-zoom-close';
  close.setAttribute('aria-label', 'Fechar captura ampliada');
  close.textContent = '\u00d7';
  const figure = document.createElement('figure');
  const large = document.createElement('img');
  large.width = 360;
  large.height = 640;
  const caption = document.createElement('figcaption');
  figure.append(large, caption);
  dialog.append(close, figure);
  document.body.append(dialog);

  let opener: HTMLElement | null = null;
  close.addEventListener('click', () => dialog.close());
  // A click outside the capture and its caption closes the dialog.
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog || event.target === figure) dialog.close();
  });
  dialog.addEventListener('close', () => {
    document.documentElement.classList.remove('capture-zoom-open');
    opener?.focus();
    opener = null;
  });

  const open = (img: HTMLImageElement): void => {
    opener = img;
    large.srcset = img.srcset;
    large.sizes = '(min-width: 40rem) 420px, 86vw';
    large.src = img.currentSrc || img.src;
    large.alt = img.alt;
    caption.textContent = img.alt;
    document.documentElement.classList.add('capture-zoom-open');
    dialog.showModal();
  };

  for (const img of captures) {
    img.tabIndex = 0;
    img.setAttribute('role', 'button');
    img.setAttribute('aria-haspopup', 'dialog');
    img.classList.add('capture-zoomable');
    img.addEventListener('click', () => open(img));
    img.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        open(img);
      }
    });
  }
}

/**
 * State picker of the hero search. The native <select> keeps the value and
 * submits the form; its popup is replaced by a listbox in the site's style.
 * Keyboard: arrows, Home/End, letters, Enter/Space to choose, Escape to close.
 */
function setupStatePicker(): void {
  const select = document.querySelector<HTMLSelectElement>('.lp-search select[name="uf"]');
  const field = select?.closest<HTMLElement>('.field');
  if (!select || !field) return;

  const options = Array.from(select.options);
  const placeholder = options.find((option) => option.value === '')?.textContent ?? '';
  const stateLabel = field.querySelector('.sr-only')?.textContent?.trim() || 'Estado';
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'uf-button';
  button.setAttribute('aria-haspopup', 'listbox');
  button.setAttribute('aria-expanded', 'false');
  const label = document.createElement('span');
  button.append(label);
  const list = document.createElement('ul');
  list.className = 'uf-list';
  list.id = 'uf-lista';
  list.tabIndex = -1;
  list.hidden = true;
  list.setAttribute('role', 'listbox');
  list.setAttribute('aria-label', stateLabel);
  button.setAttribute('aria-controls', list.id);

  const items = options.filter((option) => option.value !== '').map((option, index) => {
    const item = document.createElement('li');
    item.className = 'uf-option';
    item.id = `uf-opcao-${index}`;
    item.dataset['uf'] = option.value;
    item.setAttribute('role', 'option');
    item.textContent = option.textContent;
    list.append(item);
    return item;
  });
  if (items.length === 0) return;

  let active = 0;
  let typed = '';
  let typedAt = 0;
  const sync = (): void => {
    const current = select.selectedOptions[0];
    const empty = !current || current.value === '';
    label.textContent = empty ? placeholder : current.textContent;
    button.toggleAttribute('data-empty', empty);
    button.setAttribute('aria-label', `${stateLabel}: ${empty ? placeholder : current.textContent}`);
    for (const item of items) item.setAttribute('aria-selected', String(item.dataset['uf'] === select.value));
  };
  const setActive = (index: number): void => {
    active = Math.max(0, Math.min(items.length - 1, index));
    items.forEach((item, i) => item.toggleAttribute('data-active', i === active));
    const item = items[active];
    if (!item) return;
    list.setAttribute('aria-activedescendant', item.id);
    item.scrollIntoView({ block: 'nearest' });
  };
  const close = (refocus: boolean): void => {
    if (list.hidden) return;
    list.hidden = true;
    button.setAttribute('aria-expanded', 'false');
    if (refocus) button.focus();
  };
  const open = (): void => {
    list.hidden = false;
    button.setAttribute('aria-expanded', 'true');
    const selected = items.findIndex((item) => item.dataset['uf'] === select.value);
    list.focus();
    setActive(selected === -1 ? 0 : selected);
  };
  const choose = (index: number): void => {
    const item = items[index];
    if (!item) return;
    select.value = item.dataset['uf'] ?? '';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    sync();
    close(true);
  };

  button.addEventListener('click', () => (list.hidden ? open() : close(true)));
  button.addEventListener('keydown', (event) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      open();
    }
  });
  list.addEventListener('click', (event) => {
    const item = (event.target as HTMLElement).closest<HTMLLIElement>('.uf-option');
    if (item) choose(items.indexOf(item));
  });
  list.addEventListener('mousemove', (event) => {
    const item = (event.target as HTMLElement).closest<HTMLLIElement>('.uf-option');
    if (item && items.indexOf(item) !== active) setActive(items.indexOf(item));
  });
  list.addEventListener('keydown', (event) => {
    const { key } = event;
    if (key === 'ArrowDown') setActive(active + 1);
    else if (key === 'ArrowUp') setActive(active - 1);
    else if (key === 'Home') setActive(0);
    else if (key === 'End') setActive(items.length - 1);
    else if (key === 'Enter' || key === ' ') choose(active);
    else if (key === 'Escape') close(true);
    else if (key === 'Tab') { close(false); return; }
    else if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      const now = Date.now();
      typed = now - typedAt > 700 ? key : typed + key;
      typedAt = now;
      const plain = (text: string): string => text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
      const match = items.findIndex((item) => plain(item.textContent ?? '').startsWith(plain(typed)));
      if (match !== -1) setActive(match);
    } else return;
    event.preventDefault();
  });
  list.addEventListener('focusout', (event) => {
    if (!field.contains(event.relatedTarget as Node | null)) close(false);
  });
  document.addEventListener('pointerdown', (event) => {
    if (!field.contains(event.target as Node)) close(false);
  });
  // The hidden select cannot show the browser's "required" bubble, so an empty
  // choice opens the list instead of failing silently.
  select.required = false;
  select.tabIndex = -1;
  select.setAttribute('aria-hidden', 'true');
  select.form?.addEventListener('submit', (event) => {
    if (select.value === '') {
      event.preventDefault();
      open();
    }
  });
  select.addEventListener('change', sync);

  field.classList.add('uf-enhanced');
  field.append(button, list);
  sync();
}

function init(): void {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      setupMobileNav();
      setupNavigationHighlight();
      setupCopyHelper();
      setupBackToTop();
      setupCaptureZoom();
      setupStatePicker();
    });
  } else {
    setupMobileNav();
    setupNavigationHighlight();
    setupCopyHelper();
    setupBackToTop();
    setupCaptureZoom();
    setupStatePicker();
  }
}

init();
