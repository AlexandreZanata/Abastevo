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

function init(): void {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      setupNavigationHighlight();
      setupCopyHelper();
      setupBackToTop();
    });
  } else {
    setupNavigationHighlight();
    setupCopyHelper();
    setupBackToTop();
  }
}

init();
