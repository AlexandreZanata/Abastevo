# Glossary

Status: **PLANNED**, 2026-11-10. Canonical vocabulary for `docs/enterprise/`. One term has one meaning. Code, contracts and documents use the English term; the interface uses the Portuguese term shown. Existing project terms keep their existing meaning and are marked `existing`.

## 1. Words that already mean something here

| Word | Existing meaning (do not reuse) | Term used in this plan |
|---|---|---|
| Contributor | `existing` Anonymous device-key identity in the community (`identity` module); also an open-source project contributor (`contributor` module) | Never used for an employee. |
| Owner | `existing` The account that owns a private record in the cloud protocol; also the project owner who holds the admin key | **Organization owner** is always written in full. |
| Enterprise | `existing` Account tier label (ADR-023), ESP data offering, single-payer payment candidate | **Organization**, **Abastevo Empresas**. |
| Premium / Pro | `existing` Individual paid bundle; also a gasoline grade in legacy enums | **Individual Premium** when the paid bundle is meant. |
| Receipt | `existing` In the cloud protocol, the stored result of a command | **Acknowledgement** for sync; **receipt** only for a proof of purchase. |
| Operator | `existing` Abastevo staff acting through `ops` or the admin suite; also the company operating a station (`station_operator_revisions`) | **Platform operator** and **station operator**, always qualified. |
| Grant | `existing` `RepresentationGrant` (station), `entitlement_grants` (tiers), `payment_grants` | **Station authority** for representation; **role assignment** for permissions. |
| Trip | `existing` A saved personal map trip (`map_saved_trip`) | **Trip** here is always an organization trip; the personal one is "saved personal trip". |
| Shift | `existing` Individual driver work session in the driver suite | Not used. An organization records **trips**, not shifts. |
| Ledger | `existing` Personal refuel, charge and expense records | Not used for organization records. |

## 2. Organization and people

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Organization | Empresa | The customer entity and the isolation boundary. Every enterprise record belongs to exactly one organization. It is not a CNPJ and not a unit. |
| Legal entity | Pessoa jurídica | A registered person (CNPJ, or CPF for a rural producer) that the organization declares as its own. Declared or verified. |
| Establishment | Estabelecimento | One full CNPJ (root, order and check digits) of a legal entity. "0001" does not prove head office. |
| Operating unit | Unidade | A place or division where the organization runs operations. May or may not match an establishment. Has a time zone. |
| Team | Equipe | A named group of employees inside an organization. |
| Cost centre | Centro de custo | An accounting dimension to which costs are attributed. |
| Organization group | Grupo empresarial | A set of organizations that agreed to consolidated reading. Sharing partners or a CNPJ root never creates one. |
| Account | Conta | A login identity in Abastevo (`existing` `accounts`). Belongs to a person, never to an organization. |
| Managed account | Acesso corporativo | An account created through an organization's activation code, usable only in that organization's enterprise mode. |
| Person | Pessoa | The human being. Not stored as a platform-wide record; an organization knows a person only through its employee record. |
| Employee | Funcionário | The organization's record of someone who works for it. Exists with or without a login. Owned by the organization. |
| Driver profile | Perfil de condutor | The part of an employee record that says the person may drive: licence category and expiry. Not every employee has one. |
| Membership | Vínculo | The link between one account and one employee record of one organization, with a state and a period. |
| Role | Papel | A named set of permissions. |
| Scope | Escopo | The part of the organization where a role applies: whole organization, unit, cost centre, team, asset, or own records. |
| Role assignment | Atribuição de papel | Membership + role + scope + period. |
| Delegation | Delegação | A time-boxed role assignment given to a substitute. |
| Credential | Credencial | What proves control of an account: account key, e-mail code, linked provider, or an enrolled device key. |
| Device | Aparelho | An installation registered for an account (`existing` device reference of the cloud protocol), revocable. |
| Invitation | Convite | A single-use, expiring offer for an existing account to become a member. |
| Activation code | Código de ativação | A single-use, expiring secret that lets an employee create a managed account on a device. |
| Active context | Contexto ativo | What the app or web is acting as right now: personal, or one organization. |
| Personal mode / Enterprise mode | Modo pessoal / Modo empresa | The two app experiences. Enterprise mode always shows the organization name. |
| Offboarding | Desligamento | Ending a membership: permissions stop, organization history stays, the personal account is untouched. |

