import { ApiError } from './api.js';
import { DATA_KEY_STATES, type DataApi, type DataKey, type DataUsage, type KeyChange } from './data.js';
import type { Authority } from './model.js';

export function mountDataPanel(doc: Document, browser: Window, api: DataApi, sessionLost: (message: string) => void) {
  function el<T extends HTMLElement = HTMLElement>(id: string): T {
    const node = doc.getElementById(id);
    if (!node) throw new Error('Company Data panel is incomplete');
    return node as T;
  }
  const panel = el('data-panel'), status = el('data-status'), usageNode = el('data-usage'), docs = el('data-docs');
  const refresh = el<HTMLButtonElement>('data-refresh'), moreDatasets = el<HTMLButtonElement>('data-more-datasets');
  const datasetList = el('data-datasets'), keySection = el('data-key-section'), keyList = el('data-keys');
  const moreKeys = el<HTMLButtonElement>('data-more-keys'), form = el<HTMLFormElement>('data-key-form');
  const label = el<HTMLInputElement>('data-key-label'), expiry = el<HTMLInputElement>('data-key-expiry');
  const overlap = el<HTMLInputElement>('data-key-overlap'), overlapField = el('data-key-overlap-field');
  const save = el<HTMLButtonElement>('data-key-save'), cancel = el<HTMLButtonElement>('data-key-cancel');
  const secretBox = el('data-secret-box'), secret = el<HTMLInputElement>('data-secret');
  const localDate = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' });
  const number = new Intl.NumberFormat('pt-BR');
  let authority: Authority | undefined;
  let pending: AbortController | undefined;
  let retry: { signature: string; operation: string } | undefined;
  let target: DataKey | undefined;
  let usage: DataUsage | undefined;
  let keys: DataKey[] = [];
  let keyCursor = '';
  let generation = 0;
  const canUse = () => authority?.permissions?.includes('data.usage.read') === true;
  const canManage = () => authority?.permissions?.includes('data.key.manage') === true;

  function add(parent: HTMLElement, tag: 'p' | 'li' | 'h4' | 'code', value: string): HTMLElement {
    const child = doc.createElement(tag);
    child.textContent = value;
    parent.appendChild(child);
    return child;
  }
  function hideSecret(): void { secret.value = ''; secretBox.hidden = true; }
  function resetForm(): void { target = undefined; form.reset(); }
  function makeButton(parent: HTMLElement, label: string, action: () => void): void {
    const button = doc.createElement('button');
    button.className = 'button-secondary'; button.type = 'button'; button.textContent = label;
    button.disabled = pending !== undefined;
    button.addEventListener('click', action); parent.appendChild(button);
  }
  function render(message: string): void {
    panel.hidden = !authority;
    panel.setAttribute('aria-busy', String(pending !== undefined));
    refresh.disabled = pending !== undefined;
    status.textContent = message;
    usageNode.hidden = !canUse();
    usageNode.replaceChildren();
    if (usage?.term) {
      add(usageNode, 'p', `${number.format(BigInt(usage.term.used))} de ${number.format(BigInt(usage.term.ceiling))} chamadas no período até ${localDate.format(new Date(usage.term.end))}.`);
      if (usage.rate) add(usageNode, 'p', `Limite por chave e suas substitutas: ${number.format(BigInt(usage.rate.calls))} chamadas a cada ${number.format(usage.rate.seconds)} s.`);
      if (!usage.enabled) add(usageNode, 'p', 'O acesso à API está suspenso.');
      else if (!usage.rate) add(usageNode, 'p', 'O limite de requisições da API aguarda configuração.');
    } else if (usage) add(usageNode, 'p', 'Nenhuma franquia de API vigente.');
    docs.hidden = !usage?.datasets.length;
    datasetList.replaceChildren();
    for (const dataset of usage?.datasets ?? []) {
      const item = add(datasetList, 'li', '');
      add(item, 'h4', `${dataset.name} · ${dataset.region}`);
      add(item, 'p', `Método ${dataset.method} · mínimo de ${dataset.minimum} postos por amostra`);
      add(item, 'code', `${dataset.key} · revisão ${dataset.revision}`);
    }
    moreDatasets.hidden = !usage?.cursor;
    moreDatasets.disabled = pending !== undefined;
    keySection.hidden = !canManage();
    keyList.replaceChildren();
    for (const key of keys) {
      const item = add(keyList, 'li', '');
      add(item, 'h4', key.label);
      add(item, 'p', `${DATA_KEY_STATES[key.state]} · expira em ${localDate.format(new Date(key.expiresAt))}`);
      if (key.state === 'ROTATING' && key.overlapUntil) add(item, 'p', `Válida até ${localDate.format(new Date(key.overlapUntil))} durante a rotação.`);
      if (key.state === 'ACTIVE' && usage?.enabled) makeButton(item, 'Rotacionar', () => {
        hideSecret(); resetForm(); target = key; label.value = key.label;
        render('Informe o vencimento da chave substituta.'); expiry.focus();
      });
      if (key.state !== 'REVOKED') makeButton(item, 'Revogar', () => {
        if (browser.confirm(`Revogar a chave “${key.label}”?`)) void change({ kind: 'revoke', key });
      });
    }
    moreKeys.hidden = !canManage() || !keyCursor;
    moreKeys.disabled = pending !== undefined;
    form.hidden = !canManage() || !usage?.enabled;
    overlapField.hidden = !target;
    cancel.hidden = !target;
    save.textContent = target ? 'Gerar chave substituta' : 'Emitir chave';
    save.disabled = pending !== undefined;
    label.disabled = expiry.disabled = overlap.disabled = pending !== undefined;
  }
  function clear(): void {
    generation++;
    pending?.abort();
    pending = undefined; authority = undefined; retry = undefined; usage = undefined;
    keys = []; keyCursor = ''; resetForm(); hideSecret(); render('');
  }
  function refusal(error: unknown): string {
    if (error instanceof ApiError && ['unauthenticated', 'fresh'].includes(error.kind)) {
      clear(); sessionLost('Entre novamente para confirmar o acesso às chaves Data.'); return '';
    }
    if (error instanceof ApiError && error.kind === 'conflict') return 'A chave mudou. Atualize a lista e tente novamente.';
    if (error instanceof ApiError && error.kind === 'invalid') return 'Confira o nome, o vencimento e o período de transição.';
    if (error instanceof ApiError && error.kind === 'denied') return 'A operação não está disponível para seu acesso atual.';
    return 'Não foi possível concluir a solicitação. Tente novamente.';
  }
  async function load(mode: 'all' | 'keys' | 'datasets', quiet = false): Promise<void> {
    const org = authority?.organization.id;
    if (!org || pending) return;
    const read = new AbortController();
    pending = read;
    if (!quiet) hideSecret();
    const loadingMessage = quiet ? status.textContent ?? '' : 'Consultando o acesso Data…';
    render(loadingMessage);
    try {
      const [newUsage, newKeys] = await Promise.all([
        canUse() && mode !== 'keys' ? api.usage(org, mode === 'datasets' ? usage?.cursor ?? '' : '', read.signal) : undefined,
        canManage() && mode !== 'datasets' ? api.keys(org, mode === 'keys' ? keyCursor : '', read.signal) : undefined,
      ]);
      if (pending !== read) return;
      if (newUsage) usage = mode === 'datasets' ? { ...newUsage, datasets: [...(usage?.datasets ?? []), ...newUsage.datasets] } : newUsage;
      if (newKeys) {
        keys = mode === 'keys' ? [...keys, ...newKeys.keys] : newKeys.keys;
        keyCursor = newKeys.cursor;
      }
      pending = undefined; render(quiet ? loadingMessage : '');
    } catch (error) {
      if (pending !== read) return;
      pending = undefined; usage = undefined; keys = []; keyCursor = ''; hideSecret(); resetForm();
      render(refusal(error));
    }
  }
  async function change(value: KeyChange): Promise<void> {
    const org = authority?.organization.id;
    if (!org || pending || !canManage()) return;
    hideSecret();
    const signature = JSON.stringify(value);
    const operation = retry?.signature === signature ? retry.operation : browser.crypto.randomUUID();
    retry = { signature, operation };
    const activeGeneration = generation;
    const request = new AbortController();
    pending = request;
    render('Salvando a alteração…');
    try {
      const result = await api.change(org, operation, value, request.signal);
      if (pending !== request) return;
      retry = undefined; pending = undefined; resetForm();
      const message = result.bearer ? 'Chave criada. Copie e guarde o código agora.' : result.replayed ? 'Alteração já registrada.' : 'Chave revogada.';
      if (authority?.organization.id !== org || generation !== activeGeneration) return;
      render(message);
      if (result.bearer) {
        secret.value = result.bearer; secretBox.hidden = false; secret.focus(); secret.select();
      }
      void load('all', true);
    } catch (error) {
      if (pending !== request) return;
      pending = undefined; render(refusal(error));
    }
  }
  function asInstant(input: HTMLInputElement): string | undefined {
    if (!input.value) return undefined;
    const value = new Date(input.value);
    return Number.isFinite(value.valueOf()) ? value.toISOString() : undefined;
  }
  form.addEventListener('submit', event => {
    event.preventDefault();
    const expiresAt = asInstant(expiry);
    const inputLabel = label.value.trim();
    const overlapUntil = asInstant(overlap);
    if (!expiresAt || !inputLabel || (overlap.value && !overlapUntil)) {
      render('Confira o nome da chave e as datas.'); return;
    }
    const terms = { label: inputLabel, expiresAt };
    void change(target ? { kind: 'rotate', key: target, terms, overlapUntil } : { kind: 'issue', terms });
  });
  cancel.addEventListener('click', () => { resetForm(); render(''); });
  el('data-hide-secret').addEventListener('click', hideSecret);
  refresh.addEventListener('click', () => { void load('all'); });
  moreKeys.addEventListener('click', () => { void load('keys'); });
  moreDatasets.addEventListener('click', () => { void load('datasets'); });
  return {
    clear,
    select(next?: Authority): void {
      clear();
      if (!next?.permissions?.some(p => p === 'data.usage.read' || p === 'data.key.manage')) return;
      authority = next;
      void load('all');
    },
  };
}
