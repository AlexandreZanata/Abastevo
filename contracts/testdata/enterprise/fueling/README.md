# Pure supply-event oracle v1

EMP-FUEL-001 / B-BR-EMP-137..143. This synthetic oracle covers a pure
submit-ready candidate, not server acceptance, payment, approval, upload,
reimbursement, consumption or an active runtime producer. The document issuer
is a separate DOC-owned source projection and is never a Candidate field.

## Format

`fueling-v1.psv` uses the exact four-column header
`id|operation|args|expected`. Fields are semicolon-separated `key=value`, split
at the first equals sign. Text inputs use lowercase even-length UTF-8 hex.
The question mark is fixture-only absence, distinct from a present zero or
present empty text. No blank or unknown rows are skipped. Consumers assert the
exact hash, count, operation counts, nonempty unique case IDs, distinct field
names and complete result maps. Unknown syntax fails closed.

Alias UUIDs use prefix `9d000000-0000-4000-8000-` plus a twelve-digit suffix:
A=1, B=2 (organizations); X=11, Y=12 (assets); E=21, J=22 (employees);
U=31, V=32 (author memberships); M=41, N=42 (meters); S=51, T=52
(segments); R=61, W=62 (readings); K=71 (destination asset tank), G=72
(destination tank group); H=81 (internal establishment source tank);
P=91, Q=92 (public stations); Z=111, C=112 (private suppliers);
L=121, D=122 (documents); F=131 (fueling identity).
An unknown alias is literal input; `?` means absent. Raw UUIDs do not prove
repository existence or provenance. Public station references have no private
organization ownership.

## Fixed semantics

- The catalog version is `fuel-products-v1`, exactly nine product keys. It
  owns fixed category/unit/storage/family descriptors. Storage tokens are
  ML/MILLI_M3/WH; families ETHANOL/GASOLINE/DIESEL/NATURAL_GAS/ELECTRIC/NONE.
  ARLA_32 has category ADDITIVE and family NONE; cylinder LPG_P13 is excluded.
- Canonical integer amounts reuse accepted bounds: quantity1..1000000000,
  paid/discount0..1000000000000, optional unit price1..1000000 milli-BRL.
  No sign, decimal, exponent, grouping, redundant integer leading zero,
  whitespace or Unicode digit normalization. A supplied unit differing from
  the owned product unit returns PRODUCT_UNIT_MISMATCH. Paid total and raw
  optional original price text remain unchanged (well-formed Unicode only;
  blank/control text is preserved without normalization or an invented cap).
  No consistency check,
  warning, derived price or discount-to-paid relationship is introduced.
- Candidate construction requires identity, asset, responsible employee,
  author membership, TimeDeclaration, product/quantity/paid, explicit fill,
  payer/payment and exactly one reading reference or explicit missing reason.
  A malformed present field or missing required field is INVALID. Candidate
  origin is independently optional; no APP default is inferred.
- FULL/PARTIAL/UNKNOWN are recorded explicitly. Effective fill maps only
  UNKNOWN to PARTIAL. Quantity does not imply FULL. No actual anchor or
  consumption-eligibility status exists. Category ADDITIVE remains separate
  even though its unit L matches liquid fuel.
- A declaration uses the accepted validation-only OrgTime entry: bounded
  instant, IANA zone and integral-second offset, normalized UTC. No server
  received_at or reporting-day field is supplied or fabricated. Explicit
  minimum declared instant and zone/offset disagreement remain valid.
- Reading evidence is a Meter001 ReadingRef matching the exact organization
  and asset, or Meter001 Missing(reason). It does not claim that a reading
  exists or is VALID. No received Reading is constructed by this fixture.
- Destination asset tank and tank group are mutually exclusive optional
  references, bound to the same asset. Optional trip and contracted supplier
  remain private same-organization references. Establishment is independently
  absent or a CATALOG_STATION, ORG_SUPPLIER, INTERNAL_TANK source, or NOT_FOUND
  typed name. NOT_FOUND name is well-formed, control-free, nonblank and has
  no invented numeric cap. No coordinates or public station writes occur.
