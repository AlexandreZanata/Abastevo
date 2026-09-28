package com.anpfuel.data.remote

import java.time.LocalDate

internal fun formatWeekCatalogPocMarkdown(
    comparison: WeekCatalogComparison,
    validatedOn: LocalDate = LocalDate.now(),
): String = buildString {
    appendLine("# Week catalog POC results")
    appendLine()
    appendLine("> **Validated:** $validatedOn")
    appendLine("> **Phase:** 12.2.5 — live ANP listing week catalog discovery")
    appendLine()
    appendLine("## Gate 12.2.5 checklist")
    appendLine()
    appendLine("| Criterion | Result | Evidence |")
    appendLine("|-----------|--------|----------|")
    appendLine(
        "| Live listing HTTP 200 | **Pass** | `scripts/validate-week-catalog-poc.sh` |",
    )
    appendLine(
        "| Catalog ≥ 50 weeks (Gate 12.2) | **${if (comparison.catalogEntryCount >= 50) "Pass" else "Fail"}** | " +
            "${comparison.catalogEntryCount} `SurveyWeekCatalogEntry` rows |",
    )
    appendLine(
        "| Catalog matches complete visible week blocks | **" +
            "${if (comparison.catalogEntryCount == comparison.completeVisibleWeekBlockCount) "Pass" else "Fail"}** | " +
            "catalog=${comparison.catalogEntryCount}, complete blocks=${comparison.completeVisibleWeekBlockCount} |",
    )
    appendLine(
        "| Catalog ≤ visible week headers | **" +
            "${if (comparison.catalogEntryCount <= comparison.visibleWeekHeaderCount) "Pass" else "Fail"}** | " +
            "visible headers=${comparison.visibleWeekHeaderCount} |",
    )
    appendLine()
    appendLine("## Live counts")
    appendLine()
    appendLine("| Metric | Count |")
    appendLine("|--------|------:|")
    appendLine("| Visible week headers on listing page | ${comparison.visibleWeekHeaderCount} |")
    appendLine("| Complete visible week blocks (summary + station links) | ${comparison.completeVisibleWeekBlockCount} |")
    appendLine("| Discovered `SurveyWeekCatalogEntry` | ${comparison.catalogEntryCount} |")
    appendLine()
    appendLine("## Automated validation")
    appendLine()
    appendLine("```bash")
    appendLine("./scripts/validate-week-catalog-poc.sh")
    appendLine("```")
    appendLine()
    appendLine("Or run the JVM live probe directly:")
    appendLine()
    appendLine("```bash")
    appendLine("ANP_LIVE_POC=true ANP_LIVE_HTML_PATH=/path/to/listing.html \\")
    appendLine("  ./gradlew :data:testDebugUnitTest \\")
    appendLine("  --tests com.anpfuel.data.remote.AnpListingLiveCatalogValidationTest")
    appendLine("```")
    appendLine()
    appendLine("User-Agent: `AnpFuel/1.0 (Android; open-source fuel price reader)`")
}
