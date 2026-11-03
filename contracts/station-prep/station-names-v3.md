# Station assertion v3: sourced names

SNA-01B adds `station-assertion-v3`; v1/v2 bytes/readers remain supported.
The new opt-in registry profile preserves the untrimmed legal name and emits
`legal_name` and `trade_name` strings. Empty trade_name means absent, never an
invented value. `business_name_raw`/`business_name_normalized` retain their old
ANP meaning. A selected trade has `trade_name_provenance`: source (`rfb` or
`anp-survey`), profile, reference, edition, consumed-file SHA-256 and observation
date. Every selected provenance must exactly bind a `name_inputs` manifest pin.
The canonical display name follows B-BR-STATION-NAME-01 in the Go domain.

`prepare_registry` keeps its seven positional arguments. An optional
`--name-inputs <manifest>` selects the new named profile. The descriptor is
`registry-name-inputs-v1`, with `inputs[]` entries carrying source, profile,
file, reference, edition, sha256, bytes and rows. Files are basenames next to
the descriptor, never URLs or parent/absolute paths. Each is read completely
within a frozen 64 MiB/500,000-row budget (256 KiB per JSONL line); the descriptor
is at most 128 KiB and admits at most two distinct sources. Hash/byte/row
mismatch, malformed metadata or ambiguity fails before output publication.
No runtime network, database, API key or unbounded source scan is introduced.

- `rfb-station-names-v1` JSONL rows: schema_version, cnpj, trade_name,
  active (boolean), observed_at (ISO date). No contacts or owner fields.
- `anp-station-price-v1` JSONL uses the existing minimized survey facts:
  cnpj, optional trade_name and observed_at; price/product fields remain
  unrelated to name selection. Observation date chooses the latest source
  value; equally dated conflicting names refuse enrichment.

An active usable RFB name wins, otherwise the latest usable ANP survey name,
otherwise empty trade_name. A newer inactive RFB row prevents reusing its older
active value. Existing tested placeholder/equality/CNPJ rules define usable.
Pins in `name_inputs` are provenance, not extra registry row denominators.
The registry's accepted/duplicate/quarantine accounting remains independently
checked. Checksums and succession links are recomputed deterministically for
v3; old output remains unchanged when the option is absent. Legal-name-only
v3 is permitted with an empty descriptor. Rights admission and acquisition
permission remain separate from successful parsing or checksum verification.
