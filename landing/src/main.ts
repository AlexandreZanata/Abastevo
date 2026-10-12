/**
 * abastevo landing page progressive enhancement.
 * Essential content and navigation work completely without JavaScript.
 */

function setupTheme(): void {
  const root = document.documentElement;
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  let preference: string | null = null;
  try { preference = localStorage.getItem('abastevo-theme'); } catch { /* Storage is optional. */ }
  if (preference !== 'light' && preference !== 'dark') preference = null;
  const buttons = document.querySelectorAll<HTMLButtonElement>('[data-theme-toggle]');
  function apply(dark: boolean): void {
    root.dataset.theme = dark ? 'dark' : 'light';
    const label = dark ? 'Ativar tema claro' : 'Ativar tema escuro';
    for (const button of buttons) {
      button.hidden = false;
      button.setAttribute('aria-label', label);
      button.title = label;
      for (const icon of button.querySelectorAll<SVGElement>('.theme-icon')) {
        icon.toggleAttribute('hidden', icon.classList.contains('theme-icon-sun') ? !dark : dark);
      }
    }
    document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.setAttribute('content', dark ? '#090909' : '#ffffff');
  }
  apply(preference ? preference === 'dark' : system.matches);
  for (const button of buttons) {
    button.addEventListener('click', () => {
      preference = root.dataset.theme === 'dark' ? 'light' : 'dark';
      apply(preference === 'dark');
      try { localStorage.setItem('abastevo-theme', preference); } catch { /* Switching still works. */ }
    });
  }
  system.addEventListener('change', (event) => { if (!preference) apply(event.matches); });
  window.addEventListener('storage', (event) => {
    if (event.key !== 'abastevo-theme' && event.key !== null) return;
    preference = event.newValue === 'light' || event.newValue === 'dark' ? event.newValue : null;
    apply(preference ? preference === 'dark' : system.matches);
  });
}

setupTheme();

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

function init(): void {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      setupMobileNav();
      setupNavigationHighlight();
      setupCopyHelper();
      setupBackToTop();
    });
  } else {
    setupMobileNav();
    setupNavigationHighlight();
    setupCopyHelper();
    setupBackToTop();
  }
}

init();
