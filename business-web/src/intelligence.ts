import { ApiError, type CompanyTransport } from './api.js';
import { type Authority, record } from './model.js';

function intelErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'STOCK_INSUFFICIENT': return 'Saldo insuficiente no tanque para a retirada informada.';
      case 'STOCK_PRODUCT_MISMATCH': return 'Produto ou unidade incompatível com o tanque.';
      case 'METER_REQUIRED': return 'Leitura de medidor válida é obrigatória para o plano.';
      case 'QUOTE_REQUIRED': return 'Orçamento aprovado é obrigatório antes de avançar a OS.';
      case 'QUOTE_EXPIRED': return 'Validade do orçamento expirada.';
      case 'OVERRUN_NOT_APPROVED': return 'Valor realizado excede a tolerância aprovada da OS.';
      case 'HOLD_OPEN': return 'Indisponibilidade aberta impede o encerramento ou expurgo.';
      case 'INVOICE_REQUIRED': return 'Documento fiscal obrigatório antes do fechamento da OS.';
      case 'CONTROL_DISABLED': return 'Controle de validade está desabilitado para a organização.';
      case 'FINDING_CLOSED': return 'Achado já encerrado; abra revisão por nova execução.';
      case 'SELF_APPROVAL': return 'Segregação de funções: outro responsável deve decidir este registro.';
      case 'CONFLICT': return 'Registro alterado por outra operação. Atualize os dados.';
      case 'FRESH_AUTH_REQUIRED': return 'Confirmação recente de acesso necessária para esta alteração.';
    }
    return 'Não foi possível concluir a operação de gestão e inteligência.';
  }
  return err instanceof Error ? err.message : 'Falha na operação de gestão e inteligência.';
}

export interface KpiValueItem {
  indicatorId: string;
  namePt: string;
  status: string;
  displayValue: string;
  unit: string;
  sampleSmall: boolean;
  insufficiencyReason: string;
}

export interface FindingItem {
  id: string;
  findingType: string;
  subjectKey: string;
  severity: string;
  state: string;
  title: string;
  nonProofNote: string;
  revision: number;
}

export interface AuditEventItem {
  id: string;
  action: string;
  targetType: string;
  targetId: string;
  reason: string;
  occurredAt: string;
}

export interface MaintenancePlanItem {
  id: string;
  assetId: string;
  name: string;
  occurrenceState: string;
  decidingTrigger: string;
  blocking: boolean;
  revision: number;
}

export interface WorkOrderItem {
  id: string;
  number: number;
  assetId: string;
  kind: string;
  description: string;
  state: string;
  holdOpen: boolean;
  revision: number;
}

export interface StockTankItem {
  id: string;
  name: string;
  product: string;
  unit: string;
  quantityMilli: number;
  valueCentavos: number;
  allowNegative: boolean;
}

export interface ConsolidationGrantItem {
  id: string;
  counterpartOrgId: string;
  state: string;
  allowedDatasets: string[];
  revision: number;
}

export interface IntelligenceSource {
  kpiOverview(orgId: string, dimension: string, includePending: boolean): Promise<{ values: KpiValueItem[] }>;
  listFindings(orgId: string): Promise<{ findings: FindingItem[] }>;
  decideFinding(orgId: string, payload: Record<string, unknown>): Promise<void>;
  listAudit(orgId: string): Promise<{ events: AuditEventItem[] }>;
  listPlans(orgId: string): Promise<{ plans: MaintenancePlanItem[]; compliance: Array<Record<string, unknown>> }>;
  listWorkOrders(orgId: string): Promise<{ workOrders: WorkOrderItem[] }>;
  saveWorkOrder(orgId: string, payload: Record<string, unknown>): Promise<void>;
  listTanks(orgId: string): Promise<{ tanks: StockTankItem[] }>;
  saveTank(orgId: string, payload: Record<string, unknown>): Promise<void>;
  recordMovement(orgId: string, payload: Record<string, unknown>): Promise<void>;
  listConsolidation(orgId: string): Promise<{ grants: ConsolidationGrantItem[] }>;
}

export interface IntelligenceState {
  phase: 'closed' | 'idle' | 'loading' | 'ready' | 'failed';
  authority: Authority | null;
  dimension: string;
  includePending: boolean;
  kpis: KpiValueItem[];
  findings: FindingItem[];
  auditEvents: AuditEventItem[];
  plans: MaintenancePlanItem[];
  complianceCount: number;
  workOrders: WorkOrderItem[];
  tanks: StockTankItem[];
  grants: ConsolidationGrantItem[];
  message: string;
}

export class IntelligenceApi implements IntelligenceSource {
  constructor(private readonly api: CompanyTransport) {}

