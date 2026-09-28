plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.android.library) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.hilt) apply false
    alias(libs.plugins.ksp) apply false
}

tasks.register<Exec>("scanSecrets") {
    group = "verification"
    description = "Scans tracked files for accidental secrets (Phase 9.4.2)"
    commandLine("bash", "scripts/scan-secrets.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validateRepoBaseline") {
    group = "verification"
    description = "Validates gitignore, cursor rules, and commit conventions (Phase 0.1 / Gate 0.1)"
    commandLine("bash", "scripts/validate-repo-baseline.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validatePocResults") {
    group = "verification"
    description = "Validates POC results documentation in .local/poc-results/ (Appendix B)"
    commandLine("bash", "scripts/validate-poc-results.sh")
    isIgnoreExitValue = false
}

tasks.register("securityCheck") {
    group = "verification"
    description = "Runs security verification tasks (Phase 9.4)"
    dependsOn(
        "scanSecrets",
        ":app:testDebugUnitTest",
        ":data:testDebugUnitTest",
    )
}

tasks.register<Exec>("validatePlayStoreListing") {
    group = "verification"
    description = "Validates Play Store listing draft limits and disclaimers (Phase 10.4)"
    commandLine("bash", "scripts/validate-play-store-listing.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validateWeekCatalogPoc") {
    group = "verification"
    description = "Live ANP listing week catalog validation (Phase 12.2.5)"
    commandLine("bash", "scripts/validate-week-catalog-poc.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validateReleaseBuild") {
    group = "verification"
    description = "Builds and validates signed release APK/AAB (Phase 10.5)"
    commandLine("bash", "scripts/validate-release-build.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validateGithubReleaseNotes") {
    group = "verification"
    description = "Validates GitHub Release v2.0.0 draft notes and disclaimers (Phase 15 / Gate 15)"
    commandLine("bash", "scripts/validate-github-release-notes.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validateReleaseTag") {
    group = "verification"
    description = "Validates v2.0.0 tag readiness — version, license, disclaimers (Gate 15)"
    commandLine("bash", "scripts/validate-release-tag.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("createReleaseTag") {
    group = "release"
    description = "Runs Gate 15 checks, signed release build, and creates annotated tag v2.0.0"
    commandLine("bash", "scripts/create-release-tag.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("validateGate15Release") {
    group = "verification"
    description = "Validates Gate 15 v2.0.0 criteria — tag, artifacts, release notes (Phase R2)"
    commandLine("bash", "scripts/validate-gate-15-release.sh")
    isIgnoreExitValue = false
}

tasks.register<Exec>("publishGithubRelease") {
    group = "release"
    description = "Dry-run GitHub Release publish for v2.0.0 (pass --args='--publish' to release)"
    commandLine("bash", "scripts/publish-github-release.sh")
    isIgnoreExitValue = false
}

gradle.projectsEvaluated {
    tasks.findByPath(":app:connectedDebugAndroidTest")?.mustRunAfter(
        ":data:connectedDebugAndroidTest",
    )
}
