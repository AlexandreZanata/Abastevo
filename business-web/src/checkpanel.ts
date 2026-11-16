import { type Authority } from './model.js';
import { type CheckSource, type CheckState, type ReportAction, type ReportSummary, type Run, CheckPanelController, reportStateLabel, runOutcomeLabel } from './checks.js';

// EMP-CHECK-004 DOM for SCR-CHECK-04/05 (EDR-330). Every label is fixed pt-BR
// copy over the server's own vocabulary; unknown values are never echoed as
// status. The asset block shown is the availability read, never inferred.
const AVAILABILITY: Record<string, string> = { AVAILABLE: 'Disponível', RESERVED: 'Reservado', IN_USE: 'Em uso', IN_MAINTENANCE: 'Em manutenção', BLOCKED: 'Bloqueado', UNAVAILABLE: 'Indisponível' };
const CAUSES: Record<string, string> = { LIFECYCLE: 'Situação do cadastro do ativo', ACTIVE_USE: 'Viagem em andamento', MAINTENANCE_HOLD: 'Manutenção', BLOCK: 'Bloqueio manual', RESERVATION: 'Reserva', CHECKLIST_RUN: 'Checklist bloqueante' };
const MISSING: Record<string, string> = { ACTIVE_USE: 'viagens em andamento', MAINTENANCE_HOLD: 'manutenções', CHECKLIST_RUN: 'checklists bloqueantes' };
const BLOCKS: Record<string, string> = { ADMINISTRATIVE: 'Administrativo', SAFETY: 'Segurança', DOCUMENT: 'Documentação', INVESTIGATION: 'Apuração', LENT_OUT: 'Emprestado' };
const CONTEXTS: Record<string, string> = { DEPARTURE: 'Saída', RETURN: 'Retorno', AD_HOC: 'Avulso' };
const KINDS: Record<string, string> = { BREAKDOWN: 'Pane', ACCIDENT: 'Acidente', THEFT: 'Furto ou roubo', FINE_NOTICE: 'Notificação de multa', OTHER: 'Outra' };
const STORAGE: Record<string, string> = { UPLOADING: 'Envio em andamento', RECEIVED: 'Recebida, em verificação', STORED: 'Armazenada', QUARANTINED: 'Retida para verificação', FAILED: 'Envio falhou', REPLACED: 'Substituída', DELETED: 'Excluída' };
const ACTIONS: Record<ReportAction, string> = { ACKNOWLEDGE: 'Ciente (sem texto)', TREAT: 'Registrar ação em andamento', RESOLVE: 'Resolver com a solução', DISMISS: 'Descartar com o motivo' };
const STEPS: Record<ReportAction, string> = { ACKNOWLEDGE: 'Ciente', TREAT: 'Em tratamento', RESOLVE: 'Resolvida', DISMISS: 'Descartada' };

