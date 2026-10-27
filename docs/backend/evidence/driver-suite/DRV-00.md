# DRV-00 — Foundation reconciliation and contract freeze

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Planning parent: 42250934; runtime parent: f1e1d0db. Selected scope is DRV-00/01/02, not the remaining suite. Owner reaffirmed Premium. [Frozen decisions](../../../product/DRIVER_FOUNDATION_DECISIONS.md) records the three ordered tasks.

## Actual reconciliation

Remote read-only fetch on the isolated clone verified main eee1b33f and dev f1e1d0db; current planning head includes both. Primary checkout e22824db is foreign-dirty. The separate MAP clone is at f1e1d0db with active GuidanceService/download/UI/resources edits. Their branch/worktree contents were not changed or imported. This independent clone uses maintained dev and does not move any other repository's dev ref.

T01 inventory: existing source has financial-v1 in Go metrics/Kotlin PremiumConsumption, MEX-09/10 arithmetic and backend acceptance, MEX-11/12/13 Room/entry/monthly UI and EE charge contracts. LedgerDraftStore is owner/origin scoped; Room is version 27. Existing envelopes lack explicit odometer-unit provenance. The km-labelled form writes a raw integer into a metre field and reopens metres as km; monthly cards print centavo rational strings. These are findings to reproduce in DRV-01, not fixes performed by this documentation.

T02 freeze: Premium/free-held-data boundaries, net/received/payout distinctions, exact units/display, no heuristic legacy conversion, explicit GPS consent with collection absent here, owner/origin/guest, UTC/IANA/monotonic gaps, method separation and append-only recovery are recorded before code. No platform OAuth/API dependency or new capability lease is fabricated.

T03 scenarios: owned/rental/EV/PHEV/GNV/multi-app and incomplete history have explicit synthetic examples. Current input ceilings, charge-specific odometer bound, 50-command batch and 1000/1001-row projection boundary are recorded with ownership. They are not a new capacity claim. Prior actual schema rather than planning-era numbers governs later migrations.

Validation: inspect source/status/diff/new files and applicable policies; fetch current main/dev; verify main/dev ancestry; check rule IDs, examples and local links; git diff --check and changed-surface review. This is docs/contract acceptance, not runtime/device/provider certification. Next: DRV-01 regression RED, then minimal conversion/presentation repair.
