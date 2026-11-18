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
  compute(values: Record<string, number | boolean | null>, ctx: ToolContext): ToolOutcome;
}

export type ToolOutcome =
  | { ok: true; lines: string[] }
  | { ok: false; errors: Record<string, string> };

export type FieldResult =
  | { ok: true; value: number | boolean | null }
  | { ok: false; error: string };

// Localized calculator strings read from the page's tool-i18n JSON block.
export interface ToolLocaleField {
  label?: string;
  unit?: string;
  example?: string;
}

export interface ToolLocale {
  decimal: '.' | ',';
  fields?: Record<string, ToolLocaleField>;
  messages?: Record<string, string>;
  validation?: Record<string, string>;
}

export interface ToolContext {
  decimal: string;
  t(key: string, vars?: Record<string, string | number>): string;
  brl(value: number, decimals?: number): string;
  number(value: number, decimals: number): string;
  percent(ratio: number): string;
}

const PT_BR_VALIDATION: Record<string, string> = {
  field: '{label}: {message}',
  required: 'campo obrigatório.',
  choice: 'escolha uma das opções.',
  number: 'digite um número{example}.',
  example: ', por exemplo {example}',
  negative: 'o valor não pode ser negativo.',
  zero: 'o valor precisa ser maior que zero.',
  integer: 'use um número inteiro.',
  max: 'valor alto demais (máximo {max}{unit}). Confira a digitação.',
  unit: ' {unit}',
  review: 'Revise os campos destacados para calcular.',
  unknown: 'Ferramenta desconhecida: {id}',
};

const PT_BR_MESSAGES: Record<string, Record<string, string>> = {
  'etanol-ou-gasolina': {
    missingConsumption: 'Para usar o consumo do seu carro, preencha os dois consumos (gasolina e etanol) ou deixe ambos em branco.',
    ethanolCheaper: 'Resultado: o etanol tende a sair mais barato por quilômetro rodado.',
    gasolineCheaper: 'Resultado: a gasolina tende a sair mais barata por quilômetro rodado.',
    tie: 'Resultado: empate técnico; os dois custam praticamente o mesmo por quilômetro.',
    ratio: 'O etanol custa {percent} do preço da gasolina.',
    thresholdOwn: 'Referência usada: {percent}, calculada com o consumo informado do seu carro.',
    thresholdRule: 'Referência usada: 70%, a regra geral para carros flex.',
    maxPrice: 'Com a gasolina a {gasoline}, o etanol compensa até {ethanol} o litro.',
    costPerKm: 'Custo por km: etanol {ethanol}, gasolina {gasoline}.',
  },
  'custo-de-viagem': {
    total: 'Custo total estimado: {total}.',
    distance: 'Distância considerada: {distance} km{trip}.',
    roundTrip: ' (ida e volta)',
    fuel: 'Combustível necessário: {liters} litros, {cost}.',
    tolls: 'Pedágios informados: {tolls}.',
    perPerson: 'Dividido por {people} pessoas: {amount} cada.',
  },
  'consumo-medio': {
    average: 'Consumo médio: {value} km/l.',
    per100: 'O mesmo consumo em litros: {value} litros a cada 100 km.',
    costPerKm: 'Custo por km: {cost} (gasto no abastecimento: {spent}).',
  },
  'custo-por-km': {
    electricCheaper: 'Resultado: o elétrico gasta {difference} a menos por km em energia.',
    combustionCheaper: 'Resultado: o carro a combustão gasta {difference} a menos por km em energia.',
    tie: 'Resultado: os dois gastam praticamente o mesmo por km em energia.',
    combustion: 'Combustão: {cost} por km.',
    electric: 'Elétrico: {cost} por km.',
    monthly: 'Em {km} km por mês: combustão {combustion}, elétrico {electric}.',
  },
  'encher-o-tanque': {
    fill: 'Para completar: cerca de {liters} litros, {cost}.',
    full: 'O tanque inteiro, vazio até cheio, custaria {cost}.',
    range: 'Autonomia estimada com o tanque cheio: {range} km.',
  },
};

function fillMessage(template: string | undefined, vars?: Record<string, string | number>): string {
  if (template === undefined) return '';
  return template.replace(/\{(\w+)\}/g, (_, name) => (vars && Object.hasOwn(vars, name) ? String(vars[name]) : `{${name}}`));
}

// ---------------------------------------------------------------------------
// Parsing and formatting per locale.

const THOUSANDS_DOT = /^\d{1,3}(\.\d{3})+$/;
const THOUSANDS_COMMA = /^[1-9]\d{0,2}(,\d{3})+$/;

