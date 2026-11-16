import { type Authority } from './model.js';
import { type BoardState, type Mission, type MissionSource, type MissionState, type LoadKind, MissionBoardController, missionStateLabel } from './missions.js';

// EMP-TRIP-022 DOM for the mission board (SCR-TRIP-03) and the vehicle agenda
// list view (SCR-FLEET-06, EDR-306). Every label is fixed pt-BR copy over the
// server's own vocabulary; unknown values are never echoed as status.
const AVAILABILITY: Record<string, string> = { AVAILABLE: 'Disponível', RESERVED: 'Reservado', IN_USE: 'Em uso', IN_MAINTENANCE: 'Em manutenção', BLOCKED: 'Bloqueado', UNAVAILABLE: 'Indisponível' };
const CAUSES: Record<string, string> = { LIFECYCLE: 'Situação do cadastro do ativo', ACTIVE_USE: 'Viagem em andamento', MAINTENANCE_HOLD: 'Manutenção', BLOCK: 'Bloqueio', RESERVATION: 'Reserva' };
const MISSING: Record<string, string> = { ACTIVE_USE: 'viagens em andamento', MAINTENANCE_HOLD: 'manutenções' };
const RESERVATIONS: Record<string, string> = { REQUESTED: 'Solicitada', CONFIRMED: 'Confirmada', FULFILLED: 'Utilizada', CANCELLED: 'Cancelada', NO_SHOW: 'Não utilizada', EXPIRED: 'Expirada' };
const BLOCKS: Record<string, string> = { ADMINISTRATIVE: 'Administrativo', SAFETY: 'Segurança', DOCUMENT: 'Documentação', INVESTIGATION: 'Apuração', LENT_OUT: 'Emprestado' };
const WARNINGS: Record<string, string> = { ASSET_UNAVAILABLE: 'a pessoa designada não poderia iniciar com este veículo no início planejado', ASSET_BUSY: 'o veículo estará ocupado no início planejado' };
const LOADS: Record<LoadKind, string> = { NONE: 'Sem carga', CARGO: 'Carga', PASSENGERS: 'Passageiros' };
const PLAN: MissionState[] = ['DRAFT', 'PLANNED', 'ASSIGNED', 'DECLINED'];
const ASSIGN: MissionState[] = ['DRAFT', 'PLANNED', 'ASSIGNED', 'ACCEPTED', 'DECLINED'];
const VEHICLE: MissionState[] = ['PLANNED', 'ASSIGNED', 'ACCEPTED', 'DECLINED'];

