# Fiscal structure oracle v1

EMP-DOC-006 / B-BR-EMP-231..234, BUC-EMP-118..119, EDR-158..159. This
synthetic oracle covers pure, offline structure checks of a 44-character
NF-e/NFC-e access key, an NFC-e QR code payload and a bounded NF-e/NFC-e XML
envelope, a key comparison and the feed into the EMP-DOC-001 Structure
dimension. A structure check is never a fiscal authorization: no row contacts
a provider, opens a URL, verifies a signature, protocol, QR hash or CSC, or
reads protNFe status. Issuers are synthetic alphanumeric CNPJs
(`SYNTHE00000140`, `SYNTHE00000221`) or the all-zero numeric field; keys are
built from synthetic fields with computed check digits; hashes and digests are
synthetic hex. The QR host names are public tax-authority bases used as
literal text only and are never contacted.

## Format

`fiscal-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Fields are semicolon-separated `key=value`, split at the first equals sign.
Expected values are literal. The text-bearing argument values (`text`, `unit`,
`tail`, `left`, `right`, `input`) are percent-decoded once by the loader:
`%` followed by two uppercase hexadecimal digits is one byte (so `%7C` is `|`,
`%3B` is `;`, `%25` is `%`, `%0A` is a newline); any other `%` fails the
loader. The decoded bytes are UTF-8 text. An input is `text`, optionally
followed by `unit` repeated `repeat` times (1..2000000) and then `tail`
(`unit`/`repeat` come together; `tail` needs them). No blank or unknown rows
are skipped. Consumers assert the exact hash, row count, operation counts,
nonempty unique case IDs, distinct field names, the allowed argument keys per
operation, complete result maps, and that every operation, outcome, check and
reason they implement appears in at least one row. Unknown syntax fails the
loader; it is never mapped to a domain result.

Go-only rows: every XML row and every COMPARE or FEED row with an `XML:`
operand (118 rows) is checked by the Go server consumer only (EDR-159). The
Kotlin consumer asserts that exact count, rejects any other unknown operand,
and runs the remaining 189 rows.

## Results

A check result is exactly `kind` (ACCESS_KEY, QR_CODE, XML, COMPARISON),
`outcome` (STRUCTURE_OK, STRUCTURE_INVALID, UNSUPPORTED), `reason` (`-` when
STRUCTURE_OK) and `checks` (the checks that ran, in order; the last one
decides a non-OK outcome). Only STRUCTURE_OK adds values:

- ACCESS_KEY: `key`, `uf`, `year_month`, `issuer`, `model`, `series`,
  `number`, `emission`, `code`, `digit` (exact substrings).
- QR_CODE: `key`, `version` (2 or 3), `environment` (PRODUCTION for 1,
  HOMOLOGATION for 2), `layout` (ONLINE, OFFLINE), `total` (exact text, `?`
  online), `centavos` (`?` online or when the exact total exceeds
  1000000000000 centavos; never rounded).
- XML: `key`, `items`, `total`, `centavos` (same rule).
- COMPARISON: `key`.

Check order and reasons (INVALID = STRUCTURE_INVALID):

- Key — KEY_FORMAT: a character outside `0-9A-Z` → KEY_CHARACTER, length
  other than 44 → KEY_LENGTH, positions 1–6 or 19–44 not digits →
  KEY_CHARACTER; KEY_CHECK_DIGIT: modulo 11 of the first 43 characters valued
  ASCII − 48 with weights 2..9 repeating from the right, digit 11 − remainder
  or 0 for remainders 0 and 1 → KEY_CHECK_DIGIT; KEY_FIELDS: UF not one of the
  27 IBGE state codes → KEY_STATE, month outside 01..12 → KEY_MONTH, model not
  55/65 → UNSUPPORTED KEY_MODEL_UNSUPPORTED. Issuer check digits, emission
  type, series and number ranges are not checked. Because remainders 0 and 1
  both give digit 0, a key whose digit is 0 can collide with a mistyped key
  (`key-check-digit-collision-accepted`): a structure check is never identity
  or authenticity.
- QR — QR_SIZE (> 2953 UTF-8 bytes → QR_TOO_LARGE); QR_URL (no
  case-insensitive `http://`/`https://` prefix → UNSUPPORTED QR_NOT_URL; host
  (up to the first `/` or `?`) not a plain DNS name under `.gov.br` — labels
  of 1–63 ASCII letters, digits or inner hyphens, at most 253 characters, no
  user info, port, IP literal or trailing dot → UNSUPPORTED
  QR_HOST_UNSUPPORTED;
  then a character outside visible ASCII, `#`, or a `%` escape that is not two
  hex digits decoding to visible ASCII → QR_MALFORMED); QR_PARAMETERS (no
  parameter named exactly `p` → UNSUPPORTED QR_FORMAT_UNSUPPORTED; any other or
  repeated parameter → QR_UNEXPECTED_PARAMETER); QR_VERSION (the decoded `p`
  split at `|`: fewer than two fields or version other than `2`/`3` →
  UNSUPPORTED QR_VERSION_UNSUPPORTED; version 3 with other than three fields →
  UNSUPPORTED QR_VARIANT_UNSUPPORTED); QR_FIELDS (→ QR_FIELDS: version 2 with
  5 fields key|2|env|csc|hash or 8 fields key|2|env|day|total|digest|csc|hash,
  version 3 key|3|env; env 1/2, csc 1–6 digits, hash 40 hex, day 01–31,
  digest 1–128 hex, total exact profile below); then the key checks; QR_MODEL
  (model other than 65 → UNSUPPORTED QR_MODEL_UNSUPPORTED).
