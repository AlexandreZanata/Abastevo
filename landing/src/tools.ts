/**
 * Fuel calculators (SEO-10): pure parsing, validation and formulas plus an
 * optional DOM enhancement. Every tool page explains the formula and a worked
 * example in plain HTML, so the page stays useful with JavaScript disabled.
 * Nothing is sent anywhere: values are computed in the browser and never stored.
 */

export type FieldKind = 'price' | 'decimal' | 'integer' | 'choice' | 'flag';

export interface FieldRule {
  name: string;
  label: string;
  kind: FieldKind;
  required: boolean;
  max?: number;
  allowZero?: boolean;
  unit?: string;
  example?: string;
  choices?: readonly string[];
}

export interface ToolSpec {
  id: string;
  fields: readonly FieldRule[];
  compute(values: Record<string, number | boolean | null>): ToolOutcome;
}

export type ToolOutcome =
  | { ok: true; lines: string[] }
  | { ok: false; errors: Record<string, string> };

export type FieldResult =
  | { ok: true; value: number | boolean | null }
  | { ok: false; error: string };

// ---------------------------------------------------------------------------
// Parsing and formatting (pt-BR).

const THOUSANDS_DOT = /^\d{1,3}(\.\d{3})+$/;

/**
 * Parses a pt-BR number. A comma is the decimal separator ("6,29"); dots
 * before a comma group thousands ("1.234,56"). Without a comma, a dot followed
 * by exact groups of three digits is read as thousands ("1.200" = 1200) unless
 * `dotIsDecimal` is set, which price fields use so "6.299" stays R$ 6,299.
 * Returns null for anything that is not a plain number.
 */
export function parseDecimalBR(raw: string, dotIsDecimal = false): number | null {
  let text = raw.trim().replace(/^R\$/i, '').replace(/[\s ]+/g, '');
  if (text === '') return null;
  let sign = 1;
  if (text.startsWith('-') || text.startsWith('+')) {
    sign = text.startsWith('-') ? -1 : 1;
    text = text.slice(1);
  }
  if (!/^[\d.,]+$/.test(text) || !/\d/.test(text)) return null;
  let normalized: string;
  const comma = text.lastIndexOf(',');
  const dot = text.lastIndexOf('.');
  if (comma !== -1 && dot !== -1) {
    if (comma > dot) {
      const intPart = text.slice(0, comma);
      if (!THOUSANDS_DOT.test(intPart) || !/^\d+$/.test(text.slice(comma + 1))) return null;
      normalized = `${intPart.replace(/\./g, '')}.${text.slice(comma + 1)}`;
    } else {
      const intPart = text.slice(0, dot);
      if (!/^\d{1,3}(,\d{3})+$/.test(intPart) || !/^\d+$/.test(text.slice(dot + 1))) return null;
      normalized = `${intPart.replace(/,/g, '')}.${text.slice(dot + 1)}`;
    }
  } else if (comma !== -1) {
    if (!/^\d*,\d+$/.test(text)) return null;
    normalized = text.replace(',', '.');
  } else if (dot !== -1) {
    if (!dotIsDecimal && THOUSANDS_DOT.test(text) && !text.startsWith('0')) {
      normalized = text.replace(/\./g, '');
    } else if (/^\d*\.\d+$/.test(text)) {
      normalized = text;
    } else {
      return null;
    }
  } else {
    normalized = text;
  }
  const value = sign * Number(normalized);
  return Number.isFinite(value) ? value : null;
}

/** Formats with a decimal comma and dot thousands, deterministic (no ICU). */
export function formatNumberBR(value: number, decimals: number): string {
  const fixed = Math.abs(value).toFixed(decimals);
  const [intPart, fraction] = fixed.split('.');
  const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, '.');
  const sign = value < 0 && Number(fixed) !== 0 ? '-' : '';
  return fraction === undefined ? `${sign}${grouped}` : `${sign}${grouped},${fraction}`;
}

export function formatBRL(value: number, decimals = 2): string {
  return `R$ ${formatNumberBR(value, decimals)}`;
}

