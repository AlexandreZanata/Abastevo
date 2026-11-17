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
        btn.textContent = 'Copiado!';
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

function init(): void {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      setupMobileNav();
      setupNavigationHighlight();
      setupCopyHelper();
      setupBackToTop();
      setupCaptureZoom();
    });
  } else {
    setupMobileNav();
    setupNavigationHighlight();
    setupCopyHelper();
    setupBackToTop();
    setupCaptureZoom();
  }
}

init();