function cleanNumber(raw: string): { text: string; sign: number } | null {
  let text = raw.trim().replace(/^R\$/i, '').replace(/[\s ]+/g, '');
  if (text === '') return null;
  let sign = 1;
  if (text.startsWith('-') || text.startsWith('+')) {
    sign = text.startsWith('-') ? -1 : 1;
    text = text.slice(1);
  }
  if (!/^[\d.,]+$/.test(text) || !/\d/.test(text)) return null;
  return { text, sign };
}

/**
 * Parses a number with the locale's decimal separator. With `,` it matches
 * parseDecimalBR exactly. With `.` a dot is always decimal ("6.299" = 6.299)
 * and a lone comma is thousands only in exact groups of three ("1,200" = 1200);
 * everything ambiguous returns null.
 */
export function parseDecimal(raw: string, decimal: string, dotIsDecimal = false): number | null {
  if (decimal !== ',' && decimal !== '.') throw new Error(`Unsupported decimal separator: ${decimal}`);
  const cleaned = cleanNumber(raw);
  if (cleaned === null) return null;
  const { text, sign } = cleaned;
  let normalized: string;
  if (decimal === ',') {
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
  } else {
    const comma = text.lastIndexOf(',');
    const dot = text.lastIndexOf('.');
    if (comma !== -1 && dot !== -1) {
      if (comma > dot) return null;
      const intPart = text.slice(0, dot);
      if (!THOUSANDS_COMMA.test(intPart) || !/^\d+$/.test(text.slice(dot + 1))) return null;
      normalized = `${intPart.replace(/,/g, '')}.${text.slice(dot + 1)}`;
    } else if (comma !== -1) {
      if (!THOUSANDS_COMMA.test(text)) return null;
      normalized = text.replace(/,/g, '');
    } else if (dot !== -1) {
      if (!/^\d*\.\d+$/.test(text)) return null;
      normalized = text;
    } else {
      normalized = text;
    }
  }
  const value = sign * Number(normalized);
  return Number.isFinite(value) ? value : null;
}

/** Formats with the locale separator and grouping, deterministic (no ICU). */
export function formatNumber(value: number, decimals: number, decimal: string): string {
  if (decimal !== ',' && decimal !== '.') throw new Error(`Unsupported decimal separator: ${decimal}`);
  const fixed = Math.abs(value).toFixed(decimals);
  const [intPart, fraction] = fixed.split('.');
  const group = decimal === ',' ? '.' : ',';
  const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, group);
  const sign = value < 0 && Number(fixed) !== 0 ? '-' : '';
  const separator = fraction === undefined ? '' : decimal;
  return fraction === undefined ? `${sign}${grouped}` : `${sign}${grouped}${separator}${fraction}`;
}

export function parseDecimalBR(raw: string, dotIsDecimal = false): number | null {
  return parseDecimal(raw, ',', dotIsDecimal);
}

export function formatNumberBR(value: number, decimals: number): string {
  return formatNumber(value, decimals, ',');
}

export function formatBRL(value: number, decimals = 2, decimal = ','): string {
  return `R$ ${formatNumber(value, decimals, decimal)}`;
}

export function formatPercent(ratio: number, decimal = ','): string {
  return `${formatNumber(ratio * 100, 1, decimal)}%`;
}

// ---------------------------------------------------------------------------
// Validation.

export function isTruthyFlag(raw: string): boolean {
  return ['on', '1', 'true', 'sim'].includes(raw.trim().toLowerCase());
}