export function formatPercent(ratio: number): string {
  return `${formatNumberBR(ratio * 100, 1)}%`;
}

// ---------------------------------------------------------------------------
// Validation.

export function isTruthyFlag(raw: string): boolean {
  return ['on', '1', 'true', 'sim'].includes(raw.trim().toLowerCase());
}

export function validateField(rule: FieldRule, raw: string): FieldResult {
  const text = raw.trim();
  if (rule.kind === 'flag') {
    return { ok: true, value: isTruthyFlag(text) };
  }
  // Messages render right under their field, prefixed by the field name.
  const fail = (message: string): FieldResult => ({ ok: false, error: `${rule.label}: ${message}` });
  if (text === '') {
    return rule.required ? fail('campo obrigatório.') : { ok: true, value: null };
  }
  if (rule.kind === 'choice') {
    if (!rule.choices?.includes(text)) return fail('escolha uma das opções.');
    return { ok: true, value: Number(text) };
  }
  const value = parseDecimalBR(text, rule.kind === 'price');
  const example = rule.example ? `, por exemplo ${rule.example}` : '';
  if (value === null) return fail(`digite um número${example}.`);
  if (value < 0) return fail('o valor não pode ser negativo.');
  if (value === 0 && !rule.allowZero) return fail('o valor precisa ser maior que zero.');
  if (rule.kind === 'integer' && !Number.isInteger(value)) return fail('use um número inteiro.');
  if (rule.max !== undefined && value > rule.max) {
    const unit = rule.unit ? ` ${rule.unit}` : '';
    return fail(`valor alto demais (máximo ${formatNumberBR(rule.max, 0)}${unit}). Confira a digitação.`);
  }
  return { ok: true, value };
}

// ---------------------------------------------------------------------------
// Formulas.

export const DEFAULT_PARITY = 0.7;
const TIE_TOLERANCE = 0.005;

export interface EthanolComparison {
  ratio: number;
  threshold: number;
  thresholdSource: 'regra' | 'consumo';
  choice: 'etanol' | 'gasolina' | 'empate';
  maxEthanolPrice: number;
  costPerKmEthanol: number | null;
  costPerKmGasoline: number | null;
}

export function compareEthanolGasoline(input: {
  gasoline: number;
  ethanol: number;
  kmPerLiterGasoline?: number | null;
  kmPerLiterEthanol?: number | null;
}): EthanolComparison {
  const { gasoline, ethanol } = input;
  const ownConsumption = Boolean(input.kmPerLiterGasoline && input.kmPerLiterEthanol);
  const threshold = ownConsumption ? (input.kmPerLiterEthanol as number) / (input.kmPerLiterGasoline as number) : DEFAULT_PARITY;
  const ratio = ethanol / gasoline;
  const diff = ratio - threshold;
  const choice = Math.abs(diff) < TIE_TOLERANCE ? 'empate' : diff < 0 ? 'etanol' : 'gasolina';
  return {
    ratio,
    threshold,
    thresholdSource: ownConsumption ? 'consumo' : 'regra',
    choice,
    maxEthanolPrice: gasoline * threshold,
    costPerKmEthanol: ownConsumption ? ethanol / (input.kmPerLiterEthanol as number) : null,
    costPerKmGasoline: ownConsumption ? gasoline / (input.kmPerLiterGasoline as number) : null,
  };
}

export function tripCost(input: {
  distanceKm: number;
  kmPerLiter: number;
  pricePerLiter: number;
  roundTrip?: boolean;
  tolls?: number | null;
  people?: number | null;
}): { totalKm: number; liters: number; fuelCost: number; total: number; perPerson: number; people: number } {
  const totalKm = input.distanceKm * (input.roundTrip ? 2 : 1);
  const liters = totalKm / input.kmPerLiter;
  const fuelCost = liters * input.pricePerLiter;
  const total = fuelCost + (input.tolls ?? 0);
  const people = input.people && input.people > 0 ? input.people : 1;
  return { totalKm, liters, fuelCost, total, perPerson: total / people, people };
}

