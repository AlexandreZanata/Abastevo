// Form validation. Pure functions, DOM-free. The server is authoritative;
// these checks only fail fast with friendly messages.

export const REASON_MIN = 10;
export const REASON_MAX = 500;
export const SEARCH_MIN = 3;
export const TIERS = ['PRO', 'CONTRIBUTOR', 'ENTERPRISE'] as const;
export type Tier = (typeof TIERS)[number];

export function reasonError(raw: string): string | null {
  const length = Array.from(raw.trim()).length;
  if (length < REASON_MIN || length > REASON_MAX) {
    return `Motivo deve ter de ${REASON_MIN} a ${REASON_MAX} caracteres.`;
  }
  return null;
}

export function isTier(raw: string): raw is Tier {
  return (TIERS as readonly string[]).includes(raw);
}

export function isUuid(raw: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(raw);
}

export function searchError(raw: string): string | null {
  if (raw.trim().length < SEARCH_MIN) {
    return `Busca precisa de ao menos ${SEARCH_MIN} caracteres.`;
  }
  return null;
}

const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;

// Optional end date. Empty means open-ended. A provided date must be a real
// calendar day, not before today and within 5 years; the server enforces the
// exact 1h..5y window.
export function endDateError(raw: string, now: Date = new Date()): string | null {
  const trimmed = raw.trim();
  if (trimmed === '') {
    return null;
  }
  const match = DATE_PATTERN.exec(trimmed);
  if (match === null) {
    return 'Data final deve estar no formato AAAA-MM-DD.';
  }
  const year = Number.parseInt(match[1], 10);
  const month = Number.parseInt(match[2], 10);
  const day = Number.parseInt(match[3], 10);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return 'Data final inválida.';
  }
  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  if (date < today) {
    return 'Data final não pode estar no passado.';
  }
  const max = new Date(today);
  max.setUTCFullYear(max.getUTCFullYear() + 5);
  if (date > max) {
    return 'Data final deve estar dentro de 5 anos.';
  }
  return null;
}

export function defaultEndDate(now: Date = new Date()): string {
  const date = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  date.setUTCDate(date.getUTCDate() + 30);
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, '0');
  const day = String(date.getUTCDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function toEndOfDayIso(date: string): string {
  return `${date}T00:00:00Z`;
}
