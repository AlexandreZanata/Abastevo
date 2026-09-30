# P09-T05 wiki overview ownership evidence

Date: 2026-09-30. Issue #15, phase PR #14. Scope: bounded snapshot configuration for generated overview page; preserve existing unmanaged Home.md and Fase-P01-Fundacoes-do-Backend.md.

The independently verified wiki origin is git@github.com:AlexandreZanata/brazil-fuel-prices.wiki.git; existing HEAD c7eef0d0c8a82d86d8f9ea94c5ff4e1b54a4b376. Read-only preview correctly refused unmanaged Home.md. No wiki mutation occurred.

Tests-first RED: new configured-overview campaign failed seven cases against the old publisher, including overview creation, internal/sidebar links, invalid path and source-SHA selection. GREEN: `bash scripts/tests/test-wiki.sh` reports **39 passed, 0 failed**, preserving previous conflict/owned-deletion/dry-run/no-op/secret tests. `bash -n scripts/wiki.sh scripts/tests/test-wiki.sh` passes. New cases prove byte-for-byte manual Home preservation, no manifest adoption, bounded filename/traversal refusal, rewritten navigation and configuration read from the requested committed SHA rather than the current tree.

The exporter now reads only overview_page from docs/planning/wiki-config.json at the selected source SHA. Project-Overview.md holds generated README content. Normal collision/refusal guards remain. No blanket delete or edited-page overwrite is added. Preview and actual one-time publication of the merged snapshot, with manual-page hash verification, are recorded in PR metadata after protected integration; this document does not preclaim wiki success.