export function averageConsumption(input: {
  distanceKm: number;
  liters: number;
  pricePerLiter?: number | null;
}): { kmPerLiter: number; litersPer100Km: number; costPerKm: number | null; spent: number | null } {
  const kmPerLiter = input.distanceKm / input.liters;
  const price = input.pricePerLiter ?? null;
  return {
    kmPerLiter,
    litersPer100Km: (input.liters / input.distanceKm) * 100,
    costPerKm: price === null ? null : price / kmPerLiter,
    spent: price === null ? null : price * input.liters,
  };
}

export function costPerKm(input: {
  fuelPrice: number;
  kmPerLiter: number;
  kwhPrice: number;
  kmPerKwh: number;
  monthlyKm?: number | null;
}): {
  combustion: number;
  electric: number;
  cheaper: 'eletrico' | 'combustao' | 'empate';
  difference: number;
  monthlyCombustion: number | null;
  monthlyElectric: number | null;
} {
  const combustion = input.fuelPrice / input.kmPerLiter;
  const electric = input.kwhPrice / input.kmPerKwh;
  const difference = Math.abs(combustion - electric);
  const cheaper = difference < 0.0005 ? 'empate' : electric < combustion ? 'eletrico' : 'combustao';
  const monthly = input.monthlyKm ?? null;
  return {
    combustion,
    electric,
    cheaper,
    difference,
    monthlyCombustion: monthly === null ? null : combustion * monthly,
    monthlyElectric: monthly === null ? null : electric * monthly,
  };
}

export function fillTank(input: {
  capacityLiters: number;
  currentPercent: number;
  pricePerLiter: number;
  kmPerLiter?: number | null;
}): { liters: number; cost: number; fullTankCost: number; rangeKm: number | null } {
  const liters = input.capacityLiters * (1 - input.currentPercent / 100);
  return {
    liters,
    cost: liters * input.pricePerLiter,
    fullTankCost: input.capacityLiters * input.pricePerLiter,
    rangeKm: input.kmPerLiter ? input.capacityLiters * input.kmPerLiter : null,
  };
}

// ---------------------------------------------------------------------------
// Tool specs: field rules + result wording. Labels must match the HTML form.

const PRICE_MAX = 30;
const KML_MAX = 60;

const num = (value: number | boolean | null): number => value as number;
const opt = (value: number | boolean | null): number | null => (typeof value === 'number' ? value : null);

