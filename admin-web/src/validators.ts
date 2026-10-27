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

// Calendar days are Brazilian days. Brazil has had no DST since 2019, so the
// civil offset is a fixed UTC-03:00.
export const BRAZIL_OFFSET = '-03:00';
const BRAZIL_OFFSET_MS = -3 * 60 * 60 * 1000;
const HOUR_MS = 60 * 60 * 1000;
const DAY_MS = 24 * HOUR_MS;

// ENT-04: now + 1h <= end_at <= now + 5 years, where the server counts
// 5 years as 5 x 365 days.
export const GRANT_MIN_MS = HOUR_MS;
export const GRANT_MAX_DAYS = 5 * 365;
export const GRANT_MAX_MS = GRANT_MAX_DAYS * DAY_MS;

// Today's calendar date in Brazil, as a UTC-midnight Date (UTC getters on a
// shifted instant).
function brazilToday(now: Date): Date {
  const shifted = new Date(now.getTime() + BRAZIL_OFFSET_MS);
  return new Date(Date.UTC(shifted.getUTCFullYear(), shifted.getUTCMonth(), shifted.getUTCDate()));
}

function formatDate(date: Date): string {
  const year = String(date.getUTCFullYear()).padStart(4, '0');
  const month = String(date.getUTCMonth() + 1).padStart(2, '0');
  const day = String(date.getUTCDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

// Instant check mirroring the server window, both bounds inclusive.
export function endInstantError(endAt: Date, now: Date = new Date()): string | null {
  const end = endAt.getTime();
  if (end < now.getTime() + GRANT_MIN_MS) {
    return 'Data final deve ser ao menos 1 hora depois de agora.';
  }
  if (end > now.getTime() + GRANT_MAX_MS) {
    return `Data final deve estar dentro de 5 anos (${GRANT_MAX_DAYS} dias).`;
  }
  return null;
}

// Optional end date. Empty means open-ended. A provided date must be a real
// calendar day; the instant actually sent (end of that day in Brazil) must
// fit the ENT-04 window. The server stays authoritative.
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
  if (date < brazilToday(now)) {
    return 'Data final não pode estar no passado.';
  }
  return endInstantError(new Date(toEndOfDayIso(trimmed)), now);
}

export function defaultEndDate(now: Date = new Date()): string {
  const date = brazilToday(now);
  date.setUTCDate(date.getUTCDate() + 30);
  return formatDate(date);
}

// Last second of the picked calendar day in Brazil, as RFC 3339.
export function toEndOfDayIso(date: string): string {
  return `${date}T23:59:59${BRAZIL_OFFSET}`;
}
