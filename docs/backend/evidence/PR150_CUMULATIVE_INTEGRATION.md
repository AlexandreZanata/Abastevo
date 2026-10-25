# PR 150 cumulative integration — 2026-10-25

Status: IN_PROGRESS
Integration: INTEGRATION_PENDING

Scope: preserve PAY through `1948035c`, native partial MAP through `e9af87dd`,
enterprise/static portal through `d74334dc`, including FQ-PORTAL-READY.
Primary dirty checkout and occupied dev remain untouched; integration uses
an isolated clone with maintained dev and protected PR 150 -> main.

B-BR-INTEGRATION-01: payment inbox, export and refresh retain independent
serial slots; the general dispatcher cannot steal these jobs.
B-BR-INTEGRATION-02: a fresh CI clone consumes the exact admitted AAR (not
substitute Java stubs), verifies its digest and both ARM cores, and performs
real app/module unit compilation. Public source references, patch, notices
and SBOM accompany the binary; no MWM packages or keys are bundled.
B-BR-INTEGRATION-03: map paths select full quick/mobile guards; unknown areas
remain refused. No required check is suppressed.

Previous Quick failure: real HTTP stream size test returned EOF on an oversized
chunked request after the harness's 30-second timeout. Local pre-change repeat
10 passed; no deterministic production defect is asserted. Isolate connections
and drain response bodies without changing exact admission/status assertions.

Map minimum acceptance remains PARTIAL. PAY-21 sandbox proof, source rights,
precision policy and public portal certification remain owed. Staging origin
`https://teste.abastevo.com.br` is explicitly authorized for subsequent scoped
updates; public-data admission remains unchanged.