export const TOOL_SPECS: Record<string, ToolSpec> = {
  'etanol-ou-gasolina': {
    id: 'etanol-ou-gasolina',
    fields: [
      { name: 'gasolina', label: 'Preço da gasolina', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '6,29' },
      { name: 'etanol', label: 'Preço do etanol', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '4,19' },
      { name: 'consumo_gasolina', label: 'Consumo com gasolina', kind: 'decimal', required: false, max: KML_MAX, unit: 'km/l', example: '12,5' },
      { name: 'consumo_etanol', label: 'Consumo com etanol', kind: 'decimal', required: false, max: KML_MAX, unit: 'km/l', example: '8,7' },
    ],
    compute(v) {
      const kmlG = opt(v.consumo_gasolina);
      const kmlE = opt(v.consumo_etanol);
      if ((kmlG === null) !== (kmlE === null)) {
        const missing = kmlG === null ? 'consumo_gasolina' : 'consumo_etanol';
        return { ok: false, errors: { [missing]: 'Para usar o consumo do seu carro, preencha os dois consumos (gasolina e etanol) ou deixe ambos em branco.' } };
      }
      const r = compareEthanolGasoline({ gasoline: num(v.gasolina), ethanol: num(v.etanol), kmPerLiterGasoline: kmlG, kmPerLiterEthanol: kmlE });
      const verdict = r.choice === 'etanol'
        ? 'Resultado: o etanol tende a sair mais barato por quilômetro rodado.'
        : r.choice === 'gasolina'
          ? 'Resultado: a gasolina tende a sair mais barata por quilômetro rodado.'
          : 'Resultado: empate técnico; os dois custam praticamente o mesmo por quilômetro.';
      const lines = [
        verdict,
        `O etanol custa ${formatPercent(r.ratio)} do preço da gasolina.`,
        r.thresholdSource === 'consumo'
          ? `Referência usada: ${formatPercent(r.threshold)}, calculada com o consumo informado do seu carro.`
          : 'Referência usada: 70%, a regra geral para carros flex.',
        `Com a gasolina a ${formatBRL(num(v.gasolina), 3)}, o etanol compensa até ${formatBRL(r.maxEthanolPrice, 3)} o litro.`,
      ];
      if (r.costPerKmEthanol !== null && r.costPerKmGasoline !== null) {
        lines.push(`Custo por km: etanol ${formatBRL(r.costPerKmEthanol, 3)}, gasolina ${formatBRL(r.costPerKmGasoline, 3)}.`);
      }
      return { ok: true, lines };
    },
  },
  'custo-de-viagem': {
    id: 'custo-de-viagem',
    fields: [
      { name: 'distancia', label: 'Distância', kind: 'decimal', required: true, max: 20000, unit: 'km', example: '430' },
      { name: 'consumo', label: 'Consumo do veículo', kind: 'decimal', required: true, max: KML_MAX, unit: 'km/l', example: '12' },
      { name: 'preco', label: 'Preço do combustível', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '6,29' },
      { name: 'ida_e_volta', label: 'Ida e volta', kind: 'flag', required: false },
      { name: 'pedagio', label: 'Total de pedágios', kind: 'price', required: false, max: 10000, allowZero: true, unit: 'reais', example: '45,80' },
      { name: 'pessoas', label: 'Pessoas para dividir', kind: 'integer', required: false, max: 50, unit: 'pessoas', example: '2' },
    ],
    compute(v) {
      const r = tripCost({
        distanceKm: num(v.distancia),
        kmPerLiter: num(v.consumo),
        pricePerLiter: num(v.preco),
        roundTrip: v.ida_e_volta === true,
        tolls: opt(v.pedagio),
        people: opt(v.pessoas),
      });
      const lines = [
        `Custo total estimado: ${formatBRL(r.total)}.`,
        `Distância considerada: ${formatNumberBR(r.totalKm, 0)} km${v.ida_e_volta === true ? ' (ida e volta)' : ''}.`,
        `Combustível necessário: ${formatNumberBR(r.liters, 1)} litros, ${formatBRL(r.fuelCost)}.`,
      ];
      if (opt(v.pedagio) !== null) lines.push(`Pedágios informados: ${formatBRL(num(v.pedagio))}.`);
      if (r.people > 1) lines.push(`Dividido por ${r.people} pessoas: ${formatBRL(r.perPerson)} cada.`);
      return { ok: true, lines };
    },
  },
  'consumo-medio': {
    id: 'consumo-medio',
    fields: [
      { name: 'distancia', label: 'Distância percorrida', kind: 'decimal', required: true, max: 5000, unit: 'km', example: '412' },
      { name: 'litros', label: 'Litros abastecidos', kind: 'decimal', required: true, max: 1000, unit: 'litros', example: '35,6' },
      { name: 'preco', label: 'Preço por litro', kind: 'price', required: false, max: PRICE_MAX, unit: 'reais por litro', example: '6,29' },
    ],
    compute(v) {
      const r = averageConsumption({ distanceKm: num(v.distancia), liters: num(v.litros), pricePerLiter: opt(v.preco) });
      const lines = [
        `Consumo médio: ${formatNumberBR(r.kmPerLiter, 1)} km/l.`,
        `O mesmo consumo em litros: ${formatNumberBR(r.litersPer100Km, 1)} litros a cada 100 km.`,
      ];
      if (r.costPerKm !== null && r.spent !== null) {
        lines.push(`Custo por km: ${formatBRL(r.costPerKm, 3)} (gasto no abastecimento: ${formatBRL(r.spent)}).`);
      }
      return { ok: true, lines };
    },
  },
  'custo-por-km': {
    id: 'custo-por-km',
    fields: [
      { name: 'preco_combustivel', label: 'Preço do combustível', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '6,29' },
      { name: 'consumo', label: 'Consumo do carro a combustão', kind: 'decimal', required: true, max: KML_MAX, unit: 'km/l', example: '12' },
      { name: 'tarifa_kwh', label: 'Tarifa de energia', kind: 'price', required: true, max: 10, unit: 'reais por kWh', example: '0,95' },
      { name: 'consumo_eletrico', label: 'Consumo do carro elétrico', kind: 'decimal', required: true, max: 20, unit: 'km/kWh', example: '6,5' },
      { name: 'km_mes', label: 'Quilômetros por mês', kind: 'decimal', required: false, max: 50000, unit: 'km', example: '1.200' },
    ],
    compute(v) {
      const r = costPerKm({
        fuelPrice: num(v.preco_combustivel),
        kmPerLiter: num(v.consumo),
        kwhPrice: num(v.tarifa_kwh),
        kmPerKwh: num(v.consumo_eletrico),
        monthlyKm: opt(v.km_mes),
      });
      const verdict = r.cheaper === 'eletrico'
        ? `Resultado: o elétrico gasta ${formatBRL(r.difference, 3)} a menos por km em energia.`
        : r.cheaper === 'combustao'
          ? `Resultado: o carro a combustão gasta ${formatBRL(r.difference, 3)} a menos por km em energia.`
          : 'Resultado: os dois gastam praticamente o mesmo por km em energia.';
      const lines = [
        verdict,
        `Combustão: ${formatBRL(r.combustion, 3)} por km.`,
        `Elétrico: ${formatBRL(r.electric, 3)} por km.`,
      ];
      if (r.monthlyCombustion !== null && r.monthlyElectric !== null) {
        lines.push(`Em ${formatNumberBR(num(v.km_mes), 0)} km por mês: combustão ${formatBRL(r.monthlyCombustion)}, elétrico ${formatBRL(r.monthlyElectric)}.`);
      }
      return { ok: true, lines };
    },
  },
  'encher-o-tanque': {
    id: 'encher-o-tanque',
    fields: [
      { name: 'capacidade', label: 'Capacidade do tanque', kind: 'decimal', required: true, max: 1500, unit: 'litros', example: '50' },
      { name: 'nivel', label: 'Nível atual do tanque', kind: 'choice', required: true, choices: ['0', '12.5', '25', '37.5', '50', '62.5', '75', '87.5'] },
      { name: 'preco', label: 'Preço por litro', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '6,29' },
      { name: 'consumo', label: 'Consumo do veículo', kind: 'decimal', required: false, max: KML_MAX, unit: 'km/l', example: '12' },
    ],
    compute(v) {
      const r = fillTank({ capacityLiters: num(v.capacidade), currentPercent: num(v.nivel), pricePerLiter: num(v.preco), kmPerLiter: opt(v.consumo) });
      const lines = [
        `Para completar: cerca de ${formatNumberBR(r.liters, 1)} litros, ${formatBRL(r.cost)}.`,
        `O tanque inteiro, vazio até cheio, custaria ${formatBRL(r.fullTankCost)}.`,
      ];
      if (r.rangeKm !== null) lines.push(`Autonomia estimada com o tanque cheio: ${formatNumberBR(r.rangeKm, 0)} km.`);
      return { ok: true, lines };
    },
  },
};

