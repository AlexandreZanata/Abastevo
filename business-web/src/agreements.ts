import { ApiError, type CompanyTransport } from './api.js';
import { type Authority, record } from './model.js';

function agreementErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'OPERATION_ID_REUSED': return 'Identificador de operação já utilizado. Gere um novo comando.';
      case 'CONFLICT': return 'Registro alterado por outra operação. Atualize os dados e tente novamente.';
      case 'VALIDATION_FAILED': return 'Produto ou unidade fora dos termos negociados do convênio.';
      case 'FRESH_AUTH_REQUIRED': return 'Confirmação recente de acesso necessária para esta alteração.';
      case 'ENTITLEMENT_REQUIRED': return 'Capacidade de organização desabilitada para convênios no momento.';
    }
    return 'Não foi possível concluir a operação de convênios e autorizações.';
  }
  return err instanceof Error ? err.message : 'Falha na operação de convênios e autorizações.';
}

export interface AgreementTermItem {
  product: string;
  unit: string;
  priceMilliBrl: number;
  conditions: string;
}

export interface AgreementItem {
  id: string;
  stationId: string;
  orgLabel: string;
  state: string;
  derivedState: string;
  revision: number;
  validFrom: string;
  validUntil: string;
  conditions: string;
  terms: AgreementTermItem[];
  usable: boolean;
}

export interface AuthorizationItem {
  id: string;
  agreementId: string;
  stationId: string;
  employeeId: string;
  assetId: string;
  product: string;
  unit: string;
  maxQuantityMilli: number;
  maxCentavos: number;
  validUntil: string;
  state: string;
  derivedState: string;
  failedAttempts: number;
}

export interface RedemptionItem {
  id: string;
  agreementId: string;
  authorizationId: string;
  stationId: string;
  employeeId: string;
  product: string;
  unit: string;
  quantityMilli: number;
  priceMilliBrl: number;
  totalCentavos: number;
  confirmedAt: string;
}

export interface AgreementsSource {
  listAgreements(orgId: string): Promise<{ agreements: AgreementItem[] }>;
  readAgreement(orgId: string, agreementId: string): Promise<{ agreement: AgreementItem | null }>;
  proposeAgreement(orgId: string, payload: Record<string, unknown>): Promise<{ agreement: AgreementItem | null; replayed: boolean }>;
  applyAgreement(orgId: string, agreementId: string, action: 'SUSPEND' | 'END', payload: Record<string, unknown>): Promise<{ agreement: AgreementItem | null; replayed: boolean }>;
  listAuthorizations(orgId: string): Promise<{ authorizations: AuthorizationItem[] }>;
  issueAuthorization(orgId: string, payload: Record<string, unknown>): Promise<{ authorization: AuthorizationItem | null; code: string; replayed: boolean }>;
  cancelAuthorization(orgId: string, authorizationId: string, payload: Record<string, unknown>): Promise<{ authorization: AuthorizationItem | null; replayed: boolean }>;
  listRedemptions(orgId: string): Promise<{ redemptions: RedemptionItem[] }>;
}

export interface AgreementsState {
  phase: 'closed' | 'idle' | 'loading' | 'ready' | 'failed';
  authority: Authority | null;
  agreements: AgreementItem[];
  authorizations: AuthorizationItem[];
  redemptions: RedemptionItem[];
  message: string;
  lastCode: string;
}

export class AgreementsApi implements AgreementsSource {
  constructor(private readonly api: CompanyTransport) {}