export function mountCheckPanel(doc: Document, browser: Window, source: CheckSource, sessionLost?: (message: string) => void): { select(authority?: Authority): void; clear(): void } {
  const el = <T extends HTMLElement>(id: string): T => { const node = doc.getElementById(id); if (!node) throw new Error('Check panel is incomplete'); return node as T; };
  const input = (id: string): HTMLInputElement => el<HTMLInputElement>(id);
  const select = (id: string): HTMLSelectElement => el<HTMLSelectElement>(id);
  let identity = '', shown = '', focused = '', lastPhase = '';
  const controller = new CheckPanelController(source, render, sessionLost);
  const zone = (() => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return 'UTC'; } })();
  const when = (value: string): string => { try { return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)); } catch { return value; } };
  const node = <K extends keyof HTMLElementTagNameMap>(tag: K, value: string): HTMLElementTagNameMap[K] => { const item = doc.createElement(tag); item.textContent = value; return item; };
  const option = (value: string, label: string): HTMLOptionElement => { const item = node('option', label); item.value = value; return item; };
  const button = (label: string, action: () => void): HTMLButtonElement => { const item = node('button', label); item.type = 'button'; item.className = 'button-quiet'; item.addEventListener('click', action); return item; };
  function assetLabel(state: CheckState, id: string): string {
    const asset = state.assets?.find(a => a.id === id);
    return asset ? [asset.name, asset.fleetNumber, asset.plate].filter(Boolean).join(' · ') : 'Veículo (fora da sua lista de veículos)';
  }
  const items = (values: number[]): string => values.length === 0 ? 'Nenhum' : values.map(v => `item ${v}`).join(', ');
  function runLine(state: CheckState, r: Run): string {
    return [assetLabel(state, r.assetId), `${CONTEXTS[r.context] ?? 'Checklist'} concluído em ${when(r.completedAt)}`, runOutcomeLabel(r.outcome),
      r.differs ? 'Resultado recalculado pela empresa — revisão necessária' : '', r.releasedAt ? `Liberado em ${when(r.releasedAt)}` : '', `Revisão ${r.revision}`].filter(Boolean).join(' · ');
  }
  function reportLine(state: CheckState, r: ReportSummary): string {
    return [r.type === 'DAMAGE' ? 'Dano' : `Ocorrência${r.kind ? ` (${KINDS[r.kind] ?? 'outra'})` : ''}`, assetLabel(state, r.assetId), `ocorrido em ${when(r.occurredAt)}`, reportStateLabel(r.state),
      r.blocksUse ? 'Marcado pelo relator como impeditivo' : '', `Revisão ${r.revision}`].filter(Boolean).join(' · ');
  }
  function runFields(state: CheckState, r: Run): [string, string][] {
    return [['Veículo', assetLabel(state, r.assetId)], ['Tipo', CONTEXTS[r.context] ?? 'Checklist'], ['Resultado atual', runOutcomeLabel(r.outcome)],
      ['Resultado no aparelho', runOutcomeLabel(r.device)], ['Resultado conferido pelo servidor', runOutcomeLabel(r.recheck) + (r.differs ? ' — difere do aparelho; revisão necessária' : '')],
      ['Itens com problema', items(r.issues)], ['Itens críticos com problema', items(r.critical)], ['Pendências confirmadas por quem respondeu', r.acknowledged ? 'Sim' : 'Não'],
      ['Modelo', `Versão ${r.version}${r.superseded ? ' (substituída por versão mais nova)' : ''} · Regra ${r.policyRevision} no aparelho, ${r.serverPolicyRevision} no servidor`],
      ['Início', when(r.startedAt)], ['Conclusão declarada', when(r.completedAt)], ['Recebido pelo servidor', when(r.receivedAt)], ['Revisão', String(r.revision)],
      ['Liberação', r.releasedAt ? `Liberado em ${when(r.releasedAt)}; o histórico do checklist foi mantido` : r.outcome === 'BLOCKING' ? 'Não liberado' : 'Não se aplica']];
  }
  function reportFields(state: CheckState): [string, string][] {
    const r = state.report!;
    return [['Tipo', r.type === 'DAMAGE' ? 'Dano' : `Ocorrência${r.kind ? ` — ${KINDS[r.kind] ?? 'outra'}` : ''}`], ['Veículo', assetLabel(state, r.assetId)], ['Situação', reportStateLabel(r.state)], ['Revisão', String(r.revision)],
      ['Impede o uso?', r.blocksUse ? 'Sim, segundo o relator. O bloqueio do veículo por ocorrência ainda não é aplicado pelo servidor; veja a disponibilidade abaixo.' : 'Não, segundo o relator'],
      ['Local informado', r.location || 'Não informado'], ['Viagem', r.tripId ? 'Registrada durante uma viagem' : 'Fora de viagem'], ['Ocorrido em', when(r.occurredAt)], ['Recebido em', when(r.receivedAt)], ['Atualizado em', when(r.updatedAt)]];
  }
  function block(state: CheckState): void {
    const value = state.availability, run = state.run;
    el('checks-causes').replaceChildren();
    if (!value) {
      el('checks-availability').textContent = 'Disponibilidade não confirmada pelo servidor. Não considere o veículo liberado.';
      el('checks-availability-note').textContent = '';
      return;
    }
    el('checks-availability').textContent = `Disponibilidade agora: ${AVAILABILITY[value.availability] ?? 'Não confirmada'} (avaliada pelo servidor em ${when(value.evaluatedAt)}).`;
    if (value.causes.length === 0) el('checks-causes').append(node('li', 'Nenhum motivo de restrição informado pelo servidor.'));
    for (const c of value.causes) {
      const mine = c.kind === 'CHECKLIST_RUN' ? (run && c.recordId === run.id ? ' — este checklist' : ' — outro checklist') : '';
      el('checks-causes').append(node('li', `${CAUSES[c.kind] ?? 'Outro motivo'}${mine}: ${AVAILABILITY[c.availability] ?? 'Não confirmada'}${c.from ? ` · desde ${when(c.from)}` : ''}${c.to ? ` até ${when(c.to)}` : ''}${c.reasonKind ? ` · ${BLOCKS[c.reasonKind] ?? 'Bloqueio'}` : ''}`));
    }
    const missing = value.missing.map(s => MISSING[s] ?? 'outra fonte');
    el('checks-availability-note').textContent = missing.length ? `Fontes ainda não consultadas pelo servidor: ${missing.join(', ')}.` : '';
  }
  function report(state: CheckState): void {
    const r = state.report;
    el('checks-report-body').hidden = r === null;
    for (const id of ['checks-history', 'checks-attachments']) el(id).replaceChildren();
    if (!r) { el('checks-description').textContent = ''; el('checks-content-help').textContent = ''; return; }
    el('checks-description').textContent = r.description;
    if (r.history.length === 0) el('checks-history').append(node('li', 'Nenhum tratamento registrado.'));
    for (const h of r.history) {
      const original = h.duplicateOf ? state.reports.find(o => o.id === h.duplicateOf) : undefined;
      el('checks-history').append(node('li', [`Revisão ${h.revision}`, STEPS[h.action], `em ${when(h.recordedAt)}`, h.note, h.duplicateOf ? `duplicata de ${original ? reportLine(state, original) : 'outra ocorrência do mesmo veículo'}` : ''].filter(Boolean).join(' · ')));
    }
    if (r.attachments.length === 0) el('checks-attachments').append(node('li', 'Nenhuma foto ou documento vinculado.'));
    r.attachments.forEach((a, i) => {
      const item = node('li', ''), stateText = STORAGE[a.storage] ?? 'Situação registrada';
      item.append(node('p', `Foto ${i + 1} anexada · ${stateText} · vinculada em ${when(a.linkedAt)}`));
      // B-BR-EMP-507: the content action exists only with its own permission.
      if (controller.canOpenContent(a.documentId)) item.append(button(`Baixar foto ${i + 1} (o acesso fica registrado)`, () => { void download(a.documentId, i + 1); }));
      el('checks-attachments').append(item);
    });
    el('checks-content-help').textContent = state.contentAllowed ? 'Baixar uma foto abre o conteúdo pelo caminho auditado da empresa; cada abertura fica registrada.'
      : 'Abrir o conteúdo das fotos exige a permissão de conteúdo de documentos. Aqui aparece apenas que a foto existe e a situação do envio.';
  }
  async function download(document: string, position: number): Promise<void> {
    const file = await controller.content(document);
    if (!file) return;
    // Handed to the browser as a saved file; nothing is kept in the page.
    const url = URL.createObjectURL(new Blob([file.bytes], { type: file.mediaType }));
    const link = doc.createElement('a');
    link.href = url; link.download = `foto-${position}.jpg`; link.hidden = true;
    doc.body.append(link); link.click(); link.remove();
    browser.setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }
  function render(state: CheckState): void {
    const root = el('checks-panel'), busy = state.phase === 'loading', run = state.run, rep = state.report, open = run ?? rep;
    root.hidden = state.phase === 'closed';
    root.setAttribute('aria-busy', String(busy));
    el('checks-status').textContent = busy ? 'Consultando o servidor…' : state.message;
    el('checks-retry').hidden = state.phase !== 'failed' || !controller.retryable;
    el('checks-load').textContent = state.runsAt ? 'Atualizar checklists e ocorrências' : 'Consultar checklists e ocorrências';
    const listed = (at: string): string => at ? `Lista consultada em ${when(at)}. Horários no fuso deste navegador (${zone}).` : '';
    el('checks-runs-at').textContent = listed(state.runsAt);
    el('checks-reports-at').textContent = listed(state.reportsAt);
    const runs = el('checks-runs'); runs.replaceChildren();
    if (state.runsAt && state.runs.length === 0) runs.append(node('li', 'Nenhum checklist no período.'));
    for (const r of state.runs) {
      const item = node('li', '');
      if (run?.id === r.id) item.setAttribute('aria-current', 'true');
      item.append(node('p', runLine(state, r)), button(`Abrir checklist ${CONTEXTS[r.context]?.toLowerCase() ?? ''} de ${when(r.completedAt)}`, () => { void controller.openRun(r.id); }));
      runs.append(item);
    }
    el('checks-runs-next').hidden = state.runCursor === '';
    const reports = el('checks-reports'); reports.replaceChildren();
    if (state.reportsAt && state.reports.length === 0) reports.append(node('li', 'Nenhuma ocorrência aguardando tratamento.'));
    for (const r of state.reports) {
      const item = node('li', '');
      if (rep?.id === r.id) item.setAttribute('aria-current', 'true');
      item.append(node('p', reportLine(state, r)), button(`Abrir ${r.type === 'DAMAGE' ? 'dano' : 'ocorrência'} de ${when(r.occurredAt)}`, () => { void controller.openReport(r.id); }));
      reports.append(item);
    }
    el('checks-reports-next').hidden = state.reportCursor === '';
    el('checks-detail').hidden = open === null;
    const fields = el('checks-fields'); fields.replaceChildren();
    if (open) {
      el('checks-detail-title').textContent = run ? `Checklist — ${assetLabel(state, run.assetId)}` : `${rep!.type === 'DAMAGE' ? 'Dano' : 'Ocorrência'} — ${assetLabel(state, rep!.assetId)}`;
      for (const [name, value] of run ? runFields(state, run) : reportFields(state)) fields.append(node('dt', name), node('dd', value));
      el('checks-read-at').textContent = `Situação consultada no servidor em ${when(state.readAt)}.${state.stale ? ' Esta situação pode estar desatualizada; atualize o registro.' : ''}${state.conflict ? ' Uma alteração concorrente foi encontrada; estes são os dados atuais.' : ''}`;
      const key = (run ? 'run:' : 'report:') + open.id + ':' + open.revision;
      if (key !== shown) { shown = key; input('release-reason').value = ''; input('decide-note').value = ''; }
      el('checks-release-form').hidden = !run || run.outcome !== 'BLOCKING';
      const actions = controller.actions();
      el('checks-decide-form').hidden = !rep || actions.length === 0;
      const action = select('decide-action'), chosen = action.value; action.replaceChildren();
      for (const a of actions) action.append(option(a, ACTIONS[a]));
      action.value = actions.includes(chosen as ReportAction) ? chosen : actions[0] ?? '';
      const dup = select('decide-duplicate'), original = dup.value; dup.replaceChildren(option('', 'Não é duplicata'));
      for (const o of controller.duplicates()) dup.append(option(o.id, reportLine(state, o)));
      dup.value = controller.duplicates().some(o => o.id === original) ? original : '';
      // Focus moves once per opened record, after its reads settle.
      if (!busy && open.id !== focused) { focused = open.id; el('checks-detail-title').focus({ preventScroll: true }); }
    } else { shown = ''; focused = ''; }
    block(state);
    report(state);
    for (const control of root.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLSelectElement>('button,input,select')) control.disabled = busy || state.authority === null;
    const action = select('decide-action').value;
    select('decide-duplicate').disabled ||= action !== 'DISMISS';
    input('decide-note').disabled ||= action === 'ACKNOWLEDGE';
    if (state.phase === 'denied' && lastPhase !== 'denied') el('checks-title').focus({ preventScroll: true });
    lastPhase = state.phase;
  }
  el('checks-load').addEventListener('click', () => { void controller.load(); });
  el('checks-runs-next').addEventListener('click', () => { void controller.moreRuns(); });
  el('checks-reports-next').addEventListener('click', () => { void controller.moreReports(); });
  el('checks-retry').addEventListener('click', () => { void controller.retry(); });
  el('checks-reload').addEventListener('click', () => { void controller.reload(); });
  el('decide-action').addEventListener('change', () => render(controller.state));
  el('checks-release-form').addEventListener('submit', event => {
    event.preventDefault();
    const run = controller.state.run;
    if (!run || !el<HTMLFormElement>('checks-release-form').reportValidity()) return;
    if (browser.confirm('Liberar o veículo? A liberação fica no histórico com o motivo e o checklist continua registrado como estava.')) void controller.release(input('release-reason').value);
  });
  el('checks-decide-form').addEventListener('submit', event => {
    event.preventDefault();
    const action = select('decide-action').value as ReportAction;
    input('decide-note').required = action !== 'ACKNOWLEDGE';
    if (!action || !el<HTMLFormElement>('checks-decide-form').reportValidity()) return;
    void controller.decide(action, action === 'ACKNOWLEDGE' ? '' : input('decide-note').value, action === 'DISMISS' ? select('decide-duplicate').value : '');
  });
  function clear(): void {
    identity = ''; shown = ''; focused = ''; controller.clear();
    for (const id of ['checks-release-form', 'checks-decide-form']) el<HTMLFormElement>(id).reset();
  }
  return {
    clear,
    select(authority?: Authority): void {
      if (!authority) { clear(); return; }
      const next = JSON.stringify(authority);
      if (identity === next) return;
      clear(); identity = next; controller.open(authority);
    },
  };
}
