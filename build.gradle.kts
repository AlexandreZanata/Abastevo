plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.android.library) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.hilt) apply false
    alias(libs.plugins.ksp) apply false
}

// Resolve once before app ABI selection and engine configuration. Fresh clones
// consume the exact admitted archive; explicit operators may override its path.
val mapLocalProperties = java.util.Properties().apply {
    val local = rootProject.file("local.properties")
    if (local.isFile) local.inputStream().use { load(it) }
}
val configuredMapSdk = providers.gradleProperty("comapsSdkDir").orNull
    ?: mapLocalProperties.getProperty("comaps.sdk.dir")
    ?: mapLocalProperties.getProperty("comapsSdkDir")
    ?: providers.environmentVariable("COMAPS_SDK_DIR").orNull
val resolvedMapSdk = configuredMapSdk ?: layout.buildDirectory.dir("comaps/7ad828883c4aca8a").get().asFile.absolutePath
if (configuredMapSdk == null) {
    providers.exec {
        commandLine("python3", rootProject.file("scripts/maps/provision-comaps-aar.py"),
            "--aar", rootProject.file("maps/engine/vendor/sdk-release.aar"),
            "--output", resolvedMapSdk)
    }.result.get().assertNormalExitValue()
}
extra["comapsSdkDirResolved"] = resolvedMapSdk.trim()

tasks.register<Exec>("scanSecrets") {
    group = "verification"
    description = "Scans tracked files for accidental secrets"
    commandLine("bash", "scripts/scan-secrets.sh")
    isIgnoreExitValue = false
}

tasks.register("securityCheck") {
    group = "verification"
    description = "Runs the secret scan and the app/data unit tests"
    dependsOn(
        "scanSecrets",
        ":app:testDebugUnitTest",
        ":data:testDebugUnitTest",
    )
}

tasks.register<Exec>("publishGithubRelease") {
    group = "release"
    description = "Show the current Android release target; use scripts/publish-github-release.sh --publish to retry"
    commandLine("bash", "scripts/publish-github-release.sh")
    isIgnoreExitValue = false
}

gradle.projectsEvaluated {
    tasks.findByPath(":app:connectedDebugAndroidTest")?.mustRunAfter(
        ":data:connectedDebugAndroidTest",
    )
}