  async listAgreements(orgId: string): Promise<{ agreements: AgreementItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/station-agreements/list`, {});
    const raw = Array.isArray(res.agreements) ? res.agreements : [];
    return { agreements: raw.map(parseAgreement) };
  }

  async readAgreement(orgId: string, agreementId: string): Promise<{ agreement: AgreementItem | null }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/station-agreements/${agreementId}/read`, {});
    return { agreement: res.agreement ? parseAgreement(res.agreement) : null };
  }

  async proposeAgreement(orgId: string, payload: Record<string, unknown>): Promise<{ agreement: AgreementItem | null; replayed: boolean }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/station-agreements/propose`, payload);
    return { agreement: res.agreement ? parseAgreement(res.agreement) : null, replayed: Boolean(res.replayed) };
  }

  async applyAgreement(orgId: string, agreementId: string, action: 'SUSPEND' | 'END', payload: Record<string, unknown>): Promise<{ agreement: AgreementItem | null; replayed: boolean }> {
    const suffix = action === 'SUSPEND' ? 'suspend' : 'end';
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/station-agreements/${agreementId}/${suffix}`, payload);
    return { agreement: res.agreement ? parseAgreement(res.agreement) : null, replayed: Boolean(res.replayed) };
  }

  async listAuthorizations(orgId: string): Promise<{ authorizations: AuthorizationItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/fueling-authorizations/list`, {});
    const raw = Array.isArray(res.authorizations) ? res.authorizations : [];
    return { authorizations: raw.map(parseAuthorization) };
  }

  async issueAuthorization(orgId: string, payload: Record<string, unknown>): Promise<{ authorization: AuthorizationItem | null; code: string; replayed: boolean }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/fueling-authorizations/issue`, payload);
    return { authorization: res.authorization ? parseAuthorization(res.authorization) : null, code: String(res.code ?? ''), replayed: Boolean(res.replayed) };
  }

  async cancelAuthorization(orgId: string, authorizationId: string, payload: Record<string, unknown>): Promise<{ authorization: AuthorizationItem | null; replayed: boolean }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/fueling-authorizations/${authorizationId}/cancel`, payload);
    return { authorization: res.authorization ? parseAuthorization(res.authorization) : null, replayed: Boolean(res.replayed) };
  }

  async listRedemptions(orgId: string): Promise<{ redemptions: RedemptionItem[] }> {
    const res = await this.api.privatePost(`/v1/orgs/${orgId}/fueling-redemptions/list`, {});
    const raw = Array.isArray(res.redemptions) ? res.redemptions : [];
    return {
      redemptions: raw.map((item) => {
        const r = record(item) ?? {};
        return {
          id: String(r.id ?? ''),
          agreementId: String(r.agreement_id ?? ''),
          authorizationId: String(r.authorization_id ?? ''),
          stationId: String(r.station_id ?? ''),
          employeeId: String(r.employee_id ?? ''),
          product: String(r.fuel_product ?? ''),
          unit: String(r.unit ?? ''),
          quantityMilli: Number(r.quantity_milli ?? 0),
          priceMilliBrl: Number(r.price_milli_brl ?? 0),
          totalCentavos: Number(r.total_centavos ?? 0),
          confirmedAt: String(r.confirmed_at ?? ''),
        };
      }),
    };
  }
}

function parseAgreement(item: unknown): AgreementItem {
  const r = record(item) ?? {};
  const rawTerms = Array.isArray(r.terms) ? r.terms : [];
  return {
    id: String(r.id ?? ''),
    stationId: String(r.station_id ?? ''),
    orgLabel: String(r.org_label ?? ''),
    state: String(r.state ?? ''),
    derivedState: String(r.derived_state ?? r.state ?? ''),
    revision: Number(r.revision ?? 1),
    validFrom: String(r.valid_from ?? ''),
    validUntil: String(r.valid_until ?? ''),
    conditions: String(r.conditions ?? ''),
    terms: rawTerms.map((term) => {
      const t = record(term) ?? {};
      return { product: String(t.fuel_product ?? ''), unit: String(t.unit ?? ''), priceMilliBrl: Number(t.price_milli_brl ?? 0), conditions: String(t.conditions ?? '') };
    }),
    usable: Boolean(r.usable),
  };
}

function parseAuthorization(item: unknown): AuthorizationItem {
  const r = record(item) ?? {};
  return {
    id: String(r.id ?? ''),
    agreementId: String(r.agreement_id ?? ''),
    stationId: String(r.station_id ?? ''),
    employeeId: String(r.employee_id ?? ''),
    assetId: String(r.asset_id ?? ''),
    product: String(r.fuel_product ?? ''),
    unit: String(r.unit ?? ''),
    maxQuantityMilli: Number(r.max_quantity_milli ?? 0),
    maxCentavos: Number(r.max_centavos ?? 0),
    validUntil: String(r.valid_until ?? ''),
    state: String(r.state ?? ''),
    derivedState: String(r.derived_state ?? r.state ?? ''),
    failedAttempts: Number(r.failed_attempts ?? 0),
  };
}

export class AgreementsController {
  state: AgreementsState = { phase: 'closed', authority: null, agreements: [], authorizations: [], redemptions: [], message: '', lastCode: '' };

  constructor(
    private readonly source: AgreementsSource,
    private readonly onRender: (state: AgreementsState) => void,
    private readonly sessionLost?: (message: string) => void,
  ) {}

  select(authority: Authority): void {
    this.state = { ...this.state, authority, phase: 'idle', message: '' };
    void this.refresh();
  }

  clear(): void {
    this.state = { phase: 'closed', authority: null, agreements: [], authorizations: [], redemptions: [], message: '', lastCode: '' };
    this.onRender(this.state);
  }

  async refresh(): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Carregando convênios, autorizações e resgates…' };
    this.onRender(this.state);
    try {
      const perms = new Set(auth.permissions ?? []);
      let agreements = this.state.agreements;
      let authorizations = this.state.authorizations;
      let redemptions = this.state.redemptions;
      if (perms.has('agree.agreement.manage')) {
        agreements = (await this.source.listAgreements(auth.organization.id)).agreements;
        redemptions = (await this.source.listRedemptions(auth.organization.id)).redemptions;
      }
      if (perms.has('agree.authorization.issue')) {
        authorizations = (await this.source.listAuthorizations(auth.organization.id)).authorizations;
      }
      this.state = { ...this.state, phase: 'ready', agreements, authorizations, redemptions, message: '' };
    } catch (err) {
      if (err instanceof ApiError && err.code === 'UNAUTHENTICATED') {
        this.sessionLost?.('Sessão encerrada. Entre novamente para continuar.');
        this.clear();
        return;
      }
      this.state = { ...this.state, phase: 'failed', message: agreementErrorMessage(err) };
    }
    this.onRender(this.state);
  }

  async propose(orgId: string, stationId: string, orgLabel: string, validFrom: string, validUntil: string, terms: Array<{ product: string; unit: string; priceMilliBrl: number }>): Promise<void> {
    try {
      await this.source.proposeAgreement(orgId, {
        schema_version: 1,
        operation_id: crypto.randomUUID(),
        station_id: stationId,
        org_label: orgLabel,
        valid_from: validFrom,
        valid_until: validUntil,
        terms: terms.map((t) => ({ fuel_product: t.product, unit: t.unit, price_milli_brl: t.priceMilliBrl })),
      });
      await this.refresh();
    } catch (err) {
      this.fail(err);
    }
  }

  async apply(orgId: string, agreementId: string, action: 'SUSPEND' | 'END', revision: number, reason: string): Promise<void> {
    try {
      await this.source.applyAgreement(orgId, agreementId, action, {
        schema_version: 1,
        operation_id: crypto.randomUUID(),
        revision,
        reason,
      });
      await this.refresh();
    } catch (err) {
      this.fail(err);
    }
  }

  async issue(orgId: string, agreementId: string, employeeId: string, product: string, unit: string, maxQuantityMilli: number, maxCentavos: number, validUntil: string): Promise<void> {
    try {
      const out = await this.source.issueAuthorization(orgId, {
        schema_version: 1,
        operation_id: crypto.randomUUID(),
        agreement_id: agreementId,
        employee_id: employeeId,
        fuel_product: product,
        unit,
        max_quantity_milli: maxQuantityMilli,
        max_centavos: maxCentavos,
        valid_until: validUntil,
      });
      // The one-time code is shown exactly once; reads never repeat it.
      this.state = { ...this.state, lastCode: out.replayed ? '' : out.code, message: out.replayed ? 'Comando repetido: autorização já emitida.' : '' };
      await this.refresh();
      if (!out.replayed && out.code) this.state = { ...this.state, lastCode: out.code };
    } catch (err) {
      this.fail(err);
    }
  }

  async cancel(orgId: string, authorizationId: string, reason: string): Promise<void> {
    try {
      await this.source.cancelAuthorization(orgId, authorizationId, {
        schema_version: 1,
        operation_id: crypto.randomUUID(),
        reason,
      });
      await this.refresh();
    } catch (err) {
      this.fail(err);
    }
  }

  private fail(err: unknown): void {
    if (err instanceof ApiError && err.code === 'UNAUTHENTICATED') {
      this.sessionLost?.('Sessão encerrada. Entre novamente para continuar.');
      this.clear();
      return;
    }
    this.state = { ...this.state, phase: 'failed', message: agreementErrorMessage(err) };
    this.onRender(this.state);
  }
}