- Separate roles projection accepts an optional DocumentRef, positive
  signed-int64 source revision and DOC-provided issuer label. DOC transport
  lexeme parsing is not a new FUEL responsibility.
  Label is well-formed, control-free, nonblank and uncapped. It is not a
  SupplierRef, fiscal validation or evidence that a document is linked/readable.
  Same or different supplied roles remain unchanged; no role substitutes for
  another. Candidate carries no issuer copy.
- Per-product purchased quantities sum exactly by product. A compatible
  cross-product total requires one category AND one unit; an empty compatible
  input is INVALID, while empty per-product input has no groups. Use widened
  exact sums, no total/count cap or value-based deduplication. Totals are
  purchased quantities, never a ratio or consumption.

## Operations and defaults

Errors are exactly `error=INVALID`, `error=SCOPE` or
`error=PRODUCT_UNIT_MISMATCH`. A rejected constructor has no successful output.

- CATALOG (`all=true`) returns version/count and sorted literal product keys.
- PRODUCT (`key`) returns version/key/category/unit/storage/family/additive.
- ENUM (`kind=FILL|PAYER|PAYMENT|ORIGIN|PLACE;value_hex`) validates the exact
  documented closed token and returns it. FILL additionally returns effective.
  Unknown fixture kind is malformed oracle grammar and fails the loader; it
  does not require a generic production enum-dispatch API.
- QUANTITY (`product;unit;amount`) returns product/category/unit/amount from
  an owned catalog-backed exact quantity.
- CANDIDATE constructs a fully supplied candidate, with field overrides below.
  Defaults: A/F, asset X, responsible E, author U, ETHANOL/L, quantity150000,
  paid97350, discount/unit_price/price_text absent, fillPARTIAL, payerCOMPANY,
  paymentCASH, originAPP, effective2026-11-14T12:00:00Z, entryUTC, offset0,
  readingREFERENCE A/X/M/S/R ODOMETER, no destination/trip/place/contracted
  supplier. These are explicit synthetic fixture values, not domain defaults.
  `omit` is a comma list of required input names removed BEFORE construction:
  id,asset,responsible,author,effective,product,quantity,paid,fill,payer,payment,
  reading. `org`, `id`, `asset`, `responsible`, `author`, `product`, `unit`,
  `quantity`, `paid`, `discount`, `unit_price`, `price_text_hex`, `fill`,
  `payer`, `payment`, `origin`, `effective`, `entry`, `offset` override values.
  The reference field's `*_org` overrides its ownership; origin `?` is absent.
  `reading=MISSING` uses `missing_hex`, default "Display unavailable";
  `reading=?` has no evidence. For REFERENCE use `reading_org`,
  `reading_asset`, `reading_meter`, `reading_segment`, `reading_id`,
  `reading_kind`; exact shape/scope is validated by Meter001 refs.
  Optional `tank`, `group`, `trip`, `contracted` accept the corresponding
  aliases, with `*_org`; tank/group also support `*_asset`.
  Optional `place` is absent or a documented kind; `place_ref` identifies its
  public station/private supplier/internal source, and `place_org` scopes
  private variants. NOT_FOUND uses `place_name_hex`. Outputs contain only the
  retained supplied fields and pure `ready=true`/effective_fill/additive facts.
- ROLES starts from the same supplied Candidate default/overrides, plus
  optional `doc` (DocumentRef), `doc_org`, `doc_revision`, `issuer_hex`.
  Absent doc means absent projection. Projection defaults when doc is supplied:
  revision1, label "Synthetic document issuer". Outputs independently retain
  establishment kind/ref/name, source document/revision/issuer label and
  contracted supplier. No equality comparison or automatic substitution.
- TOTAL (`mode=PRODUCT|COMPATIBLE;items`) builds catalog-backed quantities
  from `~`-separated `product,unit,amount` tuples. PRODUCT returns sorted
  `product:category:unit:exactSum` groups and group count. COMPATIBLE returns
  category/unit/amount for a nonempty compatible input. It has no record
  identity, dedupe, review filter, ratio or source-access claim.

The frozen corpus contains200 cases: CATALOG1, PRODUCT15, ENUM38, QUANTITY27,
CANDIDATE91, ROLES15, TOTAL13. SHA256:
`bca4934b4a21935cd1a1728c087fadef5d0935fa39f596b261f844464685395a`.