- XML — XML_SIZE (> 1048576 bytes → XML_TOO_LARGE, before parsing; a leading
  UTF-8 BOM is skipped); XML_PARSE (a `<!` declaration → XML_DOCTYPE; a
  processing instruction other than a first XML declaration →
  XML_INSTRUCTION; depth > 32 → XML_DEPTH; > 100000 elements → XML_ELEMENTS;
  anything else not well formed, not UTF-8, undefined entity, text outside or
  no/second root → XML_MALFORMED); XML_ROOT (not `NFe`/`nfeProc` in
  `http://www.portalfiscal.inf.br/nfe` → UNSUPPORTED XML_ROOT_UNSUPPORTED);
  XML_ENVELOPE (`NFe` once under `nfeProc`, `infNFe` once with one `Id`
  starting `NFe` and one `versao` → XML_ENVELOPE); XML_VERSION (`versao` other
  than `4.00` → UNSUPPORTED XML_VERSION_UNSUPPORTED); the key checks on the
  rest of `Id`; XML_FIELDS (one leaf each of ide cUF, cNF, mod, serie, nNF,
  dhEmi, tpEmis, cDV; one emit; one total/ICMSTot/vNF; at most one emit/CNPJ,
  emit/CPF, protNFe; one protNFe/infProt/chNFe when protNFe exists; leaves
  without child elements → XML_FIELDS); XML_ISSUER (emit/CNPJ not 14
  characters accepted by the kernel CNPJ type, or no issuer → XML_ISSUER;
  emit/CPF only → UNSUPPORTED XML_ISSUER_UNSUPPORTED); XML_KEY_CONSISTENCY
  (cUF, dhEmi `YYYY-MM-` year-month, emit/CNPJ, mod, canonical serie/nNF
  zero-padded, tpEmis, cNF, cDV, protNFe chNFe → XML_KEY_MISMATCH); XML_ITEMS
  (`det` under infNFe outside 1..990 → XML_ITEMS); XML_TOTAL (vNF not
  `0|0\.[0-9]{2}|[1-9][0-9]{0,12}(\.[0-9]{2})?` → XML_TOTAL).
- COMPARISON — KEY_MATCH: equal keys → STRUCTURE_OK, else STRUCTURE_INVALID
  KEY_MISMATCH. An operand that is not STRUCTURE_OK is `error=NOT_COMPARABLE`.

## Operations

- KEY / QR / XML (`text`, optional `unit`, `repeat`, `tail`) run one check
  and return the result above.
- COMPARE (`left`, `right`, each `KEY:`, `QR:` or `XML:` followed by the
  operand text) checks both operands, then compares them.
- FEED (`input`, an operand as in COMPARE) creates a synthetic document,
  moves Structure NOT_APPLICABLE → UNCHECKED, then applies the result's
  structure command when there is one. It returns `outcome`, `transition`
  (STRUCTURE_OK, STRUCTURE_INVALID or `-` when the result commands nothing),
  `structure`, `fiscal` and `consulted_at` of the document afterwards; a
  structurally valid document with an authorization protocol stays
  `fiscal=NOT_CONSULTED`.

The frozen corpus contains 307 cases: KEY 64, QR 110, XML 111, COMPARE 13,
FEED 9. SHA256:
`ad8cd1e15a4359b3e622741cd23030420612f506a0ff2238c40ce20502d84f08`.