export function validateField(rule: FieldRule, raw: string, locale: ToolLocale | null = null): FieldResult {
  const decimal = locale?.decimal ?? ',';
  const validate = (key: string, vars?: Record<string, string | number>): string => fillMessage(locale?.validation?.[key] ?? PT_BR_VALIDATION[key], vars);
  const label = locale?.fields?.[rule.name]?.label ?? rule.label;
  const text = raw.trim();
  if (rule.kind === 'flag') {
    return { ok: true, value: isTruthyFlag(text) };
  }
  // Messages render right under their field, prefixed by the field name.
  const fail = (message: string): FieldResult => ({ ok: false, error: validate('field', { label, message }) });
  if (text === '') {
    return rule.required ? fail(validate('required')) : { ok: true, value: null };
  }
  if (rule.kind === 'choice') {
    if (!rule.choices?.includes(text)) return fail(validate('choice'));
    return { ok: true, value: Number(text) };
  }
  const value = parseDecimal(text, decimal, rule.kind === 'price');
  const exampleValue = locale?.fields?.[rule.name]?.example ?? rule.example;
  const example = exampleValue ? validate('example', { example: exampleValue }) : '';
  if (value === null) return fail(validate('number', { example }));
  if (value < 0) return fail(validate('negative'));
  if (value === 0 && !rule.allowZero) return fail(validate('zero'));
  if (rule.kind === 'integer' && !Number.isInteger(value)) return fail(validate('integer'));
  if (rule.max !== undefined && value > rule.max) {
    const unit = rule.unit ? validate('unit', { unit: locale?.fields?.[rule.name]?.unit ?? rule.unit }) : '';
    return fail(validate('max', { max: formatNumber(rule.max, 0, decimal), unit }));
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

function toolContext(id: string, locale: ToolLocale | null): ToolContext {
  const decimal = locale?.decimal ?? ',';
  return {
    decimal,
    t: (key, vars) => fillMessage(locale?.messages?.[key] ?? PT_BR_MESSAGES[id]?.[key], vars),
    brl: (value, decimals = 2) => formatBRL(value, decimals, decimal),
    number: (value, decimals) => formatNumber(value, decimals, decimal),
    percent: (ratio) => `${formatNumber(ratio * 100, 1, decimal)}%`,
  };
}

export const TOOL_SPECS: Record<string, ToolSpec> = {
  'etanol-ou-gasolina': {
    id: 'etanol-ou-gasolina',
    fields: [
      { name: 'gasolina', label: 'Preço da gasolina', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '6,29' },
      { name: 'etanol', label: 'Preço do etanol', kind: 'price', required: true, max: PRICE_MAX, unit: 'reais por litro', example: '4,19' },
      { name: 'consumo_gasolina', label: 'Consumo com gasolina', kind: 'decimal', required: false, max: KML_MAX, unit: 'km/l', example: '12,5' },
      { name: 'consumo_etanol', label: 'Consumo com etanol', kind: 'decimal', required: false, max: KML_MAX, unit: 'km/l', example: '8,7' },
    ],
    compute(v, ctx) {
      const kmlG = opt(v.consumo_gasolina);
      const kmlE = opt(v.consumo_etanol);
      if ((kmlG === null) !== (kmlE === null)) {
        const missing = kmlG === null ? 'consumo_gasolina' : 'consumo_etanol';
        return { ok: false, errors: { [missing]: ctx.t('missingConsumption') } };
      }
      const r = compareEthanolGasoline({ gasoline: num(v.gasolina), ethanol: num(v.etanol), kmPerLiterGasoline: kmlG, kmPerLiterEthanol: kmlE });
      const verdict = r.choice === 'etanol'
        ? ctx.t('ethanolCheaper')
        : r.choice === 'gasolina'
          ? ctx.t('gasolineCheaper')
          : ctx.t('tie');
      const lines = [
        verdict,
        ctx.t('ratio', { percent: ctx.percent(r.ratio) }),
        r.thresholdSource === 'consumo'
          ? ctx.t('thresholdOwn', { percent: ctx.percent(r.threshold) })
          : ctx.t('thresholdRule'),
        ctx.t('maxPrice', { gasoline: ctx.brl(num(v.gasolina), 3), ethanol: ctx.brl(r.maxEthanolPrice, 3) }),
      ];
      if (r.costPerKmEthanol !== null && r.costPerKmGasoline !== null) {
        lines.push(ctx.t('costPerKm', { ethanol: ctx.brl(r.costPerKmEthanol, 3), gasoline: ctx.brl(r.costPerKmGasoline, 3) }));
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
    compute(v, ctx) {
      const r = tripCost({
        distanceKm: num(v.distancia),
        kmPerLiter: num(v.consumo),
        pricePerLiter: num(v.preco),
        roundTrip: v.ida_e_volta === true,
        tolls: opt(v.pedagio),
        people: opt(v.pessoas),
      });
      const lines = [
        ctx.t('total', { total: ctx.brl(r.total) }),
        ctx.t('distance', { distance: ctx.number(r.totalKm, 0), trip: v.ida_e_volta === true ? ctx.t('roundTrip') : '' }),
        ctx.t('fuel', { liters: ctx.number(r.liters, 1), cost: ctx.brl(r.fuelCost) }),
      ];
      if (opt(v.pedagio) !== null) lines.push(ctx.t('tolls', { tolls: ctx.brl(num(v.pedagio)) }));
      if (r.people > 1) lines.push(ctx.t('perPerson', { people: r.people, amount: ctx.brl(r.perPerson) }));
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
    compute(v, ctx) {
      const r = averageConsumption({ distanceKm: num(v.distancia), liters: num(v.litros), pricePerLiter: opt(v.preco) });
      const lines = [
        ctx.t('average', { value: ctx.number(r.kmPerLiter, 1) }),
        ctx.t('per100', { value: ctx.number(r.litersPer100Km, 1) }),
      ];
      if (r.costPerKm !== null && r.spent !== null) {
        lines.push(ctx.t('costPerKm', { cost: ctx.brl(r.costPerKm, 3), spent: ctx.brl(r.spent) }));
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
    compute(v, ctx) {
      const r = costPerKm({
        fuelPrice: num(v.preco_combustivel),
        kmPerLiter: num(v.consumo),
        kwhPrice: num(v.tarifa_kwh),
        kmPerKwh: num(v.consumo_eletrico),
        monthlyKm: opt(v.km_mes),
      });
      const verdict = r.cheaper === 'eletrico'
        ? ctx.t('electricCheaper', { difference: ctx.brl(r.difference, 3) })
        : r.cheaper === 'combustao'
          ? ctx.t('combustionCheaper', { difference: ctx.brl(r.difference, 3) })
          : ctx.t('tie');
      const lines = [
        verdict,
        ctx.t('combustion', { cost: ctx.brl(r.combustion, 3) }),
        ctx.t('electric', { cost: ctx.brl(r.electric, 3) }),
      ];
      if (r.monthlyCombustion !== null && r.monthlyElectric !== null) {
        lines.push(ctx.t('monthly', { km: ctx.number(num(v.km_mes), 0), combustion: ctx.brl(r.monthlyCombustion), electric: ctx.brl(r.monthlyElectric) }));
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
    compute(v, ctx) {
      const r = fillTank({ capacityLiters: num(v.capacidade), currentPercent: num(v.nivel), pricePerLiter: num(v.preco), kmPerLiter: opt(v.consumo) });
      const lines = [
        ctx.t('fill', { liters: ctx.number(r.liters, 1), cost: ctx.brl(r.cost) }),
        ctx.t('full', { cost: ctx.brl(r.fullTankCost) }),
      ];
      if (r.rangeKm !== null) lines.push(ctx.t('range', { range: ctx.number(r.rangeKm, 0) }));
      return { ok: true, lines };
    },
  },
};

/** Validates raw form strings and runs the tool. Pure and DOM-free. */
export function runTool(id: string, raw: Record<string, string>, locale: ToolLocale | null = null): ToolOutcome {
  const spec = TOOL_SPECS[id];
  if (!spec) return { ok: false, errors: { _: fillMessage(locale?.validation?.unknown ?? PT_BR_VALIDATION.unknown, { id }) } };
  const values: Record<string, number | boolean | null> = {};
  const errors: Record<string, string> = {};
  for (const rule of spec.fields) {
    const result = validateField(rule, raw[rule.name] ?? '', locale);
    if (result.ok) values[rule.name] = result.value;
    else errors[rule.name] = result.error;
  }
  if (Object.keys(errors).length > 0) return { ok: false, errors };
  return spec.compute(values, toolContext(id, locale));
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

export function renderOutcome(form: HTMLFormElement, result: HTMLElement, spec: ToolSpec, moveFocus: boolean, locale: ToolLocale | null = null): ToolOutcome {
  const raw: Record<string, string> = {};
  const elements = new Map<string, FieldElement>();
  for (const rule of spec.fields) {
    const el = form.elements.namedItem(rule.name) as FieldElement | null;
    if (el) {
      elements.set(rule.name, el);
      raw[rule.name] = fieldValue(el);
    }
  }
  const outcome = runTool(spec.id, raw, locale);
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
    p.textContent = fillMessage(locale?.validation?.review ?? PT_BR_VALIDATION.review);
    nodes.push(p);
  }
  result.replaceChildren(...nodes);
  result.hidden = false;
  if (moveFocus && firstInvalid?.focus) firstInvalid.focus();
  return outcome;
}

/** Reads the page's tool-i18n JSON block, or null on pt-BR pages. */
export function readToolI18n(doc: Document): ToolLocale | null {
  if (!doc || typeof doc.getElementById !== 'function') return null;
  const element = doc.getElementById('tool-i18n');
  if (!element || !element.textContent) return null;
  try {
    const data = JSON.parse(element.textContent) as ToolLocale;
    if (data.decimal !== ',' && data.decimal !== '.') return null;
    return data;
  } catch {
    return null;
  }
}

export function setupTools(doc: Document, search: string): void {
  const locale = readToolI18n(doc);
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
      renderOutcome(form, result, spec, true, locale);
    });
    if (Object.keys(prefill).length > 0) renderOutcome(form, result, spec, false, locale);
  }
}

if (typeof document !== 'undefined' && typeof location !== 'undefined') {
  const start = (): void => setupTools(document, location.search);
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
}