## 3. Fleet

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Asset | Ativo | A vehicle, machine or implement registered by an organization. |
| Vehicle / Machine / Implement | Veículo / Máquina / Implemento | Asset kinds. A machine is usually measured in hours. An implement has no engine and is attached to a powered asset. |
| Tenure | Posse | How the organization holds an asset: owned, leased, rented, third-party, loaned. |
| Tank | Tanque | A fuel container of an asset, with capacity and accepted products. |
| Meter | Medidor | An odometer (metres) or an hour meter (seconds) installed on an asset. A replaced meter is a new meter. |
| Meter reading | Leitura | One observed value of a meter, with instant, source and author. Append-only evidence. |
| Assignment | Atribuição | A period in which an employee is responsible for, or allowed to use, an asset. |
| Reservation | Reserva | A planned period in which an asset is held for a mission. |
| Availability | Disponibilidade | Derived answer to "can this asset start an operation now", from lifecycle, reservations, trips, maintenance and blocks. |
| Supplier | Fornecedor | A business the organization buys from: station, workshop, lessor. Private to the organization. |

## 4. Operations

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Mission | Atividade | Work the organization wants done, planned by a manager and assigned to people. May need several trips. The request's "missão ou ordem de serviço" in operations. |
| Trip | Viagem | One continuous use of one main asset by one or more drivers, from start to end. |
| Leg | Trecho | The part of a trip between two consecutive stops. |
| Stop | Parada | A planned or actual halt, with a place and a reason. |
| Logbook | Diário de bordo | The ordered timeline of a trip's events. Operational record only; not a time clock. |
| Logbook event | Evento | One timeline entry: start, stop, fueling, toll, parking, incident, handover, note, end. |
| Handover | Troca de condutor | The moment the driver responsible for a trip changes. |
| Trip closing | Encerramento | Finishing a trip: final readings, conference of expenses and documents, submission. |
| Checklist | Checklist | A list of items verified at departure or return. |
| Incident | Ocorrência | Something that happened and must be known: breakdown, accident, fine notice, theft. |
| Damage | Avaria | A physical defect found on an asset. |

## 5. Location

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Route plan | Rota planejada | Destinations and stops chosen for a mission or trip, with a planned distance. |
| Navigation | Navegação | Turn-by-turn guidance in the app. |
| Tracking session | Sessão de rastreamento | The period in which the app records location for one trip, between an explicit start and an explicit stop. |
| Track | Trajeto | The recorded positions of a tracking session. |
| Fix | Posição | One recorded position with its time and accuracy. |
| Gap | Lacuna | A period of a tracking session with no usable fixes. |
| Live position | Última posição | The latest fix received for a trip in progress, always shown with its collection time. |
| Stale | Desatualizada | A live position older than the freshness limit. Never shown as current. |
| Geofence | Cerca virtual | An area whose entry or exit produces an alert. |
| Planned / provider / GPS / odometer distance | Distância planejada / estimada / por GPS / por odômetro | Four different numbers with four different sources. They are never added or substituted silently. |

## 6. Fuel, documents and money

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Fueling | Abastecimento | One supply of one product into one tank of one asset at one time, recorded by an organization. |
| Fuel product | Produto | What was supplied: a fuel, a gas, electricity, or an additive such as ARLA 32. Each has one unit. |
| Fill level | Tanque cheio / parcial | Whether the fueling filled the tank: full, partial or unknown. |
| Fueling establishment | Local do abastecimento | Where the fueling physically happened. |
| Document issuer | Emitente | The legal entity named as issuer on the fiscal document. |
| Contracted supplier | Fornecedor contratado | The party with whom the organization has the commercial relation. |
| Document | Documento | A stored original file with its metadata: photo, PDF or XML. |
| Receipt | Comprovante | A document that shows a purchase or payment. |
| Fiscal document | Documento fiscal | A structured NF-e or NFC-e read from a document: access key, issuer, items, totals. |
| Fiscal item | Item fiscal | One line of a fiscal document. |
| Extraction | Extração | Reading fields from a document, by parser or OCR, with a confidence per field. |
| Conference | Conferência | A person checking a document against the operation. |
| Allocation | Rateio | Assigning part of a document's or an expense's amount to an operation, asset or cost centre. |
| Reconciliation | Conciliação | The state of agreement between a document's amounts and the operations allocated to it. |
| Expense | Despesa | An economic cost of the organization, counted once. |
| Company amount | Valor da empresa | The part of an expense the organization bears. |
| Review | Revisão | The administrative lifecycle of a submitted record: approve, return, reject. |
| Approval | Aprovação | A positive review decision by someone allowed to decide. |
| Payable | Obrigação | An amount the organization owes: to a supplier, to an employee, to a card issuer. |
| Payment | Pagamento | A record that a payable was settled, usually outside Abastevo. |
| Advance | Adiantamento | Money given to an employee before expenses happen. |
| Reimbursement | Reembolso | A payable to an employee for an expense paid from their own pocket. |
| Budget | Orçamento | A limit for a period and a dimension. |
| Budget hold | Reserva de limite | An amount set aside when an expense is authorized before it happens. |
| Period lock | Fechamento de período | Closing a period so that approved records in it change only by amendment after an authorized reopening. |
| Amendment | Retificação | A new version of an approved record, with reason, that keeps the previous version. |
| Work order | Ordem de serviço | A maintenance job on an asset. Only maintenance uses this term. |
| Internal tank | Tanque interno | A fuel tank owned by the organization from which assets are fuelled. |
| Stock movement | Movimentação | A receipt into, dispensing from, transfer between or adjustment of an internal tank. |

