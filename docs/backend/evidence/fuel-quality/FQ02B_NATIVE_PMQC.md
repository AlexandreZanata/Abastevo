# FQ-02B — Native PMQC compatibility prerequisite

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING on maintained dev, 2026-10-14.

Before behavior: B-BR-FQ-01/03/04/05, BUC-FQ-01/03. Preserve the existing
synthetic eight-column prepared contract and separately accept the verified
native 19-column October-2026 publication. Unknown/repeated/missing headers
still quarantine the complete run. Native IdNumeric plus exact Produto identifies
one sample; multiple assays never increase sample count. Native fuel labels map
to the existing eight-product kernel; Sim/Não map only assay conformity. Results
and units remain verbatim, including empty results observed in the real file.
An empty result is missing evidence, never an invented zero or chemical finding.
No sample judgment or numeric score follows from this parser.

Verified sources (TLS, bounded downloads, raw files outside Git):

- [PMQC official register](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis).
- [September ZIP](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_09.zip):
  838,040 bytes, SHA256 `98721074799569affbbcbd99e173f42e26cb1107fceed707af7fe4975e95c6ab`.
  Members: CSV 15,937,906 bytes; JSON 4,748,834 bytes. CSV UTF-8/semicolon.
- [Metadata](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc-metadados.pdf):
  DOCX despite suffix; 84,545 bytes, SHA256 `9eaed1444197e46f7bdc0afc1405cbbcb39612bc26e4097cabb32c3e2f144e3f`.
- [Analysis guidance](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/orientacoes-analises-pmqc.docx):
  83,667 bytes, SHA256 `5c4f1cf8daab5174922c9d2c13df2f86a62145c2bfd697e77780454a6a899f2b`.

The source has 66,225 assay rows, 4,164 native sample/product groups and no
full-row duplicates or native IDs shared across products in this edition.
Conforme counts: 65,788 Sim / 437 Não. Seven result cells are empty. Collection
range: September 1–28. Nine source product labels include premium gasoline and
common/additivated ethanol/diesel; all require explicit mapping. This bounded
edition is source-compatibility evidence, not national completeness/calibration.

Native header: DataColeta, IdNumeric, GrupoProduto, Produto, RazaoSocialPosto,
CnpjPosto, Distribuidora, Endereço, Complemento, Bairro, Município, Latitude,
Longitude, Uf, RegiaoPolitica, Ensaio, Resultado, UnidadeEnsaio, Conforme.
Business addresses/coordinates are ignored by this quality stream; it does not
publish coordinates, resolve station identity heuristically or alter Directory.

Guidance distinguishes assay status from sample judgment, with fuel/date-specific
Aspecto exceptions. FQ-12 must implement reviewed sample semantics or retain
insufficient/unclassified state; a raw Não count is not a station score.

Immediate acceptance: native synthetic fixtures, all nine fuel labels, Sim/Não,
empty/quoted/comparator result preservation, sample/product separation, invalid
encoding/date/schema/size, duplicate/reorder determinism, old parser compatibility
and an actual bounded official-file preparation. Append-only schema expansion
must preserve empty results and pass real PostGIS upgrade/recovery checks.


## Delivered and actual validation

The existing eight-column preparation remains compatible. Native input has
strict independent header recognition, all nine verified product labels,
Sim/Não mapping, IdNumeric+Produto sample separation, real Gregorian dates and
verbatim results/units. Rust's new `prepare_quality CSV NEW_OUTPUT_DIRECTORY`
command writes hashed file-backed assay/duplicate/quarantine artifacts and a
`quality-prepared-v1` manifest last. It refuses existing output directories,
invalid/truncated/quarantined headers, oversize input and row bounds without a
publishable result. No new dependency, network call, SQL or canonical publication
inside Rust. Go's conformity vocabulary mirrors the verified native values.

Append-only migration000097 retains source-empty results with the existing
1,024-character ceiling and NOT NULL. Prior files/checksums are untouched.

RED→GREEN: native-layout tests initially quarantined everything, Go initially
returned UNKNOWN for Sim/Não and the old schema rejected an empty source result.
All now pass. One CLI serialization build failure was fixed with explicit count
fields; no test or static rule was disabled.

- Complete Rust crate: 111 tests PASS, zero failures/ignored; native parser five
  cases and CLI two cases, existing coordinate/registry/enforcement/sanction
  suites preserved. Locked dependencies, fmt and all-target clippy PASS.
- Actual September CSV preparation: input66,225 = accepted34,158 + duplicates32,067
  + quarantined0. Sample/product grouping remains4,164. Repeated quality content
  differs only in source station Latitude/Longitude metadata; those coordinates
  cannot create extra assay support. All repetitions remain accounted in their
  own hashed artifact. No business row or coordinates copied into Git/logs.
- Input CSV SHA256 `cecf8ac7d4eadddc65c7238f19c7a63498af3fc0273abbfc9828803a2cf67b7d`;
  prepared manifest hash `4cfde2093b610127112880c79c0dd82e2c79876ea7564e994441035bb82119ba`;
  assay output12,376,488 bytes, SHA256 `6a973a4f7a9e25509ccf357f7fb2655336c9ed5c011a460849cecb45acf4f6e8`.
- Real PostGIS race: native-empty persistence and genuine previous-chain96→97
  upgrade with representative existing history, migration replay, incompatible
  binary refusal, preserved source rows and oversized-publication rollback PASS.
  Platform migration race/recovery suites PASS. Go domain parity, sqlc vet/drift,
  affected vet/staticcheck, formatting/whitespace and scoped secret review PASS.

Source compatibility does not certify sampling policy, score thresholds, complete
historical coverage, automatic station linkage or deployment. No staging migration
or official publication in this task. Native source ingestion and immutable
revision provenance are prerequisites to remaining FQ-12/FQ-17 acceptance.