/** Validates raw form strings and runs the tool. Pure and DOM-free. */
export function runTool(id: string, raw: Record<string, string>): ToolOutcome {
  const spec = TOOL_SPECS[id];
  if (!spec) return { ok: false, errors: { _: `Ferramenta desconhecida: ${id}` } };
  const values: Record<string, number | boolean | null> = {};
  const errors: Record<string, string> = {};
  for (const rule of spec.fields) {
    const result = validateField(rule, raw[rule.name] ?? '');
    if (result.ok) values[rule.name] = result.value;
    else errors[rule.name] = result.error;
  }
  if (Object.keys(errors).length > 0) return { ok: false, errors };
  return spec.compute(values);
}

/** Reads optional `?campo=valor` prefill for known fields only. */
export function readPrefill(search: string, spec: ToolSpec): Record<string, string> {
  const params = new URLSearchParams(search);
  const prefill: Record<string, string> = {};
  for (const rule of spec.fields) {
    const value = params.get(rule.name);
    if (value !== null && value.length <= 32) prefill[rule.name] = value;
  }
  return prefill;
}

// ---------------------------------------------------------------------------
// DOM enhancement (browser only). Uses textContent exclusively.

interface FieldElement {
  value: string;
  checked?: boolean;
  type?: string;
  setAttribute(name: string, value: string): void;
  removeAttribute(name: string): void;
  focus?(): void;
}