## 7. Prices and stations

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Station | Posto | `existing` The canonical establishment in the directory catalog (one UUID). |
| Official price | Preço oficial ANP | `existing` A price from the ANP survey, dated. |
| Community price | Preço da comunidade | `existing` The projection of community observations. |
| Announced price | Preço informado pelo posto | A price published by a station's verified representative, with validity and conditions. A separate source. |
| Negotiated price | Preço negociado | A private price under an agreement between a fleet and a station. Never public. |
| Price reference | Referência de preço | The comparable price, with its source, place, period and conditions, used in one analysis. |
| Claim | Reivindicação | `existing` A request to represent a station (`profile_claims`). |
| Station authority | Autorização de representação | The reviewed right of an organization or account to manage a station's business data. Extends `existing` `RepresentationGrant`. |
| Representative | Representante | A member who acts for a station under a station authority. |
| Representation badge | Selo de representação | `existing` "Representante da empresa verificado". Means reviewed representation only. |
| Agreement | Convênio | A contract between a fleet organization and a station organization. |
| Authorization | Autorização de abastecimento | Permission issued by a fleet for one fueling under an agreement, with limits and expiry. |
| Voucher code | Código | The single-use secret that identifies an authorization at the station. |
| Redemption | Resgate | The station confirming a fueling against an authorization. |

## 8. Platform

| Term | Interface (pt-BR) | Definition |
|---|---|---|
| Operation | Operação | One change recorded on a device and sent to the server, with a stable identifier. |
| Acknowledgement | Confirmação | The server's durable answer to an operation. |
| Sync state | Situação de envio | Where a record is between the device and the organization: saved on device, sent, accepted, needs review, rejected. |
| Operational state | Estado | What is happening in the real world to a trip, a work order, an asset. |
| Review state | Situação de revisão | Where a record is in the administrative lifecycle. |
| Capability | Capacidade | `existing` A named feature right evaluated on the server (`entitlement`). |
| Entitlement | Direito do plano | What an organization's plan allows: capabilities and limits. |
| Limit | Limite | A measurable ceiling of a plan: active vehicles, managed stations, storage, API volume. |
| Subscription | Assinatura | An organization's commercial relation for one product line, with a state. |
| Indicator | Indicador | A defined, versioned measure with formula and inclusion rules. |
| Finding | Achado | A detected difference that needs human review, with evidence and an outcome. Never a statement of fraud. |
| Dataset | Conjunto de dados | A documented, versioned data product with origin, coverage and method. |
| API key | Chave de API | An organization credential for the Data API, with scopes and quota. |
| Audit event | Evento de auditoria | An append-only record of who did what to which record and why. |
| Support session | Sessão de suporte | A time-boxed, organization-approved and audited access by a platform operator. |
| Platform operator | Operador Abastevo | Abastevo staff acting through the admin suite or `ops`. |

## 9. Status words with a fixed meaning

| Word | Means | Does not mean |
|---|---|---|
| Saved on device | The record is durable on the phone | The company has it |
| Sent | The server stored the operation | The company accepted it |
| Accepted | The server applied the operation to the organization's records | A manager approved it |
| Needs review | A person must resolve something before the record counts | The record was lost |
| Submitted | The author asked for review | Anyone approved it |
| Approved | An authorized reviewer accepted the record | The fiscal document is authentic |
| Received (document) | The file is stored | Its content was checked |
| Structure checked | The key or XML is well formed | The document is authorized or not cancelled |
| Conferred | A person compared the document with the operation | A tax authority confirmed it |
| Reconciled | Allocated amounts agree with the document within tolerance | The cost is approved |
| Consulted | An authorized source was queried at a stated time | The answer is still true now |
| Insufficient data | The method cannot produce a result from what was recorded | The result is zero |