  async kpiOverview(orgId: string, dimension: string, includePending: boolean): Promise<{ values: KpiValueItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/intelligence/kpis/overview`, { dimension, include_pending: includePending });
    const ov = record(res.overview) ?? {};
    const raw = Array.isArray(ov.values) ? ov.values : [];
    return {
      values: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          indicatorId: String(r.indicator_id ?? ''),
          namePt: String(r.name_pt ?? ''),
          status: String(r.status ?? ''),
          displayValue: String(r.display_value ?? ''),
          unit: String(r.unit ?? ''),
          sampleSmall: Boolean(r.sample_small),
          insufficiencyReason: String(r.insufficiency_reason ?? ''),
        };
      }),
    };
  }

  async listFindings(orgId: string): Promise<{ findings: FindingItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/intelligence/findings/list`, {});
    const raw = Array.isArray(res.findings) ? res.findings : [];
    return {
      findings: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          findingType: String(r.finding_type ?? ''),
          subjectKey: String(r.subject_key ?? ''),
          severity: String(r.severity ?? ''),
          state: String(r.state ?? ''),
          title: String(r.title ?? ''),
          nonProofNote: String(r.non_proof_note ?? ''),
          revision: Number(r.revision ?? 1),
        };
      }),
    };
  }

  async decideFinding(orgId: string, payload: Record<string, unknown>): Promise<void> {
    await this.api.privatePost(`/v1/orgs/${orgId}/intelligence/findings/decide`, payload);
  }

  async listAudit(orgId: string): Promise<{ events: AuditEventItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/audit/events/list`, { limit: 25 });
    const raw = Array.isArray(res.events) ? res.events : [];
    return {
      events: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          action: String(r.action ?? ''),
          targetType: String(r.target_type ?? ''),
          targetId: String(r.target_id ?? ''),
          reason: String(r.reason ?? ''),
          occurredAt: String(r.occurred_at ?? ''),
        };
      }),
    };
  }

  async listPlans(orgId: string): Promise<{ plans: MaintenancePlanItem[]; compliance: Array<Record<string, unknown>> }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/maintenance/plans/list`, {});
    const rawPlans = Array.isArray(res.plans) ? res.plans : [];
    const rawComp = Array.isArray(res.compliance) ? res.compliance : [];
    return {
      plans: rawPlans.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          assetId: String(r.asset_id ?? ''),
          name: String(r.name ?? ''),
          occurrenceState: String(r.occurrence_state ?? ''),
          decidingTrigger: String(r.deciding_trigger ?? ''),
          blocking: Boolean(r.blocking),
          revision: Number(r.revision ?? 1),
        };
      }),
      compliance: rawComp.map((c) => record(c) ?? {}),
    };
  }

  async listWorkOrders(orgId: string): Promise<{ workOrders: WorkOrderItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/maintenance/work-orders/list`, {});
    const raw = Array.isArray(res.work_orders) ? res.work_orders : [];
    return {
      workOrders: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          number: Number(r.number ?? 0),
          assetId: String(r.asset_id ?? ''),
          kind: String(r.kind ?? ''),
          description: String(r.description ?? ''),
          state: String(r.state ?? ''),
          holdOpen: Boolean(r.hold_open),
          revision: Number(r.revision ?? 1),
        };
      }),
    };
  }

  async saveWorkOrder(orgId: string, payload: Record<string, unknown>): Promise<void> {
    await this.api.privatePost(`/v1/orgs/${orgId}/maintenance/work-orders/save`, payload);
  }

  async listTanks(orgId: string): Promise<{ tanks: StockTankItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/stock/tanks/list`, {});
    const raw = Array.isArray(res.tanks) ? res.tanks : [];
    return {
      tanks: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          name: String(r.name ?? ''),
          product: String(r.product ?? ''),
          unit: String(r.unit ?? ''),
          quantityMilli: Number(r.quantity_milli ?? 0),
          valueCentavos: Number(r.value_centavos ?? 0),
          allowNegative: Boolean(r.allow_negative),
        };
      }),
    };
  }

  async saveTank(orgId: string, payload: Record<string, unknown>): Promise<void> {
    await this.api.privatePost(`/v1/orgs/${orgId}/stock/tanks/save`, payload);
  }

  async recordMovement(orgId: string, payload: Record<string, unknown>): Promise<void> {
    await this.api.privatePost(`/v1/orgs/${orgId}/stock/movements/record`, payload);
  }

  async listConsolidation(orgId: string): Promise<{ grants: ConsolidationGrantItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/consolidation/grants/list`, {});
    const raw = Array.isArray(res.grants) ? res.grants : [];
    return {
      grants: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          counterpartOrgId: String(r.counterpart_org_id ?? ''),
          state: String(r.state ?? ''),
          allowedDatasets: Array.isArray(r.allowed_datasets) ? r.allowed_datasets.map(String) : [],
          revision: Number(r.revision ?? 1),
        };
      }),
    };
  }
}

export class IntelligenceController {
  state: IntelligenceState = {
    phase: 'closed', authority: null, dimension: 'ORG', includePending: false,
    kpis: [], findings: [], auditEvents: [], plans: [], complianceCount: 0,
    workOrders: [], tanks: [], grants: [], message: '',
  };

  constructor(
    private readonly source: IntelligenceSource,
    private readonly onRender: (state: IntelligenceState) => void,
    private readonly sessionLost?: (message: string) => void,
  ) {}

  select(authority: Authority): void {
    this.state = { ...this.state, authority, phase: 'idle', message: '' };
    void this.refresh();
  }

  clear(): void {
    this.state = {
      phase: 'closed', authority: null, dimension: 'ORG', includePending: false,
      kpis: [], findings: [], auditEvents: [], plans: [], complianceCount: 0,
      workOrders: [], tanks: [], grants: [], message: '',
    };
    this.onRender(this.state);
  }

  setFilters(dimension: string, includePending: boolean): void {
    this.state = { ...this.state, dimension, includePending };
    void this.refresh();
  }

  async refresh(): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Carregando indicadores e controles de gestão…' };
    this.onRender(this.state);
    try {
      let kpis = this.state.kpis;
      let findings = this.state.findings;
      let auditEvents = this.state.auditEvents;
      let plans = this.state.plans;
      let complianceCount = this.state.complianceCount;
      let workOrders = this.state.workOrders;
      let tanks = this.state.tanks;
      let grants = this.state.grants;

      const perms = new Set(auth.permissions ?? []);
      if (perms.has('kpi.dashboard.read')) {
        kpis = (await this.source.kpiOverview(auth.organization.id, this.state.dimension, this.state.includePending)).values;
      }
      if (perms.has('find.finding.read')) {
        findings = (await this.source.listFindings(auth.organization.id)).findings;
      }
      if (perms.has('audit.event.read')) {
        auditEvents = (await this.source.listAudit(auth.organization.id)).events;
      }
      if (perms.has('maint.plan.manage')) {
        const res = await this.source.listPlans(auth.organization.id);
        plans = res.plans;
        complianceCount = res.compliance.length;
      }
      if (perms.has('maint.workorder.manage')) {
        workOrders = (await this.source.listWorkOrders(auth.organization.id)).workOrders;
      }
      if (perms.has('stock.tank.manage')) {
        tanks = (await this.source.listTanks(auth.organization.id)).tanks;
      }
      if (perms.has('org.group.manage')) {
        grants = (await this.source.listConsolidation(auth.organization.id)).grants;
      }
      this.state = {
        ...this.state, phase: 'ready',
        kpis, findings, auditEvents, plans, complianceCount, workOrders, tanks, grants,
        message: 'Painel de gestão e inteligência atualizado.',
      };
    } catch (err) {
      if (err instanceof ApiError && err.kind === 'unauthenticated' && this.sessionLost) {
        this.sessionLost(intelErrorMessage(err));
        return;
      }
      this.state = { ...this.state, phase: 'failed', message: intelErrorMessage(err) };
    }
    this.onRender(this.state);
  }

  async detectFindings(operationId: string): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Executando detecção de achados…' };
    this.onRender(this.state);
    try {
      await this.source.decideFinding(auth.organization.id, { schema_version: 1, operation_id: operationId, action: 'REPROCESS' });
      await this.refresh();
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: intelErrorMessage(err) };
      this.onRender(this.state);
    }
  }

  async resolveFinding(operationId: string, findingId: string, revision: number, action: 'CONFIRM' | 'EXPLAIN' | 'FALSE_POSITIVE', reason: string): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Registrando decisão sobre achado…' };
    this.onRender(this.state);
    try {
      await this.source.decideFinding(auth.organization.id, { schema_version: 1, operation_id: operationId, action, finding_id: findingId, revision, reason });
      await this.refresh();
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: intelErrorMessage(err) };
      this.onRender(this.state);
    }
  }

  async createTank(operationId: string, name: string, product: string, unit: string, capacityMilli: number): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Salvando tanque interno…' };
    this.onRender(this.state);
    try {
      await this.source.saveTank(auth.organization.id, { schema_version: 1, operation_id: operationId, name, product, unit, capacity_milli: capacityMilli });
      await this.refresh();
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: intelErrorMessage(err) };
      this.onRender(this.state);
    }
  }

  async receiveStock(operationId: string, tankId: string, product: string, unit: string, quantityMilli: number, amountCentavos: number): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Registrando entrada em tanque…' };
    this.onRender(this.state);
    try {
      await this.source.recordMovement(auth.organization.id, {
        schema_version: 1, operation_id: operationId, tank_id: tankId, kind: 'RECEIPT',
        product, unit, quantity_milli: quantityMilli, amount_centavos: amountCentavos,
      });
      await this.refresh();
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: intelErrorMessage(err) };
      this.onRender(this.state);
    }
  }

  async createWorkOrder(operationId: string, assetId: string, kind: string, description: string): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Abrindo ordem de serviço…' };
    this.onRender(this.state);
    try {
      await this.source.saveWorkOrder(auth.organization.id, {
        schema_version: 1, operation_id: operationId, action: 'CREATE', asset_id: assetId, kind, description,
      });
      await this.refresh();
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: intelErrorMessage(err) };
      this.onRender(this.state);
    }
  }
}