function fieldValue(el: FieldElement): string {
  return el.type === 'checkbox' ? (el.checked ? 'on' : '') : el.value;
}

export function renderOutcome(form: HTMLFormElement, result: HTMLElement, spec: ToolSpec, moveFocus: boolean): ToolOutcome {
  const raw: Record<string, string> = {};
  const elements = new Map<string, FieldElement>();
  for (const rule of spec.fields) {
    const el = form.elements.namedItem(rule.name) as FieldElement | null;
    if (el) {
      elements.set(rule.name, el);
      raw[rule.name] = fieldValue(el);
    }
  }
  const outcome = runTool(spec.id, raw);
  const errors = outcome.ok ? {} : outcome.errors;
  let firstInvalid: FieldElement | null = null;
  for (const rule of spec.fields) {
    const el = elements.get(rule.name);
    const message = form.querySelector<HTMLElement>(`[data-error-for="${rule.name}"]`);
    const error = errors[rule.name];
    if (error) {
      el?.setAttribute('aria-invalid', 'true');
      if (message) {
        message.textContent = error;
        message.hidden = false;
      }
      if (!firstInvalid && el) firstInvalid = el;
    } else {
      el?.removeAttribute('aria-invalid');
      if (message) {
        message.textContent = '';
        message.hidden = true;
      }
    }
  }
  const doc = form.ownerDocument;
  const nodes: HTMLElement[] = [];
  if (outcome.ok) {
    for (const line of outcome.lines) {
      const p = doc.createElement('p');
      p.textContent = line;
      nodes.push(p);
    }
  } else {
    const p = doc.createElement('p');
    p.textContent = 'Revise os campos destacados para calcular.';
    nodes.push(p);
  }
  result.replaceChildren(...nodes);
  result.hidden = false;
  if (moveFocus && firstInvalid?.focus) firstInvalid.focus();
  return outcome;
}

export function setupTools(doc: Document, search: string): void {
  for (const form of Array.from(doc.querySelectorAll<HTMLFormElement>('form[data-tool]'))) {
    const spec = TOOL_SPECS[form.dataset.tool ?? ''];
    const panel = form.closest<HTMLElement>('[data-tool-panel]');
    const result = panel?.querySelector<HTMLElement>('[data-tool-result]');
    if (!spec || !panel || !result) continue;
    panel.querySelector<HTMLElement>('[data-tool-fallback]')?.setAttribute('hidden', '');
    form.hidden = false;
    const prefill = readPrefill(search, spec);
    for (const [name, value] of Object.entries(prefill)) {
      const el = form.elements.namedItem(name) as FieldElement | null;
      if (!el) continue;
      if (el.type === 'checkbox') el.checked = isTruthyFlag(value);
      else el.value = value;
    }
    form.addEventListener('submit', (event) => {
      event.preventDefault();
      renderOutcome(form, result, spec, true);
    });
    if (Object.keys(prefill).length > 0) renderOutcome(form, result, spec, false);
  }
}

if (typeof document !== 'undefined' && typeof location !== 'undefined') {
  const start = (): void => setupTools(document, location.search);
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
}
