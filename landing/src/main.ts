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
  // Enhancement only: the details/summary menu already works without JavaScript.
  const toggle = document.querySelector<HTMLDetailsElement>('.nav-toggle');
  if (!toggle) return;
  toggle.querySelectorAll('a').forEach((link) => {
    link.addEventListener('click', () => {
      toggle.removeAttribute('open');
    });
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      toggle.removeAttribute('open');
    }
  });
}

function setupTheme(): void {
  // Enhancement: without JavaScript the OS-level dark scheme still applies via CSS.
  const toggle = document.querySelector<HTMLButtonElement>('[data-theme-toggle]');
  if (!toggle) return;
  const root = document.documentElement;
  const sun = toggle.querySelector('.icon-sun');
  const moon = toggle.querySelector('.icon-moon');
  const apply = (theme: string): void => {
    root.setAttribute('data-theme', theme);
    const dark = theme === 'dark';
    if (sun) {
      if (dark) {
        sun.setAttribute('hidden', '');
      } else {
        sun.removeAttribute('hidden');
      }
    }
    if (moon) {
      if (dark) {
        moon.removeAttribute('hidden');
      } else {
        moon.setAttribute('hidden', '');
      }
    }
  };
  let initial = 'light';
  try {
    const stored = window.localStorage.getItem('abastevo-theme');
    if (stored === 'light' || stored === 'dark') {
      initial = stored;
    } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      initial = 'dark';
    }
  } catch {
    initial = 'light';
  }
  apply(initial);
  toggle.addEventListener('click', () => {
    const next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    try {
      window.localStorage.setItem('abastevo-theme', next);
    } catch {
      // Private mode: keep the session-only choice.
    }
    apply(next);
  });
}

function init(): void {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      setupNavigationHighlight();
      setupCopyHelper();
      setupBackToTop();
      setupMobileNav();
      setupTheme();
    });
  } else {
    setupNavigationHighlight();
    setupCopyHelper();
    setupBackToTop();
    setupMobileNav();
    setupTheme();
  }
}

init();