export function mountMissionBoard(doc: Document, browser: Window, source: MissionSource, sessionLost?: (message: string) => void): { select(authority?: Authority): void; clear(): void } {
  const el = <T extends HTMLElement>(id: string): T => { const node = doc.getElementById(id); if (!node) throw new Error('Mission board is incomplete'); return node as T; };
  const input = (id: string): HTMLInputElement => el<HTMLInputElement>(id);
  const select = (id: string): HTMLSelectElement => el<HTMLSelectElement>(id);
  let identity = '', shown = '', focused = '', lastPhase = '';
  const controller = new MissionBoardController(source, render, sessionLost);
  const zone = (() => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return 'UTC'; } })();
  const when = (value: string): string => { try { return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)); } catch { return value; } };
  const node = <K extends keyof HTMLElementTagNameMap>(tag: K, value: string): HTMLElementTagNameMap[K] => { const item = doc.createElement(tag); item.textContent = value; return item; };
  const option = (value: string, label: string): HTMLOptionElement => { const item = node('option', label); item.value = value; return item; };
  function assetLabel(state: BoardState, id: string): string {
    if (!id) return 'Sem veículo';
    const asset = state.assets?.find(a => a.id === id);
    return asset ? [asset.name, asset.fleetNumber, asset.plate].filter(Boolean).join(' · ') : 'Veículo vinculado (fora da lista de veículos ativos)';
  }
  function personLabel(state: BoardState, id: string): string {
    if (!id) return 'Ninguém designado';
    const person = state.assignees.find(a => a.id === id);
    return person ? `${person.name || 'Cadastro sem perfil'} · Matrícula ${person.registration || 'não informada'}` : 'Pessoa designada (não está entre as pessoas elegíveis consultadas agora)';
  }
  function fields(state: BoardState, m: Mission): [string, string][] {
    const plan = m.plan, centre = plan?.costCentreId ? state.costCentres?.find(c => c.id === plan.costCentreId) : undefined;
    const rows: [string, string][] = [['Situação', missionStateLabel(m)], ['Revisão', String(m.revision)], ['Início planejado', when(m.startsAt)], ['Fim planejado', when(m.endsAt)], ['Veículo', assetLabel(state, m.assetId)], ['Pessoa designada', personLabel(state, m.assigneeId)]];
    if (m.answer === 'DECLINE') rows.push(['Motivo da recusa', m.answerReason || 'Não informado']);
    rows.push(['Destino', plan ? plan.destinationText || 'Unidade da empresa vinculada' : 'Não planejado'], ['Motivo', plan?.reason || 'Não planejado'],
      ['Centro de custo', !plan?.costCentreId ? 'Não informado' : centre ? `${centre.name} (${centre.code})` : 'Centro de custo vinculado'],
      ['Carga', !plan ? 'Não planejada' : plan.load === 'CARGO' ? `Carga · ${plan.cargo}` : plan.load === 'PASSENGERS' ? `${plan.passengers} passageiro(s)` : LOADS[plan.load]]);
    return rows;
  }
  function overlaps(m: Mission, from: string, to: string): boolean { return Date.parse(from) < Date.parse(m.endsAt) && Date.parse(to || '9999-12-31T00:00:00Z') > Date.parse(m.startsAt); }
  function agenda(state: BoardState): void {
    const value = state.agenda, mission = state.selected;
    el('mission-agenda').hidden = value === null;
    for (const id of ['agenda-causes', 'agenda-reservations', 'agenda-blocks']) el(id).replaceChildren();
    if (!value) { el('agenda-availability').textContent = ''; el('agenda-note').textContent = ''; return; }
    el('agenda-title').textContent = `Agenda do veículo — ${assetLabel(state, value.assetId)}`;
    el('agenda-availability').textContent = `Disponibilidade agora: ${AVAILABILITY[value.availability.availability] ?? 'Não confirmada'} (avaliada em ${when(value.availability.evaluatedAt)}).`;
    for (const cause of value.availability.causes) el('agenda-causes').append(node('li', `${CAUSES[cause.kind] ?? 'Outro motivo'}: ${AVAILABILITY[cause.availability] ?? 'Não confirmada'}${cause.from ? ` · de ${when(cause.from)}` : ''}${cause.to ? ` até ${when(cause.to)}` : ''}${cause.reasonKind ? ` · ${BLOCKS[cause.reasonKind] ?? 'Bloqueio'}` : ''}`));
    if (value.reservations.length === 0) el('agenda-reservations').append(node('li', 'Nenhuma reserva registrada.'));
    for (const r of value.reservations) el('agenda-reservations').append(node('li', `${RESERVATIONS[r.state] ?? 'Situação registrada'} · ${when(r.startsAt)} até ${when(r.endsAt)}${mission && overlaps(mission, r.startsAt, r.endsAt) ? ' · coincide com o horário da missão' : ''}`));
    if (value.blocks.length === 0) el('agenda-blocks').append(node('li', 'Nenhum bloqueio registrado.'));
    for (const b of value.blocks) el('agenda-blocks').append(node('li', `${BLOCKS[b.reasonKind] ?? 'Bloqueio'} · desde ${when(b.validFrom)}${b.validTo ? ` até ${when(b.validTo)}` : ' sem data de fim'}${b.removedAt ? ` · removido em ${when(b.removedAt)}` : ''}${mission && !b.removedAt && overlaps(mission, b.validFrom, b.validTo) ? ' · coincide com o horário da missão' : ''}`));
    const missing = value.availability.missing.map(s => MISSING[s] ?? 'outra fonte');
    el('agenda-note').textContent = [missing.length ? `Fontes ainda não consultadas pelo servidor: ${missing.join(', ')}.` : '', value.more ? 'Há mais registros do que os exibidos aqui.' : ''].filter(Boolean).join(' ');
  }
  function render(state: BoardState): void {
    const root = el('mission-board'), busy = state.phase === 'loading', detail = state.selected;
    root.hidden = state.phase === 'closed';
    root.setAttribute('aria-busy', String(busy));
    el('mission-status').textContent = busy ? 'Consultando o servidor…' : state.message;
    el('mission-retry').hidden = state.phase !== 'failed' || !controller.retryable;
    el('missions-load').textContent = state.listedAt ? 'Atualizar missões' : 'Consultar missões';
    el('mission-listed-at').textContent = state.listedAt ? `Lista consultada em ${when(state.listedAt)}. Horários no fuso deste navegador (${zone}).` : '';
    const list = el('mission-list'); list.replaceChildren();
    if (state.listedAt && state.missions.length === 0) list.append(node('li', 'Nenhuma missão nesta empresa. Crie uma missão para planejar a operação.'));
    for (const m of state.missions) {
      const item = node('li', ''), open = node('button', 'Abrir ' + m.title);
      open.type = 'button'; open.className = 'button-quiet'; open.addEventListener('click', () => { void controller.select(m.id); });
      if (detail?.id === m.id) item.setAttribute('aria-current', 'true');
      item.append(node('p', `${m.title} · ${when(m.startsAt)} até ${when(m.endsAt)} · ${missionStateLabel(m)} · Revisão ${m.revision} · Veículo: ${assetLabel(state, m.assetId)}`), open);
      list.append(item);
    }
    el('missions-next').hidden = state.cursor === '';
    el('mission-create').hidden = !state.listedAt || state.assets === null;
    el('mission-zone').textContent = `Informe os horários no fuso deste navegador (${zone}).`;
    const vehicles = select('mission-vehicle'), chosen = vehicles.value; vehicles.replaceChildren(option('', 'Selecione um veículo'));
    for (const a of state.assets ?? []) vehicles.append(option(a.id, assetLabel(state, a.id)));
    vehicles.value = chosen;
    el('mission-detail').hidden = detail === null;
    const values = el('mission-fields'); values.replaceChildren();
    if (detail) {
      el('mission-detail-title').textContent = detail.title;
      for (const [name, value] of fields(state, detail)) values.append(node('dt', name), node('dd', value));
      el('mission-read-at').textContent = `Situação consultada no servidor em ${when(state.readAt)}.${state.stale ? ' Esta situação pode estar desatualizada; atualize a missão.' : ''}${state.conflict ? ' Uma alteração concorrente foi encontrada; estes são os dados atuais.' : ''}`;
      el('mission-warning').hidden = state.warning === null;
      el('mission-warning').textContent = state.warning ? `Aviso de disponibilidade: ${WARNINGS[state.warning.code] ?? 'o veículo pode não estar disponível'} (${AVAILABILITY[state.warning.availability] ?? 'situação não confirmada'}). A designação foi registrada; nada foi reservado ou bloqueado.` : '';
      const key = detail.id + ':' + detail.revision, changed = key !== shown;
      if (changed) {
        shown = key;
        input('plan-destination').value = detail.plan?.destinationText ?? ''; input('plan-reason').value = detail.plan?.reason ?? '';
        select('plan-load').value = detail.plan?.load ?? 'NONE'; input('plan-cargo').value = detail.plan?.cargo ?? ''; input('plan-passengers').value = detail.plan?.passengers ? String(detail.plan.passengers) : '';
        for (const id of ['vehicle-reason', 'cancel-reason']) input(id).value = '';
      }
      // Focus moves once per opened mission, after its reads settle.
      if (!busy && detail.id !== focused) { focused = detail.id; el('mission-detail-title').focus({ preventScroll: true }); }
      // A new server revision resets the draft; otherwise the choice is kept.
      const centres = select('plan-cost-centre'), centre = changed ? detail.plan?.costCentreId ?? '' : centres.value;
      centres.replaceChildren(option('', 'Sem centro de custo'));
      for (const c of state.costCentres ?? []) centres.append(option(c.id, `${c.name} (${c.code})`));
      centres.value = state.costCentres?.some(c => c.id === centre) ? centre : '';
      el('plan-cost-help').textContent = state.costCentres === null ? 'Sua conta não pode consultar os centros de custo; o planejamento será salvo sem centro de custo.' : 'Escolha o cliente, contrato ou projeto, se houver.';
      const people = select('assign-employee'), person = people.value; people.replaceChildren(option('', state.assignees.some(a => a.id !== detail.assigneeId) ? 'Selecione uma pessoa elegível' : 'Nenhuma pessoa elegível nesta consulta'));
      for (const a of state.assignees) if (a.id !== detail.assigneeId) people.append(option(a.id, personLabel(state, a.id)));
      people.value = state.assignees.some(a => a.id === person) ? person : '';
      el('assignees-next').hidden = state.assigneeCursor === '';
      const swap = select('vehicle-asset'), next = swap.value; swap.replaceChildren(option('', 'Selecione o novo veículo'));
      for (const a of state.assets ?? []) if (a.id !== detail.assetId) swap.append(option(a.id, assetLabel(state, a.id)));
      swap.value = state.assets?.some(a => a.id === next) ? next : '';
      el('mission-plan-form').hidden = !PLAN.includes(detail.state);
      el('mission-assign-form').hidden = !ASSIGN.includes(detail.state);
      el('mission-vehicle-form').hidden = !VEHICLE.includes(detail.state) || state.assets === null;
      el('mission-cancel-form').hidden = !ASSIGN.includes(detail.state);
    } else { shown = ''; focused = ''; }
    agenda(state);
    for (const control of root.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLSelectElement>('button,input,select')) control.disabled = busy || state.authority === null;
    if (state.phase === 'denied' && lastPhase !== 'denied') el('missions-title').focus({ preventScroll: true });
    lastPhase = state.phase;
  }
  const local = (value: string): string => { const at = new Date(value); return Number.isFinite(at.getTime()) ? at.toISOString() : ''; };
  el('missions-load').addEventListener('click', () => { void controller.load(); });
  el('missions-next').addEventListener('click', () => { void controller.more(); });
  el('mission-retry').addEventListener('click', () => { void controller.retry(); });
  el('mission-reload').addEventListener('click', () => { void controller.reload(); });
  el('assignees-next').addEventListener('click', () => { void controller.moreAssignees(); });
  el('vehicle-preview').addEventListener('click', () => { if (select('vehicle-asset').value) void controller.agendaFor(select('vehicle-asset').value); });
  el('mission-create-form').addEventListener('submit', event => {
    event.preventDefault();
    const start = local(input('mission-start').value), end = local(input('mission-end').value);
    input('mission-end').setCustomValidity(start && end && end <= start ? 'O fim deve ser depois do início.' : '');
    if (!el<HTMLFormElement>('mission-create-form').reportValidity()) return;
    void controller.create({ title: input('mission-title').value.trim(), startsAt: start, endsAt: end, assetId: select('mission-vehicle').value });
  });
  el('mission-end').addEventListener('input', () => input('mission-end').setCustomValidity(''));
  el('mission-plan-form').addEventListener('submit', event => {
    event.preventDefault();
    const load = select('plan-load').value as LoadKind;
    input('plan-cargo').required = load === 'CARGO'; input('plan-passengers').required = load === 'PASSENGERS';
    if (!el<HTMLFormElement>('mission-plan-form').reportValidity()) return;
    void controller.plan({ destination: input('plan-destination').value.trim(), reason: input('plan-reason').value.trim(), costCentreId: select('plan-cost-centre').value, load, cargo: input('plan-cargo').value.trim(), passengers: Number(input('plan-passengers').value) });
  });
  el('mission-assign-form').addEventListener('submit', event => { event.preventDefault(); const id = select('assign-employee').value; if (!id) { select('assign-employee').focus(); return; } void controller.assign(id); });
  el('mission-vehicle-form').addEventListener('submit', event => {
    event.preventDefault();
    if (!select('vehicle-asset').value) { select('vehicle-asset').focus(); return; }
    if (!el<HTMLFormElement>('mission-vehicle-form').reportValidity()) return;
    void controller.changeVehicle(select('vehicle-asset').value, input('vehicle-reason').value.trim());
  });
  el('mission-cancel-form').addEventListener('submit', event => {
    event.preventDefault();
    const mission = controller.state.selected;
    if (!mission || !el<HTMLFormElement>('mission-cancel-form').reportValidity()) return;
    if (browser.confirm(`Cancelar a missão "${mission.title}"? A pessoa designada deixa de ter esta missão e a decisão fica no histórico.`)) void controller.cancel(input('cancel-reason').value.trim());
  });
  function clear(): void {
    identity = ''; shown = ''; focused = ''; controller.clear();
    for (const id of ['mission-create-form', 'mission-plan-form', 'mission-vehicle-form', 'mission-cancel-form']) el<HTMLFormElement>(id).reset();
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
